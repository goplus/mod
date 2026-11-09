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

	"github.com/goplus/mod/goxmod"
)

// Classes represents a collection of classfiles. It is a type alias for
// [goxmod.Classes], so callers get the full goxmod.Classes API without importing
// goxmod just to name the type.
type Classes = goxmod.Classes

// Classes returns the collection of classfiles for the main module.
//
// Whether a file such as main.gsh, foo_test.gox or Sprite1.spx is a classfile
// is not decided by the file itself. It depends on the built-in projects (.gsh
// and _test.gox), the projects declared in the main module's gox.mod (falling
// back to gop.mod), and the projects declared by dependency modules that the
// main module marks with //xgo:class (or the legacy //gop:class) on a require
// line in go.mod. Classes resolves all of this into a single, read-only lookup
// table. See [Classes] (the type) for the questions it answers.
//
// The work happens on the first call, guarded by sync.Once, and the result is
// cached: every later call returns the same *Classes and the same error, so
// Classes is safe for concurrent use. A first attempt that fails is cached too;
// a long-lived process that wants to retry after fixing the environment should
// create a new Workspace.
//
// A class module that is not yet in the module cache is fetched during the first
// Classes call. Callers on latency-sensitive paths (such as an editor keystroke
// handler) should call it once at startup rather than per request.
//
// In a workspace with several main modules (a go.work with several use entries),
// Classes deliberately consults the first main module only. A class decides how
// the user's own code is interpreted, and that code always belongs to the first
// main module; the classfiles of sibling main modules are implementation details
// of those modules and are not merged in.
//
// The returned *Classes is a lookup table and must be treated as read-only.
func (w *Workspace) Classes() (*Classes, error) {
	w.onceCls.Do(func() {
		w.classes, w.clsErr = w.buildClasses()
	})
	return w.classes, w.clsErr
}

// buildClasses loads the gox.mod of the first main module (enriched with its
// go.mod) and imports its classfiles, resolving class modules through
// LookupModule.
func (w *Workspace) buildClasses() (_ *Classes, err error) {
	if len(w.mainMods) == 0 {
		return nil, errors.New("no modules were found in the current workspace; see 'go help work'")
	}
	mainMod := w.mainMods[0]
	gox, err := goxmod.LoadModfile(mainMod.file, filepath.Join(mainMod.dir, "gox.mod"))
	if err != nil {
		return
	}
	return goxmod.ImportClasses(gox, w.LookupModule)
}

// -----------------------------------------------------------------------------
