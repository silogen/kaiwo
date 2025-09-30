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

package common

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metautil "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	kueuev1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"
)

func GetKueueWorkload(ctx context.Context, k8sClient client.Client, namespace string, uid string) (*kueuev1beta1.Workload, error) {
	workloadList := &kueuev1beta1.WorkloadList{}
	listOptions := []client.ListOption{
		client.InNamespace(namespace),
		client.MatchingLabels(map[string]string{"kueue.x-k8s.io/job-uid": uid}),
	}
	if err := k8sClient.List(ctx, workloadList, listOptions...); err != nil {
		return nil, fmt.Errorf("failed to list kueue workloads: %w", err)
	}
	if len(workloadList.Items) == 0 {
		return nil, nil
	} else if len(workloadList.Items) > 1 {
		return nil, fmt.Errorf("expected a single workload for job '%s/%s', but found %d workloads", namespace, uid, len(workloadList.Items))
	}
	return &workloadList.Items[0], nil
}

// IsAdmitted checks if a workload is fully admitted by Kueue
func IsAdmitted(ctx context.Context, k8sClient client.Client, workload WorkloadReconciler) (bool, error) {
	workloads, err := workload.GetKueueWorkloads(ctx, k8sClient)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to get kueue workloads: %w", err)
	}
	if len(workloads) == 0 {
		return false, nil
	}
	for _, w := range workloads {
		admittedCondition := metautil.FindStatusCondition(w.Status.Conditions, kueuev1beta1.WorkloadAdmitted)
		if admittedCondition == nil || admittedCondition.Status == metav1.ConditionFalse {
			return false, nil
		}
	}
	return true, nil
}
