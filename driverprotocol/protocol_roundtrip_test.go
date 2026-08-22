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
	request.DriverOrigin.Replace.Path = testPath("does", "not", "exist", "framework")
	request.DriverOrigin.Replace.Dir = testPath("does", "not", "exist", "framework")
	request.DriverOrigin.Replace.GoMod = testPath("does", "not", "exist", "framework", "go.mod")
	if err := request.Validate(); err != nil {
		t.Fatalf("structural validation consulted ambient filesystem: %v", err)
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
