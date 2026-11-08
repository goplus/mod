/*
 * Copyright (c) 2021 The XGo Authors (xgo.dev). All rights reserved.
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
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// -----------------------------------------------------------------------------

const ErrNotFound = syscall.ENOENT

// FindGoMod searches for the nearest go.mod file starting from the specified directory
// and moving up the directory tree. It returns the directory containing the go.mod file,
// the file path, and an error if not found.
//
// Deprecated: Use Discover instead.
func FindGoMod(dirFrom string) (dir, file string, err error) {
	if dirFrom == "" {
		dirFrom = "."
	}
	if dir, err = filepath.Abs(dirFrom); err != nil {
		return
	}
	for dir != "" {
		file = filepath.Join(dir, "go.mod")
		if fi, e := os.Lstat(file); e == nil && !fi.IsDir() {
			return
		}
		if dir, file = filepath.Split(strings.TrimRight(dir, "/\\")); file == "" {
			break
		}
	}
	err = ErrNotFound
	return
}

// -----------------------------------------------------------------------------

// GOMOD returns the path to the nearest go.mod file starting from the specified directory.
//
// Deprecated: Use Discover instead.
func GOMOD(dirFrom string) (file string, err error) {
	_, file, err = FindGoMod(dirFrom)
	return
}

// GOXMOD finds the directory of the nearest go.mod starting from dirFrom and returns the
// gox.mod path in that directory (falling back to gop.mod for backward compatibility).
//
// Deprecated: Use Discover instead.
func GOXMOD(dirFrom string) (file string, err error) {
	dir, _, err := FindGoMod(dirFrom)
	if err != nil {
		return
	}
	file = filepath.Join(dir, "gox.mod")
	if _, e := os.Lstat(file); e == nil { // gox.mod exists
		return
	}
	return filepath.Join(dir, "gop.mod"), nil // backward compatibility
}

// -----------------------------------------------------------------------------
