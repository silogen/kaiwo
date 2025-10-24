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
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
)

func TestNormalizeGPUModel(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"MI300X", "MI300X"},
		{"mi300x", "MI300X"},
		{"MI300X (rev 2)", "MI300X"},
		{"Instinct MI300X", "MI300X"},
		{"A100-SXM4-40GB", "A100"},
		{"Tesla T4", "T4"},
		{"NVIDIA-A100-SXM4-40GB", "A100"},
		{"amd_mi210", "MI210"},
		{"RTX-4090", "4090"},   // Hyphenated variant extracts the numeric model
		{"RTX4090", "RTX4090"}, // No hyphen preserves prefix
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeGPUModel(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeGPUModel(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsGPUAvailableWithVariantNames(t *testing.T) {
	// Create a fake node with MI300X GPU
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Labels: map[string]string{
				"amd.com/gpu.product-name": "MI300X (rev 2)", // Variant name with extra tokens
			},
		},
		Status: corev1.NodeStatus{
			Capacity: corev1.ResourceList{
				"amd.com/gpu": resource.MustParse("4"),
			},
			Allocatable: corev1.ResourceList{
				"amd.com/gpu": resource.MustParse("4"),
			},
		},
	}

	scheme := GetTestScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()

	tests := []struct {
		name          string
		queryModel    string
		shouldBeFound bool
	}{
		{"exact match", "MI300X", true},
		{"lowercase", "mi300x", true},
		{"with extra tokens", "MI300X (rev 2)", true},
		{"with prefix", "Instinct MI300X", true},
		{"different model", "MI210", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			available, err := IsGPUAvailable(context.Background(), fakeClient, tt.queryModel)
			if err != nil {
				t.Fatalf("IsGPUAvailable failed: %v", err)
			}
			if available != tt.shouldBeFound {
				t.Errorf("IsGPUAvailable(%q) = %v, want %v", tt.queryModel, available, tt.shouldBeFound)
			}
		})
	}
}

func TestUpdateTemplateGPUAvailabilityNormalization(t *testing.T) {
	// Create a fake node with MI300X GPU labeled with variant name
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Labels: map[string]string{
				"amd.com/gpu.product-name": "Instinct MI300X",
			},
		},
		Status: corev1.NodeStatus{
			Capacity: corev1.ResourceList{
				"amd.com/gpu": resource.MustParse("8"),
			},
			Allocatable: corev1.ResourceList{
				"amd.com/gpu": resource.MustParse("8"),
			},
		},
	}

	scheme := GetTestScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()

	tests := []struct {
		name             string
		templateGPUModel string
		expectAvailable  bool
		expectedGPUModel string
	}{
		{
			name:             "simple model name",
			templateGPUModel: "MI300X",
			expectAvailable:  true,
			expectedGPUModel: "MI300X",
		},
		{
			name:             "lowercase model",
			templateGPUModel: "mi300x",
			expectAvailable:  true,
			expectedGPUModel: "MI300X",
		},
		{
			name:             "model with extra tokens",
			templateGPUModel: "MI300X (rev 2)",
			expectAvailable:  true,
			expectedGPUModel: "MI300X",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := aimv1alpha1.AIMServiceTemplateSpecCommon{
				AIMRuntimeParameters: aimv1alpha1.AIMRuntimeParameters{
					GpuSelector: &aimv1alpha1.AimGpuSelector{
						Model: tt.templateGPUModel,
						Count: 1,
					},
				},
			}

			obs := &TemplateObservation{}
			err := UpdateTemplateGPUAvailability(context.Background(), fakeClient, spec, obs)
			if err != nil {
				t.Fatalf("UpdateTemplateGPUAvailability failed: %v", err)
			}

			if obs.GPUAvailable != tt.expectAvailable {
				t.Errorf("GPUAvailable = %v, want %v", obs.GPUAvailable, tt.expectAvailable)
			}

			if obs.GPUModel != tt.expectedGPUModel {
				t.Errorf("GPUModel = %q, want %q", obs.GPUModel, tt.expectedGPUModel)
			}

			if !obs.GPUChecked {
				t.Error("GPUChecked should be true")
			}
		})
	}
}

func TestExtractGPUModelFromNodeLabels(t *testing.T) {
	tests := []struct {
		name         string
		labels       map[string]string
		resourceName string
		expected     string
	}{
		{
			name: "AMD product-name direct",
			labels: map[string]string{
				"amd.com/gpu.product-name": "MI300X",
			},
			resourceName: "amd.com/gpu",
			expected:     "MI300X",
		},
		{
			name: "AMD product-name with variant",
			labels: map[string]string{
				"amd.com/gpu.product-name": "MI300X (rev 2)",
			},
			resourceName: "amd.com/gpu",
			expected:     "MI300X",
		},
		{
			name: "AMD product-name count-encoded",
			labels: map[string]string{
				"amd.com/gpu.product-name.MI300X": "4",
			},
			resourceName: "amd.com/gpu",
			expected:     "MI300X",
		},
		{
			name: "AMD device-id",
			labels: map[string]string{
				"amd.com/gpu.device-id": "0x74a1",
			},
			resourceName: "amd.com/gpu",
			expected:     "MI300X",
		},
		{
			name: "NVIDIA product",
			labels: map[string]string{
				"nvidia.com/gpu.product": "A100-SXM4-40GB",
			},
			resourceName: "nvidia.com/gpu",
			expected:     "A100",
		},
		{
			name: "NVIDIA NFD label",
			labels: map[string]string{
				"feature.node.kubernetes.io/nvidia-gpu-model": "Tesla-T4",
			},
			resourceName: "nvidia.com/gpu",
			expected:     "T4",
		},
		{
			name: "AMD GPU with no identifying labels (strict)",
			labels: map[string]string{
				"kubernetes.io/hostname": "node1",
			},
			resourceName: "amd.com/gpu",
			expected:     "", // Strict mode returns empty string
		},
		{
			name: "NVIDIA GPU with no identifying labels (strict)",
			labels: map[string]string{
				"kubernetes.io/hostname": "node1",
			},
			resourceName: "nvidia.com/gpu",
			expected:     "", // Strict mode returns empty string
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractGPUModelFromNodeLabels(tt.labels, tt.resourceName)
			if result != tt.expected {
				t.Errorf("extractGPUModelFromNodeLabels() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestGetClusterGPUResourcesStrictMatching(t *testing.T) {
	scheme := GetTestScheme()

	// Node with properly labeled GPU
	labeledNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "labeled-node",
			Labels: map[string]string{
				"amd.com/gpu.product-name": "MI300X",
			},
		},
		Status: corev1.NodeStatus{
			Capacity: corev1.ResourceList{
				"amd.com/gpu": resource.MustParse("4"),
			},
			Allocatable: corev1.ResourceList{
				"amd.com/gpu": resource.MustParse("4"),
			},
		},
	}

	// Node with GPU resource but no identifying labels (should be excluded)
	unlabeledNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "unlabeled-node",
			Labels: map[string]string{
				"kubernetes.io/hostname": "unlabeled-node",
			},
		},
		Status: corev1.NodeStatus{
			Capacity: corev1.ResourceList{
				"amd.com/gpu": resource.MustParse("8"),
			},
			Allocatable: corev1.ResourceList{
				"amd.com/gpu": resource.MustParse("8"),
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(labeledNode, unlabeledNode).
		Build()

	resources, err := GetClusterGPUResources(context.Background(), fakeClient)
	if err != nil {
		t.Fatalf("GetClusterGPUResources failed: %v", err)
	}

	// Should only have MI300X from the labeled node
	if len(resources) != 1 {
		t.Errorf("Expected 1 GPU model, got %d: %+v", len(resources), resources)
	}

	mi300x, ok := resources["MI300X"]
	if !ok {
		t.Fatal("MI300X not found in resources")
	}

	// Should only count the 4 GPUs from the labeled node, not the 8 from unlabeled
	expectedCapacity := resource.MustParse("4")
	if mi300x.Capacity.Cmp(expectedCapacity) != 0 {
		t.Errorf("MI300X capacity = %v, want %v (unlabeled node GPUs should be excluded)",
			mi300x.Capacity.String(), expectedCapacity.String())
	}
}

func GetTestScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = aimv1alpha1.AddToScheme(scheme)
	return scheme
}
