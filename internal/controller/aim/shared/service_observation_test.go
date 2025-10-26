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
	"context"
	"testing"

	"k8s.io/utils/ptr"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

func TestFindMatchingTemplateForDerivedSpecNamespaceMatch(t *testing.T) {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := aimv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add scheme: %v", err)
	}

	service := &aimv1alpha1.AIMService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example",
			Namespace: "default",
		},
		Spec: aimv1alpha1.AIMServiceSpec{
			Model: aimv1alpha1.AIMServiceModel{Ref: baseutils.Pointer("example-image")},
			Overrides: &aimv1alpha1.AIMServiceOverrides{
				AIMRuntimeParameters: aimv1alpha1.AIMRuntimeParameters{
					Metric:    ptr.To(aimv1alpha1.AIMMetricThroughput),
					Precision: ptr.To(aimv1alpha1.AIMPrecisionFP16),
				},
			},
		},
	}

	baseSpec := &aimv1alpha1.AIMServiceTemplateSpec{
		AIMServiceTemplateSpecCommon: aimv1alpha1.AIMServiceTemplateSpecCommon{
			AIMModelName:      "example-image",
			RuntimeConfigName: "default",
			AIMRuntimeParameters: aimv1alpha1.AIMRuntimeParameters{
				GpuSelector: &aimv1alpha1.AIMGpuSelector{
					Model: "mi300",
					Count: 1,
				},
			},
		},
	}

	existing := BuildDerivedTemplate(service, "existing-derived", "example-image", baseSpec)

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existing).
		Build()

	match, err := findMatchingTemplateForDerivedSpec(context.Background(), k8sClient, service, "example-image", baseSpec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if match == nil || match.NamespaceTemplate == nil {
		t.Fatalf("expected namespace template match, got %+v", match)
	}
	if match.NamespaceTemplate.Name != existing.Name {
		t.Fatalf("expected template name %q, got %q", existing.Name, match.NamespaceTemplate.Name)
	}
}

func TestFindMatchingTemplateForDerivedSpecClusterMatch(t *testing.T) {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := aimv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add scheme: %v", err)
	}

	service := &aimv1alpha1.AIMService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-service",
			Namespace: "default",
		},
		Spec: aimv1alpha1.AIMServiceSpec{
			Model: aimv1alpha1.AIMServiceModel{Ref: baseutils.Pointer("cluster-image")},
			Overrides: &aimv1alpha1.AIMServiceOverrides{
				AIMRuntimeParameters: aimv1alpha1.AIMRuntimeParameters{
					Metric: ptr.To(aimv1alpha1.AIMMetricLatency),
				},
			},
		},
	}

	baseSpec := &aimv1alpha1.AIMServiceTemplateSpec{
		AIMServiceTemplateSpecCommon: aimv1alpha1.AIMServiceTemplateSpecCommon{
			AIMModelName:      "cluster-image",
			RuntimeConfigName: "default",
			AIMRuntimeParameters: aimv1alpha1.AIMRuntimeParameters{
				Metric: ptr.To(aimv1alpha1.AIMMetricLatency),
			},
		},
	}

	clusterTemplate := &aimv1alpha1.AIMClusterServiceTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster-existing",
		},
		Spec: aimv1alpha1.AIMClusterServiceTemplateSpec{
			AIMServiceTemplateSpecCommon: baseSpec.AIMServiceTemplateSpecCommon,
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(clusterTemplate).
		Build()

	match, err := findMatchingTemplateForDerivedSpec(context.Background(), k8sClient, service, "cluster-image", baseSpec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if match == nil || match.ClusterTemplate == nil {
		t.Fatalf("expected cluster template match, got %+v", match)
	}
	if match.ClusterTemplate.Name != clusterTemplate.Name {
		t.Fatalf("expected cluster template %q, got %q", clusterTemplate.Name, match.ClusterTemplate.Name)
	}
}

func TestFindMatchingTemplateForDerivedSpecNoMatch(t *testing.T) {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := aimv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add scheme: %v", err)
	}

	service := &aimv1alpha1.AIMService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "no-match",
			Namespace: "default",
		},
		Spec: aimv1alpha1.AIMServiceSpec{
			Model: aimv1alpha1.AIMServiceModel{Ref: baseutils.Pointer("original-image")},
			Overrides: &aimv1alpha1.AIMServiceOverrides{
				AIMRuntimeParameters: aimv1alpha1.AIMRuntimeParameters{
					Precision: ptr.To(aimv1alpha1.AIMPrecisionBF16),
				},
			},
		},
	}

	baseSpec := &aimv1alpha1.AIMServiceTemplateSpec{
		AIMServiceTemplateSpecCommon: aimv1alpha1.AIMServiceTemplateSpecCommon{
			AIMModelName:      "original-image",
			RuntimeConfigName: "default",
		},
	}

	otherTemplate := &aimv1alpha1.AIMServiceTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "different-spec",
			Namespace: "default",
		},
		Spec: aimv1alpha1.AIMServiceTemplateSpec{
			AIMServiceTemplateSpecCommon: aimv1alpha1.AIMServiceTemplateSpecCommon{
				AIMModelName:      "original-image",
				RuntimeConfigName: "default",
			},
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(otherTemplate).
		Build()

	match, err := findMatchingTemplateForDerivedSpec(context.Background(), k8sClient, service, "original-image", baseSpec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if match != nil {
		t.Fatalf("expected no match, got %+v", match)
	}
}
