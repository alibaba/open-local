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
	"context"

	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
)

func (f *Framework) GetCSIDriver(ctx context.Context, name string) (*storagev1.CSIDriver, error) {
	return f.KubeClient.StorageV1().CSIDrivers().Get(ctx, name, metav1.GetOptions{})
}

func (f *Framework) CreateOrUpdateCSIDriver(ctx context.Context, source string) (*storagev1.CSIDriver, error) {
	csiDriver, err := parseCSIDriverYaml(source)
	if err != nil {
		return nil, err
	}

	_, err = f.KubeClient.StorageV1().CSIDrivers().Get(ctx, csiDriver.Name, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	}

	if apierrors.IsNotFound(err) {
		// csiDriver doesn't exists -> Create
		csiDriver, err = f.KubeClient.StorageV1().CSIDrivers().Create(ctx, csiDriver, metav1.CreateOptions{})
		if err != nil {
			return nil, err
		}
	} else {
		// csiDriver already exists -> Update
		csiDriver, err = f.KubeClient.StorageV1().CSIDrivers().Update(ctx, csiDriver, metav1.UpdateOptions{})
		if err != nil {
			return nil, err
		}
	}

	return csiDriver, nil
}

func (f *Framework) DeleteCSIDriver(ctx context.Context, source string) error {
	csiDriver, err := parseCSIDriverYaml(source)
	if err != nil {
		return err
	}

	return f.KubeClient.StorageV1().CSIDrivers().Delete(ctx, csiDriver.Name, metav1.DeleteOptions{})
}

func (f *Framework) UpdateCSIDriver(ctx context.Context, csiDriver *storagev1.CSIDriver) error {
	_, err := f.KubeClient.StorageV1().CSIDrivers().Update(ctx, csiDriver, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	return nil
}

func parseCSIDriverYaml(source string) (*storagev1.CSIDriver, error) {
	manifest, err := SourceToIOReader(source)
	if err != nil {
		return nil, err
	}

	csiDriver := storagev1.CSIDriver{}
	if err := yaml.NewYAMLOrJSONDecoder(manifest, 100).Decode(&csiDriver); err != nil {
		return nil, err
	}

	return &csiDriver, nil
}
