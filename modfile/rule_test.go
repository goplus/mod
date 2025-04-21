/*
 * Copyright (c) 2021 The GoPlus Authors (goplus.org). All rights reserved.
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
package modfile

import (
	"syscall"
	"testing"
)

// -----------------------------------------------------------------------------

var addParseExtTests = []struct {
	desc    string
	ext     string
	want    string
	wantF   string
	wantErr string
	isProj  bool
}{
	{
		"spx ok",
		".spx",
		".spx",
		".spx",
		"",
		false,
	},
	{
		"yap ok",
		"_yap.gox",
		"_yap.gox",
		"_yap.gox",
		"",
		false,
	},
	{
		"yap ok",
		"*_yap.gox",
		"_yap.gox",
		"*_yap.gox",
		"",
		false,
	},
	{
		"yap ok",
		"main_yap.gox",
		"_yap.gox",
		"main_yap.gox",
		"",
		true,
	},
	{
		"yap ok",
		"main_yap.gox",
		"",
		"",
		"ext main_yap.gox invalid: invalid ext format",
		false,
	},
	{
		"not a ext",
		"gmx",
		"",
		"",
		"ext gmx invalid: invalid ext format",
		false,
	},
}

func TestParseExt(t *testing.T) {
	if (&InvalidExtError{Err: syscall.EINVAL}).Unwrap() != syscall.EINVAL {
		t.Fatal("InvalidExtError.Unwrap failed")
	}
	if (&InvalidSymbolError{Err: syscall.EINVAL}).Unwrap() != syscall.EINVAL {
		t.Fatal("InvalidSymbolError.Unwrap failed")
	}
	for _, tt := range addParseExtTests {
		t.Run(tt.desc, func(t *testing.T) {
			ext, extF, err := parseExt(&tt.ext, tt.isProj)
			if err != nil {
				if err.Error() != tt.wantErr {
					t.Fatalf("wanterr: %s, but got: %s", tt.wantErr, err)
				}
			}
			if ext != tt.want || extF != tt.wantF {
				t.Fatalf("want: %s %s, but got: %s %s", tt.want, tt.wantF, ext, extF)
			}
		})
	}
}

func TestIsDirectoryPath(t *testing.T) {
	if !IsDirectoryPath("./...") {
		t.Fatal("IsDirectoryPath failed")
	}
}

func TestFormat(t *testing.T) {
	if b := Format(&FileSyntax{}); len(b) != 0 {
		t.Fatal("Format failed:", b)
	}
}

func TestForma2t(t *testing.T) {
	f := New("/foo/gop.mod", "1.2.0")
	if b := string(Format(f.Syntax)); b != "gop 1.2.0\n" {
		t.Fatal("Format failed:", b)
	}
}

func TestMustQuote(t *testing.T) {
	if !MustQuote("") {
		t.Fatal("MustQuote failed")
	}
}

// -----------------------------------------------------------------------------
