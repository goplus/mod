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

package goxmod

import (
	"os"
	"strings"

	"github.com/goplus/mod/modfile"

	gomodfile "golang.org/x/mod/modfile"
)

// defaultXgoVer is the XGo version used when a module has no gox.mod (and no
// gop.mod).
const defaultXgoVer = "1.5"

// LoadModfile reads and parses the gox.mod module description at goxmod,
// enriched with information that only exists in go.mod. It is [LoadModfileEx]
// with os.ReadFile; see that function for the full contract.
func LoadModfile(gomod *gomodfile.File, goxmod string) (*modfile.File, error) {
	return LoadModfileEx(gomod, goxmod, os.ReadFile)
}

// LoadModfileEx reads and parses the gox.mod module description at goxmod,
// enriched with information that only exists in go.mod. The readFile hook lets
// editors and tests supply file contents from memory; LoadModfile passes
// os.ReadFile. gomod is the parsed go.mod of the same module and may be nil.
//
// It performs these steps, in order:
//
//  1. Read goxmod (a path ending in "gox.mod"). If that fails and the name ends
//     in "gox.mod", read "gop.mod" in the same directory instead.
//  2. If a file was read, parse it with [modfile.ParseLax]. A parse error is
//     returned to the caller.
//  3. If no file could be read, the result is an empty module description with
//     the default XGo version, and no error is returned. A module without
//     gox.mod is a normal case, not a failure. Any read error leads here, not
//     only "file not found".
//  4. If gomod is not nil, two things from go.mod are folded into the result:
//     every require whose trailing comment starts with "xgo:class" (or the
//     legacy "gop:class") is appended to ClassMods, and a trailing comment on
//     the go directive that starts with "llgo " or "tinygo " (for example
//     "go 1.18 // llgo 0.9") sets Compiler. The first matching comment wins, and
//     a compiler found in go.mod replaces any compiler set by gox.mod.
//
// The result is therefore the union of what gox.mod declares and what go.mod
// adds, so callers never have to merge the two themselves.
func LoadModfileEx(gomod *gomodfile.File, goxmod string, readFile func(string) ([]byte, error)) (*modfile.File, error) {
	var opt *modfile.File
	if goxmod != "" {
		data, err := readFile(goxmod)
		if err != nil {
			const goxmodSuffix = "gox.mod" // fallback to gop.mod
			if strings.HasSuffix(goxmod, goxmodSuffix) {
				goxmod = goxmod[:len(goxmod)-len(goxmodSuffix)] + "gop.mod"
				data, err = readFile(goxmod)
			}
		}
		if err == nil {
			opt, err = modfile.ParseLax(goxmod, data, nil)
			if err != nil {
				return nil, err
			}
		}
	}
	if opt == nil {
		opt = modfile.New(goxmod, defaultXgoVer)
	}
	if gomod != nil {
		importClassfileFromGoMod(opt, gomod)
		if cl := getGoCompiler(gomod); cl != nil {
			opt.Compiler = cl
		}
	}
	return opt, nil
}

// importClassfileFromGoMod appends to opt.ClassMods every require of f whose
// trailing comment marks it as a class module (//xgo:class or the legacy
// //gop:class).
func importClassfileFromGoMod(opt *modfile.File, f *gomodfile.File) {
	for _, r := range f.Require {
		if isClass(r) {
			opt.ClassMods = append(opt.ClassMods, r.Mod.Path)
		}
	}
}

// isClass reports whether a require is marked as a class module by a trailing
// //xgo:class (or the legacy //gop:class) comment. The marker is matched after
// leading whitespace is trimmed, as a prefix.
func isClass(r *gomodfile.Require) bool {
	if line := r.Syntax; line != nil {
		for _, c := range line.Suffix {
			text := strings.TrimLeft(c.Token[2:], " \t")
			if strings.HasPrefix(text, "xgo:class") || strings.HasPrefix(text, "gop:class") {
				return true
			}
		}
	}
	return false
}

/*
getGoCompiler reads a trailing comment on the go directive that names a custom
compiler, for example:

	go 1.18 // llgo 0.9
	go 1.18 // tinygo 0.32

The first matching comment wins. It returns nil if there is none.
*/
func getGoCompiler(f *gomodfile.File) *modfile.Compiler {
	if gostmt := f.Go; gostmt != nil {
		if line := gostmt.Syntax; line != nil {
			for _, c := range line.Suffix {
				text := strings.TrimLeft(c.Token[2:], " \t")
				if strings.HasPrefix(text, "llgo ") {
					return &modfile.Compiler{
						Name:    "llgo",
						Version: strings.TrimSpace(text[5:]),
					}
				} else if strings.HasPrefix(text, "tinygo ") {
					return &modfile.Compiler{
						Name:    "tinygo",
						Version: strings.TrimSpace(text[7:]),
					}
				}
			}
		}
	}
	return nil
}

// -----------------------------------------------------------------------------
