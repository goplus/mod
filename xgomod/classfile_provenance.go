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

	"github.com/goplus/mod/modload"
)

// LookupClassInfo returns class metadata and provenance; built-ins have none.
func (p *Module) LookupClassInfo(ext string) (*ProjectInfo, bool) {
	if info, ok := p.infos[ext]; ok {
		return info, true
	}
	if project, ok := p.projs[ext]; ok {
		// Preserve legacy lookups without fabricating provenance.
		return &ProjectInfo{Project: project}, true
	}
	return nil, false
}

// ImportClassesResolved imports class metadata from a validated graph.
func (p *Module) ImportClassesResolved(graph ResolvedClassGraph) error {
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
	if !moduleSourceMatches(graph.Target, p.Path()) {
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
	if receiverIdentity.SHA256 != graph.TargetModFile.SHA256 {
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
	register := func(info *ProjectInfo) error {
		return registerProject(projects, infos, info)
	}
	// Built-ins have no module provenance.
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
	if !pathWithin(targetRoot, path) {
		return "", "", fmt.Errorf("receiver target gox.mod is outside graph target source")
	}
	_, digest, err = readFileSHA256(path)
	if err != nil {
		return "", "", fmt.Errorf("read receiver target gox.mod: %w", err)
	}
	if digest != identity.SHA256 {
		return "", "", fmt.Errorf("receiver target gox.mod contents changed after load")
	}
	return path, digest, nil
}

func registerProject(projects map[string]*Project, infos map[string]*ProjectInfo, info *ProjectInfo) error {
	if info == nil || info.Project == nil {
		return fmt.Errorf("class metadata contains a nil project")
	}
	if info.Project.Driver != nil && info.Origin == nil {
		return fmt.Errorf("driver-backed project %q has no module provenance", info.Project.Ext)
	}
	for _, ext := range projectExts(info.Project) {
		if old, ok := infos[ext]; ok && old != info {
			if old.Project == info.Project {
				continue
			}
			if old.Project.Driver != nil || info.Project.Driver != nil {
				return fmt.Errorf("driver-backed class extension collision for %q between %q and %q", ext, old.Project.Class, info.Project.Class)
			}
			// Preserve legacy last-wins precedence for non-driver projects.
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

func importResolvedModule(ref ResolvedModule) ([]*ProjectInfo, error) {
	effective := ref.Effective()
	m, err := modload.LoadFrom(effective.GoMod, filepath.Join(effective.Dir, "gox.mod"))
	if err != nil {
		return nil, err
	}
	if loadedPath := m.Path(); !moduleSourceMatches(ref, loadedPath) {
		return nil, fmt.Errorf("module source declares %q, graph selects %q", loadedPath, ref.Selected.Path)
	}
	projects := m.Projects()
	if len(projects) == 0 {
		return nil, ErrNotClassFileMod
	}
	infos := make([]*ProjectInfo, 0, len(projects))
	required := ""
	if m.Opt != nil && m.Opt.XGo != nil {
		required = m.Opt.XGo.Version
	}
	origin := cloneResolvedModule(ref)
	declaration := m.GoxModIdentity()
	if declaration.Path == "" || declaration.SHA256 == "" {
		return nil, fmt.Errorf("module %q has projects without a declaring metadata snapshot", ref.Selected.Path)
	}
	for _, project := range projects {
		infos = append(infos, &ProjectInfo{Project: project, Origin: origin, Declaration: declaration, RequiredXGo: required})
	}
	return infos, nil
}

func cloneResolvedModule(m ResolvedModule) *ResolvedModule {
	clone := m
	if m.Replace != nil {
		replacement := *m.Replace
		clone.Replace = &replacement
	}
	return &clone
}

func moduleSourceMatches(ref ResolvedModule, declaredPath string) bool {
	if declaredPath == "" {
		return false
	}
	if ref.Replace != nil && ref.Replace.Version == "" {
		return true
	}
	return declaredPath == ref.Selected.Path || declaredPath == ref.Effective().Path
}
