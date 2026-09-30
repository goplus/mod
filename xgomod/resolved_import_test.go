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

	"github.com/goplus/mod/modload"
)

func TestImportClassesResolvedProvenanceAndSelfOverlap(t *testing.T) {
	root := t.TempDir()
	targetGox := `xgo 1.9

project .foo Game example.com/app
class .foo Sprite
	class .work Worker
	driver v1 example.com/app/cmd/driver
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
	// The resolved graph, not legacy ClassMods, is authoritative.
	loaded.Opt.ClassMods = []string{"example.com/class"}
	m := New(loaded)
	target := graphModule("example.com/app", "", root, targetGoMod, true)
	depRecord := graphModule("example.com/class", "v1.2.3", dep, depGoMod, false)
	graph := ResolvedClassGraph{
		Target:        target,
		ClassModules:  []ResolvedModule{depRecord},
		TargetModFile: graphIdentity(t, targetGoMod),
	}
	if err := m.ImportClassesResolved(graph); err != nil {
		t.Fatal(err)
	}
	targetInfo, ok := m.LookupClassInfo(".foo")
	if !ok || targetInfo.Project.Driver == nil {
		t.Fatalf("target info = %#v, ok=%v", targetInfo, ok)
	}
	if targetInfo.Origin == nil || targetInfo.Origin.Selected.Path != "example.com/app" || targetInfo.RequiredXGo != "1.9" {
		t.Fatalf("target provenance = %#v", targetInfo)
	}
	if targetInfo.Declaration != graphIdentity(t, filepath.Join(root, "gox.mod")) {
		t.Fatalf("target declaration = %#v", targetInfo.Declaration)
	}
	workInfo, ok := m.LookupClassInfo(".work")
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
}

func TestImportClassesResolvedUsesGraphClassModules(t *testing.T) {
	root := t.TempDir()
	targetGoMod := writeModule(t, root, "example.com/app", "xgo 1.9\nproject .foo Game example.com/app\n")
	loaded, err := modload.LoadFrom(targetGoMod, filepath.Join(root, "gox.mod"))
	if err != nil {
		t.Fatal(err)
	}
	// Stale ClassMods must not affect resolved imports.
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
	secondGoMod := writeModule(t, secondDir, "example.com/second", "xgo 1.9\nproject .shared Second example.com/second\n")
	firstDir := filepath.Join(root, "first")
	firstGoMod := writeModule(t, firstDir, "example.com/first", "xgo 1.9\nproject .shared First example.com/first\n")
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
	m := New(loaded)
	if err := m.ImportClassesResolved(graph); err != nil {
		t.Fatal(err)
	}
	info, ok := m.LookupClassInfo(".shared")
	if !ok || info.Origin == nil || info.Origin.Selected.Path != "example.com/first" {
		t.Fatalf("shared class = %#v, ok=%v", info, ok)
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
	depGopMod := filepath.Join(dep, "gop.mod")
	if err := os.Rename(filepath.Join(dep, "gox.mod"), depGopMod); err != nil {
		t.Fatal(err)
	}
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
	if info.Declaration != graphIdentity(t, depGopMod) {
		t.Fatalf("framework declaration = %#v", info.Declaration)
	}
}

func TestImportClassesResolvedAllowsLocalReplacementModulePaths(t *testing.T) {
	if moduleSourceMatches(ResolvedModule{}, "") {
		t.Fatal("empty module declaration matched")
	}
	root := t.TempDir()
	targetGoMod := writeModule(t, root, "example.com/local-app", "")
	if err := os.WriteFile(targetGoMod, []byte("module example.com/local-app\n\ngo 1.25\n\nrequire example.com/class v1.0.0 //xgo:class\n"), 0644); err != nil {
		t.Fatal(err)
	}
	classDir := filepath.Join(root, "class")
	classGoMod := writeModule(t, classDir, "example.com/local-class", "xgo 1.8\nproject .foo Framework example.com/local-class\n")
	loaded, err := modload.LoadFrom(targetGoMod, filepath.Join(root, "gox.mod"))
	if err != nil {
		t.Fatal(err)
	}
	targetDir, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	classDir, err = filepath.EvalSymlinks(classDir)
	if err != nil {
		t.Fatal(err)
	}
	targetGoMod, err = filepath.EvalSymlinks(targetGoMod)
	if err != nil {
		t.Fatal(err)
	}
	classGoMod, err = filepath.EvalSymlinks(classGoMod)
	if err != nil {
		t.Fatal(err)
	}
	target := ResolvedModule{
		Selected: ModuleRef{Path: "example.com/app", Version: "v1.0.0"},
		Replace:  &ModuleRef{Path: targetDir, Dir: targetDir, GoMod: targetGoMod},
	}
	class := ResolvedModule{
		Selected: ModuleRef{Path: "example.com/class", Version: "v1.0.0"},
		Replace:  &ModuleRef{Path: classDir, Dir: classDir, GoMod: classGoMod},
	}
	graph := ResolvedClassGraph{Target: target, ClassModules: []ResolvedModule{class}, TargetModFile: graphIdentity(t, targetGoMod)}
	m := New(loaded)
	if err := m.ImportClassesResolved(graph); err != nil {
		t.Fatal(err)
	}
	if info, ok := m.LookupClassInfo(".foo"); !ok || info.Origin == nil || info.Origin.Selected.Path != "example.com/class" {
		t.Fatalf("local replacement provenance = %#v, ok=%v", info, ok)
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
