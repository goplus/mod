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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

func validateFileIdentity(identity FileIdentity) ([]byte, error) {
	if identity.Path == "" || identity.SHA256 == "" {
		return nil, fmt.Errorf("target modfile identity requires path and SHA-256")
	}
	if len(identity.SHA256) != sha256.Size*2 {
		return nil, fmt.Errorf("target modfile SHA-256 must be %d hex characters", sha256.Size*2)
	}
	if _, err := hex.DecodeString(identity.SHA256); err != nil {
		return nil, fmt.Errorf("invalid target modfile SHA-256: %w", err)
	}
	if identity.SHA256 != strings.ToLower(identity.SHA256) {
		return nil, fmt.Errorf("target modfile SHA-256 must use lowercase hexadecimal")
	}
	path, err := canonicalSourcePath(identity.Path, "target modfile path", false)
	if err != nil {
		return nil, err
	}
	data, got, err := readFileSHA256(path)
	if err != nil {
		return nil, fmt.Errorf("read target modfile: %w", err)
	}
	if got != identity.SHA256 {
		return nil, fmt.Errorf("target modfile SHA-256 mismatch for %s", identity.Path)
	}
	return data, nil
}

func readFileSHA256(path string) ([]byte, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	return data, sha256Hex(data), nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
