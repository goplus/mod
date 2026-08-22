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

type Compiler struct {
	Name    string
	Version string
}

// A File is the parsed, interpreted form of a gox.mod file.
type File struct {
	XGo       *XGo
	Compiler  *Compiler // the underlying go compiler in go.mod (not gox.mod)
	Projects  []*Project
	ClassMods []string // calc by require statements in go.mod (not gox.mod)

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

// A XGo is the xgo statement.
type XGo = modfile.Go

// A Class is the work class statement.
type Class struct {
	Ext      string // can be "_[class].gox" or ".[class]", eg. "_yap.gox" or ".spx"
	FullExt  string // can be "*_[class].gox", "_[class].gox", "*.[class]" or ".[class]"
	Class    string // "Sprite"
	Proto    string // prototype of the work class (not empty if multiple work classes)
	Prefix   string // prefix of the work class
	Embedded bool   // if true, the class instance will be embedded in the project
	Syntax   *Line
}

// A Import is the import statement.
type Import struct {
	Name   string // maybe empty
	Path   string
	Syntax *Line
}

// A Pack is the pack statement.
type Pack struct {
	Directory string // relative path to the project root, no ".." components
	IndexFile string // plain file name (e.g. "index.json"), no path separators
	Syntax    *Line
}

// Driver declares a project's driver package and protocol version.
type Driver struct {
	Protocol string
	Package  string
	Syntax   *Line
}

// A Project is the project statement.
type Project struct {
	Ext      string    // can be "_[class].gox" or ".[class]", eg. "_yap.gox" or ".gmx"
	FullExt  string    // can be "main_[class].gox", "*_[class].gox", "_[class].gox", "main.[class]", "*.[class]" or ".[class]"
	Class    string    // "Game"
	Works    []*Class  // work class of classfile
	PkgPaths []string  // package paths of classfile and optional inline-imported packages.
	Import   []*Import // auto-imported packages
	Pack     *Pack     // pack directive (at most one per project)
	Driver   *Driver   // project driver

	// AutoLambdas maps command => number of parameters before auto lambda.
	// See https://github.com/goplus/xgo/issues/2828.
	AutoLambdas map[string]int

	// if Flat is true, all matching files are fragments of the project class (no work classfiles).
	Flat bool

	Syntax *Line
}

// IsProj checks if a (ext, fname) pair is a project file or not.
func (p *Project) IsProj(ext, fname string) bool {
	if p.Flat {
		return fname == "main"+ext
	}
	for _, w := range p.Works {
		if w.Ext == ext {
			if ext != p.Ext || fname != "main"+ext {
				return false
			}
			break
		}
	}
	return true
}

func New(goxmod, xgoVer string) *File {
	xgo := &Line{
		Token: []string{"xgo", xgoVer},
	}
	return &File{
		XGo: &XGo{
			Version: xgoVer,
			Syntax:  xgo,
		},
		Syntax: &FileSyntax{
			Name: goxmod,
			Stmt: []Expr{xgo},
		},
	}
}

type VersionFixer = modfile.VersionFixer

// Parse parses and returns a gox.mod file.
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
// It is used when parsing gox.mod files other than the main module,
// under the theory that most statement types we add in the future will
// only apply in the main module, like exclude and replace,
// and so we get better gradual deployments if old go commands
// simply ignore those statements when found in gox.mod files
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
			if verb == "driver" && len(x.Line) == 0 {
				parsed.parseVerb(&errs, verb, &Line{Comments: x.Comments, Start: x.Start, End: x.RParen.Pos, Token: x.Token, InBlock: true}, nil, strict)
				continue
			}
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
	case "xgo", "gop":
		if f.XGo != nil {
			errorf("repeated xgo statement")
			return
		}
		if len(args) != 1 {
			errorf("xgo directive expects exactly one argument")
			return
		} else if !modfile.GoVersionRE.MatchString(args[0]) {
			errorf("invalid xgo version '%s': must match format 1.23", args[0])
			return
		}
		f.XGo = &XGo{Syntax: line}
		f.XGo.Version = args[0]
	case "project":
		if len(args) < 1 {
			errorf("usage: project [-flat] [*.projExt ProjectClass] classFilePkgPath ...")
			return
		}
		flat := false
		for len(args) > 0 && strings.HasPrefix(args[0], "-") {
			sw := args[0][1:]
			if sw == "flat" {
				flat = true
			} else {
				errorf("unknown flag: -%s\nusage: project [-flat] [*.projExt ProjectClass] classFilePkgPath ...", sw)
				return
			}
			args = args[1:]
		}
		if len(args) < 1 {
			errorf("usage: project [-flat] [*.projExt ProjectClass] classFilePkgPath ...")
			return
		}
		if isExt(args[0], true) {
			if len(args) < 3 || strings.Contains(args[1], "/") {
				errorf("usage: project [-flat] [*.projExt ProjectClass] classFilePkgPath ...")
				return
			}
			ext, fullExt, err := parseExt(&args[0], true)
			if err != nil {
				wrapError(err)
				return
			}
			class, err := parseType(&args[1])
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
				Ext: ext, FullExt: fullExt, Class: class, PkgPaths: pkgPaths, Flat: flat, Syntax: line,
			})
			return
		}
		pkgPaths, err := parsePkgPaths(args)
		if err != nil {
			wrapError(err)
			return
		}
		f.addProj(&Project{
			PkgPaths: pkgPaths, Flat: flat, Syntax: line,
		})
	case "class":
		proj := f.proj()
		if proj == nil {
			errorf("work class must declare after a project definition")
			return
		}
		if proj.Flat {
			errorf("class directive is not allowed in a flat mode project")
			return
		}
		prefix := ""
		embedded := false
		for len(args) > 0 && strings.HasPrefix(args[0], "-") {
			sw := args[0][1:]
			if sw == "embed" {
				embedded = true
			} else if strings.HasPrefix(sw, "prefix=") {
				prefix = sw[7:]
			} else {
				errorf(`unknown flag: -%s
usage: class [-embed -prefix=Prefix] *.workExt WorkClass [WorkPrototype]`, sw)
				return
			}
			args = args[1:]
		}
		if len(args) < 2 {
			errorf("usage: class [-embed -prefix=Prefix] *.workExt WorkClass [WorkPrototype]")
			return
		}
		workExt, fullExt, err := parseExt(&args[0], false)
		if err != nil {
			wrapError(err)
			return
		}
		class, err := parseType(&args[1])
		if err != nil {
			wrapError(err)
			return
		}
		protoClass := ""
		if len(args) > 2 {
			protoClass, err = parseType(&args[2])
			if err != nil {
				wrapError(err)
				return
			}
		}
		proj.Works = append(proj.Works, &Class{
			Ext:      workExt,
			FullExt:  fullExt,
			Class:    class,
			Proto:    protoClass,
			Prefix:   prefix,
			Embedded: embedded,
			Syntax:   line,
		})
	case "import":
		proj := f.proj()
		if proj == nil {
			errorf("import must declare after a project definition")
			return
		}
		var name string
		switch len(args) {
		case 2:
			v, err := parseString(&args[0])
			if err != nil {
				wrapError(err)
				return
			}
			name = v
			args = args[1:]
			fallthrough
		case 1:
			pkgPath, err := parsePkgPath(&args[0])
			if err != nil {
				wrapError(err)
				return
			}
			proj.Import = append(proj.Import, &Import{Name: name, Path: pkgPath, Syntax: line})
		default:
			errorf("usage: import [name] pkgPath")
			return
		}
	case "pack":
		proj := f.proj()
		if proj == nil {
			errorf("pack must declare after a project definition")
			return
		}
		if proj.Pack != nil {
			errorf("duplicate pack directive in the same project")
			return
		}
		if len(args) != 2 {
			errorf("usage: pack <directory> <indexfile>")
			return
		}
		dir, err := parseString(&args[0])
		if err != nil {
			wrapError(err)
			return
		}
		if strings.HasPrefix(dir, "/") || strings.HasPrefix(dir, "\\") || strings.Contains(dir, "..") {
			errorf(`pack directory must be a relative path and ".." is not allowed`)
			return
		}
		indexFile, err := parseString(&args[1])
		if err != nil {
			wrapError(err)
			return
		}
		if strings.ContainsAny(indexFile, "/\\") {
			errorf("pack indexfile must be a plain file name without path separators")
			return
		}
		proj.Pack = &Pack{Directory: dir, IndexFile: indexFile, Syntax: line}
	case "driver":
		if line.InBlock {
			errorf("driver directive must not be a block")
			return
		}
		proj := f.proj()
		if proj == nil {
			errorf("driver must declare after a project definition")
			return
		}
		if proj.Driver != nil {
			errorf("duplicate driver directive in the same project")
			return
		}
		if len(args) != 2 {
			errorf("usage: driver <protocol> <package>")
			return
		}
		protocol, err := parseString(&args[0])
		if err != nil {
			wrapError(err)
			return
		}
		if !driverProtocolRE.MatchString(protocol) {
			errorf("driver protocol must match v[1-9][0-9]*, got %q", protocol)
			return
		}
		pkgPath, err := parseString(&args[1])
		if err != nil {
			wrapError(err)
			return
		}
		if err := module.CheckImportPath(pkgPath); err != nil {
			errorf("driver package %q is not a valid import path: %v", pkgPath, err)
			return
		}
		proj.Driver = &Driver{Protocol: protocol, Package: pkgPath, Syntax: line}
	case "autolambda":
		proj := f.proj()
		if proj == nil {
			errorf("autolambda must declare after a project definition")
			return
		}
		if len(args) == 0 {
			errorf("usage: autolambda name(n), ...")
			return
		}
		if proj.AutoLambdas == nil {
			proj.AutoLambdas = make(map[string]int)
		}
		err := parseAutoLambdas(proj.AutoLambdas, args)
		if err != nil {
			wrapError(err)
			return
		}
	default:
		if strict {
			errorf("unknown directive: %s", verb)
		}
	}
}

// parseAutoLambdas parses the argument tokens of an autolambda directive into a
// list of `name(n)` entries. The gox.mod tokenizer splits parentheses and commas
// into their own tokens, so a directive like:
//
//	autolambda times(1), forEver(0), onKey(1)
//
// arrives here as: [times ( 1 ) , forEver ( 0 ) , onKey ( 1 )].
func parseAutoLambdas(ret map[string]int, args []string) error {
	i, n := 0, len(args)
	for i < n {
		name, err := parseIdent(&args[i])
		if err != nil {
			return fmt.Errorf("autolambda: invalid command name %q", args[i])
		}
		i++
		if i >= n || args[i] != "(" {
			return fmt.Errorf("autolambda %s: expect '(' after command name", name)
		}
		i++
		nArgs, e := strconv.Atoi(args[i])
		if e != nil || nArgs < 0 {
			return fmt.Errorf("autolambda %s: invalid number of arguments %q", name, args[i])
		}
		i++
		if i >= n || args[i] != ")" {
			return fmt.Errorf("autolambda %s: expect ')' after number of arguments", name)
		}
		i++
		if _, ok := ret[name]; ok {
			return fmt.Errorf("autolambda: duplicate command %q", name)
		}
		ret[name] = nArgs
		if i < n {
			if args[i] != "," {
				return fmt.Errorf("autolambda: expect ',' between entries, got %q", args[i])
			}
			i++
			if i >= n {
				return fmt.Errorf("autolambda: trailing ',' without an entry")
			}
		}
	}
	return nil
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
// a single token in a gox.mod line.
func MustQuote(s string) bool {
	return modfile.MustQuote(s)
}

// AutoQuote returns s or, if quoting is required for s to appear in a gox.mod,
// the quotation of s.
func AutoQuote(s string) string {
	return modfile.AutoQuote(s)
}

var (
	typeRE           = regexp.MustCompile(`\*?[A-Z]\w*`)
	idenRE           = regexp.MustCompile(`\w+`)
	driverProtocolRE = regexp.MustCompile(`^v[1-9][0-9]*$`)
)

// TODO(xsw): to be optimized
func parseType(s *string) (t string, err error) {
	t, err = parseString(s)
	if err != nil {
		goto failed
	}
	if typeRE.MatchString(t) {
		return
	}
	err = errors.New("invalid Go export type")
failed:
	return "", &InvalidSymbolError{
		Sym: *s,
		Err: err,
	}
}

func parseIdent(s *string) (t string, err error) {
	t, err = parseString(s)
	if err != nil {
		goto failed
	}
	if idenRE.MatchString(t) {
		return
	}
	err = errors.New("invalid XGo identifier")
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

func parsePkgPath(s *string) (path string, err error) {
	if path, err = parseString(s); err != nil {
		err = fmt.Errorf("invalid quoted string: %v", err)
		return
	}
	if !isPkgPath(path) {
		err = fmt.Errorf(`"%s" is not a valid package path`, path)
	}
	return
}

func parsePkgPaths(args []string) (paths []string, err error) {
	paths = make([]string, len(args))
	for i := range args {
		if paths[i], err = parsePkgPath(&args[i]); err != nil {
			return
		}
	}
	return
}

func isPkgPath(s string) bool {
	return s != "" && (s[0] != '.' && s[0] != '_')
}

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
