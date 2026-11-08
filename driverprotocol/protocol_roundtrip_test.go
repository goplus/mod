/*
 * Copyright (c) 2026 The XGo Authors (xgo.dev). All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package driverprotocol

import (
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/goplus/mod/xgomod"
)

func TestRoundTripRunReplacement(t *testing.T) {
	want := testRequest()
	args, err := Encode(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Parse(args)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip = %#v, want %#v", got, want)
	}
	joined := strings.Join(args, "\n")
	if strings.Contains(joined, "selected-dir") || !strings.Contains(joined, "--replace-dir="+testPath("workspace", "framework")) {
		t.Fatalf("replacement identity was flattened:\n%s", joined)
	}
	if got.ApplicationArgs[0] != "" || got.ApplicationArgs[2] != "--" {
		t.Fatalf("application argv changed: %#v", got.ApplicationArgs)
	}
}

func TestParseAcceptsReorderedOptions(t *testing.T) {
	want := testRequest()
	reordered, err := Encode(want)
	if err != nil {
		t.Fatal(err)
	}
	reordered[2], reordered[3] = reordered[3], reordered[2]
	secondGraph, firstBuild := -1, -1
	graphCount := 0
	for i, arg := range reordered {
		switch {
		case strings.HasPrefix(arg, "--graph-flag="):
			graphCount++
			if graphCount == 2 {
				secondGraph = i
			}
		case firstBuild < 0 && strings.HasPrefix(arg, "--build-flag="):
			firstBuild = i
		}
	}
	if secondGraph < 0 || firstBuild < 0 {
		t.Fatal("test request needs two graph flags and one build flag")
	}
	reordered[secondGraph], reordered[firstBuild] = reordered[firstBuild], reordered[secondGraph]

	got, err := Parse(reordered)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Graph.Flags, want.Graph.Flags) {
		t.Fatalf("graph flags = %#v, want %#v", got.Graph.Flags, want.Graph.Flags)
	}
	if !reflect.DeepEqual(got.BuildFlags, want.BuildFlags) {
		t.Fatalf("build flags = %#v, want %#v", got.BuildFlags, want.BuildFlags)
	}
	if !reflect.DeepEqual(got.ApplicationArgs, want.ApplicationArgs) {
		t.Fatalf("application args = %#v, want %#v", got.ApplicationArgs, want.ApplicationArgs)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Parse(reordered) = %#v, want %#v", got, want)
	}
}

func TestRoundTripBuildSelectedWithoutPack(t *testing.T) {
	want := testRequest()
	want.Action = ActionBuild
	want.ApplicationArgs = nil
	want.Project.Pack = nil
	want.DriverOrigin = xgomod.ResolvedModule{
		Selected: xgomod.ModuleRef{
			Path: "example.test/framework", Version: "v1.2.3",
			Dir: testPath("workspace", "framework"), GoMod: testPath("workspace", "framework", "go.mod"),
		},
	}
	want.Output = &BuildOutput{
		Staging: testPath("workspace", "out", ".game.tmp"),
		Final:   testPath("workspace", "out", "game"),
	}
	args, err := Encode(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Parse(args)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip = %#v, want %#v", got, want)
	}
	joined := strings.Join(args, "\n")
	if strings.Contains(joined, "--pack-") || strings.Contains(joined, "--replace-") || strings.Contains(joined, "\n--\n") {
		t.Fatalf("optional/action fields leaked:\n%s", joined)
	}
}

func TestRoundTripOriginVariantsAndWorkspace(t *testing.T) {
	tests := map[string]xgomod.ResolvedModule{
		"main": {
			Selected: xgomod.ModuleRef{
				Path: "example.test/framework", Dir: testPath("workspace", "framework"), GoMod: testPath("workspace", "framework", "go.mod"),
			},
			Main: true,
		},
		"version replacement": {
			Selected: xgomod.ModuleRef{Path: "example.test/framework", Version: "v1.2.3"},
			Replace: &xgomod.ModuleRef{
				Path: "example.test/framework-fork", Version: "v1.4.0",
				Dir: testPath("workspace", "framework-fork"), GoMod: testPath("workspace", "framework-fork", "go.mod"),
			},
		},
	}
	for name, origin := range tests {
		t.Run(name, func(t *testing.T) {
			want := testRequest()
			want.DriverOrigin = origin
			want.Declaration.Path = filepath.Join(origin.Effective().Dir, "gox.mod")
			want.Graph.GoWork = testPath("workspace", "go.work")
			want.Graph.Flags = append(want.Graph.Flags, "-overlay="+testPath("workspace", "overlay.json"))
			args, err := Encode(want)
			if err != nil {
				t.Fatal(err)
			}
			wantOriginMain := "--origin-main=false"
			if origin.Main {
				wantOriginMain = "--origin-main=true"
			}
			if !slices.Contains(args, wantOriginMain) {
				t.Fatalf("Encode() omitted canonical %q: %#v", wantOriginMain, args)
			}
			got, err := Parse(args)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("round trip = %#v, want %#v", got, want)
			}
		})
	}
}

func TestValidationIsStructural(t *testing.T) {
	request := testRequest()
	request.Project.Dir = testPath("does", "not", "exist", "game")
	request.Project.File = testPath("does", "not", "exist", "game", "main.foo")
	request.Project.ModuleRoot = testPath("does", "not", "exist")
	request.Declaration.Path = testPath("does", "not", "exist", "framework", "gox.mod")
	request.TargetModFile.Path = testPath("does", "not", "exist", "app", "go.mod")
	request.DriverOrigin.Replace.Path = testPath("does", "not", "exist", "framework")
	request.DriverOrigin.Replace.Dir = testPath("does", "not", "exist", "framework")
	request.DriverOrigin.Replace.GoMod = testPath("does", "not", "exist", "framework", "go.mod")
	if err := request.Validate(); err != nil {
		t.Fatalf("structural validation consulted ambient filesystem: %v", err)
	}
}

func TestTargetModFileDoesNotRequireContainment(t *testing.T) {
	request := testRequest()
	request.TargetModFile.Path = testPath("module-cache", "cache", "download", "framework", "@v", "v1.2.3.mod")
	args, err := Encode(request)
	if err != nil {
		t.Fatalf("Encode() imposed target modfile containment: %v", err)
	}
	got, err := Parse(args)
	if err != nil {
		t.Fatal(err)
	}
	if got.TargetModFile != request.TargetModFile {
		t.Fatalf("target modfile = %#v, want %#v", got.TargetModFile, request.TargetModFile)
	}
}

func TestPackDotIsDriverNeutral(t *testing.T) {
	request := testRequest()
	request.Project.Pack.Directory = "."
	args, err := Encode(request)
	if err != nil {
		t.Fatalf("Encode() rejected modfile-valid pack directory dot: %v", err)
	}
	if _, err := Parse(args); err != nil {
		t.Fatalf("Parse() rejected modfile-valid pack directory dot: %v", err)
	}
}

func TestEncodeDeterministicAndDetached(t *testing.T) {
	request := testRequest()
	first, err := Encode(request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Encode(request)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("Encode is not deterministic:\n%#v\n%#v", first, second)
	}
	parsed, err := Parse(first)
	if err != nil {
		t.Fatal(err)
	}
	first[len(first)-1] = "changed"
	if parsed.ApplicationArgs[len(parsed.ApplicationArgs)-1] != "--" {
		t.Fatalf("Parse retained argv backing storage: %#v", parsed.ApplicationArgs)
	}
}

func TestV1GoldenArgv(t *testing.T) {
	request := testRequest()
	want := []string{
		"xgo-driver-v1",
		"run",
		"--project-dir=" + testPath("workspace", "app", "game"),
		"--project-file=" + testPath("workspace", "app", "game", "main.foo"),
		"--module-root=" + testPath("workspace", "app"),
		"--driver-package=example.test/framework/cmd/driver",
		"--selected-path=example.test/framework",
		"--selected-version=v1.2.3",
		"--origin-main=false",
		"--replace-path=" + testPath("workspace", "framework"),
		"--replace-version=",
		"--replace-dir=" + testPath("workspace", "framework"),
		"--replace-gomod=" + testPath("workspace", "framework", "go.mod"),
		"--project-ext=.foo",
		"--project-full-ext=*.foo",
		"--pack-dir=payload",
		"--pack-index=index.data",
		"--declaration-file=" + testPath("workspace", "framework", "gox.mod"),
		"--declaration-sha256=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"--target-modfile=" + testPath("workspace", "app", "alt.mod"),
		"--target-modfile-sha256=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"--go-command=" + testPath("usr", "bin", "go"),
		"--graph-work-dir=" + testPath("workspace", "app"),
		"--go-work=off",
		"--graph-flag=-mod=readonly",
		"--graph-flag=-modfile=" + testPath("workspace", "app", "alt.mod"),
		"--build-flag=-v=true",
		"--build-flag=-trimpath=true",
		"--build-flag=-buildvcs=false",
		"--",
		"",
		"a b",
		"--",
	}
	got, err := Encode(request)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Encode() = %#v, want %#v", got, want)
	}
	parsed, err := Parse(want)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(parsed, request) {
		t.Fatalf("Parse(golden) = %#v, want %#v", parsed, request)
	}
}

func FuzzRequestRoundTrip(f *testing.F) {
	for selector := byte(0); selector < 16; selector++ {
		f.Add(selector, "payload/data", "index.data", "", "--")
	}
	f.Fuzz(func(t *testing.T, selector byte, packDirectory, packIndex, firstArg, secondArg string) {
		request := conformanceRequest(selector)
		if request.Project.Pack != nil {
			request.Project.Pack.Directory = packDirectory
			request.Project.Pack.IndexFile = packIndex
		}
		if request.Action == ActionRun {
			request.ApplicationArgs = []string{firstArg, secondArg}
		}
		if err := request.Validate(); err != nil {
			return
		}
		args, err := Encode(request)
		if err != nil {
			t.Fatalf("Encode(valid request): %v", err)
		}
		got, err := Parse(args)
		if err != nil {
			t.Fatalf("Parse(Encode(request)): %v", err)
		}
		if !reflect.DeepEqual(got, request) {
			t.Fatalf("Parse(Encode(request)) = %#v, want %#v", got, request)
		}
	})
}

func conformanceRequest(selector byte) Request {
	request := testRequest()
	switch selector % 4 {
	case 0:
		request.DriverOrigin = xgomod.ResolvedModule{
			Selected: xgomod.ModuleRef{
				Path: "example.test/framework", Version: "v1.2.3",
				Dir: testPath("workspace", "framework"), GoMod: testPath("workspace", "framework", "go.mod"),
			},
		}
	case 1:
		request.DriverOrigin = xgomod.ResolvedModule{
			Selected: xgomod.ModuleRef{
				Path: "example.test/framework",
				Dir:  testPath("workspace", "framework"), GoMod: testPath("workspace", "framework", "go.mod"),
			},
			Main: true,
		}
	case 2:
	case 3:
		request.DriverOrigin = xgomod.ResolvedModule{
			Selected: xgomod.ModuleRef{Path: "example.test/framework", Version: "v1.2.3"},
			Replace: &xgomod.ModuleRef{
				Path: "example.test/framework-fork", Version: "v1.4.0",
				Dir: testPath("workspace", "framework-fork"), GoMod: testPath("workspace", "framework-fork", "go.mod"),
			},
		}
	}
	request.Declaration.Path = filepath.Join(request.DriverOrigin.Effective().Dir, "gox.mod")
	if selector&4 != 0 {
		request.Action = ActionBuild
		request.ApplicationArgs = nil
		request.Output = &BuildOutput{
			Staging: testPath("workspace", "out", ".game.tmp"),
			Final:   testPath("workspace", "out", "game"),
		}
	}
	if selector&8 != 0 {
		request.Project.Pack = nil
	}
	return request
}
