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
	"log"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"

	"github.com/goplus/mod/modcache"
	"github.com/goplus/mod/modfetch"
	"github.com/goplus/mod/modload"
	"golang.org/x/mod/module"
)

type GopEnv = modload.GopEnv

// -----------------------------------------------------------------------------

type depmodInfo struct {
	path string
	real module.Version
}

type Module struct {
	modload.Module
	classes map[string]*Class
	depmods []depmodInfo
	env     *GopEnv
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
}

func (p *Module) Lookup(pkgPath string) (pkg *Package, err error) {
	switch pt := p.PkgType(pkgPath); pt {
	case PkgtStandard:
		modDir := runtime.GOROOT()
		pkg = &Package{Type: PkgtStandard, ModDir: modDir, Dir: filepath.Join(modDir, pkgPath)}
	case PkgtModule:
		modPath := p.Path()
		modDir := p.Root()
		dir := modDir + pkgPath[:len(modPath)]
		pkg = &Package{Type: PkgtModule, ModPath: modPath, ModDir: modDir, Dir: dir}
	case PkgtExtern:
		if modPath, modVer, ok := p.LookupExternPkg(pkgPath); ok {
			if modDir, e := modcache.Path(modVer); e == nil {
				dir := modDir + pkgPath[:len(modPath)]
				pkg = &Package{Type: PkgtExtern, ModPath: modPath, ModDir: modDir, Dir: dir}
			} else {
				return nil, e
			}
		} else {
			err = syscall.ENOENT
		}
	default:
		log.Panicln("Module.Lookup:", pkgPath, "unsupported pkgType:", pt)
	}
	return
}

// LookupExternPkg lookups a external package from depended modules.
// If modVer.Path is replace to be a local path, it will be canonical to an absolute path.
func (p *Module) LookupExternPkg(pkgPath string) (modPath string, modVer module.Version, ok bool) {
	for _, m := range p.depmods {
		if isPkgInMod(pkgPath, m.path) {
			modPath, modVer, ok = m.path, m.real, true
			break
		}
	}
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
func New(mod modload.Module, env *GopEnv) *Module {
	classes := make(map[string]*Class)
	depmods := getDepMods(mod)
	return &Module{classes: classes, depmods: depmods, Module: mod, env: env}
}

// Load loads a module from a local dir.
// If we only want to load a Go modfile, pass env parameter as nil.
func Load(dir string, env *GopEnv) (*Module, error) {
	var mode modload.Mode
	if env == nil {
		mode = modload.GoModOnly
	}
	mod, err := modload.Load(dir, mode)
	if err != nil {
		return nil, err
	}
	return New(mod, env), nil
}

// LoadMod loads a module from a versioned module path.
// If we only want to load a Go modfile, pass env parameter as nil.
func LoadMod(mod module.Version, env *GopEnv) (p *Module, err error) {
	p, err = loadModFrom(mod, env)
	if err != syscall.ENOENT {
		return
	}
	mod, _, err = modfetch.Get(env, mod.String())
	if err != nil {
		return
	}
	return loadModFrom(mod, env)
}

func loadModFrom(mod module.Version, env *GopEnv) (p *Module, err error) {
	dir, err := modcache.Path(mod)
	if err != nil {
		return
	}
	return Load(dir, env)
}

// -----------------------------------------------------------------------------
