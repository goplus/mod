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

const (
	defaultXGoVer = "1.5"
)

// LoadModfile loads a gox.mod file. gomod can be nil.
func LoadModfile(gomod *gomodfile.File, goxmod string) (ret *modfile.File, err error) {
	return LoadModfileEx(gomod, goxmod, os.ReadFile)
}

func LoadModfileEx(gomod *gomodfile.File, goxmod string, readFile func(string) ([]byte, error)) (ret *modfile.File, err error) {
	data, err := readFile(goxmod)
	if err != nil {
		const goxmodSuffix = "gox.mod" // fallback to gop.mod
		if strings.HasSuffix(goxmod, goxmodSuffix) {
			goxmod = goxmod[:len(goxmod)-len(goxmodSuffix)] + "gop.mod"
			data, err = readFile(goxmod)
		}
	}
	if err == nil {
		if ret, err = modfile.ParseLax(goxmod, data, nil); err != nil {
			return
		}
	} else {
		ret, err = newMod(goxmod, defaultXGoVer), nil
	}
	if gomod != nil {
		importClassfileFromGoMod(ret, gomod)
		if cl := getGoCompiler(gomod); cl != nil {
			ret.Compiler = cl
		}
	}
	return
}

func newMod(goxmod, xgoVer string) *modfile.File {
	return modfile.New(goxmod, xgoVer)
}

/*
go 1.18 // llgo 0.9
go 1.18 // tinygo 0.32
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

func importClassfileFromGoMod(ret *modfile.File, f *gomodfile.File) {
	for _, r := range f.Require {
		if isClass(r) {
			ret.ClassMods = append(ret.ClassMods, r.Mod.Path)
		}
	}
}

func isClass(r *gomodfile.Require) bool {
	if line := r.Syntax; line != nil {
		for _, c := range line.Suffix {
			text := strings.TrimLeft(c.Token[2:], " \t") // TODO(xsw): no space before xgo:class
			if strings.HasPrefix(text, "xgo:class") || strings.HasPrefix(text, "gop:class") {
				return true
			}
		}
	}
	return false
}
