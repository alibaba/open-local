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
	"flag"
	"fmt"
	"log"
	"os"
	"testing"

	"github.com/alibaba/open-local/pkg/apis/storage"
	"github.com/alibaba/open-local/test/framework"
	"github.com/blang/semver/v4"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var (
	testframework *framework.Framework
	testImage     *string
)

func TestMain(m *testing.M) {
	kubeconfig := flag.String(
		"kubeconfig",
		"",
		"kube config path, e.g. $HOME/.kube/config",
	)
	testImage = flag.String(
		"test-image",
		"",
		"test image, e.g. docker.io/openlocal/open-local",
	)
	flag.Parse()

	var (
		err      error
		exitCode int
	)

	logger := log.New(os.Stdout, "", log.Lshortfile)

	currentVersion, err := os.ReadFile("../../VERSION")
	if err != nil {
		logger.Printf("failed to read version file: %v\n", err)
		os.Exit(1)
	}
	currentSemVer, err := semver.ParseTolerant(string(currentVersion))
	if err != nil {
		logger.Printf("failed to parse current version: %v\n", err)
		os.Exit(1)
	}

	exampleDir := "../../"
	resourcesDir := "../framework/resources"

	nextSemVer, err := semver.ParseTolerant(fmt.Sprintf("0.%d.0", currentSemVer.Minor))
	if err != nil {
		logger.Printf("failed to parse next version: %v\n", err)
		os.Exit(1)
	}

	if testframework, err = framework.New(*kubeconfig, *testImage, exampleDir, resourcesDir, nextSemVer); err != nil {
		logger.Printf("failed to setup fk: %v\n", err)
		os.Exit(1)
	}

	exitCode = m.Run()

	os.Exit(exitCode)
}

func TestE2E(t *testing.T) {
	testCtx := testframework.NewTestCtx(t)
	defer testCtx.Cleanup(t)

	t.Run("TestInstallCRD", testInsertCRD(context.Background()))
	t.Run("TestInsertAgent", testInsertAgent(context.Background()))
}

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

func testInsertAgent(ctx context.Context) func(t *testing.T) {
	return func(t *testing.T) {
		testCtx := testframework.NewTestCtx(t)
		defer testCtx.Cleanup(t)

		testNS := testframework.CreateNamespace(context.Background(), t, testCtx)

		err := testframework.CreateOrUpdateDaemonSetAndWaitUntilReady(context.Background(), testNS, &appsv1.DaemonSet{
			TypeMeta: metav1.TypeMeta{
				Kind:       "DaemonSet",
				APIVersion: "apps/v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:        "test-agent",
				Namespace:   testNS,
				Annotations: map[string]string{},
			},
			Spec: appsv1.DaemonSetSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"name": "test-agent",
					},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"name": "test-agent",
						},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "agent",
								Image: testframework.Image,
							},
						},
					},
				},
			},
		})

		if err != nil {
			t.Fatal(err)
		}
	}
}
