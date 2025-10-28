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
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

const (
	testImageURI  = "docker.io/amd/llama-3-8b-instruct:latest"
	testNamespace = "default"
)

func TestResolveOrCreateModelFromImage_CreatesNamespaceModel(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := aimv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add scheme: %v", err)
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	runtimeConfig := &aimv1alpha1.AIMRuntimeConfigSpec{
		AIMRuntimeConfigCommon: aimv1alpha1.AIMRuntimeConfigCommon{
			Model: &aimv1alpha1.AIMModelConfig{
				CreationScope: "Namespace",
			},
		},
	}

	modelName, scope, err := ResolveOrCreateModelFromImage(context.Background(), k8sClient, testNamespace, testImageURI, runtimeConfig)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if scope != TemplateScopeNamespace {
		t.Fatalf("expected namespace scope, got %v", scope)
	}

	if modelName == "" {
		t.Fatal("expected non-empty model name")
	}

	// Verify the model was created
	var model aimv1alpha1.AIMModel
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Name: modelName, Namespace: testNamespace}, &model); err != nil {
		t.Fatalf("failed to get created model: %v", err)
	}

	if model.Spec.Image != testImageURI {
		t.Fatalf("expected image %q, got %q", testImageURI, model.Spec.Image)
	}

	if model.Labels[LabelAutoCreated] != "true" {
		t.Fatalf("expected auto-created label to be %q, got %q", "true", model.Labels[LabelAutoCreated])
	}
}

func TestResolveOrCreateModelFromImage_CreatesClusterModel(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := aimv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add scheme: %v", err)
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	runtimeConfig := &aimv1alpha1.AIMRuntimeConfigSpec{
		AIMRuntimeConfigCommon: aimv1alpha1.AIMRuntimeConfigCommon{
			Model: &aimv1alpha1.AIMModelConfig{
				CreationScope: "Cluster",
			},
		},
	}

	modelName, scope, err := ResolveOrCreateModelFromImage(context.Background(), k8sClient, testNamespace, testImageURI, runtimeConfig)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if scope != TemplateScopeCluster {
		t.Fatalf("expected cluster scope, got %v", scope)
	}

	if modelName == "" {
		t.Fatal("expected non-empty model name")
	}

	// Verify the cluster model was created
	var clusterModel aimv1alpha1.AIMClusterModel
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Name: modelName}, &clusterModel); err != nil {
		t.Fatalf("failed to get created cluster model: %v", err)
	}

	if clusterModel.Spec.Image != testImageURI {
		t.Fatalf("expected image %q, got %q", testImageURI, clusterModel.Spec.Image)
	}

	if clusterModel.Labels[LabelAutoCreated] != "true" {
		t.Fatalf("expected auto-created label to be %q, got %q", "true", clusterModel.Labels[LabelAutoCreated])
	}
}

func TestResolveOrCreateModelFromImage_FindsExistingModel(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := aimv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add scheme: %v", err)
	}

	existingModel := &aimv1alpha1.AIMModel{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "existing-model",
			Namespace: testNamespace,
		},
		Spec: aimv1alpha1.AIMModelSpec{
			Image: testImageURI,
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existingModel).
		Build()

	runtimeConfig := &aimv1alpha1.AIMRuntimeConfigSpec{
		AIMRuntimeConfigCommon: aimv1alpha1.AIMRuntimeConfigCommon{
			Model: &aimv1alpha1.AIMModelConfig{
				CreationScope: "Namespace",
			},
		},
	}

	modelName, scope, err := ResolveOrCreateModelFromImage(context.Background(), k8sClient, testNamespace, testImageURI, runtimeConfig)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if scope != TemplateScopeNamespace {
		t.Fatalf("expected namespace scope, got %v", scope)
	}

	if modelName != "existing-model" {
		t.Fatalf("expected model name %q, got %q", "existing-model", modelName)
	}
}

func TestResolveOrCreateModelFromImage_ErrorOnMultipleModels(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := aimv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add scheme: %v", err)
	}

	model1 := &aimv1alpha1.AIMModel{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "model-1",
			Namespace: testNamespace,
		},
		Spec: aimv1alpha1.AIMModelSpec{
			Image: testImageURI,
		},
	}

	model2 := &aimv1alpha1.AIMModel{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "model-2",
			Namespace: testNamespace,
		},
		Spec: aimv1alpha1.AIMModelSpec{
			Image: testImageURI,
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(model1, model2).
		Build()

	runtimeConfig := &aimv1alpha1.AIMRuntimeConfigSpec{
		AIMRuntimeConfigCommon: aimv1alpha1.AIMRuntimeConfigCommon{
			Model: &aimv1alpha1.AIMModelConfig{
				CreationScope: "Namespace",
			},
		},
	}

	_, _, err := ResolveOrCreateModelFromImage(context.Background(), k8sClient, testNamespace, testImageURI, runtimeConfig)
	if err == nil {
		t.Fatal("expected error when multiple models match the same image")
	}
}

func TestGenerateModelName(t *testing.T) {
	tests := []struct {
		name         string
		imageURI     string
		wantName     string // Expected exact name (deterministic)
		wantContains []string
	}{
		{
			name:         "simple image with tag",
			imageURI:     "docker.io/amd/llama-3-8b:latest",
			wantName:     "llama-3-8b-latest-789ec35b",
			wantContains: []string{"llama-3-8b", "latest"},
		},
		{
			name:         "image with version tag",
			imageURI:     "gcr.io/my-project/models/llama-3-70b-instruct:v1.2.3",
			wantName:     "llama-3-70b-instruct-v1-2-3-8924e87d",
			wantContains: []string{"llama-3-70b-instruct", "v1-2-3"},
		},
		{
			name:         "image with special characters in tag",
			imageURI:     "docker.io/org/Model_Name-v2:tag",
			wantName:     "model-name-v2-tag-80c44dff",
			wantContains: []string{"model-name-v2", "tag"},
		},
		{
			name:         "image without tag defaults to latest",
			imageURI:     "docker.io/amd/mistral",
			wantName:     "mistral-latest-859158af",
			wantContains: []string{"mistral", "latest"},
		},
		{
			name:         "image with digest",
			imageURI:     "ghcr.io/silogen/vllm@sha256:abcdef1234567890",
			wantName:     "vllm-abcdef-c6bc0374",
			wantContains: []string{"vllm", "abcdef"},
		},
		{
			name:         "very long image name gets truncated",
			imageURI:     "registry.example.com/very-long-organization-name/extremely-long-model-name-that-exceeds-limits:v2.0.1",
			wantName:     "extremely-long-model-name-that-exceeds-limits-v2-0-1-53334e16",
			wantContains: []string{"v2-0-1"}, // Tag should always be present
		},
		{
			name:         "ghcr.io example with semantic version",
			imageURI:     "ghcr.io/silogen/aim-meta-llama-llama-3-1-8b-instruct:0.7.0",
			wantName:     "aim-meta-llama-llama-3-1-8b-instruct-0-7-0-e2103ebd",
			wantContains: []string{"aim-meta-llama", "0-7-0"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := generateModelName(tt.imageURI)

			// Check exact expected name if provided
			if tt.wantName != "" {
				if got != tt.wantName {
					t.Errorf("generateModelName() = %q, want %q", got, tt.wantName)
				}
			}

			// Verify length constraint
			if len(got) > 63 {
				t.Errorf("generated name %q exceeds max length 63, got %d", got, len(got))
			}

			// Verify name is not empty
			if got == "" {
				t.Fatal("expected non-empty name")
			}

			// Verify required parts are present
			for _, part := range tt.wantContains {
				if !strings.Contains(got, part) {
					t.Errorf("generated name %q should contain %q", got, part)
				}
			}

			// Verify Kubernetes name compliance
			if strings.ToLower(got) != got {
				t.Errorf("name %q should be lowercase", got)
			}
			if strings.HasPrefix(got, "-") || strings.HasSuffix(got, "-") {
				t.Errorf("name %q should not start or end with hyphen", got)
			}
		})
	}
}

func TestGenerateModelName_Uniqueness(t *testing.T) {
	// Different image URIs should produce different names
	images := []string{
		"ghcr.io/silogen/llama-3-8b:v1.0.0",
		"ghcr.io/silogen/llama-3-8b:v1.0.1",
		"ghcr.io/silogen/llama-3-8b:v2.0.0",
		"ghcr.io/silogen/llama-3-8b:latest",
		"docker.io/amd/llama-3-8b:v1.0.0", // Different registry
	}

	names := make(map[string]string)
	for _, imageURI := range images {
		name := generateModelName(imageURI)
		if existingURI, found := names[name]; found {
			t.Errorf("collision detected: %q and %q both generate name %q", imageURI, existingURI, name)
		}
		names[name] = imageURI
	}

	// Should have unique name for each image
	if len(names) != len(images) {
		t.Errorf("expected %d unique names, got %d", len(images), len(names))
	}
}

func TestGenerateModelName_RegistryInHash(t *testing.T) {
	// Same image name and tag but different registries should produce different names
	testCases := []struct {
		name     string
		imageURI string
	}{
		{
			name:     "docker.io registry",
			imageURI: "docker.io/amd/llama-3-8b:v1.0.0",
		},
		{
			name:     "ghcr.io registry",
			imageURI: "ghcr.io/amd/llama-3-8b:v1.0.0",
		},
		{
			name:     "gcr.io registry",
			imageURI: "gcr.io/amd/llama-3-8b:v1.0.0",
		},
		{
			name:     "quay.io registry",
			imageURI: "quay.io/amd/llama-3-8b:v1.0.0",
		},
	}

	names := make(map[string]string)
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			name := generateModelName(tc.imageURI)

			// All should have same image name and tag components
			if !strings.Contains(name, "llama-3-8b") {
				t.Errorf("name %q should contain image name llama-3-8b", name)
			}
			if !strings.Contains(name, "v1-0-0") {
				t.Errorf("name %q should contain tag v1-0-0", name)
			}

			// But hashes should be different due to different registries
			if existingURI, found := names[name]; found {
				t.Errorf("COLLISION: %q and %q both generate name %q - registry should be in hash!",
					tc.imageURI, existingURI, name)
			}
			names[name] = tc.imageURI
		})
	}

	// Verify all produced unique names
	if len(names) != len(testCases) {
		t.Errorf("expected %d unique names (one per registry), got %d", len(testCases), len(names))
		for name, uri := range names {
			t.Logf("  %s -> %s", uri, name)
		}
	}
}

func TestGenerateModelName_HashStability(t *testing.T) {
	// Same image URI should always produce the same name
	imageURI := "ghcr.io/silogen/test-model:v1.2.3"

	name1 := generateModelName(imageURI)
	name2 := generateModelName(imageURI)
	name3 := generateModelName(imageURI)

	if name1 != name2 || name2 != name3 {
		t.Errorf("generateModelName() is not deterministic: %q, %q, %q", name1, name2, name3)
	}
}

func TestResolveModelNameFromService_Ref(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := aimv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add scheme: %v", err)
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	service := &aimv1alpha1.AIMService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-service",
			Namespace: testNamespace,
		},
		Spec: aimv1alpha1.AIMServiceSpec{
			Model: aimv1alpha1.AIMServiceModel{
				Ref: baseutils.Pointer("my-model"),
			},
		},
	}

	modelName, err := resolveModelNameFromService(context.Background(), k8sClient, service)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if modelName != "my-model" {
		t.Fatalf("expected model name %q, got %q", "my-model", modelName)
	}
}
