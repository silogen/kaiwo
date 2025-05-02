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

package workloadutils

import (
	"context"
	"fmt"
	"strings"
	"time"

	kaiwo "github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"

	controllerutils "github.com/silogen/kaiwo/internal/controller/utils"

	"k8s.io/apimachinery/pkg/types"

	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metautil "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	baseutils "github.com/silogen/kaiwo/pkg/utils"
	common "github.com/silogen/kaiwo/pkg/workloads/common"
)

func UpdatePodSpec(config controllerutils.KaiwoConfigContext, kaiwoCommonMetaSpec kaiwo.CommonMetaSpec, labelContext common.KaiwoLabelContext, template *corev1.PodTemplateSpec, name string, replicas int, gpusPerReplica int, override bool, rayhead bool) error {
	// Update labels
	if template.ObjectMeta.Labels == nil {
		template.ObjectMeta.Labels = map[string]string{}
	}
	common.CopyLabels(kaiwoCommonMetaSpec.PodTemplateSpecLabels, &template.ObjectMeta)
	common.SetKaiwoSystemLabels(labelContext, &template.ObjectMeta)

	// Make sure there is an image set for each container
	// init containers are not included, as they are assumed to always be user given
	for i := range template.Spec.Containers {
		// If the container has no image set
		if template.Spec.Containers[i].Image == "" {
			if kaiwoCommonMetaSpec.Image != "" {
				// If a default image is provided, use it
				template.Spec.Containers[i].Image = kaiwoCommonMetaSpec.Image
			} else {
				// Otherwise use the default Ray image
				template.Spec.Containers[i].Image = config.Ray.DefaultRayImage
			}
		}
	}

	// Ensure that all image pull secrets are set
	if kaiwoCommonMetaSpec.ImagePullSecrets != nil {
		template.Spec.ImagePullSecrets = append(template.Spec.ImagePullSecrets, kaiwoCommonMetaSpec.ImagePullSecrets...)
	}

	if kaiwoCommonMetaSpec.SecretVolumes != nil {
		addSecretVolumes(template, kaiwoCommonMetaSpec.SecretVolumes)
	}

	// Update resources if specified in the CRD

	FillPodResources(&template.Spec, kaiwoCommonMetaSpec.Resources, false)

	vendor := "amd"
	if kaiwoCommonMetaSpec.GpuVendor != "" {
		vendor = kaiwoCommonMetaSpec.GpuVendor
	}

	gpus := kaiwoCommonMetaSpec.Gpus

	if kaiwoCommonMetaSpec.Gpus == 0 {
		gpuResourceKey := corev1.ResourceName(getGpuResourceKey(vendor, config.Nodes.DefaultGpuResourceKey))
		if gpuQuantity, exists := template.Spec.Containers[0].Resources.Requests[gpuResourceKey]; exists {
			gpus = int(gpuQuantity.Value())
		}
	}

	// Adjust resource requests and limits based on GPUs
	if err := adjustResourceRequestsAndLimits(config, vendor, gpus, replicas, gpusPerReplica, template, override, rayhead); err != nil {
		return fmt.Errorf("failed to adjust resource requests and limits: %w", err)
	}

	// Add environmental variables
	if err := addEnvVars(kaiwoCommonMetaSpec.Env, template); err != nil {
		return fmt.Errorf("failed to add env vars: %w", err)
	}

	// Attach storage
	if kaiwoCommonMetaSpec.Storage != nil && kaiwoCommonMetaSpec.Storage.StorageEnabled {
		updatePodSpecStorage(&template.Spec, *kaiwoCommonMetaSpec.Storage, name)
	}

	return nil
}

func addSecretVolumes(template *corev1.PodTemplateSpec, secretVolumes []kaiwo.SecretVolume) {
	for _, volume := range secretVolumes {
		template.Spec.Volumes = append(template.Spec.Volumes, corev1.Volume{
			Name: volume.Name,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: volume.SecretName,
				},
			},
		})
		for i := range template.Spec.Containers {
			template.Spec.Containers[i].VolumeMounts = append(template.Spec.Containers[i].VolumeMounts, corev1.VolumeMount{
				Name:      volume.Name,
				MountPath: volume.MountPath,
				SubPath:   volume.SubPath,
			})
		}
		for i := range template.Spec.InitContainers {
			template.Spec.InitContainers[i].VolumeMounts = append(template.Spec.InitContainers[i].VolumeMounts, corev1.VolumeMount{
				Name:      volume.Name,
				MountPath: volume.MountPath,
				SubPath:   volume.SubPath,
			})
		}
	}
}

// FillPodResources fills pod resources with a given template if they are not already set
func FillPodResources(podSpec *corev1.PodSpec, resources *corev1.ResourceRequirements, override bool) {
	for i := range podSpec.Containers {
		fillContainerResources(&podSpec.Containers[i], resources, override)
	}
	for i := range podSpec.InitContainers {
		fillContainerResources(&podSpec.InitContainers[i], resources, override)
	}
}

// fillContainerResources fills container resources with a given template if they are not already set
func fillContainerResources(container *corev1.Container, resources *corev1.ResourceRequirements, override bool) {
	if resources == nil {
		return
	}

	fillResourceList(&container.Resources.Requests, resources.Requests, override)
	fillResourceList(&container.Resources.Limits, resources.Limits, override)
}

func fillResourceList(dest *corev1.ResourceList, src corev1.ResourceList, override bool) {
	if *dest == nil {
		*dest = corev1.ResourceList{}
	}
	for k, v := range src {
		if _, exists := (*dest)[k]; override || !exists {
			(*dest)[k] = v
		}
	}
}

var (
	DefaultMemory = resource.MustParse("16Gi")
	DefaultCPU    = resource.MustParse("2")
)

func GetPodTemplate(config controllerutils.KaiwoConfigContext, dshmSize resource.Quantity, dangerous bool, resources corev1.ResourceRequirements, workloadContainerName string) corev1.PodTemplateSpec {
	resourceRequirements := resources.DeepCopy()
	if resourceRequirements.Requests == nil {
		resourceRequirements.Requests = corev1.ResourceList{}
	}
	if resourceRequirements.Limits == nil {
		resourceRequirements.Limits = corev1.ResourceList{}
	}
	if _, ok := resourceRequirements.Limits[corev1.ResourceMemory]; !ok {
		resourceRequirements.Limits[corev1.ResourceMemory] = DefaultMemory
	}
	if _, ok := resourceRequirements.Limits[corev1.ResourceCPU]; !ok {
		resourceRequirements.Limits[corev1.ResourceCPU] = DefaultCPU
	}
	if _, ok := resourceRequirements.Requests[corev1.ResourceMemory]; !ok {
		resourceRequirements.Requests[corev1.ResourceMemory] = DefaultMemory
	}
	if _, ok := resourceRequirements.Requests[corev1.ResourceCPU]; !ok {
		resourceRequirements.Requests[corev1.ResourceCPU] = DefaultCPU
	}

	podTemplate := corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			SchedulerName: config.Scheduling.KubeSchedulerName,
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:            workloadContainerName,
					ImagePullPolicy: corev1.PullAlways,
					Resources:       *resourceRequirements,
					VolumeMounts: []corev1.VolumeMount{
						{Name: "dshm", MountPath: "/dev/shm"},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "dshm",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{
							Medium: corev1.StorageMediumMemory,
							// Default size limit, can be overridden in `getPodTemplate`
							SizeLimit: &dshmSize,
						},
					},
				},
			},
		},
	}
	if !dangerous {
		podTemplate.Spec.SecurityContext = &corev1.PodSecurityContext{
			RunAsUser:  baseutils.Pointer(int64(1000)),
			RunAsGroup: baseutils.Pointer(int64(1000)),
			FSGroup:    baseutils.Pointer(int64(1000)),
		}
	}

	return podTemplate
}

func adjustResourceRequestsAndLimits(config controllerutils.KaiwoConfigContext, gpuVendor string, gpuCount int, replicas int, gpusPerReplica int, podTemplateSpec *corev1.PodTemplateSpec, override bool, rayhead bool) error {
	if len(podTemplateSpec.Spec.Containers) == 0 {
		err := fmt.Errorf("podTemplateSpec has no containers to modify")
		return err
	}

	gpuResourceKey := getGpuResourceKey(gpuVendor, config.Nodes.DefaultGpuResourceKey)

	// Modify resource requests/limits only if GPUs are requested
	if gpusPerReplica > 0 && !rayhead {
		updatedResources := getResourceRequestsAndLimits(gpuResourceKey, int32(gpusPerReplica))
		// Update the resources that have not been set yet (allowing the user to override the defaults here)
		fillContainerResources(&podTemplateSpec.Spec.Containers[0], &updatedResources, override)
	}

	// Append new GPU-related environment variables to the container
	envVarsToAppend := []corev1.EnvVar{
		{Name: "NUM_GPUS", Value: fmt.Sprintf("%d", gpuCount)},
		{Name: "NUM_REPLICAS", Value: fmt.Sprintf("%d", replicas)},
		{Name: "NUM_GPUS_PER_REPLICA", Value: fmt.Sprintf("%d", gpusPerReplica)},
	}

	// Append to existing environment variables
	podTemplateSpec.Spec.Containers[0].Env = append(podTemplateSpec.Spec.Containers[0].Env, envVarsToAppend...)

	// Check all containers for any GPU resource requests
	if config.Nodes.AddTaintsToGpuNodes {
		for _, container := range podTemplateSpec.Spec.Containers {
			requests := container.Resources.Requests
			if requests != nil {
				if _, ok := requests[corev1.ResourceName("nvidia.com/gpu")]; ok {
					goto AddToleration
				}
				if _, ok := requests[corev1.ResourceName("amd.com/gpu")]; ok {
					goto AddToleration
				}
			}
		}
	}
	return nil

AddToleration:
	podTemplateSpec.Spec.Tolerations = append(podTemplateSpec.Spec.Tolerations, corev1.Toleration{
		Key:      config.Nodes.DefaultGpuTaintKey,
		Operator: corev1.TolerationOpExists,
		Effect:   corev1.TaintEffectNoSchedule,
	})
	return nil
}

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

func getGpuResourceKey(vendor string, defaultVendor string) string {
	vendor = strings.ToUpper(vendor)
	switch vendor {
	case "NVIDIA":
		return "nvidia.com/gpu"
	case "AMD":
		return "amd.com/gpu"
	default:
		return defaultVendor
	}
}

func getResourceRequestsAndLimits(gpuResourceKey string, gpuCount int32) corev1.ResourceRequirements {
	return corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:                  resource.MustParse(fmt.Sprintf("%d", gpuCount*4)),
			corev1.ResourceMemory:               resource.MustParse(fmt.Sprintf("%dGi", gpuCount*32)),
			corev1.ResourceName(gpuResourceKey): resource.MustParse(fmt.Sprintf("%d", gpuCount)),
		},
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:                  resource.MustParse(fmt.Sprintf("%d", gpuCount*4)),
			corev1.ResourceMemory:               resource.MustParse(fmt.Sprintf("%dGi", gpuCount*32)),
			corev1.ResourceName(gpuResourceKey): resource.MustParse(fmt.Sprintf("%d", gpuCount)),
		},
	}
}

func updatePodSpecStorage(podSpec *corev1.PodSpec, storageSpec kaiwo.StorageSpec, ownerName string) {
	if !storageSpec.StorageEnabled || storageSpec.Data == nil {
		return
	}

	addStorageVolume := func(name string, claimName string) {
		podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
			Name: name,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: claimName,
				},
			},
		})
	}

	// Add volumes
	if storageSpec.Data.IsRequested() {
		addStorageVolume(common.DataStoragePostfix, baseutils.FormatNameWithPostfix(ownerName, common.DataStoragePostfix))
	}

	if storageSpec.HuggingFace.IsRequested() {
		addStorageVolume(common.HfStoragePostfix, baseutils.FormatNameWithPostfix(ownerName, common.HfStoragePostfix))
	}

	addVolumeMount := func(container *corev1.Container, name string, path string) {
		// logger.Info(fmt.Sprintf("Adding %s volume mount to %s", name, container.Name))
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      name,
			MountPath: path,
		})
	}

	addEnvVar := func(container *corev1.Container, name string, value string) {
		// logger.Info(fmt.Sprintf("Adding %s env var to %s", name, container.Name))
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  name,
			Value: value,
		})
	}

	addContainerInfo := func(container *corev1.Container) {
		if storageSpec.Data.IsRequested() {
			addVolumeMount(container, common.DataStoragePostfix, storageSpec.Data.MountPath)
		}
		if storageSpec.HuggingFace.IsRequested() {
			addVolumeMount(container, common.HfStoragePostfix, storageSpec.HuggingFace.MountPath)
			addEnvVar(container, "HF_HOME", storageSpec.HuggingFace.MountPath)
		}
	}

	for i := range podSpec.Containers {
		addContainerInfo(&podSpec.Containers[i])
	}
	for i := range podSpec.InitContainers {
		addContainerInfo(&podSpec.InitContainers[i])
	}
}

func GetEarliestPodStartTime(ctx context.Context, k8sClient client.Client, name string, namespace string) *metav1.Time {
	podList := &corev1.PodList{}
	if err := k8sClient.List(ctx, podList, client.InNamespace(namespace), client.MatchingLabels{common.KaiwoNameLabel: name}); err != nil {
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

func CheckPodStatus(ctx context.Context, k8sClient client.Client, name string, namespace string, startTime *metav1.Time) (lastStartTime *metav1.Time, status kaiwo.Status, err error) {
	logger := log.FromContext(ctx)

	podList := &corev1.PodList{}
	err = k8sClient.List(ctx, podList, client.InNamespace(namespace), client.MatchingLabels{common.KaiwoNameLabel: name})
	if err != nil {
		logger.Error(err, "Failed to list pods for job", "KaiwoJob", name)
		return nil, status, err
	}

	// FIXME
	// Interpreting pending pods to mean starting status is disabled for now, as this leads
	// KaiwoService (ray: false) to be interpreted as starting, even when it is still pending admission

	// var runningPods, pendingPods []corev1.Pod
	var runningPods []corev1.Pod

	for _, pod := range podList.Items {
		switch pod.Status.Phase {
		case corev1.PodRunning:
			runningPods = append(runningPods, pod)
			if pod.Status.StartTime != nil {
				if lastStartTime == nil || pod.Status.StartTime.After(lastStartTime.Time) {
					lastStartTime = pod.Status.StartTime
				}
			}
		}
	}

	if lastStartTime != nil && startTime == nil {
		return lastStartTime, status, nil
	}

	if len(runningPods) > 0 {
		status = kaiwo.StatusRunning
	} else {
		status = kaiwo.StatusPending
	}

	return lastStartTime, status, nil
}

func ValidateKaiwoResourceBeforeCreateOrUpdate(ctx context.Context, actual client.Object, kaiwoObjectMeta metav1.ObjectMeta) (*ctrl.Result, error) {
	if actual == nil && kaiwoObjectMeta.Labels != nil && kaiwoObjectMeta.Labels[common.KaiwoManagedLabel] == "true" {
		logger := log.FromContext(ctx)
		baseutils.Debug(logger, "Aborting reconciliation to avoid recreating a webhook-managed object")
		return &ctrl.Result{}, nil
	}
	return nil, nil
}

func ShouldPreempt(ctx context.Context, obj common.KaiwoWorkload, k8sClient client.Client) bool {
	duration := obj.GetDuration()
	startTime := obj.GetStartTime()
	if duration == nil || obj.GetStatus() != string(kaiwo.StatusRunning) {
		return false
	}
	now := time.Now()
	deadline := startTime.Time.Add(duration.Duration)
	if now.After(deadline) {
		var queue string
		if obj.GetClusterQueue() == "" {
			queue = common.DefaultClusterQueueName
		} else {
			queue = obj.GetClusterQueue()
		}
		hasDemand, err := ClusterHasGpuDemand(ctx, k8sClient, queue, obj.GetGPUVendor())
		if err != nil {
			return false
		}
		return hasDemand
	}
	return false
}

func ClusterHasGpuDemand(ctx context.Context, k8sClient client.Client, clusterQueue string, gpuVendor string) (bool, error) {
	var jobs kaiwo.KaiwoJobList
	if err := k8sClient.List(ctx, &jobs); err != nil {
		return false, err
	}
	for _, job := range jobs.Items {
		if job.Spec.ClusterQueue == "" {
			job.Spec.ClusterQueue = common.DefaultClusterQueueName
		}
		if job.Status.Status == kaiwo.StatusPending &&
			job.Spec.CommonMetaSpec.Gpus > 0 &&
			job.Spec.ClusterQueue == clusterQueue &&
			job.Spec.CommonMetaSpec.GpuVendor == gpuVendor &&
			isPendingForLong(ctx, job.ObjectMeta) {
			return true, nil
		}
	}

	var services kaiwo.KaiwoServiceList
	if err := k8sClient.List(ctx, &services); err != nil {
		return false, err
	}
	for _, svc := range services.Items {
		if svc.Spec.ClusterQueue == "" {
			svc.Spec.ClusterQueue = common.DefaultClusterQueueName
		}
		if svc.Status.Status == kaiwo.StatusPending &&
			svc.Spec.Gpus > 0 &&
			svc.Spec.ClusterQueue == clusterQueue &&
			svc.Spec.GpuVendor == gpuVendor &&
			isPendingForLong(ctx, svc.ObjectMeta) {
			return true, nil
		}
	}

	return false, nil
}

func isPendingForLong(ctx context.Context, meta metav1.ObjectMeta) bool {
	config := controllerutils.ConfigFromContext(ctx)
	age := time.Since(meta.CreationTimestamp.Time)
	return age > config.Scheduling.PendingThresholdForPreemption.Duration
}

func SyncGpuMetaFromPodSpec(podSpec corev1.PodSpec, meta *kaiwo.CommonMetaSpec) {
	if meta.Gpus == 0 {
		for _, c := range podSpec.Containers {
			for _, gpuKey := range []string{"amd.com/gpu", "nvidia.com/gpu"} {
				if gpuQty, ok := c.Resources.Requests[corev1.ResourceName(gpuKey)]; ok {
					if gpuVal := gpuQty.Value(); gpuVal > 0 {
						meta.Gpus = int(gpuVal)
						meta.GpuVendor = strings.Split(gpuKey, ".")[0]
						return
					}
				}
			}
		}
	}
}

func DeleteUnderlyingWorkload(ctx context.Context, uid types.UID, name string, namespace string, k8sClient client.Client) error {
	workloadTypes := []client.ObjectList{
		&appsv1.DeploymentList{},
		&rayv1.RayServiceList{},
		&batchv1.JobList{},
		&rayv1.RayJobList{},
	}

	var deletedAny bool

	for _, list := range workloadTypes {
		if err := k8sClient.List(ctx, list, client.InNamespace(namespace)); err != nil {
			return fmt.Errorf("failed to list workloads: %w", err)
		}

		items, err := metautil.ExtractList(list)
		if err != nil {
			return fmt.Errorf("failed to extract items: %w", err)
		}

		for _, item := range items {
			obj, ok := item.(client.Object)
			if !ok {
				continue
			}
			for _, owner := range obj.GetOwnerReferences() {
				if owner.UID == uid && owner.Controller != nil && *owner.Controller {
					foreground := metav1.DeletePropagationForeground
					if err := k8sClient.Delete(ctx, obj, &client.DeleteOptions{
						PropagationPolicy: &foreground,
					}); err != nil {
						return fmt.Errorf("failed to delete workload %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
					}
					deletedAny = true
				}
			}
		}
	}

	if !deletedAny {
		return fmt.Errorf("no owned workloads found to delete for KaiwoService %s", name)
	}
	return nil
}
