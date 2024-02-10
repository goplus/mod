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

package gopmod

import (
	"log"
	"runtime"
	"testing"

	"github.com/goplus/mod/modload/modtest"
)

func TestLookup(t *testing.T) {
	mod := New(modtest.GopClass(t))
	if modv, ok := mod.LookupDepMod("github.com/qiniu/x"); !ok || modv.Version != "v1.13.2" {
		t.Fatal("mod.LookupDepMod:", modv)
	}
	if pkg, err := mod.Lookup("fmt"); err != nil || pkg.ModPath != "" || pkg.ModDir != runtime.GOROOT()+"/src" {
		t.Fatal("mod.Lookup fmt:", pkg.ModPath, pkg.ModDir, "err:", err)
	}
	if pkg, err := mod.Lookup("github.com/goplus/community/foo"); err != nil || pkg.ModPath != "github.com/goplus/community" {
		t.Fatal("mod.Lookup github.com/goplus/community/foo:", pkg.ModPath, pkg.ModDir, "err:", err)
	}
	if _, err := mod.Lookup("github.com/qiniu/y/mockhttp"); err == nil || err.Error() != `no required module provides package github.com/qiniu/y/mockhttp; to add it:
	gop get github.com/qiniu/y/mockhttp` {
		t.Fatal("mod.Lookup github.com/qiniu/y/mockhttp:", err)
	}
	if pkg, err := mod.Lookup("github.com/qiniu/x/mockhttp"); err != nil || pkg.ModPath != "github.com/qiniu/x" {
		t.Fatal("mod.Lookup github.com/qiniu/x/mockhttp:", pkg.ModPath, pkg.ModDir, "err:", err)
	}
	defer func() {
		if e := recover(); e == nil {
			log.Fatal("mod.Lookup: no panic?")
		}
	}()
	mod.Lookup("")
}

func TestPkgType(t *testing.T) {
	mod := New(modtest.GopClass(t))
	if mod.IsPkgtStandard("github.com/qiniu/x") {
		t.Fatal("mod.IsPkgtStandard: true?")
	}
	if !mod.IsPkgtStandard("fmt") {
		t.Fatal("mod.IsPkgtStandard: false?")
	}
	if pt := mod.PkgType(""); pt != PkgtInvalid {
		t.Fatal("mod.PkgType:", pt)
	}
	if pt := mod.PkgType("./fmt"); pt != PkgtLocal {
		t.Fatal("mod.PkgType ./fmt:", pt)
	}
	if pt := mod.PkgType("github.com/goplus/community/foo"); pt != PkgtModule {
		t.Fatal("mod.PkgType github.com/goplus/community/foo:", pt)
	}
}
