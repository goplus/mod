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

	"github.com/goplus/mod/modload"
	gomodfile "golang.org/x/mod/modfile"
)

func (g ResolvedClassGraph) validate() error {
	if err := validateResolvedModule(g.Target); err != nil {
		return fmt.Errorf("target: %w", err)
	}
	targetModData, err := validateFileIdentity(g.TargetModFile)
	if err != nil {
		return err
	}
	markerPaths, err := classModulePaths(g.TargetModFile.Path, targetModData)
	if err != nil {
		return fmt.Errorf("parse target modfile: %w", err)
	}
	seenMarkers := make(map[string]struct{}, len(markerPaths))
	for _, path := range markerPaths {
		if path == g.Target.Selected.Path {
			return fmt.Errorf("target module %q is also marked as a class module", path)
		}
		if _, ok := seenMarkers[path]; ok {
			return fmt.Errorf("duplicate class module marker %q", path)
		}
		seenMarkers[path] = struct{}{}
	}
	if len(g.ClassModules) != len(markerPaths) {
		return fmt.Errorf("resolved class module count %d does not match target modfile marker count %d", len(g.ClassModules), len(markerPaths))
	}
	for i, mod := range g.ClassModules {
		path := mod.Selected.Path
		if path != markerPaths[i] {
			return fmt.Errorf("class module %d has logical path %q, want marker %q", i, path, markerPaths[i])
		}
		if err := validateResolvedModule(mod); err != nil {
			return fmt.Errorf("class module %q: %w", path, err)
		}
	}
	return nil
}

func classModulePaths(path string, data []byte) ([]string, error) {
	f, err := gomodfile.Parse(path, data, nil)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0)
	for _, require := range f.Require {
		if require.Syntax != nil && modload.HasClassMarker(require.Syntax.Suffix) {
			paths = append(paths, require.Mod.Path)
		}
	}
	return paths, nil
}
