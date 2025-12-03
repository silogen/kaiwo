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

package shared

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"

	servingv1alpha1 "github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	servingv1beta1 "github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
	"github.com/silogen/kaiwo/internal/controller/aim/helpers"
	aimstate "github.com/silogen/kaiwo/internal/controller/aim/state"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

const (
	// DefaultGPUResourceName is the default resource name for AMD GPUs in Kubernetes
	DefaultGPUResourceName = "amd.com/gpu"

	// DefaultSharedMemorySize is the default size allocated for /dev/shm in inference containers.
	// This is required for efficient inter-process communication in model serving workloads.
	DefaultSharedMemorySize = "8Gi"

	// KubernetesLabelValueMaxLength is the maximum length for a Kubernetes label value
	KubernetesLabelValueMaxLength = 63
)

var labelValueRegex = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

// SanitizeLabelValue converts a string to a valid Kubernetes label value.
// Valid label values must:
// - Be empty or consist of alphanumeric characters, '-', '_' or '.'
// - Start and end with an alphanumeric character
// - Be at most 63 characters
// Returns "unknown" if the sanitized value is empty.
func SanitizeLabelValue(s string) string {
	// Replace invalid characters with underscores
	sanitized := labelValueRegex.ReplaceAllString(s, "_")

	// Trim leading and trailing non-alphanumeric characters
	sanitized = strings.TrimLeft(sanitized, "_.-")
	sanitized = strings.TrimRight(sanitized, "_.-")

	// Truncate to maximum label value length
	if len(sanitized) > KubernetesLabelValueMaxLength {
		sanitized = sanitized[:KubernetesLabelValueMaxLength]
		// Trim trailing non-alphanumeric after truncation
		sanitized = strings.TrimRight(sanitized, "_.-")
	}

	// Return "unknown" if fully sanitized string is empty
	if sanitized == "" {
		return "unknown"
	}

	return sanitized
}

// GenerateInferenceServiceName creates a KServe InferenceService name that fits DNS label constraints.
// KServe creates hostnames in the format: {isvc-name}-predictor-{namespace}
// These hostnames must be ≤ 63 characters to comply with DNS label limits.
//
// If the original name would exceed the limit, this function:
// 1. Truncates the base name
// 2. Appends an 8-character hash of the full original name
// 3. Ensures the result is RFC1123 compliant
//
// The hash ensures uniqueness while keeping names deterministic and short.
func GenerateInferenceServiceName(serviceName, namespace string) string {
	const (
		// DNS label maximum length per RFC1123
		maxDNSLabelLength = 63
		// KServe adds "-predictor" to the ISVC name
		kserveSuffix = "-predictor"
		// Length of hash suffix we'll use (8 chars + 1 for hyphen)
		hashSuffixLength = 9
	)

	// Calculate how much space we have for the ISVC name
	// Format: {isvc-name}-predictor-{namespace}
	maxISVCNameLength := maxDNSLabelLength - len(kserveSuffix) - len(namespace) - 1 // -1 for the hyphen before namespace

	// If the service name fits, use it as-is
	if len(serviceName) <= maxISVCNameLength {
		return serviceName
	}

	// Otherwise, truncate and add a hash suffix for uniqueness
	// Reserve space for the hash suffix
	maxPrefixLength := maxISVCNameLength - hashSuffixLength
	if maxPrefixLength < 1 {
		// Edge case: namespace is so long we can barely fit anything
		// Use just the hash
		maxPrefixLength = 1
	}

	// Truncate the service name
	prefix := serviceName
	if len(prefix) > maxPrefixLength {
		prefix = prefix[:maxPrefixLength]
	}

	// Generate a deterministic hash from the full service name
	hash := sha256.Sum256([]byte(serviceName))
	hashStr := hex.EncodeToString(hash[:])[:8]

	// Combine prefix and hash
	result := fmt.Sprintf("%s-%s", prefix, hashStr)

	// Ensure RFC1123 compliance (handles edge cases like trailing hyphens)
	return baseutils.MakeRFC1123Compliant(result)
}

// BuildClusterServingRuntime creates a KServe ClusterServingRuntime for a cluster-scoped template.
func BuildClusterServingRuntime(template aimstate.TemplateState, ownerRef metav1.OwnerReference) *servingv1alpha1.ClusterServingRuntime {
	runtime := &servingv1alpha1.ClusterServingRuntime{
		TypeMeta: metav1.TypeMeta{
			APIVersion: servingv1alpha1.SchemeGroupVersion.String(),
			Kind:       "ClusterServingRuntime",
		},
		ObjectMeta: buildServingRuntimeObjectMeta(template, ownerRef, nil),
		Spec:       buildServingRuntimeSpec(template),
	}

	return runtime
}

// BuildServingRuntime creates a KServe ServingRuntime for a namespace-scoped template.
func BuildServingRuntime(template aimstate.TemplateState, ownerRef metav1.OwnerReference) *servingv1alpha1.ServingRuntime {
	runtime := &servingv1alpha1.ServingRuntime{
		TypeMeta: metav1.TypeMeta{
			APIVersion: servingv1alpha1.SchemeGroupVersion.String(),
			Kind:       "ServingRuntime",
		},
		ObjectMeta: buildServingRuntimeObjectMeta(template, ownerRef, &template.Namespace),
		Spec:       buildServingRuntimeSpec(template),
	}

	return runtime
}

func buildServingRuntimeObjectMeta(template aimstate.TemplateState, ownerRef metav1.OwnerReference, namespace *string) metav1.ObjectMeta {
	meta := metav1.ObjectMeta{
		Name: template.Name,
		Labels: map[string]string{
			"app.kubernetes.io/name":       LabelValueRuntimeName,
			"app.kubernetes.io/component":  LabelValueRuntimeComponent,
			"app.kubernetes.io/managed-by": LabelValueManagedBy,
			LabelKeyModelID:                SanitizeLabelValue(template.SpecCommon.ModelName),
		},
		OwnerReferences: []metav1.OwnerReference{ownerRef},
	}

	if namespace != nil {
		meta.Namespace = *namespace
	}

	return meta
}

// getGPUResourceName returns the GPU resource name from the template's gpuSelector.
// If the ResourceName is specified in gpuSelector, it will be used.
// Otherwise, the default value of "amd.com/gpu" is returned.
func getGPUResourceName(template aimstate.TemplateState) corev1.ResourceName {
	if template.SpecCommon.GpuSelector != nil && template.SpecCommon.GpuSelector.ResourceName != "" {
		return corev1.ResourceName(template.SpecCommon.GpuSelector.ResourceName)
	}
	return corev1.ResourceName(DefaultGPUResourceName)
}

func buildServingRuntimeSpec(template aimstate.TemplateState) servingv1alpha1.ServingRuntimeSpec {
	dshmSizeLimit := resource.MustParse(DefaultSharedMemorySize)

	// Determine model ID: prefer ModelSource.Name, fall back to ModelName
	//modelID := template.SpecCommon.ModelName
	//if template.ModelSource != nil {
	//	modelID = template.ModelSource.Name
	//}

	// Get the GPU resource name from the template, or use the default
	gpuResourceName := getGPUResourceName(template)

	return servingv1alpha1.ServingRuntimeSpec{
		// The AIM containers handle downloading themselves
		StorageHelper: &servingv1alpha1.StorageHelper{
			Disabled: true,
		},
		SupportedModelFormats: []servingv1alpha1.SupportedModelFormat{
			{
				Name:    "aim",
				Version: baseutils.Pointer("1"),
			},
		},
		ServingRuntimePodSpec: servingv1alpha1.ServingRuntimePodSpec{
			ImagePullSecrets: helpers.CopyPullSecrets(template.ImagePullSecrets),
			Containers: []corev1.Container{
				{
					Name:  "kserve-container",
					Image: template.Image,
					Env: []corev1.EnvVar{
						//{
						//	Name:  "AIM_MODEL_ID",
						//	Value: modelID,
						//},
						{
							Name:  "VLLM_ENABLE_METRICS",
							Value: "true",
						},
					},
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							gpuResourceName: *resource.NewQuantity(int64(template.Status.Profile.Metadata.GPUCount), resource.DecimalSI),
						},
						Limits: corev1.ResourceList{
							gpuResourceName: *resource.NewQuantity(int64(template.Status.Profile.Metadata.GPUCount), resource.DecimalSI),
						},
					},
					Ports: []corev1.ContainerPort{
						{
							ContainerPort: 8000,
							Name:          "http",
							Protocol:      corev1.ProtocolTCP,
						},
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "dshm",
							MountPath: "/dev/shm",
						},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "dshm",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{
							Medium:    corev1.StorageMediumMemory,
							SizeLimit: &dshmSizeLimit,
						},
					},
				},
			},
		},
	}
}

// CopyPullSecrets and CopyEnvVars helpers live under internal/controller/aim/helpers.

// BuildInferenceService constructs a KServe InferenceService referencing a ServingRuntime or ClusterServingRuntime.
func BuildInferenceService(serviceState aimstate.ServiceState, ownerRef metav1.OwnerReference) *servingv1beta1.InferenceService {
	labels := make(map[string]string, 8)
	if serviceState.Metadata.Labels != nil {
		for k, v := range serviceState.Metadata.Labels {
			labels[k] = v
		}
	}

	systemLabels := map[string]string{
		"app.kubernetes.io/name":       LabelValueServiceName,
		"app.kubernetes.io/component":  LabelValueServiceComponent,
		"app.kubernetes.io/managed-by": LabelValueManagedBy,
		LabelKeyTemplate:               serviceState.Template.Name,
		LabelKeyModelID:                SanitizeLabelValue(serviceState.ModelID),
		LabelKeyImageName:              SanitizeLabelValue(serviceState.Template.SpecCommon.ModelName),
		LabelKeyServiceName:            SanitizeLabelValue(serviceState.Name),
	}
	for k, v := range systemLabels {
		labels[k] = v
	}

	inferenceService := &servingv1beta1.InferenceService{
		TypeMeta: metav1.TypeMeta{
			APIVersion: servingv1beta1.SchemeGroupVersion.String(),
			Kind:       "InferenceService",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            GenerateInferenceServiceName(serviceState.Name, serviceState.Namespace),
			Namespace:       serviceState.Namespace,
			Annotations:     serviceState.Metadata.Annotations,
			Labels:          labels,
			OwnerReferences: []metav1.OwnerReference{ownerRef},
		},
		Spec: servingv1beta1.InferenceServiceSpec{
			Predictor: servingv1beta1.PredictorSpec{
				ComponentExtensionSpec: servingv1beta1.ComponentExtensionSpec{},
				PodSpec: servingv1beta1.PodSpec{
					ImagePullSecrets:   helpers.CopyPullSecrets(serviceState.ImagePullSecrets),
					ServiceAccountName: serviceState.ServiceAccountName,
				},
				Model: &servingv1beta1.ModelSpec{
					ModelFormat: servingv1beta1.ModelFormat{
						Name:    "aim",
						Version: baseutils.Pointer("1"),
					},
					Runtime: baseutils.Pointer(serviceState.RuntimeName),
					PredictorExtensionSpec: servingv1beta1.PredictorExtensionSpec{
						Container: corev1.Container{
							Env: helpers.CopyEnvVars(serviceState.Env),
						},
					},
				},
			},
		},
	}

	container := &inferenceService.Spec.Predictor.Model.Container
	container.Resources = resolveServiceResources(serviceState)

	if metric := serviceState.Template.StatusMetric(); metric != nil {
		inferenceService.Labels[LabelKeyMetric] = SanitizeLabelValue(string(*metric))
	}

	if precision := serviceState.Template.StatusPrecision(); precision != nil {
		inferenceService.Labels[LabelKeyPrecision] = SanitizeLabelValue(string(*precision))
	}

	// Auto-inject autoscaling-related annotations when AutoScaling is configured
	if serviceState.AutoScaling != nil {
		injectAutoscalingAnnotations(inferenceService)
	}

	// Handle replica configuration with priority: AutoScaling config > explicit min/max > legacy Replicas field
	if serviceState.AutoScaling != nil || serviceState.MinReplicas != nil || serviceState.MaxReplicas != nil {
		// Use explicit min/max replicas if provided
		if serviceState.MinReplicas != nil {
			inferenceService.Spec.Predictor.MinReplicas = serviceState.MinReplicas
		}
		if serviceState.MaxReplicas != nil {
			inferenceService.Spec.Predictor.MaxReplicas = *serviceState.MaxReplicas
		}

		// Apply autoscaling configuration if provided
		if serviceState.AutoScaling != nil {
			inferenceService.Spec.Predictor.AutoScaling = convertToKServeAutoScaling(serviceState.AutoScaling)
		}
	} else if serviceState.Replicas != nil {
		// Legacy: Use Replicas field for both min and max (backwards compatible)
		inferenceService.Spec.Predictor.MinReplicas = serviceState.Replicas
		inferenceService.Spec.Predictor.MaxReplicas = *serviceState.Replicas
	}

	return inferenceService
}

// convertToKServeAutoScaling converts AIM autoscaling config to KServe AutoScalingSpec format.
// Maps AIM's simplified autoscaling API to KServe's native autoscaling types, supporting
// various metric sources including PodMetric, Resource, and External metrics.
func convertToKServeAutoScaling(aimAutoScaling *aimv1alpha1.AIMServiceAutoScaling) *servingv1beta1.AutoScalingSpec {
	if aimAutoScaling == nil {
		return nil
	}

	kserveAutoScaling := &servingv1beta1.AutoScalingSpec{}

	if len(aimAutoScaling.Metrics) > 0 {
		kserveAutoScaling.Metrics = make([]servingv1beta1.MetricsSpec, len(aimAutoScaling.Metrics))
		for i, metric := range aimAutoScaling.Metrics {
			kserveMetric := servingv1beta1.MetricsSpec{
				Type: servingv1beta1.MetricSourceType(metric.Type),
			}

			// Handle PodMetric type - used for custom per-pod metrics (e.g., from OpenTelemetry)
			if metric.Type == "PodMetric" && metric.PodMetric != nil {
				kserveMetric.PodMetric = &servingv1beta1.PodMetricSource{}

				// Map metric identification and backend configuration
				if metric.PodMetric.Metric != nil {
					kserveMetric.PodMetric.Metric = servingv1beta1.PodMetrics{
						Backend:           servingv1beta1.PodsMetricsBackend(metric.PodMetric.Metric.Backend),
						ServerAddress:     metric.PodMetric.Metric.ServerAddress,
						MetricNames:       metric.PodMetric.Metric.MetricNames,
						Query:             metric.PodMetric.Metric.Query,
						OperationOverTime: metric.PodMetric.Metric.OperationOverTime,
					}
				}

				// Map target value configuration
				if metric.PodMetric.Target != nil {
					kserveMetric.PodMetric.Target = servingv1beta1.MetricTarget{
						Type: servingv1beta1.MetricTargetType(metric.PodMetric.Target.Type),
					}

					// Convert Value field (for "Value" type targets)
					if metric.PodMetric.Target.Value != "" {
						kserveMetric.PodMetric.Target.Value = servingv1beta1.NewMetricQuantity(metric.PodMetric.Target.Value)
					}

					// Convert AverageValue field (for "AverageValue" type targets)
					if metric.PodMetric.Target.AverageValue != "" {
						kserveMetric.PodMetric.Target.AverageValue = servingv1beta1.NewMetricQuantity(metric.PodMetric.Target.AverageValue)
					}

					// Copy AverageUtilization field (for "Utilization" type targets)
					if metric.PodMetric.Target.AverageUtilization != nil {
						kserveMetric.PodMetric.Target.AverageUtilization = metric.PodMetric.Target.AverageUtilization
					}
				}
			}

			// Future: Add support for Resource and External metric types here
			// For now, PodMetric covers the most common custom metrics use case

			kserveAutoScaling.Metrics[i] = kserveMetric
		}
	}

	return kserveAutoScaling
}

// injectAutoscalingAnnotations automatically adds required annotations when autoscaling is enabled.
// This includes KEDA autoscaler class, OpenTelemetry sidecar injection, and Prometheus metrics port.
// These annotations are only added if they don't already exist, allowing users to override defaults.
func injectAutoscalingAnnotations(inferenceService *servingv1beta1.InferenceService) {
	if inferenceService.Annotations == nil {
		inferenceService.Annotations = make(map[string]string)
	}

	// Add KEDA autoscaler class annotation if not already present
	// This tells KServe to use KEDA for autoscaling instead of default HPA
	if _, exists := inferenceService.Annotations["serving.kserve.io/autoscalerClass"]; !exists {
		inferenceService.Annotations["serving.kserve.io/autoscalerClass"] = "keda"
	}

	// Add OpenTelemetry sidecar injection annotation if not already present
	// This injects the OTel collector sidecar to collect metrics from the inference container
	// The sidecar name follows KServe naming convention: <service-name>-predictor
	if _, exists := inferenceService.Annotations["sidecar.opentelemetry.io/inject"]; !exists {
		predictorName := inferenceService.Name + "-predictor"
		inferenceService.Annotations["sidecar.opentelemetry.io/inject"] = predictorName
	}

	// Add Prometheus metrics port annotation if not already present
	// vLLM exposes metrics on port 8000 by default
	if _, exists := inferenceService.Annotations["prometheus.kserve.io/port"]; !exists {
		inferenceService.Annotations["prometheus.kserve.io/port"] = "8000"
	}
}

func resolveServiceResources(serviceState aimstate.ServiceState) corev1.ResourceRequirements {
	gpuCount := templateGPUCount(serviceState.Template)

	resolved := defaultResourceRequirementsForGPU(gpuCount)

	if serviceState.Resources != nil {
		resolved = mergeResourceRequirements(resolved, serviceState.Resources)
	}

	if gpuCount > 0 {
		gpuResourceName := getGPUResourceName(serviceState.Template)
		if resolved.Requests == nil {
			resolved.Requests = corev1.ResourceList{}
		}
		if resolved.Limits == nil {
			resolved.Limits = corev1.ResourceList{}
		}
		if _, ok := resolved.Requests[gpuResourceName]; !ok {
			if qty := resource.NewQuantity(gpuCount, resource.DecimalSI); qty != nil {
				resolved.Requests[gpuResourceName] = *qty
			}
		}
		if _, ok := resolved.Limits[gpuResourceName]; !ok {
			if qty := resource.NewQuantity(gpuCount, resource.DecimalSI); qty != nil {
				resolved.Limits[gpuResourceName] = *qty
			}
		}
	}

	return resolved
}

func templateGPUCount(template aimstate.TemplateState) int64 {
	if template.Status == nil {
		return 0
	}
	gpuCount := template.Status.Profile.Metadata.GPUCount
	if gpuCount <= 0 {
		return 0
	}
	return int64(gpuCount)
}

func defaultResourceRequirementsForGPU(gpuCount int64) corev1.ResourceRequirements {
	if gpuCount <= 0 {
		return corev1.ResourceRequirements{}
	}

	requests := corev1.ResourceList{
		corev1.ResourceCPU:    *resource.NewQuantity(gpuCount*4, resource.DecimalSI),
		corev1.ResourceMemory: quantityGi(gpuCount * 32),
	}

	limits := corev1.ResourceList{
		corev1.ResourceMemory: quantityGi(gpuCount * 48),
	}

	return corev1.ResourceRequirements{
		Requests: requests,
		Limits:   limits,
	}
}

func mergeResourceRequirements(base corev1.ResourceRequirements, override *corev1.ResourceRequirements) corev1.ResourceRequirements {
	if override == nil {
		return base
	}

	if len(override.Requests) > 0 {
		if base.Requests == nil {
			base.Requests = corev1.ResourceList{}
		}
		for name, qty := range override.Requests {
			base.Requests[name] = qty.DeepCopy()
		}
	}

	if len(override.Limits) > 0 {
		if base.Limits == nil {
			base.Limits = corev1.ResourceList{}
		}
		for name, qty := range override.Limits {
			base.Limits[name] = qty.DeepCopy()
		}
	}

	if override.Claims != nil {
		base.Claims = append([]corev1.ResourceClaim{}, override.Claims...)
	}

	return base
}

func quantityGi(value int64) resource.Quantity {
	if value <= 0 {
		return resource.Quantity{}
	}
	return resource.MustParse(fmt.Sprintf("%dGi", value))
}

// GetClusterServingRuntime fetches a ClusterServingRuntime by name
func GetClusterServingRuntime(ctx context.Context, k8sClient client.Client, name string) (*servingv1alpha1.ClusterServingRuntime, error) {
	runtime := &servingv1alpha1.ClusterServingRuntime{}
	if err := k8sClient.Get(ctx, client.ObjectKey{Name: name}, runtime); err != nil {
		return nil, err
	}
	return runtime, nil
}

// GetServingRuntime fetches a ServingRuntime by namespace and name
func GetServingRuntime(ctx context.Context, k8sClient client.Client, namespace, name string) (*servingv1alpha1.ServingRuntime, error) {
	runtime := &servingv1alpha1.ServingRuntime{}
	if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, runtime); err != nil {
		return nil, err
	}
	return runtime, nil
}
