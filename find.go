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

package mod

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

const (
	// GoWorkFile is the file name of a workspace file.
	GoWorkFile = "go.work"

	// GoModFile is the file name of a module file.
	GoModFile = "go.mod"
)

const (
	ErrNotFound = syscall.ENOENT
)

// Finder locates go.work and go.mod files. The zero value uses the real file
// system and the process environment; the fields exist so that tests (and
// tools that run against a virtual file system) can substitute them.
type Finder struct {
	// Getenv reads an environment variable. If nil, os.Getenv is used.
	Getenv func(key string) string

	// Stat reports file information. If nil, os.Stat is used. Like the go
	// command it should follow symbolic links.
	Stat func(name string) (fs.FileInfo, error)
}

// Discover is like Find but also honors the GOWORK environment variable.
// It is shorthand for Finder{}.Discover.
func Discover(dirFrom string) (gowork, gomod string, err error) {
	return Finder{}.Discover(dirFrom)
}

// Find starts at dirFrom (the current directory if empty) and walks up
// towards the filesystem root, looking for go.work and go.mod in every
// directory on the way. The two searches are independent, and the go.work
// search does not stop when a go.mod has been seen.
//
// The first go.work found ends the search. In that case gowork is set and
// gomod is always empty: with a workspace, the main modules are the ones
// listed in its "use" directives, and the nearest go.mod is not what defines
// them. Use Workspace.CheckDir to verify that the module of the current
// directory is one of them.
//
// If there is no go.work, gomod is the nearest go.mod and gowork is empty.
// ErrNotFound is returned if there is neither.
//
// A directory named go.work or go.mod is not a match. Find does not consult
// the GOWORK environment variable; see Discover.
func (f Finder) Find(dirFrom string) (gowork, gomod string, err error) {
	dir, err := absDir(dirFrom)
	if err != nil {
		return
	}
	for {
		if file := filepath.Join(dir, GoWorkFile); f.isFile(file) {
			return file, "", nil
		}
		if gomod == "" {
			if file := filepath.Join(dir, GoModFile); f.isFile(file) {
				gomod = file
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir { // reached the root
			if gomod == "" {
				err = ErrNotFound
			}
			return
		}
		dir = parent
	}
}

// Discover behaves like Find but honors GOWORK the way the go command does:
//
//   - GOWORK=off disables workspace mode: only go.mod is searched for.
//   - GOWORK=<file> uses that file as the workspace file, without searching.
//   - Otherwise (unset or empty) it is exactly Find.
func (f Finder) Discover(dirFrom string) (gowork, gomod string, err error) {
	switch env := f.getenv("GOWORK"); env {
	case "":
		return f.Find(dirFrom)
	case "off":
		gomod, err = f.FindGoMod(dirFrom)
		return "", gomod, err
	default:
		abs, err := filepath.Abs(env)
		if err != nil {
			return "", "", err
		}
		if !f.isFile(abs) {
			return "", "", fmt.Errorf("GOWORK=%s: not a file", env)
		}
		return abs, "", nil
	}
}

// FindGoMod returns the nearest go.mod at or above dirFrom (the current
// directory if empty), ignoring any go.work. It returns ErrNotFound if there
// is none.
func (f Finder) FindGoMod(dirFrom string) (gomod string, err error) {
	dir, err := absDir(dirFrom)
	if err != nil {
		return "", err
	}
	for {
		if file := filepath.Join(dir, GoModFile); f.isFile(file) {
			return file, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", ErrNotFound
		}
		dir = parent
	}
}

func (f Finder) getenv(key string) string {
	if f.Getenv != nil {
		return f.Getenv(key)
	}
	return os.Getenv(key)
}

func (f Finder) isFile(name string) bool {
	stat := f.Stat
	if stat == nil {
		stat = os.Stat
	}
	fi, err := stat(name)
	return err == nil && !fi.IsDir()
}

func absDir(dir string) (string, error) {
	if dir == "" {
		dir = "."
	}
	return filepath.Abs(dir)
}
