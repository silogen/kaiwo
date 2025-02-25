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
		Selector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				"job-name": "PLACEHOLDER",
			},
		},
	}
}

type BatchJobReconciler struct {
	workloadutils.ResourceReconcilerBase[*batchv1.Job]
	KaiwoJob *v1alpha1.KaiwoJob
}

func NewBatchJobReconciler(kaiwoJob *v1alpha1.KaiwoJob) *BatchJobReconciler {
	reconciler := &BatchJobReconciler{
		ResourceReconcilerBase: workloadutils.ResourceReconcilerBase[*batchv1.Job]{
			ObjectKey: client.ObjectKeyFromObject(kaiwoJob),
		},
		KaiwoJob: kaiwoJob,
	}
	reconciler.Self = reconciler
	return reconciler
}

func (r *BatchJobReconciler) Build(ctx context.Context, _ client.Client) (*batchv1.Job, error) {
	logger := log.FromContext(ctx)

	spec := r.KaiwoJob.Spec

	var jobSpec batchv1.JobSpec

	if spec.Job == nil {
		jobSpec = GetDefaultJobSpec(baseutils.ValueOrDefault(spec.Dangerous))
	} else {
		jobSpec = spec.Job.Spec
	}

	if jobSpec.Template.ObjectMeta.Labels == nil {
		jobSpec.Template.ObjectMeta.Labels = make(map[string]string)
	}

	jobSpec.Template.ObjectMeta.Labels["job-name"] = r.ObjectKey.Name

	if err := controllerutils.AddEntrypoint(ctx, baseutils.ValueOrDefault(spec.EntryPoint), &jobSpec.Template); err != nil {
		return nil, baseutils.LogErrorf(logger, "failed to add entrypoint: %v", err)
	}

	vendor := "AMD"
	if spec.GpuVendor != nil {
		vendor = *spec.GpuVendor
	}
	// logger.Info("GPU Vendor", "vendor", vendor)

	gpus := spec.Gpus

	if baseutils.ValueOrDefault(spec.Gpus) == 0 {
		gpuResourceKey := corev1.ResourceName(controllerutils.GetGpuResourceKey(vendor))
		if gpuQuantity, exists := jobSpec.Template.Spec.Containers[0].Resources.Requests[gpuResourceKey]; exists {
			gpus = baseutils.Pointer(int(gpuQuantity.Value()))
		}
		// logger.Info("GPU resource request", "resource", gpuResourceKey.String())
	}

	if err := controllerutils.AdjustResourceRequestsAndLimits(ctx, vendor, baseutils.ValueOrDefault(gpus), 1, baseutils.ValueOrDefault(gpus), &jobSpec.Template); err != nil {
		return nil, baseutils.LogErrorf(logger, "failed to adjust resource requests and limits", err)
	}

	if err := controllerutils.AddEnvVars(ctx, baseutils.ValueOrDefault(spec.EnvVars), &jobSpec.Template); err != nil {
		return nil, baseutils.LogErrorf(logger, "failed to add env vars", err)
	}

	jobSpec.Suspend = baseutils.Pointer(true)

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.ObjectKey.Name,
			Namespace: r.ObjectKey.Namespace,
			Labels:    r.KaiwoJob.Labels,
		},
		Spec: jobSpec,
	}

	if spec.Storage != nil && spec.Storage.StorageEnabled {
		if err := controllerutils.UpdatePodSpecStorage(ctx, &job.Spec.Template.Spec, *spec.Storage, r.ObjectKey.Name); err != nil {
			return nil, err
		}
	}

	return job, nil
}

func (r *BatchJobReconciler) GetEmptyObject() *batchv1.Job {
	return &batchv1.Job{}
}
