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
	"strconv"
	"strings"

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
		option(optionProjectDir, request.Project.Dir),
		option(optionProjectFile, request.Project.File),
		option(optionModuleRoot, request.Project.ModuleRoot),
		option(optionDriverPackage, request.DriverPackage),
		option(optionSelectedPath, request.DriverOrigin.Selected.Path),
		option(optionSelectedVersion, request.DriverOrigin.Selected.Version),
		option(optionOriginMain, strconv.FormatBool(request.DriverOrigin.Main)),
	}
	if request.DriverOrigin.Replace == nil {
		args = append(args,
			option(optionSelectedDir, request.DriverOrigin.Selected.Dir),
			option(optionSelectedGoMod, request.DriverOrigin.Selected.GoMod),
		)
	} else {
		replacement := request.DriverOrigin.Replace
		args = append(args,
			option(optionReplacePath, replacement.Path),
			option(optionReplaceVersion, replacement.Version),
			option(optionReplaceDir, replacement.Dir),
			option(optionReplaceGoMod, replacement.GoMod),
		)
	}
	args = append(args,
		option(optionProjectExt, request.Project.Extension),
		option(optionProjectFullExt, request.Project.FullExtension),
	)
	if request.Project.Pack != nil {
		args = append(args,
			option(optionPackDir, request.Project.Pack.Directory),
			option(optionPackIndex, request.Project.Pack.IndexFile),
		)
	}
	args = append(args,
		option(optionDeclarationFile, request.Declaration.Path),
		option(optionDeclarationSHA256, request.Declaration.SHA256),
		option(optionTargetModFile, request.TargetModFile.Path),
		option(optionTargetModFileSHA256, request.TargetModFile.SHA256),
		option(optionGoCommand, request.Graph.GoCommand),
		option(optionGraphWorkDir, request.Graph.WorkDir),
		option(optionGoWork, request.Graph.GoWork),
	)
	for _, flag := range request.Graph.Flags {
		args = append(args, option(optionGraphFlag, flag))
	}
	for _, flag := range request.BuildFlags {
		args = append(args, option(optionBuildFlag, flag))
	}
	if request.Action == ActionRun {
		args = append(args, "--")
		args = append(args, request.ApplicationArgs...)
	} else {
		args = append(args,
			option(optionOutput, request.Output.Staging),
			option(optionFinalOutput, request.Output.Final),
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

	request.Project = Project{
		Dir:           raw.values[optionProjectDir],
		File:          raw.values[optionProjectFile],
		ModuleRoot:    raw.values[optionModuleRoot],
		Extension:     raw.values[optionProjectExt],
		FullExtension: raw.values[optionProjectFullExt],
	}
	request.Declaration = xgomod.FileIdentity{
		Path: raw.values[optionDeclarationFile], SHA256: raw.values[optionDeclarationSHA256],
	}
	request.TargetModFile = xgomod.FileIdentity{
		Path: raw.values[optionTargetModFile], SHA256: raw.values[optionTargetModFileSHA256],
	}
	hasPack, completePack := optionGroup(raw.values, optionPackDir, optionPackIndex)
	if hasPack && !completePack {
		return Request{}, fmt.Errorf("driverprotocol: pack options must be supplied as a complete group")
	}
	if hasPack {
		request.Project.Pack = &Pack{Directory: raw.values[optionPackDir], IndexFile: raw.values[optionPackIndex]}
	}

	request.DriverPackage = raw.values[optionDriverPackage]
	request.DriverOrigin, err = parseDriverOrigin(raw)
	if err != nil {
		return Request{}, err
	}

	request.Graph = Graph{
		GoCommand: raw.values[optionGoCommand],
		WorkDir:   raw.values[optionGraphWorkDir],
		GoWork:    raw.values[optionGoWork],
		Flags:     append([]string(nil), raw.graphFlags...),
	}
	request.BuildFlags = append([]string(nil), raw.buildFlags...)
	output, hasOutput := raw.values[optionOutput]
	final, hasFinal := raw.values[optionFinalOutput]
	if hasOutput || hasFinal {
		request.Output = &BuildOutput{Staging: output, Final: final}
	}
	if err := request.Validate(); err != nil {
		return Request{}, err
	}
	return request, nil
}

func parseDriverOrigin(raw rawOptions) (xgomod.ResolvedModule, error) {
	origin := xgomod.ResolvedModule{
		Selected: xgomod.ModuleRef{Path: raw.values[optionSelectedPath], Version: raw.values[optionSelectedVersion]},
	}
	switch raw.values[optionOriginMain] {
	case "true":
		origin.Main = true
	case "false":
	default:
		return xgomod.ResolvedModule{}, fmt.Errorf("driverprotocol: invalid --%s %q: expected true or false", optionOriginMain, raw.values[optionOriginMain])
	}
	origin.Selected.Dir = raw.values[optionSelectedDir]
	origin.Selected.GoMod = raw.values[optionSelectedGoMod]
	hasSelected, _ := optionGroup(raw.values, optionSelectedDir, optionSelectedGoMod)
	hasReplacement, completeReplacement := optionGroup(raw.values, replacementOptions...)
	if hasReplacement && !completeReplacement {
		return xgomod.ResolvedModule{}, fmt.Errorf("driverprotocol: replacement options must be supplied as a complete group")
	}
	if hasReplacement && hasSelected {
		return xgomod.ResolvedModule{}, fmt.Errorf("driverprotocol: origin with replacement forbids --selected-dir and --selected-gomod")
	}
	if hasReplacement {
		origin.Replace = &xgomod.ModuleRef{
			Path:    raw.values[optionReplacePath],
			Version: raw.values[optionReplaceVersion],
			Dir:     raw.values[optionReplaceDir],
			GoMod:   raw.values[optionReplaceGoMod],
		}
	}
	return origin, nil
}

type optionSpec struct {
	name     string
	required bool
}

const (
	optionProjectDir          = "project-dir"
	optionProjectFile         = "project-file"
	optionModuleRoot          = "module-root"
	optionDriverPackage       = "driver-package"
	optionSelectedPath        = "selected-path"
	optionSelectedVersion     = "selected-version"
	optionOriginMain          = "origin-main"
	optionSelectedDir         = "selected-dir"
	optionSelectedGoMod       = "selected-gomod"
	optionReplacePath         = "replace-path"
	optionReplaceVersion      = "replace-version"
	optionReplaceDir          = "replace-dir"
	optionReplaceGoMod        = "replace-gomod"
	optionProjectExt          = "project-ext"
	optionProjectFullExt      = "project-full-ext"
	optionPackDir             = "pack-dir"
	optionPackIndex           = "pack-index"
	optionDeclarationFile     = "declaration-file"
	optionDeclarationSHA256   = "declaration-sha256"
	optionTargetModFile       = "target-modfile"
	optionTargetModFileSHA256 = "target-modfile-sha256"
	optionGoCommand           = "go-command"
	optionGraphWorkDir        = "graph-work-dir"
	optionGoWork              = "go-work"
	optionGraphFlag           = "graph-flag"
	optionBuildFlag           = "build-flag"
	optionOutput              = "output"
	optionFinalOutput         = "final-output"
)

// singularOptionSpecs defines accepted options and missing-field error order.
var singularOptionSpecs = []optionSpec{
	{optionProjectDir, true},
	{optionProjectFile, true},
	{optionModuleRoot, true},
	{optionDriverPackage, true},
	{optionSelectedPath, true},
	{optionSelectedVersion, true},
	{optionOriginMain, true},
	{optionSelectedDir, false},
	{optionSelectedGoMod, false},
	{optionReplacePath, false},
	{optionReplaceVersion, false},
	{optionReplaceDir, false},
	{optionReplaceGoMod, false},
	{optionProjectExt, true},
	{optionProjectFullExt, true},
	{optionPackDir, false},
	{optionPackIndex, false},
	{optionDeclarationFile, true},
	{optionDeclarationSHA256, true},
	{optionTargetModFile, true},
	{optionTargetModFileSHA256, true},
	{optionGoCommand, true},
	{optionGraphWorkDir, true},
	{optionGoWork, true},
	{optionOutput, false},
	{optionFinalOutput, false},
}

var replacementOptions = []string{
	optionReplacePath,
	optionReplaceVersion,
	optionReplaceDir,
	optionReplaceGoMod,
}

type rawOptions struct {
	values     map[string]string
	graphFlags []string
	buildFlags []string
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
		case optionGraphFlag:
			raw.graphFlags = append(raw.graphFlags, value)
		case optionBuildFlag:
			raw.buildFlags = append(raw.buildFlags, value)
		default:
			if !isSingularOption(name) {
				return rawOptions{}, fmt.Errorf("driverprotocol: unknown option --%s", name)
			}
			if _, duplicate := raw.values[name]; duplicate {
				return rawOptions{}, fmt.Errorf("driverprotocol: option --%s may not be repeated", name)
			}
			raw.values[name] = value
		}
	}
	for _, spec := range singularOptionSpecs {
		if _, ok := raw.values[spec.name]; spec.required && !ok {
			return rawOptions{}, fmt.Errorf("driverprotocol: option --%s is required", spec.name)
		}
	}
	return raw, nil
}

func isSingularOption(name string) bool {
	for _, spec := range singularOptionSpecs {
		if spec.name == name {
			return true
		}
	}
	return false
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
