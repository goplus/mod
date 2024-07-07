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

package modload_test

import (
	"os"
	"testing"

	"github.com/goplus/mod/gopmod"
	"github.com/goplus/mod/modload/modtest"
	"golang.org/x/mod/module"
)

func TestGopClass(t *testing.T) {
	modtest.GopClass(t)
}

func TestGoCompiler(t *testing.T) {
	modtest.LLGoCompiler(t)
	modtest.TinyGoCompiler(t)
}

func TestImport(t *testing.T) {
	modtest.Import(t)
}

func TestClassfile(t *testing.T) {
	t.Log(os.Getwd())
	modVer := module.Version{Path: "github.com/goplus/yap", Version: "v0.5.0"}
	if _, err := gopmod.LoadMod(modVer); err != nil {
		t.Fatal("gopmod.LoadMod:", err)
	}
}
