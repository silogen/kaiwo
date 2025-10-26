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
		name     string
		imageURI string
		wantLen  int
	}{
		{
			name:     "simple image name",
			imageURI: "docker.io/amd/llama-3-8b:latest",
			wantLen:  63, // max Kubernetes name length
		},
		{
			name:     "complex image name with registry",
			imageURI: "gcr.io/my-project/models/llama-3-70b-instruct:v1.2.3",
			wantLen:  63,
		},
		{
			name:     "image with special characters",
			imageURI: "docker.io/org/Model_Name-v2:tag",
			wantLen:  63,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name := generateModelName(tt.imageURI)
			if len(name) > tt.wantLen {
				t.Fatalf("generated name %q exceeds max length %d", name, tt.wantLen)
			}
			if name == "" {
				t.Fatal("expected non-empty name")
			}
			// Verify it ends with -<8-char-hash>
			if len(name) < 9 || name[len(name)-9] != '-' {
				t.Fatalf("expected name to end with -<8-char-hash>, got %q", name)
			}
		})
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
