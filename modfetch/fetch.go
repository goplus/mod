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
	"syscall"

	"github.com/goplus/mod/modcache"
	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
)

type dbgFlags int

const (
	DbgFlagVerbose dbgFlags = 1 << iota
	DbgFlagAll              = DbgFlagVerbose
)

var (
	debugVerbose bool
)

// ErrNotFound is the sentinel reported when a module or package does not exist.
// It is the same value as github.com/goplus/mod.ErrNotFound (both are
// syscall.ENOENT), kept here so modfetch does not import the root mod package
// and create an import cycle through packages that depend on both.
const ErrNotFound = syscall.ENOENT

// SetDebug sets debug flags.
func SetDebug(flags dbgFlags) {
	debugVerbose = (flags & DbgFlagVerbose) != 0
}

// -----------------------------------------------------------------------------

var (
	errEmptyModPath = errors.New("empty module path")
)

// Get downloads a modPath to GOMODCACHE.
func Get(modPath string, noCache ...bool) (mod module.Version, err error) {
	if debugVerbose {
		log.Println("modfetch.Get", modPath)
	}
	if modPath == "" {
		err = errEmptyModPath
		return
	}
	if noCache == nil || !noCache[0] {
		mod, err = getFromCache(modPath)
		if err != ErrNotFound {
			return
		}
	}
	mod, err = goGet(modPath)
	if err != ErrNotFound {
		if debugVerbose {
			log.Println("modfetch.Get ret:", err)
		}
		return
	}
	return getFromCache(modPath)
}

// goGet runs `go get modPath` (adding @latest when no version is given) and
// reports the downloaded module. It returns ErrNotFound when `go get` did not
// report any downloaded module; the caller decides whether to fall back to the
// module cache. Unlike Get, goGet does not consult the cache itself.
func goGet(modPath string) (mod module.Version, err error) {
	var stdout, stderr bytes.Buffer
	var modPathVer = modPath
	if strings.IndexByte(modPath, '@') < 0 {
		modPathVer += "@latest"
	}
	cmd := exec.Command("go", "get", modPathVer)
	if debugVerbose {
		log.Println("==>", cmd)
	}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Run()
	if stderr.Len() > 0 {
		reqPath := modPath
		if pos := strings.IndexByte(reqPath, '@'); pos > 0 {
			reqPath = reqPath[:pos]
		}
		return getResult(stderr.String(), reqPath)
	}
	err = ErrNotFound
	return
}

func getResult(data string, reqPath string) (mod module.Version, err error) {
	if debugVerbose {
		log.Println("modfetch.getResult:", data)
	}
	// go: downloading github.com/xushiwei/foogop v0.1.0
	//
	// A single `go get` may emit multiple downloading lines (e.g. when the
	// requested module lives in a multi-module repository, its parent module is
	// downloaded too) and their order is not deterministic. Prefer the line
	// whose module path exactly matches the requested one; fall back to the
	// first downloading line to preserve the single-module behavior.
	const downloading = "go: downloading "
	var first module.Version
	var found bool
	for _, line := range strings.Split(data, "\n") {
		if !strings.HasPrefix(line, downloading) {
			continue
		}
		m, e := getMod(line[len(downloading):]+"\n", nil)
		if e != nil {
			continue
		}
		if !found {
			first, found = m, true
		}
		if reqPath != "" && m.Path == reqPath {
			mod = m
			break
		}
	}
	if !found {
		err = ErrNotFound
		return
	}
	if mod.Path == "" {
		mod = first
	}
	fmt.Fprintln(os.Stderr, "xgo: downloading", mod.Path, mod.Version)
	return
}

func getMod(data string, next *string) (mod module.Version, err error) {
	if pos := strings.IndexByte(data, '\n'); pos > 0 {
		line := data[:pos]
		if next != nil {
			*next = data[pos+1:]
		}
		if pos = strings.IndexByte(line, ' '); pos > 0 {
			mod.Path, mod.Version = line[:pos], line[pos+1:]
			return
		}
	}
	err = ErrNotFound
	return
}

// -----------------------------------------------------------------------------

func getFromCache(modPath string) (modVer module.Version, err error) {
	_, modVer, err = lookupFromCache(modPath)
	return
}

func lookupFromCache(modPath string) (modRoot string, mod module.Version, err error) {
	mod.Path = modPath
	pos := strings.IndexByte(modPath, '@')
	if pos > 0 {
		mod.Path, mod.Version = modPath[:pos], modPath[pos+1:]
	}
	encPath, err := module.EscapePath(mod.Path)
	if err != nil {
		return
	}
	modRoot = filepath.Join(modcache.GOMODCACHE, encPath+"@"+mod.Version)
	if pos > 0 { // has version
		fi, e := os.Stat(modRoot)
		if e != nil || !fi.IsDir() {
			err = ErrNotFound
		}
		return
	}
	dir, fname := filepath.Split(modRoot)
	fis, err := os.ReadDir(dir)
	if err != nil {
		err = errors.Unwrap(err)
		return
	}
	err = ErrNotFound
	for _, fi := range fis {
		if fi.IsDir() {
			if name := fi.Name(); strings.HasPrefix(name, fname) {
				ver := name[len(fname):]
				if semver.Compare(mod.Version, ver) < 0 {
					modRoot, mod.Version, err = dir+name, ver, nil
				}
			}
		}
	}
	return
}

// -----------------------------------------------------------------------------
