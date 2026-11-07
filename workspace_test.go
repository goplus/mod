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

package mod

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/mod/module"
)

// loadWS writes files (slash-separated paths relative to a fresh temporary
// root) and loads the workspace described by "ws/go.work".
func loadWS(t *testing.T, files map[string]string) (root string, w *Workspace) {
	t.Helper()
	root = tempRoot(t)
	for rel, content := range files {
		writeFile(t, filepath.Join(root, filepath.FromSlash(rel)), content)
	}
	w, err := Load(filepath.Join(root, "ws", "go.work"))
	if err != nil {
		t.Fatal(err)
	}
	return root, w
}

// standard is a workspace whose go.work is not next to any of its modules'
// go.mod files, and one of them lives outside the go.work directory.
//
//	ws/go.work                     use ./app ../shared/lib
//	ws/app/go.mod                  example.com/app
//	ws/app/internal/nested/go.mod  example.com/app/nested  (not in use)
//	ws/other/go.mod                example.com/other       (not in use)
//	shared/lib/go.mod              example.com/lib
var standard = map[string]string{
	"ws/go.work":                    "go 1.22\n\nuse (\n\t./app\n\t../shared/lib\n)\n",
	"ws/app/go.mod":                 "module example.com/app\n\ngo 1.21\n",
	"ws/app/cmd/main.go":            "package main\n",
	"ws/app/internal/nested/go.mod": "module example.com/app/nested\n",
	"ws/other/go.mod":               "module example.com/other\n",
	"shared/lib/go.mod":             "module example.com/lib\n\ngo 1.20\n",
	"shared/lib/sub/x.go":           "package sub\n",
}

func TestLoad(t *testing.T) {
	root, w := loadWS(t, standard)
	if w.Path != filepath.Join(root, "ws", "go.work") || w.Dir != filepath.Join(root, "ws") {
		t.Errorf("Path/Dir = %q, %q", w.Path, w.Dir)
	}
	if got := w.GoVersion(); got != "1.22" {
		t.Errorf("GoVersion = %q", got)
	}
	if len(w.Modules) != 2 {
		t.Fatalf("got %d modules, want 2", len(w.Modules))
	}
	want := []struct{ path, dir, gover string }{
		{"example.com/app", filepath.Join(root, "ws", "app"), "1.21"},
		{"example.com/lib", filepath.Join(root, "shared", "lib"), "1.20"},
	}
	for i, x := range want {
		m := w.Modules[i]
		if m.Path != x.path || m.Dir != x.dir || m.GoVersion() != x.gover {
			t.Errorf("module %d = {%q %q %q}, want %+v", i, m.Path, m.Dir, m.GoVersion(), x)
		}
		if m.GoMod != filepath.Join(x.dir, "go.mod") {
			t.Errorf("module %d GoMod = %q", i, m.GoMod)
		}
	}
	if w.ModuleByPath("example.com/lib") != w.Modules[1] || w.ModuleByPath("example.com/none") != nil {
		t.Error("ModuleByPath is wrong")
	}
}

func TestLoadErrors(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{
			name:  "use directory without go.mod",
			files: map[string]string{"ws/go.work": "go 1.22\n\nuse ./missing\n"},
			want:  "use ./missing",
		},
		{
			name: "go.mod without module directive",
			files: map[string]string{
				"ws/go.work":  "go 1.22\n\nuse ./a\n",
				"ws/a/go.mod": "go 1.22\n",
			},
			want: "no module declaration",
		},
		{
			name:  "syntax error in go.work",
			files: map[string]string{"ws/go.work": "use (\n"},
			want:  "go.work",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := tempRoot(t)
			for rel, content := range c.files {
				writeFile(t, filepath.Join(root, filepath.FromSlash(rel)), content)
			}
			_, err := Load(filepath.Join(root, "ws", "go.work"))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want it to contain %q", err, c.want)
			}
		})
	}
	if _, err := Load(filepath.Join(tempRoot(t), "nope", "go.work")); err == nil {
		t.Error("loading a missing file should fail")
	}
}

func TestLoadModuleIsStrict(t *testing.T) {
	root := tempRoot(t)

	// Main-module-only directives are kept.
	writeFile(t, filepath.Join(root, "ok", "go.mod"),
		"module example.com/x\n\ngo 1.21\n\nreplace example.com/dep => ../dep\n")
	m, err := LoadModule(filepath.Join(root, "ok"))
	if err != nil {
		t.Fatal(err)
	}
	if m.Path != "example.com/x" || len(m.File.Replace) != 1 {
		t.Errorf("Path = %q, replaces = %d", m.Path, len(m.File.Replace))
	}

	// Unknown directives are an error, as they are for the go command.
	writeFile(t, filepath.Join(root, "bad", "go.mod"),
		"module example.com/y\n\nunknowndirective foo bar\n")
	if _, err := LoadModule(filepath.Join(root, "bad")); err == nil {
		t.Error("an unknown directive should be rejected")
	}
}

func TestParseKeepsRelativeUseBase(t *testing.T) {
	root := tempRoot(t)
	writeFile(t, filepath.Join(root, "ws", "a", "go.mod"), "module example.com/a\n")
	w, err := Parse(filepath.Join(root, "ws", "go.work"), []byte("go 1.22\nuse ./a\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Modules) != 1 || w.Modules[0].Dir != filepath.Join(root, "ws", "a") {
		t.Fatalf("modules = %+v", w.Modules)
	}
}

func TestModuleOf(t *testing.T) {
	root, w := loadWS(t, standard)
	cases := []struct {
		dir  string // relative to root
		want string // module path, "" for nil
	}{
		{"ws/app", "example.com/app"},
		{"ws/app/cmd", "example.com/app"},
		{"ws/app/does/not/exist", "example.com/app"},
		{"shared/lib", "example.com/lib"},
		{"shared/lib/sub", "example.com/lib"},
		{"ws/app/internal", "example.com/app"},
		{"ws/app/internal/nested", ""},   // nested module, not in use
		{"ws/app/internal/nested/x", ""}, // same
		{"ws/other", ""},                 // module, not in use
		{"ws/other/pkg", ""},
	}
	for _, c := range cases {
		m := w.ModuleOf(filepath.Join(root, filepath.FromSlash(c.dir)))
		got := ""
		if m != nil {
			got = m.Path
		}
		if got != c.want {
			t.Errorf("ModuleOf(%s) = %q, want %q", c.dir, got, c.want)
		}
	}
}

func TestResolve(t *testing.T) {
	root, w := loadWS(t, standard)
	cases := []struct {
		path    string
		wantMod string // "" means not resolved
		wantDir string // relative to root
	}{
		{"example.com/app", "example.com/app", "ws/app"},
		{"example.com/app/cmd", "example.com/app", "ws/app/cmd"},
		{"example.com/app/cmd/deep/er", "example.com/app", "ws/app/cmd/deep/er"},
		{"example.com/app/internal", "example.com/app", "ws/app/internal"},
		{"example.com/lib", "example.com/lib", "shared/lib"},
		{"example.com/lib/sub", "example.com/lib", "shared/lib/sub"},

		{"example.com/appx", "", ""},                  // not an element boundary
		{"example.com/app/internal/nested", "", ""},   // nested module, not in use
		{"example.com/app/internal/nested/y", "", ""}, // same
		{"example.com/other", "", ""},                 // not in use
		{"fmt", "", ""},
		{"github.com/foo/bar", "", ""},
		{"example.com/app/../x", "", ""},
		{"example.com/app/./x", "", ""},
		{"example.com/app//x", "", ""},
		{"example.com/app/", "", ""},
		{"", "", ""},
	}
	for _, c := range cases {
		m, dir, ok := w.Resolve(c.path)
		if c.wantMod == "" {
			if ok || m != nil || dir != "" {
				t.Errorf("Resolve(%q) = %v, %q, %v; want no match", c.path, m, dir, ok)
			}
			continue
		}
		wantDir := filepath.Join(root, filepath.FromSlash(c.wantDir))
		if !ok || m == nil || m.Path != c.wantMod || dir != wantDir {
			t.Errorf("Resolve(%q) = %v, %q, %v; want %s at %s", c.path, m, dir, ok, c.wantMod, wantDir)
		}
	}
}

func TestResolveLongestPrefix(t *testing.T) {
	root, w := loadWS(t, map[string]string{
		"ws/go.work":    "go 1.22\nuse (\n\t./a\n\t./a/b\n)\n",
		"ws/a/go.mod":   "module example.com/a\n",
		"ws/a/b/go.mod": "module example.com/a/b\n",
	})
	cases := []struct{ path, mod, dir string }{
		{"example.com/a/x", "example.com/a", "ws/a/x"},
		{"example.com/a/b", "example.com/a/b", "ws/a/b"},
		{"example.com/a/b/c", "example.com/a/b", "ws/a/b/c"},
	}
	for _, c := range cases {
		m, dir, ok := w.Resolve(c.path)
		if !ok || m.Path != c.mod || dir != filepath.Join(root, filepath.FromSlash(c.dir)) {
			t.Errorf("Resolve(%q) = %v, %q, %v; want %s", c.path, m, dir, ok, c.mod)
		}
	}
	// The directory walk agrees with the import path walk.
	if m := w.ModuleOf(filepath.Join(root, "ws", "a", "b", "c")); m == nil || m.Path != "example.com/a/b" {
		t.Errorf("ModuleOf picked %v", m)
	}
	if m := w.ModuleOf(filepath.Join(root, "ws", "a", "x")); m == nil || m.Path != "example.com/a" {
		t.Errorf("ModuleOf picked %v", m)
	}
}

func TestImportPath(t *testing.T) {
	root, w := loadWS(t, standard)
	cases := []struct {
		dir  string
		want string
		ok   bool
	}{
		{"ws/app", "example.com/app", true},
		{"ws/app/cmd", "example.com/app/cmd", true},
		{"shared/lib/sub", "example.com/lib/sub", true},
		{"ws/other", "", false},
		{"ws/app/internal/nested", "", false},
	}
	for _, c := range cases {
		got, ok := w.ImportPath(filepath.Join(root, filepath.FromSlash(c.dir)))
		if got != c.want || ok != c.ok {
			t.Errorf("ImportPath(%s) = %q, %v; want %q, %v", c.dir, got, ok, c.want, c.ok)
		}
	}
	// ImportPath and Resolve are inverse of each other.
	for _, d := range []string{"ws/app/cmd", "shared/lib/sub"} {
		dir := filepath.Join(root, filepath.FromSlash(d))
		ip, _ := w.ImportPath(dir)
		if _, back, ok := w.Resolve(ip); !ok || back != dir {
			t.Errorf("Resolve(ImportPath(%s)) = %q, %v", d, back, ok)
		}
	}
}

func TestCheckDir(t *testing.T) {
	root, w := loadWS(t, standard)
	p := func(rel string) string { return filepath.Join(root, filepath.FromSlash(rel)) }

	for _, ok := range []string{"ws/app", "ws/app/cmd", "shared/lib/sub"} {
		if err := w.CheckDir(p(ok)); err != nil {
			t.Errorf("CheckDir(%s) = %v, want nil", ok, err)
		}
	}
	bad := []struct{ dir, gomod string }{
		{"ws/other", "ws/other/go.mod"},
		{"ws/other/pkg", "ws/other/go.mod"},
		{"ws/app/internal/nested", "ws/app/internal/nested/go.mod"},
	}
	for _, c := range bad {
		err := w.CheckDir(p(c.dir))
		var nw *NotInWorkspaceError
		if !errors.As(err, &nw) {
			t.Errorf("CheckDir(%s) = %v, want *NotInWorkspaceError", c.dir, err)
			continue
		}
		if nw.GoMod != p(c.gomod) || nw.Dir != p(c.dir) {
			t.Errorf("CheckDir(%s): error = %+v", c.dir, nw)
		}
		if !strings.Contains(nw.Error(), "not one of the workspace modules") {
			t.Errorf("unexpected message %q", nw.Error())
		}
	}
}

func TestValidate(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		_, w := loadWS(t, standard)
		if err := w.Validate(); err != nil {
			t.Fatal(err)
		}
	})

	cases := []struct {
		name  string
		files map[string]string
		want  []string
	}{
		{
			name: "no modules",
			files: map[string]string{
				"ws/go.work": "go 1.22\n",
			},
			want: []string{"no modules were found"},
		},
		{
			name: "same module path twice",
			files: map[string]string{
				"ws/go.work":  "go 1.22\nuse (\n\t./a\n\t./b\n)\n",
				"ws/a/go.mod": "module example.com/dup\n",
				"ws/b/go.mod": "module example.com/dup\n",
			},
			want: []string{"module example.com/dup appears multiple times"},
		},
		{
			name: "same directory twice",
			files: map[string]string{
				"ws/go.work":  "go 1.22\nuse (\n\t./a\n\t./b/../a\n)\n",
				"ws/a/go.mod": "module example.com/a\n",
				"ws/b/x":      "",
			},
			want: []string{"listed more than once"},
		},
		{
			name: "module needs newer go",
			files: map[string]string{
				"ws/go.work":  "go 1.20\nuse ./a\n",
				"ws/a/go.mod": "module example.com/a\n\ngo 1.21\n",
			},
			want: []string{"module example.com/a listed in go.work file requires go >= 1.21, but go.work lists go 1.20"},
		},
		{
			name: "older go patch release is fine",
			files: map[string]string{
				"ws/go.work":  "go 1.21.3\nuse ./a\n",
				"ws/a/go.mod": "module example.com/a\n\ngo 1.21.0\n",
			},
		},
		{
			name: "conflicting replaces",
			files: map[string]string{
				"ws/go.work":  "go 1.22\nuse (\n\t./a\n\t./b\n)\n",
				"ws/a/go.mod": "module example.com/a\n\nreplace example.com/dep => ../dep1\n",
				"ws/b/go.mod": "module example.com/b\n\nreplace example.com/dep => ../dep2\n",
			},
			want: []string{"conflicting replacements for example.com/dep"},
		},
		{
			name: "several problems are reported together",
			files: map[string]string{
				"ws/go.work":  "go 1.20\nuse (\n\t./a\n\t./b\n)\n",
				"ws/a/go.mod": "module example.com/dup\n\ngo 1.21\n",
				"ws/b/go.mod": "module example.com/dup\n",
			},
			want: []string{"appears multiple times", "requires go >= 1.21"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, w := loadWS(t, c.files)
			err := w.Validate()
			if len(c.want) == 0 {
				if err != nil {
					t.Fatalf("Validate = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate = nil, want errors containing %q", c.want)
			}
			for _, s := range c.want {
				if !strings.Contains(err.Error(), s) {
					t.Errorf("Validate = %q, want it to contain %q", err, s)
				}
			}
		})
	}
}

func TestReplacements(t *testing.T) {
	dep := func(path, ver string) module.Version { return module.Version{Path: path, Version: ver} }

	t.Run("go.work wins over go.mod and ignores the version", func(t *testing.T) {
		root, w := loadWS(t, map[string]string{
			"ws/go.work": "go 1.22\nuse ./a\nreplace example.com/dep => ./local-dep\n",
			"ws/a/go.mod": "module example.com/a\n\n" +
				"replace example.com/dep v1.0.0 => example.com/fork v1.0.1\n" +
				"replace example.com/other => ../elsewhere\n",
		})
		got, err := w.Replacements()
		if err != nil {
			t.Fatal(err)
		}
		want := map[module.Version]module.Version{
			dep("example.com/dep", ""):   dep(filepath.Join(root, "ws", "local-dep"), ""),
			dep("example.com/other", ""): dep(filepath.Join(root, "ws", "elsewhere"), ""),
		}
		assertRepl(t, got, want)
	})

	t.Run("relative directories are resolved against the declaring file", func(t *testing.T) {
		root, w := loadWS(t, map[string]string{
			"ws/go.work":  "go 1.22\nuse (\n\t./a\n\t./b\n)\n",
			"ws/a/go.mod": "module example.com/a\n\nreplace example.com/x => ../x\n",
			"ws/b/go.mod": "module example.com/b\n\nreplace example.com/y => ./y\nreplace example.com/z v1.2.3 => example.com/zfork v1.2.4\n",
		})
		got, err := w.Replacements()
		if err != nil {
			t.Fatal(err)
		}
		want := map[module.Version]module.Version{
			dep("example.com/x", ""):       dep(filepath.Join(root, "ws", "x"), ""),
			dep("example.com/y", ""):       dep(filepath.Join(root, "ws", "b", "y"), ""),
			dep("example.com/z", "v1.2.3"): dep("example.com/zfork", "v1.2.4"),
		}
		assertRepl(t, got, want)
	})

	t.Run("same replacement in two modules is not a conflict", func(t *testing.T) {
		root, w := loadWS(t, map[string]string{
			"ws/go.work":  "go 1.22\nuse (\n\t./a\n\t./b\n)\n",
			"ws/a/go.mod": "module example.com/a\n\nreplace example.com/dep => ../dep\n",
			"ws/b/go.mod": "module example.com/b\n\nreplace example.com/dep => ../dep\n",
		})
		got, err := w.Replacements()
		if err != nil {
			t.Fatal(err)
		}
		assertRepl(t, got, map[module.Version]module.Version{
			dep("example.com/dep", ""): dep(filepath.Join(root, "ws", "dep"), ""),
		})
	})

	t.Run("conflict", func(t *testing.T) {
		_, w := loadWS(t, map[string]string{
			"ws/go.work":  "go 1.22\nuse (\n\t./a\n\t./b\n)\n",
			"ws/a/go.mod": "module example.com/a\n\nreplace example.com/dep v1.0.0 => example.com/forkA v1.0.0\n",
			"ws/b/go.mod": "module example.com/b\n\nreplace example.com/dep v1.0.0 => example.com/forkB v1.0.0\n",
		})
		_, err := w.Replacements()
		var ce *ReplaceConflictError
		if !errors.As(err, &ce) {
			t.Fatalf("err = %v, want *ReplaceConflictError", err)
		}
		if ce.Old != dep("example.com/dep", "v1.0.0") ||
			ce.A != dep("example.com/forkA", "v1.0.0") || ce.B != dep("example.com/forkB", "v1.0.0") {
			t.Errorf("conflict = %+v", ce)
		}
	})

	t.Run("conflict settled by go.work", func(t *testing.T) {
		_, w := loadWS(t, map[string]string{
			"ws/go.work":  "go 1.22\nuse (\n\t./a\n\t./b\n)\nreplace example.com/dep => example.com/winner v1.0.0\n",
			"ws/a/go.mod": "module example.com/a\n\nreplace example.com/dep => ../dep1\n",
			"ws/b/go.mod": "module example.com/b\n\nreplace example.com/dep => ../dep2\n",
		})
		got, err := w.Replacements()
		if err != nil {
			t.Fatal(err)
		}
		assertRepl(t, got, map[module.Version]module.Version{
			dep("example.com/dep", ""): dep("example.com/winner", "v1.0.0"),
		})
	})

	t.Run("the last replace within one go.mod wins", func(t *testing.T) {
		_, w := loadWS(t, map[string]string{
			"ws/go.work": "go 1.22\nuse ./a\n",
			"ws/a/go.mod": "module example.com/a\n\n" +
				"replace example.com/dep => example.com/one v1.0.0\n" +
				"replace example.com/dep => example.com/two v1.0.0\n",
		})
		got, err := w.Replacements()
		if err != nil {
			t.Fatal(err)
		}
		assertRepl(t, got, map[module.Version]module.Version{
			dep("example.com/dep", ""): dep("example.com/two", "v1.0.0"),
		})
	})
}

func assertRepl(t *testing.T, got, want map[module.Version]module.Version) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("got %d replacements %v, want %d %v", len(got), got, len(want), want)
	}
	for k, v := range want {
		if g, ok := got[k]; !ok || g != v {
			t.Errorf("replacement of %v = %v (present: %v), want %v", k, g, ok, v)
		}
	}
}
