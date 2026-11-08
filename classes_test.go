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
	"path/filepath"
	"sync"
	"testing"

	"github.com/goplus/mod/goxmod"
	"golang.org/x/mod/module"
)

// fakeModGox writes a gox.mod into the extracted module directory of (path, ver)
// in the fake module cache under root/modcache. Pair it with fakeMod to also
// write the module's .mod download-cache entry, so the module is treated as
// already present and no fetch is attempted.
func fakeModGox(t *testing.T, root, path, ver, goxmodText string) {
	t.Helper()
	enc, err := module.EscapePath(path)
	if err != nil {
		t.Fatal(err)
	}
	modDir := filepath.Join(root, "modcache", enc+"@"+ver)
	writeFile(t, filepath.Join(modDir, "gox.mod"), goxmodText)
}

// -----------------------------------------------------------------------------

// TestClassesBuiltinsOnly checks that a go.mod-only module with no gox.mod still
// recognizes the built-in classfile extensions.
func TestClassesBuiltinsOnly(t *testing.T) {
	root := writeFiles(t, map[string]string{
		"app/go.mod": "module example.com/app\n\ngo 1.21\n",
	})
	w, err := LoadGoMod(filepath.Join(root, "app", "go.mod"))
	if err != nil {
		t.Fatal("LoadGoMod:", err)
	}
	cls, err := w.Classes()
	if err != nil {
		t.Fatal("Classes:", err)
	}
	if !cls.IsClass("_test.gox") || !cls.IsClass(".gsh") {
		t.Fatal("built-in extensions not recognized")
	}
	if cls.IsClass(".spx") {
		t.Fatal("IsClass .spx: ok?")
	}
}

// TestClassesOwnProject checks that a module's own gox.mod project, and its work
// extensions, resolve through IsClass and LookupClass.
func TestClassesOwnProject(t *testing.T) {
	root := writeFiles(t, map[string]string{
		"app/go.mod": "module example.com/app\n\ngo 1.21\n",
		"app/gox.mod": "xgo 1.2\n\n" +
			"project _yap.gox App example.com/app\n" +
			"project _ytest.gox App example.com/app/test\n" +
			"class _ytest.gox Case\n",
	})
	w, err := LoadGoMod(filepath.Join(root, "app", "go.mod"))
	if err != nil {
		t.Fatal("LoadGoMod:", err)
	}
	cls, err := w.Classes()
	if err != nil {
		t.Fatal("Classes:", err)
	}
	if c, ok := cls.LookupClass("_yap.gox"); !ok || c.Class != "App" {
		t.Fatal("LookupClass _yap.gox:", c, ok)
	}
	if !cls.IsClass("_ytest.gox") {
		t.Fatal("IsClass _ytest.gox: not ok?")
	}
}

// TestClassesGopModFallback checks that Classes falls back to gop.mod when no
// gox.mod exists.
func TestClassesGopModFallback(t *testing.T) {
	root := writeFiles(t, map[string]string{
		"app/go.mod":  "module example.com/app\n\ngo 1.21\n",
		"app/gop.mod": "gop 1.1\n\nproject _spx.gox Game example.com/app\n",
	})
	w, err := LoadGoMod(filepath.Join(root, "app", "go.mod"))
	if err != nil {
		t.Fatal("LoadGoMod:", err)
	}
	cls, err := w.Classes()
	if err != nil {
		t.Fatal("Classes:", err)
	}
	if !cls.IsClass("_spx.gox") {
		t.Fatal("IsClass _spx.gox: not ok?")
	}
}

// TestClassesFromGoModClassMarker checks that a dependency marked with
// //xgo:class in go.mod contributes its projects after Classes.
func TestClassesFromGoModClassMarker(t *testing.T) {
	testClassesFromGoModMarker(t, "//xgo:class")
}

// TestClassesFromGoModLegacyMarker checks that the legacy //gop:class marker
// behaves the same way.
func TestClassesFromGoModLegacyMarker(t *testing.T) {
	testClassesFromGoModMarker(t, "//gop:class")
}

func testClassesFromGoModMarker(t *testing.T, marker string) {
	t.Helper()
	root := writeFiles(t, map[string]string{
		"app/go.mod": "module example.com/app\n\ngo 1.21\n\n" +
			"require github.com/dep/foo v1.0.0 " + marker + "\n",
	})
	// The dependency is in the module cache with a gox.mod declaring a project.
	fakeMod(t, root, "github.com/dep/foo", "v1.0.0", "module github.com/dep/foo\n\ngo 1.21\n")
	fakeModGox(t, root, "github.com/dep/foo", "v1.0.0", "xgo 1.2\n\nproject _foo.gox App github.com/dep/foo\n")
	setModCache(t, filepath.Join(root, "modcache"))

	w, err := LoadGoMod(filepath.Join(root, "app", "go.mod"))
	if err != nil {
		t.Fatal("LoadGoMod:", err)
	}
	cls, err := w.Classes()
	if err != nil {
		t.Fatal("Classes:", err)
	}
	if c, ok := cls.LookupClass("_foo.gox"); !ok || c.Class != "App" {
		t.Fatalf("LookupClass _foo.gox: %v, %v (marker %q)", c, ok, marker)
	}
}

// TestClassesProjVsWork checks that ClassInfo reports isProj correctly for a
// project file versus a work file.
func TestClassesProjVsWork(t *testing.T) {
	root := writeFiles(t, map[string]string{
		"app/go.mod": "module example.com/app\n\ngo 1.21\n",
		"app/gox.mod": "xgo 1.2\n\n" +
			"project _yap.gox App example.com/app\n" +
			"class _ytest.gox Case\n",
	})
	w, err := LoadGoMod(filepath.Join(root, "app", "go.mod"))
	if err != nil {
		t.Fatal("LoadGoMod:", err)
	}
	cls, err := w.Classes()
	if err != nil {
		t.Fatal("Classes:", err)
	}
	if _, isProj, ok := cls.ClassInfo("main_yap.gox"); !ok || !isProj {
		t.Fatal("ClassInfo main_yap.gox:", isProj, ok)
	}
	if _, isProj, ok := cls.ClassInfo("foo_ytest.gox"); !ok || isProj {
		t.Fatal("ClassInfo foo_ytest.gox:", isProj, ok)
	}
}

// TestClassesOnlyFirstMainMod checks that in a workspace with two main modules,
// only a project declared by the first main module is recognized; one declared
// only by the second is not.
func TestClassesOnlyFirstMainMod(t *testing.T) {
	root, w := loadWS(t, map[string]string{
		"ws/go.work":  "go 1.22\n\nuse (\n\t./a\n\t./b\n)\n",
		"ws/a/go.mod": "module example.com/a\n\ngo 1.21\n",
		"ws/a/gox.mod": "xgo 1.2\n\n" +
			"project _aaa.gox App example.com/a\n",
		"ws/b/go.mod": "module example.com/b\n\ngo 1.21\n",
		"ws/b/gox.mod": "xgo 1.2\n\n" +
			"project _bbb.gox App example.com/b\n",
	})
	_ = root
	cls, err := w.Classes()
	if err != nil {
		t.Fatal("Classes:", err)
	}
	if !cls.IsClass("_aaa.gox") {
		t.Fatal("IsClass _aaa.gox: not ok? (declared by the first main module)")
	}
	if cls.IsClass("_bbb.gox") {
		t.Fatal("IsClass _bbb.gox: ok? (declared only by the second main module)")
	}
}

// TestClassesClassModNoProjects checks that a required class module that
// resolves and is in the cache but declares no projects yields
// ErrNotClassFileMod, and that the error is cached across calls.
func TestClassesClassModNoProjects(t *testing.T) {
	root := writeFiles(t, map[string]string{
		"app/go.mod": "module example.com/app\n\ngo 1.21\n\n" +
			"require github.com/dep/empty v1.0.0 //xgo:class\n",
	})
	// The dependency's extracted directory is in the cache (so no fetch is
	// attempted), but it declares no projects.
	fakeMod(t, root, "github.com/dep/empty", "v1.0.0", "module github.com/dep/empty\n\ngo 1.21\n")
	fakeModGox(t, root, "github.com/dep/empty", "v1.0.0", "xgo 1.2\n") // no projects
	setModCache(t, filepath.Join(root, "modcache"))

	w, err := LoadGoMod(filepath.Join(root, "app", "go.mod"))
	if err != nil {
		t.Fatal("LoadGoMod:", err)
	}
	c1, e1 := w.Classes()
	if e1 != goxmod.ErrNotClassFileMod {
		t.Fatalf("Classes = %v, %v; want ErrNotClassFileMod", c1, e1)
	}
	// The error is cached: a later call returns the identical error.
	c2, e2 := w.Classes()
	if c1 != c2 || e1 != e2 {
		t.Fatalf("Classes not cached: %p/%v vs %p/%v", c1, e1, c2, e2)
	}
}

// TestClassesMalformedGoxMod checks that a malformed gox.mod in the main module
// surfaces as an error from Classes (and is cached).
func TestClassesMalformedGoxMod(t *testing.T) {
	root := writeFiles(t, map[string]string{
		"app/go.mod":  "module example.com/app\n\ngo 1.21\n",
		"app/gox.mod": "xgo 1.2\n\nproject\n", // incomplete project directive
	})
	w, err := LoadGoMod(filepath.Join(root, "app", "go.mod"))
	if err != nil {
		t.Fatal("LoadGoMod:", err)
	}
	c1, e1 := w.Classes()
	if e1 == nil {
		t.Fatal("Classes: expected an error for a malformed gox.mod")
	}
	c2, e2 := w.Classes()
	if c1 != c2 || e1 != e2 {
		t.Fatalf("Classes not cached: %p/%v vs %p/%v", c1, e1, c2, e2)
	}
}

// TestClassesNoMainModules checks that Classes returns an error (rather than
// panicking) for a workspace with no main modules.
func TestClassesNoMainModules(t *testing.T) {
	root := writeFiles(t, map[string]string{
		"ws/go.work": "go 1.22\n", // no use entries
	})
	w, err := LoadGoWork(filepath.Join(root, "ws", "go.work"))
	if err != nil {
		t.Fatal("LoadGoWork:", err)
	}
	if _, err := w.Classes(); err == nil {
		t.Fatal("Classes: expected an error for a workspace with no main modules")
	}
}

// TestClassesCachedPointer checks that repeated calls return the identical
// *Classes pointer and the identical error.
func TestClassesCachedPointer(t *testing.T) {
	root := writeFiles(t, map[string]string{
		"app/go.mod":  "module example.com/app\n\ngo 1.21\n",
		"app/gox.mod": "xgo 1.2\n\nproject _yap.gox App example.com/app\n",
	})
	w, err := LoadGoMod(filepath.Join(root, "app", "go.mod"))
	if err != nil {
		t.Fatal("LoadGoMod:", err)
	}
	c1, e1 := w.Classes()
	c2, e2 := w.Classes()
	if c1 != c2 || e1 != e2 {
		t.Fatalf("Classes not cached: %p/%v vs %p/%v", c1, e1, c2, e2)
	}
}

// TestClassesConcurrent checks that concurrent calls are safe and return the
// identical pointer and error.
func TestClassesConcurrent(t *testing.T) {
	root := writeFiles(t, map[string]string{
		"app/go.mod":  "module example.com/app\n\ngo 1.21\n",
		"app/gox.mod": "xgo 1.2\n\nproject _yap.gox App example.com/app\n",
	})
	w, err := LoadGoMod(filepath.Join(root, "app", "go.mod"))
	if err != nil {
		t.Fatal("LoadGoMod:", err)
	}

	const n = 16
	var wg sync.WaitGroup
	results := make([]*Classes, n)
	errs := make([]error, n)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = w.Classes()
		}(i)
	}
	wg.Wait()

	for i := 1; i < n; i++ {
		if results[i] != results[0] || errs[i] != errs[0] {
			t.Fatalf("Classes[%d] = %p/%v, want %p/%v", i, results[i], errs[i], results[0], errs[0])
		}
	}
}
