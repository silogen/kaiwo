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

package workloadjob

import (
	"fmt"

	"github.com/silogen/kaiwo/pkg/api/v1alpha1"

	workloadutils "github.com/silogen/kaiwo/pkg/workloads2/utils"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	controllerutils "github.com/silogen/kaiwo/internal/controller/utils"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

var DefaultJobSpec = batchv1.JobSpec{
	TTLSecondsAfterFinished: baseutils.Pointer(int32(3600)),
	Template:                controllerutils.DefaultPodTemplateSpec,
}

type BatchJobCommand struct {
	workloadutils.CommandBase[workloadutils.CommandStateBase]
	KaiwoJob *v1alpha1.KaiwoJob
}

func (k *BatchJobCommand) Build() (client.Object, error) {
	kaiwoJob := k.KaiwoJob

	var jobSpec batchv1.JobSpec

	if kaiwoJob.Spec.Job == nil {
		// logger.Info("JobSpec is nil, using default JobSpec", "KaiwoJob", kaiwoJob.Name)
		jobSpec = DefaultJobSpec
	} else {
		jobSpec = kaiwoJob.Spec.Job.Spec
	}

	if jobSpec.Template.ObjectMeta.Labels == nil {
		jobSpec.Template.ObjectMeta.Labels = make(map[string]string)
	}

	jobSpec.Template.ObjectMeta.Labels["job-name"] = kaiwoJob.Name

	if err := controllerutils.AddEntrypoint(k.Context, baseutils.ValueOrDefault(kaiwoJob.Spec.EntryPoint), &jobSpec.Template); err != nil {
		return nil, fmt.Errorf("failed to add entrypoint: %v", err)
	}

	vendor := "AMD"
	if kaiwoJob.Spec.GpuVendor != nil {
		vendor = *kaiwoJob.Spec.GpuVendor
	}

	if baseutils.ValueOrDefault(kaiwoJob.Spec.Gpus) == 0 {
		gpuResourceKey := corev1.ResourceName(controllerutils.GetGpuResourceKey(vendor))
		if gpuQuantity, exists := jobSpec.Template.Spec.Containers[0].Resources.Requests[gpuResourceKey]; exists {
			kaiwoJob.Spec.Gpus = baseutils.Pointer(int(gpuQuantity.Value()))
		}
	}

	if err := controllerutils.AdjustResourceRequestsAndLimits(k.Context, vendor, baseutils.ValueOrDefault(kaiwoJob.Spec.Gpus), 1, baseutils.ValueOrDefault(kaiwoJob.Spec.Gpus), &jobSpec.Template); err != nil {
		return nil, fmt.Errorf("failed to adjust resource requests and limits: %v", err)
	}

	if err := controllerutils.AddEnvVars(k.Context, baseutils.ValueOrDefault(kaiwoJob.Spec.EnvVars), &jobSpec.Template); err != nil {
		return nil, fmt.Errorf("failed to add env vars: %v", err)
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kaiwoJob.Name,
			Namespace: kaiwoJob.Namespace,
			Labels:    kaiwoJob.Spec.Labels,
		},
		Spec: jobSpec,
	}

	if kaiwoJob.Spec.Storage != nil && kaiwoJob.Spec.Storage.StorageEnabled {
		if err := controllerutils.UpdatePodSpecStorage(k.Context, &job.Spec.Template.Spec, *kaiwoJob.Spec.Storage, kaiwoJob.Name); err != nil {
			return nil, err
		}
	}

	return job, nil
}
