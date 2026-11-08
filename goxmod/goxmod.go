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

// Package goxmod answers two questions about an XGo module:
//
//  1. What does its module description say? That is gox.mod (with gop.mod as a
//     legacy fallback), enriched with information that only exists in go.mod.
//     See [LoadModfile].
//  2. Which classfile extensions are in effect for it, and what are their
//     projects? See [ImportClasses] and [Classes].
//
// A classfile is not decided by the file itself. Whether a file such as
// main.gsh, foo_test.gox or Sprite1.spx is a classfile depends on the built-in
// projects, the projects declared by the module's own gox.mod, and the projects
// declared by the dependency modules that the module marks with //xgo:class (or
// the legacy //gop:class) on a require line in go.mod. goxmod resolves all of
// this into a single, read-only lookup table.
package goxmod

import (
	"github.com/goplus/mod/modfile"
	"github.com/qiniu/x/errors"
)

// Class is a work class statement, re-exported from
// [github.com/goplus/mod/modfile] so callers do not have to import modfile just
// to name the type.
type Class = modfile.Class

// Project is a project statement, re-exported from
// [github.com/goplus/mod/modfile] so callers do not have to import modfile just
// to name the type.
type Project = modfile.Project

var (
	// GshProject is the built-in project for the XGo shell (".gsh") classfile.
	// It is a package-level value shared by every Classes, so callers must
	// treat it, and every *Project obtained from a Classes, as read-only.
	GshProject = &Project{
		Ext:      ".gsh",
		Class:    "App",
		PkgPaths: []string{"github.com/qiniu/x/gsh", "math"},
	}
	// TestProject is the built-in project for the XGo test ("_test.gox")
	// classfile, with the "Case" work class. It is a package-level value shared
	// by every Classes, so callers must treat it, and every *Project obtained
	// from a Classes, as read-only.
	TestProject = &Project{
		Ext:      "_test.gox",
		Class:    "App",
		PkgPaths: []string{"github.com/goplus/xgo/test", "testing"},
		Works:    []*modfile.Class{{Ext: "_test.gox", Class: "Case"}},
	}
)

// ErrNotClassFileMod reports that a module marked as a class module declares no
// projects, so it contributes no classfile. See [ImportClasses].
var ErrNotClassFileMod = errors.New("not a classfile module")

// Classes represents a collection of classfiles, indexed by classfile
// extension. It is a read-only lookup table: it is complete once [ImportClasses]
// returns, and all of its methods are safe for concurrent use.
type Classes struct {
	projs map[string]*Project // ext -> project
}

// ClassInfo reports whether fname is a known classfile.
//
// It takes the classfile extension of fname (see [modfile.ClassExt]). If the
// extension is known, ClassInfo returns the project's auto-lambda map, whether
// fname is the project (main) file of its class family as opposed to a work
// file, and ok == true. For an unknown file it returns nil, false, false.
//
// See https://github.com/goplus/xgo/issues/2828 to learn about auto lambda.
func (p *Classes) ClassInfo(fname string) (autoLambdas map[string]int, isProj, ok bool) {
	ext := modfile.ClassExt(fname)
	if c, ok := p.projs[ext]; ok {
		return c.AutoLambdas, c.IsProj(ext, fname), true
	}
	return
}

// IsClass reports whether ext is a known classfile extension.
func (p *Classes) IsClass(ext string) (ok bool) {
	_, ok = p.projs[ext]
	return
}

// LookupClass returns the project that owns ext, if any.
func (p *Classes) LookupClass(ext string) (c *Project, ok bool) {
	c, ok = p.projs[ext]
	return
}

// -----------------------------------------------------------------------------
