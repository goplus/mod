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
		Ext:      ".gmx",
		Class:    "Game",
		Works:    []*Class{{Ext: ".spx", Class: "Sprite"}},
		PkgPaths: []string{"github.com/goplus/spx", "math"},
	}
)

var (
	ErrNotClassFileMod = errors.New("not a classfile module")
)

// -----------------------------------------------------------------------------

func (p *Module) IsClass(ext string) (isProj bool, ok bool) {
	c, ok := p.projects[ext]
	if ok {
		isProj = (ext == c.Ext)
	}
	return
}

func (p *Module) LookupClass(ext string) (c *Project, ok bool) {
	c, ok = p.projects[ext]
	return
}

func (p *Module) RegisterClasses(registerClass ...func(c *Project)) (err error) {
	var regcls func(c *Project)
	if registerClass != nil {
		regcls = registerClass[0]
	}
	p.registerClass(SpxProject, regcls)
	if c := p.Project; c != nil {
		p.registerClass(c, regcls)
	}
	for _, r := range p.Register {
		if err = p.registerMod(r.ClassfileMod, regcls); err != nil {
			return
		}
	}
	return
}

func (p *Module) registerMod(modPath string, regcls func(c *Project)) (err error) {
	mod, ok := p.LookupDepMod(modPath)
	if !ok {
		return syscall.ENOENT
	}
	err = p.registerClassFrom(mod, regcls)
	if err != syscall.ENOENT {
		return
	}
	mod, err = modfetch.Get(mod.String())
	if err != nil {
		return
	}
	return p.registerClassFrom(mod, regcls)
}

func (p *Module) registerClassFrom(modVer module.Version, regcls func(c *Project)) (err error) {
	dir, err := modcache.Path(modVer)
	if err != nil {
		return
	}
	mod, err := modload.Load(dir, 0)
	if err != nil {
		return
	}
	c := mod.Project
	if c == nil {
		return ErrNotClassFileMod
	}
	p.registerClass(c, regcls)
	return
}

func (p *Module) registerClass(c *Project, regcls func(c *Project)) {
	p.projects[c.Ext] = c
	for _, w := range c.Works {
		p.projects[w.Ext] = c
	}
	if regcls != nil {
		regcls(c)
	}
}

// -----------------------------------------------------------------------------
