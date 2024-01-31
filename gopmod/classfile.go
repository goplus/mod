/*
 * Copyright (c) 2021 The GoPlus Authors (goplus.org). All rights reserved.
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

package gopmod

import (
	"errors"
	"syscall"

	"github.com/goplus/mod/modcache"
	"github.com/goplus/mod/modfetch"
	"github.com/goplus/mod/modfile"
	"github.com/goplus/mod/modload"
	"golang.org/x/mod/module"
)

type Class = modfile.Class
type Project = modfile.Project

var (
	SpxProject = &Project{
		Ext:      ".spx",
		Class:    "Game",
		Works:    []*Class{{Ext: ".spx", Class: "Sprite"}},
		PkgPaths: []string{"github.com/goplus/spx", "math"},
	}
	TestProject = &Project{
		Ext:      "_test.gox",
		Class:    "App",
		Works:    []*modfile.Class{{Ext: "_test.gox", Class: "Case"}},
		PkgPaths: []string{"github.com/goplus/gop/test", "testing"},
	}
)

var (
	ErrNotClassFileMod = errors.New("not a classfile module")
)

// -----------------------------------------------------------------------------

// ClassKind checks a fname is a known classfile or not.
// If it is, then it checks the fname is a project file or not.
func (p *Module) ClassKind(fname string) (isProj, ok bool) {
	ext := modfile.ClassExt(fname)
	if c, ok := p.projects[ext]; ok {
		return c.IsProj(ext, fname), true
	}
	return
}

// IsClass checks ext is a known classfile or not.
func (p *Module) IsClass(ext string) (ok bool) {
	_, ok = p.projects[ext]
	return
}

// LookupClass lookups a classfile by ext.
func (p *Module) LookupClass(ext string) (c *Project, ok bool) {
	c, ok = p.projects[ext]
	return
}

// ImportClasses imports all classfiles found in this module (from go.mod/gop.mod).
func (p *Module) ImportClasses(importClass ...func(c *Project)) (err error) {
	var impcls func(c *Project)
	if importClass != nil {
		impcls = importClass[0]
	}
	p.importClass(TestProject, impcls)
	p.importClass(SpxProject, impcls)
	p.projects[".gmx"] = SpxProject // old style
	opt := p.Opt
	for _, c := range opt.Projects {
		p.importClass(c, impcls)
	}
	for _, classMod := range opt.ClassMods {
		if err = p.importMod(classMod, impcls); err != nil {
			return
		}
	}
	return
}

func (p *Module) importMod(modPath string, imcls func(c *Project)) (err error) {
	mod, ok := p.LookupDepMod(modPath)
	if !ok {
		return syscall.ENOENT
	}
	err = p.importClassFrom(mod, imcls)
	if err != syscall.ENOENT {
		return
	}
	mod, err = modfetch.Get(mod.String())
	if err != nil {
		return
	}
	return p.importClassFrom(mod, imcls)
}

func (p *Module) importClassFrom(modVer module.Version, impcls func(c *Project)) (err error) {
	dir, err := modcache.Path(modVer)
	if err != nil {
		return
	}
	mod, err := modload.Load(dir)
	if err != nil {
		return
	}
	projs := mod.Projects()
	if len(projs) == 0 {
		return ErrNotClassFileMod
	}
	for _, c := range projs {
		p.importClass(c, impcls)
	}
	return
}

func (p *Module) importClass(c *Project, impcls func(c *Project)) {
	p.projects[c.Ext] = c
	for _, w := range c.Works {
		p.projects[w.Ext] = c
	}
	if impcls != nil {
		impcls(c)
	}
}

// -----------------------------------------------------------------------------
