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
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"

	servingv1alpha1 "github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	servingv1beta1 "github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aimstate "github.com/silogen/kaiwo/internal/controller/aim/state"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

var labelValueRegex = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

// sanitizeLabelValue converts a string to a valid Kubernetes label value.
// Valid label values must:
// - Be empty or consist of alphanumeric characters, '-', '_' or '.'
// - Start and end with an alphanumeric character
// - Be at most 63 characters
// Returns "unknown" if the sanitized value is empty.
func sanitizeLabelValue(s string) string {
	// Replace invalid characters with underscores
	sanitized := labelValueRegex.ReplaceAllString(s, "_")

	// Trim leading and trailing non-alphanumeric characters
	sanitized = strings.TrimLeft(sanitized, "_.-")
	sanitized = strings.TrimRight(sanitized, "_.-")

	// Truncate to 63 characters
	if len(sanitized) > 63 {
		sanitized = sanitized[:63]
		// Trim trailing non-alphanumeric after truncation
		sanitized = strings.TrimRight(sanitized, "_.-")
	}

	// Return "unknown" if fully sanitized string is empty
	if sanitized == "" {
		return "unknown"
	}

	return sanitized
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
			LabelKeyModelID:                sanitizeLabelValue(template.SpecCommon.AIMImageName),
		},
		OwnerReferences: []metav1.OwnerReference{ownerRef},
	}

	if namespace != nil {
		meta.Namespace = *namespace
	}

	return meta
}

func buildServingRuntimeSpec(template aimstate.TemplateState) servingv1alpha1.ServingRuntimeSpec {
	dshmSizeLimit := resource.MustParse("8Gi")
	return servingv1alpha1.ServingRuntimeSpec{
		SupportedModelFormats: []servingv1alpha1.SupportedModelFormat{
			{
				Name:    "aim",
				Version: baseutils.Pointer("1"),
			},
		},
		ServingRuntimePodSpec: servingv1alpha1.ServingRuntimePodSpec{
			ImagePullSecrets: CopyPullSecrets(template.ImagePullSecrets),
			Containers: []corev1.Container{
				{
					Name:  "kserve-container",
					Image: template.Image,
					Env: []corev1.EnvVar{
						{
							Name:  "AIM_MODEL_ID",
							Value: template.ModelSource.Name,
						},
						{
							Name:  "VLLM_ENABLE_METRICS",
							Value: "true",
						},
					},
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							"amd.com/gpu": *resource.NewQuantity(int64(template.Status.Profile.Metadata.GPUCount), resource.DecimalSI),
						},
						Limits: corev1.ResourceList{
							"amd.com/gpu": *resource.NewQuantity(int64(template.Status.Profile.Metadata.GPUCount), resource.DecimalSI),
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

func CopyPullSecrets(in []corev1.LocalObjectReference) []corev1.LocalObjectReference {
	if len(in) == 0 {
		return nil
	}
	out := make([]corev1.LocalObjectReference, len(in))
	copy(out, in)
	return out
}

// CopyEnvVars returns a deep-copy of environment variables.
func CopyEnvVars(in []corev1.EnvVar) []corev1.EnvVar {
	if len(in) == 0 {
		return nil
	}
	out := make([]corev1.EnvVar, len(in))
	copy(out, in)
	return out
}

// BuildInferenceService constructs a KServe InferenceService referencing a ServingRuntime or ClusterServingRuntime.
func BuildInferenceService(serviceState aimstate.ServiceState, ownerRef metav1.OwnerReference) *servingv1beta1.InferenceService {
	inferenceService := &servingv1beta1.InferenceService{
		TypeMeta: metav1.TypeMeta{
			APIVersion: servingv1beta1.SchemeGroupVersion.String(),
			Kind:       "InferenceService",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceState.Name,
			Namespace: serviceState.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       LabelValueServiceName,
				"app.kubernetes.io/component":  LabelValueServiceComponent,
				"app.kubernetes.io/managed-by": LabelValueManagedBy,
				LabelKeyTemplate:               serviceState.Template.Name,
				LabelKeyModelID:                sanitizeLabelValue(serviceState.ModelID),
			},
			OwnerReferences: []metav1.OwnerReference{ownerRef},
		},
		Spec: servingv1beta1.InferenceServiceSpec{
			Predictor: servingv1beta1.PredictorSpec{
				ComponentExtensionSpec: servingv1beta1.ComponentExtensionSpec{},
				PodSpec: servingv1beta1.PodSpec{
					ImagePullSecrets: CopyPullSecrets(serviceState.ImagePullSecrets),
				},
				Model: &servingv1beta1.ModelSpec{
					ModelFormat: servingv1beta1.ModelFormat{
						Name:    "aim",
						Version: baseutils.Pointer("1"),
					},
					Runtime: baseutils.Pointer(serviceState.RuntimeName),
					PredictorExtensionSpec: servingv1beta1.PredictorExtensionSpec{
						Container: corev1.Container{
							Env: CopyEnvVars(serviceState.Env),
						},
					},
				},
			},
		},
	}

	if serviceState.Replicas != nil {
		inferenceService.Spec.Predictor.MinReplicas = serviceState.Replicas
		inferenceService.Spec.Predictor.MaxReplicas = *serviceState.Replicas
	}

	if serviceState.ModelSource != nil {
		inferenceService.Spec.Predictor.Model.StorageURI = baseutils.Pointer(serviceState.ModelSource.SourceURI)
	}

	return inferenceService
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
