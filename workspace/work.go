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

package workspace

import (
	"os"
	"path/filepath"
	"syscall"
)

const (
	ErrNotFound = syscall.ENOENT
)

// -----------------------------------------------------------------------------

// Find locates the file that defines the build context for dirFrom: either
// a go.work (workspace mode) or a go.mod (single-module mode).
//
// It starts at dirFrom (the current directory if empty) and walks up towards
// the filesystem root, looking for go.work and go.mod in each directory.
// The first go.work found wins and ends the search; in that case gowork is
// set and gomod is always empty, even if a go.mod was seen on the way up.
// Otherwise gomod is the nearest go.mod found, and gowork is empty.
//
// ErrNotFound is returned if neither file exists in dirFrom or any of its
// ancestors.
//
// Find does not consult the GOWORK environment variable.
func Find(dirFrom string) (gowork, gomod string, err error) {
	if dirFrom == "" {
		dirFrom = "."
	}
	dir, err := filepath.Abs(dirFrom)
	if err != nil {
		return
	}
	for {
		file := filepath.Join(dir, "go.work")
		if fi, e := os.Stat(file); e == nil && !fi.IsDir() {
			return file, "", nil
		}
		if gomod == "" {
			file = filepath.Join(dir, "go.mod")
			if fi, e := os.Stat(file); e == nil && !fi.IsDir() {
				gomod = file
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			if gomod == "" {
				err = ErrNotFound
			}
			return
		}
		dir = parent
	}
}

// -----------------------------------------------------------------------------
