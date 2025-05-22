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
	"unicode"
	"unicode/utf8"

	"github.com/project-codeflare/appwrapper/api/v1beta2"

	"k8s.io/client-go/util/retry"

	"k8s.io/client-go/tools/record"

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
	"github.com/silogen/kaiwo/pkg/workloads/common"
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

	if len(kaiwoCommonMetaSpec.GpuModels) > 0 && !rayhead {
		UpdatePodSpecWithGPUModelAffinity(template, kaiwoCommonMetaSpec.GpuModels, common.GPUModelLabel)
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

func UpdatePodSpecWithGPUModelAffinity(template *corev1.PodTemplateSpec, gpuModels []string, gpuModelLabel string) {
	if len(gpuModels) == 0 {
		return
	}

	selectorTerm := corev1.NodeSelectorTerm{
		MatchExpressions: []corev1.NodeSelectorRequirement{
			{
				Key:      gpuModelLabel,
				Operator: corev1.NodeSelectorOpIn,
				Values:   gpuModels,
			},
		},
	}

	nodeAffinity := &corev1.NodeAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{selectorTerm},
		},
	}

	if template.Spec.Affinity == nil {
		template.Spec.Affinity = &corev1.Affinity{}
	}

	template.Spec.Affinity.NodeAffinity = nodeAffinity
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

const WorkloadPreempted WorkloadPreemptedReason = "WorkloadPreempted"

type WorkloadPreemptedReason string

func ShouldPreempt(ctx context.Context, obj common.KaiwoWorkload, k8sClient client.Client) bool {
	duration := obj.GetDuration()
	startTime := obj.GetStartTime()
	if duration == nil || obj.GetStatusString() != string(kaiwo.StatusRunning) {
		return false
	}
	now := time.Now()
	deadline := startTime.Time.Add(duration.Duration)
	if now.After(deadline) {
		config := controllerutils.ConfigFromContext(ctx)
		var queue string
		if obj.GetClusterQueue() == "" {
			queue = config.DefaultClusterQueueName
		} else {
			queue = obj.GetClusterQueue()
		}
		hasDemand, err := ClusterHasGpuDemand(ctx, k8sClient, queue, obj.GetGPUVendor(), config)
		if err != nil {
			return false
		}
		return hasDemand
	}
	return false
}

func ClusterHasGpuDemand(ctx context.Context, k8sClient client.Client, clusterQueue string, gpuVendor string, config controllerutils.KaiwoConfigContext) (bool, error) {
	var jobs kaiwo.KaiwoJobList
	if err := k8sClient.List(ctx, &jobs); err != nil {
		return false, err
	}
	for _, job := range jobs.Items {
		if job.Spec.ClusterQueue == "" {
			job.Spec.ClusterQueue = config.DefaultClusterQueueName
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
			svc.Spec.ClusterQueue = config.DefaultClusterQueueName
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
	logger := log.FromContext(ctx)
	config := controllerutils.ConfigFromContext(ctx)
	age := time.Since(meta.CreationTimestamp.Time)
	duration, err := time.ParseDuration(config.Scheduling.PendingThresholdForPreemption)
	if err != nil {
		logger.Error(err, "Failed to parse duration", "duration", config.Scheduling.PendingThresholdForPreemption)
		return false
	}
	return age > duration
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

// RetryForWorkload provides a wrapper for an atomic action on a workload object by first reading a fresh copy of the object
// and passing it to the function, and retrying on conflict
func RetryForWorkload[T common.KaiwoWorkload](ctx context.Context, k8sClient client.Client, workload T, fn func(T) error) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		copyObj := workload.DeepCopyObject().(T)
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(workload), copyObj); err != nil {
			return err
		}
		return fn(copyObj)
	})
}

const WorkloadEarlyTerminationConditionType = "WorkloadTerminatedEarly"

type WorkloadTerminationReason string

// TerminateWorkload terminates a given workload by deleting all the child objects and setting
// an early termination condition and emitting an event
func TerminateWorkload(
	ctx context.Context,
	k8sClient client.Client,
	recorder record.EventRecorder,
	workload common.KaiwoWorkload,
) error {
	logger := log.FromContext(ctx)
	var condition *metav1.Condition
	if err := RetryForWorkload(ctx, k8sClient, workload, func(obj common.KaiwoWorkload) error {
		statusSpec := obj.GetCommonStatusSpec()
		statusSpec.Status = kaiwo.StatusTerminated

		condition = metautil.FindStatusCondition(statusSpec.Conditions, WorkloadEarlyTerminationConditionType)
		if condition == nil {
			condition = &metav1.Condition{
				Type:    WorkloadEarlyTerminationConditionType,
				Status:  metav1.ConditionTrue,
				Reason:  "EarlyTermination",
				Message: "Workload terminated early (check events and other conditions for likely causes)",
			}
			metautil.SetStatusCondition(&statusSpec.Conditions, *condition)
		} else {
			// Conditional already exists (created when setting the TERMINATING status), just update to true
			condition.Status = metav1.ConditionTrue
			metautil.SetStatusCondition(&statusSpec.Conditions, *condition)
		}

		// Update status first to avoid doing any further reconciliation
		if err := k8sClient.Status().Update(ctx, obj); err != nil {
			return fmt.Errorf("failed to update status: %w", err)
		}

		return nil
	}); err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}

	logger.Info("Terminating workload", "reason", condition.Reason, "message", condition.Message)

	if err := DeleteUnderlyingResources(ctx, workload.GetUID(), workload.GetName(), workload.GetNamespace(), k8sClient); err != nil {
		return fmt.Errorf("failed to delete workload resources: %w", err)
	}

	recorder.Event(workload, corev1.EventTypeWarning, condition.Reason, condition.Message)

	return nil
}

// DeleteUnderlyingResources deletes all the underlying resources that a workload owns
func DeleteUnderlyingResources(ctx context.Context, uid types.UID, name string, namespace string, k8sClient client.Client) error {
	resourceTypes := []client.ObjectList{
		&appsv1.DeploymentList{},
		&rayv1.RayServiceList{},
		&v1beta2.AppWrapperList{},
		&batchv1.JobList{},
		&rayv1.RayJobList{},
		&corev1.PersistentVolumeClaimList{},
	}

	logger := log.FromContext(ctx)

	var deletedAny bool

	for _, list := range resourceTypes {
		if err := k8sClient.List(ctx, list, client.InNamespace(namespace)); err != nil {
			return fmt.Errorf("failed to list resources: %w", err)
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
						return fmt.Errorf("failed to delete resource %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
					}
					logger.Info("Deleted resource", "namespace", obj.GetNamespace(), "name", obj.GetName(), "kind", obj.GetObjectKind().GroupVersionKind().Kind)
					deletedAny = true
				}
			}
		}
	}

	if !deletedAny {
		return fmt.Errorf("no owned resources found to delete for %s", name)
	}
	return nil
}

// CheckKaiwoWorkloadShouldBeTerminatedForUnderutilization checks if the Kaiwo workload should be terminated due to resource underutilization
func CheckKaiwoWorkloadShouldBeTerminatedForUnderutilization(ctx context.Context, workload common.KaiwoWorkload) (bool, string) {
	config := controllerutils.ConfigFromContext(ctx)
	logger := log.FromContext(ctx).WithName("CheckKaiwoWorkloadShouldBeTerminatedForUnderutilization")

	if !config.ResourceMonitoring.TerminateUnderutilized {
		return false, ""
	}

	condition := metautil.FindStatusCondition(workload.GetCommonStatusSpec().Conditions, kaiwo.KaiwoResourceUtilizationType)
	if condition == nil || condition.Status == metav1.ConditionFalse {
		return false, ""
	}

	terminateAfter, err := time.ParseDuration(config.ResourceMonitoring.TerminateUnderutilizedAfter)
	if err != nil {
		logger.Error(err, "Failed to parse duration", "duration", config.ResourceMonitoring.TerminateUnderutilizedAfter)
		return false, ""
	}

	if time.Since(condition.LastTransitionTime.Time) > terminateAfter {
		return true, condition.Reason
	}

	return false, ""
}

// SetEarlyTermination flags a workload for early termination by
// 1. Setting the status to TERMINATING
// 2. Creating the WorkloadTerminatedEarly condition, but keeping its status as False (in order to record the reason)
func SetEarlyTermination(ctx context.Context, k8sClient client.Client, workload common.KaiwoWorkload, reason string, message string) error {
	logger := log.FromContext(ctx)

	logger.Info("Flagging workload for early termination", "reason", reason, "message", message)

	statusSpec := workload.GetCommonStatusSpec()
	statusSpec.Status = kaiwo.StatusTerminating

	metautil.SetStatusCondition(&statusSpec.Conditions, metav1.Condition{
		Type:    WorkloadEarlyTerminationConditionType,
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: message,
	})

	// mutate and Status().Update
	if err := k8sClient.Status().Update(ctx, workload); err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}

	return nil
}

func GetWorkloadPods(ctx context.Context, k8sClient client.Client, workload common.KaiwoWorkload) ([]corev1.Pod, error) {
	podList := &corev1.PodList{}
	if err := k8sClient.List(ctx, podList, client.InNamespace(workload.GetNamespace()), client.MatchingLabels{
		common.KaiwoRunIdLabel: string(workload.GetUID()),
	}); err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}
	return podList.Items, nil
}
