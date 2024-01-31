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
	"testing"

	"github.com/goplus/mod/modfile"
	gomodfile "golang.org/x/mod/modfile"
)

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
	if v := mod.Modfile(); v != "/foo/bar/go.mod" {
		t.Fatal("mod.Modfile:", v)
	}
	if v := mod.Root(); v != "/foo/bar" {
		t.Fatal("mod.Root:", v)
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
}
