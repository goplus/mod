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

package mod

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"
)

// newDefault builds a default workspace rooted at a fresh temporary directory
// and returns both.
func newDefault(t *testing.T) (root string, w *Workspace) {
	t.Helper()
	root = tempRoot(t)
	w, err := NewDefault(root)
	if err != nil {
		t.Fatal(err)
	}
	return root, w
}

// TestNewDefault checks the shape of a default workspace: no go.work, exactly
// one main module with an empty path and a non-nil empty go.mod, and an
// absolute directory.
func TestNewDefault(t *testing.T) {
	root, w := newDefault(t)
	if w.file != nil {
		t.Errorf("file = %v, want nil", w.file)
	}
	if len(w.mainMods) != 1 {
		t.Fatalf("mainMods = %d, want 1", len(w.mainMods))
	}
	m := w.mainMods[0]
	if m.path != "" {
		t.Errorf("module path = %q, want empty", m.path)
	}
	if m.file == nil {
		t.Error("mainMods[0].file is nil, want a non-nil empty go.mod")
	}
	if m.dir != root || w.dir != root {
		t.Errorf("dir = %q / %q, want %q", m.dir, w.dir, root)
	}
}

// TestNewDefaultMakesAbsolute checks that a relative dir is made absolute.
func TestNewDefaultMakesAbsolute(t *testing.T) {
	root := tempRoot(t)
	wd, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	defer chdir(t, wd)
	chdir(t, root)
	w, err := NewDefault(".")
	if err != nil {
		t.Fatal(err)
	}
	if w.dir != root {
		t.Errorf("dir = %q, want %q", w.dir, root)
	}
}

// TestIsDefault checks that IsDefault is true only for a default workspace and
// false for every real one.
func TestIsDefault(t *testing.T) {
	_, def := newDefault(t)
	if !def.IsDefault() {
		t.Error("NewDefault workspace: IsDefault = false, want true")
	}

	// A go.mod-only workspace is not default.
	root := writeFiles(t, map[string]string{
		"proj/go.mod": "module example.com/proj\n\ngo 1.21\n",
	})
	gomod, err := LoadGoMod(filepath.Join(root, "proj", "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if gomod.IsDefault() {
		t.Error("LoadGoMod workspace: IsDefault = true, want false")
	}

	// A go.work workspace is not default.
	_, work := loadWS(t, standard)
	if work.IsDefault() {
		t.Error("LoadGoWork workspace: IsDefault = true, want false")
	}
}

// TestLoadFromDirDefault checks that LoadFromDir returns a default workspace,
// and never ErrNoModule, when neither a go.work nor a go.mod is found.
func TestLoadFromDirDefault(t *testing.T) {
	root := tempRoot(t)
	w, err := LoadFromDir(root)
	if err != nil {
		t.Fatalf("LoadFromDir = %v, want a default workspace", err)
	}
	if !w.IsDefault() {
		t.Error("LoadFromDir outside any module: IsDefault = false, want true")
	}
	if w.dir != root {
		t.Errorf("dir = %q, want %q", w.dir, root)
	}
}

// TestLoadFromDirDefaultGOWORKoff checks that GOWORK=off with no go.mod also
// gives a default workspace.
func TestLoadFromDirDefaultGOWORKoff(t *testing.T) {
	root := tempRoot(t)
	t.Setenv("GOWORK", "off")
	w, err := LoadFromDir(root)
	if err != nil {
		t.Fatalf("LoadFromDir GOWORK=off = %v, want a default workspace", err)
	}
	if !w.IsDefault() {
		t.Error("GOWORK=off with no go.mod: IsDefault = false, want true")
	}
}

// TestLoadFromDirErrorsNotDegraded checks that a broken go.mod, a broken
// go.work, and a GOWORK that names a missing file all still return an error,
// rather than silently degrading into a default workspace.
func TestLoadFromDirErrorsNotDegraded(t *testing.T) {
	t.Run("malformed go.mod", func(t *testing.T) {
		root := writeFiles(t, map[string]string{
			"proj/go.mod": "this is not a valid go.mod\n",
		})
		if _, err := LoadFromDir(filepath.Join(root, "proj")); err == nil {
			t.Fatal("malformed go.mod: want an error, got a workspace")
		}
	})

	t.Run("malformed go.work", func(t *testing.T) {
		root := writeFiles(t, map[string]string{
			"ws/go.work": "this is not a valid go.work\n",
		})
		if _, err := LoadFromDir(filepath.Join(root, "ws")); err == nil {
			t.Fatal("malformed go.work: want an error, got a workspace")
		}
	})

	t.Run("GOWORK names a missing file", func(t *testing.T) {
		root := tempRoot(t)
		t.Setenv("GOWORK", filepath.Join(root, "nope", "go.work"))
		if _, err := LoadFromDir(root); err == nil {
			t.Fatal("GOWORK missing file: want an error, got a workspace")
		}
	})
}

// TestDefaultStdlibLookup checks that standard-library lookups behave exactly as
// in a normal workspace: fmt and net/http resolve under GOROOT/src.
func TestDefaultStdlibLookup(t *testing.T) {
	_, w := newDefault(t)
	for _, imp := range []string{"fmt", "net/http"} {
		pkg, err := w.Lookup(imp)
		if err != nil {
			t.Fatalf("Lookup(%q) = %v", imp, err)
		}
		if pkg.Type != PkgtStandard {
			t.Errorf("Lookup(%q).Type = %v, want standard", imp, pkg.Type)
		}
		wantModDir := filepath.Join(w.goroot, "src")
		wantDir := filepath.Join(wantModDir, filepath.FromSlash(imp))
		if pkg.ModDir != wantModDir || pkg.Dir != wantDir {
			t.Errorf("Lookup(%q) = %+v, want ModDir %q Dir %q", imp, pkg, wantModDir, wantDir)
		}
	}
}

// TestDefaultStdlibMatchesNormal checks that a standard lookup on a default
// workspace gives the same Dir as on a normal go.mod-only workspace.
func TestDefaultStdlibMatchesNormal(t *testing.T) {
	_, def := newDefault(t)
	root := writeFiles(t, map[string]string{
		"proj/go.mod": "module example.com/proj\n\ngo 1.21\n",
	})
	normal, err := LoadGoMod(filepath.Join(root, "proj", "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	dp, err := def.Lookup("fmt")
	if err != nil {
		t.Fatal(err)
	}
	np, err := normal.Lookup("fmt")
	if err != nil {
		t.Fatal(err)
	}
	if dp.Dir != np.Dir {
		t.Errorf("default fmt Dir = %q, normal = %q", dp.Dir, np.Dir)
	}
}

// TestDefaultLookupMissingAndInvalid checks the error shapes for non-standard,
// empty, and relative paths on a default workspace.
func TestDefaultLookupMissingAndInvalid(t *testing.T) {
	_, w := newDefault(t)

	_, err := w.Lookup("github.com/x/y")
	var me *MissingError
	if !errors.As(err, &me) {
		t.Fatalf("Lookup(github.com/x/y) = %v, want *MissingError", err)
	}
	if me.Path != "github.com/x/y" {
		t.Errorf("MissingError.Path = %q", me.Path)
	}

	for _, bad := range []string{"", "./x", "../y"} {
		if _, err := w.Lookup(bad); !errors.Is(err, ErrInvalidPkgPath) {
			t.Errorf("Lookup(%q) = %v, want ErrInvalidPkgPath", bad, err)
		}
	}
}

// TestDefaultPkgType checks the package classification on a default workspace.
func TestDefaultPkgType(t *testing.T) {
	_, w := newDefault(t)
	cases := []struct {
		path string
		want PkgType
	}{
		{"", PkgtInvalid},
		{"./x", PkgtLocal},
		{"../y", PkgtLocal},
		{"fmt", PkgtStandard},
		{"net/http", PkgtStandard},
		{"github.com/x/y", PkgtExtern}, // first element has a dot
		{"notadomain", PkgtInvalid},    // dotless, not standard, in no module
	}
	for _, c := range cases {
		if got := w.PkgType(c.path); got != c.want {
			t.Errorf("PkgType(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

// TestDefaultModules checks that a default workspace has an empty module list
// with a nil error, and that LookupModule never succeeds, in particular not for
// the empty path.
func TestDefaultModules(t *testing.T) {
	_, w := newDefault(t)
	mods, err := w.Modules()
	if err != nil {
		t.Fatalf("Modules() err = %v", err)
	}
	if len(mods) != 0 {
		t.Errorf("Modules() = %+v, want empty", mods)
	}
	if _, ok := w.LookupModule(""); ok {
		t.Error(`LookupModule("") = ok, want not ok`)
	}
	if _, ok := w.LookupModule("example.com/x"); ok {
		t.Error("LookupModule(example.com/x) = ok, want not ok")
	}
}

// TestDefaultValidateAndGoVersion checks that Validate is nil (one main module,
// nothing to complain about) and GoVersion is empty on a default workspace.
func TestDefaultValidateAndGoVersion(t *testing.T) {
	_, w := newDefault(t)
	if err := w.Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
	if v := w.GoVersion(); v != "" {
		t.Errorf("GoVersion() = %q, want empty", v)
	}
}

// TestDefaultClasses checks that a default workspace recognizes the built-in
// classfiles (.gsh and _test.gox) and nothing else, and that repeated calls
// return the same pointer.
func TestDefaultClasses(t *testing.T) {
	root, w := newDefault(t)
	cls, err := w.Classes(root)
	if err != nil {
		t.Fatalf("Classes() = %v", err)
	}
	if !cls.IsClass(".gsh") {
		t.Error("IsClass(.gsh) = false, want true")
	}
	if !cls.IsClass("_test.gox") {
		t.Error("IsClass(_test.gox) = false, want true")
	}
	if cls.IsClass(".spx") {
		t.Error("IsClass(.spx) = true, want false")
	}

	// hello.gsh is a recognized classfile and the project (main) file of .gsh.
	_, isProj, ok := cls.ClassInfo("hello.gsh")
	if !ok {
		t.Error("ClassInfo(hello.gsh): not ok, want ok")
	}
	if !isProj {
		t.Error("ClassInfo(hello.gsh): isProj = false, want true")
	}

	// A second call returns the same cached pointer.
	cls2, err := w.Classes(root)
	if err != nil {
		t.Fatal(err)
	}
	if cls2 != cls {
		t.Error("Classes() returned a different pointer on the second call")
	}
}

// TestDefaultClassesIgnoresGoxmod checks that a gox.mod in the start directory
// of a default workspace is ignored: only the built-ins are recognized, because
// a gox.mod without a go.mod is not a module description.
func TestDefaultClassesIgnoresGoxmod(t *testing.T) {
	root := tempRoot(t)
	// A gox.mod declaring a .spx project, but no go.mod in the same directory.
	writeFile(t, filepath.Join(root, "gox.mod"),
		"gop 1.5\n\nproject .spx Game github.com/goplus/spx\n")
	w, err := LoadFromDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if !w.IsDefault() {
		t.Fatal("want a default workspace")
	}
	cls, err := w.Classes(root)
	if err != nil {
		t.Fatalf("Classes() = %v", err)
	}
	if cls.IsClass(".spx") {
		t.Error("IsClass(.spx) = true, want false: a gox.mod without a go.mod must be ignored")
	}
	if !cls.IsClass(".gsh") || !cls.IsClass("_test.gox") {
		t.Error("the built-in classfiles should still be recognized")
	}
}

// TestDefaultClassesConcurrent runs Lookup and Classes in parallel on one
// default workspace, to exercise the once-guarded caches under -race.
func TestDefaultClassesConcurrent(t *testing.T) {
	root, w := newDefault(t)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := w.Lookup("fmt"); err != nil {
				t.Errorf("Lookup(fmt) = %v", err)
			}
			if _, err := w.Classes(root); err != nil {
				t.Errorf("Classes() = %v", err)
			}
		}()
	}
	wg.Wait()
}

// TestBuildModulesRealMainUnchanged checks that the empty-path guard in
// buildModules does not affect a workspace with a real main module: its entry
// is still published.
func TestBuildModulesRealMainUnchanged(t *testing.T) {
	root := writeFiles(t, map[string]string{
		"proj/go.mod": "module example.com/proj\n\ngo 1.21\n",
	})
	w, err := LoadGoMod(filepath.Join(root, "proj", "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	mods, err := w.Modules()
	if err != nil {
		t.Fatal(err)
	}
	if len(mods) != 1 || mods[0].Path != "example.com/proj" {
		t.Fatalf("Modules() = %+v, want the one real main module", mods)
	}
	if mv, ok := w.LookupModule("example.com/proj"); !ok || mv.Path != filepath.Join(root, "proj") {
		t.Errorf("LookupModule(example.com/proj) = %+v, %v", mv, ok)
	}
}
