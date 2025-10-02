/*
MIT License

Copyright (c) 2025 Advanced Micro Devices, Inc.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package cliutils

import (
	"context"
	"fmt"
	"strings"

	"github.com/silogen/kaiwo/pkg/workloads/common"

	kaiwo "github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type PodSelectionPredicate func(pod corev1.Pod) bool

func IsGPUPod(pod corev1.Pod) bool {
	for _, container := range pod.Spec.Containers {
		for resourceName := range container.Resources.Limits {
			if resourceName == "nvidia.com/gpu" || resourceName == "amd.com/gpu" {
				return true
			}
		}
	}
	return false
}

func GetWorkload(ctx context.Context, k8sClient client.Client, workloadSelector string, namespace string) (common.KaiwoWorkload, error) {
	parts := strings.Split(workloadSelector, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid workload selector, must be type/name: %s", workloadSelector)
	}
	type_ := parts[0]
	name := parts[1]
	var obj client.Object
	switch type_ {
	case "job":
		obj = &kaiwo.KaiwoJob{}
	case "service":
		obj = &kaiwo.KaiwoService{}
	default:
		return nil, fmt.Errorf("invalid workload type: %s, must be either 'job' or 'service'", type_)
	}
	if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, obj); err != nil {
		return nil, fmt.Errorf("error getting Kaiwo %s %s/%s: %v", type_, namespace, name, err)
	}
	return obj.(common.KaiwoWorkload), nil
}
