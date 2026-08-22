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
	"encoding/hex"
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

func validateAbsolutePath(name, value string) error {
	if value == "" || strings.IndexByte(value, 0) >= 0 {
		return fmt.Errorf("driverprotocol: path --%s may not be empty or contain NUL", name)
	}
	if !filepath.IsAbs(value) {
		return fmt.Errorf("driverprotocol: path --%s must be absolute: %q", name, value)
	}
	if filepath.Clean(value) != value {
		return fmt.Errorf("driverprotocol: path --%s must be clean: %q", name, value)
	}
	return nil
}

func validatePackDirectory(value string) error {
	if value == "" || strings.Contains(value, "\\") || strings.IndexByte(value, 0) >= 0 || path.IsAbs(value) || path.Clean(value) != value {
		return fmt.Errorf("driverprotocol: pack directory must be a clean non-empty relative slash path: %q", value)
	}
	if value == ".." || strings.HasPrefix(value, "../") {
		return fmt.Errorf("driverprotocol: pack directory escapes the project: %q", value)
	}
	return nil
}

func validatePackIndex(value string) error {
	if value == "" || value == "." || value == ".." || strings.ContainsAny(value, "/\\\x00") {
		return fmt.Errorf("driverprotocol: pack index must be a plain file name: %q", value)
	}
	return nil
}

func validateSHA256(name, value string) error {
	if len(value) != 64 {
		return fmt.Errorf("driverprotocol: --%s must contain 64 hexadecimal characters", name)
	}
	if _, err := hex.DecodeString(value); err != nil {
		return fmt.Errorf("driverprotocol: --%s is not a SHA-256 digest: %w", name, err)
	}
	if value != strings.ToLower(value) {
		return fmt.Errorf("driverprotocol: --%s must use lowercase hexadecimal", name)
	}
	return nil
}

func validateGraphFlags(flags []string) error {
	return validateFlags("graph", flags, func(name, value string) error {
		switch name {
		case "mod":
			if value != "mod" && value != "readonly" && value != "vendor" {
				return fmt.Errorf("driverprotocol: graph flag -mod has unsupported value %q", value)
			}
		case "modfile", "overlay":
			if err := validateAbsolutePath("graph flag -"+name, value); err != nil {
				return err
			}
		default:
			return fmt.Errorf("driverprotocol: graph flag -%s is not supported", name)
		}
		return nil
	})
}

func validateBuildFlags(flags []string) error {
	return validateFlags("build", flags, func(name, value string) error {
		switch name {
		case "v", "x", "work", "trimpath":
			if value != "true" {
				return fmt.Errorf("driverprotocol: build flag -%s has unsupported value %q", name, value)
			}
		case "buildvcs":
			if value != "false" {
				return fmt.Errorf("driverprotocol: build flag -buildvcs has unsupported value %q", value)
			}
		default:
			return fmt.Errorf("driverprotocol: build flag -%s is not supported", name)
		}
		return nil
	})
}

func validateFlags(kind string, flags []string, validateValue func(name, value string) error) error {
	seen := make(map[string]struct{}, len(flags))
	for _, flag := range flags {
		name, value, ok := splitCanonicalFlag(flag)
		if !ok {
			return fmt.Errorf("driverprotocol: %s flag %q must use -name=value", kind, flag)
		}
		if _, duplicate := seen[name]; duplicate {
			return fmt.Errorf("driverprotocol: %s flag -%s may not be repeated", kind, name)
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
