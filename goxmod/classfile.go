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

	"github.com/goplus/mod/modcache"
	"github.com/goplus/mod/modfetch"
	"github.com/goplus/mod/modfile"
	"golang.org/x/mod/module"
)

// LookupModVer looks up a module by its path, returning the selected version
// and whether such a module exists. [Workspace.LookupModule] has this shape; it
// already applies replace directives and the selected versions, so a class
// module replaced by a local directory is found there, without a version, and
// is not fetched.
type LookupModVer func(modPath string) (modVer module.Version, ok bool)

// ImportClasses builds a *Classes from the module description returned by
// [LoadModfile]. Projects are registered in this order, and when two projects
// use the same extension the later one replaces the earlier one:
//
//  1. TestProject (built-in).
//  2. GshProject (built-in).
//  3. Every project in goxmod.Projects (declared by the module itself).
//  4. For each module path in goxmod.ClassMods, in order, the projects of that
//     module.
//
// A project is indexed by its own extension and by the extension of every class
// in its Works list, so a lookup by any of those extensions reaches the same
// *Project.
//
// For each class module, lookupModVer(modPath) resolves the path to a version.
// If it reports ok == false, the module is skipped and no error is raised. A
// module with a version whose go.mod is not yet in the module cache is fetched
// first; a module without a version is a local directory and is not fetched.
// The module's gox.mod (or gop.mod) is then loaded with go.mod == nil, because
// only the projects matter. A class module that resolves but declares no
// projects yields [ErrNotClassFileMod].
//
// Any error (module cache lookup, fetch, parse, or ErrNotClassFileMod) stops the
// import and is returned. The *Classes returned together with an error is
// partial and must not be used.
//
// lookupModVer must not be nil when goxmod.ClassMods is not empty. GshProject
// and TestProject are package-level values shared by every Classes, so callers
// must treat them, and every *Project obtained from a Classes, as read-only.
func ImportClasses(goxmod *modfile.File, lookupModVer LookupModVer) (p *Classes, err error) {
	p = DefaultClasses()
	for _, c := range goxmod.Projects {
		p.importClass(c)
	}
	for _, classMod := range goxmod.ClassMods {
		if err = p.importMod(classMod, lookupModVer); err != nil {
			return
		}
	}
	return
}

// DefaultClasses returns the classfiles that are known without any module: the
// built-in projects TestProject ("_test.gox") and GshProject (".gsh"), in that
// registration order. The result is a fresh collection, so it is safe to use
// like any other *Classes.
//
// GshProject and TestProject are package-level values shared by every Classes,
// so callers must treat them, and every *Project obtained from a Classes, as
// read-only.
func DefaultClasses() *Classes {
	p := &Classes{projs: make(map[string]*Project)}
	p.importClass(TestProject)
	p.importClass(GshProject)
	return p
}

// importMod registers the projects of the class module at modPath. A module that
// lookupModVer cannot resolve is skipped. A versioned module whose extracted
// directory is not yet in the module cache is fetched first; a module without a
// version is a local directory and is never fetched.
func (p *Classes) importMod(modPath string, lookupModVer LookupModVer) (err error) {
	modVer, ok := lookupModVer(modPath)
	if !ok {
		return nil // unresolvable class modules are skipped
	}
	if modVer.Version != "" {
		// A versioned module whose extracted directory is not in the cache must
		// be fetched first. The download-cache .mod file alone is not enough,
		// because the gox.mod we need lives in the extracted directory. A module
		// without a version is a local directory and is read in place.
		if dir, e := modcache.Path(modVer); e != nil || !exists(dir) {
			if modVer, err = modfetch.Get(modVer.String()); err != nil {
				return
			}
		}
	}
	return p.importClassFrom(modVer)
}

// exists reports whether name is an existing directory.
func exists(name string) bool {
	_, err := os.Stat(name)
	return err == nil
}

// importClassFrom loads the gox.mod (or gop.mod) of modVer from the module cache
// and registers its projects. A module that declares no projects yields
// ErrNotClassFileMod.
func (p *Classes) importClassFrom(modVer module.Version) (err error) {
	dir, err := modcache.Path(modVer)
	if err != nil {
		return
	}
	// Only the projects matter here, so go.mod is not merged (gomod == nil).
	opt, err := LoadModfile(nil, filepath.Join(dir, "gox.mod"))
	if err != nil {
		return
	}
	if len(opt.Projects) == 0 {
		return ErrNotClassFileMod
	}
	for _, c := range opt.Projects {
		p.importClass(c)
	}
	return
}

// importClass indexes c by its own extension and by every extension in its Works
// list.
func (p *Classes) importClass(c *Project) {
	p.projs[c.Ext] = c
	for _, w := range c.Works {
		p.projs[w.Ext] = c
	}
}

// -----------------------------------------------------------------------------
