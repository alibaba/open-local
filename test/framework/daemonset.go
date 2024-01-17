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
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
)

func (f *Framework) GetDaemonSet(ctx context.Context, ns, name string) (*appsv1.DaemonSet, error) {
	return f.KubeClient.AppsV1().DaemonSets(ns).Get(ctx, name, metav1.GetOptions{})
}

func (f *Framework) UpdateDaemonSet(ctx context.Context, daemonset *appsv1.DaemonSet) (*appsv1.DaemonSet, error) {
	return f.KubeClient.AppsV1().DaemonSets(daemonset.Namespace).Update(ctx, daemonset, metav1.UpdateOptions{})
}

func MakeDaemonSet(source string) (*appsv1.DaemonSet, error) {
	manifest, err := SourceToIOReader(source)
	if err != nil {
		return nil, err
	}
	daemonset := appsv1.DaemonSet{}
	if err := yaml.NewYAMLOrJSONDecoder(manifest, 100).Decode(&daemonset); err != nil {
		return nil, fmt.Errorf("failed to decode file %s: %w", source, err)
	}

	return &daemonset, nil
}

func (f *Framework) CreateDaemonSet(ctx context.Context, namespace string, daemonset *appsv1.DaemonSet) error {
	daemonset.Namespace = namespace
	_, err := f.KubeClient.AppsV1().DaemonSets(namespace).Create(ctx, daemonset, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create daemonset %s: %w", daemonset.Name, err)
	}
	return nil
}

func (f *Framework) CreateOrUpdateDaemonSetAndWaitUntilReady(ctx context.Context, namespace string, daemonset *appsv1.DaemonSet) error {
	daemonset.Namespace = namespace
	d, err := f.KubeClient.AppsV1().DaemonSets(namespace).Get(ctx, daemonset.Name, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to get daemonset %s: %w", daemonset.Name, err)
	}

	if apierrors.IsNotFound(err) {
		// DaemonSet doesn't exists -> Create
		_, err = f.KubeClient.AppsV1().DaemonSets(namespace).Create(ctx, daemonset, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("failed to create daemonset %s: %w", daemonset.Name, err)
		}

		err = f.WaitForDaemonSetReady(ctx, namespace, daemonset.Name, 1)
		if err != nil {
			return fmt.Errorf("after create, waiting for daemonset %v to become ready timed out: %w", daemonset.Name, err)
		}
	} else {
		// DaemonSet already exists -> Update
		_, err = f.KubeClient.AppsV1().DaemonSets(namespace).Update(ctx, daemonset, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update daemonset %s: %w", daemonset.Name, err)
		}

		err = f.WaitForDaemonSetReady(ctx, namespace, daemonset.Name, d.Status.ObservedGeneration+1)
		if err != nil {
			return fmt.Errorf("after update, waiting for daemonset %v to become ready timed out: %w", daemonset.Name, err)
		}
	}

	return nil
}

func (f *Framework) WaitForDaemonSetReady(ctx context.Context, namespace, daemonsetName string, expectedGeneration int64) error {
	err := wait.PollUntilWithContext(ctx, time.Second, func(ctx context.Context) (bool, error) {
		d, err := f.KubeClient.AppsV1().DaemonSets(namespace).Get(ctx, daemonsetName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		if d.Status.ObservedGeneration == expectedGeneration {
			return true, nil
		}
		return false, nil
	})
	return err
}

func (f *Framework) DeleteDaemonSet(ctx context.Context, namespace, name string) error {
	d, err := f.KubeClient.AppsV1().DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}

	d, err = f.KubeClient.AppsV1().DaemonSets(namespace).Update(ctx, d, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	return f.KubeClient.AppsV1beta2().DaemonSets(namespace).Delete(ctx, d.Name, metav1.DeleteOptions{})
}

func (f *Framework) WaitUntilDaemonSetGone(ctx context.Context, kubeClient kubernetes.Interface, namespace, name string, timeout time.Duration) error {
	return wait.PollUntilWithContext(ctx, time.Second, func(ctx context.Context) (bool, error) {
		_, err := f.KubeClient.
			AppsV1beta2().DaemonSets(namespace).
			Get(ctx, name, metav1.GetOptions{})

		if err != nil {
			if apierrors.IsNotFound(err) {
				return true, nil
			}

			return false, err
		}

		return false, nil
	})
}
