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
	"strings"
	"testing"

	"github.com/goplus/mod/xgomod"
)

func TestRejectMalformedArgv(t *testing.T) {
	valid, err := Encode(testRequest())
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string][]string{
		"unknown":                         append(append([]string(nil), valid[:len(valid)-4]...), "--unknown=value", "--", ""),
		"duplicate":                       append(append([]string(nil), valid[:2]...), append([]string{valid[2]}, valid[2:]...)...),
		"partial pack":                    removeOption(valid, "--pack-index="),
		"partial replace":                 removeOption(valid, "--replace-gomod="),
		"missing work dir":                removeOption(valid, "--graph-work-dir="),
		"uppercase digest":                replaceOptionValue(valid, "--declaration-sha256=", strings.Repeat("A", 64)),
		"missing target modfile":          removeOption(valid, "--target-modfile="),
		"missing target modfile digest":   removeOption(valid, "--target-modfile-sha256="),
		"uppercase target modfile digest": replaceOptionValue(valid, "--target-modfile-sha256=", strings.Repeat("B", 64)),
		"missing delimiter": func() []string {
			copy := append([]string(nil), valid...)
			for i, value := range copy {
				if value == "--" {
					return copy[:i]
				}
			}
			return copy
		}(),
	}
	for name, args := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(args); err == nil {
				t.Fatalf("Parse(%#v) succeeded", args)
			}
		})
	}
}

func TestSingularOptionSchema(t *testing.T) {
	seen := make(map[string]struct{}, len(singularOptionSpecs))
	for _, spec := range singularOptionSpecs {
		if _, duplicate := seen[spec.name]; duplicate {
			t.Fatalf("duplicate singular option %q", spec.name)
		}
		seen[spec.name] = struct{}{}
	}

	args, err := Encode(testRequest())
	if err != nil {
		t.Fatal(err)
	}
	args = removeOption(args, "--project-dir=")
	args = removeOption(args, "--project-file=")
	if _, err := Parse(args); err == nil || !strings.Contains(err.Error(), "option --project-dir is required") {
		t.Fatalf("Parse() error = %v, want first missing required option", err)
	}
}

func TestParseRejectsMalformedRequests(t *testing.T) {
	runArgs, err := Encode(testRequest())
	if err != nil {
		t.Fatal(err)
	}
	buildRequest := testRequest()
	buildRequest.Action = ActionBuild
	buildRequest.ApplicationArgs = nil
	buildRequest.Project.Pack = nil
	buildRequest.DriverOrigin = xgomod.ResolvedModule{
		Selected: xgomod.ModuleRef{
			Path: "example.test/framework", Version: "v1.2.3",
			Dir: testPath("workspace", "framework"), GoMod: testPath("workspace", "framework", "go.mod"),
		},
	}
	buildRequest.Output = &BuildOutput{
		Staging: testPath("workspace", "out", ".game.tmp"),
		Final:   testPath("workspace", "out", "game"),
	}
	buildArgs, err := Encode(buildRequest)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		args func() []string
		want string
	}{
		{
			name: "empty argv",
			args: func() []string { return nil },
			want: "requires preamble and action",
		},
		{
			name: "preamble only",
			args: func() []string { return []string{PreambleV1} },
			want: "requires preamble and action",
		},
		{
			name: "unsupported preamble",
			args: func() []string {
				args := append([]string(nil), runArgs...)
				args[0] = "other-driver"
				return args
			},
			want: "unsupported preamble",
		},
		{
			name: "unsupported action",
			args: func() []string { return []string{PreambleV1, "test"} },
			want: "unsupported action",
		},
		{
			name: "build delimiter",
			args: func() []string {
				return append(append([]string(nil), buildArgs...), "--")
			},
			want: "build does not accept",
		},
		{
			name: "run missing delimiter",
			args: func() []string {
				args := append([]string(nil), runArgs...)
				for i, arg := range args {
					if arg == "--" {
						return args[:i]
					}
				}
				return args
			},
			want: "run requires --",
		},
		{
			name: "positional option",
			args: func() []string {
				args := append([]string(nil), runArgs...)
				args[2] = "project-dir"
				return args
			},
			want: "unexpected positional argument",
		},
		{
			name: "malformed option",
			args: func() []string {
				args := append([]string(nil), runArgs...)
				args[2] = "--project-dir"
				return args
			},
			want: "must use --name=value",
		},
		{
			name: "missing required option",
			args: func() []string { return removeOption(runArgs, "--project-dir=") },
			want: "option --project-dir is required",
		},
		{
			name: "invalid origin main",
			args: func() []string { return replaceOptionValue(runArgs, "--origin-main=", "maybe") },
			want: "invalid --origin-main",
		},
		{
			name: "missing selected source",
			args: func() []string {
				args := removeOption(buildArgs, "--selected-dir=")
				return removeOption(args, "--selected-gomod=")
			},
			want: "selected must provide both",
		},
		{
			name: "replacement with empty selected source",
			args: func() []string {
				return insertBeforeDelimiter(runArgs, "--selected-dir=")
			},
			want: "with replacement forbids",
		},
		{
			name: "missing build output",
			args: func() []string { return removeOption(buildArgs, "--output=") },
			want: "path --output may not be empty",
		},
		{
			name: "missing build final output",
			args: func() []string { return removeOption(buildArgs, "--final-output=") },
			want: "path --final-output may not be empty",
		},
		{
			name: "run output",
			args: func() []string {
				return insertBeforeDelimiter(runArgs, "--output="+testPath("workspace", "out", "game"))
			},
			want: "run request cannot contain output paths",
		},
		{
			name: "run final output",
			args: func() []string {
				return insertBeforeDelimiter(runArgs, "--final-output="+testPath("workspace", "out", "game"))
			},
			want: "run request cannot contain output paths",
		},
		{
			name: "incomplete replacement",
			args: func() []string { return removeOption(runArgs, "--replace-gomod=") },
			want: "complete group",
		},
		{
			name: "missing empty replacement version",
			args: func() []string { return removeOption(runArgs, "--replace-version=") },
			want: "complete group",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Parse(test.args()); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Parse() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func removeOption(args []string, prefix string) []string {
	result := make([]string, 0, len(args))
	for _, arg := range args {
		if !strings.HasPrefix(arg, prefix) {
			result = append(result, arg)
		}
	}
	return result
}

func insertBeforeDelimiter(args []string, value string) []string {
	result := make([]string, 0, len(args)+1)
	inserted := false
	for _, arg := range args {
		if !inserted && arg == "--" {
			result = append(result, value)
			inserted = true
		}
		result = append(result, arg)
	}
	if !inserted {
		result = append(result, value)
	}
	return result
}

func replaceOptionValue(args []string, prefix, value string) []string {
	result := append([]string(nil), args...)
	for i, arg := range result {
		if strings.HasPrefix(arg, prefix) {
			result[i] = prefix + value
			return result
		}
	}
	return result
}
