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
	"go/version"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
)

// Module is one main module: a directory with a go.mod file.
type Module struct {
	Dir   string        // absolute directory of the module
	GoMod string        // absolute path of its go.mod
	Path  string        // module path, from the "module" directive
	File  *modfile.File // parsed go.mod (strictly, see LoadModule)
}

// GoVersion returns the version of the "go" directive, or "" if there is none.
func (m *Module) GoVersion() string {
	if m.File != nil && m.File.Go != nil {
		return m.File.Go.Version
	}
	return ""
}

// Workspace is a parsed go.work together with the main modules it lists.
// It is read-only: nothing in this package modifies the files.
type Workspace struct {
	Path    string            // absolute path of go.work
	Dir     string            // directory containing go.work
	File    *modfile.WorkFile // parsed go.work
	Modules []*Module         // main modules, in the order of the "use" directives
}

// GoVersion returns the version of the "go" directive of go.work, or "" if
// there is none.
func (w *Workspace) GoVersion() string {
	if w.File != nil && w.File.Go != nil {
		return w.File.Go.Version
	}
	return ""
}

// Load reads and parses the go.work file at gowork, and loads the go.mod of
// every directory listed in its "use" directives. Relative "use" paths are
// resolved against the directory of go.work and may point outside of it.
func Load(gowork string) (*Workspace, error) {
	abs, err := filepath.Abs(gowork)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, err
	}
	return Parse(abs, data)
}

// Parse is like Load, with the content of the go.work file given by data.
// path only names the file: it is used in error messages and as the base for
// relative "use" paths. The go.mod files of the used directories are still
// read from disk.
func Parse(path string, data []byte) (*Workspace, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	file, err := modfile.ParseWork(abs, data, nil)
	if err != nil {
		return nil, err
	}
	w := &Workspace{Path: abs, Dir: filepath.Dir(abs), File: file}
	for _, u := range file.Use {
		dir := filepath.FromSlash(u.Path)
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(w.Dir, dir)
		}
		m, err := LoadModule(dir)
		if err != nil {
			return nil, fmt.Errorf("%s: use %s: %w", abs, u.Path, err)
		}
		w.Modules = append(w.Modules, m)
	}
	return w, nil
}

// LoadModule reads the go.mod in dir. Like the go command does for a main
// module, the file is parsed strictly (modfile.Parse): unknown directives are
// an error, and directives that only apply to main modules, such as replace,
// are kept. The module directive is required.
func LoadModule(dir string) (*Module, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	gomod := filepath.Join(dir, GoModFile)
	data, err := os.ReadFile(gomod)
	if err != nil {
		return nil, err
	}
	file, err := modfile.Parse(gomod, data, nil)
	if err != nil {
		return nil, err
	}
	if file.Module == nil {
		return nil, fmt.Errorf("%s: no module declaration", gomod)
	}
	return &Module{Dir: dir, GoMod: gomod, Path: file.Module.Mod.Path, File: file}, nil
}

// -----------------------------------------------------------------------------
// Queries

// ModuleByPath returns the main module with the given module path, or nil.
func (w *Workspace) ModuleByPath(modPath string) *Module {
	for _, m := range w.Modules {
		if m.Path == modPath {
			return m
		}
	}
	return nil
}

// ModuleOf reports which main module contains dir. It returns nil if dir
// belongs to no main module: either it is outside every main module, or the
// nearest go.mod above it is a module that is not listed in "use" (a nested
// module is not part of the module that encloses it).
//
// dir does not have to exist.
func (w *Workspace) ModuleOf(dir string) *Module {
	m, _ := w.locate(dir)
	return m
}

// locate walks up from dir. It returns the main module that owns dir, or
// otherwise the go.mod of the nearest non-main module (if any).
func (w *Workspace) locate(dir string) (m *Module, otherGoMod string) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, ""
	}
	for {
		for _, x := range w.Modules {
			if samePath(x.Dir, dir) {
				return x, ""
			}
		}
		if file := filepath.Join(dir, GoModFile); isFile(file) {
			return nil, file
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, ""
		}
		dir = parent
	}
}

// ImportPath returns the import path of the package in dir: the module path
// of the main module that owns dir, followed by the directory relative to the
// module root. ok is false if dir belongs to no main module (see ModuleOf).
func (w *Workspace) ImportPath(dir string) (importPath string, ok bool) {
	m := w.ModuleOf(dir)
	if m == nil {
		return "", false
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(m.Dir, abs)
	if err != nil {
		return "", false
	}
	if rel == "." {
		return m.Path, true
	}
	return m.Path + "/" + filepath.ToSlash(rel), true
}

// Resolve maps an import path to the local source of a main module.
//
// The main module whose path is the longest prefix of importPath (at a path
// element boundary) is chosen, and dir is the corresponding directory inside
// it. ok is false when the package has to come from the dependency graph
// instead: no main module matches, importPath is malformed (empty, "." or
// ".." elements), or a nested go.mod between the module root and dir makes the
// package part of another module.
//
// Resolve does not check that dir exists, and it does not know about the
// standard library.
func (w *Workspace) Resolve(importPath string) (m *Module, dir string, ok bool) {
	for _, x := range w.Modules {
		if !hasPathPrefix(importPath, x.Path) {
			continue
		}
		if m == nil || len(x.Path) > len(m.Path) {
			m = x
		}
	}
	if m == nil {
		return nil, "", false
	}
	dir = m.Dir
	if rest := importPath[len(m.Path):]; rest != "" {
		for _, elem := range strings.Split(rest[1:], "/") {
			if elem == "" || elem == "." || elem == ".." {
				return nil, "", false
			}
			dir = filepath.Join(dir, elem)
			if isFile(filepath.Join(dir, GoModFile)) {
				return nil, "", false // belongs to a nested module
			}
		}
	}
	return m, dir, true
}

// hasPathPrefix reports whether path equals prefix or starts with prefix
// followed by a slash.
func hasPathPrefix(path, prefix string) bool {
	if !strings.HasPrefix(path, prefix) {
		return false
	}
	return len(path) == len(prefix) || path[len(prefix)] == '/'
}

// -----------------------------------------------------------------------------
// Validation

// NotInWorkspaceError reports a directory that is inside a module which is not
// listed in the "use" directives of go.work.
type NotInWorkspaceError struct {
	Dir   string // the directory that was checked
	GoMod string // go.mod of the module that contains it
}

func (e *NotInWorkspaceError) Error() string {
	return fmt.Sprintf("directory %s is contained in a module (%s) that is not one of the workspace modules listed in go.work", e.Dir, e.GoMod)
}

// CheckDir verifies that the module containing dir is one of the main
// modules. Like the go command, it reports a *NotInWorkspaceError when dir is
// inside a module that go.work does not list.
//
// A dir outside of any module is not an error here (the go command allows
// some operations there); neither is a dir inside a main module.
func (w *Workspace) CheckDir(dir string) error {
	m, other := w.locate(dir)
	if m != nil || other == "" {
		return nil
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	return &NotInWorkspaceError{Dir: abs, GoMod: other}
}

// Validate checks the workspace for the problems the go command rejects:
//
//   - there are no main modules at all;
//   - a directory, or a module path, is listed more than once;
//   - a module requires a newer Go version than go.work declares;
//   - the replace directives conflict (see Replacements).
//
// All problems found are reported together (see errors.Join).
func (w *Workspace) Validate() error {
	var errs []error
	if len(w.Modules) == 0 {
		errs = append(errs, errors.New("no modules were found in the current workspace; see 'go help work'"))
	}

	byDir := make(map[string]*Module)
	byPath := make(map[string]*Module)
	for _, m := range w.Modules {
		key := pathKey(m.Dir)
		if prev := byDir[key]; prev != nil {
			errs = append(errs, fmt.Errorf("directory %s is listed more than once in go.work", m.Dir))
			continue
		}
		byDir[key] = m
		if prev := byPath[m.Path]; prev != nil {
			errs = append(errs, fmt.Errorf("module %s appears multiple times in workspace (%s and %s)", m.Path, prev.Dir, m.Dir))
			continue
		}
		byPath[m.Path] = m
	}

	if wv := w.GoVersion(); wv != "" {
		for _, m := range w.Modules {
			if mv := m.GoVersion(); mv != "" && version.Compare("go"+mv, "go"+wv) > 0 {
				errs = append(errs, fmt.Errorf("module %s listed in go.work file requires go >= %s, but go.work lists go %s", m.Path, mv, wv))
			}
		}
	}

	if _, err := w.Replacements(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// -----------------------------------------------------------------------------
// Replacements

// ReplaceConflictError reports two main modules whose go.mod files replace the
// same module differently, with no replace in go.work to settle it.
type ReplaceConflictError struct {
	Old  module.Version // the replaced module (version may be empty)
	A, B module.Version // the two different replacements
}

func (e *ReplaceConflictError) Error() string {
	return fmt.Sprintf("conflicting replacements for %v:\n\t%v\n\t%v\nadd a replace for it to go.work to resolve", e.Old, e.A, e.B)
}

// Replacements returns the replace directives in effect in the workspace.
//
// The replaces of go.work take precedence: as soon as go.work replaces a
// module path (with or without a version), the replaces of that module path
// in the go.mod files are ignored. Among the go.mod files, replacing the same
// module the same way is fine, but replacing it differently is reported as a
// *ReplaceConflictError.
//
// Replacements that point to a directory are returned as absolute paths,
// resolved against the directory of the file that declares them.
func (w *Workspace) Replacements() (map[module.Version]module.Version, error) {
	repl := make(map[module.Version]module.Version)
	byWork := make(map[string]bool)
	if w.File != nil {
		for _, r := range w.File.Replace {
			byWork[r.Old.Path] = true
			repl[r.Old] = absReplace(w.Dir, r.New)
		}
	}
	for _, m := range w.Modules {
		if m.File == nil {
			continue
		}
		declared := make(map[module.Version]bool) // replaces of this go.mod, so far
		for _, r := range m.File.Replace {
			if byWork[r.Old.Path] {
				continue
			}
			nv := absReplace(m.Dir, r.New)
			if prev, ok := repl[r.Old]; ok && !declared[r.Old] && prev != nv {
				return nil, &ReplaceConflictError{Old: r.Old, A: prev, B: nv}
			}
			declared[r.Old] = true
			repl[r.Old] = nv
		}
	}
	return repl, nil
}

// absReplace makes a directory replacement absolute against dir.
func absReplace(dir string, v module.Version) module.Version {
	if v.Version == "" && !filepath.IsAbs(v.Path) {
		v.Path = filepath.Join(dir, filepath.FromSlash(v.Path))
	}
	return v
}

// -----------------------------------------------------------------------------

func isFile(name string) bool {
	fi, err := os.Stat(name)
	return err == nil && !fi.IsDir()
}

// -----------------------------------------------------------------------------
