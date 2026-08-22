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

package driverprotocol

import (
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/mod/module"
)

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
	if err := validateSHA256("declaration-sha256", r.Declaration.SHA256); err != nil {
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
		if err := validateAbsolutePath("output", r.Output.Staging); err != nil {
			return err
		}
		if err := validateAbsolutePath("final-output", r.Output.Final); err != nil {
			return err
		}
		if r.Output.Staging == r.Output.Final {
			return fmt.Errorf("driverprotocol: output and final-output must be different paths")
		}
	}
	return nil
}
