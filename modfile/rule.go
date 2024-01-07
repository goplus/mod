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

package modfile

import (
	"fmt"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/qiniu/x/errors"
	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
)

// A File is the parsed, interpreted form of a gop.mod file.
type File struct {
	Gop      *Gop
	Projects []*Project
	Import   []*Import

	Syntax *FileSyntax
}

func (p *File) addProj(proj *Project) {
	p.Projects = append(p.Projects, proj)
}

func (p *File) proj() *Project { // current project
	n := len(p.Projects)
	if n == 0 {
		return nil
	}
	return p.Projects[n-1]
}

// A Module is the module statement.
type Module = modfile.Module

// A Gop is the gop statement.
type Gop = modfile.Go

// A Import is the import statement.
type Import struct {
	ClassfileMod string // module path of classfile
	Syntax       *Line
}

// A Project is the project statement.
type Project struct {
	Ext      string   // can be "_[class].gox" or ".[class]", eg "_yap.gox" or ".gmx"
	Class    string   // "Game"
	Works    []*Class // work class of classfile
	PkgPaths []string // package paths of classfile
	Syntax   *Line
}

// A Class is the work class statement.
type Class struct {
	Ext    string // can be "_[class].gox" or ".[class]", eg "_yap.gox" or ".spx"
	Class  string // "Sprite"
	Syntax *Line
}

type VersionFixer = modfile.VersionFixer

// Parse parses and returns a gop.mod file.
//
// file is the name of the file, used in positions and errors.
//
// data is the content of the file.
//
// fix is an optional function that canonicalizes module versions.
// If fix is nil, all module versions must be canonical (module.CanonicalVersion
// must return the same string).
func Parse(file string, data []byte, fix VersionFixer) (*File, error) {
	return parseToFile(file, data, fix, true)
}

// ParseLax is like Parse but ignores unknown statements.
// It is used when parsing gop.mod files other than the main module,
// under the theory that most statement types we add in the future will
// only apply in the main module, like exclude and replace,
// and so we get better gradual deployments if old go commands
// simply ignore those statements when found in gop.mod files
// in dependencies.
func ParseLax(file string, data []byte, fix VersionFixer) (*File, error) {
	return parseToFile(file, data, fix, false)
}

func parseToFile(file string, data []byte, fix VersionFixer, strict bool) (parsed *File, err error) {
	f, err := modfile.ParseLax(file, data, fix)
	if err != nil {
		err = errors.NewWith(err, `modfile.ParseLax(file, data, fix)`, -2, "modfile.ParseLax", file, data, fix)
		return
	}
	parsed = &File{Syntax: f.Syntax}

	var errs ErrorList
	var fs = f.Syntax
	for _, x := range fs.Stmt {
		switch x := x.(type) {
		case *Line:
			parsed.parseVerb(&errs, x.Token[0], x, x.Token[1:], strict)
		case *LineBlock:
			verb := x.Token[0]
			for _, line := range x.Line {
				parsed.parseVerb(&errs, verb, line, line.Token, strict)
			}
		}
	}
	if len(errs) > 0 {
		return nil, errors.NewWith(errs, `len(errs) > 0`, -1, ">", len(errs), 0)
	}
	return
}

func (f *File) parseVerb(errs *ErrorList, verb string, line *Line, args []string, strict bool) {
	wrapError1 := func(err error) {
		errs.Add(&Error{
			Filename: f.Syntax.Name,
			Pos:      line.Start,
			Err:      err,
		})
	}
	wrapError := func(err error) {
		file, line := fileLine(2)
		e := errors.NewFrame(err, "", file, line, "wrapError", err)
		wrapError1(e)
	}
	errorf := func(format string, args ...interface{}) {
		file, line := fileLine(2)
		e := errors.NewFrame(fmt.Errorf(format, args...), "", file, line, "errorf", format, args)
		wrapError1(e)
	}
	switch verb {
	case "gop":
		if f.Gop != nil {
			errorf("repeated gop statement")
			return
		}
		if len(args) != 1 {
			errorf("gop directive expects exactly one argument")
			return
		} else if !modfile.GoVersionRE.MatchString(args[0]) {
			errorf("invalid gop version '%s': must match format 1.23", args[0])
			return
		}
		f.Gop = &Gop{Syntax: line}
		f.Gop.Version = args[0]
	case "import", "register": // register => import
		if len(args) != 1 {
			errorf("import directive expects exactly one argument")
			return
		}
		s, err := parseString(&args[0])
		if err != nil {
			errorf("invalid quoted string: %v", err)
			return
		}
		err = module.CheckPath(s)
		if err != nil {
			wrapError(err)
			return
		}
		f.Import = append(f.Import, &Import{
			ClassfileMod: s,
			Syntax:       line,
		})
	case "project":
		if len(args) < 1 {
			errorf("usage: project [.projExt ProjClass] classFilePkgPath ...")
			return
		}
		if isExt(args[0]) {
			if len(args) < 3 || strings.Contains(args[1], "/") {
				errorf("usage: project [.projExt ProjClass] classFilePkgPath ...")
				return
			}
			ext, err := parseExt(&args[0])
			if err != nil {
				wrapError(err)
				return
			}
			class, err := parseSymbol(&args[1])
			if err != nil {
				wrapError(err)
				return
			}
			pkgPaths, err := parsePkgPaths(args[2:])
			if err != nil {
				wrapError(err)
				return
			}
			f.addProj(&Project{
				Ext: ext, Class: class, PkgPaths: pkgPaths, Syntax: line,
			})
			return
		}
		pkgPaths, err := parsePkgPaths(args)
		if err != nil {
			wrapError(err)
			return
		}
		f.addProj(&Project{
			PkgPaths: pkgPaths, Syntax: line,
		})
	case "class":
		proj := f.proj()
		if proj == nil {
			errorf("work class must declare after a project definition")
			return
		}
		if len(args) < 2 {
			errorf("usage: class .workExt WorkClass")
			return
		}
		workExt, err := parseExt(&args[0])
		if err != nil {
			wrapError(err)
			return
		}
		class, err := parseSymbol(&args[1])
		if err != nil {
			wrapError(err)
			return
		}
		proj.Works = append(proj.Works, &Class{
			Ext:    workExt,
			Class:  class,
			Syntax: line,
		})
	default:
		if strict {
			errorf("unknown directive: %s", verb)
		}
	}
}

func fileLine(n int) (file string, line int) {
	_, file, line, _ = runtime.Caller(n)
	return
}

// IsDirectoryPath reports whether the given path should be interpreted
// as a directory path. Just like on the go command line, relative paths
// and rooted paths are directory paths; the rest are module paths.
func IsDirectoryPath(ns string) bool {
	return modfile.IsDirectoryPath(ns)
}

// MustQuote reports whether s must be quoted in order to appear as
// a single token in a gop.mod line.
func MustQuote(s string) bool {
	return modfile.MustQuote(s)
}

// AutoQuote returns s or, if quoting is required for s to appear in a gop.mod,
// the quotation of s.
func AutoQuote(s string) string {
	return modfile.AutoQuote(s)
}

var (
	symbolRE = regexp.MustCompile("\\*?[A-Z]\\w*")
)

// TODO: to be optimized
func parseSymbol(s *string) (t string, err error) {
	t, err = parseString(s)
	if err != nil {
		goto failed
	}
	if symbolRE.MatchString(t) {
		return
	}
	err = errors.New("invalid Go export symbol format")
failed:
	return "", &InvalidSymbolError{
		Sym: *s,
		Err: err,
	}
}

func parseString(s *string) (string, error) {
	t := *s
	if strings.HasPrefix(t, `"`) {
		var err error
		if t, err = strconv.Unquote(t); err != nil {
			return "", err
		}
	} else if strings.ContainsAny(t, "\"'`") {
		// Other quotes are reserved both for possible future expansion
		// and to avoid confusion. For example if someone types 'x'
		// we want that to be a syntax error and not a literal x in literal quotation marks.
		return "", fmt.Errorf("unquoted string cannot contain quote")
	}
	*s = AutoQuote(t)
	return t, nil
}

func parseStrings(args []string) (arr []string, err error) {
	arr = make([]string, len(args))
	for i := range args {
		if arr[i], err = parseString(&args[i]); err != nil {
			return
		}
	}
	return
}

func parsePkgPaths(args []string) (paths []string, err error) {
	if paths, err = parseStrings(args); err != nil {
		return nil, fmt.Errorf("invalid quoted string: %v", err)
	}
	for _, pkg := range paths {
		if !isPkgPath(pkg) {
			return nil, fmt.Errorf(`"%s" is not a valid package path`, pkg)
		}
	}
	return
}

func isPkgPath(s string) bool {
	return s != "" && (s[0] != '.' && s[0] != '_')
}

type InvalidExtError struct {
	Ext string
	Err error
}

func (e *InvalidExtError) Error() string {
	return fmt.Sprintf("ext %s invalid: %s", e.Ext, e.Err)
}

func (e *InvalidExtError) Unwrap() error { return e.Err }

type InvalidSymbolError struct {
	Sym string
	Err error
}

func (e *InvalidSymbolError) Error() string {
	return fmt.Sprintf("symbol %s invalid: %s", e.Sym, e.Err)
}

func (e *InvalidSymbolError) Unwrap() error { return e.Err }

type ErrorList = errors.List
type Error modfile.Error

func (p *Error) Error() string {
	return (*modfile.Error)(p).Error()
}

func (p *Error) Unwrap() error {
	return p.Err
}

func (p *Error) Summary() string {
	cpy := *(*modfile.Error)(p)
	cpy.Err = errors.New(errors.Summary(p.Unwrap()))
	return cpy.Error()
}

// -----------------------------------------------------------------------------

const (
	directiveInvalid = iota
	directiveModule
	directiveGop
	directiveProject
	directiveClass
)

const (
	directiveLineBlock = 0x80 + iota
	directiveImport
)

var directiveWeights = map[string]int{
	"module":   directiveModule,
	"gop":      directiveGop,
	"import":   directiveImport,
	"register": directiveImport, // register => import
	"project":  directiveProject,
	"class":    directiveClass,
}

func getWeight(e Expr) int {
	if line, ok := e.(*Line); ok {
		return directiveWeights[line.Token[0]]
	}
	if w, ok := directiveWeights[e.(*LineBlock).Token[0]]; ok {
		return w
	}
	return directiveLineBlock
}

func updateLine(line *Line, tokens ...string) {
	if line.InBlock {
		tokens = tokens[1:]
	}
	line.Token = tokens
}

func addLine(x *FileSyntax, tokens ...string) *Line {
	new := &Line{Token: tokens}
	w := directiveWeights[tokens[0]]
	for i, e := range x.Stmt {
		w2 := getWeight(e)
		if w <= w2 {
			x.Stmt = append(x.Stmt, nil)
			copy(x.Stmt[i+1:], x.Stmt[i:])
			x.Stmt[i] = new
			return new
		}
	}
	x.Stmt = append(x.Stmt, new)
	return new
}

func (f *File) AddGopStmt(version string) error {
	if !modfile.GoVersionRE.MatchString(version) {
		return fmt.Errorf("invalid language version string %q", version)
	}
	if f.Gop == nil {
		if f.Syntax == nil {
			f.Syntax = new(FileSyntax)
		}
		f.Gop = &Gop{
			Version: version,
			Syntax:  addLine(f.Syntax, "gop", version),
		}
	} else {
		f.Gop.Version = version
		updateLine(f.Gop.Syntax, "gop", version)
	}
	return nil
}

func (f *File) AddImport(modPath string) {
	for _, r := range f.Import {
		if r.ClassfileMod == modPath {
			return
		}
	}
	f.AddNewImport(modPath)
}

func (f *File) AddNewImport(modPath string) {
	line := addLine(f.Syntax, "import", AutoQuote(modPath))
	r := &Import{
		ClassfileMod: modPath,
		Syntax:       line,
	}
	f.Import = append(f.Import, r)
}

func (f *File) Format() ([]byte, error) {
	return modfile.Format(f.Syntax), nil
}

// -----------------------------------------------------------------------------
