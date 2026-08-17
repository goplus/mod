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

package runtimeprotocol

import (
	"reflect"
	"strings"
	"testing"

	"github.com/goplus/mod/xgomod"
)

func testRequest() Request {
	return Request{
		Version: Version1,
		Action:  ActionRun,
		Project: Project{
			Dir:           "/workspace/app/game",
			File:          "/workspace/app/game/main.foo",
			ModuleRoot:    "/workspace/app",
			Extension:     ".foo",
			FullExtension: "*.foo",
			Pack:          &Pack{Directory: "payload", IndexFile: "index.data"},
		},
		ProviderPackage: "example.test/framework/cmd/provider",
		ProviderOrigin: xgomod.ResolvedModule{
			Selected: xgomod.ModuleRef{Path: "example.test/framework", Version: "v1.2.3"},
			Replace:  &xgomod.ModuleRef{Path: "/workspace/framework", Dir: "/workspace/framework", GoMod: "/workspace/framework/go.mod"},
		},
		Declaration:     xgomod.FileIdentity{Path: "/workspace/framework/gox.mod", SHA256: strings.Repeat("a", 64)},
		Graph:           Graph{GoCommand: "/usr/bin/go", WorkDir: "/workspace/app", GoWork: "off", Flags: []string{"-mod=readonly", "-modfile=/workspace/app/alt.mod"}},
		BuildFlags:      []string{"-v=true", "-trimpath=true", "-buildvcs=false"},
		ApplicationArgs: []string{"", "a b", "--"},
	}
}

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
	if strings.Contains(joined, "selected-dir") || !strings.Contains(joined, "--replace-dir=/workspace/framework") {
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
	want.ProviderOrigin = xgomod.ResolvedModule{
		Selected: xgomod.ModuleRef{
			Path: "example.test/framework", Version: "v1.2.3",
			Dir: "/workspace/framework", GoMod: "/workspace/framework/go.mod",
		},
	}
	want.Output = &BuildOutput{Staging: "/workspace/out/.game.tmp", Final: "/workspace/out/game"}
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
				Path: "example.test/framework", Dir: "/workspace/framework", GoMod: "/workspace/framework/go.mod",
			},
			Main: true,
		},
		"version replacement": {
			Selected: xgomod.ModuleRef{Path: "example.test/framework", Version: "v1.2.3"},
			Replace: &xgomod.ModuleRef{
				Path: "example.test/framework-fork", Version: "v1.4.0",
				Dir: "/workspace/framework-fork", GoMod: "/workspace/framework-fork/go.mod",
			},
		},
	}
	for name, origin := range tests {
		t.Run(name, func(t *testing.T) {
			want := testRequest()
			want.ProviderOrigin = origin
			want.Declaration.Path = origin.Effective().Dir + "/gox.mod"
			want.Graph.GoWork = "/workspace/go.work"
			want.Graph.Flags = append(want.Graph.Flags, "-overlay=/workspace/overlay.json")
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
	request.Project.Dir = "/does/not/exist/game"
	request.Project.File = "/does/not/exist/game/main.foo"
	request.Project.ModuleRoot = "/does/not/exist"
	request.Declaration.Path = "/does/not/exist/framework/gox.mod"
	request.ProviderOrigin.Replace.Path = "/does/not/exist/framework"
	request.ProviderOrigin.Replace.Dir = "/does/not/exist/framework"
	request.ProviderOrigin.Replace.GoMod = "/does/not/exist/framework/go.mod"
	if err := request.Validate(); err != nil {
		t.Fatalf("structural validation consulted ambient filesystem: %v", err)
	}
}

func TestPackDotIsProviderNeutral(t *testing.T) {
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

func TestRejectMalformedArgv(t *testing.T) {
	valid, err := Encode(testRequest())
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string][]string{
		"unknown":          append(append([]string(nil), valid[:len(valid)-4]...), "--unknown=value", "--", ""),
		"duplicate":        append(append([]string(nil), valid[:2]...), append([]string{valid[2]}, valid[2:]...)...),
		"partial pack":     removeOption(valid, "--pack-index="),
		"partial replace":  removeOption(valid, "--replace-gomod="),
		"missing work dir": removeOption(valid, "--graph-work-dir="),
		"uppercase digest": replaceOptionValue(valid, "--declaration-sha256=", strings.Repeat("A", 64)),
		"missing delimiter": func() []string {
			copy := append([]string(nil), valid...)
			for i, value := range copy {
				if value == "--" {
					return copy[:i]
				}
			}
			return copy
		}(),
	}
	for name, args := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(args); err == nil {
				t.Fatalf("Parse(%#v) succeeded", args)
			}
		})
	}
}

func TestRejectInvalidRequestShapes(t *testing.T) {
	tests := map[string]func(*Request){
		"unsupported version": func(r *Request) { r.Version = "v2" },
		"build without output": func(r *Request) {
			r.Action = ActionBuild
			r.ApplicationArgs = nil
		},
		"run with output":         func(r *Request) { r.Output = &BuildOutput{Staging: "/tmp/a", Final: "/tmp/b"} },
		"bad graph flag":          func(r *Request) { r.Graph.Flags = []string{"-modfile=relative.mod"} },
		"relative graph work dir": func(r *Request) { r.Graph.WorkDir = "relative" },
		"bad build flag":          func(r *Request) { r.BuildFlags = []string{"-ldflags=-s"} },
		"duplicate flag":          func(r *Request) { r.BuildFlags = []string{"-v=true", "-v=true"} },
		"provider outside module": func(r *Request) { r.ProviderPackage = "example.test/other/cmd/provider" },
		"flattened replacement":   func(r *Request) { r.ProviderOrigin.Selected.Dir = "/workspace/framework" },
		"pack escapes":            func(r *Request) { r.Project.Pack.Directory = "../payload" },
		"uppercase digest":        func(r *Request) { r.Declaration.SHA256 = strings.Repeat("A", 64) },
		"declaration outside provider": func(r *Request) {
			r.Declaration.Path = "/workspace/other/gox.mod"
		},
		"main origin with version": func(r *Request) {
			r.ProviderOrigin = xgomod.ResolvedModule{
				Selected: xgomod.ModuleRef{Path: "example.test/framework", Version: "v1.2.3", Dir: "/workspace/framework", GoMod: "/workspace/framework/go.mod"}, Main: true,
			}
		},
		"local replace with module path": func(r *Request) {
			r.ProviderOrigin.Replace.Path = "example.test/framework-fork"
		},
		"local replace identity mismatch": func(r *Request) {
			r.ProviderOrigin.Replace.Path = "/workspace/other-framework"
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			request := testRequest()
			mutate(&request)
			if _, err := Encode(request); err == nil {
				t.Fatal("Encode succeeded")
			}
		})
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

func removeOption(args []string, prefix string) []string {
	result := make([]string, 0, len(args))
	for _, arg := range args {
		if !strings.HasPrefix(arg, prefix) {
			result = append(result, arg)
		}
	}
	return result
}

func replaceOptionValue(args []string, prefix, value string) []string {
	result := append([]string(nil), args...)
	for i, arg := range result {
		if strings.HasPrefix(arg, prefix) {
			result[i] = prefix + value
			return result
		}
	}
	return result
}
