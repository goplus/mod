/*
 * Copyright (c) 2021 The XGo Authors (xgo.dev). All rights reserved.
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

package modfile_test

import (
	"testing"

	"github.com/goplus/mod/modload/modtest"
	"github.com/goplus/mod/xgomod"
)

func TestGopMod(t *testing.T) {
	mod := xgomod.Default
	if mod.HasModfile() {
		t.Fatal("gopmod.Default HasModfile?")
	}
	if path := mod.Modfile(); path != "" {
		t.Fatal("gopmod.Default.Modfile?", path)
	}
	if path := mod.Path(); path != "" {
		t.Fatal("gopmod.Default.Path?", path)
	}
	if path := mod.Root(); path != "" {
		t.Fatal("gopmod.Default.Root?", path)
	}
	if pt := mod.PkgType("foo"); pt != xgomod.PkgtStandard {
		t.Fatal("PkgType foo:", pt)
	}
}

func TestGopClass(t *testing.T) {
	modtest.GopClass(t)
}

func TestImport(t *testing.T) {
	modtest.Import(t)
}
