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

	"golang.org/x/mod/module"
)

func validateModulePath(path string) error {
	if path == "" {
		return fmt.Errorf("module path is empty")
	}
	if err := module.CheckPath(path); err != nil {
		return fmt.Errorf("invalid module path %q: %w", path, err)
	}
	return nil
}

func validateVersion(path, version string) error {
	if version == "" {
		return nil
	}
	canonical := module.CanonicalVersion(version)
	if canonical == "" || canonical != version {
		return fmt.Errorf("invalid non-canonical version %q for %s", version, path)
	}
	if err := module.Check(path, version); err != nil {
		return fmt.Errorf("invalid module version %q for %s: %w", version, path, err)
	}
	return nil
}

func validateResolvedModule(m ResolvedModule) error {
	if err := validateResolvedModuleSyntax(m); err != nil {
		return err
	}
	if m.Replace == nil {
		return validateSourceFiles(m.Selected, "selected")
	}
	return validateSourceFiles(*m.Replace, "replacement")
}

func validateResolvedModuleSyntax(m ResolvedModule) error {
	if err := validateModulePath(m.Selected.Path); err != nil {
		return fmt.Errorf("selected: %w", err)
	}
	if err := validateVersion(m.Selected.Path, m.Selected.Version); err != nil {
		return fmt.Errorf("selected: %w", err)
	}
	if m.Main {
		if m.Selected.Version != "" {
			return fmt.Errorf("main module selected version must be empty")
		}
		if m.Replace != nil {
			return fmt.Errorf("main module cannot have a replacement")
		}
	} else if m.Selected.Version == "" {
		return fmt.Errorf("non-main module selected version must not be empty")
	}
	if m.Replace == nil {
		return validateSourceSyntax(m.Selected, "selected")
	}
	if m.Selected.Dir != "" || m.Selected.GoMod != "" {
		return fmt.Errorf("selected Dir/GoMod must be empty when replacement is present")
	}
	if m.Replace.Path == "" {
		return fmt.Errorf("replacement path is empty")
	}
	if m.Replace.Version == "" {
		if !isAbsoluteCleanPath(m.Replace.Path) {
			return fmt.Errorf("local replacement.Path must be an absolute clean path: %q", m.Replace.Path)
		}
		if m.Replace.Dir != m.Replace.Path {
			return fmt.Errorf("local replacement.Path and replacement.Dir must identify the same canonical directory")
		}
	} else if filepath.IsAbs(m.Replace.Path) {
		return fmt.Errorf("versioned replacement.Path must be a module path: %q", m.Replace.Path)
	} else {
		if err := validateModulePath(m.Replace.Path); err != nil {
			return fmt.Errorf("replacement: %w", err)
		}
	}
	if err := validateVersion(m.Replace.Path, m.Replace.Version); err != nil {
		return fmt.Errorf("replacement: %w", err)
	}
	return validateSourceSyntax(*m.Replace, "replacement")
}
