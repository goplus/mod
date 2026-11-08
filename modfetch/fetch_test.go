/*
 * Copyright (c) 2021 The XGo Authors (xgo.dev). All rights reserved.
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

package modfetch

import (
	"testing"

	xmod "github.com/goplus/mod"
)

func TestGetResult(t *testing.T) {
	const parentFirst = "go: downloading github.com/llarhub/libcxx v0.1.1\n" +
		"go: downloading github.com/llarhub/libcxx/c v0.1.1\n" +
		"go: added github.com/llarhub/libcxx/c v0.1.1\n"
	const childFirst = "go: downloading github.com/llarhub/libcxx/c v0.1.1\n" +
		"go: downloading github.com/llarhub/libcxx v0.1.1\n" +
		"go: added github.com/llarhub/libcxx/c v0.1.1\n"

	// Whatever the order of the downloading lines, the requested module must be
	// selected instead of the first-printed one (which may be its parent module
	// in a multi-module repository). This ordering is non-deterministic in
	// `go get`, which is the root cause of the intermittent failure this fixes.
	for name, data := range map[string]string{
		"parentFirst": parentFirst,
		"childFirst":  childFirst,
	} {
		t.Run(name, func(t *testing.T) {
			mod, err := getResult(data, "github.com/llarhub/libcxx/c")
			if err != nil {
				t.Fatal("getResult:", err)
			}
			if mod.Path != "github.com/llarhub/libcxx/c" || mod.Version != "v0.1.1" {
				t.Fatalf("getResult: got %v %v", mod.Path, mod.Version)
			}
		})
	}
}

func TestGetResultFallback(t *testing.T) {
	// No exact match for reqPath: fall back to the first downloading line, and
	// with an empty reqPath (single-module callers) keep the first line too.
	const data = "go: downloading github.com/xushiwei/foogop v0.1.0\n"
	for _, reqPath := range []string{"", "github.com/not/matched"} {
		mod, err := getResult(data, reqPath)
		if err != nil {
			t.Fatal("getResult:", err)
		}
		if mod.Path != "github.com/xushiwei/foogop" || mod.Version != "v0.1.0" {
			t.Fatalf("getResult(%q): got %v %v", reqPath, mod.Path, mod.Version)
		}
	}
}

func TestGetResultNotFound(t *testing.T) {
	if _, err := getResult("go: added something\n", ""); err != xmod.ErrNotFound {
		t.Fatal("getResult: expected ErrNotFound, got", err)
	}
}
