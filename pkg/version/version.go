/*
Copyright © 2021 Alibaba Group Holding Ltd.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package version

import "fmt"

var (
	Version string = ""
	// GitCommit generated on build time
	GitCommit string = ""
)

const (
	Name = "open-local"
)

func GetFullVersion(longFormat bool) string {
	git := GitCommit
	if !longFormat && GitCommit != "" {
		git = GitCommit[:12]
	}
	return fmt.Sprintf("%s-%s", Version, git)
}

func NameWithVersion(longFormat bool) string {
	return fmt.Sprintf("%s: %s", Name, GetFullVersion(longFormat))
}
