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
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/goplus/mod/modcache"
	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
)

// ErrInvalidPkgPath is returned by LookupFrom for a package path that cannot be
// resolved to a location: an empty path, or a relative path that leaves every
// local module (or whose fromDir is empty).
var ErrInvalidPkgPath = errors.New("invalid package path")

// MissingError reports a package path that is not standard and is in no module
// of the workspace.
type MissingError struct {
	Path string
}

func (e *MissingError) Error() string {
	return fmt.Sprintf("no required module provides package %s", e.Path)
}

// Module is a module of the workspace: a main module or a dependency. Path is
// the module path as it is required (and as import paths are rooted at it);
// Real is the module version that really supplies the code after replace
// directives. If the code lives in a local directory, which is the case for a
// main module and for a module replaced by a local directory, Real.Path is that
// directory as an absolute path and Real.Version is empty.
type Module struct {
	Path string
	Real module.Version
}

// isLocal reports whether the module's code lives in a local directory (a main
// module or a module replaced by a local directory), as opposed to the module
// cache.
func (m Module) isLocal() bool {
	return m.Real.Version == ""
}

// PkgType specifies a package type.
type PkgType int

const (
	PkgtInvalid  PkgType = iota // an invalid package (the zero value; LookupFrom never returns it)
	PkgtStandard                // a standard Go package
	PkgtLocal                   // a package in a local directory: a workspace module or a local replace
	PkgtExtern                  // an external package
)

// String returns a short name of the package type: "invalid", "standard",
// "local" or "extern".
func (t PkgType) String() string {
	switch t {
	case PkgtStandard:
		return "standard"
	case PkgtLocal:
		return "local"
	case PkgtExtern:
		return "extern"
	default:
		return "invalid"
	}
}

// Package is where a package lives (see LookupFrom).
type Package struct {
	Type    PkgType
	Dir     string
	ModDir  string
	ModPath string
	Real    module.Version // only when Type == PkgtExtern
}

// -----------------------------------------------------------------------------
// The module list

// Modules returns all modules of the workspace, sorted by module path in
// descending order (longest prefix first). The main modules are included: from
// the workspace's point of view they are modules in local directories, like any
// dependency replaced by a local directory. If replace directives conflict, it
// returns the *ReplaceConflictError, as Replacements does. The returned slice
// is shared and must not be modified.
func (w *Workspace) Modules() ([]Module, error) {
	w.once.Do(func() {
		w.modules, w.modErr = w.buildModules()
	})
	return w.modules, w.modErr
}

// buildModules derives the module list from the main modules and their go.mod
// files, following the pruned-graph rule of Go 1.17 and later: the
// requirements of the main modules, plus the immediate requirements of every
// module version they list, and nothing deeper.
func (w *Workspace) buildModules() ([]Module, error) {
	repl, err := w.Replacements()
	if err != nil {
		return nil, err
	}

	mainByPath := make(map[string]bool, len(w.mainMods))
	for _, m := range w.mainMods {
		mainByPath[m.path] = true
	}

	// Step 1: roots are the require lines of the main modules. Requirements
	// that name a main module are ignored: a module that is local stays local.
	selected := make(map[string]string) // module path -> highest required version
	var roots []module.Version
	for _, m := range w.mainMods {
		if m.file == nil {
			continue
		}
		for _, r := range m.file.Require {
			if r.Mod.Path == "" || mainByPath[r.Mod.Path] {
				continue
			}
			roots = append(roots, r.Mod)
			raiseVersion(selected, r.Mod)
		}
	}

	// Step 2: one more level. For each distinct root, read the go.mod of that
	// module version and add its require lines. The requirements found here are
	// not followed further.
	seen := make(map[module.Version]bool, len(roots))
	for _, root := range roots {
		if seen[root] {
			continue
		}
		seen[root] = true
		reqs, err := w.readRootRequires(root, repl)
		if err != nil {
			return nil, err
		}
		for _, req := range reqs {
			if req.Path == "" || mainByPath[req.Path] {
				continue
			}
			raiseVersion(selected, req)
		}
	}

	// Steps 3 and 4: for each selected path, the entry is the replacement if
	// there is one, otherwise {path, selected version}.
	mods := make([]Module, 0, len(selected)+len(w.mainMods))
	for path, ver := range selected {
		real := resolveReplace(repl, module.Version{Path: path, Version: ver})
		mods = append(mods, Module{Path: path, Real: real})
	}

	// Step 5: add every main module as an entry whose real location is its own
	// absolute directory, with no version. If a dependency entry for the same
	// path already exists, the main module replaces it.
	byPath := make(map[string]int, len(mods))
	for i, m := range mods {
		byPath[m.Path] = i
	}
	for _, m := range w.mainMods {
		entry := Module{Path: m.path, Real: module.Version{Path: m.dir}}
		if i, ok := byPath[m.path]; ok {
			mods[i] = entry
		} else {
			byPath[m.path] = len(mods)
			mods = append(mods, entry)
		}
	}

	// Step 6: sort by Path in descending order.
	sort.Slice(mods, func(i, j int) bool { return mods[i].Path > mods[j].Path })
	return mods, nil
}

// raiseVersion keeps the highest version seen for a module path, in semantic
// version order.
func raiseVersion(selected map[string]string, mv module.Version) {
	if prev, ok := selected[mv.Path]; !ok || semver.Compare(mv.Version, prev) > 0 {
		selected[mv.Path] = mv.Version
	}
}

// resolveReplace applies the replace directives to a selected module version,
// trying the version-specific form first, then the wildcard form.
func resolveReplace(repl map[module.Version]module.Version, mv module.Version) module.Version {
	if r, ok := repl[mv]; ok {
		return r
	}
	if r, ok := repl[module.Version{Path: mv.Path}]; ok {
		return r
	}
	return mv
}

// readRootRequires reads the go.mod of a root module version and returns its
// require lines. Which file is read depends on the replacement of the root: a
// directory replacement reads <dir>/go.mod; a module replacement reads the .mod
// of the replacement version; otherwise the root's own .mod. The file is parsed
// leniently (modfile.ParseLax), as the go command does for a dependency's
// go.mod. A file that is not present contributes nothing (nil); a file that is
// present but cannot be parsed is an error.
func (w *Workspace) readRootRequires(root module.Version, repl map[module.Version]module.Version) ([]module.Version, error) {
	target := resolveReplace(repl, root)
	var data []byte
	var name string
	var err error
	if target.Version == "" { // directory replacement (or a main module, excluded earlier)
		name = filepath.Join(target.Path, GoModFile)
		data, err = os.ReadFile(name)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, nil
			}
			return nil, err
		}
	} else {
		name = target.Path + "@" + target.Version
		data, err = readModFile(target)
		if err != nil {
			return nil, err
		}
		if data == nil {
			return nil, nil
		}
	}
	f, err := modfile.ParseLax(name, data, nil)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	reqs := make([]module.Version, 0, len(f.Require))
	for _, r := range f.Require {
		reqs = append(reqs, r.Mod)
	}
	return reqs, nil
}

// readModFile reads the .mod file of a module version from the module cache.
// It first tries $GOMODCACHE/cache/download/<escaped path>/@v/<escaped
// version>.mod, then the go.mod inside the module's extracted directory. It
// returns (nil, nil) when neither is present: the module is not downloaded and
// contributes no requirements.
func readModFile(mv module.Version) ([]byte, error) {
	download, err := modcache.GoModPath(mv)
	if err != nil {
		return nil, err
	}
	if data, err := os.ReadFile(download); err == nil {
		return data, nil
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	dir, err := modcache.Path(mv)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(dir, GoModFile))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return data, nil
}

// LookupModule looks up a module by its exact module path. ok is false if there
// is no such module, or if the modules cannot be determined (see Modules).
func (w *Workspace) LookupModule(modPath string) (modVer module.Version, ok bool) {
	mods, err := w.Modules()
	if err != nil {
		return
	}
	i := sort.Search(len(mods), func(i int) bool { return mods[i].Path <= modPath })
	if i < len(mods) && mods[i].Path == modPath {
		return mods[i].Real, true
	}
	return
}

// lookupModule returns the first module whose path is a prefix of pkgPath at an
// element boundary. Because the list is sorted by path in descending order, the
// first match is the longest one.
func lookupModule(mods []Module, pkgPath string) (Module, bool) {
	for _, m := range mods {
		if hasPathPrefix(pkgPath, m.Path) {
			return m, true
		}
	}
	return Module{}, false
}

// -----------------------------------------------------------------------------
// Classification and lookup

// PkgType returns the type of a package from its path. For a path that needs
// the module list while the list cannot be determined, it returns PkgtInvalid.
func (w *Workspace) PkgType(pkgPath string) PkgType {
	if pkgPath == "" {
		return PkgtInvalid
	}
	if pkgPath[0] == '.' {
		return PkgtLocal
	}
	if w.isStandard(pkgPath) {
		return PkgtStandard
	}
	mods, err := w.Modules()
	if err != nil {
		return PkgtInvalid
	}
	if m, ok := lookupModule(mods, pkgPath); ok {
		if m.isLocal() {
			return PkgtLocal
		}
		return PkgtExtern
	}
	if strings.Contains(firstElem(pkgPath), ".") {
		return PkgtExtern
	}
	return PkgtInvalid
}

// isStandard reports whether the first element of pkgPath has no dot and names
// a directory under GOROOT/src: the go command's own test for a standard
// package. Because the answer depends only on the first element, it is both
// decided by and cached per first path element — the stat target must be elem,
// not pkgPath, or a query for a non-existent subpackage would poison the entry
// for its (standard) root.
func (w *Workspace) isStandard(pkgPath string) bool {
	elem := firstElem(pkgPath)
	if strings.Contains(elem, ".") {
		return false
	}
	w.stdMu.RLock()
	ok, found := w.stdCache[elem]
	w.stdMu.RUnlock()
	if found {
		return ok
	}
	fi, err := os.Stat(filepath.Join(goroot, "src", elem))
	ok = err == nil && fi.IsDir()
	w.stdMu.Lock()
	if w.stdCache == nil {
		w.stdCache = make(map[string]bool)
	}
	w.stdCache[elem] = ok
	w.stdMu.Unlock()
	return ok
}

// firstElem returns the first path element of a slash-separated path.
func firstElem(pkgPath string) string {
	if i := strings.Index(pkgPath, "/"); i >= 0 {
		return pkgPath[:i]
	}
	return pkgPath
}

// LookupFrom returns where a package lives. pkgPath is an import path, or a
// relative path ("./x", "../y") that is resolved against fromDir, the directory
// of the code that contains the import. For any other path fromDir is ignored
// and may be empty. It is safe for concurrent use.
//
// On failure it returns nil and an error: ErrInvalidPkgPath for an empty path
// or a relative path that cannot be resolved, a *MissingError for a path that
// is not standard and in no module, or the error from Modules when the module
// list cannot be built.
func (w *Workspace) LookupFrom(pkgPath, fromDir string) (pkg *Package, err error) {
	if pkgPath == "" {
		return nil, ErrInvalidPkgPath
	}
	if pkgPath[0] == '.' {
		return w.lookupRelative(pkgPath, fromDir)
	}
	if w.isStandard(pkgPath) {
		modDir := filepath.Join(goroot, "src")
		return &Package{
			Type:   PkgtStandard,
			ModDir: modDir,
			Dir:    filepath.Join(modDir, filepath.FromSlash(pkgPath)),
		}, nil
	}
	mods, err := w.Modules()
	if err != nil {
		return nil, err
	}
	m, ok := lookupModule(mods, pkgPath)
	if !ok {
		return nil, &MissingError{Path: pkgPath}
	}
	rest := pkgPath[len(m.Path):]
	if m.isLocal() {
		modDir := m.Real.Path
		return &Package{
			Type:    PkgtLocal,
			ModPath: m.Path,
			ModDir:  modDir,
			Dir:     joinPkgDir(modDir, rest),
		}, nil
	}
	modDir, err := modcache.Path(m.Real)
	if err != nil {
		return nil, err
	}
	return &Package{
		Type:    PkgtExtern,
		Real:    m.Real,
		ModPath: m.Path,
		ModDir:  modDir,
		Dir:     joinPkgDir(modDir, rest),
	}, nil
}

// lookupRelative resolves a relative import ("./x", "../y") against fromDir.
// The local module whose directory is the longest prefix of the resulting
// directory owns the package.
func (w *Workspace) lookupRelative(pkgPath, fromDir string) (*Package, error) {
	if fromDir == "" {
		return nil, ErrInvalidPkgPath
	}
	base, err := filepath.Abs(fromDir)
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(base, filepath.FromSlash(pkgPath))
	mods, err := w.Modules()
	if err != nil {
		return nil, err
	}
	var best Module
	var bestLen int
	found := false
	for _, m := range mods {
		if !m.isLocal() {
			continue
		}
		if dirHasPrefix(dir, m.Real.Path) {
			if !found || len(m.Real.Path) > bestLen {
				best, bestLen, found = m, len(m.Real.Path), true
			}
		}
	}
	if !found {
		return nil, ErrInvalidPkgPath
	}
	return &Package{
		Type:    PkgtLocal,
		ModPath: best.Path,
		ModDir:  best.Real.Path,
		Dir:     dir,
	}, nil
}

// joinPkgDir joins the rest of an import path (below the module path, with a
// leading slash or empty) to a module directory, using the OS separators.
func joinPkgDir(modDir, rest string) string {
	if rest == "" {
		return modDir
	}
	return filepath.Join(modDir, filepath.FromSlash(rest))
}

// dirHasPrefix reports whether dir equals prefix or is a subdirectory of it,
// comparing path elements (not raw strings), honoring case folding where the
// platform does.
func dirHasPrefix(dir, prefix string) bool {
	if samePath(dir, prefix) {
		return true
	}
	if !strings.HasSuffix(prefix, string(filepath.Separator)) {
		prefix += string(filepath.Separator)
	}
	return len(dir) > len(prefix) && samePath(dir[:len(prefix)], prefix)
}

// -----------------------------------------------------------------------------

var goroot = runtime.GOROOT()

// -----------------------------------------------------------------------------
