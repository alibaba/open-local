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

	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
)

func (f *Framework) GetJob(ctx context.Context, ns, name string) (*batchv1.Job, error) {
	return f.KubeClient.BatchV1().Jobs(ns).Get(ctx, name, metav1.GetOptions{})
}

func (f *Framework) UpdateJob(ctx context.Context, job *batchv1.Job) (*batchv1.Job, error) {
	return f.KubeClient.BatchV1().Jobs(job.Namespace).Update(ctx, job, metav1.UpdateOptions{})
}

func MakeJob(source string) (*batchv1.Job, error) {
	manifest, err := SourceToIOReader(source)
	if err != nil {
		return nil, err
	}
	job := batchv1.Job{}
	if err := yaml.NewYAMLOrJSONDecoder(manifest, 100).Decode(&job); err != nil {
		return nil, fmt.Errorf("failed to decode file %s: %w", source, err)
	}

	return &job, nil
}

func (f *Framework) CreateJob(ctx context.Context, namespace string, job *batchv1.Job) error {
	job.Namespace = namespace
	_, err := f.KubeClient.BatchV1().Jobs(namespace).Create(ctx, job, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create job %s: %w", job.Name, err)
	}
	return nil
}

func (f *Framework) CreateOrUpdateJobAndWaitUntilReady(ctx context.Context, namespace string, job *batchv1.Job) error {
	job.Namespace = namespace
	d, err := f.KubeClient.BatchV1().Jobs(namespace).Get(ctx, job.Name, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to get job %s: %w", job.Name, err)
	}

	if apierrors.IsNotFound(err) {
		// Job doesn't exists -> Create
		_, err = f.KubeClient.BatchV1().Jobs(namespace).Create(ctx, job, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("failed to create job %s: %w", job.Name, err)
		}

		err = f.WaitForJobReady(ctx, namespace, job.Name, d.Status.Succeeded)
		if err != nil {
			return fmt.Errorf("after create, waiting for job %v to become ready timed out: %w", job.Name, err)
		}
	} else {
		// Job already exists -> Update
		_, err = f.KubeClient.BatchV1().Jobs(namespace).Update(ctx, job, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update job %s: %w", job.Name, err)
		}

		err = f.WaitForJobReady(ctx, namespace, job.Name, d.Status.Succeeded)
		if err != nil {
			return fmt.Errorf("after update, waiting for job %v to become ready timed out: %w", job.Name, err)
		}
	}

	return nil
}

func (f *Framework) WaitForJobReady(ctx context.Context, namespace, jobName string, expectedGeneration int32) error {
	err := wait.PollUntilWithContext(ctx, time.Second, func(ctx context.Context) (bool, error) {
		d, err := f.KubeClient.BatchV1().Jobs(namespace).Get(ctx, jobName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		if d.Status.Succeeded == expectedGeneration {
			return true, nil
		}
		return false, nil
	})
	return err
}

func (f *Framework) DeleteJob(ctx context.Context, namespace, name string) error {
	job, err := f.KubeClient.BatchV1().Jobs(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}

	job, err = f.KubeClient.BatchV1().Jobs(namespace).Update(ctx, job, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	return f.KubeClient.BatchV1().Jobs(namespace).Delete(ctx, job.Name, metav1.DeleteOptions{})
}

func (f *Framework) WaitUntilJobGone(ctx context.Context, kubeClient kubernetes.Interface, namespace, name string, timeout time.Duration) error {
	return wait.PollUntilWithContext(ctx, time.Second, func(ctx context.Context) (bool, error) {
		_, err := f.KubeClient.
			BatchV1().Jobs(namespace).
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
