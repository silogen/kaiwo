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
	"fmt"

	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	workloadshared "github.com/silogen/kaiwo/pkg/workloads/shared"
	workloadutils "github.com/silogen/kaiwo/pkg/workloads/utils"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kaiwov1alpha1 "github.com/silogen/kaiwo/pkg/api/v1alpha1"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

const DefaultKaiwoQueueConfigName = "kaiwo2"

// BuildKaiwoJobInvoker builds the invoker required to construct the Kaiwo job resources
func BuildKaiwoJobInvoker(ctx context.Context, scheme *runtime.Scheme, k8sClient client.Client, manifest *kaiwov1alpha1.KaiwoJob) (workloadutils.CommandInvoker, error) {
	logger := log.FromContext(ctx)

	// Ensure labels are set
	if manifest.Spec.Labels == nil {
		manifest.Spec.Labels = make(map[string]string)
	}

	user := ""
	if manifest.Spec.User != nil {
		user = *manifest.Spec.User
	}

	manifest.Spec.Labels[kaiwov1alpha1.UserLabel] = baseutils.SanitizeStringForKubernetes(user)

	if manifest.Spec.ClusterQueue == nil {
		manifest.Spec.Labels[kaiwov1alpha1.QueueLabel] = DefaultKaiwoQueueConfigName
	} else {
		manifest.Spec.Labels[kaiwov1alpha1.QueueLabel] = *manifest.Spec.ClusterQueue
	}

	state := &workloadutils.CommandStateBase{}

	base := workloadutils.CommandBase[workloadutils.CommandStateBase]{
		Owner:  manifest,
		Scheme: scheme,
		State:  state,
	}

	// Build invoker
	invoker := workloadutils.CommandInvoker{}

	invoker.AddCommand(NewKaiwoJobCommand(base, manifest))

	storageSpec := manifest.Spec.Storage

	if storageSpec != nil && storageSpec.StorageEnabled {

		// Ensure mount paths are set
		if storageSpec.Data != nil && storageSpec.Data.IsRequested() && storageSpec.Data.MountPath == "" {
			// logger.Info("Data storage mount path not set, using default:" + defaultDataMountPath)
			storageSpec.Data.MountPath = workloadshared.DefaultDataMountPath
		}
		if storageSpec.HuggingFace != nil && storageSpec.HuggingFace.IsRequested() && storageSpec.HuggingFace.MountPath == "" {
			// logger.Info("Hugging Face storage mount path not set, using default:" + defaultHfMountPath)
			storageSpec.HuggingFace.MountPath = workloadshared.DefaultHfMountPath
		}

		if storageSpec.Data != nil {
			// Add data PVC
			invoker.AddCommand(workloadshared.NewStorageCommand(
				base,
				workloadshared.DataStoragePostfix,
				storageSpec.AccessMode,
				storageSpec.StorageClassName,
				storageSpec.Data.StorageSize,
				workloadshared.SetStorage,
			))
		}
		if storageSpec.HuggingFace != nil {
			// Add HuggingFace PVC
			invoker.AddCommand(workloadshared.NewStorageCommand(
				base,
				workloadshared.HfStoragePostfix,
				storageSpec.AccessMode,
				storageSpec.StorageClassName,
				storageSpec.HuggingFace.StorageSize,
				workloadshared.SetStorage,
			))
		}

		if storageSpec.HasObjectStorageDownloads() || storageSpec.HasHfDownloads() {
			invoker.AddCommand(workloadshared.NewDownloadJobConfigMapCommand(base, storageSpec))
			invoker.AddCommand(workloadshared.NewDownloadJobCommand(base, storageSpec))
		}
	}

	if manifest.Spec.Image == nil || *manifest.Spec.Image == "" {
		manifest.Spec.Image = baseutils.Pointer(baseutils.DefaultRayImage)
	}

	invoker.AddCommand(workloadshared.NewKaiwoLocalQueueCommand(base, manifest.Spec.Labels[kaiwov1alpha1.QueueLabel], manifest.Namespace))

	if manifest.Spec.IsBatchJob() {
		// logger.Info("Manifest describes a batch job")
		invoker.AddCommand(NewBatchJobCommand(base, manifest))
	} else if manifest.Spec.IsRayJob() {
		// logger.Info("Manifest describes a Ray job")
		invoker.AddCommand(NewRayJobCommand(base, manifest))
	} else {
		return workloadutils.CommandInvoker{}, baseutils.LogErrorf(logger, "KaiwoJob does not specify a valid Job or RayJob. You must either set ray to true or false, or provide a rayClusterSpec", nil)
	}

	return invoker, nil
}

//type KaiwoJobCommandState struct {
//	workloadutils.CommandStateBase
//	KaiwoJob *kaiwov1alpha1.KaiwoJob
//}

type KaiwoJobManifestCommand struct {
	workloadutils.CommandBase[workloadutils.CommandStateBase]
	KaiwoJob *kaiwov1alpha1.KaiwoJob
}

func NewKaiwoJobCommand(base workloadutils.CommandBase[workloadutils.CommandStateBase], kaiwoJob *kaiwov1alpha1.KaiwoJob) *KaiwoJobManifestCommand {
	cmd := &KaiwoJobManifestCommand{
		CommandBase: base,
		KaiwoJob:    kaiwoJob,
	}
	cmd.Self = cmd
	return cmd
}

func (k *KaiwoJobManifestCommand) Build(ctx context.Context, k8sClient client.Client) (client.Object, error) {
	return k.KaiwoJob, nil
}

func (k *KaiwoJobManifestCommand) GetObjectKey() client.ObjectKey {
	return client.ObjectKey{
		Namespace: k.KaiwoJob.Namespace,
		Name:      k.KaiwoJob.Name,
	}
}

func (k *KaiwoJobManifestCommand) Update(ctx context.Context, k8sClient client.Client) error {
	logger := log.FromContext(ctx)

	obj, err := k.Get(ctx, k8sClient)
	if err != nil {
		return fmt.Errorf("error fetching object: %w", err)
	}
	kaiwoJob, ok := obj.(*kaiwov1alpha1.KaiwoJob)
	if !ok {
		return fmt.Errorf("expected KaiwoJob got %T", kaiwoJob)
	}

	jobSucceeded, jobFailed, err := checkJobCompletion(ctx, k8sClient, kaiwoJob)
	if err != nil {
		return baseutils.LogErrorf(logger, "failed to check job completion", err)
	}

	if kaiwoJob.Status.StartTime == nil {
		if startTime := getEarliestPodStartTime(ctx, k8sClient, kaiwoJob); startTime != nil {
			kaiwoJob.Status.StartTime = startTime
			// logger.Info("Set KaiwoJob StartTime", "KaiwoJob", kaiwoJob.Name, "StartTime", startTime)
		}
	}

	if jobSucceeded || jobFailed {
		return handleJobCompletion(ctx, k8sClient, kaiwoJob, jobSucceeded)
	}

	startTimeUpdated, err := checkPodStatus(ctx, k8sClient, kaiwoJob)
	if err != nil {
		return baseutils.LogErrorf(logger, "failed to check pod status", err)
	}

	if startTimeUpdated && kaiwoJob.Status.CompletionTime != nil {
		kaiwoJob.Status.Duration = int64(kaiwoJob.Status.CompletionTime.Time.Sub(kaiwoJob.Status.StartTime.Time).Seconds())
	}

	patch := client.MergeFrom(kaiwoJob.DeepCopy())

	if err := k8sClient.Status().Patch(ctx, kaiwoJob, patch); err != nil {
		return baseutils.LogErrorf(logger, "failed to update KaiwoJob status", err)
	}

	logger.Info("Updated KaiwoJob status", "KaiwoJob", kaiwoJob.Name, "Status", kaiwoJob.Status.Status)
	return nil
}

func (k *KaiwoJobManifestCommand) GetEmptyObject() client.Object {
	return &kaiwov1alpha1.KaiwoJob{}
}

func checkJobCompletion(ctx context.Context, k8sClient client.Client, kaiwoJob *kaiwov1alpha1.KaiwoJob) (bool, bool, error) {
	logger := log.FromContext(ctx)
	var jobSucceeded, jobFailed bool

	if kaiwoJob.Spec.IsBatchJob() {
		var job batchv1.Job
		if err := k8sClient.Get(ctx, client.ObjectKey{Name: kaiwoJob.Name, Namespace: kaiwoJob.Namespace}, &job); err == nil {
			jobFailed = job.Status.Failed > 0
			jobSucceeded = job.Status.Succeeded > 0
		} else if !errors.IsNotFound(err) {
			return false, false, baseutils.LogErrorf(logger, "failed to check job status", err)
		}
	} else if kaiwoJob.Spec.IsRayJob() {
		var rayJob rayv1.RayJob
		if err := k8sClient.Get(ctx, client.ObjectKey{Name: kaiwoJob.Name, Namespace: kaiwoJob.Namespace}, &rayJob); err == nil {
			switch rayJob.Status.JobStatus {
			case rayv1.JobStatusFailed:
				jobFailed = true
			case rayv1.JobStatusSucceeded:
				jobSucceeded = true
			}
		} else if !errors.IsNotFound(err) {
			return false, false, baseutils.LogErrorf(logger, "failed to check ray job status", err)
		}
	} else {
		return false, false, baseutils.LogErrorf(logger, "KaiwoJob does not specify a valid Job or RayJob", nil)
	}

	return jobSucceeded, jobFailed, nil
}

func handleJobCompletion(ctx context.Context, k8sClient client.Client, kaiwoJob *kaiwov1alpha1.KaiwoJob, jobSucceeded bool) error {
	logger := log.FromContext(ctx)
	now := metav1.Now()

	if jobSucceeded {
		kaiwoJob.Status.Status = kaiwov1alpha1.StatusComplete
	} else {
		kaiwoJob.Status.Status = kaiwov1alpha1.StatusFailed
	}

	kaiwoJob.Status.CompletionTime = &now

	if kaiwoJob.Status.StartTime != nil {
		kaiwoJob.Status.Duration = int64(kaiwoJob.Status.CompletionTime.Time.Sub(kaiwoJob.Status.StartTime.Time).Seconds())
	}

	if err := k8sClient.Status().Update(ctx, kaiwoJob); err != nil {
		return baseutils.LogErrorf(logger, "failed to update KaiwoJob status on completion", err)
	}

	logger.Info("Updated KaiwoJob on completion", "KaiwoJob", kaiwoJob.Name, "Status", kaiwoJob.Status.Status)
	return nil
}

func checkPodStatus(ctx context.Context, k8sClient client.Client, kaiwoJob *kaiwov1alpha1.KaiwoJob) (bool, error) {
	logger := log.FromContext(ctx)

	podList := &corev1.PodList{}
	err := k8sClient.List(ctx, podList, client.InNamespace(kaiwoJob.Namespace), client.MatchingLabels{"job-name": kaiwoJob.Name})
	if err != nil {
		return false, baseutils.LogErrorf(logger, "failed to list pods for job", err)
	}

	var runningPods, pendingPods []corev1.Pod
	var earliestRunningTime *metav1.Time

	for _, pod := range podList.Items {
		switch pod.Status.Phase {
		case corev1.PodRunning:
			runningPods = append(runningPods, pod)
			if pod.Status.StartTime != nil {
				if earliestRunningTime == nil || pod.Status.StartTime.Before(earliestRunningTime) {
					earliestRunningTime = pod.Status.StartTime
				}
			}
		case corev1.PodPending:
			pendingPods = append(pendingPods, pod)
		}
	}

	if earliestRunningTime != nil && kaiwoJob.Status.StartTime == nil {
		kaiwoJob.Status.StartTime = earliestRunningTime
		// logger.Info("Set KaiwoJob StartTime", "KaiwoJob", kaiwoJob.Name, "StartTime", earliestRunningTime)
		return true, nil
	}

	if len(runningPods) > 0 {
		kaiwoJob.Status.Status = kaiwov1alpha1.StatusRunning
	} else if len(pendingPods) > 0 {
		kaiwoJob.Status.Status = kaiwov1alpha1.StatusStarting
	} else {
		kaiwoJob.Status.Status = kaiwov1alpha1.StatusPending
	}

	return false, nil
}

func getEarliestPodStartTime(ctx context.Context, k8sClient client.Client, kaiwoJob *kaiwov1alpha1.KaiwoJob) *metav1.Time {
	podList := &corev1.PodList{}
	if err := k8sClient.List(ctx, podList, client.InNamespace(kaiwoJob.Namespace), client.MatchingLabels{"job-name": kaiwoJob.Name}); err != nil {
		return nil
	}

	var earliestStartTime *metav1.Time
	for _, pod := range podList.Items {
		if pod.Status.StartTime != nil {
			if earliestStartTime == nil || pod.Status.StartTime.Before(earliestStartTime) {
				earliestStartTime = pod.Status.StartTime
			}
		}
	}
	return earliestStartTime
}
