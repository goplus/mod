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

	"github.com/goplus/mod/modfile"
	"github.com/goplus/mod/modload"
	"golang.org/x/mod/module"
)

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
		{"duplicate", []ResolvedModule{first, first}, "want marker"},
		{"target repeated", []ResolvedModule{target, second}, "want marker"},
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
	driverBackedProject := &Project{
		Ext:    ".driver",
		Driver: &modfile.Driver{Protocol: "v1", Package: "example.com/driver"},
	}
	if err := registerProject(map[string]*Project{}, map[string]*ProjectInfo{}, &ProjectInfo{Project: driverBackedProject}); err == nil || !strings.Contains(err.Error(), "driver-backed project") {
		t.Fatalf("orphan driver-backed project error = %v", err)
	}

	same := &Project{Ext: ".same", Class: "Same"}
	projects := map[string]*Project{same.Ext: same}
	infos := map[string]*ProjectInfo{same.Ext: {Project: same}}
	if err := registerProject(projects, infos, &ProjectInfo{Project: same}); err != nil {
		t.Fatalf("same project registration failed: %v", err)
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
