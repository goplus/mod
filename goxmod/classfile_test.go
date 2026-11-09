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

	"github.com/goplus/mod/modcache"
	"github.com/goplus/mod/modfile"
	"golang.org/x/mod/module"
)

// setModCache points modcache.GOMODCACHE at dir for the duration of the test.
func setModCache(t *testing.T, dir string) {
	t.Helper()
	old := modcache.GOMODCACHE
	modcache.GOMODCACHE = dir
	t.Cleanup(func() { modcache.GOMODCACHE = old })
}

// writeFile writes content to name, creating parent directories.
func writeFile(t *testing.T, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// seedModGox writes a gox.mod into the extracted module directory of modVer in a
// fake module cache rooted at root/modcache, and points modcache.GOMODCACHE at
// it. It also writes the module's .mod download-cache entry, so the module is
// treated as already present (no fetch).
func seedModGox(t *testing.T, root string, modVer module.Version, goxmodText string) {
	t.Helper()
	enc, err := module.EscapePath(modVer.Path)
	if err != nil {
		t.Fatal(err)
	}
	encVer, err := module.EscapeVersion(modVer.Version)
	if err != nil {
		t.Fatal(err)
	}
	modDir := filepath.Join(root, "modcache", enc+"@"+modVer.Version)
	writeFile(t, filepath.Join(modDir, "gox.mod"), goxmodText)
	// The download-cache .mod file marks the module as already in the cache, so
	// importMod does not try to fetch it.
	dlMod := filepath.Join(root, "modcache", "cache", "download", enc, "@v", encVer+".mod")
	writeFile(t, dlMod, "module "+modVer.Path+"\n\ngo 1.18\n")
}

// lookupFrom builds a LookupModVer from a fixed path->version table.
func lookupFrom(table map[string]module.Version) LookupModVer {
	return func(modPath string) (module.Version, bool) {
		modVer, ok := table[modPath]
		return modVer, ok
	}
}

// -----------------------------------------------------------------------------

// TestImportClassesBuiltins checks that the built-in projects (_test.gox and
// .gsh) are always present, even with an empty module description.
func TestImportClassesBuiltins(t *testing.T) {
	cls, err := ImportClasses(&modfile.File{}, nil)
	if err != nil {
		t.Fatal("ImportClasses:", err)
	}
	if !cls.IsClass("_test.gox") {
		t.Fatal("IsClass _test.gox: not ok?")
	}
	if !cls.IsClass(".gsh") {
		t.Fatal("IsClass .gsh: not ok?")
	}
	if cls.IsClass(".spx") {
		t.Fatal("IsClass .spx: ok?")
	}
}

// TestImportClassesOwnProject checks that a project declared by the module
// itself, and its work-class extension, resolve through IsClass and LookupClass.
func TestImportClassesOwnProject(t *testing.T) {
	gox := &modfile.File{
		Projects: []*Project{
			{
				Ext:      ".spx",
				Class:    "Game",
				PkgPaths: []string{"github.com/goplus/spx"},
				Works:    []*modfile.Class{{Ext: ".spx", Class: "Sprite"}},
			},
		},
	}
	cls, err := ImportClasses(gox, nil)
	if err != nil {
		t.Fatal("ImportClasses:", err)
	}
	c, ok := cls.LookupClass(".spx")
	if !ok || c.Class != "Game" {
		t.Fatal("LookupClass .spx:", c, ok)
	}
	if c.PkgPaths[0] != "github.com/goplus/spx" {
		t.Fatal("PkgPaths:", c.PkgPaths)
	}
}

// TestImportClassesWorkExtResolves checks that a work-class extension distinct
// from the project extension resolves to the same project.
func TestImportClassesWorkExtResolves(t *testing.T) {
	gox := &modfile.File{
		Projects: []*Project{
			{
				Ext:   "_yap.gox",
				Class: "App",
				Works: []*modfile.Class{{Ext: "_ytest.gox", Class: "Case"}},
			},
		},
	}
	cls, err := ImportClasses(gox, nil)
	if err != nil {
		t.Fatal("ImportClasses:", err)
	}
	proj, ok := cls.LookupClass("_yap.gox")
	if !ok {
		t.Fatal("LookupClass _yap.gox: not ok?")
	}
	work, ok := cls.LookupClass("_ytest.gox")
	if !ok {
		t.Fatal("LookupClass _ytest.gox: not ok?")
	}
	if work != proj {
		t.Fatal("work extension does not resolve to its project")
	}
}

// TestImportClassesProjectReplacesBuiltin checks that a module project with the
// same extension as a built-in replaces the built-in.
func TestImportClassesProjectReplacesBuiltin(t *testing.T) {
	gox := &modfile.File{
		Projects: []*Project{
			{Ext: ".gsh", Class: "Shell", PkgPaths: []string{"example.com/shell"}},
		},
	}
	cls, err := ImportClasses(gox, nil)
	if err != nil {
		t.Fatal("ImportClasses:", err)
	}
	c, ok := cls.LookupClass(".gsh")
	if !ok || c.Class != "Shell" {
		t.Fatal("LookupClass .gsh:", c, ok)
	}
}

// TestImportClassesClassModFromCache checks that a class module's projects
// become visible, loaded from the module cache without a fetch.
func TestImportClassesClassModFromCache(t *testing.T) {
	root := t.TempDir()
	modVer := module.Version{Path: "example.com/foo", Version: "v1.0.0"}
	seedModGox(t, root, modVer, `
xgo 1.2

project _foo.gox App example.com/foo
`)
	setModCache(t, filepath.Join(root, "modcache"))

	gox := &modfile.File{ClassMods: []string{"example.com/foo"}}
	cls, err := ImportClasses(gox, lookupFrom(map[string]module.Version{
		"example.com/foo": modVer,
	}))
	if err != nil {
		t.Fatal("ImportClasses:", err)
	}
	c, ok := cls.LookupClass("_foo.gox")
	if !ok || c.Class != "App" {
		t.Fatal("LookupClass _foo.gox:", c, ok)
	}
}

// TestImportClassesUnresolvableSkipped checks that a class module that
// lookupModVer cannot resolve is skipped with no error.
func TestImportClassesUnresolvableSkipped(t *testing.T) {
	gox := &modfile.File{ClassMods: []string{"example.com/missing"}}
	cls, err := ImportClasses(gox, lookupFrom(map[string]module.Version{}))
	if err != nil {
		t.Fatal("ImportClasses:", err)
	}
	// Built-ins are still present; the missing module simply contributed
	// nothing.
	if !cls.IsClass("_test.gox") {
		t.Fatal("IsClass _test.gox: not ok?")
	}
}

// TestImportClassesNotClassFileMod checks that a class module without projects
// yields ErrNotClassFileMod.
func TestImportClassesNotClassFileMod(t *testing.T) {
	root := t.TempDir()
	modVer := module.Version{Path: "example.com/bar", Version: "v1.0.0"}
	seedModGox(t, root, modVer, "xgo 1.2\n") // no projects
	setModCache(t, filepath.Join(root, "modcache"))

	gox := &modfile.File{ClassMods: []string{"example.com/bar"}}
	_, err := ImportClasses(gox, lookupFrom(map[string]module.Version{
		"example.com/bar": modVer,
	}))
	if err != ErrNotClassFileMod {
		t.Fatal("ImportClasses:", err)
	}
}

// TestImportClassesLocalDirNotFetched checks that a class module replaced by a
// local directory (no version) is loaded from that directory without a fetch.
func TestImportClassesLocalDirNotFetched(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "gox.mod"), `
xgo 1.2

project _loc.gox App example.com/local
`)
	// A local replacement resolves to the directory path with an empty version.
	modVer := module.Version{Path: dir}
	gox := &modfile.File{ClassMods: []string{"example.com/local"}}
	cls, err := ImportClasses(gox, lookupFrom(map[string]module.Version{
		"example.com/local": modVer,
	}))
	if err != nil {
		t.Fatal("ImportClasses:", err)
	}
	if !cls.IsClass("_loc.gox") {
		t.Fatal("IsClass _loc.gox: not ok?")
	}
}

// seedModGop writes a gop.mod (not gox.mod) into the extracted module directory
// of modVer, plus its .mod download-cache entry, so the legacy fallback is
// exercised and no fetch is attempted.
func seedModGop(t *testing.T, root string, modVer module.Version, gopmodText string) {
	t.Helper()
	enc, err := module.EscapePath(modVer.Path)
	if err != nil {
		t.Fatal(err)
	}
	encVer, err := module.EscapeVersion(modVer.Version)
	if err != nil {
		t.Fatal(err)
	}
	modDir := filepath.Join(root, "modcache", enc+"@"+modVer.Version)
	writeFile(t, filepath.Join(modDir, "gop.mod"), gopmodText)
	dlMod := filepath.Join(root, "modcache", "cache", "download", enc, "@v", encVer+".mod")
	writeFile(t, dlMod, "module "+modVer.Path+"\n\ngo 1.18\n")
}

// TestImportClassesClassModGopFallback checks that a class module whose
// directory has only gop.mod (not gox.mod) is loaded via the legacy fallback.
func TestImportClassesClassModGopFallback(t *testing.T) {
	root := t.TempDir()
	modVer := module.Version{Path: "example.com/legacy", Version: "v1.0.0"}
	seedModGop(t, root, modVer, `
gop 1.1

project _gop.gox App example.com/legacy
`)
	setModCache(t, filepath.Join(root, "modcache"))

	gox := &modfile.File{ClassMods: []string{"example.com/legacy"}}
	cls, err := ImportClasses(gox, lookupFrom(map[string]module.Version{
		"example.com/legacy": modVer,
	}))
	if err != nil {
		t.Fatal("ImportClasses:", err)
	}
	if !cls.IsClass("_gop.gox") {
		t.Fatal("IsClass _gop.gox: not ok?")
	}
}

// TestImportClassesClassModMalformed checks that a parse error in a class
// module's gox.mod stops the import and is returned.
func TestImportClassesClassModMalformed(t *testing.T) {
	root := t.TempDir()
	modVer := module.Version{Path: "example.com/bad", Version: "v1.0.0"}
	seedModGox(t, root, modVer, "xgo 1.2\n\nproject\n") // incomplete project directive
	setModCache(t, filepath.Join(root, "modcache"))

	gox := &modfile.File{ClassMods: []string{"example.com/bad"}}
	_, err := ImportClasses(gox, lookupFrom(map[string]module.Version{
		"example.com/bad": modVer,
	}))
	if err == nil {
		t.Fatal("ImportClasses: expected a parse error for a malformed class module gox.mod")
	}
	if err == ErrNotClassFileMod {
		t.Fatal("ImportClasses: got ErrNotClassFileMod, want a parse error")
	}
}

// -----------------------------------------------------------------------------
// maps, and unknown files.
func TestClassInfo(t *testing.T) {
	gox := &modfile.File{
		Projects: []*Project{
			{
				Ext:         "_yap.gox",
				Class:       "App",
				Works:       []*modfile.Class{{Ext: "_ytest.gox", Class: "Case"}},
				AutoLambdas: map[string]int{"get": 1},
			},
		},
	}
	cls, err := ImportClasses(gox, nil)
	if err != nil {
		t.Fatal("ImportClasses:", err)
	}

	// The project (main) file of its class family.
	autoLambdas, isProj, ok := cls.ClassInfo("main_yap.gox")
	if !ok || !isProj {
		t.Fatal("ClassInfo main_yap.gox:", isProj, ok)
	}
	if autoLambdas["get"] != 1 {
		t.Fatal("ClassInfo autoLambdas:", autoLambdas)
	}

	// A work file: recognized but not a project file.
	if _, isProj, ok := cls.ClassInfo("foo_ytest.gox"); !ok || isProj {
		t.Fatal("ClassInfo foo_ytest.gox:", isProj, ok)
	}

	// Any file with the project extension is a project file when the project's
	// work class has a distinct extension.
	if _, isProj, ok := cls.ClassInfo("foo_yap.gox"); !ok || !isProj {
		t.Fatal("ClassInfo foo_yap.gox:", isProj, ok)
	}

	// An unknown file.
	if autoLambdas, isProj, ok := cls.ClassInfo("foo.go"); ok || isProj || autoLambdas != nil {
		t.Fatal("ClassInfo foo.go:", autoLambdas, isProj, ok)
	}
}

// TestClassInfoSharedExt checks ClassInfo for the built-in _test.gox, whose
// project and work class share the same extension: only "main_test.gox" is the
// project file.
func TestClassInfoSharedExt(t *testing.T) {
	cls, err := ImportClasses(&modfile.File{}, nil)
	if err != nil {
		t.Fatal("ImportClasses:", err)
	}
	if _, isProj, ok := cls.ClassInfo("main_test.gox"); !ok || !isProj {
		t.Fatal("ClassInfo main_test.gox:", isProj, ok)
	}
	if _, isProj, ok := cls.ClassInfo("foo_test.gox"); !ok || isProj {
		t.Fatal("ClassInfo foo_test.gox:", isProj, ok)
	}
}

// TestLookupClassMissing checks LookupClass for an unknown extension.
func TestLookupClassMissing(t *testing.T) {
	cls, err := ImportClasses(&modfile.File{}, nil)
	if err != nil {
		t.Fatal("ImportClasses:", err)
	}
	if c, ok := cls.LookupClass(".unknown"); ok || c != nil {
		t.Fatal("LookupClass .unknown:", c, ok)
	}
}
