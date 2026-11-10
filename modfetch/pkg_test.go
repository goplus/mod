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

package modfetch

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestIsStdlibPath(t *testing.T) {
	for _, c := range []struct {
		path string
		want bool
	}{
		{"fmt", true},
		{"net/http", true},
		{"internal/abi", true},
		{"github.com/foo/bar", false},
		{"gopkg.in/yaml.v3", false},
		{"example.com", false},
	} {
		if got := isStdlibPath(c.path); got != c.want {
			t.Errorf("isStdlibPath(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestModPathPrefixes(t *testing.T) {
	got := modPathPrefixes("github.com/foo/bar/baz/qux")
	want := []string{
		"github.com/foo/bar/baz/qux",
		"github.com/foo/bar/baz",
		"github.com/foo/bar",
		"github.com/foo",
		"github.com",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("modPathPrefixes = %v, want %v", got, want)
	}
	if got := modPathPrefixes("single"); !reflect.DeepEqual(got, []string{"single"}) {
		t.Fatalf("modPathPrefixes(single) = %v", got)
	}
}

func TestValidModulePrefix(t *testing.T) {
	for _, c := range []struct {
		path string
		want bool
	}{
		{"github.com/foo/bar", true},
		{"github.com/foo", true}, // host/user is syntactically plausible
		{"github.com", false},    // bare host, no element after it
		{"foo/bar", false},       // first element is not a host (no dot)
		{"", false},
		{"github.com/foo//bar", false}, // invalid module path (double slash)
	} {
		if got := validModulePrefix(c.path); got != c.want {
			t.Errorf("validModulePrefix(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestRelToModRoot(t *testing.T) {
	for _, c := range []struct {
		pkg, mod, want string
	}{
		{"github.com/foo/bar", "github.com/foo/bar", ""},
		{"github.com/foo/bar/baz/qux", "github.com/foo/bar", "baz/qux"},
		{"github.com/foo/bar/v2/sub", "github.com/foo/bar/v2", "sub"},
	} {
		if got := relToModRoot(c.pkg, c.mod); got != c.want {
			t.Errorf("relToModRoot(%q, %q) = %q, want %q", c.pkg, c.mod, got, c.want)
		}
	}
}

func TestIsNotFoundOutput(t *testing.T) {
	notFound := []string{
		`reading https://proxy.golang.org/x/@v/list: 404 Not Found`,
		`reading https://proxy.golang.org/x/@v/list: 410 Gone`,
		`server response: not found: module github.com/foo/bar`,
		"remote: Repository not found.\nfatal: repository 'https://github.com/foo/bar' not found",
		`invalid github.com import path "github.com/foo"`,
		`malformed import path "foo//bar": double slash`,
		`no matching versions for query "latest"`,
		`unrecognized import path "foo"`,
		`unknown revision v9.9.9`,
	}
	for _, s := range notFound {
		if !isNotFoundOutput(s) {
			t.Errorf("isNotFoundOutput(%q) = false, want true", s)
		}
	}
	// Real / transient errors must NOT be classified as "not found", otherwise a
	// shorter, wrong module could be picked silently.
	real := []string{
		`dial tcp 127.0.0.1:1: connect: connection refused`,
		`reading .../@v/list: 500 Internal Server Error`,
		`fatal: could not read Username for 'https://github.com': terminal prompts disabled`,
		`verifying module: checksum mismatch`,
		`Get "https://proxy": net/http: TLS handshake timeout`,
	}
	for _, s := range real {
		if isNotFoundOutput(s) {
			t.Errorf("isNotFoundOutput(%q) = true, want false", s)
		}
	}
}

func TestPkgInModule(t *testing.T) {
	root := t.TempDir()
	// root/              (module root, no .go files -> a pure container)
	//   pkg/a.go         (a real package)
	//   empty/b.txt      (a directory with no .go files)
	//   nested/go.mod    (a nested module)
	//   nested/n.go
	//   nested/deep/d.go (shadowed by nested/go.mod)
	mustWrite(t, filepath.Join(root, "pkg", "a.go"), "package pkg\n")
	mustWrite(t, filepath.Join(root, "empty", "b.txt"), "not go\n")
	mustWrite(t, filepath.Join(root, "nested", "go.mod"), "module example.com/nested\n")
	mustWrite(t, filepath.Join(root, "nested", "n.go"), "package nested\n")
	mustWrite(t, filepath.Join(root, "nested", "deep", "d.go"), "package deep\n")

	for _, c := range []struct {
		rel  string
		want bool
	}{
		{"", true},             // module root itself (container is fine)
		{"pkg", true},          // real package with a .go file
		{"empty", false},       // directory without .go files
		{"missing", false},     // directory does not exist
		{"nested", false},      // nested go.mod -> different module
		{"nested/deep", false}, // shadowed by nested/go.mod
	} {
		if got := pkgInModule(root, c.rel); got != c.want {
			t.Errorf("pkgInModule(root, %q) = %v, want %v", c.rel, got, c.want)
		}
	}
	if pkgInModule("", "") {
		t.Error("pkgInModule with empty modRoot should be false")
	}
	if pkgInModule(filepath.Join(root, "missing-root"), "") {
		t.Error("pkgInModule with missing modRoot should be false")
	}
}

// GetPkg input validation does not touch the network, so it is always exercised.
func TestGetPkgRejects(t *testing.T) {
	// Invalid import paths.
	for _, p := range []string{"", "foo//bar"} {
		if _, _, err := GetPkg(p); err == nil {
			t.Errorf("GetPkg(%q): expected an error", p)
		}
	}
	// Standard-library paths are rejected without a network probe.
	for _, p := range []string{"fmt", "net/http"} {
		_, _, err := GetPkg(p)
		if !errors.Is(err, ErrStdlibPkg) {
			t.Errorf("GetPkg(%q): err = %v, want ErrStdlibPkg", p, err)
		}
	}
}

func mustWrite(t *testing.T, name, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
}
