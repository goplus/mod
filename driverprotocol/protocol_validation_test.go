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
	"fmt"
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
			name: "relative target modfile",
			mutate: func(r *Request) {
				r.TargetModFile.Path = "workspace/app/go.mod"
			},
			want: "path --target-modfile must be absolute",
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
			name: "volume relative pack directory",
			mutate: func(r *Request) {
				r.Project.Pack.Directory = "C:payload"
			},
			want: "non-portable path element",
		},
		{
			name: "drive rooted pack directory",
			mutate: func(r *Request) {
				r.Project.Pack.Directory = "C:/payload"
			},
			want: "non-portable path element",
		},
		{
			name: "reserved device pack directory",
			mutate: func(r *Request) {
				r.Project.Pack.Directory = "payload/CON/cache"
			},
			want: "non-portable path element",
		},
		{
			name: "reserved device pack directory with extension",
			mutate: func(r *Request) {
				r.Project.Pack.Directory = "payload/lpt1.data"
			},
			want: "non-portable path element",
		},
		{
			name: "windows-normalized pack directory",
			mutate: func(r *Request) {
				r.Project.Pack.Directory = "payload/cache."
			},
			want: "non-portable path element",
		},
		{
			name: "invalid pack index",
			mutate: func(r *Request) {
				r.Project.Pack.IndexFile = "index/data"
			},
			want: "pack index must be a plain file name",
		},
		{
			name: "reserved device pack index",
			mutate: func(r *Request) {
				r.Project.Pack.IndexFile = "nul.json"
			},
			want: "pack index must be a plain file name",
		},
		{
			name: "alternate data stream pack index",
			mutate: func(r *Request) {
				r.Project.Pack.IndexFile = "index.json:stream"
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
			name: "short target modfile digest",
			mutate: func(r *Request) {
				r.TargetModFile.SHA256 = strings.Repeat("b", 63)
			},
			want: "--target-modfile-sha256 must contain 64 hexadecimal characters",
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
		{name: "unsupported version", mutate: func(r *Request) { r.Version = "v2" }, want: "unsupported version"},
		{name: "build without output", mutate: func(r *Request) { r.Action, r.ApplicationArgs = ActionBuild, nil }, want: "build request requires output"},
		{name: "run with output", mutate: func(r *Request) { r.Output = &BuildOutput{Staging: testPath("tmp", "a"), Final: testPath("tmp", "b")} }, want: "run request cannot contain output"},
		{name: "relative modfile", mutate: func(r *Request) { r.Graph.Flags = []string{"-modfile=relative.mod"} }, want: "must be absolute"},
		{name: "relative graph work dir", mutate: func(r *Request) { r.Graph.WorkDir = "relative" }, want: "path --graph-work-dir must be absolute"},
		{name: "duplicate build flag", mutate: func(r *Request) { r.BuildFlags = []string{"-v=true", "-v=true"} }, want: "may not be repeated"},
		{name: "driver outside module", mutate: func(r *Request) { r.DriverPackage = "example.test/other/cmd/driver" }, want: "outside selected module"},
		{name: "flattened replacement", mutate: func(r *Request) { r.DriverOrigin.Selected.Dir = testPath("workspace", "framework") }, want: "selected Dir/GoMod must be empty"},
		{name: "uppercase digest", mutate: func(r *Request) { r.Declaration.SHA256 = strings.Repeat("A", 64) }, want: "lowercase hexadecimal"},
		{name: "declaration outside driver", mutate: func(r *Request) { r.Declaration.Path = testPath("workspace", "other", "gox.mod") }, want: "declaration-file must be driver metadata"},
		{
			name: "main origin with version",
			mutate: func(r *Request) {
				r.DriverOrigin = xgomod.ResolvedModule{
					Selected: xgomod.ModuleRef{Path: "example.test/framework", Version: "v1.2.3", Dir: testPath("workspace", "framework"), GoMod: testPath("workspace", "framework", "go.mod")},
					Main:     true,
				}
			},
			want: "main module selected version must be empty",
		},
		{name: "local replace with module path", mutate: func(r *Request) { r.DriverOrigin.Replace.Path = "example.test/framework-fork" }, want: "local replacement.Path must be"},
		{name: "local replace identity mismatch", mutate: func(r *Request) { r.DriverOrigin.Replace.Path = testPath("workspace", "other-framework") }, want: "replacement.Path and replacement.Dir"},
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

func TestActionValidate(t *testing.T) {
	tests := []struct {
		action Action
		valid  bool
	}{
		{ActionRun, true},
		{ActionBuild, true},
		{"", false},
		{"publish", false},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("%q", test.action), func(t *testing.T) {
			err := test.action.Validate()
			if (err == nil) != test.valid {
				t.Fatalf("Action(%q).Validate() = %v, valid = %v", test.action, err, test.valid)
			}
			if test.valid {
				return
			}
			request := testRequest()
			request.Action = test.action
			requestErr := request.Validate()
			if requestErr == nil || requestErr.Error() != err.Error() {
				t.Fatalf("Request.Validate() = %v, Action.Validate() = %v", requestErr, err)
			}
			_, parseErr := Parse([]string{PreambleV1, string(test.action)})
			if parseErr == nil || parseErr.Error() != err.Error() {
				t.Fatalf("Parse() = %v, Action.Validate() = %v", parseErr, err)
			}
		})
	}
}

func TestPortablePackPathElements(t *testing.T) {
	tests := []struct {
		element string
		want    bool
	}{
		{"assets", true},
		{"name.txt", true},
		{"name .txt", true},
		{"console", true},
		{"auxiliary", true},
		{"nulled.json", true},
		{"CONSOLE .txt", true},
		{"COM", true},
		{"COM0", true},
		{"COM01", true},
		{"com10", true},
		{"COM10 .txt", true},
		{"computer", true},
		{"LPT", true},
		{"LPT0", true},
		{"lpt10", true},
		{"LPT10 .txt", true},
		{"COM\u2074", true},
		{"LPT\u00b2x", true},
		{"", false},
		{"cache.", false},
		{"cache ", false},
		{"index:stream", false},
		{"CON", false},
		{"prn.txt", false},
		{"Aux", false},
		{"nul.json", false},
		{"CLOCK$", false},
		{"conin$.log", false},
		{"ConOut$ .txt", false},
		{"CON .txt", false},
		{"COM1", false},
		{"com9.data", false},
		{"COM1 .data", false},
		{"LPT1", false},
		{"lpt9.data", false},
		{"LPT1 .data", false},
		{"COM\u00b9", false},
		{"com\u00b2.data", false},
		{"COM\u00b3", false},
		{"LPT\u00b9", false},
		{"lpt\u00b2.data", false},
		{"LPT\u00b3", false},
	}
	for _, test := range tests {
		if got := isPortablePathElement(test.element); got != test.want {
			t.Errorf("isPortablePathElement(%q) = %t, want %t", test.element, got, test.want)
		}
	}
}
