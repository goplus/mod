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

package goxmod

import (
	"os"
	"path/filepath"
	"testing"

	gomodfile "golang.org/x/mod/modfile"
)

// memFiles returns a readFile hook that serves the given slash-keyed files from
// memory, reporting os.ErrNotExist for anything else.
func memFiles(files map[string]string) func(string) ([]byte, error) {
	return func(name string) ([]byte, error) {
		if data, ok := files[name]; ok {
			return []byte(data), nil
		}
		return nil, os.ErrNotExist
	}
}

// parseGoMod parses go.mod text, failing the test on error.
func parseGoMod(t *testing.T, text string) *gomodfile.File {
	t.Helper()
	f, err := gomodfile.Parse("go.mod", []byte(text), nil)
	if err != nil {
		t.Fatal("parseGoMod:", err)
	}
	return f
}

// TestLoadModfileGoxPresent loads a gox.mod that declares a project and checks
// that the project is parsed.
func TestLoadModfileGoxPresent(t *testing.T) {
	const goxmodText = `
xgo 1.2

project _yap.gox App github.com/goplus/yap
`
	f, err := LoadModfileEx(nil, "/foo/gox.mod", memFiles(map[string]string{
		"/foo/gox.mod": goxmodText,
	}))
	if err != nil {
		t.Fatal("LoadModfileEx:", err)
	}
	if n := len(f.Projects); n != 1 {
		t.Fatal("len(Projects):", n)
	}
	if f.Projects[0].Ext != "_yap.gox" {
		t.Fatal("Projects[0].Ext:", f.Projects[0].Ext)
	}
	if f.XGo == nil || f.XGo.Version != "1.2" {
		t.Fatal("XGo:", f.XGo)
	}
}

// TestLoadModfileGopFallback loads a module where only gop.mod exists and checks
// that LoadModfile falls back to it.
func TestLoadModfileGopFallback(t *testing.T) {
	const gopmodText = `
gop 1.1

project _spx.gox Game github.com/goplus/spx
`
	f, err := LoadModfileEx(nil, "/foo/gox.mod", memFiles(map[string]string{
		"/foo/gop.mod": gopmodText,
	}))
	if err != nil {
		t.Fatal("LoadModfileEx:", err)
	}
	if n := len(f.Projects); n != 1 {
		t.Fatal("len(Projects):", n)
	}
	if f.Projects[0].Ext != "_spx.gox" {
		t.Fatal("Projects[0].Ext:", f.Projects[0].Ext)
	}
}

// TestLoadModfileNeither loads a module with no gox.mod and no gop.mod: the
// result is an empty module description with the default XGo version and no
// error.
func TestLoadModfileNeither(t *testing.T) {
	f, err := LoadModfileEx(nil, "/foo/gox.mod", memFiles(nil))
	if err != nil {
		t.Fatal("LoadModfileEx:", err)
	}
	if n := len(f.Projects); n != 0 {
		t.Fatal("len(Projects):", n)
	}
	if f.XGo == nil || f.XGo.Version != defaultXgoVer {
		t.Fatal("XGo:", f.XGo)
	}
}

// TestLoadModfileReadError checks that any read error (not only file-not-found)
// leads to the empty-module result, not an error.
func TestLoadModfileReadError(t *testing.T) {
	readFile := func(name string) ([]byte, error) {
		return nil, os.ErrPermission
	}
	f, err := LoadModfileEx(nil, "/foo/gox.mod", readFile)
	if err != nil {
		t.Fatal("LoadModfileEx:", err)
	}
	if f.XGo == nil || f.XGo.Version != defaultXgoVer {
		t.Fatal("XGo:", f.XGo)
	}
}

// TestLoadModfileMalformed checks that a parse error in gox.mod is returned.
func TestLoadModfileMalformed(t *testing.T) {
	const goxmodText = `
xgo 1.2

project
`
	_, err := LoadModfileEx(nil, "/foo/gox.mod", memFiles(map[string]string{
		"/foo/gox.mod": goxmodText,
	}))
	if err == nil {
		t.Fatal("LoadModfileEx: expected a parse error, got nil")
	}
}

// TestLoadModfileEmptyPath checks that an empty goxmod path yields the empty
// module description.
func TestLoadModfileEmptyPath(t *testing.T) {
	f, err := LoadModfileEx(nil, "", memFiles(nil))
	if err != nil {
		t.Fatal("LoadModfileEx:", err)
	}
	if f.XGo == nil || f.XGo.Version != defaultXgoVer {
		t.Fatal("XGo:", f.XGo)
	}
}

// TestLoadModfileReadFileHook checks that the readFile hook is used instead of
// the disk: a path that does not exist on disk is served from memory.
func TestLoadModfileReadFileHook(t *testing.T) {
	const goxmodText = `
xgo 1.2

project _yap.gox App github.com/goplus/yap
`
	// A path that certainly does not exist on disk.
	goxmod := filepath.Join(t.TempDir(), "does-not-exist", "gox.mod")
	f, err := LoadModfileEx(nil, goxmod, memFiles(map[string]string{
		goxmod: goxmodText,
	}))
	if err != nil {
		t.Fatal("LoadModfileEx:", err)
	}
	if n := len(f.Projects); n != 1 {
		t.Fatal("len(Projects):", n)
	}
}

// TestLoadModfileClassMods checks that //xgo:class and //gop:class requires in
// go.mod land in ClassMods, while unmarked requires do not.
func TestLoadModfileClassMods(t *testing.T) {
	const gomodText = `
module github.com/goplus/community

go 1.18

require (
	github.com/goplus/yap v0.5.0 //xgo:class
	github.com/qiniu/a v0.1.0
	github.com/qiniu/x v1.13.2 // gop:class
)
`
	gomod := parseGoMod(t, gomodText)
	f, err := LoadModfileEx(gomod, "/foo/gox.mod", memFiles(nil))
	if err != nil {
		t.Fatal("LoadModfileEx:", err)
	}
	want := []string{"github.com/goplus/yap", "github.com/qiniu/x"}
	if len(f.ClassMods) != len(want) {
		t.Fatalf("ClassMods = %v, want %v", f.ClassMods, want)
	}
	for i, w := range want {
		if f.ClassMods[i] != w {
			t.Fatalf("ClassMods[%d] = %q, want %q", i, f.ClassMods[i], w)
		}
	}
}

// TestLoadModfileCompilerLLGo checks that a `go 1.18 // llgo 0.9` comment sets
// the compiler.
func TestLoadModfileCompilerLLGo(t *testing.T) {
	const gomodText = `
module github.com/goplus/llgo

go 1.18 // llgo 0.9
`
	gomod := parseGoMod(t, gomodText)
	f, err := LoadModfileEx(gomod, "/foo/gox.mod", memFiles(nil))
	if err != nil {
		t.Fatal("LoadModfileEx:", err)
	}
	if cl := f.Compiler; cl == nil || cl.Name != "llgo" || cl.Version != "0.9" {
		t.Fatal("Compiler:", cl)
	}
}

// TestLoadModfileCompilerTinyGo checks that a `go 1.18 // tinygo 0.32` comment
// sets the compiler.
func TestLoadModfileCompilerTinyGo(t *testing.T) {
	const gomodText = `
module github.com/tinygo-org/tinygo

go 1.18 // tinygo 0.32
`
	gomod := parseGoMod(t, gomodText)
	f, err := LoadModfileEx(gomod, "/foo/gox.mod", memFiles(nil))
	if err != nil {
		t.Fatal("LoadModfileEx:", err)
	}
	if cl := f.Compiler; cl == nil || cl.Name != "tinygo" || cl.Version != "0.32" {
		t.Fatal("Compiler:", cl)
	}
}

// TestLoadModfileCompilerFromGoMod checks that the compiler from go.mod is
// applied to the result. Because LoadModfile assigns opt.Compiler from go.mod
// unconditionally, this is also what makes go.mod win over any compiler a
// future gox.mod directive might set.
func TestLoadModfileCompilerFromGoMod(t *testing.T) {
	const goxmodText = `
xgo 1.2

project _yap.gox App github.com/goplus/yap
`
	const gomodText = `
module github.com/goplus/llgo

go 1.18 // llgo 0.9
`
	gomod := parseGoMod(t, gomodText)
	f, err := LoadModfileEx(gomod, "/foo/gox.mod", memFiles(map[string]string{
		"/foo/gox.mod": goxmodText,
	}))
	if err != nil {
		t.Fatal("LoadModfileEx:", err)
	}
	if cl := f.Compiler; cl == nil || cl.Name != "llgo" || cl.Version != "0.9" {
		t.Fatal("Compiler:", cl)
	}
}

// TestLoadModfileNilGoMod checks that a nil go.mod leaves ClassMods and Compiler
// untouched.
func TestLoadModfileNilGoMod(t *testing.T) {
	f, err := LoadModfileEx(nil, "/foo/gox.mod", memFiles(nil))
	if err != nil {
		t.Fatal("LoadModfileEx:", err)
	}
	if len(f.ClassMods) != 0 {
		t.Fatal("ClassMods:", f.ClassMods)
	}
	if f.Compiler != nil {
		t.Fatal("Compiler:", f.Compiler)
	}
}
