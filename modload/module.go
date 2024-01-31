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
	"github.com/goplus/mod/modfile"
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
	data, err = readFile(gopmod)
	if err != nil {
		opt = newGopMod(gopmod, defaultGopVer)
	} else {
		opt, err = modfile.ParseLax(gopmod, data, fix)
		if err != nil {
			err = errors.NewWith(err, `modfile.Parse(gopmod, data, fix)`, -2, "modfile.Parse", gopmod, data, fix)
			return
		}
	}
	importClassfileFromGoMod(opt, f)
	return Module{f, opt}, nil
}

func (p Module) AddRequire(path, vers string, hasProj bool) error {
	f := p.File
	if err := f.AddRequire(path, vers); err != nil {
		return err
	}
	for _, r := range f.Require {
		if r.Mod.Path == path {
			if !isClass(r) {
				addClass(p.Opt, r)
			}
			break
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

/*
const (
	gopMod = "github.com/goplus/gop"
)

// UpdateGoMod updates the go.mod file.
func (p Module) UpdateGoMod(env *env.Gop, checkChanged bool) error {
	gopmod := p.Modfile()
	dir, file := filepath.Split(gopmod)
	if file == "go.mod" {
		return nil
	}
	gomod := dir + "go.mod"
	if checkChanged && notChanged(gomod, gopmod) {
		return nil
	}
	return p.saveGoMod(gomod, env)
}

func (p Module) saveGoMod(gomod string, env *env.Gop) error {
	gof := p.convToGoMod(env)
	data, err := gof.Format()
	if err == nil {
		err = os.WriteFile(gomod, data, 0644)
	}
	return err
}

func (p Module) convToGoMod(env *env.Gop) *gomodfile.File {
	copy := p.File.File
	copy.Syntax = cloneGoFileSyntax(copy.Syntax)
	addRequireIfNotExist(&copy, gopMod, env.Version)
	addReplaceIfNotExist(&copy, gopMod, "", env.Root, "")
	return &copy
}

func addRequireIfNotExist(f *gomodfile.File, path, vers string) {
	for _, r := range f.Require {
		if r.Mod.Path == path {
			return
		}
	}
	f.AddNewRequire(path, vers, false)
}

func addReplaceIfNotExist(f *gomodfile.File, oldPath, oldVers, newPath, newVers string) {
	for _, r := range f.Replace {
		if r.Old.Path == oldPath && (oldVers == "" || r.Old.Version == oldVers) {
			return
		}
	}
	f.AddReplace(oldPath, oldVers, newPath, newVers)
}

func notChanged(target, src string) bool {
	fiTarget, err := os.Stat(target)
	if err != nil {
		return false
	}
	fiSrc, err := os.Stat(src)
	if err != nil {
		return false
	}
	return fiTarget.ModTime().After(fiSrc.ModTime())
}

// -----------------------------------------------------------------------------

func cloneGoFileSyntax(syn *modfile.FileSyntax) *modfile.FileSyntax {
	stmt := make([]modfile.Expr, 0, len(syn.Stmt))
	for _, e := range syn.Stmt {
		if isGopOrDeletedExpr(e) {
			continue
		}
		stmt = append(stmt, cloneExpr(e))
	}
	return &modfile.FileSyntax{
		Name:     syn.Name,
		Comments: syn.Comments,
		Stmt:     stmt,
	}
}

func cloneExpr(e modfile.Expr) modfile.Expr {
	if v, ok := e.(*modfile.LineBlock); ok {
		copy := *v
		return &copy
	}
	return e
}

func isGopOrDeletedExpr(e modfile.Expr) bool {
	switch verb := getVerb(e); verb {
	case "", "gop", "register", "project", "class":
		return true
	}
	return false
}

func getVerb(e modfile.Expr) string {
	if line, ok := e.(*modfile.Line); ok {
		if token := line.Token; len(token) > 0 {
			return token[0]
		}
		return "" // deleted line
	}
	return e.(*modfile.LineBlock).Token[0]
}
*/

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
