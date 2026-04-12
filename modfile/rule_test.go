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
package modfile

import (
	"syscall"
	"testing"
)

// -----------------------------------------------------------------------------

var addParseExtTests = []struct {
	desc    string
	ext     string
	want    string
	wantF   string
	wantErr string
	isProj  bool
}{
	{
		"spx ok",
		".spx",
		".spx",
		".spx",
		"",
		false,
	},
	{
		"yap ok",
		"_yap.gox",
		"_yap.gox",
		"_yap.gox",
		"",
		false,
	},
	{
		"yap ok",
		"*_yap.gox",
		"_yap.gox",
		"*_yap.gox",
		"",
		false,
	},
	{
		"yap ok",
		"main_yap.gox",
		"_yap.gox",
		"main_yap.gox",
		"",
		true,
	},
	{
		"yap ok",
		"main_yap.gox",
		"",
		"",
		"ext main_yap.gox invalid: invalid ext format",
		false,
	},
	{
		"not a ext",
		"gmx",
		"",
		"",
		"ext gmx invalid: invalid ext format",
		false,
	},
}

func TestParseExt(t *testing.T) {
	if (&InvalidExtError{Err: syscall.EINVAL}).Unwrap() != syscall.EINVAL {
		t.Fatal("InvalidExtError.Unwrap failed")
	}
	if (&InvalidSymbolError{Err: syscall.EINVAL}).Unwrap() != syscall.EINVAL {
		t.Fatal("InvalidSymbolError.Unwrap failed")
	}
	for _, tt := range addParseExtTests {
		t.Run(tt.desc, func(t *testing.T) {
			ext, extF, err := parseExt(&tt.ext, tt.isProj)
			if err != nil {
				if err.Error() != tt.wantErr {
					t.Fatalf("wanterr: %s, but got: %s", tt.wantErr, err)
				}
			}
			if ext != tt.want || extF != tt.wantF {
				t.Fatalf("want: %s %s, but got: %s %s", tt.want, tt.wantF, ext, extF)
			}
		})
	}
}

func TestIsDirectoryPath(t *testing.T) {
	if !IsDirectoryPath("./...") {
		t.Fatal("IsDirectoryPath failed")
	}
}

func TestFormat(t *testing.T) {
	if b := Format(&FileSyntax{}); len(b) != 0 {
		t.Fatal("Format failed:", b)
	}
}

func TestForma2t(t *testing.T) {
	f := New("/foo/gox.mod", "1.2.0")
	if b := string(Format(f.Syntax)); b != "xgo 1.2.0\n" {
		t.Fatal("Format failed:", b)
	}
}

func TestMustQuote(t *testing.T) {
	if !MustQuote("") {
		t.Fatal("MustQuote failed")
	}
}

// -----------------------------------------------------------------------------

const goxmodWithPack = `
xgo 1.6

project main.spx Game github.com/goplus/spx/v2 math
class -embed *.spx SpriteImpl
pack assets index.json
`

func TestParsePack(t *testing.T) {
	f, err := ParseLax("gox.mod", []byte(goxmodWithPack), nil)
	if err != nil {
		t.Fatal("ParseLax failed:", err)
	}
	proj := f.proj()
	if proj == nil {
		t.Fatal("expected a project")
	}
	if proj.Pack == nil {
		t.Fatal("expected pack directive")
	}
	if proj.Pack.Directory != "assets" {
		t.Errorf("pack directory expected be assets, but %s got", proj.Pack.Directory)
	}
	if proj.Pack.IndexFile != "index.json" {
		t.Errorf("pack indexfile expected be index.json, but %s got", proj.Pack.IndexFile)
	}
}

const goxmodMultiProject = `
xgo 1.6

project main.spx Game github.com/goplus/spx/v2 math
class -embed *.spx SpriteImpl
pack assets index.json

project .yap YapApp github.com/goplus/yap
import "github.com/goplus/yap/test"
`

func TestParsePackMultiProject(t *testing.T) {
	f, err := ParseLax("gox.mod", []byte(goxmodMultiProject), nil)
	if err != nil {
		t.Fatal("ParseLax failed:", err)
	}
	if len(f.Projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(f.Projects))
	}
	// First project has pack
	if f.Projects[0].Pack == nil {
		t.Fatal("expected pack directive in first project")
	}
	if f.Projects[0].Pack.Directory != "assets" {
		t.Errorf("pack directory expected be assets, but %s got", f.Projects[0].Pack.Directory)
	}
	if f.Projects[0].Pack.IndexFile != "index.json" {
		t.Errorf("pack indexfile expected be index.json, but %s got", f.Projects[0].Pack.IndexFile)
	}
	// Second project has no pack
	if f.Projects[1].Pack != nil {
		t.Error("expected no pack directive in second project")
	}
}

const goxmodNoPack = `
xgo 1.6

project main.spx Game github.com/goplus/spx/v2 math
class -embed *.spx SpriteImpl
`

func TestParseNoPack(t *testing.T) {
	f, err := ParseLax("gox.mod", []byte(goxmodNoPack), nil)
	if err != nil {
		t.Fatal("ParseLax failed:", err)
	}
	if f.proj().Pack != nil {
		t.Error("expected no pack directive")
	}
}

func TestParsePackErr(t *testing.T) {
	// pack before project
	doTestParseErr(t, `gop.mod:2: pack must declare after a project definition`, `
pack assets index.json
`)
	// duplicate pack in same project
	doTestParseErr(t, `gop.mod:4: duplicate pack directive in the same project`, `
project github.com/goplus/spx math
pack assets index.json
pack assets2 index.yaml
`)
	// too few arguments
	doTestParseErr(t, `gop.mod:3: usage: pack <directory> <indexfile>`, `
project github.com/goplus/spx math
pack assets
`)
	// too many arguments
	doTestParseErr(t, `gop.mod:3: usage: pack <directory> <indexfile>`, `
project github.com/goplus/spx math
pack assets index.json extra
`)
	// ".." in directory
	doTestParseErr(t, `gop.mod:3: ".." is not allowed in pack directory`, `
project github.com/goplus/spx math
pack ../assets index.json
`)
	// path separator in indexfile
	doTestParseErr(t, `gop.mod:3: pack indexfile must be a plain file name without path separators`, `
project github.com/goplus/spx math
pack assets sub/index.json
`)
	// backslash path separator in indexfile
	doTestParseErr(t, `gop.mod:3: pack indexfile must be a plain file name without path separators`, `
project github.com/goplus/spx math
pack assets sub\index.json
`)
}

// -----------------------------------------------------------------------------
