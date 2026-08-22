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
)

type optionSpec struct {
	name     string
	required bool
}

// singularOptionSpecs is both the accepted singular option set and the stable
// order in which missing required options are reported.
var singularOptionSpecs = []optionSpec{
	{"project-dir", true},
	{"project-file", true},
	{"module-root", true},
	{"driver-package", true},
	{"selected-path", true},
	{"selected-version", true},
	{"origin-main", true},
	{"selected-dir", false},
	{"selected-gomod", false},
	{"replace-path", false},
	{"replace-version", false},
	{"replace-dir", false},
	{"replace-gomod", false},
	{"project-ext", true},
	{"project-full-ext", true},
	{"pack-dir", false},
	{"pack-index", false},
	{"declaration-file", true},
	{"declaration-sha256", true},
	{"go-command", true},
	{"graph-work-dir", true},
	{"go-work", true},
	{"output", false},
	{"final-output", false},
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
			if !isSingularOption(name) {
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
