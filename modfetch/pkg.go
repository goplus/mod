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
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/mod/module"
)

// ErrPkgNotFound is the sentinel reported by GetPkg when no module provides the
// requested package path. It is wrapped by the returned error, so callers can
// use errors.Is to tell "no such package" apart from a transient failure.
var ErrPkgNotFound = errors.New("cannot find module providing package")

// ErrStdlibPkg is wrapped by the error GetPkg returns for a standard-library
// import path (one whose first element has no dot). Such a path belongs to no
// downloadable module, so GetPkg reports it without touching the network.
var ErrStdlibPkg = errors.New("standard-library package")

// GetPkg resolves the module that provides pkgPath, downloads it to GOMODCACHE
// the same way Get does, and reports where the package sits inside that module.
//
// mod is the resolved module (path and version), exactly as Get would return
// it. relToRoot is the package path relative to the module root, using forward
// slashes; it is empty when pkgPath is the module root itself. The invariant is:
//
//	pkgPath == mod.Path                   when relToRoot == ""
//	pkgPath == mod.Path + "/" + relToRoot otherwise
//
// GetPkg enumerates the slash-separated prefixes of pkgPath from longest to
// shortest and probes each as a module. A "not found" / "gone" answer means the
// prefix is not a module and is skipped; any other failure (network error, 5xx,
// authentication, checksum problems) is a real error and is returned, so GetPkg
// never silently picks a shorter, wrong module. Among the prefixes that are
// modules and actually contain the package, the longest module path wins, which
// makes a /vN module and a nested module win over their unsuffixed parent.
//
// If no prefix qualifies, the returned error wraps ErrPkgNotFound and lists the
// prefixes that were tried. Passing noCache bypasses the cache for the probes
// and the final fetch, matching Get.
func GetPkg(pkgPath string, noCache ...bool) (mod module.Version, relToRoot string, err error) {
	if debugVerbose {
		log.Println("modfetch.GetPkg", pkgPath)
	}
	if err = module.CheckImportPath(pkgPath); err != nil {
		return
	}
	if isStdlibPath(pkgPath) {
		err = fmt.Errorf("%q is a %w", pkgPath, ErrStdlibPkg)
		return
	}
	// Candidate module paths: all prefixes of pkgPath from longest to shortest,
	// down to the first path element. The longest valid one that is a module and
	// contains the package wins, so probe longest first and stop at the first
	// qualifying candidate.
	var tried []string
	for _, candidate := range modPathPrefixes(pkgPath) {
		if !validModulePrefix(candidate) {
			// Not a plausible module path (invalid syntax, or a bare host with
			// no element after it); it cannot be the module, so do not count it
			// among the tried prefixes.
			continue
		}
		tried = append(tried, candidate)
		modRoot, cand, e := probePkg(candidate, noCache...)
		if e == ErrNotFound {
			continue // this prefix is not a module
		}
		if e != nil {
			err = e // transient / non-404 failure: surface it, never skip
			return
		}
		rel := relToModRoot(pkgPath, cand.Path)
		if !pkgInModule(modRoot, rel) {
			// The prefix is a module but does not contain pkgPath (the directory
			// is missing, holds no package source, or belongs to a nested
			// module). Keep looking at shorter prefixes.
			continue
		}
		mod, relToRoot = cand, rel
		return
	}
	err = fmt.Errorf("%w %q (tried %s)", ErrPkgNotFound, pkgPath, strings.Join(tried, ", "))
	return
}

// isStdlibPath reports whether pkgPath is a standard-library import path, i.e.
// its first path element contains no dot. Such a path belongs to no module.
func isStdlibPath(pkgPath string) bool {
	first := pkgPath
	if i := strings.IndexByte(first, '/'); i >= 0 {
		first = first[:i]
	}
	return !strings.Contains(first, ".")
}

// modPathPrefixes returns the slash-separated prefixes of pkgPath, from longest
// (pkgPath itself) to shortest (its first element).
func modPathPrefixes(pkgPath string) []string {
	var prefixes []string
	for p := pkgPath; ; {
		prefixes = append(prefixes, p)
		i := strings.LastIndexByte(p, '/')
		if i < 0 {
			break
		}
		p = p[:i]
	}
	return prefixes
}

// validModulePrefix reports whether candidate is a plausible module path worth
// probing: it passes module.CheckPath, its first element looks like a host
// (contains a dot), and it has at least one element after the host. This skips
// bare hosts such as "github.com" without a network probe. A host/user prefix
// such as "github.com/goplus" still passes here because it is a legal module
// path for some hosts; when it is not a module the probe reports "not found".
func validModulePrefix(candidate string) bool {
	if module.CheckPath(candidate) != nil {
		return false
	}
	i := strings.IndexByte(candidate, '/')
	if i < 0 {
		return false // no path element after the host
	}
	return strings.Contains(candidate[:i], ".")
}

// relToModRoot returns pkgPath relative to modPath (which must be a prefix of
// pkgPath), using forward slashes. It is "" when pkgPath == modPath.
func relToModRoot(pkgPath, modPath string) string {
	return strings.TrimPrefix(strings.TrimPrefix(pkgPath, modPath), "/")
}

// probePkg resolves candidate as a module the same way Get does and returns the
// module root in GOMODCACHE together with the resolved module.Version. It
// returns ErrNotFound when candidate is not a module (a 404/410 from the proxy,
// or a VCS lookup that finds no repository). Any other failure is returned as a
// real error so the caller never mistakes it for "not a module".
func probePkg(candidate string, noCache ...bool) (modRoot string, mod module.Version, err error) {
	if noCache == nil || !noCache[0] {
		if modRoot, mod, err = lookupFromCache(candidate); err != ErrNotFound {
			return // cache hit (or a real cache error)
		}
	}
	if err = goGetNotFound(candidate); err != nil {
		return
	}
	return lookupFromCache(candidate)
}

// goGetNotFound runs `go get candidate@latest` and classifies the outcome for
// GetPkg: nil when the module was resolved, ErrNotFound when the proxy/VCS says
// the module does not exist (404 / 410 / no repository), and the underlying
// error otherwise (network, 5xx, authentication, checksum database, ...).
//
// The command runs inside a disposable module so it only downloads into
// GOMODCACHE and never mutates the caller's go.mod.
func goGetNotFound(candidate string) error {
	dir, cleanup, err := tempModule()
	if err != nil {
		return err
	}
	defer cleanup()

	var stdout, stderr bytes.Buffer
	cmd := exec.Command("go", "get", candidate+"@latest")
	cmd.Dir = dir
	if debugVerbose {
		log.Println("==>", cmd)
	}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	errText := stderr.String()
	if runErr == nil {
		return nil // resolved; module is now in the cache
	}
	if isNotFoundOutput(errText) {
		return ErrNotFound
	}
	if errText != "" {
		return fmt.Errorf("modfetch: probing %q: %s", candidate, strings.TrimSpace(errText))
	}
	return fmt.Errorf("modfetch: probing %q: %w", candidate, runErr)
}

// tempModule creates a throwaway module directory with a minimal go.mod so that
// `go get` (which refuses to run outside a module) downloads into GOMODCACHE
// without touching the caller's own go.mod. The returned cleanup removes it.
func tempModule() (dir string, cleanup func(), err error) {
	dir, err = os.MkdirTemp("", "modfetch-getpkg-")
	if err != nil {
		return
	}
	gomod := "module modfetch.probe/tmp\n\ngo 1.21\n"
	if err = os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0644); err != nil {
		os.RemoveAll(dir)
		return "", nil, err
	}
	return dir, func() { os.RemoveAll(dir) }, nil
}

// isNotFoundOutput reports whether `go get` stderr describes a missing module
// (as opposed to a transient or configuration failure). A 404/410 from the
// proxy, or a VCS lookup that finds no repository, means "not a module"; a
// 5xx, a dial/connection error, or an authentication/checksum failure does not.
//
// Authentication prompts (for example "could not read Username") are left as
// real errors on purpose: a private module that genuinely exists must not be
// mistaken for "not a module", which would silently pick a shorter prefix.
func isNotFoundOutput(errText string) bool {
	for _, sig := range notFoundSignals {
		if strings.Contains(errText, sig) {
			return true
		}
	}
	// `go get` rejects a bare host or host/user prefix as a module root with
	// "invalid <host> import path ...". The host varies, so match the two
	// bracketing phrases instead of a fixed substring.
	if strings.Contains(errText, "invalid ") && strings.Contains(errText, " import path") {
		return true
	}
	return false
}

// notFoundSignals are substrings `go get` prints when a module or its repository
// does not exist, across the proxy (404/410, "not found: module") and the VCS
// fallback ("Repository not found", "repository ... not found", bad import path,
// no tagged versions). A path the toolchain rejects as an invalid module import
// path (for example a bare host or host/user prefix) also cannot be a module and
// is treated as "not found" so GetPkg skips it and keeps probing.
var notFoundSignals = []string{
	"404 Not Found",
	"410 Gone",
	"not found: module",
	"Repository not found", // GitHub VCS, credentials available
	"repository not found", // lowercase variants from other hosts
	"' not found",          // git "fatal: repository '...' not found"
	"no matching versions",
	"unrecognized import path",
	"unknown revision",
	"malformed import path", // syntactically invalid path
	"invalid pseudo-version",
}

// pkgInModule reports whether the module rooted at modRoot provides the package
// at the relative path rel.
//
//   - When rel == "", the requested package is the module root itself. The
//     module having been resolved and extracted is enough; a module root may be
//     a pure container with no .go files directly in it, so no source-file check
//     is applied.
//   - When rel != "", the directory must exist, hold at least one .go file, and
//     neither it nor any of its parents below the root may carry its own go.mod.
//     A nested go.mod marks a different module, so a package shadowed by one does
//     not count as provided by the outer module.
func pkgInModule(modRoot, rel string) bool {
	if modRoot == "" {
		return false
	}
	if rel == "" {
		return dirExists(modRoot)
	}
	if nestedModuleShadows(modRoot, rel) {
		return false
	}
	return hasGoFiles(filepath.Join(modRoot, filepath.FromSlash(rel)))
}

// nestedModuleShadows reports whether a go.mod below modRoot (but not at the
// root itself) governs the directory rel, which would place rel in a nested
// module rather than the one rooted at modRoot. It checks rel and each of its
// parents down to, but excluding, the module root.
func nestedModuleShadows(modRoot, rel string) bool {
	for p := rel; p != ""; {
		if fileExists(filepath.Join(modRoot, filepath.FromSlash(p), "go.mod")) {
			return true
		}
		i := strings.LastIndexByte(p, '/')
		if i < 0 {
			break
		}
		p = p[:i]
	}
	return false
}

// hasGoFiles reports whether dir is a directory containing at least one .go
// file directly in it.
func hasGoFiles(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") {
			return true
		}
	}
	return false
}

func fileExists(name string) bool {
	fi, err := os.Stat(name)
	return err == nil && !fi.IsDir()
}

func dirExists(name string) bool {
	fi, err := os.Stat(name)
	return err == nil && fi.IsDir()
}

// -----------------------------------------------------------------------------
