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

package modtest

import (
	"os"
	"testing"

	"github.com/goplus/mod/modload"
)

func LoadFrom(gomod, gopmod string, gomodText, gopmodText string) (mod modload.Module, err error) {
	return modload.LoadFromEx(gomod, gopmod, func(s string) ([]byte, error) {
		if s == gomod {
			return []byte(gomodText), nil
		} else if s == gopmod && gopmodText != "" {
			return []byte(gopmodText), nil
		}
		return nil, os.ErrNotExist
	})
}

func Load(t *testing.T, gomodText, gopmodText string, errMsg string) modload.Module {
	mod, err := LoadFrom("/foo/go.mod", "/foo/gop.mod", gomodText, gopmodText)
	if err != nil {
		if err.Error() != errMsg {
			t.Fatal("LoadFrom:", err)
		}
	}
	return mod
}

func GopClass(t *testing.T) {
	const gomodText = `
module github.com/goplus/community

go 1.18

require (
	github.com/goplus/yap v0.5.0 //gop:class
	github.com/qiniu/a v0.1.0
	github.com/qiniu/x v1.13.2 // gop:class
)
`
	mod := Load(t, gomodText, ``, ``)
	if n := len(mod.Opt.ClassMods); n != 2 {
		t.Fatal("len(mod.Opt.Import):", n)
	}
}

func Import(t *testing.T) {
	const gomodText = `
module github.com/goplus/yap

go 1.18
`
	const gopmodText = `
gop 1.2

project _yap.gox App github.com/goplus/yap

project _ytest.gox App github.com/goplus/yap/test
class _ytest.gox Case
import github.com/goplus/yap/ytest/auth/jwt
import yauth github.com/goplus/yap/ytest/auth
`
	mod := Load(t, gomodText, gopmodText, ``)
	if n := len(mod.Opt.Projects); n != 2 {
		t.Fatal("len(mod.Opt.Projects):", n)
	}
}
