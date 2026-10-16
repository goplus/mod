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
	"os"
	"path/filepath"
	"strings"

	"github.com/goplus/mod/modfile"
	"github.com/goplus/mod/modload"
	gomodfile "golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
)

// ModuleRef identifies a logical selection or its effective source.
type ModuleRef struct {
	Path    string
	Version string
	Dir     string
	GoMod   string
}

// ResolvedModule separates selection from replacement source paths.
type ResolvedModule struct {
	Selected ModuleRef
	Replace  *ModuleRef
	Main     bool
}

// Effective returns the source used for files and metadata.
func (m ResolvedModule) Effective() ModuleRef {
	if m.Replace != nil {
		return *m.Replace
	}
	return m.Selected
}

// Equal reports whether two resolved module identities are identical.
func (m ResolvedModule) Equal(other ResolvedModule) bool {
	if m.Main != other.Main || m.Selected != other.Selected {
		return false
	}
	if m.Replace == nil || other.Replace == nil {
		return m.Replace == nil && other.Replace == nil
	}
	return *m.Replace == *other.Replace
}

// IsLocal reports whether the module uses a local, unversioned filesystem source.
func (m ResolvedModule) IsLocal() bool {
	return m.Main || (m.Replace != nil && m.Replace.Version == "")
}

// Validate checks the resolved module identity and its effective source.
func (m ResolvedModule) Validate() error {
	return validateResolvedModule(m)
}

// ValidateSyntax checks identity and path spelling without filesystem access.
func (m ResolvedModule) ValidateSyntax() error {
	return validateResolvedModuleSyntax(m)
}

// ResolvedClassGraph is XGo's resolved graph snapshot; it is not rediscovered.
type ResolvedClassGraph struct {
	Target ResolvedModule
	// ClassModules follows class-marked require order; order controls registration precedence.
	ClassModules  []ResolvedModule
	TargetModFile FileIdentity
}

// FileIdentity binds metadata to the exact bytes parsed by the caller.
type FileIdentity = modload.FileIdentity

// ProjectInfo pairs class metadata with its origin; built-ins omit provenance.
type ProjectInfo struct {
	Project     *modfile.Project
	Origin      *ResolvedModule
	Declaration FileIdentity
	RequiredXGo string
}

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
	} else if err := validateModulePath(m.Replace.Path); err != nil {
		return fmt.Errorf("replacement: %w", err)
	}
	if err := validateVersion(m.Replace.Path, m.Replace.Version); err != nil {
		return fmt.Errorf("replacement: %w", err)
	}
	return validateSourceSyntax(*m.Replace, "replacement")
}

func canonicalPath(path string, wantDir bool) (string, error) {
	if path == "" || !filepath.IsAbs(path) {
		return "", fmt.Errorf("path must be absolute: %q", path)
	}
	path = filepath.Clean(path)
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if wantDir && !info.IsDir() {
		return "", fmt.Errorf("path is not a directory: %s", path)
	}
	if !wantDir && (info.IsDir() || !info.Mode().IsRegular()) {
		return "", fmt.Errorf("path is not a regular file: %s", path)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return "", err
	}
	return filepath.Clean(resolved), nil
}

func validateSourceFiles(ref ModuleRef, label string) error {
	canonDir, err := canonicalSourcePath(ref.Dir, label+".Dir", true)
	if err != nil {
		return err
	}
	canonGoMod, err := canonicalSourcePath(ref.GoMod, label+".GoMod", false)
	if err != nil {
		return err
	}
	if pathWithin(canonDir, canonGoMod) {
		return nil
	}
	if err := validateModuleCacheSplitSource(ref, canonDir, canonGoMod); err != nil {
		return fmt.Errorf("%s.GoMod must be inside %s or be matching Go module-cache metadata: %w", label, canonDir, err)
	}
	return nil
}

func canonicalSourcePath(value, label string, wantDir bool) (string, error) {
	canonical, err := canonicalPath(value, wantDir)
	if err != nil {
		return "", fmt.Errorf("%s: %w", label, err)
	}
	if filepath.Clean(value) != canonical {
		return "", fmt.Errorf("%s must be canonical: %q", label, value)
	}
	return canonical, nil
}

func validateSourceSyntax(ref ModuleRef, label string) error {
	if ref.Dir == "" || ref.GoMod == "" {
		return fmt.Errorf("%s must provide both Dir and GoMod", label)
	}
	for _, item := range []struct {
		field string
		value string
	}{{"Dir", ref.Dir}, {"GoMod", ref.GoMod}} {
		if !isAbsoluteCleanPath(item.value) {
			return fmt.Errorf("%s.%s must be an absolute clean path: %q", label, item.field, item.value)
		}
	}
	return nil
}

func isAbsoluteCleanPath(value string) bool {
	return filepath.IsAbs(value) && filepath.Clean(value) == value && strings.IndexByte(value, 0) < 0
}

func pathWithin(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func validateModuleCacheSplitSource(ref ModuleRef, dir, goMod string) error {
	if ref.Version == "" {
		return fmt.Errorf("module has no version")
	}
	escapedPath, err := module.EscapePath(ref.Path)
	if err != nil {
		return fmt.Errorf("escape module path: %w", err)
	}
	escapedVersion, err := module.EscapeVersion(ref.Version)
	if err != nil {
		return fmt.Errorf("escape module version: %w", err)
	}
	sourceSuffix := filepath.FromSlash(escapedPath) + "@" + escapedVersion
	cacheRoot := dir
	for range strings.Split(sourceSuffix, string(filepath.Separator)) {
		parent := filepath.Dir(cacheRoot)
		if parent == cacheRoot {
			return fmt.Errorf("source directory does not have module-cache layout")
		}
		cacheRoot = parent
	}
	expectedDir := filepath.Join(cacheRoot, sourceSuffix)
	if !sameCanonicalPath(expectedDir, dir, true) {
		return fmt.Errorf("source directory does not match %s@%s module-cache identity", ref.Path, ref.Version)
	}
	expectedGoMod := filepath.Join(cacheRoot, "cache", "download", filepath.FromSlash(escapedPath), "@v", escapedVersion+".mod")
	expectedInfo, err := os.Lstat(expectedGoMod)
	if err != nil || expectedInfo.Mode()&os.ModeSymlink != 0 || !expectedInfo.Mode().IsRegular() {
		return fmt.Errorf("download-cache go.mod is not a regular non-symlink file")
	}
	if !sameCanonicalPath(expectedGoMod, goMod, false) {
		return fmt.Errorf("go.mod does not match %s@%s download-cache identity", ref.Path, ref.Version)
	}
	b, err := os.ReadFile(goMod)
	if err != nil {
		return fmt.Errorf("read download-cache go.mod: %w", err)
	}
	if declared := gomodfile.ModulePath(b); declared != ref.Path {
		return fmt.Errorf("download-cache go.mod declares %q, want %q", declared, ref.Path)
	}
	return nil
}

func sameCanonicalPath(expected, actual string, wantDir bool) bool {
	canonical, err := canonicalPath(expected, wantDir)
	return err == nil && canonical == actual
}
