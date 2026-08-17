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

package runtimeprotocol

import (
	"fmt"
	"strings"

	"github.com/goplus/mod/xgomod"
)

var singularOptions = map[string]struct{}{
	"project-dir":        {},
	"project-file":       {},
	"module-root":        {},
	"provider-package":   {},
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
	"provider-package",
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

// Encode returns the deterministic argv following the provider executable.
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
		option("provider-package", request.ProviderPackage),
		option("selected-path", request.ProviderOrigin.Selected.Path),
		option("selected-version", request.ProviderOrigin.Selected.Version),
		option("origin-main", fmt.Sprint(request.ProviderOrigin.Main)),
	}
	if request.ProviderOrigin.Replace == nil {
		args = append(args,
			option("selected-dir", request.ProviderOrigin.Selected.Dir),
			option("selected-gomod", request.ProviderOrigin.Selected.GoMod),
		)
	} else {
		replacement := request.ProviderOrigin.Replace
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

// Parse decodes the complete provider argv following argv[0]. Unknown,
// duplicate, partial, and action-inapplicable fields fail closed.
func Parse(args []string) (Request, error) {
	var request Request
	if len(args) < 2 {
		return request, fmt.Errorf("runtimeprotocol: request requires preamble and action")
	}
	if args[0] != PreambleV1 {
		return request, fmt.Errorf("runtimeprotocol: unsupported preamble %q", args[0])
	}
	request.Version = Version1
	request.Action = Action(args[1])
	if request.Action != ActionRun && request.Action != ActionBuild {
		return Request{}, fmt.Errorf("runtimeprotocol: unsupported action %q", args[1])
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
			return Request{}, fmt.Errorf("runtimeprotocol: run requires -- before application arguments")
		}
		request.ApplicationArgs = append([]string(nil), optionArgs[delimiter+1:]...)
		optionArgs = optionArgs[:delimiter]
	} else {
		for _, arg := range optionArgs {
			if arg == "--" {
				return Request{}, fmt.Errorf("runtimeprotocol: build does not accept -- or positional arguments")
			}
		}
	}

	raw, err := parseOptions(optionArgs)
	if err != nil {
		return Request{}, err
	}
	for _, name := range commonRequiredOptions {
		if _, ok := raw.values[name]; !ok {
			return Request{}, fmt.Errorf("runtimeprotocol: option --%s is required", name)
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
	_, hasPackDir := raw.values["pack-dir"]
	_, hasPackIndex := raw.values["pack-index"]
	if hasPackDir != hasPackIndex {
		return Request{}, fmt.Errorf("runtimeprotocol: pack options must be supplied as a complete group")
	}
	if hasPackDir {
		request.Project.Pack = &Pack{Directory: raw.values["pack-dir"], IndexFile: raw.values["pack-index"]}
	}

	request.ProviderPackage = raw.values["provider-package"]
	request.ProviderOrigin = xgomod.ResolvedModule{
		Selected: xgomod.ModuleRef{
			Path:    raw.values["selected-path"],
			Version: raw.values["selected-version"],
		},
	}
	switch raw.values["origin-main"] {
	case "true":
		request.ProviderOrigin.Main = true
	case "false":
	default:
		return Request{}, fmt.Errorf("runtimeprotocol: invalid --origin-main %q: expected true or false", raw.values["origin-main"])
	}
	if err := parseModuleSource(&request.ProviderOrigin, raw); err != nil {
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
			return Request{}, fmt.Errorf("runtimeprotocol: build requires --output and --final-output")
		}
		request.Output = &BuildOutput{Staging: output, Final: final}
	} else if _, ok := raw.values["output"]; ok {
		return Request{}, fmt.Errorf("runtimeprotocol: run does not accept --output")
	} else if _, ok := raw.values["final-output"]; ok {
		return Request{}, fmt.Errorf("runtimeprotocol: run does not accept --final-output")
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
			return rawOptions{}, fmt.Errorf("runtimeprotocol: unexpected positional argument %q", arg)
		}
		name, value, ok := strings.Cut(strings.TrimPrefix(arg, "--"), "=")
		if !ok || name == "" {
			return rawOptions{}, fmt.Errorf("runtimeprotocol: option %q must use --name=value", arg)
		}
		switch name {
		case "graph-flag":
			raw.graphFlags = append(raw.graphFlags, value)
		case "build-flag":
			raw.buildFlags = append(raw.buildFlags, value)
		default:
			if _, ok := singularOptions[name]; !ok {
				return rawOptions{}, fmt.Errorf("runtimeprotocol: unknown option --%s", name)
			}
			if _, duplicate := raw.values[name]; duplicate {
				return rawOptions{}, fmt.Errorf("runtimeprotocol: option --%s may not be repeated", name)
			}
			raw.values[name] = value
		}
	}
	return raw, nil
}

func parseModuleSource(origin *xgomod.ResolvedModule, raw rawOptions) error {
	replacementCount := 0
	for _, name := range replacementOptions {
		if _, ok := raw.values[name]; ok {
			replacementCount++
		}
	}
	_, selectedDir := raw.values["selected-dir"]
	_, selectedGoMod := raw.values["selected-gomod"]
	switch replacementCount {
	case 0:
		if !selectedDir || !selectedGoMod {
			return fmt.Errorf("runtimeprotocol: origin without replacement requires --selected-dir and --selected-gomod")
		}
		origin.Selected.Dir = raw.values["selected-dir"]
		origin.Selected.GoMod = raw.values["selected-gomod"]
	case len(replacementOptions):
		if selectedDir || selectedGoMod {
			return fmt.Errorf("runtimeprotocol: origin with replacement forbids --selected-dir and --selected-gomod")
		}
		origin.Replace = &xgomod.ModuleRef{
			Path:    raw.values["replace-path"],
			Version: raw.values["replace-version"],
			Dir:     raw.values["replace-dir"],
			GoMod:   raw.values["replace-gomod"],
		}
	default:
		return fmt.Errorf("runtimeprotocol: replacement options must be supplied as a complete group")
	}
	return nil
}

func option(name, value string) string { return "--" + name + "=" + value }
