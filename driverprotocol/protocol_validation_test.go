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
	"strings"
	"testing"

	"github.com/goplus/mod/xgomod"
)

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
			name: "invalid driver origin",
			mutate: func(r *Request) {
				r.DriverOrigin.Selected.Path = "bad path"
			},
			want: "driver origin",
		},
		{
			name: "declaration outside driver metadata",
			mutate: func(r *Request) {
				r.Declaration.Path = testPath("workspace", "framework", "metadata.txt")
			},
			want: "declaration-file must be driver metadata",
		},
		{
			name: "invalid driver package",
			mutate: func(r *Request) {
				r.DriverPackage = "bad package"
			},
			want: "invalid driver package",
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
		"driver outside module":   func(r *Request) { r.DriverPackage = "example.test/other/cmd/driver" },
		"flattened replacement":   func(r *Request) { r.DriverOrigin.Selected.Dir = testPath("workspace", "framework") },
		"pack escapes":            func(r *Request) { r.Project.Pack.Directory = "../payload" },
		"uppercase digest":        func(r *Request) { r.Declaration.SHA256 = strings.Repeat("A", 64) },
		"declaration outside driver": func(r *Request) {
			r.Declaration.Path = testPath("workspace", "other", "gox.mod")
		},
		"main origin with version": func(r *Request) {
			r.DriverOrigin = xgomod.ResolvedModule{
				Selected: xgomod.ModuleRef{
					Path: "example.test/framework", Version: "v1.2.3",
					Dir: testPath("workspace", "framework"), GoMod: testPath("workspace", "framework", "go.mod"),
				},
				Main: true,
			}
		},
		"local replace with module path": func(r *Request) {
			r.DriverOrigin.Replace.Path = "example.test/framework-fork"
		},
		"local replace identity mismatch": func(r *Request) {
			r.DriverOrigin.Replace.Path = testPath("workspace", "other-framework")
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
