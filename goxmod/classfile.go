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
	"github.com/qiniu/x/errors"
	"golang.org/x/mod/module"
)

type Class = modfile.Class
type Project = modfile.Project

var (
	GshProject = &Project{
		Ext:      ".gsh",
		Class:    "App",
		PkgPaths: []string{"github.com/qiniu/x/gsh", "math"},
	}
	TestProject = &Project{
		Ext:      "_test.gox",
		Class:    "App",
		PkgPaths: []string{"github.com/goplus/xgo/test", "testing"},
		Works:    []*modfile.Class{{Ext: "_test.gox", Class: "Case"}},
	}
)

var (
	ErrNotClassFileMod = errors.New("not a classfile module")
)

// -----------------------------------------------------------------------------

// Classes represents a collection of classfiles,
type Classes struct {
	projs map[string]*Project // ext -> project
}

// ClassKind checks a fname is a known classfile or not.
// If it is, then it checks the fname is a project file or not.
//
// Deprecated: use ClassInfo instead.
func (p *Classes) ClassKind(fname string) (isProj, ok bool) {
	_, isProj, ok = p.ClassInfo(fname)
	return
}

// ClassInfo checks a fname is a known classfile or not.
// If it is, it returns the auto-lambda map, whether it is a project file, and true.
// See https://github.com/goplus/xgo/issues/2828 to learn about auto lambda.
func (p *Classes) ClassInfo(fname string) (autoLambdas map[string]int, isProj, ok bool) {
	ext := modfile.ClassExt(fname)
	if c, ok := p.projs[ext]; ok {
		return c.AutoLambdas, c.IsProj(ext, fname), true
	}
	return
}

// IsClass checks ext is a known classfile or not.
func (p *Classes) IsClass(ext string) (ok bool) {
	_, ok = p.projs[ext]
	return
}

// LookupClass lookups a classfile by ext.
func (p *Classes) LookupClass(ext string) (c *Project, ok bool) {
	c, ok = p.projs[ext]
	return
}

// -----------------------------------------------------------------------------

// LookupModVer defines a function type for looking up a module by its path. It
// returns the module version and a boolean indicating if the module was found.
type LookupModVer func(modPath string) (modVer module.Version, ok bool)

// ImportClasses imports all classfiles found in this module (from go.mod/gox.mod).
func ImportClasses(goxmod *modfile.File, lookupModVer LookupModVer) (p *Classes, err error) {
	p = &Classes{projs: make(map[string]*Project)}
	p.importClass(TestProject)
	p.importClass(GshProject)
	for _, c := range goxmod.Projects {
		p.importClass(c)
	}
	for _, classMod := range goxmod.ClassMods {
		if modVer, ok := lookupModVer(classMod); ok {
			if err = p.importClassFrom(modVer); err != nil {
				return
			}
		}
	}
	return
}

func (p *Classes) importClassFrom(modVer module.Version) (err error) {
	dir, err := modcache.Path(modVer)
	if err != nil {
		return
	}
	if modVer.Version != "" { // not a local module
		gomod := filepath.Join(dir, "go.mod")
		if _, err = os.Lstat(gomod); err != nil { // not in cache
			if !os.IsNotExist(err) {
				return
			}
			modVer, err = modfetch.Get(modVer.String()) // fetch it
			if err != nil {
				return
			}
		}
	}
	mod, err := LoadModfile(nil, filepath.Join(dir, "gox.mod"))
	if err != nil {
		return
	}
	projs := mod.Projects
	if len(projs) == 0 {
		return ErrNotClassFileMod
	}
	for _, c := range projs {
		p.importClass(c)
	}
	return
}

func (p *Classes) importClass(c *Project) {
	p.projs[c.Ext] = c
	for _, w := range c.Works {
		p.projs[w.Ext] = c
	}
}

// -----------------------------------------------------------------------------
