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
	"strings"

	"github.com/goplus/mod/xgomod"
)

var singularOptions = map[string]struct{}{
	"project-dir":        {},
	"project-file":       {},
	"module-root":        {},
	"driver-package":     {},
	"selected-path":      {},
	"selected-version":   {},
	"origin-main":        {},
	"selected-dir":       {},
	"selected-gomod":     {},
	"replace-path":       {},
	"replace-version":    {},
	"replace-dir":        {},
	"replace-gomod":      {},
	"project-ext":        {},
	"project-full-ext":   {},
	"pack-dir":           {},
	"pack-index":         {},
	"declaration-file":   {},
	"declaration-sha256": {},
	"go-command":         {},
	"graph-work-dir":     {},
	"go-work":            {},
	"output":             {},
	"final-output":       {},
}

var commonRequiredOptions = []string{
	"project-dir",
	"project-file",
	"module-root",
	"driver-package",
	"selected-path",
	"selected-version",
	"origin-main",
	"project-ext",
	"project-full-ext",
	"declaration-file",
	"declaration-sha256",
	"go-command",
	"graph-work-dir",
	"go-work",
}

var replacementOptions = []string{
	"replace-path",
	"replace-version",
	"replace-dir",
	"replace-gomod",
}

type rawOptions struct {
	values     map[string]string
	graphFlags []string
	buildFlags []string
}

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

// Parse decodes driver argv and rejects unknown, duplicate, partial, or inapplicable fields.
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
	if request.Action != ActionRun && request.Action != ActionBuild {
		return Request{}, fmt.Errorf("driverprotocol: unsupported action %q", args[1])
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
	for _, name := range commonRequiredOptions {
		if _, ok := raw.values[name]; !ok {
			return Request{}, fmt.Errorf("driverprotocol: option --%s is required", name)
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
	if err := parseModuleSource(&request.DriverOrigin, raw); err != nil {
		return Request{}, err
	}

	request.Graph = Graph{
		GoCommand: raw.values["go-command"],
		WorkDir:   raw.values["graph-work-dir"],
		GoWork:    raw.values["go-work"],
		Flags:     append([]string(nil), raw.graphFlags...),
	}
	request.BuildFlags = append([]string(nil), raw.buildFlags...)
	if request.Action == ActionBuild {
		output, hasOutput := raw.values["output"]
		final, hasFinal := raw.values["final-output"]
		if !hasOutput || !hasFinal {
			return Request{}, fmt.Errorf("driverprotocol: build requires --output and --final-output")
		}
		request.Output = &BuildOutput{Staging: output, Final: final}
	} else if _, ok := raw.values["output"]; ok {
		return Request{}, fmt.Errorf("driverprotocol: run does not accept --output")
	} else if _, ok := raw.values["final-output"]; ok {
		return Request{}, fmt.Errorf("driverprotocol: run does not accept --final-output")
	}
	if err := request.Validate(); err != nil {
		return Request{}, err
	}
	return request, nil
}

func parseOptions(args []string) (rawOptions, error) {
	raw := rawOptions{values: make(map[string]string)}
	for _, arg := range args {
		if !strings.HasPrefix(arg, "--") || arg == "--" {
			return rawOptions{}, fmt.Errorf("driverprotocol: unexpected positional argument %q", arg)
		}
		name, value, ok := strings.Cut(strings.TrimPrefix(arg, "--"), "=")
		if !ok || name == "" {
			return rawOptions{}, fmt.Errorf("driverprotocol: option %q must use --name=value", arg)
		}
		switch name {
		case "graph-flag":
			raw.graphFlags = append(raw.graphFlags, value)
		case "build-flag":
			raw.buildFlags = append(raw.buildFlags, value)
		default:
			if _, ok := singularOptions[name]; !ok {
				return rawOptions{}, fmt.Errorf("driverprotocol: unknown option --%s", name)
			}
			if _, duplicate := raw.values[name]; duplicate {
				return rawOptions{}, fmt.Errorf("driverprotocol: option --%s may not be repeated", name)
			}
			raw.values[name] = value
		}
	}
	return raw, nil
}

func parseModuleSource(origin *xgomod.ResolvedModule, raw rawOptions) error {
	hasReplacement, completeReplacement := optionGroup(raw.values, replacementOptions...)
	selectedDir, completeSelected := optionGroup(raw.values, "selected-dir", "selected-gomod")
	if hasReplacement {
		if !completeReplacement {
			return fmt.Errorf("driverprotocol: replacement options must be supplied as a complete group")
		}
		if selectedDir {
			return fmt.Errorf("driverprotocol: origin with replacement forbids --selected-dir and --selected-gomod")
		}
		origin.Replace = &xgomod.ModuleRef{
			Path:    raw.values["replace-path"],
			Version: raw.values["replace-version"],
			Dir:     raw.values["replace-dir"],
			GoMod:   raw.values["replace-gomod"],
		}
		return nil
	}
	if !completeSelected {
		return fmt.Errorf("driverprotocol: origin without replacement requires --selected-dir and --selected-gomod")
	}
	origin.Selected.Dir = raw.values["selected-dir"]
	origin.Selected.GoMod = raw.values["selected-gomod"]
	return nil
}

func optionGroup(values map[string]string, names ...string) (present, complete bool) {
	complete = true
	for _, name := range names {
		_, ok := values[name]
		present = present || ok
		complete = complete && ok
	}
	return
}

func option(name, value string) string { return "--" + name + "=" + value }
