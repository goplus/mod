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
	"strings"
	"syscall"
	"testing"

	"github.com/goplus/mod/modfile"
	"github.com/goplus/mod/modload"
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

func TestResolvedModuleEffectiveAndLocal(t *testing.T) {
	selected := ModuleRef{Path: "example.com/framework", Version: "v1.2.3"}
	local := ModuleRef{Path: "/tmp/framework", Dir: "/tmp/framework", GoMod: "/tmp/framework/go.mod"}
	resolved := ResolvedModule{Selected: selected, Replace: &local}
	if got := resolved.Effective(); got != local {
		t.Fatalf("Effective = %#v, want %#v", got, local)
	}
	if !resolved.IsLocal() {
		t.Fatal("filesystem replacement should be local")
	}
	resolved.Replace.Version = "v1.2.3"
	if resolved.IsLocal() {
		t.Fatal("versioned replacement should not be local")
	}
	main := ResolvedModule{Selected: selected, Main: true}
	if !main.IsLocal() {
		t.Fatal("main module should be local")
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

func TestImportClassesResolvedProvenanceAndSelfOverlap(t *testing.T) {
	root := t.TempDir()
	targetGox := `xgo 1.9

project .foo Game example.com/app
class .foo Sprite
	runtime v1 example.com/app/cmd/runtime
`
	targetGoMod := writeModule(t, root, "example.com/app", targetGox)
	if err := os.WriteFile(targetGoMod, []byte("module example.com/app\n\ngo 1.25\n\nrequire example.com/class v1.2.3 //xgo:class\n"), 0644); err != nil {
		t.Fatal(err)
	}
	dep := filepath.Join(root, "dep")
	depGox := `xgo 1.8

project .dep Dep example.com/class
`
	depGoMod := writeModule(t, dep, "example.com/class", depGox)

	loaded, err := modload.LoadFrom(targetGoMod, filepath.Join(root, "gox.mod"))
	if err != nil {
		t.Fatal(err)
	}
	// The graph is deliberately supplied independently of the receiver's
	// legacy ClassMods field. It must be the sole source of imported classes.
	loaded.Opt.ClassMods = []string{"example.com/class"}
	m := New(loaded)
	target := graphModule("example.com/app", "", root, targetGoMod, true)
	depRecord := graphModule("example.com/class", "v1.2.3", dep, depGoMod, false)
	graph := ResolvedClassGraph{
		Target:        target,
		ClassModules:  []ResolvedModule{depRecord},
		TargetModFile: graphIdentity(t, targetGoMod),
	}
	var callbacks []*ProjectInfo
	if err := m.ImportClassesResolved(graph, func(info *ProjectInfo) { callbacks = append(callbacks, info) }); err != nil {
		t.Fatal(err)
	}
	targetInfo, ok := m.LookupClassInfo(".foo")
	if !ok || targetInfo.Project.Runtime == nil {
		t.Fatalf("target info = %#v, ok=%v", targetInfo, ok)
	}
	if targetInfo.Origin == nil || targetInfo.Origin.Selected.Path != "example.com/app" || targetInfo.RequiredXGo != "1.9" {
		t.Fatalf("target provenance = %#v", targetInfo)
	}
	if targetInfo.Declaration != graphIdentity(t, filepath.Join(root, "gox.mod")) {
		t.Fatalf("target declaration = %#v", targetInfo.Declaration)
	}
	workInfo, ok := m.LookupClassInfo(".foo")
	if !ok || workInfo != targetInfo {
		t.Fatal("project and work extension must share one ProjectInfo")
	}
	depInfo, ok := m.LookupClassInfo(".dep")
	if !ok || depInfo.Origin == nil || depInfo.Origin.Selected.Path != "example.com/class" || depInfo.RequiredXGo != "1.8" {
		t.Fatalf("dep provenance = %#v", depInfo)
	}
	if depInfo.Declaration != graphIdentity(t, filepath.Join(dep, "gox.mod")) {
		t.Fatalf("dependency declaration = %#v", depInfo.Declaration)
	}
	builtin, ok := m.LookupClassInfo(".gsh")
	if !ok || builtin.Origin != nil || builtin.Declaration != (FileIdentity{}) || builtin.RequiredXGo != "" {
		t.Fatalf("builtin provenance = %#v", builtin)
	}
	if len(callbacks) != 4 { // Test, Gsh, target and dependency projects.
		t.Fatalf("callback count = %d", len(callbacks))
	}
}

func TestImportClassesResolvedUsesGraphClassModules(t *testing.T) {
	root := t.TempDir()
	targetGoMod := writeModule(t, root, "example.com/app", "xgo 1.9\nproject .foo Game example.com/app\n")
	loaded, err := modload.LoadFrom(targetGoMod, filepath.Join(root, "gox.mod"))
	if err != nil {
		t.Fatal(err)
	}
	// A stale receiver value must not cause an import when the graph says no
	// class module is selected.
	loaded.Opt.ClassMods = []string{"example.com/missing"}
	m := New(loaded)
	target := graphModule("example.com/app", "", root, targetGoMod, true)
	graph := ResolvedClassGraph{Target: target, TargetModFile: graphIdentity(t, targetGoMod)}
	if err := m.ImportClassesResolved(graph); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.LookupClass(".missing"); ok {
		t.Fatal("stale ClassMods imported a class module")
	}
}

func TestImportClassesResolvedPreservesClassModuleOrder(t *testing.T) {
	root := t.TempDir()
	targetGoMod := writeModule(t, root, "example.com/app", "")
	if err := os.WriteFile(targetGoMod, []byte(`module example.com/app

go 1.25

require (
	example.com/second v1.0.0 //gop:class payload
	example.com/first v1.0.0 //xgo:class
)
`), 0644); err != nil {
		t.Fatal(err)
	}
	secondDir := filepath.Join(root, "second")
	secondGoMod := writeModule(t, secondDir, "example.com/second", "xgo 1.9\nproject .second Second example.com/second\n")
	firstDir := filepath.Join(root, "first")
	firstGoMod := writeModule(t, firstDir, "example.com/first", "xgo 1.9\nproject .first First example.com/first\n")
	loaded, err := modload.LoadFrom(targetGoMod, filepath.Join(root, "gox.mod"))
	if err != nil {
		t.Fatal(err)
	}
	target := graphModule("example.com/app", "", root, targetGoMod, true)
	second := graphModule("example.com/second", "v1.0.0", secondDir, secondGoMod, false)
	first := graphModule("example.com/first", "v1.0.0", firstDir, firstGoMod, false)
	graph := ResolvedClassGraph{
		Target: target, ClassModules: []ResolvedModule{second, first}, TargetModFile: graphIdentity(t, targetGoMod),
	}
	var imported []string
	if err := New(loaded).ImportClassesResolved(graph, func(info *ProjectInfo) {
		if info.Origin != nil && info.Origin.Selected.Path != target.Selected.Path {
			imported = append(imported, info.Origin.Selected.Path)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(imported, ","), "example.com/second,example.com/first"; got != want {
		t.Fatalf("import order = %q, want %q", got, want)
	}
}

func TestImportClassesResolvedAllowsAbsentTargetGoxMod(t *testing.T) {
	root := t.TempDir()
	targetGoMod := writeModule(t, root, "example.com/app", "")
	if err := os.WriteFile(targetGoMod, []byte("module example.com/app\n\ngo 1.25\n\nrequire example.com/framework v1.2.3 //xgo:class\n"), 0644); err != nil {
		t.Fatal(err)
	}
	dep := filepath.Join(root, "framework")
	depGoMod := writeModule(t, dep, "example.com/framework", "xgo 1.8\nproject .foo Framework example.com/framework\n")
	loaded, err := modload.LoadFrom(targetGoMod, filepath.Join(root, "gox.mod"))
	if err != nil {
		t.Fatal(err)
	}
	m := New(loaded)
	target := graphModule("example.com/app", "", root, targetGoMod, true)
	depRecord := graphModule("example.com/framework", "v1.2.3", dep, depGoMod, false)
	graph := ResolvedClassGraph{
		Target:        target,
		ClassModules:  []ResolvedModule{depRecord},
		TargetModFile: graphIdentity(t, targetGoMod),
	}
	if err := m.ImportClassesResolved(graph); err != nil {
		t.Fatal(err)
	}
	info, ok := m.LookupClassInfo(".foo")
	if !ok || info.Origin == nil || info.Origin.Selected.Path != "example.com/framework" {
		t.Fatalf("framework info = %#v, ok=%v", info, ok)
	}
}

func TestImportClassesResolvedModuleCacheSplitGoMod(t *testing.T) {
	root := t.TempDir()
	targetGoMod := writeModule(t, root, "example.com/app", "")
	const (
		frameworkPath    = "example.com/Framework"
		frameworkVersion = "v1.2.3"
	)
	if err := os.WriteFile(targetGoMod, []byte("module example.com/app\n\ngo 1.25\n\nrequire "+frameworkPath+" "+frameworkVersion+" //xgo:class\n"), 0644); err != nil {
		t.Fatal(err)
	}
	frameworkDir, frameworkGoMod := writeModuleCacheSource(t, filepath.Join(root, "modcache"), frameworkPath, frameworkVersion,
		"xgo 1.8\nproject .foo Framework example.com/framework\n")
	loaded, err := modload.LoadFrom(targetGoMod, filepath.Join(root, "gox.mod"))
	if err != nil {
		t.Fatal(err)
	}
	m := New(loaded)
	target := graphModule("example.com/app", "", root, targetGoMod, true)
	framework := graphModule(frameworkPath, frameworkVersion, frameworkDir, frameworkGoMod, false)
	graph := ResolvedClassGraph{
		Target:        target,
		ClassModules:  []ResolvedModule{framework},
		TargetModFile: graphIdentity(t, targetGoMod),
	}
	if err := m.ImportClassesResolved(graph); err != nil {
		t.Fatal(err)
	}
	info, ok := m.LookupClassInfo(".foo")
	if !ok || info.Origin == nil {
		t.Fatalf("framework info = %#v, ok=%v", info, ok)
	}
	effective := info.Origin.Effective()
	canonicalGoMod, err := filepath.EvalSymlinks(frameworkGoMod)
	if err != nil {
		t.Fatal(err)
	}
	if effective.Path != frameworkPath || effective.Version != frameworkVersion || effective.GoMod != canonicalGoMod {
		t.Fatalf("effective origin = %#v", effective)
	}
}

func TestResolvedGraphRejectsUnrelatedExternalGoMod(t *testing.T) {
	root := t.TempDir()
	const (
		modulePath    = "example.com/Framework"
		moduleVersion = "v1.2.3"
	)
	dir, goMod := writeModuleCacheSource(t, filepath.Join(root, "cache-a"), modulePath, moduleVersion, "")
	_, otherGoMod := writeModuleCacheSource(t, filepath.Join(root, "cache-b"), modulePath, moduleVersion, "")
	canonicalDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	canonicalGoMod, err := filepath.EvalSymlinks(goMod)
	if err != nil {
		t.Fatal(err)
	}
	canonicalOther, err := filepath.EvalSymlinks(otherGoMod)
	if err != nil {
		t.Fatal(err)
	}
	externalGoMod := filepath.Join(root, "external.mod")
	if err := os.WriteFile(externalGoMod, []byte("module "+modulePath+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	canonicalExternal, err := filepath.EvalSymlinks(externalGoMod)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name  string
		ref   ModuleRef
		match string
	}{
		{
			name:  "different cache root",
			ref:   ModuleRef{Path: modulePath, Version: moduleVersion, Dir: canonicalDir, GoMod: canonicalOther},
			match: "download-cache identity",
		},
		{
			name:  "arbitrary external go.mod",
			ref:   ModuleRef{Path: modulePath, Version: moduleVersion, Dir: canonicalDir, GoMod: canonicalExternal},
			match: "download-cache identity",
		},
		{
			name:  "wrong logical version",
			ref:   ModuleRef{Path: modulePath, Version: "v1.2.4", Dir: canonicalDir, GoMod: canonicalGoMod},
			match: "source directory does not match",
		},
		{
			name:  "local source cannot split",
			ref:   ModuleRef{Path: modulePath, Dir: canonicalDir, GoMod: canonicalGoMod},
			match: "non-main module selected version must not be empty",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := (ResolvedModule{Selected: tt.ref}).Validate()
			if err == nil || !strings.Contains(err.Error(), tt.match) {
				t.Fatalf("error = %v, want substring %q", err, tt.match)
			}
		})
	}
	badDir, badGoMod := writeModuleCacheSource(t, filepath.Join(root, "cache-content"), modulePath, moduleVersion, "")
	if err := os.WriteFile(badGoMod, []byte("module example.com/Other\n"), 0644); err != nil {
		t.Fatal(err)
	}
	badRecord := graphModule(modulePath, moduleVersion, badDir, badGoMod, false)
	if err := badRecord.Validate(); err == nil || !strings.Contains(err.Error(), "declares") {
		t.Fatalf("mismatched module declaration error = %v", err)
	}
}

func TestImportClassesResolvedRuntimeCollision(t *testing.T) {
	root := t.TempDir()
	targetGox := "xgo 1.9\nproject .foo Game example.com/app\nruntime v1 example.com/app/runtime\n"
	targetGoMod := writeModule(t, root, "example.com/app", targetGox)
	if err := os.WriteFile(targetGoMod, []byte("module example.com/app\n\ngo 1.25\n\nrequire example.com/class v1.2.3 //xgo:class\n"), 0644); err != nil {
		t.Fatal(err)
	}
	dep := filepath.Join(root, "dep")
	depGox := "xgo 1.8\nproject .foo Other example.com/class\nruntime v1 example.com/class/runtime\n"
	depGoMod := writeModule(t, dep, "example.com/class", depGox)
	loaded, err := modload.LoadFrom(targetGoMod, filepath.Join(root, "gox.mod"))
	if err != nil {
		t.Fatal(err)
	}
	m := New(loaded)
	target := graphModule("example.com/app", "", root, targetGoMod, true)
	depRecord := graphModule("example.com/class", "v1.2.3", dep, depGoMod, false)
	graph := ResolvedClassGraph{Target: target, ClassModules: []ResolvedModule{depRecord}, TargetModFile: graphIdentity(t, targetGoMod)}
	err = m.ImportClassesResolved(graph)
	if err == nil || !strings.Contains(err.Error(), "runtime class extension collision") {
		t.Fatalf("error = %v", err)
	}
}

func TestImportClassesResolvedRejectsChangedTargetSnapshots(t *testing.T) {
	root := t.TempDir()
	targetGox := "xgo 1.9\nproject .foo Game example.com/app\n"
	targetGoMod := writeModule(t, root, "example.com/app", targetGox)
	loaded, err := modload.LoadFrom(targetGoMod, filepath.Join(root, "gox.mod"))
	if err != nil {
		t.Fatal(err)
	}
	m := New(loaded)
	target := graphModule("example.com/app", "", root, targetGoMod, true)
	graph := ResolvedClassGraph{Target: target, TargetModFile: graphIdentity(t, targetGoMod)}
	if err := os.WriteFile(filepath.Join(root, "gox.mod"), []byte("xgo 1.9\nproject .bar Changed example.com/app\n"), 0644); err != nil {
		t.Fatal(err)
	}
	err = m.ImportClassesResolved(graph)
	if err == nil || !strings.Contains(err.Error(), "target gox.mod contents changed") {
		t.Fatalf("changed gox.mod error = %v", err)
	}

	// Restore the gox snapshot, then replace go.mod at the same path. The
	// graph digest and the receiver's load digest must reject the mix too.
	if err := os.WriteFile(filepath.Join(root, "gox.mod"), []byte(targetGox), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(targetGoMod, []byte("module example.com/app\n\ngo 1.25\n\n// changed\n"), 0644); err != nil {
		t.Fatal(err)
	}
	err = m.ImportClassesResolved(graph)
	if err == nil || !strings.Contains(err.Error(), "target modfile SHA-256 mismatch") {
		t.Fatalf("changed go.mod error = %v", err)
	}
}

func TestImportClassesResolvedRejectsInMemoryReceiverWithoutSnapshot(t *testing.T) {
	root := t.TempDir()
	targetGoMod := writeModule(t, root, "example.com/app", "xgo 1.9\nproject .foo Game example.com/app\n")
	loaded, err := modload.LoadFrom(targetGoMod, filepath.Join(root, "gox.mod"))
	if err != nil {
		t.Fatal(err)
	}
	inMemory := modload.Module{File: loaded.File, Opt: loaded.Opt}
	target := graphModule("example.com/app", "", root, targetGoMod, true)
	graph := ResolvedClassGraph{Target: target, TargetModFile: graphIdentity(t, targetGoMod)}
	if err := New(inMemory).ImportClassesResolved(graph); err == nil || !strings.Contains(err.Error(), "no target modfile snapshot") {
		t.Fatalf("error = %v", err)
	}
}

func TestResolvedClassGraphRejectsInvalidClassModuleLists(t *testing.T) {
	root := t.TempDir()
	targetGoMod := writeModule(t, root, "example.com/app", "")
	if err := os.WriteFile(targetGoMod, []byte(`module example.com/app

go 1.25

require (
	example.com/first v1.0.0 //xgo:class
	example.com/second v1.0.0 //gop:class
)
`), 0644); err != nil {
		t.Fatal(err)
	}
	target := graphModule("example.com/app", "", root, targetGoMod, true)
	moduleRecord := func(path string) ResolvedModule {
		dir := filepath.Join(root, filepath.Base(path))
		goMod := writeModule(t, dir, path, "")
		return graphModule(path, "v1.0.0", dir, goMod, false)
	}
	first := moduleRecord("example.com/first")
	second := moduleRecord("example.com/second")
	third := moduleRecord("example.com/third")
	identity := graphIdentity(t, targetGoMod)
	tests := []struct {
		name    string
		modules []ResolvedModule
		match   string
	}{
		{"wrong order", []ResolvedModule{second, first}, "want marker"},
		{"duplicate", []ResolvedModule{first, first}, "duplicate resolved class module"},
		{"target repeated", []ResolvedModule{target, second}, "repeated in ClassModules"},
		{"missing", []ResolvedModule{first}, "module count"},
		{"extra", []ResolvedModule{first, second, third}, "module count"},
		{"wrong logical path", []ResolvedModule{first, third}, "want marker"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			graph := ResolvedClassGraph{Target: target, ClassModules: test.modules, TargetModFile: identity}
			if err := graph.validate(); err == nil || !strings.Contains(err.Error(), test.match) {
				t.Fatalf("error = %v, want substring %q", err, test.match)
			}
		})
	}
}

func TestResolvedClassGraphRejectsDuplicateAndTargetMarkers(t *testing.T) {
	for _, test := range []struct {
		name  string
		body  string
		match string
	}{
		{"duplicate", "require example.com/dup v1.0.0 //xgo:class\nrequire example.com/dup v1.0.1 //gop:class\n", "duplicate class module marker"},
		{"target", "require example.com/app v1.0.0 //xgo:class\n", "also marked as a class module"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			goMod := writeModule(t, root, "example.com/app", "")
			if err := os.WriteFile(goMod, []byte("module example.com/app\n\ngo 1.25\n\n"+test.body), 0644); err != nil {
				t.Fatal(err)
			}
			target := graphModule("example.com/app", "", root, goMod, true)
			graph := ResolvedClassGraph{Target: target, TargetModFile: graphIdentity(t, goMod)}
			if err := graph.validate(); err == nil || !strings.Contains(err.Error(), test.match) {
				t.Fatalf("error = %v, want substring %q", err, test.match)
			}
		})
	}
}

func TestResolvedGraphRejectsReplacementPathLeak(t *testing.T) {
	root := t.TempDir()
	goMod := writeModule(t, root, "example.com/app", "xgo 1.9\n")
	bad := ResolvedModule{Selected: ModuleRef{Path: "example.com/app", Version: "v1.0.0"}}
	bad.Selected.Dir = filepath.Join(root, "selected")
	bad.Selected.GoMod = filepath.Join(root, "selected", "go.mod")
	bad.Replace = &ModuleRef{Path: root, Dir: root, GoMod: goMod}
	graph := ResolvedClassGraph{Target: bad, TargetModFile: graphIdentity(t, goMod)}
	if err := graph.validate(); err == nil || !strings.Contains(err.Error(), "selected Dir/GoMod must be empty") {
		t.Fatalf("error = %v", err)
	}
}

func TestResolvedGraphRejectsClassModWithoutRecord(t *testing.T) {
	root := t.TempDir()
	goMod := writeModule(t, root, "example.com/app", "xgo 1.9\n")
	if err := os.WriteFile(goMod, []byte("module example.com/app\n\ngo 1.25\n\nrequire example.com/missing v1.0.0 //xgo:class\n"), 0644); err != nil {
		t.Fatal(err)
	}
	target := graphModule("example.com/app", "", root, goMod, true)
	graph := ResolvedClassGraph{Target: target, TargetModFile: graphIdentity(t, goMod)}
	if err := graph.validate(); err == nil || !strings.Contains(err.Error(), "module count") {
		t.Fatalf("error = %v", err)
	}
}

func TestLookupClassInfoLegacyFallbackAndRegistrationSafety(t *testing.T) {
	legacy := &Project{Ext: ".legacy", Class: "Legacy"}
	m := &Module{projs: map[string]*Project{legacy.Ext: legacy}}
	info, ok := m.LookupClassInfo(legacy.Ext)
	if !ok || info.Project != legacy || info.Origin != nil || info.RequiredXGo != "" {
		t.Fatalf("legacy info = %#v, ok=%v", info, ok)
	}
	if _, ok := m.LookupClassInfo(".missing"); ok {
		t.Fatal("missing class unexpectedly resolved")
	}

	if err := registerProject(nil, nil, nil); err == nil || !strings.Contains(err.Error(), "nil project") {
		t.Fatalf("nil project error = %v", err)
	}
	runtimeProject := &Project{
		Ext:     ".runtime",
		Runtime: &modfile.Runtime{Protocol: "v1", Package: "example.com/provider"},
	}
	if err := registerProject(map[string]*Project{}, map[string]*ProjectInfo{}, &ProjectInfo{Project: runtimeProject}); err == nil || !strings.Contains(err.Error(), "no module provenance") {
		t.Fatalf("orphan runtime error = %v", err)
	}

	same := &Project{Ext: ".same", Class: "Same"}
	projects := map[string]*Project{same.Ext: same}
	infos := map[string]*ProjectInfo{same.Ext: {Project: same}}
	if err := registerProject(projects, infos, &ProjectInfo{Project: same}); err != nil {
		t.Fatalf("same project registration failed: %v", err)
	}
}

func TestImportClassesResolvedRejectsReceiverState(t *testing.T) {
	var nilModule *Module
	if err := nilModule.ImportClassesResolved(ResolvedClassGraph{}); err == nil || !strings.Contains(err.Error(), "no target module snapshot") {
		t.Fatalf("nil receiver error = %v", err)
	}
	if err := (&Module{}).ImportClassesResolved(ResolvedClassGraph{}); err == nil || !strings.Contains(err.Error(), "no target module snapshot") {
		t.Fatalf("empty receiver error = %v", err)
	}

	root := t.TempDir()
	goMod := writeModule(t, root, "example.com/app", "xgo 1.9\n")
	loaded, err := modload.LoadFrom(goMod, filepath.Join(root, "gox.mod"))
	if err != nil {
		t.Fatal(err)
	}
	m := New(loaded)
	target := graphModule("example.com/other", "", root, goMod, true)
	graph := ResolvedClassGraph{Target: target, TargetModFile: graphIdentity(t, goMod)}
	if err := m.ImportClassesResolved(graph); err == nil || !strings.Contains(err.Error(), "does not match graph target") {
		t.Fatalf("target mismatch error = %v", err)
	}
}

func TestImportClassesResolvedPreservesReceiverOnClassImportFailure(t *testing.T) {
	root := t.TempDir()
	goMod := writeModule(t, root, "example.com/app", "xgo 1.9\n")
	if err := os.WriteFile(goMod, []byte("module example.com/app\n\ngo 1.25\n\nrequire example.com/dep v1.0.0 //xgo:class\n"), 0644); err != nil {
		t.Fatal(err)
	}
	depDir := filepath.Join(root, "dep")
	depGoMod := writeModule(t, depDir, "example.com/dep", "")
	loaded, err := modload.LoadFrom(goMod, filepath.Join(root, "gox.mod"))
	if err != nil {
		t.Fatal(err)
	}
	m := New(loaded)
	old := &Project{Ext: ".old", Class: "Old"}
	m.projs = map[string]*Project{old.Ext: old}
	m.infos = map[string]*ProjectInfo{old.Ext: {Project: old}}

	target := graphModule("example.com/app", "", root, goMod, true)
	dep := graphModule("example.com/dep", "v1.0.0", depDir, depGoMod, false)
	graph := ResolvedClassGraph{
		Target:        target,
		ClassModules:  []ResolvedModule{dep},
		TargetModFile: graphIdentity(t, goMod),
	}
	if err := m.ImportClassesResolved(graph); err == nil || !strings.Contains(err.Error(), "not a classfile module") {
		t.Fatalf("class import error = %v", err)
	}
	if got, ok := m.LookupClassInfo(old.Ext); !ok || got.Project != old {
		t.Fatalf("receiver changed after failed import: %#v, ok=%v", got, ok)
	}
}

func TestImportClassesResolvedRejectsReceiverSnapshotMismatch(t *testing.T) {
	t.Run("path", func(t *testing.T) {
		root := t.TempDir()
		goMod := writeModule(t, root, "example.com/app", "xgo 1.9\n")
		copyPath := filepath.Join(root, "graph.go.mod")
		data, err := os.ReadFile(goMod)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(copyPath, data, 0644); err != nil {
			t.Fatal(err)
		}
		loaded, err := modload.LoadFrom(goMod, filepath.Join(root, "gox.mod"))
		if err != nil {
			t.Fatal(err)
		}
		graph := ResolvedClassGraph{
			Target:        graphModule("example.com/app", "", root, goMod, true),
			TargetModFile: graphIdentity(t, copyPath),
		}
		if err := New(loaded).ImportClassesResolved(graph); err == nil || !strings.Contains(err.Error(), "snapshots differ") {
			t.Fatalf("path mismatch error = %v", err)
		}
	})

	t.Run("content", func(t *testing.T) {
		root := t.TempDir()
		goMod := writeModule(t, root, "example.com/app", "xgo 1.9\n")
		loadedSnapshot := []byte("module example.com/app\n\ngo 1.25\n\n// loaded snapshot\n")
		loaded, err := modload.LoadFromEx(goMod, filepath.Join(root, "gox.mod"), func(path string) ([]byte, error) {
			if path == goMod {
				return loadedSnapshot, nil
			}
			return os.ReadFile(path)
		})
		if err != nil {
			t.Fatal(err)
		}
		graph := ResolvedClassGraph{
			Target:        graphModule("example.com/app", "", root, goMod, true),
			TargetModFile: graphIdentity(t, goMod),
		}
		if err := New(loaded).ImportClassesResolved(graph); err == nil || !strings.Contains(err.Error(), "contents differ") {
			t.Fatalf("content mismatch error = %v", err)
		}
	})
}

func TestReceiverGoxSnapshotRequiresLoadedMetadata(t *testing.T) {
	t.Run("absent", func(t *testing.T) {
		root := t.TempDir()
		goMod := writeModule(t, root, "example.com/app", "")
		loaded, err := modload.LoadFrom(goMod, "")
		if err != nil {
			t.Fatal(err)
		}
		graph := ResolvedClassGraph{
			Target:        graphModule("example.com/app", "", root, goMod, true),
			TargetModFile: graphIdentity(t, goMod),
		}
		if err := New(loaded).ImportClassesResolved(graph); err != nil {
			t.Fatalf("absent metadata should be accepted: %v", err)
		}
	})

	t.Run("appeared", func(t *testing.T) {
		root := t.TempDir()
		goMod := writeModule(t, root, "example.com/app", "xgo 1.9\n")
		goxMod := filepath.Join(root, "gox.mod")
		loaded, err := modload.LoadFromEx(goMod, goxMod, func(path string) ([]byte, error) {
			if path == goxMod {
				return nil, os.ErrPermission
			}
			return os.ReadFile(path)
		})
		if err != nil {
			t.Fatal(err)
		}
		graph := ResolvedClassGraph{
			Target:        graphModule("example.com/app", "", root, goMod, true),
			TargetModFile: graphIdentity(t, goMod),
		}
		if err := New(loaded).ImportClassesResolved(graph); err == nil || !strings.Contains(err.Error(), "appeared without load snapshot") {
			t.Fatalf("appeared metadata error = %v", err)
		}
	})
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

func TestReceiverGoxSnapshotRejectsUntrustedIdentity(t *testing.T) {
	t.Run("relative path", func(t *testing.T) {
		loaded, err := modload.LoadFromEx("relative/go.mod", "relative/gox.mod", func(path string) ([]byte, error) {
			switch path {
			case "relative/go.mod":
				return []byte("module example.com/app\n\ngo 1.25\n"), nil
			case "relative/gox.mod":
				return []byte("xgo 1.9\n"), nil
			default:
				return nil, os.ErrNotExist
			}
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := receiverGoxSnapshot(New(loaded), t.TempDir()); err == nil || !strings.Contains(err.Error(), "path must be absolute") {
			t.Fatalf("relative identity error = %v", err)
		}
	})

	t.Run("outside target source", func(t *testing.T) {
		root := t.TempDir()
		goMod := writeModule(t, root, "example.com/app", "")
		outside := filepath.Join(t.TempDir(), "gox.mod")
		if err := os.WriteFile(outside, []byte("xgo 1.9\n"), 0644); err != nil {
			t.Fatal(err)
		}
		loaded, err := modload.LoadFrom(goMod, outside)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := receiverGoxSnapshot(New(loaded), root); err == nil || !strings.Contains(err.Error(), "outside graph target source") {
			t.Fatalf("outside identity error = %v", err)
		}
	})

	t.Run("projects without snapshot", func(t *testing.T) {
		root := t.TempDir()
		goMod := writeModule(t, root, "example.com/app", "")
		loaded, err := modload.LoadFrom(goMod, "")
		if err != nil {
			t.Fatal(err)
		}
		loaded.Opt.Projects = []*modfile.Project{{Ext: ".foo", Class: "Game"}}
		if _, _, err := receiverGoxSnapshot(New(loaded), root); err == nil || !strings.Contains(err.Error(), "projects without") {
			t.Fatalf("projects without snapshot error = %v", err)
		}
	})

	t.Run("declaration disappeared", func(t *testing.T) {
		root := t.TempDir()
		goMod := writeModule(t, root, "example.com/app", "xgo 1.9\n")
		goxMod := filepath.Join(root, "gox.mod")
		loaded, err := modload.LoadFrom(goMod, goxMod)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(goxMod); err != nil {
			t.Fatal(err)
		}
		if _, _, err := receiverGoxSnapshot(New(loaded), root); err == nil || !strings.Contains(err.Error(), "receiver target gox.mod") {
			t.Fatalf("disappeared declaration error = %v", err)
		}
	})
}

func TestImportClassesResolvedRejectsRelativeReceiverModfile(t *testing.T) {
	root := t.TempDir()
	graphGoMod := writeModule(t, root, "example.com/app", "")
	loaded, err := modload.LoadFromEx("relative/go.mod", "", func(path string) ([]byte, error) {
		if path == "relative/go.mod" {
			return []byte("module example.com/app\n\ngo 1.25\n"), nil
		}
		return nil, os.ErrNotExist
	})
	if err != nil {
		t.Fatal(err)
	}
	target := graphModule("example.com/app", "", root, graphGoMod, true)
	graph := ResolvedClassGraph{Target: target, TargetModFile: graphIdentity(t, graphGoMod)}
	if err := New(loaded).ImportClassesResolved(graph); err == nil || !strings.Contains(err.Error(), "receiver target modfile") {
		t.Fatalf("relative receiver modfile error = %v", err)
	}
}

func TestResolvedGraphRejectsMalformedTargetAndMismatchedSource(t *testing.T) {
	t.Run("malformed target modfile", func(t *testing.T) {
		root := t.TempDir()
		goMod := writeModule(t, root, "example.com/app", "")
		if err := os.WriteFile(goMod, []byte("module example.com/app\n\nrequire (\n"), 0644); err != nil {
			t.Fatal(err)
		}
		graph := ResolvedClassGraph{
			Target:        graphModule("example.com/app", "", root, goMod, true),
			TargetModFile: graphIdentity(t, goMod),
		}
		if err := graph.validate(); err == nil || !strings.Contains(err.Error(), "parse target modfile") {
			t.Fatalf("malformed target error = %v", err)
		}
	})

	t.Run("source declares another module", func(t *testing.T) {
		root := t.TempDir()
		targetGoMod := writeModule(t, root, "example.com/app", "")
		if err := os.WriteFile(targetGoMod, []byte("module example.com/app\n\ngo 1.25\n\nrequire example.com/dep v1.0.0 //xgo:class\n"), 0644); err != nil {
			t.Fatal(err)
		}
		depDir := filepath.Join(root, "dep")
		depGoMod := writeModule(t, depDir, "example.com/wrong", "xgo 1.9\nproject .dep Dep example.com/wrong\n")
		loaded, err := modload.LoadFrom(targetGoMod, "")
		if err != nil {
			t.Fatal(err)
		}
		m := New(loaded)
		old := &Project{Ext: ".old", Class: "Old"}
		m.projs = map[string]*Project{old.Ext: old}
		m.infos = map[string]*ProjectInfo{old.Ext: {Project: old}}
		graph := ResolvedClassGraph{
			Target:        graphModule("example.com/app", "", root, targetGoMod, true),
			ClassModules:  []ResolvedModule{graphModule("example.com/dep", "v1.0.0", depDir, depGoMod, false)},
			TargetModFile: graphIdentity(t, targetGoMod),
		}
		if err := m.ImportClassesResolved(graph); err == nil || !strings.Contains(err.Error(), "declares") {
			t.Fatalf("source identity error = %v", err)
		}
		if got, ok := m.LookupClassInfo(old.Ext); !ok || got.Project != old {
			t.Fatalf("receiver changed after source identity error: %#v, ok=%v", got, ok)
		}
	})
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
		if err := resolved.Validate(); err == nil || !strings.Contains(err.Error(), "replacement.Path must be canonical") {
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

func TestImportClassesLegacyReportsMissingAndNonClassModules(t *testing.T) {
	root := t.TempDir()
	goMod := writeModule(t, root, "example.com/app", "")
	loaded, err := modload.LoadFrom(goMod, "")
	if err != nil {
		t.Fatal(err)
	}
	loaded.Opt.ClassMods = []string{"example.com/missing"}
	if err := New(loaded).ImportClasses(); err == nil || !IsNotFound(err) {
		t.Fatalf("missing class module error = %v", err)
	}

	noClassDir := filepath.Join(root, "no-class")
	writeModule(t, noClassDir, "example.com/no-class", "")
	if err := (&Module{}).importClassFrom(module.Version{Path: noClassDir}, nil); err != ErrNotClassFileMod {
		t.Fatalf("non-class module error = %v, want %v", err, ErrNotClassFileMod)
	}
}
