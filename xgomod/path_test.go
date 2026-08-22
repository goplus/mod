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
	"path/filepath"
	"testing"
)

func TestPathWithin(t *testing.T) {
	root := t.TempDir()
	tests := []struct {
		name   string
		target string
		want   bool
	}{
		{"root", root, true},
		{"descendant", filepath.Join(root, "nested", "go.mod"), true},
		{"parent", filepath.Dir(root), false},
		{"sibling with common prefix", root + "-other", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := pathWithin(root, test.target); got != test.want {
				t.Fatalf("pathWithin(%q, %q) = %v, want %v", root, test.target, got, test.want)
			}
		})
	}
}
