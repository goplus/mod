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

package gomod

import (
	"os"
	"path/filepath"
	"testing"
)

// readFile returns the content of name, failing the test on error.
func readFile(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestCreateSave(t *testing.T) {
	gomod := filepath.Join(t.TempDir(), "go.mod")
	f, err := Create(gomod, "example.com/foo", "1.21")
	if err != nil {
		t.Fatal("Create:", err)
	}
	if _, err := os.Stat(gomod); !os.IsNotExist(err) {
		t.Fatal("Create should not write go.mod:", err)
	}
	if err := f.Save(); err != nil {
		t.Fatal("Save:", err)
	}
	const want = "module example.com/foo\n\ngo 1.21\n"
	if got := readFile(t, gomod); got != want {
		t.Fatalf("go.mod =\n%s\nwant\n%s", got, want)
	}
}

func TestCreateBadGoVer(t *testing.T) {
	if _, err := Create("go.mod", "example.com/foo", "not-a-version"); err == nil {
		t.Fatal("Create: expected error for invalid go version")
	}
}

func TestLoadNotFound(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "go.mod")); !os.IsNotExist(err) {
		t.Fatal("Load: expected not-exist error, got", err)
	}
}

func TestLoadBadSyntax(t *testing.T) {
	gomod := filepath.Join(t.TempDir(), "go.mod")
	if err := os.WriteFile(gomod, []byte("module\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(gomod); err == nil {
		t.Fatal("Load: expected parse error")
	}
}

// TestLoadEditSave loads an existing go.mod, edits it and checks that Save
// writes back to the same file with the XGo markers in place.
func TestLoadEditSave(t *testing.T) {
	gomod := filepath.Join(t.TempDir(), "go.mod")
	const text = `module example.com/foo

go 1.21

require example.com/bar v1.0.0
`
	if err := os.WriteFile(gomod, []byte(text), 0644); err != nil {
		t.Fatal(err)
	}
	f, err := Load(gomod)
	if err != nil {
		t.Fatal("Load:", err)
	}
	f.AddCompiler("llgo", "0.9")
	if err := f.AddRequire("example.com/yap", "v0.8.0", true); err != nil {
		t.Fatal("AddRequire:", err)
	}
	if err := f.Save(); err != nil {
		t.Fatal("Save:", err)
	}
	const want = `module example.com/foo

go 1.21 // llgo 0.9

require (
	example.com/bar v1.0.0
	example.com/yap v0.8.0 //xgo:class
)
`
	got := readFile(t, gomod)
	if got != want {
		t.Fatalf("go.mod =\n%s\nwant\n%s", got, want)
	}

	// The saved file round-trips through Load, IsClass and Compiler.
	f, err = Load(gomod)
	if err != nil {
		t.Fatal("Load:", err)
	}
	if cl := Compiler(f.File); cl == nil || cl.Name != "llgo" || cl.Version != "0.9" {
		t.Fatalf("Compiler = %+v, want llgo 0.9", cl)
	}
	for _, r := range f.Require {
		if want := r.Mod.Path == "example.com/yap"; IsClass(r) != want {
			t.Fatalf("IsClass(%s) = %v, want %v", r.Mod.Path, !want, want)
		}
	}
}

func TestAddCompilerReplace(t *testing.T) {
	f, err := Create("go.mod", "example.com/foo", "1.21")
	if err != nil {
		t.Fatal("Create:", err)
	}
	f.AddCompiler("llgo", "0.9")
	f.AddCompiler("tinygo", "0.32")
	if n := len(f.Go.Syntax.Suffix); n != 1 {
		t.Fatalf("go directive has %d suffix comments, want 1", n)
	}
	if cl := Compiler(f.File); cl == nil || cl.Name != "tinygo" || cl.Version != "0.32" {
		t.Fatalf("Compiler = %+v, want tinygo 0.32", cl)
	}
}

func TestAddCompilerNoGo(t *testing.T) {
	gomod := filepath.Join(t.TempDir(), "go.mod")
	if err := os.WriteFile(gomod, []byte("module example.com/foo\n"), 0644); err != nil {
		t.Fatal(err)
	}
	f, err := Load(gomod)
	if err != nil {
		t.Fatal("Load:", err)
	}
	f.AddCompiler("tinygo", "0.32")
	if f.Go == nil || f.Go.Version != defaultGoVer {
		t.Fatalf("go directive = %+v, want %s", f.Go, defaultGoVer)
	}
	if err := f.Save(); err != nil {
		t.Fatal("Save:", err)
	}
	const want = "module example.com/foo\n\ngo " + defaultGoVer + " // tinygo 0.32\n"
	if got := readFile(t, gomod); got != want {
		t.Fatalf("go.mod =\n%s\nwant\n%s", got, want)
	}
}

func TestAddRequire(t *testing.T) {
	f, err := Create("go.mod", "example.com/foo", "1.21")
	if err != nil {
		t.Fatal("Create:", err)
	}
	if err := f.AddRequire("example.com/plain", "v1.0.0", false); err != nil {
		t.Fatal("AddRequire:", err)
	}
	if err := f.AddRequire("example.com/class", "v1.0.0", true); err != nil {
		t.Fatal("AddRequire:", err)
	}
	// Re-adding a class require updates the version without duplicating the
	// marker, and hasProj=false keeps an existing marker.
	if err := f.AddRequire("example.com/class", "v1.1.0", true); err != nil {
		t.Fatal("AddRequire:", err)
	}
	if err := f.AddRequire("example.com/class", "v1.2.0", false); err != nil {
		t.Fatal("AddRequire:", err)
	}
	// Marking an existing plain require turns it into a class require.
	if err := f.AddRequire("example.com/plain", "v1.0.0", true); err != nil {
		t.Fatal("AddRequire:", err)
	}
	if n := len(f.Require); n != 2 {
		t.Fatalf("len(Require) = %d, want 2", n)
	}
	for _, r := range f.Require {
		if !IsClass(r) {
			t.Errorf("IsClass(%s) = false, want true", r.Mod.Path)
		}
		if n := len(r.Syntax.Suffix); n != 1 {
			t.Errorf("%s has %d suffix comments, want 1", r.Mod.Path, n)
		}
	}
	if r := requireOf(t, f.File, "example.com/class"); r.Mod.Version != "v1.2.0" {
		t.Errorf("class version = %s, want v1.2.0", r.Mod.Version)
	}
}

func TestSaveError(t *testing.T) {
	gomod := filepath.Join(t.TempDir(), "nodir", "go.mod")
	f, err := Create(gomod, "example.com/foo", "1.21")
	if err != nil {
		t.Fatal("Create:", err)
	}
	if err := f.Save(); err == nil {
		t.Fatal("Save: expected error writing into a missing directory")
	}
}
