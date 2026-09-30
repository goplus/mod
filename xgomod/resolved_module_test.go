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
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolvedModuleEffective(t *testing.T) {
	selected := ModuleRef{Path: "example.com/framework", Version: "v1.2.3"}
	local := ModuleRef{Path: "/tmp/framework", Dir: "/tmp/framework", GoMod: "/tmp/framework/go.mod"}
	resolved := ResolvedModule{Selected: selected, Replace: &local}
	if got := resolved.Effective(); got != local {
		t.Fatalf("Effective = %#v, want %#v", got, local)
	}
	if !resolved.IsLocal() {
		t.Fatal("local replacement is not local")
	}
	resolved.Replace = &ModuleRef{Path: "example.com/fork", Version: "v1.0.0"}
	if resolved.IsLocal() {
		t.Fatal("versioned replacement is local")
	}
	if !((ResolvedModule{Main: true}).IsLocal()) {
		t.Fatal("main module is not local")
	}
}

func TestResolvedModuleValidateSyntaxDoesNotReadFilesystem(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	resolved := ResolvedModule{Selected: ModuleRef{
		Path: "example.com/framework", Version: "v1.2.3",
		Dir: missing, GoMod: filepath.Join(missing, "go.mod"),
	}}
	if err := resolved.ValidateSyntax(); err != nil {
		t.Fatalf("ValidateSyntax consulted ambient filesystem: %v", err)
	}
	if err := resolved.Validate(); err == nil {
		t.Fatal("Validate accepted a missing effective source")
	}
}

func TestResolvedModuleValidateSyntaxRejectsImpossibleGraphStates(t *testing.T) {
	tests := map[string]ResolvedModule{
		"main with version": {
			Selected: ModuleRef{Path: "example.com/framework", Version: "v1.2.3", Dir: "/workspace/framework", GoMod: "/workspace/framework/go.mod"}, Main: true,
		},
		"main with replacement": {
			Selected: ModuleRef{Path: "example.com/framework", Dir: "/workspace/framework", GoMod: "/workspace/framework/go.mod"}, Main: true,
			Replace: &ModuleRef{Path: "/workspace/local", Dir: "/workspace/local", GoMod: "/workspace/local/go.mod"},
		},
		"non-main without version": {
			Selected: ModuleRef{Path: "example.com/framework", Dir: "/workspace/framework", GoMod: "/workspace/framework/go.mod"},
		},
		"local replacement with module path": {
			Selected: ModuleRef{Path: "example.com/framework", Version: "v1.2.3"},
			Replace:  &ModuleRef{Path: "example.com/fork", Dir: "/workspace/fork", GoMod: "/workspace/fork/go.mod"},
		},
		"local replacement path differs from dir": {
			Selected: ModuleRef{Path: "example.com/framework", Version: "v1.2.3"},
			Replace:  &ModuleRef{Path: "/workspace/fork", Dir: "/workspace/other", GoMod: "/workspace/other/go.mod"},
		},
		"versioned replacement with filesystem path": {
			Selected: ModuleRef{Path: "example.com/framework", Version: "v1.2.3"},
			Replace:  &ModuleRef{Path: "/workspace/fork", Version: "v1.4.0", Dir: "/workspace/fork", GoMod: "/workspace/fork/go.mod"},
		},
	}
	for name, resolved := range tests {
		t.Run(name, func(t *testing.T) {
			if err := resolved.ValidateSyntax(); err == nil {
				t.Fatal("ValidateSyntax accepted impossible graph state")
			}
		})
	}
}

func TestResolvedModuleValidateDirectAndReplacements(t *testing.T) {
	directDir := filepath.Join(t.TempDir(), "direct")
	directGoMod := writeModule(t, directDir, "example.com/direct", "")
	direct := graphModule("example.com/direct", "v1.2.3", directDir, directGoMod, false)

	localDir := filepath.Join(t.TempDir(), "local")
	localGoMod := writeModule(t, localDir, "example.com/local", "")
	localDir, err := filepath.EvalSymlinks(localDir)
	if err != nil {
		t.Fatal(err)
	}
	localGoMod, err = filepath.EvalSymlinks(localGoMod)
	if err != nil {
		t.Fatal(err)
	}
	localReplace := ResolvedModule{
		Selected: ModuleRef{Path: "example.com/original", Version: "v1.2.3"},
		Replace:  &ModuleRef{Path: localDir, Dir: localDir, GoMod: localGoMod},
	}

	versionDir := filepath.Join(t.TempDir(), "version")
	versionGoMod := writeModule(t, versionDir, "example.com/fork", "")
	versionReplaceSource := graphModule("example.com/fork", "v1.4.0", versionDir, versionGoMod, false).Selected
	versionReplace := ResolvedModule{
		Selected: ModuleRef{Path: "example.com/original", Version: "v1.2.3"},
		Replace:  &versionReplaceSource,
	}

	for name, resolved := range map[string]ResolvedModule{
		"direct":              direct,
		"local replacement":   localReplace,
		"version replacement": versionReplace,
	} {
		t.Run(name, func(t *testing.T) {
			if err := resolved.Validate(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestResolvedModuleValidateOfficialModuleCacheSplit(t *testing.T) {
	const (
		path    = "example.com/Upper/legacy"
		version = "v1.2.3"
	)
	dir, goMod := writeModuleCacheSource(t, filepath.Join(t.TempDir(), "pkg", "mod"), path, version, "")
	resolved := graphModule(path, version, dir, goMod, false)
	if err := resolved.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestResolvedModuleValidateRejectsFilesystemAndIdentityShapes(t *testing.T) {
	root := t.TempDir()
	goMod := writeModule(t, root, "example.com/app", "")
	canonicalDir, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	canonicalGoMod, err := filepath.EvalSymlinks(goMod)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name     string
		resolved ResolvedModule
		want     string
	}{
		{
			name: "directory used as go.mod",
			resolved: ResolvedModule{Selected: ModuleRef{
				Path: "example.com/app", Version: "v1.0.0", Dir: canonicalDir, GoMod: canonicalDir,
			}},
			want: "not a regular file",
		},
		{
			name: "file used as module directory",
			resolved: ResolvedModule{Selected: ModuleRef{
				Path: "example.com/app", Version: "v1.0.0", Dir: canonicalGoMod, GoMod: canonicalGoMod,
			}},
			want: "not a directory",
		},
		{
			name: "invalid module path",
			resolved: ResolvedModule{Selected: ModuleRef{
				Path: "../app", Version: "v1.0.0", Dir: canonicalDir, GoMod: canonicalGoMod,
			}},
			want: "invalid module path",
		},
		{
			name: "non-canonical version",
			resolved: ResolvedModule{Selected: ModuleRef{
				Path: "example.com/app", Version: "v1", Dir: canonicalDir, GoMod: canonicalGoMod,
			}},
			want: "invalid non-canonical version",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.resolved.Validate()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestResolvedModuleValidateRejectsReplacementAndCanonicalShapes(t *testing.T) {
	root := t.TempDir()
	goMod := writeModule(t, root, "example.com/app", "")
	canonicalDir, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	canonicalGoMod, err := filepath.EvalSymlinks(goMod)
	if err != nil {
		t.Fatal(err)
	}
	validSource := func() ModuleRef {
		return ModuleRef{Path: "example.com/app", Version: "v1.0.0", Dir: canonicalDir, GoMod: canonicalGoMod}
	}
	validSelected := func() ModuleRef {
		return ModuleRef{Path: "example.com/app", Version: "v1.0.0"}
	}
	for _, test := range []struct {
		name     string
		resolved ResolvedModule
		want     string
	}{
		{name: "empty selected path", resolved: ResolvedModule{Selected: ModuleRef{Version: "v1.0.0", Dir: canonicalDir, GoMod: canonicalGoMod}}, want: "module path is empty"},
		{name: "invalid selected major", resolved: ResolvedModule{Selected: ModuleRef{Path: "example.com/app/v2", Version: "v1.0.0", Dir: canonicalDir, GoMod: canonicalGoMod}}, want: "invalid module version"},
		{name: "missing source", resolved: ResolvedModule{Selected: ModuleRef{Path: "example.com/app", Version: "v1.0.0"}}, want: "must provide both"},
		{name: "relative source", resolved: ResolvedModule{Selected: ModuleRef{Path: "example.com/app", Version: "v1.0.0", Dir: "relative", GoMod: canonicalGoMod}}, want: "absolute clean path"},
		{name: "empty replacement path", resolved: ResolvedModule{Selected: validSelected(), Replace: &ModuleRef{Version: "v1.0.0", Dir: canonicalDir, GoMod: canonicalGoMod}}, want: "replacement path is empty"},
		{name: "invalid replacement path", resolved: ResolvedModule{Selected: validSelected(), Replace: &ModuleRef{Path: "bad path", Version: "v1.0.0", Dir: canonicalDir, GoMod: canonicalGoMod}}, want: "invalid module path"},
		{name: "invalid replacement version", resolved: ResolvedModule{Selected: validSelected(), Replace: &ModuleRef{Path: "example.com/fork", Version: "v1", Dir: canonicalDir, GoMod: canonicalGoMod}}, want: "invalid non-canonical version"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.resolved.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want substring %q", err, test.want)
			}
		})
	}

	t.Run("non-canonical directory", func(t *testing.T) {
		alias := filepath.Join(filepath.Dir(canonicalDir), "xgomod-resolved-dir-alias")
		makeSymlink(t, canonicalDir, alias)
		defer os.Remove(alias)
		ref := validSource()
		ref.Dir = alias
		if err := (ResolvedModule{Selected: ref}).Validate(); err == nil || !strings.Contains(err.Error(), "Dir must be canonical") {
			t.Fatalf("non-canonical directory error = %v", err)
		}
	})

	t.Run("non-canonical go.mod", func(t *testing.T) {
		alias := filepath.Join(canonicalDir, "xgomod-resolved-go.mod-alias")
		makeSymlink(t, canonicalGoMod, alias)
		defer os.Remove(alias)
		ref := validSource()
		ref.GoMod = alias
		if err := (ResolvedModule{Selected: ref}).Validate(); err == nil || !strings.Contains(err.Error(), "GoMod must be canonical") {
			t.Fatalf("non-canonical go.mod error = %v", err)
		}
	})

	t.Run("non-canonical local replacement path", func(t *testing.T) {
		alias := filepath.Join(filepath.Dir(canonicalDir), "xgomod-replacement-dir-alias")
		makeSymlink(t, canonicalDir, alias)
		defer os.Remove(alias)
		replacement := &ModuleRef{Path: alias, Dir: alias, GoMod: filepath.Join(alias, "go.mod")}
		resolved := ResolvedModule{Selected: validSelected(), Replace: replacement}
		if err := resolved.Validate(); err == nil || !strings.Contains(err.Error(), "replacement.Dir must be canonical") {
			t.Fatalf("non-canonical replacement path error = %v", err)
		}
	})
}

func TestValidateFileIdentityRejectsMalformedShapes(t *testing.T) {
	root := t.TempDir()
	goMod := writeModule(t, root, "example.com/app", "")
	identity := graphIdentity(t, goMod)
	for _, test := range []struct {
		name     string
		identity FileIdentity
		want     string
	}{
		{name: "missing fields", identity: FileIdentity{}, want: "requires path and SHA-256"},
		{name: "short digest", identity: FileIdentity{Path: goMod, SHA256: "abcd"}, want: "must be 64 hex characters"},
		{name: "non-hex digest", identity: FileIdentity{Path: goMod, SHA256: strings.Repeat("g", 64)}, want: "invalid target modfile SHA-256"},
		{name: "uppercase digest", identity: FileIdentity{Path: goMod, SHA256: strings.ToUpper(identity.SHA256)}, want: "must use lowercase"},
		{name: "relative path", identity: FileIdentity{Path: "go.mod", SHA256: identity.SHA256}, want: "target modfile path"},
		{name: "directory path", identity: FileIdentity{Path: root, SHA256: identity.SHA256}, want: "target modfile path"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := validateFileIdentity(test.identity); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateFileIdentity() error = %v, want substring %q", err, test.want)
			}
		})
	}

	t.Run("symlink path", func(t *testing.T) {
		alias := filepath.Join(root, "go.mod.alias")
		makeSymlink(t, goMod, alias)
		defer os.Remove(alias)
		if _, err := validateFileIdentity(FileIdentity{Path: alias, SHA256: identity.SHA256}); err == nil || !strings.Contains(err.Error(), "path must be canonical") {
			t.Fatalf("symlink identity error = %v", err)
		}
	})
}

func TestReadFileSHA256ReturnsOneSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.mod")
	want := []byte("module example.com/snapshot\n")
	if err := os.WriteFile(path, want, 0644); err != nil {
		t.Fatal(err)
	}
	data, digest, err := readFileSHA256(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(want) {
		t.Fatalf("snapshot data = %q, want %q", data, want)
	}
	if digest != sha256Hex(data) {
		t.Fatalf("snapshot digest = %q, want digest of returned bytes", digest)
	}
}

func TestValidateModuleCacheSplitSourceRejectsMissingMetadata(t *testing.T) {
	t.Run("missing download metadata", func(t *testing.T) {
		path := "example.com/framework"
		version := "v1.2.3"
		dir, goMod := writeModuleCacheSource(t, filepath.Join(t.TempDir(), "modcache"), path, version, "")
		dir, err := filepath.EvalSymlinks(dir)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(goMod); err != nil {
			t.Fatal(err)
		}
		if err := validateModuleCacheSplitSource(ModuleRef{Path: path, Version: version}, dir, goMod); err == nil || !strings.Contains(err.Error(), "download-cache go.mod") {
			t.Fatalf("missing metadata error = %v", err)
		}
	})

	if err := validateModuleCacheSplitSource(ModuleRef{Path: "example.com/framework"}, "/", "/"); err == nil || !strings.Contains(err.Error(), "module has no version") {
		t.Fatalf("missing version error = %v", err)
	}
}

func TestCloneResolvedModuleCopiesReplacement(t *testing.T) {
	original := ResolvedModule{
		Selected: ModuleRef{Path: "example.com/framework", Version: "v1.2.3"},
		Replace:  &ModuleRef{Path: "/workspace/framework", Dir: "/workspace/framework", GoMod: "/workspace/framework/go.mod"},
	}
	clone := cloneResolvedModule(original)
	if clone == nil {
		t.Fatal("clone is nil")
	}
	if clone.Replace == nil || clone.Replace == original.Replace {
		t.Fatalf("clone replacement pointer = %p, original = %p", clone.Replace, original.Replace)
	}
	clone.Replace.Path = "/workspace/other"
	if original.Replace.Path == clone.Replace.Path {
		t.Fatal("mutating clone changed original replacement")
	}
}
