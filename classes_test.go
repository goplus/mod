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
	"os"
	"path/filepath"
	"strings"
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
	cls, err := w.Classes(filepath.Join(root, "app"))
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
	cls, err := w.Classes(filepath.Join(root, "app"))
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
	cls, err := w.Classes(filepath.Join(root, "app"))
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
	cls, err := w.Classes(filepath.Join(root, "app"))
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
	cls, err := w.Classes(filepath.Join(root, "app"))
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

// TestClassesPerMainMod checks that in a workspace with two main modules, a
// directory in one module gets that module's projects and not the other's.
func TestClassesPerMainMod(t *testing.T) {
	root, w := loadWS(t, map[string]string{
		"ws/go.work":  "go 1.22\n\nuse (\n\t./a\n\t./b\n)\n",
		"ws/a/go.mod": "module example.com/a\n\ngo 1.21\n",
		"ws/a/gox.mod": "xgo 1.2\n\n" +
			"project _aaa.gox App example.com/a\n",
		"ws/b/go.mod": "module example.com/b\n\ngo 1.21\n",
		"ws/b/gox.mod": "xgo 1.2\n\n" +
			"project _bbb.gox App example.com/b\n",
	})
	clsA, err := w.Classes(filepath.Join(root, "ws", "a"))
	if err != nil {
		t.Fatal("Classes(a):", err)
	}
	if !clsA.IsClass("_aaa.gox") {
		t.Fatal("Classes(a): IsClass _aaa.gox: not ok? (declared by module a)")
	}
	if clsA.IsClass("_bbb.gox") {
		t.Fatal("Classes(a): IsClass _bbb.gox: ok? (declared only by module b)")
	}

	clsB, err := w.Classes(filepath.Join(root, "ws", "b"))
	if err != nil {
		t.Fatal("Classes(b):", err)
	}
	if !clsB.IsClass("_bbb.gox") {
		t.Fatal("Classes(b): IsClass _bbb.gox: not ok? (declared by module b)")
	}
	if clsB.IsClass("_aaa.gox") {
		t.Fatal("Classes(b): IsClass _aaa.gox: ok? (declared only by module a)")
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
	appDir := filepath.Join(root, "app")
	c1, e1 := w.Classes(appDir)
	if e1 != goxmod.ErrNotClassFileMod {
		t.Fatalf("Classes = %v, %v; want ErrNotClassFileMod", c1, e1)
	}
	// The error is cached: a later call returns the identical error.
	c2, e2 := w.Classes(appDir)
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
	appDir := filepath.Join(root, "app")
	c1, e1 := w.Classes(appDir)
	if e1 == nil {
		t.Fatal("Classes: expected an error for a malformed gox.mod")
	}
	c2, e2 := w.Classes(appDir)
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
	_, err := LoadGoWork(filepath.Join(root, "ws", "go.work"))
	if err != ErrNoModule {
		t.Fatal("LoadGoWork:", err)
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
	appDir := filepath.Join(root, "app")
	c1, e1 := w.Classes(appDir)
	c2, e2 := w.Classes(appDir)
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
	appDir := filepath.Join(root, "app")

	const n = 16
	var wg sync.WaitGroup
	results := make([]*Classes, n)
	errs := make([]error, n)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = w.Classes(appDir)
		}(i)
	}
	wg.Wait()

	for i := 1; i < n; i++ {
		if results[i] != results[0] || errs[i] != errs[0] {
			t.Fatalf("Classes[%d] = %p/%v, want %p/%v", i, results[i], errs[i], results[0], errs[0])
		}
	}
}

// TestClassesSamePointerForModuleDirs checks that the module root and a package
// directory below it share the same cached *Classes pointer.
func TestClassesSamePointerForModuleDirs(t *testing.T) {
	root := writeFiles(t, map[string]string{
		"app/go.mod":  "module example.com/app\n\ngo 1.21\n",
		"app/gox.mod": "xgo 1.2\n\nproject _yap.gox App example.com/app\n",
	})
	w, err := LoadGoMod(filepath.Join(root, "app", "go.mod"))
	if err != nil {
		t.Fatal("LoadGoMod:", err)
	}
	cRoot, err := w.Classes(filepath.Join(root, "app"))
	if err != nil {
		t.Fatal("Classes(root):", err)
	}
	cSub, err := w.Classes(filepath.Join(root, "app", "sub", "pkg"))
	if err != nil {
		t.Fatal("Classes(sub/pkg):", err)
	}
	if cRoot != cSub {
		t.Fatalf("different pointers for two directories of the same module: %p vs %p", cRoot, cSub)
	}
}

// TestClassesDirOutsideWorkspace checks that a directory not under any main
// module returns an error satisfying errors.Is(err, ErrDirNotInWorkspace) and
// mentioning the path.
func TestClassesDirOutsideWorkspace(t *testing.T) {
	root := writeFiles(t, map[string]string{
		"app/go.mod": "module example.com/app\n\ngo 1.21\n",
	})
	w, err := LoadGoMod(filepath.Join(root, "app", "go.mod"))
	if err != nil {
		t.Fatal("LoadGoMod:", err)
	}
	outside := filepath.Join(root, "elsewhere")
	cls, err := w.Classes(outside)
	if cls != nil {
		t.Fatalf("Classes = %p, want nil", cls)
	}
	if !errors.Is(err, ErrDirNotInWorkspace) {
		t.Fatalf("Classes error = %v, want ErrDirNotInWorkspace", err)
	}
	if !strings.Contains(err.Error(), outside) {
		t.Fatalf("Classes error = %q, want it to mention %q", err, outside)
	}
}

// TestClassesSiblingPrefix checks that a sibling module whose root shares a
// string prefix (/work/app vs /work/app2) is not mistaken for the owner:
// /work/app2/x resolves to app2, not app.
func TestClassesSiblingPrefix(t *testing.T) {
	root, w := loadWS(t, map[string]string{
		"ws/go.work":    "go 1.22\n\nuse (\n\t./app\n\t./app2\n)\n",
		"ws/app/go.mod": "module example.com/app\n\ngo 1.21\n",
		"ws/app/gox.mod": "xgo 1.2\n\n" +
			"project _app.gox App example.com/app\n",
		"ws/app2/go.mod": "module example.com/app2\n\ngo 1.21\n",
		"ws/app2/gox.mod": "xgo 1.2\n\n" +
			"project _app2.gox App example.com/app2\n",
	})
	cls, err := w.Classes(filepath.Join(root, "ws", "app2", "x"))
	if err != nil {
		t.Fatal("Classes(app2/x):", err)
	}
	if !cls.IsClass("_app2.gox") {
		t.Fatal("app2/x did not resolve to module app2")
	}
	if cls.IsClass("_app.gox") {
		t.Fatal("app2/x resolved to module app (prefix confusion)")
	}
}

// TestClassesNestedWorkspaceModule checks that a module nested under another
// main module and also listed in go.work owns its own subtree (longest root
// wins).
func TestClassesNestedWorkspaceModule(t *testing.T) {
	root, w := loadWS(t, map[string]string{
		"ws/go.work":      "go 1.22\n\nuse (\n\t./outer\n\t./outer/inner\n)\n",
		"ws/outer/go.mod": "module example.com/outer\n\ngo 1.21\n",
		"ws/outer/gox.mod": "xgo 1.2\n\n" +
			"project _outer.gox App example.com/outer\n",
		"ws/outer/inner/go.mod": "module example.com/inner\n\ngo 1.21\n",
		"ws/outer/inner/gox.mod": "xgo 1.2\n\n" +
			"project _inner.gox App example.com/inner\n",
	})
	// A directory under inner resolves to inner (longest root), not outer.
	cls, err := w.Classes(filepath.Join(root, "ws", "outer", "inner", "pkg"))
	if err != nil {
		t.Fatal("Classes(inner/pkg):", err)
	}
	if !cls.IsClass("_inner.gox") {
		t.Fatal("inner/pkg did not resolve to the nested module inner")
	}
	if cls.IsClass("_outer.gox") {
		t.Fatal("inner/pkg resolved to outer, not the nearest module inner")
	}
	// A directory under outer but not inner resolves to outer.
	clsOuter, err := w.Classes(filepath.Join(root, "ws", "outer", "pkg"))
	if err != nil {
		t.Fatal("Classes(outer/pkg):", err)
	}
	if !clsOuter.IsClass("_outer.gox") {
		t.Fatal("outer/pkg did not resolve to module outer")
	}
}

// TestClassesForeignNestedModule checks that a go.mod that is not part of the
// workspace, nested under a main module, takes its subtree out of the
// workspace.
func TestClassesForeignNestedModule(t *testing.T) {
	root := writeFiles(t, map[string]string{
		"app/go.mod":  "module example.com/app\n\ngo 1.21\n",
		"app/gox.mod": "xgo 1.2\n\nproject _app.gox App example.com/app\n",
		// A nested module that is NOT listed in any go.work/use.
		"app/vendored/go.mod": "module example.com/vendored\n\ngo 1.21\n",
	})
	w, err := LoadGoMod(filepath.Join(root, "app", "go.mod"))
	if err != nil {
		t.Fatal("LoadGoMod:", err)
	}
	// A directory under the foreign nested module is not covered.
	_, err = w.Classes(filepath.Join(root, "app", "vendored", "pkg"))
	if !errors.Is(err, ErrDirNotInWorkspace) {
		t.Fatalf("Classes(vendored/pkg) = %v, want ErrDirNotInWorkspace", err)
	}
	// The foreign module root itself is also not covered.
	_, err = w.Classes(filepath.Join(root, "app", "vendored"))
	if !errors.Is(err, ErrDirNotInWorkspace) {
		t.Fatalf("Classes(vendored) = %v, want ErrDirNotInWorkspace", err)
	}
	// A sibling directory of the foreign module, still under app, is covered.
	cls, err := w.Classes(filepath.Join(root, "app", "ok"))
	if err != nil {
		t.Fatal("Classes(app/ok):", err)
	}
	if !cls.IsClass("_app.gox") {
		t.Fatal("app/ok did not resolve to module app")
	}
}

// TestClassesRelativeAndNonExistentDir checks that a relative dir resolves
// against the working directory and that a non-existent directory under a
// module root still resolves to that module.
func TestClassesRelativeAndNonExistentDir(t *testing.T) {
	root := writeFiles(t, map[string]string{
		"app/go.mod":  "module example.com/app\n\ngo 1.21\n",
		"app/gox.mod": "xgo 1.2\n\nproject _app.gox App example.com/app\n",
	})
	w, err := LoadGoMod(filepath.Join(root, "app", "go.mod"))
	if err != nil {
		t.Fatal("LoadGoMod:", err)
	}

	// Relative dir: resolve against the module directory used as the working dir.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal("Getwd:", err)
	}
	t.Cleanup(func() { os.Chdir(wd) })
	if err := os.Chdir(filepath.Join(root, "app")); err != nil {
		t.Fatal("Chdir:", err)
	}
	cls, err := w.Classes(".")
	if err != nil {
		t.Fatal(`Classes("."):`, err)
	}
	if !cls.IsClass("_app.gox") {
		t.Fatal(`Classes(".") did not resolve to module app`)
	}

	// Non-existent directory under the module root still resolves to the module.
	clsNew, err := w.Classes(filepath.Join(root, "app", "does", "not", "exist"))
	if err != nil {
		t.Fatal("Classes(non-existent):", err)
	}
	if clsNew != cls {
		t.Fatal("non-existent subdir did not share the module's cached classes")
	}
}

// TestClassesPerModuleErrorIsolation checks that a build failure in one module
// is returned for every call into that module and does not affect another.
func TestClassesPerModuleErrorIsolation(t *testing.T) {
	root, w := loadWS(t, map[string]string{
		"ws/go.work":  "go 1.22\n\nuse (\n\t./a\n\t./b\n)\n",
		"ws/a/go.mod": "module example.com/a\n\ngo 1.21\n",
		"ws/a/gox.mod": "xgo 1.2\n\n" +
			"project _a.gox App example.com/a\n",
		// b's gox.mod is malformed, so building b fails.
		"ws/b/go.mod":  "module example.com/b\n\ngo 1.21\n",
		"ws/b/gox.mod": "xgo 1.2\n\nproject\n", // incomplete project directive
	})
	// a builds fine and is cached.
	clsA, errA := w.Classes(filepath.Join(root, "ws", "a"))
	if errA != nil {
		t.Fatal("Classes(a):", errA)
	}
	if !clsA.IsClass("_a.gox") {
		t.Fatal("Classes(a): _a.gox not recognized")
	}
	// b fails, and the same error is returned on a later call (cached per module).
	b1, eb1 := w.Classes(filepath.Join(root, "ws", "b"))
	if eb1 == nil {
		t.Fatal("Classes(b): expected an error for a malformed gox.mod")
	}
	b2, eb2 := w.Classes(filepath.Join(root, "ws", "b", "sub"))
	if b1 != b2 || eb1 != eb2 {
		t.Fatalf("Classes(b) not cached per module: %p/%v vs %p/%v", b1, eb1, b2, eb2)
	}
	// a is still fine after b failed.
	clsA2, errA2 := w.Classes(filepath.Join(root, "ws", "a"))
	if errA2 != nil || clsA2 != clsA {
		t.Fatalf("Classes(a) changed after b failed: %p/%v, want %p/nil", clsA2, errA2, clsA)
	}
}

// TestClassesNotInWorkspaceNotCached checks that an ErrDirNotInWorkspace result
// is not cached: a directory that is outside one workspace resolves once a new
// workspace covers it.
func TestClassesNotInWorkspaceNotCached(t *testing.T) {
	root := writeFiles(t, map[string]string{
		"app/go.mod":  "module example.com/app\n\ngo 1.21\n",
		"app/gox.mod": "xgo 1.2\n\nproject _app.gox App example.com/app\n",
	})
	// First a go.mod-only workspace that does not cover the sibling directory.
	w1, err := LoadGoMod(filepath.Join(root, "app", "go.mod"))
	if err != nil {
		t.Fatal("LoadGoMod:", err)
	}
	sibling := filepath.Join(root, "lib")
	if _, err := w1.Classes(sibling); !errors.Is(err, ErrDirNotInWorkspace) {
		t.Fatalf("Classes(lib) in single-module workspace = %v, want ErrDirNotInWorkspace", err)
	}

	// A new workspace that use-s both modules now covers the directory.
	writeFile(t, filepath.Join(root, "lib", "go.mod"), "module example.com/lib\n\ngo 1.21\n")
	writeFile(t, filepath.Join(root, "lib", "gox.mod"), "xgo 1.2\n\nproject _lib.gox App example.com/lib\n")
	writeFile(t, filepath.Join(root, "go.work"), "go 1.22\n\nuse (\n\t./app\n\t./lib\n)\n")
	w2, err := LoadGoWork(filepath.Join(root, "go.work"))
	if err != nil {
		t.Fatal("LoadGoWork:", err)
	}
	cls, err := w2.Classes(sibling)
	if err != nil {
		t.Fatal("Classes(lib) in two-module workspace:", err)
	}
	if !cls.IsClass("_lib.gox") {
		t.Fatal("lib did not resolve to module lib")
	}
}

// TestClassesConcurrentDifferentModules exercises concurrent Classes calls that
// resolve to different modules, to catch cross-module data races under -race.
func TestClassesConcurrentDifferentModules(t *testing.T) {
	root, w := loadWS(t, map[string]string{
		"ws/go.work":  "go 1.22\n\nuse (\n\t./a\n\t./b\n)\n",
		"ws/a/go.mod": "module example.com/a\n\ngo 1.21\n",
		"ws/a/gox.mod": "xgo 1.2\n\n" +
			"project _a.gox App example.com/a\n",
		"ws/b/go.mod": "module example.com/b\n\ngo 1.21\n",
		"ws/b/gox.mod": "xgo 1.2\n\n" +
			"project _b.gox App example.com/b\n",
	})
	dirs := []string{filepath.Join(root, "ws", "a"), filepath.Join(root, "ws", "b")}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := w.Classes(dirs[i%2]); err != nil {
				t.Errorf("Classes(%s) = %v", dirs[i%2], err)
			}
		}(i)
	}
	wg.Wait()
}
