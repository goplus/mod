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
	"path/filepath"
	"strings"

	"github.com/goplus/mod/goxmod"
)

// Classes represents a collection of classfiles. It is a type alias for
// [goxmod.Classes], so callers get the full goxmod.Classes API without importing
// goxmod just to name the type.
type Classes = goxmod.Classes

// ErrDirNotInWorkspace is returned (wrapped, with the directory) by
// [Workspace.Classes] when dir does not belong to any main module of the
// workspace. Detect it with errors.Is(err, ErrDirNotInWorkspace).
var ErrDirNotInWorkspace = errors.New("directory is not in the workspace")

// Classes returns the collection of classfiles for the main module that
// contains dir, the directory of the package being compiled.
//
// Whether a file such as main.gsh, foo_test.gox or Sprite1.spx is a classfile
// is not decided by the file itself. It depends on the built-in projects (.gsh
// and _test.gox), the projects declared in the owning module's gox.mod (falling
// back to gop.mod), and the projects declared by dependency modules that the
// module marks with //xgo:class (or the legacy //gop:class) on a require line
// in go.mod. Classes resolves all of this into a single, read-only lookup
// table. See [Classes] (the type) for the questions it answers.
//
// Which main module owns dir follows how the go command assigns directories to
// modules. dir is made absolute (against the process working directory) and
// cleaned first, and need not exist on disk: only its path is used, so a tool
// can ask about a directory it is about to create. Among the main modules whose
// root contains dir (containment is checked on path components, so /work/app2
// is not inside /work/app), the one with the longest root wins. If a go.mod
// that is not part of the workspace sits strictly below that root on the way
// down to dir, dir belongs to that foreign module instead, not to the
// workspace. When no main module owns dir, Classes returns an error wrapping
// [ErrDirNotInWorkspace] that includes dir.
//
// The result is cached per main module, not per directory: all directories of
// one module share the same *Classes pointer and the same error. The work for a
// module happens on the first call that reaches it, guarded by that module's
// own sync.Once, so different modules build independently and in parallel and
// Classes is safe for concurrent use. A build failure is cached too, and is
// returned for every later call into that module; it does not affect the other
// modules. The ErrDirNotInWorkspace result is not cached, because it depends
// only on the path. A long-lived process that wants to retry a build after
// fixing the environment should create a new Workspace.
//
// A class module that is not yet in the module cache is fetched during the
// first Classes call that reaches its module. Only the build result is cached;
// the ownership resolution runs on every call and touches the filesystem (it
// probes for a foreign go.mod on the way down to dir), so callers on
// latency-sensitive paths (such as an editor keystroke handler) should resolve
// dir to a *Classes once at startup and reuse it rather than call per request.
//
// In a workspace with several main modules (a go.work with several use
// entries), Classes answers for the module that owns dir and for that module
// only. A class decides how the user's own code is interpreted, and that code
// belongs to the module the package being compiled lives in; the classfiles of
// sibling main modules are implementation details of those modules and are not
// merged in.
//
// The returned *Classes is a lookup table and must be treated as read-only.
func (w *Workspace) Classes(dir string) (*Classes, error) {
	i, err := w.mainModIndex(dir)
	if err != nil {
		return nil, err // not cached: it depends only on dir
	}
	m := w.mainMods[i]
	m.onceCls.Do(func() {
		m.classes, m.clsErr = w.buildClasses(m)
	})
	return m.classes, m.clsErr
}

// mainModIndex returns the index into w.mainMods of the main module that owns
// dir, or -1 and an error wrapping ErrDirNotInWorkspace if no main module does.
// See Classes for the ownership rules it implements.
func (w *Workspace) mainModIndex(dir string) (int, error) {
	// A default workspace has no real module; like the old Classes(), it answers
	// for any directory (with the built-ins only). There is exactly one entry.
	if w.IsDefault() {
		return 0, nil
	}

	abs, err := filepath.Abs(dir)
	if err != nil {
		return -1, err
	}

	// Find the nearest containing main module (longest root wins).
	best := -1
	for i, m := range w.mainMods {
		if dirContains(m.dir, abs) {
			if best < 0 || len(m.dir) > len(w.mainMods[best].dir) {
				best = i
			}
		}
	}
	if best < 0 {
		return -1, fmt.Errorf("%w: %s", ErrDirNotInWorkspace, abs)
	}

	// Reject dir if a go.mod of a module not in the workspace sits strictly
	// below the chosen module root on the way down to dir: that go.mod is the
	// root of a different module, so dir is not covered. The module root itself
	// needs no probe, matching how the go command assigns directories.
	root := w.mainMods[best].dir
	for d := abs; !samePath(d, root); {
		if (Finder{}).isFile(filepath.Join(d, gomodFile)) {
			return -1, fmt.Errorf("%w: %s", ErrDirNotInWorkspace, abs)
		}
		parent := filepath.Dir(d)
		if parent == d { // defensive: should not happen, dir is below root
			break
		}
		d = parent
	}
	return best, nil
}

// dirContains reports whether dir is root or lies below it, comparing on path
// components rather than as strings (so /work/app2 is not inside /work/app). It
// assumes both paths are already absolute and cleaned.
func dirContains(root, dir string) bool {
	if samePath(root, dir) {
		return true
	}
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// buildClasses loads the gox.mod (or, when absent, gop.mod) of mod, enriched
// with its go.mod, and imports its classfiles, resolving class modules through
// LookupModule.
//
// A default workspace has no module, so by design it has no gox.mod to read
// (a gox.mod only exists in the directory of its go.mod): it returns the
// built-in classfiles only, from goxmod.DefaultClasses.
func (w *Workspace) buildClasses(mod *mainMod) (_ *Classes, err error) {
	if mod.path == "" {
		return goxmod.DefaultClasses(), nil
	}
	gox, err := goxmod.LoadModfile(mod.file, filepath.Join(mod.dir, "gox.mod"))
	if err != nil {
		return
	}
	return goxmod.ImportClasses(gox, w.LookupModule)
}

// -----------------------------------------------------------------------------
