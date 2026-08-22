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

	gomodfile "golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
)

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
