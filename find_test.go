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
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// tempRoot returns a temporary directory whose path contains no symlinks.
func tempRoot(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// confined returns a Stat function that only sees files below root, so that a
// stray go.mod or go.work above the temporary directory cannot affect a test.
func confined(root string) func(string) (fs.FileInfo, error) {
	return func(name string) (fs.FileInfo, error) {
		if name != root && !strings.HasPrefix(name, root+string(filepath.Separator)) {
			return nil, fs.ErrNotExist
		}
		return os.Stat(name)
	}
}

func finderFor(root string, env map[string]string) Finder {
	return Finder{
		Stat:   confined(root),
		Getenv: func(k string) string { return env[k] },
	}
}

func mkdir(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestFind(t *testing.T) {
	type tc struct {
		name   string
		files  []string // relative to the root
		dirs   []string // directories that must exist (also relative)
		from   string   // relative to the root
		work   string   // expected, relative to the root ("" for none)
		mod    string
		wantNF bool
	}
	tests := []tc{
		{
			name:  "work above module",
			files: []string{"go.work", "app/go.mod"},
			dirs:  []string{"app/cmd"},
			from:  "app/cmd",
			work:  "go.work", // go.mod is deliberately not reported
		},
		{
			name:  "work beside module",
			files: []string{"go.work", "go.mod"},
			from:  ".",
			work:  "go.work",
		},
		{
			name:  "module only",
			files: []string{"go.mod"},
			dirs:  []string{"a/b"},
			from:  "a/b",
			mod:   "go.mod",
		},
		{
			name:  "nearest module wins",
			files: []string{"go.mod", "a/go.mod"},
			dirs:  []string{"a/b"},
			from:  "a/b",
			mod:   "a/go.mod",
		},
		{
			name:  "work below module is still found",
			files: []string{"go.mod", "sub/go.work"},
			dirs:  []string{"sub/x"},
			from:  "sub/x",
			work:  "sub/go.work",
		},
		{
			name:  "nearest work wins",
			files: []string{"go.work", "inner/go.work"},
			dirs:  []string{"inner/x"},
			from:  "inner/x",
			work:  "inner/go.work",
		},
		{
			name:  "work and module in different branches",
			files: []string{"w/go.work", "m/go.mod"},
			from:  "m",
			mod:   "m/go.mod", // a go.work elsewhere is not found by walking up
		},
		{
			name:   "nothing",
			dirs:   []string{"a/b"},
			from:   "a/b",
			wantNF: true,
		},
	}
	for _, c := range tests {
		t.Run(c.name, func(t *testing.T) {
			root := tempRoot(t)
			for _, f := range c.files {
				writeFile(t, filepath.Join(root, filepath.FromSlash(f)), "")
			}
			for _, d := range c.dirs {
				mkdir(t, filepath.Join(root, filepath.FromSlash(d)))
			}
			from := filepath.Join(root, filepath.FromSlash(c.from))
			gowork, gomod, err := finderFor(root, nil).Find(from)
			abs := func(rel string) string {
				if rel == "" {
					return ""
				}
				return filepath.Join(root, filepath.FromSlash(rel))
			}
			if c.wantNF {
				if !errors.Is(err, ErrNoModule) {
					t.Fatalf("err = %v, want ErrNoModule", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if want := abs(c.work); gowork != want {
				t.Errorf("gowork = %q, want %q", gowork, want)
			}
			if want := abs(c.mod); gomod != want {
				t.Errorf("gomod = %q, want %q", gomod, want)
			}
		})
	}
}

func TestFindIgnoresDirectories(t *testing.T) {
	root := tempRoot(t)
	mkdir(t, filepath.Join(root, "go.work"))
	mkdir(t, filepath.Join(root, "go.mod"))
	_, _, err := finderFor(root, nil).Find(root)
	if !errors.Is(err, ErrNoModule) {
		t.Fatalf("err = %v, want ErrNoModule", err)
	}
}

func TestFindReachesRoot(t *testing.T) {
	// A file system with no files: the walk must terminate at the root.
	f := Finder{Stat: func(string) (fs.FileInfo, error) { return nil, fs.ErrNotExist }}
	for _, from := range []string{string(filepath.Separator), "", "a/b/c"} {
		if _, _, err := f.Find(from); !errors.Is(err, ErrNoModule) {
			t.Errorf("Find(%q): err = %v, want ErrNoModule", from, err)
		}
	}
}

func TestFindChecksRoot(t *testing.T) {
	// The root directory itself must be examined.
	vol := filepath.VolumeName(os.TempDir())
	rootDir := vol + string(filepath.Separator)
	want := filepath.Join(rootDir, goworkFile)
	f := Finder{Stat: func(name string) (fs.FileInfo, error) {
		if name == want {
			return os.Stat(os.Args[0]) // any regular file
		}
		return nil, fs.ErrNotExist
	}}
	gowork, _, err := f.Find(filepath.Join(rootDir, "a", "b"))
	if err != nil || gowork != want {
		t.Fatalf("Find = %q, %v; want %q", gowork, err, want)
	}
}

func TestFindEmptyDirFromIsCwd(t *testing.T) {
	root := tempRoot(t)
	writeFile(t, filepath.Join(root, "app", "go.mod"), "")
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(filepath.Join(root, "app")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(old) })

	_, gomod, err := finderFor(root, nil).Find("")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "app", "go.mod"); gomod != want {
		t.Errorf("gomod = %q, want %q", gomod, want)
	}
}

func TestFindGoMod(t *testing.T) {
	root := tempRoot(t)
	writeFile(t, filepath.Join(root, "go.work"), "")
	writeFile(t, filepath.Join(root, "app", "go.mod"), "")
	mkdir(t, filepath.Join(root, "app", "x"))
	f := finderFor(root, nil)

	got, err := f.FindGoMod(filepath.Join(root, "app", "x"))
	if err != nil || got != filepath.Join(root, "app", "go.mod") {
		t.Fatalf("FindGoMod = %q, %v", got, err)
	}
	if _, err := f.FindGoMod(root); !errors.Is(err, ErrNoModule) {
		t.Fatalf("err = %v, want ErrNoModule", err)
	}
}

func TestDiscover(t *testing.T) {
	root := tempRoot(t)
	writeFile(t, filepath.Join(root, "ws", "go.work"), "")
	writeFile(t, filepath.Join(root, "ws", "app", "go.mod"), "")
	writeFile(t, filepath.Join(root, "other", "go.work"), "")
	mkdir(t, filepath.Join(root, "ws", "app", "cmd"))
	mkdir(t, filepath.Join(root, "lone"))
	writeFile(t, filepath.Join(root, "lone", "go.mod"), "")
	from := filepath.Join(root, "ws", "app", "cmd")

	t.Run("unset behaves like Find", func(t *testing.T) {
		gowork, gomod, err := finderFor(root, nil).Discover(from)
		if err != nil || gowork != filepath.Join(root, "ws", "go.work") || gomod != "" {
			t.Fatalf("got %q, %q, %v", gowork, gomod, err)
		}
	})
	t.Run("off", func(t *testing.T) {
		env := map[string]string{"GOWORK": "off"}
		gowork, gomod, err := finderFor(root, env).Discover(from)
		if err != nil || gowork != "" || gomod != filepath.Join(root, "ws", "app", "go.mod") {
			t.Fatalf("got %q, %q, %v", gowork, gomod, err)
		}
	})
	t.Run("off without go.mod", func(t *testing.T) {
		env := map[string]string{"GOWORK": "off"}
		_, _, err := finderFor(root, env).Discover(filepath.Join(root, "ws"))
		if !errors.Is(err, ErrNoModule) {
			t.Fatalf("err = %v, want ErrNoModule", err)
		}
	})
	t.Run("explicit file", func(t *testing.T) {
		want := filepath.Join(root, "other", "go.work")
		env := map[string]string{"GOWORK": want}
		gowork, gomod, err := finderFor(root, env).Discover(filepath.Join(root, "lone"))
		if err != nil || gowork != want || gomod != "" {
			t.Fatalf("got %q, %q, %v", gowork, gomod, err)
		}
	})
	t.Run("explicit file missing", func(t *testing.T) {
		env := map[string]string{"GOWORK": filepath.Join(root, "nope", "go.work")}
		if _, _, err := finderFor(root, env).Discover(from); err == nil {
			t.Fatal("want an error for a missing GOWORK file")
		}
	})
}
