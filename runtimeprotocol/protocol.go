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

// Package runtimeprotocol defines the provider request model and argv codec.
// Validation is structural; consumers verify identity-bearing paths.
package runtimeprotocol

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
	// Version1 is the gox.mod runtime protocol value.
	Version1 = "v1"
	// PreambleV1 is the first argv element passed to a v1 provider.
	PreambleV1 = "xgo-runtime-v1"
)

// Action identifies the requested provider operation.
type Action string

const (
	ActionRun   Action = "run"
	ActionBuild Action = "build"
)

// Pack describes optional project pack metadata.
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

// Request is one provider request; run has no Output, build has no ApplicationArgs.
type Request struct {
	Version         string
	Action          Action
	Project         Project
	ProviderPackage string
	ProviderOrigin  xgomod.ResolvedModule
	Declaration     xgomod.FileIdentity
	Graph           Graph
	BuildFlags      []string
	Output          *BuildOutput
	ApplicationArgs []string
}

// Validate checks the request without reading the filesystem.
func (r Request) Validate() error {
	if r.Version != Version1 {
		return fmt.Errorf("runtimeprotocol: unsupported version %q", r.Version)
	}
	if r.Action != ActionRun && r.Action != ActionBuild {
		return fmt.Errorf("runtimeprotocol: unsupported action %q", r.Action)
	}
	for _, item := range []struct {
		name  string
		value string
	}{
		{"project-dir", r.Project.Dir},
		{"project-file", r.Project.File},
		{"module-root", r.Project.ModuleRoot},
		{"declaration-file", r.Declaration.Path},
		{"go-command", r.Graph.GoCommand},
		{"graph-work-dir", r.Graph.WorkDir},
	} {
		if err := validateAbsolutePath(item.name, item.value); err != nil {
			return err
		}
	}
	if filepath.Dir(r.Project.File) != r.Project.Dir {
		return fmt.Errorf("runtimeprotocol: project-file must be a top-level file in project-dir")
	}
	if !pathWithin(r.Project.ModuleRoot, r.Project.Dir) {
		return fmt.Errorf("runtimeprotocol: project-dir must be within module-root")
	}
	if r.Project.Extension == "" || strings.IndexByte(r.Project.Extension, 0) >= 0 {
		return fmt.Errorf("runtimeprotocol: project extension may not be empty or contain NUL")
	}
	if r.Project.FullExtension == "" || strings.IndexByte(r.Project.FullExtension, 0) >= 0 {
		return fmt.Errorf("runtimeprotocol: project full extension may not be empty or contain NUL")
	}
	if r.Project.Pack != nil {
		if err := validatePackDirectory(r.Project.Pack.Directory); err != nil {
			return err
		}
		if err := validatePackIndex(r.Project.Pack.IndexFile); err != nil {
			return err
		}
	}
	if err := validateProviderOrigin(r.ProviderOrigin); err != nil {
		return fmt.Errorf("runtimeprotocol: provider origin: %w", err)
	}
	if err := validateSHA256("declaration-sha256", r.Declaration.SHA256); err != nil {
		return err
	}
	effective := r.ProviderOrigin.Effective()
	declarationBase := filepath.Base(r.Declaration.Path)
	if filepath.Dir(r.Declaration.Path) != effective.Dir || (declarationBase != "gox.mod" && declarationBase != "gop.mod") {
		return fmt.Errorf("runtimeprotocol: declaration-file must be provider metadata (gox.mod or gop.mod) in %q", effective.Dir)
	}
	if err := module.CheckImportPath(r.ProviderPackage); err != nil {
		return fmt.Errorf("runtimeprotocol: invalid provider package %q: %w", r.ProviderPackage, err)
	}
	if !moduleContainsPackage(r.ProviderOrigin.Selected.Path, r.ProviderPackage) {
		return fmt.Errorf("runtimeprotocol: provider package %q is outside selected module %q", r.ProviderPackage, r.ProviderOrigin.Selected.Path)
	}
	if r.Graph.GoWork != "off" {
		if err := validateAbsolutePath("go-work", r.Graph.GoWork); err != nil {
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
			return fmt.Errorf("runtimeprotocol: application argument contains NUL")
		}
	}
	switch r.Action {
	case ActionRun:
		if r.Output != nil {
			return fmt.Errorf("runtimeprotocol: run request cannot contain output paths")
		}
	case ActionBuild:
		if r.Output == nil {
			return fmt.Errorf("runtimeprotocol: build request requires output paths")
		}
		if len(r.ApplicationArgs) != 0 {
			return fmt.Errorf("runtimeprotocol: build request cannot contain application arguments")
		}
		if err := validateAbsolutePath("output", r.Output.Staging); err != nil {
			return err
		}
		if err := validateAbsolutePath("final-output", r.Output.Final); err != nil {
			return err
		}
		if r.Output.Staging == r.Output.Final {
			return fmt.Errorf("runtimeprotocol: output and final-output must be different paths")
		}
	}
	return nil
}

func validateProviderOrigin(origin xgomod.ResolvedModule) error {
	return origin.ValidateSyntax()
}

func validateAbsolutePath(name, value string) error {
	if value == "" || strings.IndexByte(value, 0) >= 0 {
		return fmt.Errorf("runtimeprotocol: path --%s may not be empty or contain NUL", name)
	}
	if !filepath.IsAbs(value) {
		return fmt.Errorf("runtimeprotocol: path --%s must be absolute: %q", name, value)
	}
	if filepath.Clean(value) != value {
		return fmt.Errorf("runtimeprotocol: path --%s must be clean: %q", name, value)
	}
	return nil
}

func validatePackDirectory(value string) error {
	if value == "" || strings.Contains(value, "\\") || strings.IndexByte(value, 0) >= 0 || path.IsAbs(value) || path.Clean(value) != value {
		return fmt.Errorf("runtimeprotocol: pack directory must be a clean non-empty relative slash path: %q", value)
	}
	if value == ".." || strings.HasPrefix(value, "../") {
		return fmt.Errorf("runtimeprotocol: pack directory escapes the project: %q", value)
	}
	return nil
}

func validatePackIndex(value string) error {
	if value == "" || value == "." || value == ".." || strings.ContainsAny(value, "/\\\x00") {
		return fmt.Errorf("runtimeprotocol: pack index must be a plain file name: %q", value)
	}
	return nil
}

func validateSHA256(name, value string) error {
	if len(value) != 64 {
		return fmt.Errorf("runtimeprotocol: --%s must contain 64 hexadecimal characters", name)
	}
	if _, err := hex.DecodeString(value); err != nil {
		return fmt.Errorf("runtimeprotocol: --%s is not a SHA-256 digest: %w", name, err)
	}
	if value != strings.ToLower(value) {
		return fmt.Errorf("runtimeprotocol: --%s must use lowercase hexadecimal", name)
	}
	return nil
}

func validateGraphFlags(flags []string) error {
	return validateFlags("graph", flags, func(name, value string) error {
		switch name {
		case "mod":
			if value != "mod" && value != "readonly" && value != "vendor" {
				return fmt.Errorf("runtimeprotocol: graph flag -mod has unsupported value %q", value)
			}
		case "modfile", "overlay":
			if err := validateAbsolutePath("graph flag -"+name, value); err != nil {
				return err
			}
		default:
			return fmt.Errorf("runtimeprotocol: graph flag -%s is not supported", name)
		}
		return nil
	})
}

func validateBuildFlags(flags []string) error {
	return validateFlags("build", flags, func(name, value string) error {
		switch name {
		case "v", "x", "work", "trimpath":
			if value != "true" {
				return fmt.Errorf("runtimeprotocol: build flag -%s has unsupported value %q", name, value)
			}
		case "buildvcs":
			if value != "false" {
				return fmt.Errorf("runtimeprotocol: build flag -buildvcs has unsupported value %q", value)
			}
		default:
			return fmt.Errorf("runtimeprotocol: build flag -%s is not supported", name)
		}
		return nil
	})
}

func validateFlags(kind string, flags []string, validateValue func(name, value string) error) error {
	seen := make(map[string]struct{}, len(flags))
	for _, flag := range flags {
		name, value, ok := splitCanonicalFlag(flag)
		if !ok {
			return fmt.Errorf("runtimeprotocol: %s flag %q must use -name=value", kind, flag)
		}
		if _, duplicate := seen[name]; duplicate {
			return fmt.Errorf("runtimeprotocol: %s flag -%s may not be repeated", kind, name)
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
