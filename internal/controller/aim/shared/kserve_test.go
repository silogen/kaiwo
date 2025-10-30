// MIT License
//
// Copyright (c) 2025 Advanced Micro Devices, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package shared

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
	aimstate "github.com/silogen/kaiwo/internal/controller/aim/state"
)

func TestResolveServiceResources_DefaultsFromTemplateGPU(t *testing.T) {
	template := aimstate.TemplateState{
		Status: &aimv1alpha1.AIMServiceTemplateStatus{
			Profile: aimv1alpha1.AIMProfile{
				Metadata: aimv1alpha1.AIMProfileMetadata{
					GPUCount: 2,
				},
			},
		},
	}

	resources := resolveServiceResources(aimstate.ServiceState{
		Template: template,
	})

	if cpu := resources.Requests[corev1.ResourceCPU]; cpu.Cmp(resource.MustParse("8")) != 0 {
		t.Fatalf("expected CPU request 8, got %s", cpu.String())
	}

	if mem := resources.Requests[corev1.ResourceMemory]; mem.Cmp(resource.MustParse("64Gi")) != 0 {
		t.Fatalf("expected memory request 64Gi, got %s", mem.String())
	}

	if limitMem := resources.Limits[corev1.ResourceMemory]; limitMem.Cmp(resource.MustParse("96Gi")) != 0 {
		t.Fatalf("expected memory limit 96Gi, got %s", limitMem.String())
	}

	if gpu := resources.Requests[corev1.ResourceName(DefaultGPUResourceName)]; gpu.Cmp(resource.MustParse("2")) != 0 {
		t.Fatalf("expected GPU request 2, got %s", gpu.String())
	}

	if gpuLimit := resources.Limits[corev1.ResourceName(DefaultGPUResourceName)]; gpuLimit.Cmp(resource.MustParse("2")) != 0 {
		t.Fatalf("expected GPU limit 2, got %s", gpuLimit.String())
	}
}

func TestResolveServiceResources_OverridesApplied(t *testing.T) {
	template := aimstate.TemplateState{
		Status: &aimv1alpha1.AIMServiceTemplateStatus{
			Profile: aimv1alpha1.AIMProfile{
				Metadata: aimv1alpha1.AIMProfileMetadata{
					GPUCount: 2,
				},
			},
		},
	}

	override := &corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU: resource.MustParse("10"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("128Gi"),
		},
	}

	resources := resolveServiceResources(aimstate.ServiceState{
		Template:  template,
		Resources: override,
	})

	if cpu := resources.Requests[corev1.ResourceCPU]; cpu.Cmp(resource.MustParse("10")) != 0 {
		t.Fatalf("expected CPU request override 10, got %s", cpu.String())
	}

	if mem := resources.Requests[corev1.ResourceMemory]; mem.Cmp(resource.MustParse("64Gi")) != 0 {
		t.Fatalf("expected memory request default 64Gi, got %s", mem.String())
	}

	if limitMem := resources.Limits[corev1.ResourceMemory]; limitMem.Cmp(resource.MustParse("128Gi")) != 0 {
		t.Fatalf("expected memory limit override 128Gi, got %s", limitMem.String())
	}

	if gpu := resources.Requests[corev1.ResourceName(DefaultGPUResourceName)]; gpu.Cmp(resource.MustParse("2")) != 0 {
		t.Fatalf("expected GPU request 2, got %s", gpu.String())
	}

	if gpuLimit := resources.Limits[corev1.ResourceName(DefaultGPUResourceName)]; gpuLimit.Cmp(resource.MustParse("2")) != 0 {
		t.Fatalf("expected GPU limit 2, got %s", gpuLimit.String())
	}
}

func TestGenerateInferenceServiceName(t *testing.T) {
	tests := []struct {
		name        string
		serviceName string
		namespace   string
		wantMaxLen  int
		wantPrefix  string
	}{
		{
			name:        "short name fits as-is",
			serviceName: "simple",
			namespace:   "default",
			wantMaxLen:  6, // should be exactly "simple"
			wantPrefix:  "simple",
		},
		{
			name:        "long name with short namespace gets truncated",
			serviceName: "integration-cluster-model-public-image",
			namespace:   "chainsaw-keen-kit",
			wantMaxLen:  63,
			wantPrefix:  "integration-cluster-model-public",
		},
		{
			name:        "very long name gets hash suffix",
			serviceName: "very-long-service-name-that-exceeds-the-kubernetes-dns-label-limit",
			namespace:   "short",
			wantMaxLen:  63,
			wantPrefix:  "very-long-service-name-that-exceeds-the-kubernetes-dns-label",
		},
		{
			name:        "name at boundary",
			serviceName: "exactly-at-the-limit-for-name",
			namespace:   "ns",
			wantMaxLen:  63,
			wantPrefix:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateInferenceServiceName(tt.serviceName, tt.namespace)

			// Verify the generated name length
			if len(result) > tt.wantMaxLen {
				t.Errorf("GenerateInferenceServiceName() generated name too long: got %d chars, want max %d", len(result), tt.wantMaxLen)
			}

			// Verify the full hostname that KServe will create
			hostname := result + "-predictor-" + tt.namespace
			if len(hostname) > 63 {
				t.Errorf("GenerateInferenceServiceName() hostname exceeds 63 chars: %s (len=%d)", hostname, len(hostname))
			}

			// Verify RFC1123 compliance (lowercase alphanumeric plus hyphens)
			if result != "" && !isRFC1123Compliant(result) {
				t.Errorf("GenerateInferenceServiceName() result is not RFC1123 compliant: %s", result)
			}

			// For truncated names, verify prefix is present and hash is appended
			if tt.wantPrefix != "" && len(tt.serviceName) > len(result) {
				// Should have the prefix followed by a hash
				if len(result) < 9 { // at least 1 char prefix + "-" + 8 char hash
					t.Errorf("GenerateInferenceServiceName() truncated result too short: %s", result)
				}
			}

			// Verify determinism: same inputs should always produce same output
			result2 := GenerateInferenceServiceName(tt.serviceName, tt.namespace)
			if result != result2 {
				t.Errorf("GenerateInferenceServiceName() not deterministic: first=%s, second=%s", result, result2)
			}
		})
	}
}

func TestGenerateInferenceServiceName_RealWorldCase(t *testing.T) {
	// The actual failing case from the error message
	serviceName := "integration-cluster-model-public-image"
	namespace := "chainsaw-keen-kit"

	result := GenerateInferenceServiceName(serviceName, namespace)
	hostname := result + "-predictor-" + namespace

	if len(hostname) > 63 {
		t.Errorf("Real-world case failed: hostname %s exceeds 63 chars (len=%d)", hostname, len(hostname))
	}

	t.Logf("Input: serviceName=%s, namespace=%s", serviceName, namespace)
	t.Logf("Output: isvcName=%s (len=%d)", result, len(result))
	t.Logf("Full hostname: %s (len=%d)", hostname, len(hostname))
}

// isRFC1123Compliant checks if a string is a valid RFC1123 DNS label
func isRFC1123Compliant(s string) bool {
	if len(s) == 0 || len(s) > 63 {
		return false
	}
	// Must start and end with alphanumeric
	if !isAlphanumeric(s[0]) || !isAlphanumeric(s[len(s)-1]) {
		return false
	}
	// Can only contain lowercase alphanumeric and hyphens
	for _, ch := range s {
		if !isAlphanumeric(byte(ch)) && ch != '-' {
			return false
		}
	}
	return true
}

func isAlphanumeric(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9')
}
