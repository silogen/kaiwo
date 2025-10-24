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

	if gpu := resources.Requests[amdGPUResourceName]; gpu.Cmp(resource.MustParse("2")) != 0 {
		t.Fatalf("expected GPU request 2, got %s", gpu.String())
	}

	if gpuLimit := resources.Limits[amdGPUResourceName]; gpuLimit.Cmp(resource.MustParse("2")) != 0 {
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

	if gpu := resources.Requests[amdGPUResourceName]; gpu.Cmp(resource.MustParse("2")) != 0 {
		t.Fatalf("expected GPU request 2, got %s", gpu.String())
	}

	if gpuLimit := resources.Limits[amdGPUResourceName]; gpuLimit.Cmp(resource.MustParse("2")) != 0 {
		t.Fatalf("expected GPU limit 2, got %s", gpuLimit.String())
	}
}
