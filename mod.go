/*
 * Copyright (c) 2021 The GoPlus Authors (goplus.org). All rights reserved.
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

type Mode int

const (
	GoModOnly Mode = 1
)

// GOPMOD checks the modfile in this dir or its ancestors.
// If mode == 0, it checks both gop.mod and go.mod
// If mode == GoModOnly, it checks go.mod
func GOPMOD(dir string, mode Mode) (file string, err error) {
	if dir == "" {
		dir = "."
	}
	if dir, err = filepath.Abs(dir); err != nil {
		return
	}
	for dir != "" {
		if mode != GoModOnly {
			file = filepath.Join(dir, "gop.mod")
			if fi, e := os.Lstat(file); e == nil && !fi.IsDir() {
				return
			}
		}
		file = filepath.Join(dir, "go.mod")
		if fi, e := os.Lstat(file); e == nil && !fi.IsDir() {
			return
		}
		if dir, file = filepath.Split(strings.TrimRight(dir, "/\\")); file == "" {
			break
		}
	}
	return "", syscall.ENOENT
}

// -----------------------------------------------------------------------------
