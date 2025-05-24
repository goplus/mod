/*
 * Copyright (c) 2024 The XGo Authors (xgo.dev). All rights reserved.
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
	if mod.checkXgoDeps() != FlagDepModX {
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

func TestCreateWithGoCompiler(t *testing.T) {
	mod, err := Create("/foo/bar", "github.com/foo/bar", "1.20", defaultXgoVer)
	if err != nil {
		t.Fatal("Create failed:", err)
	}
	mod.AddCompiler("llgo", "0.9")
	if b, err := mod.File.Format(); err != nil {
		t.Fatal("AddCompiler & Format:", err)
	} else if v := string(b); v != `module github.com/foo/bar

go 1.20 // llgo 0.9
` {
		t.Fatal("AddCompiler:", v)
	}

	mod.File.DropGoStmt()
	mod.AddCompiler("tinygo", "0.32")
	if b, err := mod.File.Format(); err != nil {
		t.Fatal("AddCompiler & Format:", err)
	} else if v := string(b); v != `module github.com/foo/bar

go 1.18 // tinygo 0.32
` {
		t.Fatal("AddCompiler:", v)
	}
}

func TestCreate(t *testing.T) {
	mod, err := Create("/foo/bar", "github.com/foo/bar", defaultGoVer, defaultXgoVer)
	if err != nil {
		t.Fatal("Create failed:", err)
	}
	mod.AddRequire("github.com/goplus/yap", "v0.7.2", true)
	if b, err := mod.File.Format(); err != nil {
		t.Fatal("AddRequire & Format:", err)
	} else if v := string(b); v != `module github.com/foo/bar

go 1.18

require github.com/goplus/yap v0.7.2 //xgo:class
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
	github.com/goplus/yap v0.7.2 //xgo:class
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
	github.com/goplus/yap v0.7.2 //xgo:class
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

	xgo := Module{
		File: &gomodfile.File{
			Module: &gomodfile.Module{
				Mod: module.Version{
					Path: "github.com/goplus/xgo",
				},
			},
			Go: &gomodfile.Go{Version: defaultGoVer},
			Syntax: &gomodfile.FileSyntax{
				Name: "/foo/bar/go.mod",
			},
		},
		Opt: &modfile.File{
			XGo: &modfile.XGo{Version: defaultXgoVer},
		},
	}
	xgo.SaveWithXGoMod(&env.XGo{Version: "v1.2.0 devel", Root: "/foo/bar/gop"}, FlagDepModXGo)
}

func TestSave(t *testing.T) {
	dir := ".xgo/_tempdir"
	os.RemoveAll(".xgo")
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
	if err = mod.SaveWithXGoMod(&env.XGo{Version: "v1.2.0 devel", Root: "/foo/bar/gop"}, FlagDepModXGo); err != nil {
		t.Fatal("mod.SaveWithGopMod:", err)
	}
	if b, err := mod.File.Format(); err != nil {
		t.Fatal("Format:", err)
	} else if v := string(b); v != `module github.com/foo/bar

go 1.18

require (
	github.com/goplus/yap v0.5.0 //xgo:class
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
	os.WriteFile(".xgo/go.mod", []byte(`
module github.com/goplus/gop

go 1.18

require (
	github.com/qiniu/x v1.13.0
)
`), 0666)
	if err = mod.SaveWithXGoMod(&env.XGo{Version: "v1.2.0 devel", Root: ".xgo"}, FlagDepModXGo|FlagDepModX); err != nil {
		t.Fatal("mod.SaveWithGopMod 2:", err)
	}
	if b, err := mod.File.Format(); err != nil {
		t.Fatal("Format:", err)
	} else if v := string(b); v != `module github.com/foo/bar

go 1.18

require (
	github.com/goplus/yap v0.5.0 //xgo:class
	github.com/goplus/gop v1.2.0
	github.com/qiniu/x v1.13.0
)
` {
		t.Fatal("SaveWithGopMod:", v)
	}
	if _, _, ok := getXVer(&env.XGo{Root: "/foo/bar"}); ok {
		t.Fatal("getXVer: ok?")
	}

	// SaveWithGopMod again. noop.
	if err = mod.SaveWithXGoMod(&env.XGo{Version: "v1.2.0 devel", Root: ".xgo"}, FlagDepModXGo|FlagDepModX); err != nil {
		log.Fatal("mod.SaveWithGopMod 3:", err)
	}

	if err = mod.updateWorkfile(&env.XGo{Version: "v1.2.0 devel", Root: ".xgo"}, ""); err != nil {
		log.Fatal("updateWorkfile:", err)
	}

	mod.Opt.Projects = append(mod.Opt.Projects, spxProject)
	mod.Save()
	b, err = os.ReadFile(mod.Opt.Syntax.Name)
	if err != nil {
		t.Fatal("read gox.mod:", err)
	}
	if v := string(b); v != `xgo 1.5
` {
		t.Fatal("gox.mod:", v)
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
