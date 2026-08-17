/*
 * Copyright (c) 2021 The XGo Authors (xgo.dev). All rights reserved.
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

package xgomod

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goplus/mod"
	"github.com/goplus/mod/modcache"
	"github.com/goplus/mod/modfetch"
	"github.com/goplus/mod/modfile"
	"github.com/goplus/mod/modload"
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
	ErrNotFound        = mod.ErrNotFound
	ErrNotClassFileMod = errors.New("not a classfile module")
)

// IsNotFound returns a boolean indicating whether the error is known to
// report that a module or package does not exist. It is satisfied by
// ErrNotFound.
func IsNotFound(err error) bool {
	return errors.Err(err) == ErrNotFound
}

// -----------------------------------------------------------------------------

// ClassKind checks a fname is a known classfile or not.
// If it is, then it checks the fname is a project file or not.
//
// Deprecated: use ClassInfo instead.
func (p *Module) ClassKind(fname string) (isProj, ok bool) {
	_, isProj, ok = p.ClassInfo(fname)
	return
}

// ClassInfo checks a fname is a known classfile or not.
// If it is, it returns the auto-lambda map, whether it is a project file, and true.
// See https://github.com/goplus/xgo/issues/2828 to learn about auto lambda.
func (p *Module) ClassInfo(fname string) (autoLambdas map[string]int, isProj, ok bool) {
	ext := modfile.ClassExt(fname)
	if c, ok := p.projs[ext]; ok {
		return c.AutoLambdas, c.IsProj(ext, fname), true
	}
	return
}

// IsClass checks ext is a known classfile or not.
func (p *Module) IsClass(ext string) (ok bool) {
	_, ok = p.projs[ext]
	return
}

// LookupClass lookups a classfile by ext.
func (p *Module) LookupClass(ext string) (c *Project, ok bool) {
	c, ok = p.projs[ext]
	return
}

// LookupClassInfo looks up a classfile and returns its declaring project and
// resolved module provenance. Built-in projects have nil Origin and an empty
// RequiredXGo.
func (p *Module) LookupClassInfo(ext string) (*ProjectInfo, bool) {
	if info, ok := p.infos[ext]; ok {
		return info, true
	}
	if project, ok := p.projs[ext]; ok {
		// Modules loaded through the legacy API predate provenance. Keep the
		// lookup useful without manufacturing an origin that could be mistaken
		// for a resolved graph record.
		return &ProjectInfo{Project: project}, true
	}
	return nil, false
}

// ImportClasses imports all classfiles found in this module (from go.mod/gox.mod).
func (p *Module) ImportClasses(importClass ...func(c *Project)) (err error) {
	var impcls func(c *Project)
	if importClass != nil {
		impcls = importClass[0]
	}
	p.projs = make(map[string]*Project)
	p.infos = make(map[string]*ProjectInfo)
	p.importClass(TestProject, impcls)
	p.importClass(GshProject, impcls)
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

// ImportClassesResolved imports class metadata from an already-resolved
// module/workspace graph. The graph is validated before the receiver is
// changed. In particular, class modules come only from graph.ClassModules; the
// receiver's legacy Opt.ClassMods is never consulted by this method.
func (p *Module) ImportClassesResolved(graph ResolvedClassGraph, importClass ...func(*ProjectInfo)) error {
	if p == nil || p.File == nil || p.Opt == nil {
		return fmt.Errorf("receiver has no target module snapshot")
	}
	if err := graph.validate(); err != nil {
		return err
	}
	receiverIdentity := p.GoModIdentity()
	if receiverIdentity.Path == "" || receiverIdentity.SHA256 == "" {
		return fmt.Errorf("receiver has no target modfile snapshot")
	}
	if p.Path() != graph.Target.Selected.Path && p.Path() != graph.Target.Effective().Path {
		return fmt.Errorf("receiver module %q does not match graph target %q", p.Path(), graph.Target.Selected.Path)
	}
	receiverModfile, err := canonicalPath(receiverIdentity.Path, false)
	if err != nil {
		return fmt.Errorf("receiver target modfile: %w", err)
	}
	graphModfile, err := canonicalPath(graph.TargetModFile.Path, false)
	if err != nil {
		return fmt.Errorf("graph target modfile: %w", err)
	}
	if receiverModfile != graphModfile {
		return fmt.Errorf("receiver and graph target modfile snapshots differ")
	}
	if !strings.EqualFold(receiverIdentity.SHA256, graph.TargetModFile.SHA256) {
		return fmt.Errorf("receiver and graph target modfile contents differ")
	}
	targetRoot, err := canonicalPath(graph.Target.Effective().Dir, true)
	if err != nil {
		return fmt.Errorf("graph target source: %w", err)
	}
	declarationPath, declarationDigest, err := receiverGoxSnapshot(p, targetRoot)
	if err != nil {
		return err
	}
	declaration := FileIdentity{Path: declarationPath, SHA256: declarationDigest}

	projects := make(map[string]*Project)
	infos := make(map[string]*ProjectInfo)
	callbacks := make([]*ProjectInfo, 0)
	register := func(info *ProjectInfo) error {
		if err := registerProject(projects, infos, info); err != nil {
			return err
		}
		if importClass != nil {
			callbacks = append(callbacks, info)
		}
		return nil
	}
	// Built-ins are deliberately provenance-free and cannot declare a runtime.
	for _, builtin := range []*Project{TestProject, GshProject} {
		if err := register(&ProjectInfo{Project: builtin}); err != nil {
			return err
		}
	}

	origin := graph.Target
	required := ""
	if p.Opt.XGo != nil {
		required = p.Opt.XGo.Version
	}
	for _, project := range p.Projects() {
		if err := register(&ProjectInfo{Project: project, Origin: cloneResolvedModule(origin), Declaration: declaration, RequiredXGo: required}); err != nil {
			return err
		}
	}

	for _, record := range graph.ClassModules {
		classMod := record.Selected.Path
		moduleProjects, err := importResolvedModule(record)
		if err != nil {
			return fmt.Errorf("import class module %q: %w", classMod, err)
		}
		for _, info := range moduleProjects {
			if err := register(info); err != nil {
				return err
			}
		}
	}
	p.projs = projects
	p.infos = infos
	if importClass != nil {
		for _, info := range callbacks {
			importClass[0](info)
		}
	}
	return nil
}

func receiverGoxSnapshot(p *Module, targetRoot string) (path, digest string, err error) {
	identity := p.GoxModIdentity()
	if identity.Path == "" && identity.SHA256 == "" {
		for _, candidate := range []string{filepath.Join(targetRoot, "gox.mod"), filepath.Join(targetRoot, "gop.mod")} {
			if _, statErr := os.Stat(candidate); statErr == nil {
				return "", "", fmt.Errorf("receiver target gox.mod appeared without load snapshot")
			} else if !os.IsNotExist(statErr) {
				return "", "", fmt.Errorf("check receiver target gox.mod: %w", statErr)
			}
		}
		if len(p.Projects()) != 0 {
			return "", "", fmt.Errorf("receiver has projects without a target gox.mod snapshot")
		}
		return "", "", nil
	}
	if identity.Path == "" || identity.SHA256 == "" {
		return "", "", fmt.Errorf("receiver target gox.mod snapshot is incomplete")
	}
	path, err = canonicalPath(identity.Path, false)
	if err != nil {
		return "", "", fmt.Errorf("receiver target gox.mod: %w", err)
	}
	rel, err := filepath.Rel(targetRoot, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", "", fmt.Errorf("receiver target gox.mod is outside graph target source")
	}
	digest, err = fileSHA256(path)
	if err != nil {
		return "", "", fmt.Errorf("read receiver target gox.mod: %w", err)
	}
	if !strings.EqualFold(digest, identity.SHA256) {
		return "", "", fmt.Errorf("receiver target gox.mod contents changed after load")
	}
	return path, digest, nil
}

func registerProject(projects map[string]*Project, infos map[string]*ProjectInfo, info *ProjectInfo) error {
	if info == nil || info.Project == nil {
		return fmt.Errorf("class metadata contains a nil project")
	}
	if info.Project.Runtime != nil && info.Origin == nil {
		return fmt.Errorf("runtime project %q has no module provenance", info.Project.Ext)
	}
	for _, ext := range projectExts(info.Project) {
		if old, ok := infos[ext]; ok && old != info {
			if old.Project == info.Project {
				continue
			}
			if old.Project.Runtime != nil || info.Project.Runtime != nil {
				return fmt.Errorf("runtime class extension collision for %q between %q and %q", ext, old.Project.Class, info.Project.Class)
			}
		}
		projects[ext] = info.Project
		infos[ext] = info
	}
	return nil
}

func projectExts(project *Project) []string {
	exts := make([]string, 0, len(project.Works)+1)
	exts = append(exts, project.Ext)
	for _, work := range project.Works {
		exts = append(exts, work.Ext)
	}
	return exts
}

func (p *Module) importMod(modPath string, imcls func(c *Project)) (err error) {
	mod, ok := p.LookupDepMod(modPath)
	if !ok {
		return ErrNotFound
	}
	err = p.importClassFrom(mod, imcls)
	if !IsNotFound(err) {
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
	info := &ProjectInfo{Project: c}
	if p.infos == nil {
		p.infos = make(map[string]*ProjectInfo)
	}
	for _, ext := range projectExts(c) {
		p.projs[ext] = c
		p.infos[ext] = info
	}
	if impcls != nil {
		impcls(c)
	}
}

// -----------------------------------------------------------------------------
