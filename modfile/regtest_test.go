/*
 * Copyright (c) 2021 The GoPlus Authors (goplus.org). All rights reserved.
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
	"os"
	"testing"

	"github.com/goplus/mod/gopmod"
	"github.com/goplus/mod/modload"
)

func TestGopMod(t *testing.T) {
	mod := gopmod.Default
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
	if pt := mod.PkgType("foo"); pt != gopmod.PkgtStandard {
		t.Fatal("PkgType foo:", pt)
	}
}

func TestLoadFromEx(t *testing.T) {
	const gomodText = `
module github.com/goplus/community

go 1.18

require (
	github.com/goplus/yap v0.5.0 //gop:class
	github.com/qiniu/a v0.1.0
	github.com/qiniu/x v1.13.2 // gop:class
)
`
	const gomod = "go.mod"
	mod, err := modload.LoadFromEx(gomod, "gop.mod", func(s string) ([]byte, error) {
		if s == gomod {
			return []byte(gomodText), nil
		}
		return nil, os.ErrNotExist
	})
	if err != nil {
		t.Fatal("LoadFromEx:", err)
	}
	if n := len(mod.Opt.ClassMods); n != 2 {
		t.Fatal("len(mod.Opt.Import):", n)
	}
}
