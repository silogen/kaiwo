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
