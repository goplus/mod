/*
 * Copyright (c) 2026 The XGo Authors (xgo.dev). All rights reserved.
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

// Package driverprotocol defines the driver request model and argv codec.
// Validation is structural; consumers verify identity-bearing paths. The
// canonical wire and provenance contract is documented in spec-v1.md.
package driverprotocol

import (
	"encoding/hex"
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/goplus/mod/xgomod"
	"golang.org/x/mod/module"
)

const (
	// Version1 is the v1 driver protocol version string.
	Version1 = "v1"
	// PreambleV1 is the first argv element passed to a v1 driver.
	PreambleV1 = "xgo-driver-v1"
)

// Action identifies the requested driver operation.
type Action string

const (
	// ActionRun requests that the driver run a project.
	ActionRun Action = "run"
	// ActionBuild requests that the driver build a project.
	ActionBuild Action = "build"
)

// Validate reports whether the action is supported by the v1 protocol.
func (a Action) Validate() error {
	if a != ActionRun && a != ActionBuild {
		return fmt.Errorf("driverprotocol: unsupported action %q", a)
	}
	return nil
}

// Pack describes optional project pack metadata. Directory is either "." or a
// portable relative slash path; IndexFile is a portable plain file name.
type Pack struct {
	Directory string
	IndexFile string
}

// Project is the project snapshot discovered by XGo.
type Project struct {
	Dir           string
	File          string
	ModuleRoot    string
	Extension     string
	FullExtension string
	Pack          *Pack
}

// Graph carries the Go command and workspace policy used for discovery.
type Graph struct {
	GoCommand string
	WorkDir   string
	GoWork    string
	Flags     []string
}

// BuildOutput contains staging and final output paths.
type BuildOutput struct {
	Staging string
	Final   string
}

// Request is one driver request; run has no Output, build has no ApplicationArgs.
// Declaration and TargetModFile carry claimed identities; Parse does not verify
// them against filesystem contents.
type Request struct {
	Version         string
	Action          Action
	Project         Project
	DriverPackage   string
	DriverOrigin    xgomod.ResolvedModule
	Declaration     xgomod.FileIdentity
	TargetModFile   xgomod.FileIdentity
	Graph           Graph
	BuildFlags      []string
	Output          *BuildOutput
	ApplicationArgs []string
}

// Validate checks the request without reading the filesystem.
func (r Request) Validate() error {
	if r.Version != Version1 {
		return fmt.Errorf("driverprotocol: unsupported version %q", r.Version)
	}
	if err := r.Action.Validate(); err != nil {
		return err
	}
	for _, item := range []struct {
		name  string
		value string
	}{
		{optionProjectDir, r.Project.Dir},
		{optionProjectFile, r.Project.File},
		{optionModuleRoot, r.Project.ModuleRoot},
		{optionDeclarationFile, r.Declaration.Path},
		{optionTargetModFile, r.TargetModFile.Path},
		{optionGoCommand, r.Graph.GoCommand},
		{optionGraphWorkDir, r.Graph.WorkDir},
	} {
		if err := validateAbsolutePath(item.name, item.value); err != nil {
			return err
		}
	}
	if filepath.Dir(r.Project.File) != r.Project.Dir {
		return fmt.Errorf("driverprotocol: project-file must be a top-level file in project-dir")
	}
	if !pathWithin(r.Project.ModuleRoot, r.Project.Dir) {
		return fmt.Errorf("driverprotocol: project-dir must be within module-root")
	}
	if r.Project.Extension == "" || strings.IndexByte(r.Project.Extension, 0) >= 0 {
		return fmt.Errorf("driverprotocol: project extension may not be empty or contain NUL")
	}
	if r.Project.FullExtension == "" || strings.IndexByte(r.Project.FullExtension, 0) >= 0 {
		return fmt.Errorf("driverprotocol: project full extension may not be empty or contain NUL")
	}
	if r.Project.Pack != nil {
		if err := validatePackDirectory(r.Project.Pack.Directory); err != nil {
			return err
		}
		if err := validatePackIndex(r.Project.Pack.IndexFile); err != nil {
			return err
		}
	}
	if err := r.DriverOrigin.ValidateSyntax(); err != nil {
		return fmt.Errorf("driverprotocol: driver origin: %w", err)
	}
	if err := validateSHA256(optionDeclarationSHA256, r.Declaration.SHA256); err != nil {
		return err
	}
	if err := validateSHA256(optionTargetModFileSHA256, r.TargetModFile.SHA256); err != nil {
		return err
	}
	effective := r.DriverOrigin.Effective()
	declarationBase := filepath.Base(r.Declaration.Path)
	if filepath.Dir(r.Declaration.Path) != effective.Dir || (declarationBase != "gox.mod" && declarationBase != "gop.mod") {
		return fmt.Errorf("driverprotocol: declaration-file must be driver metadata (gox.mod or gop.mod) in %q", effective.Dir)
	}
	if err := module.CheckImportPath(r.DriverPackage); err != nil {
		return fmt.Errorf("driverprotocol: invalid driver package %q: %w", r.DriverPackage, err)
	}
	if !moduleContainsPackage(r.DriverOrigin.Selected.Path, r.DriverPackage) {
		return fmt.Errorf("driverprotocol: driver package %q is outside selected module %q", r.DriverPackage, r.DriverOrigin.Selected.Path)
	}
	if r.Graph.GoWork != "off" {
		if err := validateAbsolutePath(optionGoWork, r.Graph.GoWork); err != nil {
			return err
		}
	}
	if err := validateGraphFlags(r.Graph.Flags); err != nil {
		return err
	}
	if err := validateBuildFlags(r.BuildFlags); err != nil {
		return err
	}
	for _, arg := range r.ApplicationArgs {
		if strings.IndexByte(arg, 0) >= 0 {
			return fmt.Errorf("driverprotocol: application argument contains NUL")
		}
	}
	switch r.Action {
	case ActionRun:
		if r.Output != nil {
			return fmt.Errorf("driverprotocol: run request cannot contain output paths")
		}
	case ActionBuild:
		if r.Output == nil {
			return fmt.Errorf("driverprotocol: build request requires output paths")
		}
		if len(r.ApplicationArgs) != 0 {
			return fmt.Errorf("driverprotocol: build request cannot contain application arguments")
		}
		if err := validateAbsolutePath(optionOutput, r.Output.Staging); err != nil {
			return err
		}
		if err := validateAbsolutePath(optionFinalOutput, r.Output.Final); err != nil {
			return err
		}
		if r.Output.Staging == r.Output.Final {
			return fmt.Errorf("driverprotocol: output and final-output must be different paths")
		}
	}
	return nil
}

func validateAbsolutePath(name, value string) error {
	if value == "" || strings.IndexByte(value, 0) >= 0 {
		return fmt.Errorf("driverprotocol: path --%s may not be empty or contain NUL", name)
	}
	if !filepath.IsAbs(value) {
		return fmt.Errorf("driverprotocol: path --%s must be absolute: %q", name, value)
	}
	if filepath.Clean(value) != value {
		return fmt.Errorf("driverprotocol: path --%s must be clean: %q", name, value)
	}
	return nil
}

func validatePackDirectory(value string) error {
	if value == "" || strings.ContainsAny(value, "\\\x00") || path.IsAbs(value) || path.Clean(value) != value {
		return fmt.Errorf("driverprotocol: pack directory must be a clean non-empty relative slash path: %q", value)
	}
	if value == ".." || strings.HasPrefix(value, "../") {
		return fmt.Errorf("driverprotocol: pack directory escapes the project: %q", value)
	}
	if value == "." {
		return nil
	}
	for _, element := range strings.Split(value, "/") {
		if !isPortablePathElement(element) {
			return fmt.Errorf("driverprotocol: pack directory contains non-portable path element %q", element)
		}
	}
	return nil
}

func validatePackIndex(value string) error {
	if strings.Contains(value, "/") || !isPortablePathElement(value) {
		return fmt.Errorf("driverprotocol: pack index must be a plain file name: %q", value)
	}
	return nil
}

func isPortablePathElement(value string) bool {
	if value == "" || strings.HasSuffix(value, ".") || strings.HasSuffix(value, " ") {
		return false
	}
	// Apply Windows rules because protocol paths must be portable.
	for _, char := range value {
		if char < ' ' || strings.ContainsRune(`<>:"\\|?*`, char) {
			return false
		}
	}
	return !isWindowsReservedName(value)
}

func isWindowsReservedName(value string) bool {
	name, _, _ := strings.Cut(value, ".")
	name = strings.ToUpper(strings.TrimRight(name, " "))
	switch name {
	case "CON", "PRN", "AUX", "NUL", "CLOCK$", "CONIN$", "CONOUT$":
		return true
	}
	if len(name) < 4 || (name[:3] != "COM" && name[:3] != "LPT") {
		return false
	}
	switch name[3:] {
	case "1", "2", "3", "4", "5", "6", "7", "8", "9", "\u00b9", "\u00b2", "\u00b3":
		return true
	}
	return false
}

func validateSHA256(name, value string) error {
	if len(value) != 64 {
		return fmt.Errorf("driverprotocol: --%s must contain 64 hexadecimal characters", name)
	}
	if _, err := hex.DecodeString(value); err != nil {
		return fmt.Errorf("driverprotocol: --%s is not a SHA-256 digest: %w", name, err)
	}
	if value != strings.ToLower(value) {
		return fmt.Errorf("driverprotocol: --%s must use lowercase hexadecimal", name)
	}
	return nil
}

func validateGraphFlags(flags []string) error {
	return validateFlags("graph", flags, func(name, value string) error {
		switch name {
		case "mod":
			if value != "mod" && value != "readonly" && value != "vendor" {
				return fmt.Errorf("driverprotocol: graph flag -mod has unsupported value %q", value)
			}
		case "modfile", "overlay":
			if err := validateAbsolutePath("graph flag -"+name, value); err != nil {
				return err
			}
		default:
			return fmt.Errorf("driverprotocol: graph flag -%s is not supported", name)
		}
		return nil
	})
}

func validateBuildFlags(flags []string) error {
	return validateFlags("build", flags, func(name, value string) error {
		switch name {
		case "v", "x", "work", "trimpath":
			if value != "true" {
				return fmt.Errorf("driverprotocol: build flag -%s has unsupported value %q", name, value)
			}
		case "buildvcs":
			if value != "false" {
				return fmt.Errorf("driverprotocol: build flag -buildvcs has unsupported value %q", value)
			}
		default:
			return fmt.Errorf("driverprotocol: build flag -%s is not supported", name)
		}
		return nil
	})
}

func validateFlags(kind string, flags []string, validateValue func(name, value string) error) error {
	seen := make(map[string]struct{}, len(flags))
	for _, flag := range flags {
		name, value, ok := splitCanonicalFlag(flag)
		if !ok {
			return fmt.Errorf("driverprotocol: %s flag %q must use -name=value", kind, flag)
		}
		if _, duplicate := seen[name]; duplicate {
			return fmt.Errorf("driverprotocol: %s flag -%s may not be repeated", kind, name)
		}
		seen[name] = struct{}{}
		if err := validateValue(name, value); err != nil {
			return err
		}
	}
	return nil
}

func splitCanonicalFlag(flag string) (name, value string, ok bool) {
	if len(flag) < 4 || flag[0] != '-' || flag[1] == '-' || strings.IndexByte(flag, 0) >= 0 {
		return "", "", false
	}
	name, value, ok = strings.Cut(flag[1:], "=")
	return name, value, ok && name != "" && value != ""
}

func pathWithin(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func moduleContainsPackage(modulePath, packagePath string) bool {
	return packagePath == modulePath || strings.HasPrefix(packagePath, modulePath+"/")
}
