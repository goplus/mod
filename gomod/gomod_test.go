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
	"testing"

	gomodfile "golang.org/x/mod/modfile"
)

// parseGoMod parses go.mod text, failing the test on error.
func parseGoMod(t *testing.T, text string) *gomodfile.File {
	t.Helper()
	f, err := gomodfile.Parse("go.mod", []byte(text), nil)
	if err != nil {
		t.Fatal("parseGoMod:", err)
	}
	return f
}

// requireOf returns the require for path, failing the test if there is none.
func requireOf(t *testing.T, f *gomodfile.File, path string) *gomodfile.Require {
	t.Helper()
	for _, r := range f.Require {
		if r.Mod.Path == path {
			return r
		}
	}
	t.Fatalf("require %s not found", path)
	return nil
}

func TestIsClass(t *testing.T) {
	f := parseGoMod(t, `
module example.com/foo

go 1.18

require (
	example.com/xgo v1.0.0 //xgo:class
	example.com/gop v1.0.0 //gop:class
	example.com/space v1.0.0 // xgo:class
	example.com/tab v1.0.0 //	gop:class
	example.com/extra v1.0.0 //xgo:class extra text
	example.com/other v1.0.0 // indirect
	example.com/plain v1.0.0
	example.com/mid v1.0.0 // see xgo:class
)
`)
	cases := []struct {
		path string
		want bool
	}{
		{"example.com/xgo", true},
		{"example.com/gop", true},
		{"example.com/space", true},
		{"example.com/tab", true},
		{"example.com/extra", true},
		{"example.com/other", false},
		{"example.com/plain", false},
		{"example.com/mid", false},
	}
	for _, c := range cases {
		if got := IsClass(requireOf(t, f, c.path)); got != c.want {
			t.Errorf("IsClass(%s) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestIsClassNoSyntax(t *testing.T) {
	r := &gomodfile.Require{}
	if IsClass(r) {
		t.Fatal("IsClass: require without syntax should not be a class module")
	}
}

func TestCompiler(t *testing.T) {
	cases := []struct {
		text string
		name string
		ver  string
	}{
		{"module foo\n\ngo 1.18 // llgo 0.9\n", "llgo", "0.9"},
		{"module foo\n\ngo 1.18 // tinygo 0.32\n", "tinygo", "0.32"},
		{"module foo\n\ngo 1.18 //llgo 0.9  \n", "llgo", "0.9"},
		{"module foo\n\ngo 1.18 //	tinygo  v0.32.0\n", "tinygo", "v0.32.0"},
		{"module foo\n\ngo 1.18\n", "", ""},
		{"module foo\n\ngo 1.18 // some note\n", "", ""},
		{"module foo\n\ngo 1.18 // llgo\n", "", ""}, // no version separator
		{"module foo\n\ngo 1.18 // gollgo 0.9\n", "", ""},
		{"module foo\n", "", ""}, // no go directive
	}
	for _, c := range cases {
		cl := Compiler(parseGoMod(t, c.text))
		if c.name == "" {
			if cl != nil {
				t.Errorf("Compiler(%q) = %+v, want nil", c.text, cl)
			}
			continue
		}
		if cl == nil || cl.Name != c.name || cl.Version != c.ver {
			t.Errorf("Compiler(%q) = %+v, want %s %s", c.text, cl, c.name, c.ver)
		}
	}
}

// TestCompilerFirstWins checks that the first matching comment on the go
// directive is used.
func TestCompilerFirstWins(t *testing.T) {
	f := parseGoMod(t, "module foo\n\ngo 1.18\n")
	f.Go.Syntax.Suffix = []gomodfile.Comment{
		{Token: "// note", Suffix: true},
		{Token: "// tinygo 0.32", Suffix: true},
		{Token: "// llgo 0.9", Suffix: true},
	}
	cl := Compiler(f)
	if cl == nil || cl.Name != "tinygo" || cl.Version != "0.32" {
		t.Fatalf("Compiler = %+v, want tinygo 0.32", cl)
	}
}
