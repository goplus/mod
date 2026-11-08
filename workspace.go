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
	"sync"

	"github.com/goplus/mod/goxmod"
	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
)

// mainMod is one main module of the workspace: a local directory with a
// parsed go.mod. It is unexported; callers see main modules only through
// Modules and LookupModule, as entries of the unified module list.
type mainMod struct {
	dir  string        // absolute directory of the module
	path string        // module path, from the "module" directive
	file *modfile.File // parsed go.mod (strictly, see loadMainMod), not nil
}

// goVersion returns the version of the "go" directive, or "" if there is none.
func (m *mainMod) goVersion() string {
	if m.file.Go != nil {
		return m.file.Go.Version
	}
	return ""
}

// Workspace is a snapshot of a go.work (or a single go.mod) together with the
// main modules it lists. It is read-only: nothing in this package modifies the
// files. A Workspace serves both a real workspace and a plain single-module
// project; a caller never has to branch on "is there a go.work?".
//
// If go.work, a go.mod, or the module cache changes, load the workspace again.
type Workspace struct {
	dir  string            // directory containing go.work, or the module's directory
	file *modfile.WorkFile // parsed go.work, nil for a go.mod-only workspace

	mainMods []*mainMod // main modules, in the order of the "use" directives

	// Lazily built, cached module list (see Modules). Guarded by once so that
	// concurrent lookups are safe.
	once    sync.Once
	modules []Module
	modErr  error

	// Cached answers of the standard-library check, keyed by the first path
	// element (see PkgType). Guarded by stdMu.
	stdMu    sync.RWMutex
	stdCache map[string]bool
	goroot   string
}

// -----------------------------------------------------------------------------
// Construction

// NewGoWork creates a workspace from a parsed go.work file. dir is the
// directory that holds the file; the "use" and "replace" directories in the
// file are relative to it. dir is made absolute first.
//
// NewGoWork reads the go.mod of every "use"d directory from disk, because the
// lookup needs the main modules' requirements, and returns an error if one
// cannot be read or parsed. It does not read the go.work file again and does
// not modify f; the workspace keeps f, so the caller must not modify it
// afterwards.
//
// By convention Workspace.Path is set to dir/go.work; LoadGoWork sets it to the
// file that was actually read, which matters when GOWORK names a file with
// another name.
func NewGoWork(dir string, f *modfile.WorkFile) (*Workspace, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	w := &Workspace{dir: abs, file: f}
	for _, u := range f.Use {
		moddir := filepath.FromSlash(u.Path)
		if !filepath.IsAbs(moddir) {
			moddir = filepath.Join(abs, moddir)
		}
		m, err := loadMainMod(moddir)
		if err != nil {
			return nil, fmt.Errorf("%s: use %s: %w", filepath.Join(abs, goworkFile), u.Path, err)
		}
		w.mainMods = append(w.mainMods, m)
	}
	return w, nil
}

// NewGoMod creates a workspace with one main module, from a parsed go.mod
// file, and no go.work. dir is the directory that holds the file, which is the
// module's directory; it is made absolute first. The module directive is
// required.
//
// NewGoMod does not read any file, and does not modify f; the workspace keeps
// f, so the caller must not modify it afterwards. Workspace.Path and
// Workspace.File are empty ("" and nil); Workspace.Dir is the module's
// directory.
func NewGoMod(dir string, f *modfile.File) (*Workspace, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if f.Module == nil {
		return nil, fmt.Errorf("%s: no module declaration", filepath.Join(abs, gomodFile))
	}
	w := &Workspace{dir: abs}
	w.mainMods = []*mainMod{{dir: abs, path: f.Module.Mod.Path, file: f}}
	return w, nil
}

// LoadGoWork reads and parses the go.work file at gowork and calls NewGoWork
// with its directory. Relative "use" paths are resolved against the directory
// of go.work and may point outside of it.
func LoadGoWork(gowork string) (*Workspace, error) {
	data, err := os.ReadFile(gowork)
	if err != nil {
		return nil, err
	}
	f, err := modfile.ParseWork(gowork, data, nil)
	if err != nil {
		return nil, err
	}
	return NewGoWork(filepath.Dir(gowork), f)
}

// LoadGoMod reads and parses the go.mod file at gomod and calls NewGoMod with
// its directory. The file is parsed strictly (modfile.Parse), as the go
// command parses a main module's go.mod.
func LoadGoMod(gomod string) (*Workspace, error) {
	data, err := os.ReadFile(gomod)
	if err != nil {
		return nil, err
	}
	f, err := modfile.Parse(gomod, data, nil)
	if err != nil {
		return nil, err
	}
	return NewGoMod(filepath.Dir(gomod), f)
}

// LoadFromDir finds the workspace that contains dir, the way the go command
// does, and loads it: the go.work file found by searching dir and its parents,
// otherwise the go.mod file found the same way. dir may be relative; it is made
// absolute first.
//
// The search honors GOWORK: GOWORK=off skips go.work, and GOWORK=<path> uses
// that go.work and does no search (see Discover). If neither a go.work nor a
// go.mod is found, LoadFromDir returns ErrNoModule.
func LoadFromDir(dir string) (*Workspace, error) {
	gowork, gomod, err := Discover(dir)
	if err != nil {
		return nil, err
	}
	if gowork != "" {
		return LoadGoWork(gowork)
	}
	return LoadGoMod(gomod)
}

// loadMainMod reads the go.mod in dir. Like the go command does for a main
// module, the file is parsed strictly (modfile.Parse): unknown directives are
// an error, and directives that only apply to main modules, such as replace,
// are kept. The module directive is required.
func loadMainMod(dir string) (*mainMod, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	gomod := filepath.Join(dir, gomodFile)
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
	return &mainMod{dir: dir, path: file.Module.Mod.Path, file: file}, nil
}

// -----------------------------------------------------------------------------
// Validation

// Validate checks the workspace for the problems the go command rejects:
//
//   - there are no main modules at all;
//   - a directory, or a module path, is listed more than once;
//   - a module requires a newer Go version than go.work declares;
//   - the replace directives conflict (see Replacements).
//
// For a go.mod-only workspace the checks that are about go.work (duplicate
// "use" directories and the go.work "go" version) do not apply; the module is
// still checked, and Replacements reports a conflict only if that one go.mod
// conflicts with itself.
//
// All problems found are reported together (see errors.Join).
func (w *Workspace) Validate() error {
	var errs []error
	if len(w.mainMods) == 0 {
		errs = append(errs, errors.New("no modules were found in the current workspace; see 'go help work'"))
	}

	byDir := make(map[string]*mainMod)
	byPath := make(map[string]*mainMod)
	for _, m := range w.mainMods {
		key := pathKey(m.dir)
		if prev := byDir[key]; prev != nil {
			errs = append(errs, fmt.Errorf("directory %s is listed more than once in go.work", m.dir))
			continue
		}
		byDir[key] = m
		if prev := byPath[m.path]; prev != nil {
			errs = append(errs, fmt.Errorf("module %s appears multiple times in workspace (%s and %s)", m.path, prev.dir, m.dir))
			continue
		}
		byPath[m.path] = m
	}

	if wv := w.workGoVersion(); wv != "" {
		for _, m := range w.mainMods {
			if mv := m.goVersion(); mv != "" && version.Compare("go"+mv, "go"+wv) > 0 {
				errs = append(errs, fmt.Errorf("module %s listed in go.work file requires go >= %s, but go.work lists go %s", m.path, mv, wv))
			}
		}
	}

	if _, err := w.Replacements(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// workGoVersion returns the "go" version of go.work, or "" for a go.mod-only
// workspace (where there is no go.work version to check a module against).
func (w *Workspace) workGoVersion() string {
	if w.file != nil && w.file.Go != nil {
		return w.file.Go.Version
	}
	return ""
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
// *ReplaceConflictError. A go.mod-only workspace has no go.work, so only that
// module's own replace directives apply.
//
// Replacements that point to a directory are returned as absolute paths,
// resolved against the directory of the file that declares them.
func (w *Workspace) Replacements() (map[module.Version]module.Version, error) {
	repl := make(map[module.Version]module.Version)
	byWork := make(map[string]bool)
	if w.file != nil {
		for _, r := range w.file.Replace {
			byWork[r.Old.Path] = true
			repl[r.Old] = absReplace(w.dir, r.New)
		}
	}
	for _, m := range w.mainMods {
		declared := make(map[module.Version]bool) // replaces of this go.mod, so far
		for _, r := range m.file.Replace {
			if byWork[r.Old.Path] {
				continue
			}
			nv := absReplace(m.dir, r.New)
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

// GoVersion returns the version of the "go" directive of go.work, or "" if
// there is none. For a go.mod-only workspace it returns the version of the
// main module's "go" directive.
func (w *Workspace) GoVersion() string {
	if w.file != nil {
		if w.file.Go != nil {
			return w.file.Go.Version
		}
	} else {
		return w.mainMods[0].goVersion()
	}
	return ""
}

// -----------------------------------------------------------------------------

// Classes represents a collection of classfiles,
type Classes = goxmod.Classes

// Classes returns the collection of classfiles for the main module.
func (w *Workspace) Classes() (p *Classes, err error) {
	mainMod := w.mainMods[0]
	gox, err := goxmod.LoadModfile(mainMod.file, filepath.Join(mainMod.dir, "gox.mod"))
	if err != nil {
		return
	}
	return goxmod.ImportClasses(gox, w.LookupModule)
}

// -----------------------------------------------------------------------------
