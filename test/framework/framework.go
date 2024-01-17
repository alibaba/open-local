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

package framework

import (
	"fmt"
	"net/http"
	"time"

	"github.com/alibaba/open-local/pkg/generated/clientset/versioned"
	"github.com/blang/semver/v4"
	apiclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type Framework struct {
	KubeClient                     kubernetes.Interface
	APIServerClient                apiclient.Interface
	NodeLocalStorageClientV1alpha1 versioned.Interface
	HTTPClient                     *http.Client
	MasterHost                     string
	DefaultTimeout                 time.Duration
	RestConfig                     *rest.Config
	Image                          string
	Version                        semver.Version

	exampleDir   string
	resourcesDir string
}

// New setups a test framework and returns it.
func New(kubeconfig, image, exampleDir, resourcesDir string, version semver.Version) (*Framework, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("build config from flags failed: %w", err)
	}

	cli, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("creating new kube-client failed: %w", err)
	}

	apiCli, err := apiclient.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("creating new kube-client failed: %w", err)
	}

	httpc := cli.CoreV1().RESTClient().(*rest.RESTClient).Client
	if err != nil {
		return nil, fmt.Errorf("creating http-client failed: %w", err)
	}

	mNodeLocalStorageClientV1alpha1, err := versioned.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("creating v1alpha1 NodeLocalStorage client failed: %w", err)
	}

	f := &Framework{
		RestConfig:                     config,
		MasterHost:                     config.Host,
		KubeClient:                     cli,
		APIServerClient:                apiCli,
		HTTPClient:                     httpc,
		DefaultTimeout:                 time.Minute,
		Version:                        version,
		Image:                          image,
		exampleDir:                     exampleDir,
		resourcesDir:                   resourcesDir,
		NodeLocalStorageClientV1alpha1: mNodeLocalStorageClientV1alpha1,
	}

	return f, nil
}
