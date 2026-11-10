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
	"testing"

	"github.com/goplus/mod/modcache"
	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
)

// writeFiles writes files (slash-separated paths relative to a fresh temporary
// root) and returns the root.
func writeFiles(t *testing.T, files map[string]string) (root string) {
	t.Helper()
	root = tempRoot(t)
	for rel, content := range files {
		writeFile(t, filepath.Join(root, filepath.FromSlash(rel)), content)
	}
	return root
}

// loadWS writes files and loads the workspace described by "ws/go.work".
func loadWS(t *testing.T, files map[string]string) (root string, w *Workspace) {
	t.Helper()
	root = writeFiles(t, files)
	w, err := LoadGoWork(filepath.Join(root, "ws", "go.work"))
	if err != nil {
		t.Fatal(err)
	}
	return root, w
}

// standard is a workspace whose go.work is not next to any of its modules'
// go.mod files, and one of them lives outside the go.work directory.
//
//	ws/go.work                     use ./app ../shared/lib
//	ws/app/go.mod                  example.com/app
//	ws/app/internal/nested/go.mod  example.com/app/nested  (not in use)
//	ws/other/go.mod                example.com/other       (not in use)
//	shared/lib/go.mod              example.com/lib
var standard = map[string]string{
	"ws/go.work":                    "go 1.22\n\nuse (\n\t./app\n\t../shared/lib\n)\n",
	"ws/app/go.mod":                 "module example.com/app\n\ngo 1.21\n",
	"ws/app/cmd/main.go":            "package main\n",
	"ws/app/internal/nested/go.mod": "module example.com/app/nested\n",
	"ws/other/go.mod":               "module example.com/other\n",
	"shared/lib/go.mod":             "module example.com/lib\n\ngo 1.20\n",
	"shared/lib/sub/x.go":           "package sub\n",
}

func TestLoadGoWork(t *testing.T) {
	w := &Workspace{
		File: new(modfile.WorkFile),
	}
	if w.GoVersion() != "" {
		t.Errorf("GoVersion of empty workspace = %q, want empty string", w.GoVersion())
	}
	root, w := loadWS(t, standard)
	if w.Dir != filepath.Join(root, "ws") {
		t.Errorf("Dir = %q", w.Dir)
	}
	if got := w.GoVersion(); got != "1.22" {
		t.Errorf("GoVersion = %q", got)
	}
	mods, err := w.Modules()
	if err != nil {
		t.Fatal(err)
	}
	// Two main modules, each an entry whose Real is its own directory.
	want := map[string]string{
		"example.com/app": filepath.Join(root, "ws", "app"),
		"example.com/lib": filepath.Join(root, "shared", "lib"),
	}
	gotMain := 0
	for _, m := range mods {
		dir, ok := want[m.Path]
		if !ok {
			continue
		}
		gotMain++
		if m.Real.Path != dir || m.Real.Version != "" {
			t.Errorf("module %q Real = %+v, want dir %q", m.Path, m.Real, dir)
		}
	}
	if gotMain != 2 {
		t.Fatalf("found %d main modules in %+v, want 2", gotMain, mods)
	}
	// LookupModule finds a main module by its exact path.
	if mv, ok := w.LookupModule("example.com/lib"); !ok || mv.Path != filepath.Join(root, "shared", "lib") {
		t.Errorf("LookupModule(lib) = %+v, %v", mv, ok)
	}
	if _, ok := w.LookupModule("example.com/none"); ok {
		t.Error("LookupModule(none) should be false")
	}
}

func TestModulesSortedDescending(t *testing.T) {
	_, w := loadWS(t, map[string]string{
		"ws/go.work":    "go 1.22\nuse (\n\t./a\n\t./a/b\n)\n",
		"ws/a/go.mod":   "module example.com/a\n",
		"ws/a/b/go.mod": "module example.com/a/b\n",
	})
	mods, err := w.Modules()
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i < len(mods); i++ {
		if mods[i-1].Path < mods[i].Path {
			t.Errorf("not sorted descending: %q before %q", mods[i-1].Path, mods[i].Path)
		}
	}
	// The longer path sorts first, so the longest prefix wins.
	if mods[0].Path != "example.com/a/b" {
		t.Errorf("first module = %q, want example.com/a/b", mods[0].Path)
	}
}

func TestPkgTypeString(t *testing.T) {
	cases := []struct {
		t    PkgType
		want string
	}{
		{PkgtInvalid, "invalid"},
		{PkgtStandard, "standard"},
		{PkgtLocal, "local"},
		{PkgtExtern, "extern"},
		{PkgType(99), "invalid"},
	}
	for _, c := range cases {
		if got := c.t.String(); got != c.want {
			t.Errorf("PkgType(%d).String() = %q, want %q", c.t, got, c.want)
		}
	}
	// PkgtInvalid is the zero value.
	var zero PkgType
	if zero != PkgtInvalid {
		t.Errorf("zero PkgType = %v, want PkgtInvalid", zero)
	}
	var zeroPkg Package
	if zeroPkg.Type != PkgtInvalid {
		t.Errorf("zero Package.Type = %v, want PkgtInvalid", zeroPkg.Type)
	}
}

// -----------------------------------------------------------------------------
// Construction

func TestLoadGoWorkErrors(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{
			name:  "use directory without go.mod",
			files: map[string]string{"ws/go.work": "go 1.22\n\nuse ./missing\n"},
			want:  "use ./missing",
		},
		{
			name: "go.mod without module directive",
			files: map[string]string{
				"ws/go.work":  "go 1.22\n\nuse ./a\n",
				"ws/a/go.mod": "go 1.22\n",
			},
			want: "no module declaration",
		},
		{
			name:  "syntax error in go.work",
			files: map[string]string{"ws/go.work": "use (\n"},
			want:  "go.work",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := writeFiles(t, c.files)
			_, err := LoadGoWork(filepath.Join(root, "ws", "go.work"))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want it to contain %q", err, c.want)
			}
		})
	}
	if _, err := LoadGoWork(filepath.Join(tempRoot(t), "nope", "go.work")); err == nil {
		t.Error("loading a missing file should fail")
	}
}

func TestLoadGoModIsStrict(t *testing.T) {
	root := tempRoot(t)

	// Main-module-only directives are kept, and Path/File are set from the file.
	writeFile(t, filepath.Join(root, "ok", "go.mod"),
		"module example.com/x\n\ngo 1.21\n\nreplace example.com/dep => ../dep\n")
	w, err := LoadGoMod(filepath.Join(root, "ok", "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	// A go.mod-only workspace has no go.work.
	if w.File != nil {
		t.Errorf("File = %v; want nil", w.File)
	}
	if w.Dir != filepath.Join(root, "ok") {
		t.Errorf("Dir = %q, want the module directory", w.Dir)
	}
	if got := w.GoVersion(); got != "1.21" {
		t.Errorf("GoVersion = %q, want 1.21", got)
	}
	if err := w.Validate(); err != nil {
		t.Errorf("Validate without go.work = %v", err)
	}
	// Its own replace applies.
	repl, err := w.replacements()
	if err != nil {
		t.Fatal(err)
	}
	want := module.Version{Path: filepath.Join(root, "dep")}
	if repl[module.Version{Path: "example.com/dep"}] != want {
		t.Errorf("replacements = %v", repl)
	}

	// Unknown directives are an error, as they are for the go command.
	writeFile(t, filepath.Join(root, "bad", "go.mod"),
		"module example.com/y\n\nunknowndirective foo bar\n")
	if _, err := LoadGoMod(filepath.Join(root, "bad", "go.mod")); err == nil {
		t.Error("an unknown directive should be rejected")
	}

	// A go.mod without a module directive is rejected.
	writeFile(t, filepath.Join(root, "nomod", "go.mod"), "go 1.21\n")
	if _, err := LoadGoMod(filepath.Join(root, "nomod", "go.mod")); err == nil {
		t.Error("a go.mod without module should be rejected")
	}
}

func TestNewGoWorkKeepsRelativeUseBase(t *testing.T) {
	root := tempRoot(t)
	writeFile(t, filepath.Join(root, "ws", "a", "go.mod"), "module example.com/a\n")
	f, err := parseWork(filepath.Join(root, "ws", "go.work"), "go 1.22\nuse ./a\n")
	if err != nil {
		t.Fatal(err)
	}
	w, err := NewGoWork(filepath.Join(root, "ws"), f)
	if err != nil {
		t.Fatal(err)
	}
	dir, _ := w.LookupModule("example.com/a")
	if dir.Path != filepath.Join(root, "ws", "a") {
		t.Fatalf("module a Real = %+v", dir)
	}
}

func TestLoadDir(t *testing.T) {
	root, _ := loadWS(t, standard)

	// From a nested directory inside a main module, find the go.work above.
	w, err := LoadFromDir(filepath.Join(root, "ws", "app", "cmd"))
	if err != nil {
		t.Fatal(err)
	}

	// A go.work in a parent wins over a nearer go.mod: ws/app has a go.mod but
	// LoadDir from ws/app still finds ws/go.work.
	w, err = LoadFromDir(filepath.Join(root, "ws", "app"))
	if err != nil {
		t.Fatal(err)
	}
	if w.File == nil {
		t.Error("LoadDir should prefer the go.work in the parent")
	}
}

func TestLoadDirGoModOnly(t *testing.T) {
	root := tempRoot(t)
	writeFile(t, filepath.Join(root, "proj", "go.mod"), "module example.com/proj\n\ngo 1.21\n")
	writeFile(t, filepath.Join(root, "proj", "sub", "x.go"), "package sub\n")

	// No go.work anywhere, so LoadDir finds the nearest go.mod.
	w, err := LoadFromDir(filepath.Join(root, "proj", "sub"))
	if err != nil {
		t.Fatal(err)
	}
	if w.File != nil {
		t.Errorf("expected a go.mod-only workspace, got File=%v", w.File)
	}
	if w.Dir != filepath.Join(root, "proj") {
		t.Errorf("Dir = %q, want the module directory", w.Dir)
	}
	pkg, err := w.Lookup("example.com/proj/sub")
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Type != PkgtLocal || pkg.Dir != filepath.Join(root, "proj", "sub") {
		t.Errorf("lookup = %+v", pkg)
	}
}

func TestLoadDirNoModule(t *testing.T) {
	root := tempRoot(t)
	// With no go.work or go.mod above it, LoadFromDir no longer fails: it returns
	// the default workspace for the directory.
	w, err := LoadFromDir(root)
	if err != nil {
		t.Fatalf("LoadFromDir outside any module = %v, want a default workspace", err)
	}
	if !w.IsDefault() {
		t.Error("LoadFromDir outside any module should return a default workspace")
	}
	if w.Dir != root {
		t.Errorf("Dir = %q, want %q", w.Dir, root)
	}
}

func TestLoadDirHonorsGOWORK(t *testing.T) {
	root := tempRoot(t)
	writeFile(t, filepath.Join(root, "proj", "go.mod"), "module example.com/proj\n\ngo 1.21\n")
	writeFile(t, filepath.Join(root, "ws.work"),
		"go 1.22\nuse ./proj\n")

	t.Run("GOWORK=path", func(t *testing.T) {
		t.Setenv("GOWORK", filepath.Join(root, "ws.work"))
		w, err := LoadFromDir(filepath.Join(root, "proj"))
		if err != nil {
			t.Fatal(err)
		}
		_ = w
	})

	t.Run("GOWORK=off", func(t *testing.T) {
		t.Setenv("GOWORK", "off")
		w, err := LoadFromDir(filepath.Join(root, "proj"))
		if err != nil {
			t.Fatal(err)
		}
		if w.File != nil {
			t.Error("GOWORK=off should skip go.work")
		}
	})
}

func TestLoadDirRelative(t *testing.T) {
	root := tempRoot(t)
	writeFile(t, filepath.Join(root, "go.mod"), "module example.com/here\n\ngo 1.21\n")
	wd, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	defer chdir(t, wd)
	chdir(t, root)
	w, err := LoadFromDir(".")
	if err != nil {
		t.Fatal(err)
	}
	if w.Dir != root {
		t.Errorf("Dir = %q, want %q", w.Dir, root)
	}
}

// TestConstructorsAgree checks that LoadGoWork, and LoadGoMod on a single-module
// project, give the same lookups through the one entry point.
func TestConstructorsAgree(t *testing.T) {
	root := tempRoot(t)
	writeFile(t, filepath.Join(root, "ws", "go.work"), "go 1.22\nuse ./app\n")
	writeFile(t, filepath.Join(root, "ws", "app", "go.mod"), "module example.com/app\n\ngo 1.21\n")
	writeFile(t, filepath.Join(root, "ws", "app", "x", "x.go"), "package x\n")

	fromWork, err := LoadGoWork(filepath.Join(root, "ws", "go.work"))
	if err != nil {
		t.Fatal(err)
	}
	fromMod, err := LoadGoMod(filepath.Join(root, "ws", "app", "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	fromDir, err := LoadFromDir(filepath.Join(root, "ws", "app"))
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range []string{"example.com/app", "example.com/app/x", "fmt"} {
		pw, ew := fromWork.Lookup(imp)
		pm, em := fromMod.Lookup(imp)
		pd, ed := fromDir.Lookup(imp)
		if (ew == nil) != (em == nil) || (ew == nil) != (ed == nil) {
			t.Errorf("%q: err work=%v mod=%v dir=%v", imp, ew, em, ed)
			continue
		}
		if ew != nil {
			continue
		}
		if pw.Type != pm.Type || pw.Dir != pm.Dir || pw.Type != pd.Type || pw.Dir != pd.Dir {
			t.Errorf("%q: work=%+v mod=%+v dir=%+v", imp, pw, pm, pd)
		}
	}
}

// -----------------------------------------------------------------------------
// Lookup: classification over a workspace with dependencies

// depWS builds a workspace with two main modules, a normal dependency, and a
// dependency replaced by a local directory, plus a fake module cache that holds
// the dependency's .mod file. It returns the root and the loaded workspace.
//
//	ws/go.work      use ./app ../shared/lib
//	ws/app/go.mod   example.com/app, requires github.com/dep/thing v1.2.0 and
//	                example.com/local v1.0.0, replaces example.com/local =>
//	                ../localdep
//	shared/lib      example.com/lib
//	localdep        example.com/local
func depWS(t *testing.T) (root string, w *Workspace) {
	t.Helper()
	root = writeFiles(t, map[string]string{
		"ws/go.work": "go 1.22\n\nuse (\n\t./app\n\t../shared/lib\n)\n",
		"ws/app/go.mod": "module example.com/app\n\ngo 1.21\n\n" +
			"require (\n\tgithub.com/dep/thing v1.2.0\n\texample.com/local v1.0.0\n)\n\n" +
			"replace example.com/local => ../../localdep\n",
		"shared/lib/go.mod": "module example.com/lib\n\ngo 1.20\n",
		"localdep/go.mod":   "module example.com/local\n\ngo 1.21\n",
	})
	// Fake module cache entry for the dependency's .mod file.
	fakeMod(t, root, "github.com/dep/thing", "v1.2.0", "module github.com/dep/thing\n\ngo 1.21\n")
	setModCache(t, filepath.Join(root, "modcache"))

	w, err := LoadGoWork(filepath.Join(root, "ws", "go.work"))
	if err != nil {
		t.Fatal(err)
	}
	return root, w
}

func TestLookup(t *testing.T) {
	root, w := depWS(t)
	cases := []struct {
		imp     string
		typ     PkgType
		dir     string // relative to root; "" when checked specially
		modPath string
		real    module.Version
	}{
		{"fmt", PkgtStandard, "", "", module.Version{}},
		{"example.com/app/cmd", PkgtLocal, "ws/app/cmd", "example.com/app", module.Version{}},
		{"example.com/app", PkgtLocal, "ws/app", "example.com/app", module.Version{}},
		{"example.com/lib/sub", PkgtLocal, "shared/lib/sub", "example.com/lib", module.Version{}},
		{"example.com/local/util", PkgtLocal, "localdep/util", "example.com/local", module.Version{}},
		{"github.com/dep/thing/x", PkgtExtern, "", "github.com/dep/thing",
			module.Version{Path: "github.com/dep/thing", Version: "v1.2.0"}},
	}
	for _, c := range cases {
		pkg, err := w.Lookup(c.imp)
		if err != nil {
			t.Errorf("LookupFrom(%q) = %v", c.imp, err)
			continue
		}
		if pkg.Type != c.typ {
			t.Errorf("LookupFrom(%q).Type = %v, want %v", c.imp, pkg.Type, c.typ)
		}
		if c.dir != "" && pkg.Dir != filepath.Join(root, filepath.FromSlash(c.dir)) {
			t.Errorf("LookupFrom(%q).Dir = %q, want %q", c.imp, pkg.Dir, c.dir)
		}
		if c.modPath != "" && pkg.ModPath != c.modPath {
			t.Errorf("LookupFrom(%q).ModPath = %q, want %q", c.imp, pkg.ModPath, c.modPath)
		}
		if pkg.Real != c.real {
			t.Errorf("LookupFrom(%q).Real = %+v, want %+v", c.imp, pkg.Real, c.real)
		}
	}

	// A standard package reads from GOROOT/src.
	pkg, err := w.Lookup("fmt")
	if err != nil {
		t.Fatal(err)
	}
	if pkg.ModDir != filepath.Join(w.goroot, "src") || pkg.Dir != filepath.Join(w.goroot, "src", "fmt") {
		t.Errorf("fmt = %+v", pkg)
	}
}

func TestPkgTypeClassification(t *testing.T) {
	root, w := depWS(t)
	cases := []struct {
		path string
		want PkgType
	}{
		{"", PkgtInvalid},
		{"./x", PkgtLocal},
		{"../y", PkgtLocal},
		{"fmt", PkgtStandard},
		{"net/http", PkgtStandard},
		{"example.com/app", PkgtLocal},
		{"example.com/app/cmd", PkgtLocal},
		{"example.com/lib", PkgtLocal},
		{"example.com/local", PkgtLocal},     // local replace
		{"github.com/dep/thing", PkgtExtern}, // in the module cache
		{"github.com/dep/thing/sub", PkgtExtern},
		{"github.com/not/listed", PkgtExtern}, // dotless-less: has a dot, meant to be extern
		{"notadomain", PkgtInvalid},           // dotless, not std, in no module
	}
	for _, c := range cases {
		if got := w.PkgType(c.path); got != c.want {
			t.Errorf("PkgType(%q) = %v, want %v", c.path, got, c.want)
		}
	}
	_ = root
}

func TestStdlibWinsOverModule(t *testing.T) {
	// A main module whose path equals a standard package: the standard package
	// wins, but a dotless main module path still works for its own subpackages.
	root := writeFiles(t, map[string]string{
		"ws/go.work":       "go 1.22\nuse (\n\t./fmtmod\n\t./hello\n)\n",
		"ws/fmtmod/go.mod": "module fmt\n\ngo 1.21\n",
		"ws/hello/go.mod":  "module hello\n\ngo 1.21\n",
	})
	w, err := LoadGoWork(filepath.Join(root, "ws", "go.work"))
	if err != nil {
		t.Fatal(err)
	}
	// "fmt" resolves to the standard library, not the module named fmt.
	if got := w.PkgType("fmt"); got != PkgtStandard {
		t.Errorf("PkgType(fmt) = %v, want standard", got)
	}
	pkg, err := w.Lookup("fmt")
	if err != nil || pkg.Type != PkgtStandard {
		t.Errorf("LookupFrom(fmt) = %+v, %v", pkg, err)
	}
	// "hello/x" is not in GOROOT, so the dotless main module owns it.
	if got := w.PkgType("hello/x"); got != PkgtLocal {
		t.Errorf("PkgType(hello/x) = %v, want local", got)
	}
	pkg, err = w.Lookup("hello/x")
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Type != PkgtLocal || pkg.Dir != filepath.Join(root, "ws", "hello", "x") {
		t.Errorf("LookupFrom(hello/x) = %+v", pkg)
	}
}

// TestStdlibCacheNotPoisoned guards against a per-first-element cache that is
// keyed on the first path element but decided by the full path: before the fix,
// a lookup of a non-existent subpackage ("encoding/doesnotexist") cached
// "encoding" as non-standard and so misclassified the real "encoding/json".
// isStandard is a first-element test, so any path rooted at a standard top-level
// package is classified standard, independent of lookup order.
func TestStdlibCacheNotPoisoned(t *testing.T) {
	root := writeFiles(t, map[string]string{
		"ws/go.work":    "go 1.22\nuse ./app\n",
		"ws/app/go.mod": "module example.com/app\n\ngo 1.21\n",
	})
	// Fresh workspaces so the two orders use independent caches.
	load := func() *Workspace {
		w, err := LoadGoWork(filepath.Join(root, "ws", "go.work"))
		if err != nil {
			t.Fatal(err)
		}
		return w
	}

	// "encoding/doesnotexist" shares the first element "encoding" with the
	// standard "encoding/json"; both are classified by that first element.
	w := load()
	_ = w.PkgType("encoding/doesnotexist") // populate the cache first
	if got := w.PkgType("encoding/json"); got != PkgtStandard {
		t.Errorf("after a sibling miss, PkgType(encoding/json) = %v, want standard", got)
	}

	// The reverse order gives the same answer.
	w = load()
	if got := w.PkgType("encoding/json"); got != PkgtStandard {
		t.Errorf("PkgType(encoding/json) = %v, want standard", got)
	}
	pkg, err := w.Lookup("encoding/json")
	if err != nil || pkg.Type != PkgtStandard {
		t.Errorf("LookupFrom(encoding/json) = %+v, %v; want standard", pkg, err)
	}
}

func TestLookupMissing(t *testing.T) {
	_, w := depWS(t)
	_, err := w.Lookup("github.com/nope/here")
	var me *MissingError
	if !errors.As(err, &me) {
		t.Fatalf("err = %v, want *MissingError", err)
	}
	if me.Path != "github.com/nope/here" {
		t.Errorf("MissingError.Path = %q", me.Path)
	}
	if !strings.Contains(me.Error(), "github.com/nope/here") {
		t.Errorf("MissingError.Error() = %q", me.Error())
	}
	if _, err := w.Lookup(""); !errors.Is(err, ErrInvalidPkgPath) {
		t.Errorf("empty path = %v, want ErrInvalidPkgPath", err)
	}
}

func TestLongestPrefixWins(t *testing.T) {
	// A required module nested under a main module's path: the longer path wins.
	root := writeFiles(t, map[string]string{
		"ws/go.work":      "go 1.22\nuse (\n\t./app\n\t./extra\n)\n",
		"ws/app/go.mod":   "module example.com/app\n\ngo 1.21\n",
		"ws/extra/go.mod": "module example.com/app/extra\n\ngo 1.21\n",
	})
	w, err := LoadGoWork(filepath.Join(root, "ws", "go.work"))
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := w.Lookup("example.com/app/extra/x")
	if err != nil {
		t.Fatal(err)
	}
	if pkg.ModPath != "example.com/app/extra" || pkg.Dir != filepath.Join(root, "ws", "extra", "x") {
		t.Errorf("lookup = %+v, want the longer module", pkg)
	}
	// A package under the shorter module is still found.
	pkg, err = w.Lookup("example.com/app/other")
	if err != nil {
		t.Fatal(err)
	}
	if pkg.ModPath != "example.com/app" {
		t.Errorf("lookup = %+v, want example.com/app", pkg)
	}
}

func TestV2ModulePaths(t *testing.T) {
	root := writeFiles(t, map[string]string{
		"ws/go.work":   "go 1.22\nuse (\n\t./v1\n\t./v2\n)\n",
		"ws/v1/go.mod": "module example.com/x\n\ngo 1.21\n",
		"ws/v2/go.mod": "module example.com/x/v2\n\ngo 1.21\n",
	})
	w, err := LoadGoWork(filepath.Join(root, "ws", "go.work"))
	if err != nil {
		t.Fatal(err)
	}
	// example.com/x/v2 is a different path from example.com/x.
	p2, err := w.Lookup("example.com/x/v2/sub")
	if err != nil {
		t.Fatal(err)
	}
	if p2.ModPath != "example.com/x/v2" || p2.Dir != filepath.Join(root, "ws", "v2", "sub") {
		t.Errorf("v2 lookup = %+v", p2)
	}
	p1, err := w.Lookup("example.com/x/sub")
	if err != nil {
		t.Fatal(err)
	}
	if p1.ModPath != "example.com/x" {
		t.Errorf("v1 lookup = %+v", p1)
	}
}

// -----------------------------------------------------------------------------
// Version selection with a fake module cache

func TestVersionSelection(t *testing.T) {
	t.Run("highest among main modules", func(t *testing.T) {
		root := writeFiles(t, map[string]string{
			"ws/go.work":  "go 1.22\nuse (\n\t./a\n\t./b\n)\n",
			"ws/a/go.mod": "module example.com/a\n\ngo 1.21\n\nrequire github.com/x/dep v1.1.0\n",
			"ws/b/go.mod": "module example.com/b\n\ngo 1.21\n\nrequire github.com/x/dep v1.3.0\n",
		})
		fakeMod(t, root, "github.com/x/dep", "v1.1.0", "module github.com/x/dep\n\ngo 1.21\n")
		fakeMod(t, root, "github.com/x/dep", "v1.3.0", "module github.com/x/dep\n\ngo 1.21\n")
		setModCache(t, filepath.Join(root, "modcache"))
		w, err := LoadGoWork(filepath.Join(root, "ws", "go.work"))
		if err != nil {
			t.Fatal(err)
		}
		mv, ok := w.LookupModule("github.com/x/dep")
		if !ok || mv.Version != "v1.3.0" {
			t.Errorf("selected = %+v, want v1.3.0", mv)
		}
	})

	t.Run("dependency raises the version", func(t *testing.T) {
		// The main module lists dep v1.0.0, but its go.mod requires upstream
		// v2.5.0, higher than the main module's own v2.0.0 for upstream.
		root := writeFiles(t, map[string]string{
			"ws/go.work": "go 1.22\nuse ./a\n",
			"ws/a/go.mod": "module example.com/a\n\ngo 1.21\n\n" +
				"require (\n\tgithub.com/x/dep v1.0.0\n\tgithub.com/x/up v2.0.0+incompatible\n)\n",
		})
		fakeMod(t, root, "github.com/x/dep", "v1.0.0",
			"module github.com/x/dep\n\ngo 1.21\n\nrequire github.com/x/up v2.5.0+incompatible\n")
		fakeMod(t, root, "github.com/x/up", "v2.0.0+incompatible", "module github.com/x/up\n\ngo 1.21\n")
		fakeMod(t, root, "github.com/x/up", "v2.5.0+incompatible", "module github.com/x/up\n\ngo 1.21\n")
		setModCache(t, filepath.Join(root, "modcache"))
		w, err := LoadGoWork(filepath.Join(root, "ws", "go.work"))
		if err != nil {
			t.Fatal(err)
		}
		mv, ok := w.LookupModule("github.com/x/up")
		if !ok || mv.Version != "v2.5.0+incompatible" {
			t.Errorf("selected = %+v, want v2.5.0+incompatible", mv)
		}
	})

	t.Run("directory replacement go.mod requirements are read", func(t *testing.T) {
		root := writeFiles(t, map[string]string{
			"ws/go.work": "go 1.22\nuse ./a\n",
			"ws/a/go.mod": "module example.com/a\n\ngo 1.21\n\n" +
				"require github.com/x/dep v1.0.0\n\n" +
				"replace github.com/x/dep => ../../depdir\n",
			"depdir/go.mod": "module github.com/x/dep\n\ngo 1.21\n\nrequire github.com/x/trans v1.4.0\n",
		})
		fakeMod(t, root, "github.com/x/trans", "v1.4.0", "module github.com/x/trans\n\ngo 1.21\n")
		setModCache(t, filepath.Join(root, "modcache"))
		w, err := LoadGoWork(filepath.Join(root, "ws", "go.work"))
		if err != nil {
			t.Fatal(err)
		}
		// dep is replaced by a local directory => PkgtLocal.
		if got := w.PkgType("github.com/x/dep"); got != PkgtLocal {
			t.Errorf("PkgType(dep) = %v, want local", got)
		}
		dep, _ := w.LookupModule("github.com/x/dep")
		if dep.Path != filepath.Join(root, "depdir") || dep.Version != "" {
			t.Errorf("dep Real = %+v, want the local dir", dep)
		}
		// The transitive requirement from the replaced go.mod is picked up.
		if _, ok := w.LookupModule("github.com/x/trans"); !ok {
			t.Error("transitive requirement from a directory replacement was not read")
		}
	})

	t.Run("missing .mod is skipped, malformed is an error", func(t *testing.T) {
		root := writeFiles(t, map[string]string{
			"ws/go.work":  "go 1.22\nuse ./a\n",
			"ws/a/go.mod": "module example.com/a\n\ngo 1.21\n\nrequire github.com/x/dep v1.0.0\n",
		})
		// No .mod written for dep: it is skipped, lookup still works.
		setModCache(t, filepath.Join(root, "modcache"))
		w, err := LoadGoWork(filepath.Join(root, "ws", "go.work"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Modules(); err != nil {
			t.Fatalf("a missing .mod should be skipped, got %v", err)
		}
		// dep is still selectable at its required version (classified extern).
		if got := w.PkgType("github.com/x/dep"); got != PkgtExtern {
			t.Errorf("PkgType(dep) = %v, want extern", got)
		}

		// Now a malformed .mod is an error.
		root2 := writeFiles(t, map[string]string{
			"ws/go.work":  "go 1.22\nuse ./a\n",
			"ws/a/go.mod": "module example.com/a\n\ngo 1.21\n\nrequire github.com/x/bad v1.0.0\n",
		})
		fakeMod(t, root2, "github.com/x/bad", "v1.0.0", "require (\n")
		setModCache(t, filepath.Join(root2, "modcache"))
		w2, err := LoadGoWork(filepath.Join(root2, "ws", "go.work"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w2.Modules(); err == nil {
			t.Error("a malformed .mod should be an error")
		}
	})

	t.Run("requirements deeper than one level are not followed", func(t *testing.T) {
		root := writeFiles(t, map[string]string{
			"ws/go.work":  "go 1.22\nuse ./a\n",
			"ws/a/go.mod": "module example.com/a\n\ngo 1.21\n\nrequire github.com/x/one v1.0.0\n",
		})
		fakeMod(t, root, "github.com/x/one", "v1.0.0",
			"module github.com/x/one\n\ngo 1.21\n\nrequire github.com/x/two v1.0.0\n")
		fakeMod(t, root, "github.com/x/two", "v1.0.0",
			"module github.com/x/two\n\ngo 1.21\n\nrequire github.com/x/three v1.0.0\n")
		setModCache(t, filepath.Join(root, "modcache"))
		w, err := LoadGoWork(filepath.Join(root, "ws", "go.work"))
		if err != nil {
			t.Fatal(err)
		}
		// one (level 0) and two (level 1) are present; three (level 2) is not.
		if _, ok := w.LookupModule("github.com/x/one"); !ok {
			t.Error("direct requirement missing")
		}
		if _, ok := w.LookupModule("github.com/x/two"); !ok {
			t.Error("one-level requirement missing")
		}
		if _, ok := w.LookupModule("github.com/x/three"); ok {
			t.Error("two-level requirement should not be followed")
		}
	})
}

// -----------------------------------------------------------------------------
// Replace conflicts

func TestConflictingReplaces(t *testing.T) {
	_, w := loadWS(t, map[string]string{
		"ws/go.work":  "go 1.22\nuse (\n\t./a\n\t./b\n)\n",
		"ws/a/go.mod": "module example.com/a\n\nreplace example.com/dep v1.0.0 => example.com/forkA v1.0.0\n",
		"ws/b/go.mod": "module example.com/b\n\nreplace example.com/dep v1.0.0 => example.com/forkB v1.0.0\n",
	})
	// Modules and Lookup return the conflict error.
	if _, err := w.Modules(); !isConflict(err) {
		t.Errorf("Modules err = %v, want *ReplaceConflictError", err)
	}
	if _, err := w.Lookup("example.com/dep/x"); !isConflict(err) {
		t.Errorf("Lookup err = %v, want *ReplaceConflictError", err)
	}
	// PkgType returns PkgtInvalid when the list cannot be built.
	if got := w.PkgType("example.com/dep/x"); got != PkgtInvalid {
		t.Errorf("PkgType on conflict = %v, want PkgtInvalid", got)
	}

	// A go.work replace of the same module makes the conflict disappear.
	_, w2 := loadWS(t, map[string]string{
		"ws/go.work": "go 1.22\nuse (\n\t./a\n\t./b\n)\n" +
			"replace example.com/dep => example.com/winner v1.0.0\n",
		"ws/a/go.mod": "module example.com/a\n\nreplace example.com/dep v1.0.0 => example.com/forkA v1.0.0\n",
		"ws/b/go.mod": "module example.com/b\n\nreplace example.com/dep v1.0.0 => example.com/forkB v1.0.0\n",
	})
	if _, err := w2.Modules(); err != nil {
		t.Errorf("go.work replace should settle the conflict, got %v", err)
	}
}

func isConflict(err error) bool {
	var ce *ReplaceConflictError
	return errors.As(err, &ce)
}

// -----------------------------------------------------------------------------
// Concurrency

func TestLookupConcurrent(t *testing.T) {
	_, w := depWS(t)
	imports := []string{
		"fmt", "example.com/app/cmd", "example.com/lib/sub",
		"example.com/local/util", "github.com/dep/thing/x", "github.com/nope/here",
	}
	const workers = 16
	done := make(chan struct{})
	for i := 0; i < workers; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 50; j++ {
				for _, imp := range imports {
					w.Lookup(imp)
					w.PkgType(imp)
				}
				w.Modules()
			}
		}()
	}
	for i := 0; i < workers; i++ {
		<-done
	}
}

// -----------------------------------------------------------------------------
// Test helpers

// parseWork parses go.work content for a NewGoWork test.
func parseWork(path, content string) (*modfile.WorkFile, error) {
	return modfile.ParseWork(path, []byte(content), nil)
}

// chdir changes the working directory and restores it on cleanup.
func chdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
}

// fakeMod writes a .mod file into a fake module cache under root/modcache.
func fakeMod(t *testing.T, root, path, ver, content string) {
	t.Helper()
	enc, err := module.EscapePath(path)
	if err != nil {
		t.Fatal(err)
	}
	encVer, err := module.EscapeVersion(ver)
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(root, "modcache", "cache", "download", enc, "@v", encVer+".mod")
	writeFile(t, dst, content)
}

// setModCache points modcache.GOMODCACHE at dir for the duration of the test.
func setModCache(t *testing.T, dir string) {
	t.Helper()
	old := modcache.GOMODCACHE
	modcache.GOMODCACHE = dir
	t.Cleanup(func() { modcache.GOMODCACHE = old })
}

// -----------------------------------------------------------------------------

func TestValidateErrors(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  []string
	}{
		{
			name: "same module path twice",
			files: map[string]string{
				"ws/go.work":  "go 1.22\nuse (\n\t./a\n\t./b\n)\n",
				"ws/a/go.mod": "module example.com/dup\n",
				"ws/b/go.mod": "module example.com/dup\n",
			},
			want: []string{"module example.com/dup appears multiple times"},
		},
		{
			name: "module needs newer go",
			files: map[string]string{
				"ws/go.work":  "go 1.20\nuse ./a\n",
				"ws/a/go.mod": "module example.com/a\n\ngo 1.21\n",
			},
			want: []string{"requires go >= 1.21, but go.work lists go 1.20"},
		},
		{
			name: "conflicting replaces",
			files: map[string]string{
				"ws/go.work":  "go 1.22\nuse (\n\t./a\n\t./b\n)\n",
				"ws/a/go.mod": "module example.com/a\n\nreplace example.com/dep => ../dep1\n",
				"ws/b/go.mod": "module example.com/b\n\nreplace example.com/dep => ../dep2\n",
			},
			want: []string{"conflicting replacements for example.com/dep"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, w := loadWS(t, c.files)
			err := w.Validate()
			if err == nil {
				t.Fatalf("Validate = nil, want errors containing %q", c.want)
			}
			for _, s := range c.want {
				if !strings.Contains(err.Error(), s) {
					t.Errorf("Validate = %q, want it to contain %q", err, s)
				}
			}
		})
	}
}
