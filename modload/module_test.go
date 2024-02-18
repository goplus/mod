/*
 * Copyright (c) 2024 The GoPlus Authors (goplus.org). All rights reserved.
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

package modload

import (
	"encoding/json"
	"log"
	"os"
	"runtime"
	"testing"

	"github.com/goplus/mod"
	"github.com/goplus/mod/env"
	"github.com/goplus/mod/modfile"
	"github.com/qiniu/x/errors"
	gomodfile "golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
)

func TestCheckGopDeps(t *testing.T) {
	mod := Default
	file := *mod.File
	mod.File = &file
	mod.File.Module = &gomodfile.Module{Mod: module.Version{
		Path: "github.com/qiniu/x",
	}}
	if mod.checkGopDeps() != FlagDepModX {
		t.Fatal("checkGopDeps")
	}
}

func TestEmpty(t *testing.T) {
	mod := &Module{File: new(gomodfile.File), Opt: new(modfile.File)}
	if mod.HasModfile() {
		t.Fatal("mod.HasModfile")
	}
	if v := mod.Modfile(); v != "" {
		t.Fatal("mod.Modfile:", v)
	}
	if v := mod.Root(); v != "" {
		t.Fatal("mod.Root:", v)
	}
	if v := mod.Path(); v != "" {
		t.Fatal("mod.Path:", v)
	}
	if mod.HasProject() {
		t.Fatal("mod.HasProject")
	}
	if v := len(mod.Opt.ClassMods); v != 0 {
		t.Fatal("len(mod.Opt.ClassMods):", v)
	}
}

func TestLoad(t *testing.T) {
	if _, e := Load("/path/not-found"); errors.Err(e) != mod.ErrNotFound {
		t.Fatal("TestLoad:", e)
	}
}

func TestCreate(t *testing.T) {
	mod, err := Create("/foo/bar", "github.com/foo/bar", defaultGoVer, defaultGopVer)
	if err != nil {
		t.Fatal("Create failed:", err)
	}
	mod.AddRequire("github.com/goplus/yap", "v0.7.2", true)
	if b, err := mod.File.Format(); err != nil {
		t.Fatal("AddRequire & Format:", err)
	} else if v := string(b); v != `module github.com/foo/bar

go 1.18

require github.com/goplus/yap v0.7.2 //gop:class
` {
		t.Fatal("AddRequire:", v)
	}

	if !mod.HasModfile() {
		t.Fatal("mod.HasModfile")
	}
	if runtime.GOOS != "windows" {
		if v := mod.Modfile(); v != "/foo/bar/go.mod" {
			t.Fatal("mod.Modfile:", v)
		}
		if v := mod.Root(); v != "/foo/bar" {
			t.Fatal("mod.Root:", v)
		}
	}
	if v := mod.Path(); v != "github.com/foo/bar" {
		t.Fatal("mod.Path:", v)
	}
	if hasGopExtended(mod.Opt) {
		t.Fatal("hasGopExtended?")
	}
	if mod.Projects() != nil {
		t.Fatal("mod.Projects != nil?")
	}
	if v := len(mod.Opt.ClassMods); v == 0 {
		t.Fatal("len(mod.Opt.ClassMods):", v)
	}
	mod.AddRequire("github.com/goplus/yap", "v0.7.2", true)
	if v := len(mod.Opt.ClassMods); v != 1 {
		t.Fatal("len(mod.Opt.ClassMods):", v)
	}

	mod.AddRequire("github.com/qiniu/x", "v0.1.0", false)
	if b, err := mod.File.Format(); err != nil {
		t.Fatal("AddRequire & Format:", err)
	} else if v := string(b); v != `module github.com/foo/bar

go 1.18

require (
	github.com/goplus/yap v0.7.2 //gop:class
	github.com/qiniu/x v0.1.0
)
` {
		t.Fatal("AddRequire:", v)
	}

	mod.AddReplace("github.com/goplus/yap", "v0.7.2", "../", "")
	if b, err := mod.File.Format(); err != nil {
		t.Fatal("AddReplace & Format:", err)
	} else if v := string(b); v != `module github.com/foo/bar

go 1.18

require (
	github.com/goplus/yap v0.7.2 //gop:class
	github.com/qiniu/x v0.1.0
)

replace github.com/goplus/yap v0.7.2 => ../
` {
		t.Fatal("AddReplace:", v)
	}

	b, _ := json.Marshal(mod.DepMods())
	if runtime.GOOS != "windows" {
		if v := string(b); v != `{"github.com/goplus/yap":{"Path":"/foo"},"github.com/qiniu/x":{"Path":"github.com/qiniu/x","Version":"v0.1.0"}}` {
			t.Fatal("mod.DepMods:", v)
		}
	}
}

func TestSaveDefault(t *testing.T) {
	if v := Default.workFile(); v != "" {
		t.Fatal("Default.workFile:", v)
	}
	if v := Default.sumFile(); v != "" {
		t.Fatal("Default.sumFile:", v)
	}
	if err := Default.Save(); err != ErrSaveDefault {
		t.Fatal("Default.Save:", err)
	}

	gop := Module{
		File: &gomodfile.File{
			Module: &gomodfile.Module{
				Mod: module.Version{
					Path: "github.com/goplus/gop",
				},
			},
			Go: &gomodfile.Go{Version: defaultGoVer},
			Syntax: &gomodfile.FileSyntax{
				Name: "/foo/bar/go.mod",
			},
		},
		Opt: &modfile.File{
			Gop: &modfile.Gop{Version: defaultGopVer},
		},
	}
	gop.SaveWithGopMod(&env.Gop{Version: "v1.2.0 devel", Root: "/foo/bar/gop"}, FlagDepModGop)
}

func TestSave(t *testing.T) {
	dir := ".gop/_tempdir"
	os.RemoveAll(".gop")
	os.MkdirAll(dir, 0777)
	mod, err := Create(dir, "github.com/foo/bar", "", "")
	if err != nil {
		t.Fatal("Create:", err)
	}
	if err = mod.AddRequire("github.com/goplus/yap", "v0.5.0", true); err != nil {
		t.Fatal("mod.AddRequire:", err)
	}
	mod.Save()

	mod, err = Load(dir)
	if err != nil {
		t.Fatal("Load:", err)
	}
	if err = mod.SaveWithGopMod(&env.Gop{Version: "v1.2.0 devel", Root: "/foo/bar/gop"}, FlagDepModGop); err != nil {
		t.Fatal("mod.SaveWithGopMod:", err)
	}
	if b, err := mod.File.Format(); err != nil {
		t.Fatal("Format:", err)
	} else if v := string(b); v != `module github.com/foo/bar

go 1.18

require (
	github.com/goplus/yap v0.5.0 //gop:class
	github.com/goplus/gop v1.2.0
)
` {
		t.Fatal("SaveWithGopMod:", v)
	}
	b, err := os.ReadFile(mod.workFile())
	if err != nil {
		t.Fatal("read workFile:", err)
	}
	if v := string(b); v != `go 1.18

use .

replace github.com/goplus/gop v1.2.0 => /foo/bar/gop
` {
		t.Fatal("workFile:", v)
	}

	// SaveWithGopMod with FlagDepModX
	os.WriteFile(".gop/go.mod", []byte(`
module github.com/goplus/gop

go 1.18

require (
	github.com/qiniu/x v1.13.0
)
`), 0666)
	if err = mod.SaveWithGopMod(&env.Gop{Version: "v1.2.0 devel", Root: ".gop"}, FlagDepModGop|FlagDepModX); err != nil {
		t.Fatal("mod.SaveWithGopMod 2:", err)
	}
	if b, err := mod.File.Format(); err != nil {
		t.Fatal("Format:", err)
	} else if v := string(b); v != `module github.com/foo/bar

go 1.18

require (
	github.com/goplus/yap v0.5.0 //gop:class
	github.com/goplus/gop v1.2.0
	github.com/qiniu/x v1.13.0
)
` {
		t.Fatal("SaveWithGopMod:", v)
	}
	if _, _, ok := getXVer(&env.Gop{Root: "/foo/bar"}); ok {
		t.Fatal("getXVer: ok?")
	}

	// SaveWithGopMod again. noop.
	if err = mod.SaveWithGopMod(&env.Gop{Version: "v1.2.0 devel", Root: ".gop"}, FlagDepModGop|FlagDepModX); err != nil {
		log.Fatal("mod.SaveWithGopMod 3:", err)
	}

	if err = mod.updateWorkfile(&env.Gop{Version: "v1.2.0 devel", Root: ".gop"}, ""); err != nil {
		log.Fatal("updateWorkfile:", err)
	}

	mod.Opt.Projects = append(mod.Opt.Projects, spxProject)
	mod.Save()
	b, err = os.ReadFile(mod.Opt.Syntax.Name)
	if err != nil {
		t.Fatal("read gop.mod:", err)
	}
	if v := string(b); v != `gop 1.2
` {
		t.Fatal("gop.mod:", v)
	}
}

var (
	spxProject = &modfile.Project{
		Ext:      ".spx",
		Class:    "Game",
		Works:    []*modfile.Class{{Ext: ".spx", Class: "Sprite"}},
		PkgPaths: []string{"github.com/goplus/spx", "math"},
	}
)
