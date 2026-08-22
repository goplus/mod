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
// Validation is structural; consumers verify identity-bearing paths.
package driverprotocol

import (
	"fmt"

	"github.com/goplus/mod/xgomod"
)

const (
	// Version1 is the gox.mod driver protocol value.
	Version1 = "v1"
	// PreambleV1 is the first argv element passed to a v1 driver.
	PreambleV1 = "xgo-driver-v1"
)

// Action identifies the requested driver operation.
type Action string

const (
	ActionRun   Action = "run"
	ActionBuild Action = "build"
)

// Validate reports whether the action is supported by the v1 protocol.
func (a Action) Validate() error {
	if a != ActionRun && a != ActionBuild {
		return fmt.Errorf("driverprotocol: unsupported action %q", a)
	}
	return nil
}

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

// Request is one driver request; run has no Output, build has no ApplicationArgs.
type Request struct {
	Version         string
	Action          Action
	Project         Project
	DriverPackage   string
	DriverOrigin    xgomod.ResolvedModule
	Declaration     xgomod.FileIdentity
	Graph           Graph
	BuildFlags      []string
	Output          *BuildOutput
	ApplicationArgs []string
}
