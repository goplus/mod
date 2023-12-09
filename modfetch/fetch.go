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
	DbgFlagShowError
	DbgFlagAll = DbgFlagShowError | DbgFlagVerbose
)

var (
	debugVerbose bool
)

// SetDebug sets debug flags.
func SetDebug(flags dbgFlags) {
	debugVerbose = (flags & DbgFlagVerbose) != 0
}

// -----------------------------------------------------------------------------

// GetPkg downloads the module that contains pkgPath to GOMODCACHE.
func GetPkg(pkgPathVer, modBase string) (modVer module.Version, relPath string, err error) {
	var ver string
	var pkgPath string = pkgPathVer
	if pos := strings.IndexByte(pkgPath, '@'); pos > 0 {
		pkgPath, ver = pkgPath[:pos], pkgPath[pos+1:]
	}
	if debugVerbose {
		log.Println("modfetch.GetPkg", pkgPathVer, modBase)
	}
	if semver.IsValid(ver) {
		modVer, relPath, err = lookupListFromCache(pkgPath, "@"+ver)
		if err == nil {
			return
		}
	}
	var stdout, stderr bytes.Buffer
	cmd := exec.Command("go", "get", pkgPathVer)
	if debugVerbose {
		log.Println("==>", cmd)
	}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Run()
	if stderr.Len() > 0 {
		modVer, err = getResult(stderr.String())
		if err != syscall.ENOENT {
			if debugVerbose {
				log.Println("modfetch.Get ret:", err)
			}
			return
		}
	}
	var foundVer string
	if semver.IsValid(ver) {
		foundVer = "@" + ver
	}
	return lookupListFromCache(pkgPath, foundVer)
}

func lookupListFromCache(pkgPath string, ver string) (modVer module.Version, relPath string, err error) {
	list := strings.Split(pkgPath, "/")
	for i := len(list); i > 0; i-- {
		modPath := strings.Join(list[:i], "/") + ver
		_, modVer, err = lookupFromCache(modPath)
		if err == nil {
			encPath, _ := module.EscapePath(modVer.Path)
			modRoot := filepath.Join(modcache.GOMODCACHE, encPath+"@"+modVer.Version, filepath.Join(list[i:]...))
			if _, e := os.Stat(modRoot); e != nil {
				err = fmt.Errorf("gop: module %v found, but does not contain package %v", modVer.Path, pkgPath)
				return
			}
			relPath = strings.Join(list[i:], "/")
			return
		}
	}
	return
}

// Split splits a pkgPath into modPath and its relPath to module root.
func Split(pkgPath, modBase string) (modPath, relPath string) {
	if modBase != "" && strings.HasPrefix(pkgPath, modBase) {
		n := len(modBase)
		if len(pkgPath) == n {
			return modBase, ""
		}
		if pkgPath[n] == '/' {
			return modBase, pkgPath[n+1:]
		}
	}
	parts := strings.SplitN(pkgPath, "/", 4)
	if !strings.Contains(parts[0], ".") { // standard package
		return "", pkgPath
	}
	switch parts[0] {
	case ".", "..": // local package
		return modBase, pkgPath
	case "github.com", "golang.org":
		if len(parts) > 3 {
			relPath = parts[3]
			if pos := strings.IndexByte(relPath, '@'); pos > 0 {
				parts[2] += relPath[pos:]
				relPath = relPath[:pos]
			}
			modPath = strings.Join(parts[:3], "/")
		} else {
			modPath = pkgPath
		}
		return
	}
	panic("TODO: modfetch.Split - unexpected pkgPath: " + pkgPath)
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
		if err != syscall.ENOENT {
			return
		}
	}
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
		mod, err = getResult(stderr.String())
		if err != syscall.ENOENT {
			if debugVerbose {
				log.Println("modfetch.Get ret:", err)
			}
			return
		}
	}
	return getFromCache(modPath)
}

func getResult(data string) (mod module.Version, err error) {
	if debugVerbose {
		log.Println("modfetch.getResult:", data)
	}
	// go: downloading github.com/xushiwei/foogop v0.1.0
	const downloading = "go: downloading "
	if strings.HasPrefix(data, downloading) {
		if pos := strings.IndexByte(data, '\n'); pos > 0 {
			fmt.Fprintln(os.Stderr, "gop:", data[4:pos])
		}
		return getMod(data[len(downloading):], nil)
	}
	err = syscall.ENOENT
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
	err = syscall.ENOENT
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
			err = syscall.ENOENT
		}
		return
	}
	dir, fname := filepath.Split(modRoot)
	fis, err := os.ReadDir(dir)
	if err != nil {
		err = errors.Unwrap(err)
		return
	}
	err = syscall.ENOENT
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
