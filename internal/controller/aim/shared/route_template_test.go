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
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

func TestResolveServiceRoutePath_ServiceOverride(t *testing.T) {
	svc := newTestService()
	svc.Labels["aim.silogen.ai/workload-id"] = "Workload-42"
	svc.Spec.Routing = &aimv1alpha1.AIMServiceRouting{
		Enabled:      true,
		PathTemplate: "/{.metadata.namespace}/{.metadata.labels['aim.silogen.ai/workload-id']}/",
	}

	path, err := ResolveServiceRoutePath(svc, aimv1alpha1.AIMRuntimeConfigSpec{})
	if err != nil {
		t.Fatalf("ResolveServiceRoutePath failed: %v", err)
	}
	if want := "/testing/workload-42"; path != want {
		t.Fatalf("unexpected path: got %q want %q", path, want)
	}
}

func TestResolveServiceRoutePath_RuntimeConfigFallback(t *testing.T) {
	svc := newTestService()
	svc.Spec.Routing = &aimv1alpha1.AIMServiceRouting{Enabled: true}
	svc.Status.ResolvedImage = &aimv1alpha1.AIMResolvedReference{
		Name: "Meta/Llama-3-8B",
	}
	runtimeCfg := aimv1alpha1.AIMRuntimeConfigSpec{
		AIMRuntimeConfigCommon: aimv1alpha1.AIMRuntimeConfigCommon{
			Routing: &aimv1alpha1.AIMRuntimeRoutingConfig{
				PathTemplate: "/{.metadata.namespace}/{.status.resolvedImage.name}",
			},
		},
	}

	path, err := ResolveServiceRoutePath(svc, runtimeCfg)
	if err != nil {
		t.Fatalf("ResolveServiceRoutePath failed: %v", err)
	}
	if want := "/testing/meta-llama-3-8b"; path != want {
		t.Fatalf("unexpected path: got %q want %q", path, want)
	}
}

func TestResolveServiceRoutePath_Annotation(t *testing.T) {
	svc := newTestService()
	svc.Annotations = map[string]string{
		"route.suffix": "Team-A/B",
	}
	svc.Spec.Routing = &aimv1alpha1.AIMServiceRouting{
		Enabled:      true,
		PathTemplate: "/{.metadata.annotations['route.suffix']}",
	}

	path, err := ResolveServiceRoutePath(svc, aimv1alpha1.AIMRuntimeConfigSpec{})
	if err != nil {
		t.Fatalf("ResolveServiceRoutePath failed: %v", err)
	}
	if want := "/team-a/b"; path != want {
		t.Fatalf("unexpected path: got %q want %q", path, want)
	}
}

func TestResolveServiceRoutePath_DefaultFallback(t *testing.T) {
	svc := newTestService()
	svc.Spec.Routing = nil

	path, err := ResolveServiceRoutePath(svc, aimv1alpha1.AIMRuntimeConfigSpec{})
	if err != nil {
		t.Fatalf("ResolveServiceRoutePath failed: %v", err)
	}
	if want := "/testing/12345678-90ab"; path != want {
		t.Fatalf("unexpected path: got %q want %q", path, want)
	}
}

func TestResolveServiceRoutePath_MissingLabel(t *testing.T) {
	svc := newTestService()
	svc.Spec.Routing = &aimv1alpha1.AIMServiceRouting{
		Enabled:      true,
		PathTemplate: "/{.metadata.labels['missing']}",
	}

	_, err := ResolveServiceRoutePath(svc, aimv1alpha1.AIMRuntimeConfigSpec{})
	if err == nil {
		t.Fatalf("expected error when label is missing")
	}
	if !strings.Contains(err.Error(), "label \"missing\" not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveServiceRoutePath_PathTooLong(t *testing.T) {
	svc := newTestService()
	segment := strings.Repeat("abcdefghijklmnopqrstuvwxyz", 4) // 104 runes per segment after RFC1123 trimming to 63
	svc.Labels["segment-a"] = segment
	svc.Labels["segment-b"] = segment
	svc.Labels["segment-c"] = segment
	svc.Labels["segment-d"] = segment
	svc.Spec.Routing = &aimv1alpha1.AIMServiceRouting{
		Enabled:      true,
		PathTemplate: "/{.metadata.labels['segment-a']}/{.metadata.labels['segment-b']}/{.metadata.labels['segment-c']}/{.metadata.labels['segment-d']}",
	}

	_, err := ResolveServiceRoutePath(svc, aimv1alpha1.AIMRuntimeConfigSpec{})
	if err == nil {
		t.Fatalf("expected error for oversized path")
	}
	if !strings.Contains(err.Error(), "exceeds 200 characters") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveServiceRoutePath_InvalidExpression(t *testing.T) {
	svc := newTestService()
	svc.Spec.Routing = &aimv1alpha1.AIMServiceRouting{
		Enabled:      true,
		PathTemplate: "/{.metadata[}",
	}

	_, err := ResolveServiceRoutePath(svc, aimv1alpha1.AIMRuntimeConfigSpec{})
	if err == nil {
		t.Fatalf("expected error for invalid jsonpath")
	}
	if !strings.Contains(err.Error(), "invalid jsonpath expression") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func newTestService() *aimv1alpha1.AIMService {
	return &aimv1alpha1.AIMService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo",
			Namespace: "Testing",
			UID:       types.UID("12345678-90ab"),
			Labels: map[string]string{
				"team": "platform",
			},
		},
		Spec: aimv1alpha1.AIMServiceSpec{
			Model:       aimv1alpha1.AIMServiceModel{Ref: baseutils.Pointer("Meta/Llama-3-8B")},
			TemplateRef: "demo-template",
		},
	}
}
