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
	"testing"
)

func TestActionValidate(t *testing.T) {
	tests := []struct {
		action Action
		valid  bool
	}{
		{ActionRun, true},
		{ActionBuild, true},
		{"", false},
		{"publish", false},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("%q", test.action), func(t *testing.T) {
			err := test.action.Validate()
			if (err == nil) != test.valid {
				t.Fatalf("Action(%q).Validate() = %v, valid = %v", test.action, err, test.valid)
			}
			if !test.valid {
				request := testRequest()
				request.Action = test.action
				requestErr := request.Validate()
				if requestErr == nil || requestErr.Error() != err.Error() {
					t.Fatalf("Request.Validate() = %v, Action.Validate() = %v", requestErr, err)
				}
				_, parseErr := Parse([]string{PreambleV1, string(test.action)})
				if parseErr == nil || parseErr.Error() != err.Error() {
					t.Fatalf("Parse() = %v, Action.Validate() = %v", parseErr, err)
				}
			}
		})
	}
}
