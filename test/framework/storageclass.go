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

func (f *Framework) GetStorageClass(ctx context.Context, name string) (*storagev1.StorageClass, error) {
	return f.KubeClient.StorageV1().StorageClasses().Get(ctx, name, metav1.GetOptions{})
}

func (f *Framework) CreateOrUpdateStorageClass(ctx context.Context, source string) (*storagev1.StorageClass, error) {
	storageClass, err := parseStorageClassYaml(source)
	if err != nil {
		return nil, err
	}

	_, err = f.KubeClient.StorageV1().StorageClasses().Get(ctx, storageClass.Name, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	}

	if apierrors.IsNotFound(err) {
		// StorageClass doesn't exists -> Create
		storageClass, err = f.KubeClient.StorageV1().StorageClasses().Create(ctx, storageClass, metav1.CreateOptions{})
		if err != nil {
			return nil, err
		}
	} else {
		// StorageClass already exists -> Update
		storageClass, err = f.KubeClient.StorageV1().StorageClasses().Update(ctx, storageClass, metav1.UpdateOptions{})
		if err != nil {
			return nil, err
		}
	}

	return storageClass, nil
}

func (f *Framework) DeleteStorageClass(ctx context.Context, source string) error {
	storageClass, err := parseStorageClassYaml(source)
	if err != nil {
		return err
	}

	return f.KubeClient.StorageV1().StorageClasses().Delete(ctx, storageClass.Name, metav1.DeleteOptions{})
}

func (f *Framework) UpdateStorageClass(ctx context.Context, storageClass *storagev1.StorageClass) error {
	_, err := f.KubeClient.StorageV1().StorageClasses().Update(ctx, storageClass, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	return nil
}

func parseStorageClassYaml(source string) (*storagev1.StorageClass, error) {
	manifest, err := SourceToIOReader(source)
	if err != nil {
		return nil, err
	}

	storageClass := storagev1.StorageClass{}
	if err := yaml.NewYAMLOrJSONDecoder(manifest, 100).Decode(&storageClass); err != nil {
		return nil, err
	}

	return &storageClass, nil
}
