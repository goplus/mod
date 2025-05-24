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

package xgomod

import (
	"fmt"
	"log"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/goplus/mod/modcache"
	"github.com/goplus/mod/modfetch"
	"github.com/goplus/mod/modload"
	"github.com/qiniu/x/errors"
	"golang.org/x/mod/module"
)

var (
	ErrInvalidPkgPath = errors.New("invalid package path")
)

// -----------------------------------------------------------------------------

type Module struct {
	modload.Module
	projs    map[string]*Project // ext -> project
	depmods_ map[string]module.Version
}

// DepMods returns all depended modules.
// If a depended module path is replace to be a local path, it will be canonical to an absolute path.
func (p *Module) DepMods() map[string]module.Version {
	if p.depmods_ == nil {
		p.depmods_ = p.Module.DepMods()
	}
	return p.depmods_
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

// IsPkgtStandard checks if a pkgPath is Go standard package or not.
func (p *Module) IsPkgtStandard(pkgPath string) bool {
	return p.PkgType(pkgPath) == PkgtStandard
}

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

// PkgId returns an unique package id of specified package.
// PkgId of a Go standard package is its package path.
// ie. PkgId("fmt") == "fmt"
func (p *Module) PkgId(pkgPath string) (string, error) {
	if pkgPath == "" {
		return "", ErrInvalidPkgPath
	}
	modPath := p.Path()
	if isPkgInMod(pkgPath, modPath) {
		return p.Root() + pkgPath[len(modPath):], nil
	}
	if pkgPath[0] == '.' { // local package: please convert it first
		return "", ErrInvalidPkgPath
	}
	domain := pkgPath
	if pos := strings.Index(pkgPath, "/"); pos > 0 {
		domain = pkgPath[:pos]
	}
	if strings.Contains(domain, ".") {
		pkg, err := p.lookupExternPkg(pkgPath)
		if err != nil {
			return "", err
		}
		return pkg.Dir, nil
	}
	return pkgPath, nil
}

func isPkgInMod(pkgPath, modPath string) bool {
	if modPath != "" && strings.HasPrefix(pkgPath, modPath) {
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
		modDir := goroot + "/src"
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
	for path, real := range p.DepMods() {
		if isPkgInMod(pkgPath, path) {
			if modDir, e := modcache.Path(real); e == nil {
				modPath := path
				dir := modDir + pkgPath[len(modPath):]
				pkg = &Package{Type: PkgtExtern, Real: real, ModPath: modPath, ModDir: modDir, Dir: dir}
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
	deps := p.DepMods()
	modVer, ok = deps[modPath]
	return
}

// New creates a module from a modload.Module instance.
func New(mod modload.Module) *Module {
	return &Module{Module: mod}
}

// Load loads a module from a local directory.
func Load(dir string) (*Module, error) {
	mod, err := modload.Load(dir)
	if err != nil {
		return nil, errors.NewWith(err, `modload.Load(dir)`, -2, "modload.Load", dir)
	}
	return New(mod), nil
}

// LoadFrom loads a module from specified go.mod file and an optional gox.mod file.
func LoadFrom(gomod, goxmod string) (*Module, error) {
	mod, err := modload.LoadFrom(gomod, goxmod)
	if err != nil {
		return nil, errors.NewWith(err, `modload.LoadFrom(gomod, gopmod)`, -2, "modload.LoadFrom", gomod, goxmod)
	}
	return New(mod), nil
}

// LoadMod loads a module from a versioned module path.
// If we only want to load a Go modfile, pass env parameter as nil.
func LoadMod(mod module.Version) (p *Module, err error) {
	p, err = loadModFrom(mod)
	if !IsNotFound(err) {
		return
	}
	mod, err = modfetch.Get(mod.String())
	if err != nil {
		return
	}
	return loadModFrom(mod)
}

func loadModFrom(mod module.Version) (p *Module, err error) {
	dir, err := modcache.Path(mod)
	if err != nil {
		return
	}
	return Load(dir)
}

type MissingError struct {
	Path string
}

func (e *MissingError) Error() string {
	return fmt.Sprintf(`no required module provides package %v; to add it:
	xgo get %v`, e.Path, e.Path)
}

// -----------------------------------------------------------------------------

// Default represents the default gox.mod object.
var Default = New(modload.Default)

var goroot = runtime.GOROOT()

// -----------------------------------------------------------------------------
