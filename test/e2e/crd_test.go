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

package e2e

import (
	"context"

	"testing"

	"github.com/alibaba/open-local/pkg/apis/storage"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func testInsertCRD(ctx context.Context) func(t *testing.T) {
	return func(t *testing.T) {
		err := testframework.CreateOrUpdateCRDAndWaitUntilReady(ctx, storage.NodeLocalStorageInitConfigName, func(opts metav1.ListOptions) (runtime.Object, error) {
			return testframework.NodeLocalStorageClientV1alpha1.CsiV1alpha1().NodeLocalStorageInitConfigs().List(ctx, opts)
		})
		if err != nil {
			t.Fatal(err)
		}
		err = testframework.CreateOrUpdateCRDAndWaitUntilReady(ctx, storage.NodeLocalStorageListName, func(opts metav1.ListOptions) (runtime.Object, error) {
			return testframework.NodeLocalStorageClientV1alpha1.CsiV1alpha1().NodeLocalStorages().List(ctx, opts)
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
