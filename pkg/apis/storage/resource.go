/*
Copyright © 2023 Alibaba Group Holding Ltd.

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

package storage

import (
	"fmt"
)

const (
	NodeLocalStorageInitConfigsKind = "NodeLocalStorageInitConfig"
	NodeLocalStorageInitConfigName  = "nodelocalstorageinitconfigs"

	NodeLocalStorageListsKind = "NodeLocalStorageList"
	NodeLocalStorageListName  = "nodelocalstorages"
)

var resourceToKindMap = map[string]string{
	NodeLocalStorageInitConfigName: NodeLocalStorageInitConfigsKind,
	NodeLocalStorageListName:       NodeLocalStorageListsKind,
}

func ResourceToKind(s string) string {
	kind, found := resourceToKindMap[s]
	if !found {
		panic(fmt.Sprintf("failed to map resource %q to a kind", s))
	}
	return kind
}
