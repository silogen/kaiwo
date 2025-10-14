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

	servingv1alpha1 "github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	baseutils "github.com/silogen/kaiwo/pkg/utils"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
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

// ClusterServingRuntimeSpec defines parameters for creating a ClusterServingRuntime
type ClusterServingRuntimeSpec struct {
	Name             string
	ModelID          string
	Image            string
	Metric           *aimv1alpha1.AIMMetric
	OwnerRef         metav1.OwnerReference
	ServiceAccount   string
	ImagePullSecrets []corev1.LocalObjectReference
}

// ServingRuntimeSpec defines parameters for creating a ServingRuntime
type ServingRuntimeSpec struct {
	Name             string
	Namespace        string
	ModelID          string
	Image            string
	OwnerRef         metav1.OwnerReference
	ServiceAccount   string
	ImagePullSecrets []corev1.LocalObjectReference
}

// BuildClusterServingRuntime creates a KServe ClusterServingRuntime for a cluster-scoped template
func BuildClusterServingRuntime(spec ClusterServingRuntimeSpec) *servingv1alpha1.ClusterServingRuntime {
	runtime := &servingv1alpha1.ClusterServingRuntime{
		TypeMeta: metav1.TypeMeta{
			APIVersion: servingv1alpha1.SchemeGroupVersion.String(),
			Kind:       "ClusterServingRuntime",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: spec.Name,
			Labels: map[string]string{
				"app.kubernetes.io/name":       LabelValueRuntimeName,
				"app.kubernetes.io/component":  LabelValueRuntimeComponent,
				"app.kubernetes.io/managed-by": LabelValueManagedBy,
				LabelKeyModelID:                sanitizeLabelValue(spec.ModelID),
			},
			OwnerReferences: []metav1.OwnerReference{spec.OwnerRef},
		},
		Spec: servingv1alpha1.ServingRuntimeSpec{
			SupportedModelFormats: []servingv1alpha1.SupportedModelFormat{
				{
					Name:    "aim",
					Version: baseutils.Pointer("1"),
				},
			},
			ServingRuntimePodSpec: servingv1alpha1.ServingRuntimePodSpec{
				ImagePullSecrets: CopyPullSecrets(spec.ImagePullSecrets),
				Containers: []corev1.Container{
					{
						Name:  "kserve-container",
						Image: spec.Image,
						Args: []string{
							"--model-id", spec.ModelID,
						},
					},
				},
			},
		},
	}

	return runtime
}

// BuildServingRuntime creates a KServe ServingRuntime for a namespace-scoped template
func BuildServingRuntime(spec ServingRuntimeSpec) *servingv1alpha1.ServingRuntime {
	runtime := &servingv1alpha1.ServingRuntime{
		TypeMeta: metav1.TypeMeta{
			APIVersion: servingv1alpha1.SchemeGroupVersion.String(),
			Kind:       "ServingRuntime",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      spec.Name,
			Namespace: spec.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       LabelValueRuntimeName,
				"app.kubernetes.io/component":  LabelValueRuntimeComponent,
				"app.kubernetes.io/managed-by": LabelValueManagedBy,
				LabelKeyModelID:                sanitizeLabelValue(spec.ModelID),
			},
			OwnerReferences: []metav1.OwnerReference{spec.OwnerRef},
		},
		Spec: servingv1alpha1.ServingRuntimeSpec{
			SupportedModelFormats: []servingv1alpha1.SupportedModelFormat{
				{
					Name:    "aim",
					Version: baseutils.Pointer("1"),
				},
			},
			ServingRuntimePodSpec: servingv1alpha1.ServingRuntimePodSpec{
				ImagePullSecrets: CopyPullSecrets(spec.ImagePullSecrets),
				Containers: []corev1.Container{
					{
						Name:  "kserve-container",
						Image: spec.Image,
						Args: []string{
							"--model-id", spec.ModelID,
						},
					},
				},
			},
		},
	}

	return runtime
}

func CopyPullSecrets(in []corev1.LocalObjectReference) []corev1.LocalObjectReference {
	if len(in) == 0 {
		return nil
	}
	out := make([]corev1.LocalObjectReference, len(in))
	copy(out, in)
	return out
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
