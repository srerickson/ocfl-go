// Copyright 2019 Seth R. Erickson
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ocfl

// func stringIn(a string, list []string) bool {
// 	for i := range list {
// 		if a == list[i] {
// 			return true
// 		}
// 	}
// 	return false
// }

// minusString returns slice of strings in a that aren't in b
func minusStrings(a []string, b []string) []string {
	var minus []string //in a but not in b
	for i := range a {
		var found bool
		for j := range b {
			if a[i] == b[j] {
				found = true
			}
		}
		if found == false {
			minus = append(minus, a[i])
		}
	}
	return minus
}
