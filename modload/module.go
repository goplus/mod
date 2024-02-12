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

package modload

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goplus/mod"
	"github.com/goplus/mod/env"
	"github.com/goplus/mod/modfile"
	"github.com/goplus/mod/sumfile"
	"github.com/qiniu/x/errors"
	"golang.org/x/mod/module"

	gomodfile "golang.org/x/mod/modfile"
)

var (
	ErrNoModDecl   = errors.New("no module declaration in a .mod file")
	ErrNoModRoot   = errors.New("go.mod file not found in current directory or any parent directory")
	ErrSaveDefault = errors.New("attemp to save default project")
)

type Module struct {
	*gomodfile.File
	Opt *modfile.File
}

// HasModfile returns if this module exists or not.
func (p Module) HasModfile() bool {
	return p.Syntax != nil
}

// Modfile returns absolute path of the module file.
func (p Module) Modfile() string {
	if syn := p.Syntax; syn != nil {
		return syn.Name
	}
	return ""
}

func (p Module) workFile() string {
	if syn := p.Syntax; syn != nil {
		dir, _ := filepath.Split(syn.Name)
		return dir + "go.work"
	}
	return ""
}

func (p Module) sumFile() string {
	if syn := p.Syntax; syn != nil {
		dir, _ := filepath.Split(syn.Name)
		return dir + "go.sum"
	}
	return ""
}

// Root returns absolute root path of this module.
func (p Module) Root() string {
	if syn := p.Syntax; syn != nil {
		return filepath.Dir(syn.Name)
	}
	return ""
}

// Path returns the module path.
func (p Module) Path() string {
	if mod := p.Module; mod != nil {
		return mod.Mod.Path
	}
	return ""
}

// DepMods returns all depended modules.
// If a depended module path is replace to be a local path, it will be canonical to an absolute path.
func (p Module) DepMods() map[string]module.Version {
	vers := make(map[string]module.Version)
	for _, r := range p.Require {
		if r.Mod.Path != "" {
			vers[r.Mod.Path] = r.Mod
		}
	}
	for _, r := range p.Replace {
		if r.Old.Path != "" {
			real := r.New
			if real.Version == "" {
				if strings.HasPrefix(real.Path, ".") {
					dir, _ := filepath.Split(p.Modfile())
					real.Path = dir + real.Path
				}
				if a, err := filepath.Abs(real.Path); err == nil {
					real.Path = a
				}
			}
			vers[r.Old.Path] = real
		}
	}
	return vers
}

// Create creates a new module in `dir`.
// You should call `Save` manually to save this module.
func Create(dir string, modPath, goVer, gopVer string) (p Module, err error) {
	dir, err = filepath.Abs(dir)
	if err != nil {
		return
	}

	gomod := filepath.Join(dir, "go.mod")
	if _, err := os.Stat(gomod); err == nil {
		return Module{}, fmt.Errorf("gop: %s already exists", gomod)
	}

	gopmod := filepath.Join(dir, "gop.mod")
	if _, err := os.Stat(gopmod); err == nil {
		return Module{}, fmt.Errorf("gop: %s already exists", gopmod)
	}

	if goVer == "" {
		goVer = defaultGoVer
	}
	if gopVer == "" {
		gopVer = defaultGopVer
	}
	mod := newGoMod(gomod, modPath, goVer)
	opt := newGopMod(gopmod, gopVer)
	return Module{mod, opt}, nil
}

func newGoMod(gomod, modPath, goVer string) *gomodfile.File {
	mod := new(gomodfile.File)
	mod.AddModuleStmt(modPath)
	mod.AddGoStmt(goVer)
	mod.Syntax.Name = gomod
	return mod
}

func newGopMod(gopmod, gopVer string) *modfile.File {
	return modfile.New(gopmod, gopVer)
}

// fixVersion returns a modfile.VersionFixer implemented using the Query function.
//
// It resolves commit hashes and branch names to versions,
// canonicalizes versions that appeared in early vgo drafts,
// and does nothing for versions that already appear to be canonical.
//
// The VersionFixer sets 'fixed' if it ever returns a non-canonical version.
func fixVersion(fixed *bool) modfile.VersionFixer {
	return func(path, vers string) (resolved string, err error) {
		// do nothing
		return vers, nil
	}
}

// Load loads a module from specified directory.
func Load(dir string) (p Module, err error) {
	dir, gomod, err := mod.FindGoMod(dir)
	if err != nil {
		err = errors.NewWith(err, `mod.FindGoMod(dir)`, -2, "mod.FindGoMod", dir)
		return
	}
	return LoadFrom(gomod, filepath.Join(dir, "gop.mod"))
}

// LoadFrom loads a module from specified go.mod file and an optional gop.mod file.
func LoadFrom(gomod, gopmod string) (p Module, err error) {
	return LoadFromEx(gomod, gopmod, os.ReadFile)
}

// LoadFromEx loads a module from specified go.mod file and an optional gop.mod file.
// It can specify a customized `readFile` to read file content.
func LoadFromEx(gomod, gopmod string, readFile func(string) ([]byte, error)) (p Module, err error) {
	data, err := readFile(gomod)
	if err != nil {
		err = errors.NewWith(err, `readFile(gomod)`, -2, "readFile", gomod)
		return
	}

	var fixed bool
	fix := fixVersion(&fixed)
	// it is go.mod file, so we need to use "Parse" parse it
	f, err := gomodfile.Parse(gomod, data, fix)
	if err != nil {
		err = errors.NewWith(err, `gomodfile.Parse(gomod, data, fix)`, -2, "gomodfile.Parse", gomod, data, fix)
		return
	}
	mod := f.Module
	if mod == nil {
		// No module declaration. Must add module path.
		err = errors.NewWith(ErrNoModDecl, `mod == nil`, -2, "==", mod, nil)
		return
	}
	if mod.Mod.Path == "std" {
		mod.Mod.Path = "" // the Go std module
	}

	var opt *modfile.File
	if gopmod != "" {
		if data, err = readFile(gopmod); err == nil {
			opt, err = modfile.ParseLax(gopmod, data, fix)
			if err != nil {
				err = errors.NewWith(err, `modfile.Parse(gopmod, data, fix)`, -2, "modfile.Parse", gopmod, data, fix)
				return
			}
		}
	}
	if opt == nil {
		opt = newGopMod(gopmod, defaultGopVer)
	}
	importClassfileFromGoMod(opt, f)
	return Module{f, opt}, nil
}

func (p Module) AddRequire(path, vers string, hasProj bool) error {
	f := p.File
	f.AddRequire(path, vers)
	if hasProj {
		for _, r := range f.Require {
			if r.Mod.Path == path {
				if !isClass(r) {
					addClass(p.Opt, r)
				}
				break
			}
		}
	}
	return nil
}

func importClassfileFromGoMod(opt *modfile.File, f *gomodfile.File) {
	for _, r := range f.Require {
		if isClass(r) {
			opt.ClassMods = append(opt.ClassMods, r.Mod.Path)
		}
	}
}

func addClass(opt *modfile.File, r *gomodfile.Require) {
	if line := r.Syntax; line != nil {
		line.Suffix = append(line.Suffix, modfile.Comment{
			Token:  "//gop:class", // without trailing newline
			Suffix: true,          // an end of line (not whole line) comment
		})
		opt.ClassMods = append(opt.ClassMods, r.Mod.Path)
	}
}

func isClass(r *gomodfile.Require) bool {
	if line := r.Syntax; line != nil {
		for _, c := range line.Suffix {
			text := strings.TrimLeft(c.Token[2:], " \t")
			if strings.HasPrefix(text, "gop:class") {
				return true
			}
		}
	}
	return false
}

// -----------------------------------------------------------------------------

func (p Module) Projects() []*modfile.Project {
	return p.Opt.Projects
}

func (p Module) HasProject() bool {
	return len(p.Opt.Projects) > 0
}

func hasGopExtended(opt *modfile.File) bool {
	return len(opt.Projects) > 0
}

// Save saves all changes of this module.
func (p Module) Save() (err error) {
	modf := p.Modfile()
	if modf == "" {
		return ErrSaveDefault
	}
	data, err := p.Format()
	if err != nil {
		return
	}
	err = os.WriteFile(modf, data, 0644)
	if err != nil {
		return
	}

	if opt := p.Opt; hasGopExtended(opt) {
		data := modfile.Format(opt.Syntax)
		err = os.WriteFile(opt.Syntax.Name, data, 0644)
	}
	return
}

func (p Module) checkGopDeps() (flags int) {
	for _, r := range p.File.Require {
		switch r.Mod.Path {
		case gopMod:
			flags |= FlagDepModGop
		case xMod:
			flags |= FlagDepModX
		}
	}
	return
}

func findReplaceGopMod(work *gomodfile.WorkFile) bool {
	for _, r := range work.Replace {
		if r.Old.Path == gopMod {
			return true
		}
	}
	return false
}

const (
	gopMod = "github.com/goplus/gop"
	xMod   = "github.com/qiniu/x"
)

const (
	FlagDepModGop = 1 << iota // depends module github.com/goplus/gop
	FlagDepModX               // depends module github.com/qiniu/x
)

// SaveWithGopMod adds `require github.com/goplus/gop` and saves all
// changes of this module.
func (p Module) SaveWithGopMod(gop *env.Gop, flags int) (err error) {
	if p.Path() == gopMod { // don't change Go+ itself
		return
	}
	old := p.checkGopDeps()
	if (flags &^ old) == 0 { // nothing to do
		return
	}

	gopVer := getGopVer(gop)
	p.requireGop(gop, gopVer, old, flags)
	return p.Save()
}

func (p Module) updateWorkfile(gop *env.Gop, gopVer string) (err error) {
	var work *gomodfile.WorkFile
	var workFile = p.workFile()
	b, err := os.ReadFile(workFile)
	if err != nil {
		if os.IsNotExist(err) {
			b = []byte(`go ` + p.Go.Version)
		} else {
			return
		}
	}
	var fixed bool
	fix := fixVersion(&fixed)
	if work, err = gomodfile.ParseWork(workFile, b, fix); err != nil {
		return
	}
	if findReplaceGopMod(work) {
		return
	}
	work.AddUse(".", p.Path())
	work.AddReplace(gopMod, gopVer, gop.Root, "")
	return os.WriteFile(workFile, gomodfile.Format(work.Syntax), 0666)
}

// requireGop adds require for the github.com/goplus/gop module.
func (p Module) requireGop(gop *env.Gop, gopVer string, old, flags int) {
	if (flags&FlagDepModGop) != 0 && (old&FlagDepModGop) == 0 {
		p.File.AddRequire(gopMod, gopVer)
		p.updateWorkfile(gop, gopVer)
	}
	if (flags&FlagDepModX) != 0 && (old&FlagDepModX) == 0 { // depends module github.com/qiniu/x
		if x, xsum, ok := getXVer(gop); ok {
			p.File.AddRequire(x.Path, x.Version)
			if sumf, err := sumfile.Load(p.sumFile()); err == nil && sumf.Lookup(xMod) == nil {
				sumf.Add(xsum)
				sumf.Save()
			}
		}
	}
}

func getXVer(gop *env.Gop) (modVer module.Version, xsum []string, ok bool) {
	if mod, err := LoadFrom(gop.Root+"/go.mod", ""); err == nil {
		for _, r := range mod.File.Require {
			if r.Mod.Path == xMod {
				if sumf, err := sumfile.Load(gop.Root + "/go.sum"); err == nil {
					return r.Mod, sumf.Lookup(xMod), true
				}
			}
		}
	}
	return
}

func getGopVer(gop *env.Gop) string {
	ver := gop.Version
	if pos := strings.IndexByte(ver, ' '); pos > 0 { // v1.2.0 devel
		ver = ver[:pos]
	}
	return ver
}

// -----------------------------------------------------------------------------

const (
	defaultGoVer  = "1.18"
	defaultGopVer = "1.2"
)

// Default represents the default gop.mod object.
var Default = Module{
	File: &gomodfile.File{
		Module: &gomodfile.Module{},
		Go:     &gomodfile.Go{Version: defaultGoVer},
	},
	Opt: &modfile.File{
		Gop: &modfile.Gop{Version: defaultGopVer},
	},
}

// -----------------------------------------------------------------------------
