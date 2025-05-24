/*
 * Copyright (c) 2024 The XGo Authors (xgo.dev). All rights reserved.
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

package sumfile

import (
	"os"
	"sort"
	"strings"
)

type File struct {
	lines []string
	gosum string
}

func Load(gosum string) (sumf *File, err error) {
	var lines []string
	b, err := os.ReadFile(gosum)
	if err != nil {
		if !os.IsNotExist(err) {
			return
		}
	} else {
		text := string(b)
		lines = strings.Split(strings.TrimRight(text, "\n"), "\n")
	}
	return &File{lines, gosum}, nil
}

func (p *File) Save() (err error) {
	n := 0
	for _, line := range p.lines {
		n += 1 + len(line)
	}
	b := make([]byte, 0, n)
	for _, line := range p.lines {
		b = append(b, line...)
		b = append(b, '\n')
	}
	return os.WriteFile(p.gosum, b, 0666)
}

func (p *File) Lookup(modPath string) []string {
	prefix := modPath + " "
	lines := p.lines
	for i, line := range lines {
		if line > prefix {
			if strings.HasPrefix(line, prefix) {
				for j, line := range lines[i+1:] {
					if !strings.HasPrefix(line, prefix) {
						lines = lines[:i+1+j]
						break
					}
				}
				return lines[i:]
			}
			break
		}
	}
	return nil
}

func (p *File) Add(lines []string) {
	p.lines = append(p.lines, lines...)
	sort.Strings(p.lines)
}
