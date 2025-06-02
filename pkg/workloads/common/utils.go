// Copyright 2025 Advanced Micro Devices, Inc.  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package common

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"k8s.io/apimachinery/pkg/api/errors"

	kaiwo "github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

// AddEntrypoint updates the entrypoint command in the PodTemplateSpec.
func AddEntrypoint(entrypoint string, podTemplateSpec *corev1.PodTemplateSpec) error {
	if entrypoint == "" {
		// logger.Info("Entrypoint is empty, skipping modification")
		return nil
	}

	if len(podTemplateSpec.Spec.Containers) == 0 {
		err := fmt.Errorf("podTemplateSpec has no containers to modify")
		return err
	}

	podTemplateSpec.Spec.Containers[0].Command = baseutils.ConvertMultilineEntrypoint(entrypoint, false).([]string)

	return nil
}

func addEnvVars(UserEnvVars []corev1.EnvVar, podTemplateSpec *corev1.PodTemplateSpec) error {
	if len(podTemplateSpec.Spec.Containers) == 0 {
		err := fmt.Errorf("podTemplateSpec has no containers to modify")
		return err
	}

	container := &podTemplateSpec.Spec.Containers[0]

	// Append UserEnvVars without overriding existing ones
	container.Env = append(container.Env, UserEnvVars...)

	return nil
}

// AreAnyPodsRunning checks if any pods are running that match a given namespace and labels
func AreAnyPodsRunning(ctx context.Context, k8sClient client.Client, namespace string, matchingLabels client.MatchingLabels) (bool, error) {
	podList := &corev1.PodList{}
	err := k8sClient.List(ctx, podList, client.InNamespace(namespace), matchingLabels)
	if err != nil {
		return false, err
	}

	for _, pod := range podList.Items {
		switch pod.Status.Phase {
		case corev1.PodRunning:
			return true, nil
		}
	}

	return false, nil
}

// capitalize returns s with its first rune upper-cased (handles Unicode).
func capitalize(s string) string {
	if s == "" {
		return ""
	}
	r, size := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[size:]
}

// ToPascalCase transforms a string like "hello there" into "HelloThere".
func ToPascalCase(s string) string {
	// Split on any whitespace
	words := strings.Fields(s)
	var b strings.Builder
	for _, w := range words {
		b.WriteString(capitalize(w))
	}
	return b.String()
}

func GetWorkloadPods(ctx context.Context, k8sClient client.Client, workload KaiwoWorkload) ([]corev1.Pod, error) {
	podList := &corev1.PodList{}
	obj := workload.GetKaiwoWorkloadObject()
	if err := k8sClient.List(ctx, podList, client.InNamespace(obj.GetNamespace()), client.MatchingLabels{
		KaiwoRunIdLabel: string(obj.GetUID()),
	}); err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}
	return podList.Items, nil
}

func GetWorkloadServices(ctx context.Context, k8sClient client.Client, workload KaiwoWorkload) ([]corev1.Service, error) {
	serviceList := &corev1.ServiceList{}
	return serviceList.Items, nil
}

func GetClusterQueueName(ctx context.Context, workload KaiwoWorkload) string {
	if clusterQueue := workload.GetCommonSpec().ClusterQueue; clusterQueue != "" {
		return clusterQueue
	} else {
		config := ConfigFromContext(ctx)
		return config.DefaultClusterQueueName
	}
}

// ObserveOverallStatus observes the overall status from a list of reconcilers, gathering any conditions they report as well
func ObserveOverallStatus(ctx context.Context, k8sClient client.Client, reconcilers []ResourceReconciler, previousWorkloadStatus kaiwo.WorkloadStatus) (*kaiwo.WorkloadStatus, []metav1.Condition, error) {
	var conditions []metav1.Condition
	var statuses []kaiwo.WorkloadStatus

	for _, reconciler := range reconcilers {
		obj := reconciler.GetInitializedObject()
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), obj); err == nil {
			reconcilerStatus, reconcilerConditions, err := reconciler.ObserveStatus(ctx, k8sClient, obj, previousWorkloadStatus)
			if err != nil {
				return nil, nil, fmt.Errorf("error observing status: %w", err)
			}
			conditions = append(conditions, reconcilerConditions...)
			if reconcilerStatus != nil {
				statuses = append(statuses, *reconcilerStatus)
			}
		} else if errors.IsNotFound(err) {
			return baseutils.Pointer(kaiwo.WorkloadStatusNew), nil, nil
		} else {
			return nil, nil, fmt.Errorf("failed to fetch object: %w", err)
		}
	}
	if len(statuses) == 0 {
		return baseutils.Pointer(kaiwo.WorkloadStatusNew), nil, nil
	}
	return baseutils.Pointer(kaiwo.DetermineOverallStatus(statuses)), conditions, nil
}

// ConditionsEqual checks if two sets of conditions are the same, ignoring the LastTransitionTime
func ConditionsEqual(a, b []metav1.Condition) bool {
	// Sort so the slices are in a consistent order
	sort.Slice(a, func(i, j int) bool { return a[i].Type < a[j].Type })
	sort.Slice(b, func(i, j int) bool { return b[i].Type < b[j].Type })

	// Use cmpopts to ignore LastTransitionTime
	return cmp.Equal(a, b,
		cmpopts.IgnoreFields(metav1.Condition{}, "LastTransitionTime"),
	)
}
