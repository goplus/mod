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
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"golang.org/x/mod/module"
)

func writeModule(t *testing.T, dir, modPath, gox string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	goMod := filepath.Join(dir, "go.mod")
	if err := os.WriteFile(goMod, []byte("module "+modPath+"\n\ngo 1.25\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if gox != "" {
		if err := os.WriteFile(filepath.Join(dir, "gox.mod"), []byte(gox), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return goMod
}

func graphModule(path, version, dir, goMod string, main bool) ResolvedModule {
	canonicalDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		panic(err)
	}
	canonicalGoMod, err := filepath.EvalSymlinks(goMod)
	if err != nil {
		panic(err)
	}
	return ResolvedModule{
		Selected: ModuleRef{Path: path, Version: version, Dir: canonicalDir, GoMod: canonicalGoMod},
		Main:     main,
	}
}

func graphIdentity(t *testing.T, path string) FileIdentity {
	t.Helper()
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(canonical)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(b)
	return FileIdentity{Path: canonical, SHA256: hex.EncodeToString(sum[:])}
}

func makeSymlink(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Symlink(oldname, newname); err != nil {
		if errors.Is(err, os.ErrPermission) || errors.Is(err, errors.ErrUnsupported) || errors.Is(err, syscall.Errno(1314)) {
			t.Skipf("symlink unavailable: %v", err)
		}
		t.Fatal(err)
	}
}

func writeModuleCacheSource(t *testing.T, cacheRoot, modPath, version, gox string) (dir, goMod string) {
	t.Helper()
	escapedPath, err := module.EscapePath(modPath)
	if err != nil {
		t.Fatal(err)
	}
	escapedVersion, err := module.EscapeVersion(version)
	if err != nil {
		t.Fatal(err)
	}
	dir = filepath.Join(cacheRoot, filepath.FromSlash(escapedPath)+"@"+escapedVersion)
	goMod = filepath.Join(cacheRoot, "cache", "download", filepath.FromSlash(escapedPath), "@v", escapedVersion+".mod")
	if err := os.MkdirAll(filepath.Dir(goMod), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(goMod, []byte("module "+modPath+"\n\ngo 1.25\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if gox != "" {
		if err := os.WriteFile(filepath.Join(dir, "gox.mod"), []byte(gox), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return dir, goMod
}

func TestResolvedModuleEqual(t *testing.T) {
	replacement := ModuleRef{Path: "/src/mod", Dir: "/src/mod", GoMod: "/src/mod/go.mod"}
	module := ResolvedModule{
		Selected: ModuleRef{Path: "example.com/mod", Version: "v1.2.3"},
		Replace:  &replacement,
	}
	copy := module
	copy.Replace = &ModuleRef{Path: replacement.Path, Dir: replacement.Dir, GoMod: replacement.GoMod}
	if !module.Equal(copy) {
		t.Fatal("identical resolved modules are not equal")
	}
	copy.Replace.Version = "v1.2.4"
	if module.Equal(copy) {
		t.Fatal("different replacements are equal")
	}
	copy = module
	copy.Replace = nil
	if module.Equal(copy) {
		t.Fatal("replacement and non-replacement modules are equal")
	}
}

func TestPathWithin(t *testing.T) {
	root := t.TempDir()
	tests := []struct {
		name   string
		target string
		want   bool
	}{
		{"root", root, true},
		{"descendant", filepath.Join(root, "nested", "go.mod"), true},
		{"parent", filepath.Dir(root), false},
		{"sibling with common prefix", root + "-other", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := pathWithin(root, test.target); got != test.want {
				t.Fatalf("pathWithin(%q, %q) = %v, want %v", root, test.target, got, test.want)
			}
		})
	}
}
