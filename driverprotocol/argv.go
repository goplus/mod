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

	"github.com/goplus/mod/xgomod"
)

// Encode returns deterministic argv following the driver executable.
func Encode(request Request) ([]string, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	args := []string{
		PreambleV1,
		string(request.Action),
		option("project-dir", request.Project.Dir),
		option("project-file", request.Project.File),
		option("module-root", request.Project.ModuleRoot),
		option("driver-package", request.DriverPackage),
		option("selected-path", request.DriverOrigin.Selected.Path),
		option("selected-version", request.DriverOrigin.Selected.Version),
		option("origin-main", fmt.Sprint(request.DriverOrigin.Main)),
	}
	if request.DriverOrigin.Replace == nil {
		args = append(args,
			option("selected-dir", request.DriverOrigin.Selected.Dir),
			option("selected-gomod", request.DriverOrigin.Selected.GoMod),
		)
	} else {
		replacement := request.DriverOrigin.Replace
		args = append(args,
			option("replace-path", replacement.Path),
			option("replace-version", replacement.Version),
			option("replace-dir", replacement.Dir),
			option("replace-gomod", replacement.GoMod),
		)
	}
	args = append(args,
		option("project-ext", request.Project.Extension),
		option("project-full-ext", request.Project.FullExtension),
	)
	if request.Project.Pack != nil {
		args = append(args,
			option("pack-dir", request.Project.Pack.Directory),
			option("pack-index", request.Project.Pack.IndexFile),
		)
	}
	args = append(args,
		option("declaration-file", request.Declaration.Path),
		option("declaration-sha256", request.Declaration.SHA256),
		option("go-command", request.Graph.GoCommand),
		option("graph-work-dir", request.Graph.WorkDir),
		option("go-work", request.Graph.GoWork),
	)
	for _, flag := range request.Graph.Flags {
		args = append(args, option("graph-flag", flag))
	}
	for _, flag := range request.BuildFlags {
		args = append(args, option("build-flag", flag))
	}
	if request.Action == ActionRun {
		args = append(args, "--")
		args = append(args, request.ApplicationArgs...)
	} else {
		args = append(args,
			option("output", request.Output.Staging),
			option("final-output", request.Output.Final),
		)
	}
	return args, nil
}

// Parse decodes driver argv and rejects invalid structure; it does not authenticate referenced files.
func Parse(args []string) (Request, error) {
	var request Request
	if len(args) < 2 {
		return request, fmt.Errorf("driverprotocol: request requires preamble and action")
	}
	if args[0] != PreambleV1 {
		return request, fmt.Errorf("driverprotocol: unsupported preamble %q", args[0])
	}
	request.Version = Version1
	request.Action = Action(args[1])
	if err := request.Action.Validate(); err != nil {
		return Request{}, err
	}

	optionArgs := args[2:]
	if request.Action == ActionRun {
		delimiter := -1
		for i, arg := range optionArgs {
			if arg == "--" {
				delimiter = i
				break
			}
		}
		if delimiter < 0 {
			return Request{}, fmt.Errorf("driverprotocol: run requires -- before application arguments")
		}
		request.ApplicationArgs = append([]string(nil), optionArgs[delimiter+1:]...)
		optionArgs = optionArgs[:delimiter]
	} else {
		for _, arg := range optionArgs {
			if arg == "--" {
				return Request{}, fmt.Errorf("driverprotocol: build does not accept -- or positional arguments")
			}
		}
	}

	raw, err := parseOptions(optionArgs)
	if err != nil {
		return Request{}, err
	}
	for _, spec := range singularOptionSpecs {
		if _, ok := raw.values[spec.name]; spec.required && !ok {
			return Request{}, fmt.Errorf("driverprotocol: option --%s is required", spec.name)
		}
	}

	request.Project = Project{
		Dir:           raw.values["project-dir"],
		File:          raw.values["project-file"],
		ModuleRoot:    raw.values["module-root"],
		Extension:     raw.values["project-ext"],
		FullExtension: raw.values["project-full-ext"],
	}
	request.Declaration = xgomod.FileIdentity{
		Path: raw.values["declaration-file"], SHA256: raw.values["declaration-sha256"],
	}
	hasPack, completePack := optionGroup(raw.values, "pack-dir", "pack-index")
	if hasPack && !completePack {
		return Request{}, fmt.Errorf("driverprotocol: pack options must be supplied as a complete group")
	}
	if hasPack {
		request.Project.Pack = &Pack{Directory: raw.values["pack-dir"], IndexFile: raw.values["pack-index"]}
	}

	request.DriverPackage = raw.values["driver-package"]
	request.DriverOrigin = xgomod.ResolvedModule{
		Selected: xgomod.ModuleRef{
			Path:    raw.values["selected-path"],
			Version: raw.values["selected-version"],
		},
	}
	switch raw.values["origin-main"] {
	case "true":
		request.DriverOrigin.Main = true
	case "false":
	default:
		return Request{}, fmt.Errorf("driverprotocol: invalid --origin-main %q: expected true or false", raw.values["origin-main"])
	}
	request.DriverOrigin.Selected.Dir = raw.values["selected-dir"]
	request.DriverOrigin.Selected.GoMod = raw.values["selected-gomod"]
	hasSelected, _ := optionGroup(raw.values, "selected-dir", "selected-gomod")
	hasReplacement, completeReplacement := optionGroup(raw.values, replacementOptions...)
	if hasReplacement && !completeReplacement {
		return Request{}, fmt.Errorf("driverprotocol: replacement options must be supplied as a complete group")
	}
	if hasReplacement && hasSelected {
		return Request{}, fmt.Errorf("driverprotocol: origin with replacement forbids --selected-dir and --selected-gomod")
	}
	if hasReplacement {
		request.DriverOrigin.Replace = &xgomod.ModuleRef{
			Path:    raw.values["replace-path"],
			Version: raw.values["replace-version"],
			Dir:     raw.values["replace-dir"],
			GoMod:   raw.values["replace-gomod"],
		}
	}

	request.Graph = Graph{
		GoCommand: raw.values["go-command"],
		WorkDir:   raw.values["graph-work-dir"],
		GoWork:    raw.values["go-work"],
		Flags:     append([]string(nil), raw.graphFlags...),
	}
	request.BuildFlags = append([]string(nil), raw.buildFlags...)
	output, hasOutput := raw.values["output"]
	final, hasFinal := raw.values["final-output"]
	if hasOutput || hasFinal {
		request.Output = &BuildOutput{Staging: output, Final: final}
	}
	if err := request.Validate(); err != nil {
		return Request{}, err
	}
	return request, nil
}
