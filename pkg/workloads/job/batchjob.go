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
	"context"

	"k8s.io/apimachinery/pkg/api/resource"

	"sigs.k8s.io/controller-runtime/pkg/log"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/silogen/kaiwo/pkg/api/v1alpha1"

	workloadutils "github.com/silogen/kaiwo/pkg/workloads/utils"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	controllerutils "github.com/silogen/kaiwo/internal/controller/utils"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

func GetDefaultJobSpec(dangerous bool) batchv1.JobSpec {
	return batchv1.JobSpec{
		TTLSecondsAfterFinished: baseutils.Pointer(int32(3600)),
		Template:                controllerutils.GetPodTemplate(*resource.NewQuantity(1*1024*1024*1024, resource.BinarySI), dangerous),
	}
}

type BatchJobCommand struct {
	workloadutils.CommandBase[workloadutils.CommandStateBase]
	KaiwoJob *v1alpha1.KaiwoJob
}

func NewBatchJobCommand(base workloadutils.CommandBase[workloadutils.CommandStateBase], kaiwoJob *v1alpha1.KaiwoJob) *BatchJobCommand {
	cmd := &BatchJobCommand{
		CommandBase: base,
		KaiwoJob:    kaiwoJob,
	}
	cmd.Self = cmd
	return cmd
}

func (k *BatchJobCommand) Build(ctx context.Context, k8sClient client.Client) (client.Object, error) {
	logger := log.FromContext(ctx)

	kaiwoJob := k.KaiwoJob

	var jobSpec batchv1.JobSpec

	if kaiwoJob.Spec.Job == nil {
		// logger.Info("JobSpec is nil, using default JobSpec", "KaiwoJob", kaiwoJob.Name)
		jobSpec = GetDefaultJobSpec(baseutils.ValueOrDefault(kaiwoJob.Spec.Dangerous))
	} else {
		// logger.Info("JobSpec is provided", "KaiwoJob", kaiwoJob.Name)
		jobSpec = kaiwoJob.Spec.Job.Spec
	}

	if jobSpec.Template.ObjectMeta.Labels == nil {
		jobSpec.Template.ObjectMeta.Labels = make(map[string]string)
	}

	jobSpec.Template.ObjectMeta.Labels["job-name"] = kaiwoJob.Name

	if err := controllerutils.AddEntrypoint(ctx, baseutils.ValueOrDefault(kaiwoJob.Spec.EntryPoint), &jobSpec.Template); err != nil {
		return nil, baseutils.LogErrorf(logger, "failed to add entrypoint: %v", err)
	}

	vendor := "AMD"
	if kaiwoJob.Spec.GpuVendor != nil {
		vendor = *kaiwoJob.Spec.GpuVendor
	}
	// logger.Info("GPU Vendor", "vendor", vendor)

	if baseutils.ValueOrDefault(kaiwoJob.Spec.Gpus) == 0 {
		gpuResourceKey := corev1.ResourceName(controllerutils.GetGpuResourceKey(vendor))
		if gpuQuantity, exists := jobSpec.Template.Spec.Containers[0].Resources.Requests[gpuResourceKey]; exists {
			kaiwoJob.Spec.Gpus = baseutils.Pointer(int(gpuQuantity.Value()))
		}
		// logger.Info("GPU resource request", "resource", gpuResourceKey.String())
	}

	if err := controllerutils.AdjustResourceRequestsAndLimits(ctx, vendor, baseutils.ValueOrDefault(kaiwoJob.Spec.Gpus), 1, baseutils.ValueOrDefault(kaiwoJob.Spec.Gpus), &jobSpec.Template); err != nil {
		return nil, baseutils.LogErrorf(logger, "failed to adjust resource requests and limits", err)
	}

	if err := controllerutils.AddEnvVars(ctx, baseutils.ValueOrDefault(kaiwoJob.Spec.EnvVars), &jobSpec.Template); err != nil {
		return nil, baseutils.LogErrorf(logger, "failed to add env vars", err)
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
		if err := controllerutils.UpdatePodSpecStorage(ctx, &job.Spec.Template.Spec, *kaiwoJob.Spec.Storage, kaiwoJob.Name); err != nil {
			return nil, err
		}
	}

	return job, nil
}

func (k *BatchJobCommand) GetEmptyObject() client.Object {
	return &batchv1.Job{}
}

func (k *BatchJobCommand) GetObjectKey() client.ObjectKey {
	return client.ObjectKey{
		Namespace: k.KaiwoJob.Namespace,
		Name:      k.KaiwoJob.Name,
	}
}
