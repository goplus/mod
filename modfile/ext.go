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

package modfile

import (
	"fmt"
	"path"
	"strings"

	"github.com/qiniu/x/errors"
)

// can be "_[class].gox", "*_[class].gox", ".[class]" or "*.[class]"
// can be "main_[class].gox", "main.[class]" if isProj is true
func isExt(s string, isProj bool) bool {
	return len(s) > 1 && (s[0] == '*' || s[0] == '_' || s[0] == '.') ||
		isProj && len(s) > 4 && s[:4] == "main" && (s[4] == '_' || s[4] == '.')
}

func getExt(s string) string {
	if len(s) > 1 && s[0] == '*' {
		return s[1:]
	}
	if len(s) > 4 && s[:4] == "main" {
		return s[4:]
	}
	return s
}

func parseExt(s *string, isProj bool) (ext, fullExt string, err error) {
	t, err := parseString(s)
	if err != nil {
		goto failed
	}
	if isExt(t, isProj) {
		return getExt(t), t, nil
	}
	err = errors.New("invalid ext format")
failed:
	return "", "", &InvalidExtError{
		Ext: *s,
		Err: err,
	}
}

type InvalidExtError struct {
	Ext string
	Err error
}

func (e *InvalidExtError) Error() string {
	return fmt.Sprintf("ext %s invalid: %s", e.Ext, e.Err)
}

func (e *InvalidExtError) Unwrap() error { return e.Err }

// SplitFname splits fname into (className, classExt).
func SplitFname(fname string) (className, classExt string) {
	classExt = path.Ext(fname)
	className = fname[:len(fname)-len(classExt)]
	if hasGoxExt := (classExt == ".gox"); hasGoxExt {
		if n := strings.LastIndexByte(className, '_'); n > 0 {
			className, classExt = fname[:n], fname[n:]
		}
	}
	return
}

// ClassExt returns classExt of specified fname.
func ClassExt(fname string) string {
	_, ext := SplitFname(fname)
	return ext
}
