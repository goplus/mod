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

package xgomod

import (
	"fmt"
	"path/filepath"

	"github.com/goplus/mod/modload"
)

func importResolvedModule(ref ResolvedModule) ([]*ProjectInfo, error) {
	effective := ref.Effective()
	goxmod := filepath.Join(effective.Dir, "gox.mod")
	m, err := modload.LoadFrom(effective.GoMod, goxmod)
	if err != nil {
		return nil, err
	}
	if loadedPath := m.Path(); loadedPath != ref.Selected.Path && loadedPath != effective.Path {
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
	c := m
	if m.Replace != nil {
		r := *m.Replace
		c.Replace = &r
	}
	return &c
}
