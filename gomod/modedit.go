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

package gomod

import (
	"os"

	"golang.org/x/mod/modfile"
)

const (
	defaultGoVer = "1.23"
)

// -----------------------------------------------------------------------------

// File wraps a modfile.File and provides higher-level operations for editing
// go.mod. File is a small value type; copies share the same underlying
// modfile.File, so edits made through any copy are visible to all of them.
type File struct {
	*modfile.File
}

// Create creates a new, in-memory go.mod for module modPath with a go directive
// of goVer. gomod is the file name used later by [File.Save]; nothing is
// written to disk until then. It fails if goVer is not a valid Go version;
// modPath is not validated.
func Create(gomod, modPath, goVer string) (_ File, err error) {
	f := new(modfile.File)
	if err = f.AddModuleStmt(modPath); err != nil {
		return
	}
	if err = f.AddGoStmt(goVer); err != nil {
		return
	}
	f.Syntax.Name = gomod
	return File{f}, nil
}

// Load reads and parses the go.mod at the given path. The path is remembered
// so that [File.Save] writes back to the same file.
func Load(gomod string) (_ File, err error) {
	data, err := os.ReadFile(gomod)
	if err != nil {
		return
	}
	f, err := modfile.Parse(gomod, data, nil)
	return File{f}, err
}

// AddCompiler records a compiler directive (llgo/tinygo) as a comment on the
// go directive in go.mod, the way the XGo toolchain reads it (see [Compiler]).
// For example, AddCompiler("llgo", "0.9") produces "go 1.18 // llgo 0.9".
// If there is no go directive, one with the default Go version is added first.
// Any existing trailing comment on the go directive is replaced.
func (f File) AddCompiler(compiler, ver string) {
	if f.Go == nil {
		f.AddGoStmt(defaultGoVer)
	}
	if line := f.Go.Syntax; line != nil {
		line.Suffix = []modfile.Comment{{
			Token:  "// " + compiler + " " + ver,
			Suffix: true,
		}}
	}
}

// AddRequire adds a require line for path at version vers, or updates the
// version if path is already required. When hasProj is set (the dependency
// declares classfile projects), the require is also marked as a class module
// with a trailing //xgo:class comment, unless it is already marked (see
// [IsClass]). An existing marker is kept even when hasProj is false.
func (f File) AddRequire(path, vers string, hasProj bool) error {
	if err := f.File.AddRequire(path, vers); err != nil {
		return err
	}
	if hasProj {
		for _, r := range f.Require {
			if r.Mod.Path == path {
				if !IsClass(r) && r.Syntax != nil {
					r.Syntax.Suffix = append(r.Syntax.Suffix, modfile.Comment{
						Token:  "//xgo:class",
						Suffix: true,
					})
				}
				break
			}
		}
	}
	return nil
}

// Save cleans up and formats go.mod, then writes it to the path given to
// [Create] or [Load]. The compiler directive (llgo/tinygo) lives in the go.mod
// `go` line suffix, and class module markers in require line suffixes, so both
// survive formatting.
func (f File) Save() error {
	f.Cleanup()
	data, err := f.Format()
	if err != nil {
		return err
	}
	return os.WriteFile(f.Syntax.Name, data, 0644)
}

// -----------------------------------------------------------------------------
