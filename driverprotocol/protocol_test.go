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

	"github.com/goplus/mod/xgomod"
)

func testPath(parts ...string) string {
	path, err := filepath.Abs(filepath.Join(append([]string{"driverprotocol-fixture"}, parts...)...))
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
		DriverPackage: "example.test/framework/cmd/driver",
		DriverOrigin: xgomod.ResolvedModule{
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
		TargetModFile: xgomod.FileIdentity{
			Path:   testPath("workspace", "app", "alt.mod"),
			SHA256: strings.Repeat("b", 64),
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
