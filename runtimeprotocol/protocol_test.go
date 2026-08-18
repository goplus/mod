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
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/goplus/mod/xgomod"
)

func testPath(parts ...string) string {
	path, err := filepath.Abs(filepath.Join(append([]string{"runtimeprotocol-fixture"}, parts...)...))
	if err != nil {
		panic(err)
	}
	return path
}

func testRequest() Request {
	return Request{
		Version: Version1,
		Action:  ActionRun,
		Project: Project{
			Dir:           testPath("workspace", "app", "game"),
			File:          testPath("workspace", "app", "game", "main.foo"),
			ModuleRoot:    testPath("workspace", "app"),
			Extension:     ".foo",
			FullExtension: "*.foo",
			Pack:          &Pack{Directory: "payload", IndexFile: "index.data"},
		},
		ProviderPackage: "example.test/framework/cmd/provider",
		ProviderOrigin: xgomod.ResolvedModule{
			Selected: xgomod.ModuleRef{Path: "example.test/framework", Version: "v1.2.3"},
			Replace: &xgomod.ModuleRef{
				Path:  testPath("workspace", "framework"),
				Dir:   testPath("workspace", "framework"),
				GoMod: testPath("workspace", "framework", "go.mod"),
			},
		},
		Declaration: xgomod.FileIdentity{
			Path:   testPath("workspace", "framework", "gox.mod"),
			SHA256: strings.Repeat("a", 64),
		},
		Graph: Graph{
			GoCommand: testPath("usr", "bin", "go"),
			WorkDir:   testPath("workspace", "app"),
			GoWork:    "off",
			Flags:     []string{"-mod=readonly", "-modfile=" + testPath("workspace", "app", "alt.mod")},
		},
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
	want.ProviderOrigin = xgomod.ResolvedModule{
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
			want.ProviderOrigin = origin
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
	request.ProviderOrigin.Replace.Path = testPath("does", "not", "exist", "framework")
	request.ProviderOrigin.Replace.Dir = testPath("does", "not", "exist", "framework")
	request.ProviderOrigin.Replace.GoMod = testPath("does", "not", "exist", "framework", "go.mod")
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

func TestValidateRejectsStructuralRequests(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Request)
		want   string
	}{
		{
			name: "unsupported action",
			mutate: func(r *Request) {
				r.Action = Action("test")
			},
			want: "unsupported action",
		},
		{
			name: "empty project directory",
			mutate: func(r *Request) {
				r.Project.Dir = ""
			},
			want: "path --project-dir may not be empty",
		},
		{
			name: "nul project file",
			mutate: func(r *Request) {
				r.Project.File += "\x00"
			},
			want: "path --project-file may not be empty or contain NUL",
		},
		{
			name: "relative module root",
			mutate: func(r *Request) {
				r.Project.ModuleRoot = "workspace/app"
			},
			want: "path --module-root must be absolute",
		},
		{
			name: "unclean declaration file",
			mutate: func(r *Request) {
				r.Declaration.Path = testPath("workspace", "framework") + string(filepath.Separator) + ".." + string(filepath.Separator) + "framework"
			},
			want: "path --declaration-file must be clean",
		},
		{
			name: "unclean go command",
			mutate: func(r *Request) {
				r.Graph.GoCommand = testPath("usr", "bin") + string(filepath.Separator) + ".." + string(filepath.Separator) + "bin" + string(filepath.Separator) + "go"
			},
			want: "path --go-command must be clean",
		},
		{
			name: "nul graph work directory",
			mutate: func(r *Request) {
				r.Graph.WorkDir += "\x00"
			},
			want: "path --graph-work-dir may not be empty or contain NUL",
		},
		{
			name: "nested project file",
			mutate: func(r *Request) {
				r.Project.File = testPath("workspace", "app", "game", "nested", "main.foo")
			},
			want: "project-file must be a top-level file",
		},
		{
			name: "project outside module root",
			mutate: func(r *Request) {
				r.Project.ModuleRoot = testPath("workspace", "other")
			},
			want: "project-dir must be within module-root",
		},
		{
			name: "empty project extension",
			mutate: func(r *Request) {
				r.Project.Extension = ""
			},
			want: "project extension may not be empty",
		},
		{
			name: "nul project extension",
			mutate: func(r *Request) {
				r.Project.Extension = ".foo\x00"
			},
			want: "project extension may not be empty or contain NUL",
		},
		{
			name: "empty full extension",
			mutate: func(r *Request) {
				r.Project.FullExtension = ""
			},
			want: "project full extension may not be empty",
		},
		{
			name: "nul full extension",
			mutate: func(r *Request) {
				r.Project.FullExtension = "*.foo\x00"
			},
			want: "project full extension may not be empty or contain NUL",
		},
		{
			name: "empty pack directory",
			mutate: func(r *Request) {
				r.Project.Pack.Directory = ""
			},
			want: "pack directory must be",
		},
		{
			name: "backslash pack directory",
			mutate: func(r *Request) {
				r.Project.Pack.Directory = `payload\\data`
			},
			want: "pack directory must be",
		},
		{
			name: "absolute pack directory",
			mutate: func(r *Request) {
				r.Project.Pack.Directory = testPath("workspace", "app", "payload")
			},
			want: "pack directory must be",
		},
		{
			name: "unclean pack directory",
			mutate: func(r *Request) {
				r.Project.Pack.Directory = "payload/../payload"
			},
			want: "pack directory must be",
		},
		{
			name: "pack directory escapes project",
			mutate: func(r *Request) {
				r.Project.Pack.Directory = "../payload"
			},
			want: "pack directory escapes",
		},
		{
			name: "invalid pack index",
			mutate: func(r *Request) {
				r.Project.Pack.IndexFile = "index/data"
			},
			want: "pack index must be a plain file name",
		},
		{
			name: "invalid provider origin",
			mutate: func(r *Request) {
				r.ProviderOrigin.Selected.Path = "bad path"
			},
			want: "provider origin",
		},
		{
			name: "declaration outside provider metadata",
			mutate: func(r *Request) {
				r.Declaration.Path = testPath("workspace", "framework", "metadata.txt")
			},
			want: "declaration-file must be provider metadata",
		},
		{
			name: "invalid provider package",
			mutate: func(r *Request) {
				r.ProviderPackage = "bad package"
			},
			want: "invalid provider package",
		},
		{
			name: "relative go work",
			mutate: func(r *Request) {
				r.Graph.GoWork = "workspace/go.work"
			},
			want: "path --go-work must be absolute",
		},
		{
			name: "malformed graph flag",
			mutate: func(r *Request) {
				r.Graph.Flags = []string{"-mod"}
			},
			want: "graph flag",
		},
		{
			name: "duplicate graph flag",
			mutate: func(r *Request) {
				r.Graph.Flags = []string{"-mod=mod", "-mod=readonly"}
			},
			want: "graph flag -mod may not be repeated",
		},
		{
			name: "unsupported graph mode",
			mutate: func(r *Request) {
				r.Graph.Flags = []string{"-mod=bad"}
			},
			want: "graph flag -mod has unsupported value",
		},
		{
			name: "unsupported graph flag",
			mutate: func(r *Request) {
				r.Graph.Flags = []string{"-tags=all"}
			},
			want: "graph flag -tags is not supported",
		},
		{
			name: "malformed build flag",
			mutate: func(r *Request) {
				r.BuildFlags = []string{"-v"}
			},
			want: "build flag",
		},
		{
			name: "unsupported build boolean",
			mutate: func(r *Request) {
				r.BuildFlags = []string{"-v=false"}
			},
			want: "build flag -v has unsupported value",
		},
		{
			name: "unsupported build vcs value",
			mutate: func(r *Request) {
				r.BuildFlags = []string{"-buildvcs=true"}
			},
			want: "build flag -buildvcs has unsupported value",
		},
		{
			name: "unsupported build flag",
			mutate: func(r *Request) {
				r.BuildFlags = []string{"-ldflags=-s"}
			},
			want: "build flag -ldflags is not supported",
		},
		{
			name: "application argument nul",
			mutate: func(r *Request) {
				r.ApplicationArgs = []string{"ok\x00"}
			},
			want: "application argument contains NUL",
		},
		{
			name: "short declaration digest",
			mutate: func(r *Request) {
				r.Declaration.SHA256 = strings.Repeat("a", 63)
			},
			want: "must contain 64 hexadecimal characters",
		},
		{
			name: "non-hex declaration digest",
			mutate: func(r *Request) {
				r.Declaration.SHA256 = strings.Repeat("g", 64)
			},
			want: "is not a SHA-256 digest",
		},
		{
			name: "build application arguments",
			mutate: func(r *Request) {
				r.Action = ActionBuild
				r.Output = &BuildOutput{Staging: testPath("workspace", "out", ".game.tmp"), Final: testPath("workspace", "out", "game")}
			},
			want: "build request cannot contain application arguments",
		},
		{
			name: "empty staging output",
			mutate: func(r *Request) {
				r.Action = ActionBuild
				r.ApplicationArgs = nil
				r.Output = &BuildOutput{Final: testPath("workspace", "out", "game")}
			},
			want: "path --output may not be empty",
		},
		{
			name: "relative staging output",
			mutate: func(r *Request) {
				r.Action = ActionBuild
				r.ApplicationArgs = nil
				r.Output = &BuildOutput{Staging: "out/.game.tmp", Final: testPath("workspace", "out", "game")}
			},
			want: "path --output must be absolute",
		},
		{
			name: "unclean final output",
			mutate: func(r *Request) {
				r.Action = ActionBuild
				r.ApplicationArgs = nil
				r.Output = &BuildOutput{Staging: testPath("workspace", "out", ".game.tmp"), Final: testPath("workspace", "out") + string(filepath.Separator) + ".." + string(filepath.Separator) + "out" + string(filepath.Separator) + "game"}
			},
			want: "path --final-output must be clean",
		},
		{
			name: "same build outputs",
			mutate: func(r *Request) {
				r.Action = ActionBuild
				r.ApplicationArgs = nil
				output := testPath("workspace", "out", "game")
				r.Output = &BuildOutput{Staging: output, Final: output}
			},
			want: "output and final-output must be different",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := testRequest()
			test.mutate(&request)
			if err := request.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestEncodeRejectsInvalidRequest(t *testing.T) {
	request := testRequest()
	request.Action = Action("test")
	if _, err := Encode(request); err == nil || !strings.Contains(err.Error(), "unsupported action") {
		t.Fatalf("Encode() error = %v", err)
	}
}

func TestParseRejectsMalformedRequests(t *testing.T) {
	runArgs, err := Encode(testRequest())
	if err != nil {
		t.Fatal(err)
	}
	buildRequest := testRequest()
	buildRequest.Action = ActionBuild
	buildRequest.ApplicationArgs = nil
	buildRequest.Project.Pack = nil
	buildRequest.ProviderOrigin = xgomod.ResolvedModule{
		Selected: xgomod.ModuleRef{
			Path: "example.test/framework", Version: "v1.2.3",
			Dir: testPath("workspace", "framework"), GoMod: testPath("workspace", "framework", "go.mod"),
		},
	}
	buildRequest.Output = &BuildOutput{
		Staging: testPath("workspace", "out", ".game.tmp"),
		Final:   testPath("workspace", "out", "game"),
	}
	buildArgs, err := Encode(buildRequest)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		args func() []string
		want string
	}{
		{
			name: "empty argv",
			args: func() []string { return nil },
			want: "requires preamble and action",
		},
		{
			name: "preamble only",
			args: func() []string { return []string{PreambleV1} },
			want: "requires preamble and action",
		},
		{
			name: "unsupported preamble",
			args: func() []string {
				args := append([]string(nil), runArgs...)
				args[0] = "other-runtime"
				return args
			},
			want: "unsupported preamble",
		},
		{
			name: "unsupported action",
			args: func() []string { return []string{PreambleV1, "test"} },
			want: "unsupported action",
		},
		{
			name: "build delimiter",
			args: func() []string {
				return append(append([]string(nil), buildArgs...), "--")
			},
			want: "build does not accept",
		},
		{
			name: "run missing delimiter",
			args: func() []string {
				args := append([]string(nil), runArgs...)
				for i, arg := range args {
					if arg == "--" {
						return args[:i]
					}
				}
				return args
			},
			want: "run requires --",
		},
		{
			name: "positional option",
			args: func() []string {
				args := append([]string(nil), runArgs...)
				args[2] = "project-dir"
				return args
			},
			want: "unexpected positional argument",
		},
		{
			name: "malformed option",
			args: func() []string {
				args := append([]string(nil), runArgs...)
				args[2] = "--project-dir"
				return args
			},
			want: "must use --name=value",
		},
		{
			name: "missing required option",
			args: func() []string { return removeOption(runArgs, "--project-dir=") },
			want: "option --project-dir is required",
		},
		{
			name: "invalid origin main",
			args: func() []string { return replaceOptionValue(runArgs, "--origin-main=", "maybe") },
			want: "invalid --origin-main",
		},
		{
			name: "missing selected source",
			args: func() []string {
				args := removeOption(buildArgs, "--selected-dir=")
				return removeOption(args, "--selected-gomod=")
			},
			want: "without replacement requires",
		},
		{
			name: "replacement with selected source",
			args: func() []string {
				args := insertBeforeDelimiter(runArgs, "--selected-dir="+testPath("workspace", "framework"))
				return insertBeforeDelimiter(args, "--selected-gomod="+testPath("workspace", "framework", "go.mod"))
			},
			want: "with replacement forbids",
		},
		{
			name: "missing build output",
			args: func() []string { return removeOption(buildArgs, "--output=") },
			want: "build requires --output and --final-output",
		},
		{
			name: "missing build final output",
			args: func() []string { return removeOption(buildArgs, "--final-output=") },
			want: "build requires --output and --final-output",
		},
		{
			name: "run output",
			args: func() []string {
				return insertBeforeDelimiter(runArgs, "--output="+testPath("workspace", "out", "game"))
			},
			want: "run does not accept --output",
		},
		{
			name: "run final output",
			args: func() []string {
				return insertBeforeDelimiter(runArgs, "--final-output="+testPath("workspace", "out", "game"))
			},
			want: "run does not accept --final-output",
		},
		{
			name: "incomplete replacement",
			args: func() []string { return removeOption(runArgs, "--replace-gomod=") },
			want: "replacement options must be supplied as a complete group",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Parse(test.args()); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Parse() error = %v, want substring %q", err, test.want)
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
		"run with output":         func(r *Request) { r.Output = &BuildOutput{Staging: testPath("tmp", "a"), Final: testPath("tmp", "b")} },
		"bad graph flag":          func(r *Request) { r.Graph.Flags = []string{"-modfile=relative.mod"} },
		"relative graph work dir": func(r *Request) { r.Graph.WorkDir = "relative" },
		"bad build flag":          func(r *Request) { r.BuildFlags = []string{"-ldflags=-s"} },
		"duplicate flag":          func(r *Request) { r.BuildFlags = []string{"-v=true", "-v=true"} },
		"provider outside module": func(r *Request) { r.ProviderPackage = "example.test/other/cmd/provider" },
		"flattened replacement":   func(r *Request) { r.ProviderOrigin.Selected.Dir = testPath("workspace", "framework") },
		"pack escapes":            func(r *Request) { r.Project.Pack.Directory = "../payload" },
		"uppercase digest":        func(r *Request) { r.Declaration.SHA256 = strings.Repeat("A", 64) },
		"declaration outside provider": func(r *Request) {
			r.Declaration.Path = testPath("workspace", "other", "gox.mod")
		},
		"main origin with version": func(r *Request) {
			r.ProviderOrigin = xgomod.ResolvedModule{
				Selected: xgomod.ModuleRef{
					Path: "example.test/framework", Version: "v1.2.3",
					Dir: testPath("workspace", "framework"), GoMod: testPath("workspace", "framework", "go.mod"),
				},
				Main: true,
			}
		},
		"local replace with module path": func(r *Request) {
			r.ProviderOrigin.Replace.Path = "example.test/framework-fork"
		},
		"local replace identity mismatch": func(r *Request) {
			r.ProviderOrigin.Replace.Path = testPath("workspace", "other-framework")
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

func insertBeforeDelimiter(args []string, value string) []string {
	result := make([]string, 0, len(args)+1)
	inserted := false
	for _, arg := range args {
		if !inserted && arg == "--" {
			result = append(result, value)
			inserted = true
		}
		result = append(result, arg)
	}
	if !inserted {
		result = append(result, value)
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
