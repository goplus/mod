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

package gopmod

import (
	"fmt"
	"log"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"

	"github.com/goplus/mod"
	"github.com/goplus/mod/modcache"
	"github.com/goplus/mod/modfetch"
	"github.com/goplus/mod/modload"
	"github.com/qiniu/x/errors"
	"golang.org/x/mod/module"
)

// -----------------------------------------------------------------------------

type depmodInfo struct {
	path string
	real module.Version
}

type Module struct {
	modload.Module
	projects map[string]*Project // ext -> project
	depmods  []depmodInfo
}

// IsValid returns if this module exists or not.
func (p *Module) IsValid() bool {
	return p != nil && p.File != nil
}

// PkgType specifies a package type.
type PkgType int

const (
	PkgtStandard PkgType = iota // a standard Go/Go+ package
	PkgtModule                  // a package in this module (in standard form)
	PkgtLocal                   // a package in this module (in relative path form)
	PkgtExtern                  // an extarnal package
	PkgtInvalid  = -1           // an invalid package
)

// PkgType returns the package type of specified package.
func (p *Module) PkgType(pkgPath string) PkgType {
	if pkgPath == "" {
		return PkgtInvalid
	}
	if isPkgInMod(pkgPath, p.Path()) {
		return PkgtModule
	}
	if pkgPath[0] == '.' {
		return PkgtLocal
	}
	pos := strings.Index(pkgPath, "/")
	if pos > 0 {
		pkgPath = pkgPath[:pos]
	}
	if strings.Contains(pkgPath, ".") {
		return PkgtExtern
	}
	return PkgtStandard
}

func isPkgInMod(pkgPath, modPath string) bool {
	if strings.HasPrefix(pkgPath, modPath) {
		suffix := pkgPath[len(modPath):]
		return suffix == "" || suffix[0] == '/'
	}
	return false
}

type Package struct {
	Type    PkgType
	Dir     string
	ModDir  string
	ModPath string
	Real    module.Version // only when Type == PkgtExtern
}

func (p *Module) Lookup(pkgPath string) (pkg *Package, err error) {
	switch pt := p.PkgType(pkgPath); pt {
	case PkgtStandard:
		modDir := runtime.GOROOT()
		pkg = &Package{Type: PkgtStandard, ModDir: modDir, Dir: filepath.Join(modDir, pkgPath)}
	case PkgtModule:
		modPath := p.Path()
		modDir := p.Root()
		dir := modDir + pkgPath[len(modPath):]
		pkg = &Package{Type: PkgtModule, ModPath: modPath, ModDir: modDir, Dir: dir}
	case PkgtExtern:
		return p.lookupExternPkg(pkgPath)
	default:
		log.Panicln("Module.Lookup:", pkgPath, "unsupported pkgType:", pt)
	}
	return
}

// lookupExternPkg lookups a external package from depended modules.
// If modVer.Path is replace to be a local path, it will be canonical to an absolute path.
func (p *Module) lookupExternPkg(pkgPath string) (pkg *Package, err error) {
	for _, m := range p.depmods {
		if isPkgInMod(pkgPath, m.path) {
			if modDir, e := modcache.Path(m.real); e == nil {
				modPath := m.path
				dir := modDir + pkgPath[len(modPath):]
				pkg = &Package{Type: PkgtExtern, Real: m.real, ModPath: modPath, ModDir: modDir, Dir: dir}
			} else {
				err = e
			}
			return
		}
	}
	err = &MissingError{Path: pkgPath}
	return
}

// LookupDepMod lookups a depended module.
// If modVer.Path is replace to be a local path, it will be canonical to an absolute path.
func (p *Module) LookupDepMod(modPath string) (modVer module.Version, ok bool) {
	for _, m := range p.depmods {
		if m.path == modPath {
			modVer, ok = m.real, true
			break
		}
	}
	return
}

// IsGopMod returns if this module is a Go+ module or not.
func (p *Module) IsGopMod() bool {
	const gopPkgPath = "github.com/goplus/gop"
	_, file := filepath.Split(p.Modfile())
	if file == "gop.mod" {
		return true
	}
	if _, ok := p.LookupDepMod(gopPkgPath); ok {
		return true
	}
	return p.Path() == gopPkgPath
}

func getDepMods(mod modload.Module) []depmodInfo {
	depmods := mod.DepMods()
	ret := make([]depmodInfo, 0, len(depmods))
	for path, m := range depmods {
		ret = append(ret, depmodInfo{path: path, real: m})
	}
	sort.Slice(ret, func(i, j int) bool {
		return ret[i].path > ret[j].path
	})
	return ret
}

// New creates a module from a modload.Module instance.
func New(mod modload.Module) *Module {
	projects := make(map[string]*Project)
	depmods := getDepMods(mod)
	return &Module{projects: projects, depmods: depmods, Module: mod}
}

// Load loads a module from a local dir.
// If we only want to load a Go modfile, pass env parameter as nil.
func Load(dir string, mode mod.Mode) (*Module, error) {
	mod, err := modload.Load(dir, mode)
	if err != nil {
		return nil, errors.NewWith(err, `modload.Load(dir, mode)`, -2, "modload.Load", dir, mode)
	}
	return New(mod), nil
}

// LoadMod loads a module from a versioned module path.
// If we only want to load a Go modfile, pass env parameter as nil.
func LoadMod(mod module.Version, mode mod.Mode) (p *Module, err error) {
	p, err = loadModFrom(mod, mode)
	if err != syscall.ENOENT {
		return
	}
	mod, err = modfetch.Get(mod.String())
	if err != nil {
		return
	}
	return loadModFrom(mod, mode)
}

func loadModFrom(mod module.Version, mode mod.Mode) (p *Module, err error) {
	dir, err := modcache.Path(mod)
	if err != nil {
		return
	}
	return Load(dir, mode)
}

type MissingError struct {
	Path string
}

func (e *MissingError) Error() string {
	return fmt.Sprintf(`no required module provides package %v; to add it:
	gop get %v`, e.Path, e.Path)
}

// -----------------------------------------------------------------------------
