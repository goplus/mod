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
)

func TestImportClassesResolvedDriverBackedCollision(t *testing.T) {
	root := t.TempDir()
	targetGox := "xgo 1.9\nproject .foo Game example.com/app\ndriver v1 example.com/app/driver\n"
	targetGoMod := writeModule(t, root, "example.com/app", targetGox)
	if err := os.WriteFile(targetGoMod, []byte("module example.com/app\n\ngo 1.25\n\nrequire example.com/class v1.2.3 //xgo:class\n"), 0644); err != nil {
		t.Fatal(err)
	}
	dep := filepath.Join(root, "dep")
	depGox := "xgo 1.8\nproject .foo Other example.com/class\ndriver v1 example.com/class/driver\n"
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
	if err == nil || !strings.Contains(err.Error(), "driver-backed class extension collision") {
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
