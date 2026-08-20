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
	"crypto/sha256"
	"encoding/hex"
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

// IsLocal reports whether the module is main or has a local replacement.
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
	Target        ResolvedModule
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

func validateSource(ref ModuleRef, label string) error {
	if err := validateSourceSyntax(ref, label); err != nil {
		return err
	}
	canonDir, err := canonicalSourcePath(ref.Dir, label+".Dir", true)
	if err != nil {
		return err
	}
	canonGoMod, err := canonicalSourcePath(ref.GoMod, label+".GoMod", false)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(canonDir, canonGoMod)
	if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
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

func validateResolvedModule(m ResolvedModule) error {
	if err := validateResolvedModuleSyntax(m); err != nil {
		return err
	}
	if m.Replace == nil {
		return validateSource(m.Selected, "selected")
	}
	if filepath.IsAbs(m.Replace.Path) {
		_, err := canonicalSourcePath(m.Replace.Path, "replacement.Path", true)
		if err != nil {
			return err
		}
	}
	return validateSource(*m.Replace, "replacement")
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
	if filepath.IsAbs(m.Replace.Path) && !isAbsoluteCleanPath(m.Replace.Path) {
		return fmt.Errorf("replacement.Path must be an absolute clean path: %q", m.Replace.Path)
	}
	if err := validateVersion(m.Replace.Path, m.Replace.Version); err != nil {
		return fmt.Errorf("replacement: %w", err)
	}
	return validateSourceSyntax(*m.Replace, "replacement")
}

func validateFileIdentity(identity FileIdentity) ([]byte, error) {
	if identity.Path == "" || identity.SHA256 == "" {
		return nil, fmt.Errorf("target modfile identity requires path and SHA-256")
	}
	if len(identity.SHA256) != sha256.Size*2 {
		return nil, fmt.Errorf("target modfile SHA-256 must be %d hex characters", sha256.Size*2)
	}
	if _, err := hex.DecodeString(identity.SHA256); err != nil {
		return nil, fmt.Errorf("invalid target modfile SHA-256: %w", err)
	}
	if identity.SHA256 != strings.ToLower(identity.SHA256) {
		return nil, fmt.Errorf("target modfile SHA-256 must use lowercase hexadecimal")
	}
	path, err := canonicalSourcePath(identity.Path, "target modfile path", false)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read target modfile: %w", err)
	}
	got := sha256Hex(data)
	if got != identity.SHA256 {
		return nil, fmt.Errorf("target modfile SHA-256 mismatch for %s", identity.Path)
	}
	return data, nil
}

func fileSHA256(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return sha256Hex(b), nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

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
	seenModules := make(map[string]struct{}, len(g.ClassModules))
	for i, mod := range g.ClassModules {
		path := mod.Selected.Path
		if path == g.Target.Selected.Path {
			return fmt.Errorf("target module %q is repeated in ClassModules", path)
		}
		if _, ok := seenModules[path]; ok {
			return fmt.Errorf("duplicate resolved class module %q", path)
		}
		seenModules[path] = struct{}{}
		if err := validateResolvedModule(mod); err != nil {
			return fmt.Errorf("class module %q: %w", path, err)
		}
		if path != markerPaths[i] {
			return fmt.Errorf("class module %d has logical path %q, want marker %q", i, path, markerPaths[i])
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
