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
	"strings"

	"github.com/goplus/mod/modfile"
	gomodfile "golang.org/x/mod/modfile"
)

// -----------------------------------------------------------------------------

// IsClass reports whether a require is marked as a class module by a trailing
// //xgo:class (or the legacy //gop:class) comment. The marker is matched after
// leading whitespace is trimmed, as a prefix.
func IsClass(r *gomodfile.Require) bool {
	if line := r.Syntax; line != nil {
		for _, c := range line.Suffix {
			text := c.Token[2:]
			if strings.HasPrefix(text, "xgo:class") || strings.HasPrefix(text, "gop:class") {
				return true
			}
		}
	}
	return false
}

// -----------------------------------------------------------------------------

/*
Compiler reads a trailing comment on the go directive that names a custom
compiler, for example:

	go 1.18 // llgo 0.9
	go 1.18 // tinygo 0.32

The first matching comment wins. It returns nil if there is none.
*/
func Compiler(f *gomodfile.File) *modfile.Compiler {
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
