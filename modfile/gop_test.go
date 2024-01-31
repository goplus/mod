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
	"testing"

	"github.com/qiniu/x/errors"
	"golang.org/x/mod/modfile"
)

func TestClassKind(t *testing.T) {
	type testCase struct {
		fname  string
		isProj bool
	}
	cases := []testCase{
		{"foo.gmx", true},
		{"foo.spx", false},
		{"foo_spx.gox", false},
		{"main_spx.gox", true},
	}
	for _, c := range cases {
		ext := ClassExt(c.fname)
		proj, ok := lookupClass(ext)
		if !ok {
			t.Fatal("TestClassKind: unkown ext -", c.fname)
		}
		if isProj := proj.IsProj(ext, c.fname); isProj != c.isProj {
			t.Fatalf("proj.IsProj(%s, %s) => %v", ext, c.fname, isProj)
		}
	}
}

func lookupClass(ext string) (c *Project, ok bool) {
	switch ext {
	case ".gmx", ".spx":
		return &Project{
			Ext: ".gmx", Class: "*MyGame",
			Works:    []*Class{{Ext: ".spx", Class: "Sprite"}},
			PkgPaths: []string{"github.com/goplus/gop/cl/internal/spx", "math"}}, true
	case "_spx.gox":
		return &Project{
			Ext: "_spx.gox", Class: "Game",
			Works:    []*Class{{Ext: "_spx.gox", Class: "Sprite"}},
			PkgPaths: []string{"github.com/goplus/gop/cl/internal/spx3", "math"}}, true
	}
	return
}

// -----------------------------------------------------------------------------

const gopmodSpx1 = `
module spx

go 1.17
gop 1.1

project .gmx Game github.com/goplus/spx math
class .spx Sprite
class .spx2 *Sprite2

require (
	github.com/ajstarks/svgo v0.0.0-20210927141636-6d70534b1098
)
`

func TestGoModCompat1(t *testing.T) {
	const (
		gopmod = gopmodSpx1
	)
	f, err := modfile.ParseLax("go.mod", []byte(gopmod), nil)
	if err != nil || len(f.Syntax.Stmt) != 7 {
		t.Fatal("modfile.ParseLax failed:", f, err)
	}

	gop := f.Syntax.Stmt[2].(*modfile.Line)
	if len(gop.Token) != 2 || gop.Token[0] != "gop" || gop.Token[1] != "1.1" {
		t.Fatal("modfile.ParseLax gop:", gop)
	}

	require := f.Syntax.Stmt[6].(*modfile.LineBlock)
	if len(require.Token) != 1 || require.Token[0] != "require" {
		t.Fatal("modfile.ParseLax require:", require)
	}
	if len(require.Line) != 1 {
		t.Fatal("modfile.ParseLax require.Line:", require.Line)
	}
}

// -----------------------------------------------------------------------------

func TestParse1(t *testing.T) {
	const (
		gopmod = gopmodSpx1
	)
	f, err := ParseLax("github.com/goplus/gop/gop.mod", []byte(gopmod), nil)
	if err != nil {
		t.Error(err)
		return
	}
	if f.Gop.Version != "1.1" {
		t.Errorf("gop version expected be 1.1, but %s got", f.Gop.Version)
	}
	if f.proj().Ext != ".gmx" {
		t.Errorf("project exts expected be .gmx, but %s got", f.proj().Ext)
	}
	if f.proj().Class != "Game" {
		t.Errorf("project class expected be Game, but %s got", f.proj().Class)
	}

	if len(f.proj().PkgPaths) != 2 {
		t.Errorf("project pkgpaths length expected be 2, but %d got", len(f.proj().PkgPaths))
	}

	if f.proj().PkgPaths[0] != "github.com/goplus/spx" {
		t.Errorf("project path expected be github.com/goplus/spx, but %s got", f.proj().PkgPaths[0])
	}
	if f.proj().PkgPaths[1] != "math" {
		t.Errorf("project path expected be math, but %s got", f.proj().PkgPaths[1])
	}

	if len(f.proj().Works) != 2 {
		t.Errorf("project workclass length expected be 2, but %d got", len(f.proj().Works))
	}
	if f.proj().Works[0].Ext != ".spx" {
		t.Errorf("project class[0] exts expected be .spx, but %s got", f.proj().Works[0].Ext)
	}
	if f.proj().Works[0].Class != "Sprite" {
		t.Errorf("project class[0] class expected be Sprite, but %s got", f.proj().Works[0].Class)
	}
	if f.proj().Works[1].Ext != ".spx2" {
		t.Errorf("project class[1] exts expected be .spx2, but %s got", f.proj().Works[1].Ext)
	}
	if f.proj().Works[1].Class != "*Sprite2" {
		t.Errorf("project class[1] class expected be Sprite, but %s got", f.proj().Works[1].Class)
	}
}

// -----------------------------------------------------------------------------

const gopmodSpx2 = `
module spx

go 1.17
gop 1.1

project github.com/goplus/spx math
class .spx Sprite

require (
	github.com/ajstarks/svgo v0.0.0-20210927141636-6d70534b1098
)
`

func TestGoModCompat2(t *testing.T) {
	const (
		gopmod = gopmodSpx2
	)
	f, err := modfile.ParseLax("go.mod", []byte(gopmod), nil)
	if err != nil || len(f.Syntax.Stmt) != 6 {
		t.Fatal("modfile.ParseLax failed:", f, err)
	}

	gop := f.Syntax.Stmt[2].(*modfile.Line)
	if len(gop.Token) != 2 || gop.Token[0] != "gop" || gop.Token[1] != "1.1" {
		t.Fatal("modfile.ParseLax gop:", gop)
	}

	require := f.Syntax.Stmt[5].(*modfile.LineBlock)
	if len(require.Token) != 1 || require.Token[0] != "require" {
		t.Fatal("modfile.ParseLax require:", require)
	}
	if len(require.Line) != 1 {
		t.Fatal("modfile.ParseLax require.Line:", require.Line)
	}
}

/* TODO
func TestGoModStd(t *testing.T) {
	const (
		gopmod = "module std\n"
	)
	f, err := ParseLax("go.mod", []byte(gopmod), nil)
	if err != nil {
		t.Fatal("modfile.ParseLax failed:", err)
	}
	if f.Module.Mod.Path != "" {
		t.Fatal("modfile.ParseLax:", f.Module.Mod.Path)
	}
}
*/

// -----------------------------------------------------------------------------

func TestParse2(t *testing.T) {
	const (
		gopmod = gopmodSpx2
	)
	f, err := ParseLax("github.com/goplus/gop/gop.mod", []byte(gopmod), nil)
	if err != nil {
		t.Error(err)
		return
	}
	if f.Gop.Version != "1.1" {
		t.Errorf("gop version expected be 1.1, but %s got", f.Gop.Version)
	}
	if f.proj().Ext != "" {
		t.Errorf("project exts expected be .gmx, but %s got", f.proj().Ext)
	}
	if f.proj().Class != "" {
		t.Errorf("project class expected be Game, but %s got", f.proj().Class)
	}

	if len(f.proj().PkgPaths) != 2 {
		t.Errorf("project pkgpaths length expected be 2, but %d got", len(f.proj().PkgPaths))
	}

	if f.proj().PkgPaths[0] != "github.com/goplus/spx" {
		t.Errorf("project path expected be github.com/goplus/spx, but %s got", f.proj().PkgPaths[0])
	}
	if f.proj().PkgPaths[1] != "math" {
		t.Errorf("project path expected be math, but %s got", f.proj().PkgPaths[1])
	}

	if len(f.proj().Works) != 1 {
		t.Errorf("project workclass length expected be 2, but %d got", len(f.proj().Works))
	}
	if f.proj().Works[0].Ext != ".spx" {
		t.Errorf("project class[0] exts expected be .spx, but %s got", f.proj().Works[0].Ext)
	}
	if f.proj().Works[0].Class != "Sprite" {
		t.Errorf("project class[0] class expected be Sprite, but %s got", f.proj().Works[0].Class)
	}
}

func TestParseErr(t *testing.T) {
	doTestParseErr(t, `gop.mod:2: unknown directive: module`, `
module foo
`)
	doTestParseErr(t, `gop.mod:2:9: unexpected newline in string`, `
foo "foo
`)
	doTestParseErr(t, `gop.mod:3: repeated gop statement`, `
gop 1.1
gop 1.2
`)
	doTestParseErr(t, `gop.mod:2: gop directive expects exactly one argument`, `
gop 1.1 1.2
`)
	doTestParseErr(t, `gop.mod:2: invalid gop version '1.x': must match format 1.23`, `
gop 1.x
`)
	doTestParseErr(t, `gop.mod:2: usage: project [.projExt ProjClass] classFilePkgPath ...`, `
project
`)
	doTestParseErr(t, `gop.mod:2: usage: project [.projExt ProjClass] classFilePkgPath ...`, `
project .gmx Game
`)
	doTestParseErr(t, `gop.mod:2: ext ." invalid: unquoted string cannot contain quote`, `
project ." Game math
`)
	doTestParseErr(t, `gop.mod:2: "." is not a valid package path`, `
project . Game math
`)
	doTestParseErr(t, `gop.mod:2: symbol game invalid: invalid Go export symbol format`, `
project .gmx game math
`)
	doTestParseErr(t, `gop.mod:2: symbol . invalid: invalid Go export symbol format`, `
project .gmx . math
`)
	doTestParseErr(t, `gop.mod:2: invalid quoted string: invalid syntax`, `
project .123 Game "\?"
`)
	doTestParseErr(t, `gop.mod:2: invalid quoted string: invalid syntax`, `
project "\?"
`)
	doTestParseErr(t, `gop.mod:2: work class must declare after a project definition`, `
class .spx Sprite
`)
	doTestParseErr(t, `gop.mod:3: usage: class .workExt WorkClass`, `
project github.com/goplus/spx math
class .spx
`)
	doTestParseErr(t, `gop.mod:3: ext . invalid: invalid ext format`, `
project github.com/goplus/spx math
class . Sprite
`)
	doTestParseErr(t, `gop.mod:3: symbol S"prite invalid: unquoted string cannot contain quote`, `
project github.com/goplus/spx math
class .spx S"prite
`)
	doTestParseErr(t, `gop.mod:3: ext ."spx invalid: unquoted string cannot contain quote`, `
project github.com/goplus/spx math
class ."spx Sprite
`)
	doTestParseErr(t, `gop.mod:3: symbol sprite invalid: invalid Go export symbol format`, `
project github.com/goplus/spx math
class .spx sprite
`)
	doTestParseErr(t, `gop.mod:3: usage: import [name] pkgPath`, `
project github.com/goplus/spx math
import
`)
	doTestParseErr(t, `gop.mod:3: invalid quoted string: invalid syntax`, `
project github.com/goplus/spx math
import "\?"
`)
	doTestParseErr(t, `gop.mod:3: invalid syntax`, `
project github.com/goplus/spx math
import "\?" math
`)
	doTestParseErr(t, `gop.mod:2: import must declare after a project definition`, `
import math
`)
	doTestParseErr(t, `gop.mod:2: unknown directive: unknown`, `
unknown .spx
`)
}

func doTestParseErr(t *testing.T, errMsg string, gopmod string) {
	t.Helper()
	// t.Run(errMsg, func(t *testing.T) {
	_, err := Parse("gop.mod", []byte(gopmod), nil)
	if err == nil || err.Error() == "" {
		t.Fatal("Parse: no error?")
		return
	}
	if errRet := errors.Summary(err); errRet != errMsg {
		t.Error("Parse got:", errRet, "\nExpected:", errMsg)
	}
	// })
}

// -----------------------------------------------------------------------------
