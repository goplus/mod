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
	"strings"
	"testing"
)

// These tests download real modules through GOPROXY and are skipped under
// `go test -short`. They exercise the end-to-end resolution of GetPkg.

func TestGetPkgRoot(t *testing.T) {
	if testing.Short() {
		t.Skip("network test skipped in -short mode")
	}
	// The package is the module root itself: relToRoot must be empty.
	mod, rel, err := GetPkg("github.com/qiniu/x")
	if err != nil {
		t.Fatal("GetPkg:", err)
	}
	if mod.Path != "github.com/qiniu/x" {
		t.Fatalf("mod.Path = %q, want github.com/qiniu/x", mod.Path)
	}
	if rel != "" {
		t.Fatalf("relToRoot = %q, want empty", rel)
	}
	if mod.Version == "" {
		t.Fatal("mod.Version is empty")
	}
}

func TestGetPkgSubPackage(t *testing.T) {
	if testing.Short() {
		t.Skip("network test skipped in -short mode")
	}
	// A path several levels below the module root resolves to the module and the
	// correct relative path.
	mod, rel, err := GetPkg("github.com/goplus/lib/c/os")
	if err != nil {
		t.Fatal("GetPkg:", err)
	}
	if mod.Path != "github.com/goplus/lib" {
		t.Fatalf("mod.Path = %q, want github.com/goplus/lib", mod.Path)
	}
	if rel != "c/os" {
		t.Fatalf("relToRoot = %q, want c/os", rel)
	}
	// The invariant must hold.
	if mod.Path+"/"+rel != "github.com/goplus/lib/c/os" {
		t.Fatalf("invariant broken: %q + %q", mod.Path, rel)
	}
}

func TestGetPkgMajorVersion(t *testing.T) {
	if testing.Short() {
		t.Skip("network test skipped in -short mode")
	}
	// A /vN path resolves to the /vN module, not to the unsuffixed module with a
	// vN/... subdirectory.
	mod, rel, err := GetPkg("github.com/go-redis/redis/v8/internal")
	if err != nil {
		t.Fatal("GetPkg:", err)
	}
	if mod.Path != "github.com/go-redis/redis/v8" {
		t.Fatalf("mod.Path = %q, want github.com/go-redis/redis/v8", mod.Path)
	}
	if rel != "internal" {
		t.Fatalf("relToRoot = %q, want internal", rel)
	}
}

func TestGetPkgNotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("network test skipped in -short mode")
	}
	const pkg = "github.com/goplus/this-module-does-not-exist-xyz123/sub"
	_, _, err := GetPkg(pkg)
	if err == nil {
		t.Fatal("GetPkg: expected an error for a nonexistent package")
	}
	if !errors.Is(err, ErrPkgNotFound) {
		// In environments without git credentials, GitHub answers a missing
		// public repo with an auth prompt rather than "not found"; that is
		// surfaced as a real error on purpose, so accept it here but require that
		// it is not silently resolved.
		t.Logf("GetPkg(%q) returned a non-ErrPkgNotFound error (acceptable if no VCS creds): %v", pkg, err)
		return
	}
	// The error should list the prefixes that were tried.
	if msg := err.Error(); !strings.Contains(msg, "tried ") {
		t.Fatalf("error does not list tried prefixes: %v", msg)
	}
}
