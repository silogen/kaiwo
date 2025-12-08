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

package aimclustermodelsource

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
	"github.com/silogen/kaiwo/internal/controller/aim/shared"
)

func TestRegistryImageToImageURI(t *testing.T) {
	tests := []struct {
		name string
		img  RegistryImage
		want string
	}{
		{
			name: "docker.io image",
			img: RegistryImage{
				Registry:   "docker.io",
				Repository: "amdenterpriseai/aim-llama3",
				Tag:        "1.0.0",
			},
			want: "amdenterpriseai/aim-llama3:1.0.0",
		},
		{
			name: "empty registry defaults to docker.io",
			img: RegistryImage{
				Registry:   "",
				Repository: "amdenterpriseai/aim-llama3",
				Tag:        "1.0.0",
			},
			want: "amdenterpriseai/aim-llama3:1.0.0",
		},
		{
			name: "ghcr.io image",
			img: RegistryImage{
				Registry:   "ghcr.io",
				Repository: "org/model",
				Tag:        "v2.1.0",
			},
			want: "ghcr.io/org/model:v2.1.0",
		},
		{
			name: "gcr.io image",
			img: RegistryImage{
				Registry:   "gcr.io",
				Repository: "project/image",
				Tag:        "latest",
			},
			want: "gcr.io/project/image:latest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.img.ToImageURI()
			if got != tt.want {
				t.Errorf("ToImageURI() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildClusterModel(t *testing.T) {
	source := &aimv1alpha1.AIMClusterModelSource{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-source",
		},
		Spec: aimv1alpha1.AIMClusterModelSourceSpec{
			ImagePullSecrets: []corev1.LocalObjectReference{
				{Name: "dockerhub-creds"},
			},
		},
	}

	img := RegistryImage{
		Registry:   "docker.io",
		Repository: "amdenterpriseai/aim-llama3",
		Tag:        "1.0.0",
	}

	model := BuildClusterModel(source, img)

	// Check basic fields
	if model.Name == "" {
		t.Error("Model name should not be empty")
	}

	// Name should be deterministic and contain repository/tag info
	if len(model.Name) > 63 {
		t.Errorf("Model name too long: %d chars (max 63)", len(model.Name))
	}

	// Check labels
	if model.Labels[shared.LabelKeyModelSource] != "test-source" {
		t.Errorf("Model source label = %v, want %v",
			model.Labels[shared.LabelKeyModelSource], "test-source")
	}

	if model.Labels[shared.LabelAutoCreated] != "true" {
		t.Errorf("Auto-created label = %v, want true",
			model.Labels[shared.LabelAutoCreated])
	}

	// Check annotations
	if model.Annotations["aim.eai.amd.com/source-registry"] != DockerRegistry {
		t.Errorf("Registry annotation = %v, want %s",
			model.Annotations["aim.eai.amd.com/source-registry"], DockerRegistry)
	}

	if model.Annotations["aim.eai.amd.com/source-repository"] != "amdenterpriseai/aim-llama3" {
		t.Errorf("Repository annotation = %v, want amdenterpriseai/aim-llama3",
			model.Annotations["aim.eai.amd.com/source-repository"])
	}

	if model.Annotations["aim.eai.amd.com/source-tag"] != "1.0.0" {
		t.Errorf("Tag annotation = %v, want 1.0.0",
			model.Annotations["aim.eai.amd.com/source-tag"])
	}

	// Check spec
	expectedImage := "amdenterpriseai/aim-llama3:1.0.0"
	if model.Spec.Image != expectedImage {
		t.Errorf("Model image = %v, want %v", model.Spec.Image, expectedImage)
	}

	if len(model.Spec.ImagePullSecrets) != 1 {
		t.Errorf("ImagePullSecrets count = %d, want 1", len(model.Spec.ImagePullSecrets))
	}

	if len(model.Spec.ImagePullSecrets) > 0 && model.Spec.ImagePullSecrets[0].Name != "dockerhub-creds" {
		t.Errorf("ImagePullSecret name = %v, want dockerhub-creds",
			model.Spec.ImagePullSecrets[0].Name)
	}
}

func TestBuildClusterModelDeterminism(t *testing.T) {
	// Build the same model twice and verify names are identical
	source := &aimv1alpha1.AIMClusterModelSource{
		ObjectMeta: metav1.ObjectMeta{Name: "test-source"},
	}

	img := RegistryImage{
		Registry:   "docker.io",
		Repository: "amdenterpriseai/aim-llama3",
		Tag:        "1.0.0",
	}

	model1 := BuildClusterModel(source, img)
	model2 := BuildClusterModel(source, img)

	if model1.Name != model2.Name {
		t.Errorf("Model names not deterministic: %v != %v", model1.Name, model2.Name)
	}
}

func TestBuildClusterModelUniqueness(t *testing.T) {
	// Different images should produce different model names
	source := &aimv1alpha1.AIMClusterModelSource{
		ObjectMeta: metav1.ObjectMeta{Name: "test-source"},
	}

	img1 := RegistryImage{
		Registry:   "docker.io",
		Repository: "amdenterpriseai/aim-llama3",
		Tag:        "1.0.0",
	}

	img2 := RegistryImage{
		Registry:   "docker.io",
		Repository: "amdenterpriseai/aim-llama3",
		Tag:        "2.0.0",
	}

	model1 := BuildClusterModel(source, img1)
	model2 := BuildClusterModel(source, img2)

	if model1.Name == model2.Name {
		t.Errorf("Different images produced same model name: %v", model1.Name)
	}
}

func TestExtractStaticImages(t *testing.T) {
	tests := []struct {
		name string
		spec aimv1alpha1.AIMClusterModelSourceSpec
		want []RegistryImage
	}{
		{
			name: "single exact image reference",
			spec: aimv1alpha1.AIMClusterModelSourceSpec{
				Filters: []aimv1alpha1.ModelSourceFilter{
					{Image: "ghcr.io/silogen/aim-google-gemma-3-1b-it:0.8.1-rc1"},
				},
			},
			want: []RegistryImage{
				{
					Registry:   "ghcr.io",
					Repository: "silogen/aim-google-gemma-3-1b-it",
					Tag:        "0.8.1-rc1",
				},
			},
		},
		{
			name: "docker.io image with tag",
			spec: aimv1alpha1.AIMClusterModelSourceSpec{
				Filters: []aimv1alpha1.ModelSourceFilter{
					{Image: "silogen/aim-llama:1.0.0"},
				},
			},
			want: []RegistryImage{
				{
					Registry:   "docker.io",
					Repository: "silogen/aim-llama",
					Tag:        "1.0.0",
				},
			},
		},
		{
			name: "multiple exact references",
			spec: aimv1alpha1.AIMClusterModelSourceSpec{
				Filters: []aimv1alpha1.ModelSourceFilter{
					{Image: "ghcr.io/org/model1:v1"},
					{Image: "gcr.io/org/model2:v2"},
				},
			},
			want: []RegistryImage{
				{
					Registry:   "ghcr.io",
					Repository: "org/model1",
					Tag:        "v1",
				},
				{
					Registry:   "gcr.io",
					Repository: "org/model2",
					Tag:        "v2",
				},
			},
		},
		{
			name: "wildcard filter skipped",
			spec: aimv1alpha1.AIMClusterModelSourceSpec{
				Filters: []aimv1alpha1.ModelSourceFilter{
					{Image: "ghcr.io/silogen/aim-*"},
				},
			},
			want: nil,
		},
		{
			name: "filter without tag or versions skipped",
			spec: aimv1alpha1.AIMClusterModelSourceSpec{
				Filters: []aimv1alpha1.ModelSourceFilter{
					{Image: "ghcr.io/silogen/aim-llama"},
				},
			},
			want: nil,
		},
		{
			name: "filter without tag but with global versions",
			spec: aimv1alpha1.AIMClusterModelSourceSpec{
				Registry: "ghcr.io",
				Filters: []aimv1alpha1.ModelSourceFilter{
					{Image: "silogen/aim-openai-gpt-oss-20b"},
				},
				Versions: []string{"0.9.0-rc2", "0.9.0-rc3"},
			},
			want: []RegistryImage{
				{
					Registry:   "ghcr.io",
					Repository: "silogen/aim-openai-gpt-oss-20b",
					Tag:        "0.9.0-rc2",
				},
				{
					Registry:   "ghcr.io",
					Repository: "silogen/aim-openai-gpt-oss-20b",
					Tag:        "0.9.0-rc3",
				},
			},
		},
		{
			name: "mixed static and dynamic filters",
			spec: aimv1alpha1.AIMClusterModelSourceSpec{
				Filters: []aimv1alpha1.ModelSourceFilter{
					{Image: "ghcr.io/org/model:v1"},    // static - included
					{Image: "ghcr.io/org/aim-*"},       // wildcard - skipped
					{Image: "docker.io/org/model2:v2"}, // static - included
					{Image: "docker.io/org/model3"},    // no tag, no versions - skipped
				},
			},
			want: []RegistryImage{
				{
					Registry:   "ghcr.io",
					Repository: "org/model",
					Tag:        "v1",
				},
				{
					Registry:   "docker.io",
					Repository: "org/model2",
					Tag:        "v2",
				},
			},
		},
		{
			name: "empty filters",
			spec: aimv1alpha1.AIMClusterModelSourceSpec{
				Filters: []aimv1alpha1.ModelSourceFilter{},
			},
			want: nil,
		},
		{
			name: "version constraint in global versions - skipped",
			spec: aimv1alpha1.AIMClusterModelSourceSpec{
				Registry: "ghcr.io",
				Filters: []aimv1alpha1.ModelSourceFilter{
					{Image: "silogen/aim-openai-gpt-oss-20b"},
				},
				Versions: []string{">=0.9.0"},
			},
			want: nil, // Version constraint should be skipped, not treated as a tag
		},
		{
			name: "mixed exact versions and constraints in global versions",
			spec: aimv1alpha1.AIMClusterModelSourceSpec{
				Registry: "ghcr.io",
				Filters: []aimv1alpha1.ModelSourceFilter{
					{Image: "silogen/aim-openai-gpt-oss-20b"},
				},
				Versions: []string{"0.9.0-rc2", ">=0.9.0", "0.9.0-rc3", "<1.0.0"},
			},
			want: []RegistryImage{
				{
					Registry:   "ghcr.io",
					Repository: "silogen/aim-openai-gpt-oss-20b",
					Tag:        "0.9.0-rc2",
				},
				{
					Registry:   "ghcr.io",
					Repository: "silogen/aim-openai-gpt-oss-20b",
					Tag:        "0.9.0-rc3",
				},
			},
		},
		{
			name: "version constraint in filter versions - skipped",
			spec: aimv1alpha1.AIMClusterModelSourceSpec{
				Registry: "ghcr.io",
				Filters: []aimv1alpha1.ModelSourceFilter{
					{
						Image:    "silogen/aim-openai-gpt-oss-20b",
						Versions: []string{"~1.0.0"},
					},
				},
			},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractStaticImages(tt.spec)
			if len(got) != len(tt.want) {
				t.Errorf("extractStaticImages() returned %d images, want %d", len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i].Registry != tt.want[i].Registry {
					t.Errorf("image[%d].Registry = %q, want %q", i, got[i].Registry, tt.want[i].Registry)
				}
				if got[i].Repository != tt.want[i].Repository {
					t.Errorf("image[%d].Repository = %q, want %q", i, got[i].Repository, tt.want[i].Repository)
				}
				if got[i].Tag != tt.want[i].Tag {
					t.Errorf("image[%d].Tag = %q, want %q", i, got[i].Tag, tt.want[i].Tag)
				}
			}
		})
	}
}
