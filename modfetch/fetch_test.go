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
	"os"
	"path/filepath"
	"testing"

	"github.com/goplus/mod/modcache"
)

func init() {
	SetDebug(DbgFlagAll)
}

// withModCache points modcache.GOMODCACHE at dir for the duration of the test,
// restoring the previous value afterwards. It lets the cache-backed paths of Get
// run against a controlled fake cache without any network access.
func withModCache(t *testing.T, dir string) {
	t.Helper()
	old := modcache.GOMODCACHE
	modcache.GOMODCACHE = dir
	t.Cleanup(func() { modcache.GOMODCACHE = old })
}

// mustMkModDir creates an extracted-module directory "<path>@<version>" inside
// the fake cache, mimicking what `go mod download` leaves in GOMODCACHE.
func mustMkModDir(t *testing.T, cache, path, version string) {
	t.Helper()
	dir := filepath.Join(cache, path+"@"+version)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
}

func TestGetEmptyPath(t *testing.T) {
	if _, err := Get(""); err != errEmptyModPath {
		t.Fatalf("Get(\"\"): err = %v, want errEmptyModPath", err)
	}
}

func TestGetFromCacheVersioned(t *testing.T) {
	cache := t.TempDir()
	withModCache(t, cache)
	mustMkModDir(t, cache, "example.com/foo", "v1.2.3")

	// A versioned path resolves straight from the cache, no network needed.
	mod, err := Get("example.com/foo@v1.2.3")
	if err != nil {
		t.Fatal("Get:", err)
	}
	if mod.Path != "example.com/foo" || mod.Version != "v1.2.3" {
		t.Fatalf("Get: got %v %v, want example.com/foo v1.2.3", mod.Path, mod.Version)
	}
}

func TestGetFromCacheLatest(t *testing.T) {
	cache := t.TempDir()
	withModCache(t, cache)
	// Several cached versions of the same module; an unversioned request must
	// resolve to the highest semver, not the first or last read from disk.
	mustMkModDir(t, cache, "example.com/foo", "v1.2.0")
	mustMkModDir(t, cache, "example.com/foo", "v1.10.0")
	mustMkModDir(t, cache, "example.com/foo", "v1.3.0")

	mod, err := Get("example.com/foo")
	if err != nil {
		t.Fatal("Get:", err)
	}
	if mod.Path != "example.com/foo" || mod.Version != "v1.10.0" {
		t.Fatalf("Get: got %v %v, want example.com/foo v1.10.0", mod.Path, mod.Version)
	}
}

func TestGetVersionedCacheMiss(t *testing.T) {
	cache := t.TempDir()
	withModCache(t, cache)
	// The module path is cached but not at the requested version. A versioned
	// request is an exact-match lookup, so this is a cache miss. getFromCache
	// (the final fallback of Get) must report ErrNotFound rather than silently
	// returning a different version.
	mustMkModDir(t, cache, "example.com/foo", "v1.0.0")

	if _, err := getFromCache("example.com/foo@v2.0.0"); err != ErrNotFound {
		t.Fatalf("getFromCache: err = %v, want ErrNotFound", err)
	}
}

func TestGetUnversionedCacheMiss(t *testing.T) {
	cache := t.TempDir()
	withModCache(t, cache)
	// Nothing cached for this module: an unversioned lookup over a missing cache
	// directory must normalize to ErrNotFound (a cache miss), the signal Get uses
	// to decide whether to fall back to the network.
	if _, err := getFromCache("example.com/absent"); err != ErrNotFound {
		t.Fatalf("getFromCache: err = %v, want ErrNotFound", err)
	}
}

func TestGetResult(t *testing.T) {
	const parentFirst = "go: downloading github.com/llarhub/libcxx v0.1.1\n" +
		"go: downloading github.com/llarhub/libcxx/c v0.1.1\n" +
		"go: added github.com/llarhub/libcxx/c v0.1.1\n"
	const childFirst = "go: downloading github.com/llarhub/libcxx/c v0.1.1\n" +
		"go: downloading github.com/llarhub/libcxx v0.1.1\n" +
		"go: added github.com/llarhub/libcxx/c v0.1.1\n"

	// Whatever the order of the downloading lines, the requested module must be
	// selected instead of the first-printed one (which may be its parent module
	// in a multi-module repository). This ordering is non-deterministic in
	// `go get`, which is the root cause of the intermittent failure this fixes.
	for name, data := range map[string]string{
		"parentFirst": parentFirst,
		"childFirst":  childFirst,
	} {
		t.Run(name, func(t *testing.T) {
			mod, err := getResult(data, "github.com/llarhub/libcxx/c")
			if err != nil {
				t.Fatal("getResult:", err)
			}
			if mod.Path != "github.com/llarhub/libcxx/c" || mod.Version != "v0.1.1" {
				t.Fatalf("getResult: got %v %v", mod.Path, mod.Version)
			}
		})
	}
}

func TestGetResultFallback(t *testing.T) {
	// No exact match for reqPath: fall back to the first downloading line, and
	// with an empty reqPath (single-module callers) keep the first line too.
	const data = "go: downloading github.com/xushiwei/foogop v0.1.0\n"
	for _, reqPath := range []string{"", "github.com/not/matched"} {
		mod, err := getResult(data, reqPath)
		if err != nil {
			t.Fatal("getResult:", err)
		}
		if mod.Path != "github.com/xushiwei/foogop" || mod.Version != "v0.1.0" {
			t.Fatalf("getResult(%q): got %v %v", reqPath, mod.Path, mod.Version)
		}
	}
}

func TestGetResultNotFound(t *testing.T) {
	if _, err := getResult("go: added something\n", ""); err != ErrNotFound {
		t.Fatal("getResult: expected ErrNotFound, got", err)
	}
}
