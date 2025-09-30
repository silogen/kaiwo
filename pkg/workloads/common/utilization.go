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
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ResourceUnderutilizationStatus string

const (
	KaiwoResourceUtilizationType                                = "ResourceUnderutilization"
	GpuResourceUtilizationNormal ResourceUnderutilizationStatus = "GpuUtilizationNormal"
	GpuResourceUtilizationLow    ResourceUnderutilizationStatus = "GpuUtilizationLow"
)

func SetEarlyTerminationIfLowUtilizationThresholdExceeded(ctx context.Context, workload KaiwoWorkload) bool {
	config := ConfigFromContext(ctx)
	status := workload.GetCommonStatusSpec()

	condition := meta.FindStatusCondition(status.Conditions, KaiwoResourceUtilizationType)
	if condition == nil || condition.Status == metav1.ConditionFalse {
		return false
	}

	terminateAfter, err := time.ParseDuration(config.ResourceMonitoring.TerminateUnderutilizedAfter)
	if err != nil {
		panic(err)
	}

	if time.Since(condition.LastTransitionTime.Time) > terminateAfter {
		SetEarlyTermination(ctx, workload, condition.Reason, "Terminating workload due to underutilized GPU resources")
		return true
	}
	return false
}
