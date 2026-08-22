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
	"strings"
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

func TestParseDriver(t *testing.T) {
	const src = `
xgo 1.6

project main.foo Game example.com/framework math
driver v1 example.com/framework/cmd/driver // driver
`
	f, err := ParseLax("gox.mod", []byte(src), nil)
	if err != nil {
		t.Fatal("ParseLax failed:", err)
	}
	proj := f.proj()
	if proj == nil || proj.Driver == nil {
		t.Fatal("expected driver")
	}
	if proj.Driver.Protocol != "v1" || proj.Driver.Package != "example.com/framework/cmd/driver" {
		t.Fatalf("driver = %#v", proj.Driver)
	}
	formatted := Format(f.Syntax)
	f2, err := ParseLax("gox.mod", formatted, nil)
	if err != nil {
		t.Fatal("round-trip ParseLax failed:", err)
	}
	if got := f2.proj().Driver; got == nil || got.Protocol != "v1" || got.Package != proj.Driver.Package {
		t.Fatalf("round-trip driver = %#v", got)
	}
}

func TestParseDriverErrors(t *testing.T) {
	tests := []struct {
		name string
		want string
		src  string
	}{
		{"before project", "driver must declare after a project definition", "driver v1 example.com/driver"},
		{"wrong arity", "usage: driver <protocol> <package>", "project example.com/app\ndriver v1"},
		{"invalid protocol", "driver protocol must match v[1-9][0-9]*", "project example.com/app\ndriver 1 example.com/driver"},
		{"zero protocol", "driver protocol must match v[1-9][0-9]*", "project example.com/app\ndriver v0 example.com/driver"},
		{"malformed protocol quote", "invalid syntax", "project example.com/app\ndriver \"bad\\q\" example.com/driver"},
		{"invalid package", "driver package", "project example.com/app\ndriver v1 ../driver"},
		{"malformed package quote", "invalid syntax", "project example.com/app\ndriver v1 \"bad\\q\""},
		{"duplicate", "duplicate driver directive in the same project", "project example.com/app\ndriver v1 example.com/driver\ndriver v1 example.com/driver"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, parse := range []func(string, []byte) (*File, error){
				func(name string, data []byte) (*File, error) { return Parse(name, data, nil) },
				func(name string, data []byte) (*File, error) { return ParseLax(name, data, nil) },
			} {
				_, err := parse("gox.mod", []byte(tt.src))
				if err == nil || !strings.Contains(err.Error(), tt.want) {
					t.Fatalf("error = %v, want substring %q", err, tt.want)
				}
			}
		})
	}
}

func TestParseDriverIsNotABlockDirective(t *testing.T) {
	_, err := ParseLax("gox.mod", []byte(`project example.com/app
driver (
v1 example.com/driver
)`), nil)
	if err == nil || !strings.Contains(err.Error(), "driver directive must not be a block") {
		t.Fatalf("error = %v", err)
	}
	_, err = ParseLax("gox.mod", []byte("project example.com/app\ndriver (\n)\n"), nil)
	if err == nil || !strings.Contains(err.Error(), "driver directive must not be a block") {
		t.Fatalf("empty block error = %v", err)
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
	doTestParseErr(t, `gop.mod:3: pack directory must be a relative path and ".." is not allowed`, `
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

const goxmodWithAutoLambda = `
xgo 1.6

project main.spx Game github.com/goplus/spx/v2 math
class -embed *.spx SpriteImpl
autolambda times(1), forEver(0), onKey(1)
`

func TestParseAutoLambda(t *testing.T) {
	f, err := ParseLax("gox.mod", []byte(goxmodWithAutoLambda), nil)
	if err != nil {
		t.Fatal("ParseLax failed:", err)
	}
	proj := f.proj()
	if proj == nil {
		t.Fatal("expected a project")
	}
	want := map[string]int{"times": 1, "forEver": 0, "onKey": 1}
	if len(proj.AutoLambdas) != len(want) {
		t.Fatalf("expected %d autolambda entries, got %d: %v", len(want), len(proj.AutoLambdas), proj.AutoLambdas)
	}
	for name, n := range want {
		if got, ok := proj.AutoLambdas[name]; !ok || got != n {
			t.Errorf("autolambda[%s] expected %d, got %d (ok=%v)", name, n, got, ok)
		}
	}
}

const goxmodMultiAutoLambda = `
xgo 1.6

project main.spx Game github.com/goplus/spx/v2 math
class -embed *.spx SpriteImpl
autolambda times(1)
autolambda forEver(0), onKey(1)
`

func TestParseMultiAutoLambda(t *testing.T) {
	f, err := ParseLax("gox.mod", []byte(goxmodMultiAutoLambda), nil)
	if err != nil {
		t.Fatal("ParseLax failed:", err)
	}
	proj := f.proj()
	if proj == nil {
		t.Fatal("expected a project")
	}
	want := map[string]int{"times": 1, "forEver": 0, "onKey": 1}
	if len(proj.AutoLambdas) != len(want) {
		t.Fatalf("expected %d autolambda entries, got %d: %v", len(want), len(proj.AutoLambdas), proj.AutoLambdas)
	}
	for name, n := range want {
		if got, ok := proj.AutoLambdas[name]; !ok || got != n {
			t.Errorf("autolambda[%s] expected %d, got %d (ok=%v)", name, n, got, ok)
		}
	}
}

const goxmodNoAutoLambda = `
xgo 1.6

project main.spx Game github.com/goplus/spx/v2 math
class -embed *.spx SpriteImpl
`

func TestParseNoAutoLambda(t *testing.T) {
	f, err := ParseLax("gox.mod", []byte(goxmodNoAutoLambda), nil)
	if err != nil {
		t.Fatal("ParseLax failed:", err)
	}
	if f.proj().AutoLambdas != nil {
		t.Error("expected no autolambda directive")
	}
}

func TestParseAutoLambdaErr(t *testing.T) {
	// autolambda before project
	doTestParseErr(t, `gop.mod:2: autolambda must declare after a project definition`, `
autolambda times(1)
`)
	// missing arguments
	doTestParseErr(t, `gop.mod:3: usage: autolambda name(n), ...`, `
project github.com/goplus/spx math
autolambda
`)
	// missing '('
	doTestParseErr(t, `gop.mod:3: autolambda times: expect '(' after command name`, `
project github.com/goplus/spx math
autolambda times 1)
`)
	// invalid number
	doTestParseErr(t, `gop.mod:3: autolambda times: invalid number of arguments "x"`, `
project github.com/goplus/spx math
autolambda times(x)
`)
	// missing ')'
	doTestParseErr(t, `gop.mod:3: autolambda times: expect ')' after number of arguments`, `
project github.com/goplus/spx math
autolambda times(1
`)
	// missing ',' between entries
	doTestParseErr(t, `gop.mod:3: autolambda: expect ',' between entries, got "forEver"`, `
project github.com/goplus/spx math
autolambda times(1) forEver(0)
`)
	// trailing ','
	doTestParseErr(t, `gop.mod:3: autolambda: trailing ',' without an entry`, `
project github.com/goplus/spx math
autolambda times(1),
`)
	// invalid command name
	doTestParseErr(t, `gop.mod:3: autolambda: invalid command name "!"`, `
project github.com/goplus/spx math
autolambda !(0)
`)
	// invalid command name
	doTestParseErr(t, `gop.mod:3: autolambda: invalid command name "'"`, `
project github.com/goplus/spx math
autolambda '
`)
	// duplicate command
	doTestParseErr(t, `gop.mod:3: autolambda: duplicate command "times"`, `
project github.com/goplus/spx math
autolambda times(1), times(2)
`)
}

// -----------------------------------------------------------------------------
