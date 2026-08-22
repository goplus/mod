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

package xgomod

import (
	"github.com/goplus/mod/modfile"
	"github.com/goplus/mod/modload"
)

// ModuleRef identifies a logical selection or its effective source.
type ModuleRef struct {
	Path    string
	Version string
	Dir     string
	GoMod   string
}

// ResolvedModule separates selection from replacement source paths.
type ResolvedModule struct {
	Selected ModuleRef
	Replace  *ModuleRef
	Main     bool
}

// Effective returns the source used for files and metadata.
func (m ResolvedModule) Effective() ModuleRef {
	if m.Replace != nil {
		return *m.Replace
	}
	return m.Selected
}

// Equal reports whether two resolved module identities are identical.
func (m ResolvedModule) Equal(other ResolvedModule) bool {
	if m.Main != other.Main || m.Selected != other.Selected {
		return false
	}
	if m.Replace == nil || other.Replace == nil {
		return m.Replace == nil && other.Replace == nil
	}
	return *m.Replace == *other.Replace
}

// IsLocal reports whether the module uses filesystem source.
func (m ResolvedModule) IsLocal() bool {
	return m.Main || m.Replace != nil && m.Replace.Version == ""
}

// Validate checks the resolved module identity and its effective source.
func (m ResolvedModule) Validate() error {
	return validateResolvedModule(m)
}

// ValidateSyntax checks identity and path spelling without filesystem access.
func (m ResolvedModule) ValidateSyntax() error {
	return validateResolvedModuleSyntax(m)
}

// ResolvedClassGraph is XGo's resolved graph snapshot; it is not rediscovered.
type ResolvedClassGraph struct {
	Target ResolvedModule
	// ClassModules follows class-marked require order; order controls registration precedence.
	ClassModules  []ResolvedModule
	TargetModFile FileIdentity
}

// FileIdentity binds metadata to the exact bytes parsed by the caller.
type FileIdentity = modload.FileIdentity

// ProjectInfo pairs class metadata with its origin; built-ins omit provenance.
type ProjectInfo struct {
	Project     *modfile.Project
	Origin      *ResolvedModule
	Declaration FileIdentity
	RequiredXGo string
}
