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
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
	"github.com/silogen/kaiwo/internal/controller/aim/shared"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

// buildClusterModel creates an AIMClusterModel resource from a discovered registry image.
// The model name is generated deterministically using the repository and tag.
// Labels and annotations track the source and original image details.
func BuildClusterModel(
	source *aimv1alpha1.AIMClusterModelSource,
	img RegistryImage,
) *aimv1alpha1.AIMClusterModel {
	// Generate deterministic name using existing naming utility
	// Format: {repository}-{tag}-{hash} (hash ensures uniqueness and handles length limits)
	modelName, _ := baseutils.GenerateDerivedNameWithHashLength(
		[]string{img.Repository, img.Tag},
		6,                                     // 6-char hash for collision avoidance
		img.Registry, img.Repository, img.Tag, // Hash inputs for determinism
	)

	imageURI := img.ToImageURI()

	return &aimv1alpha1.AIMClusterModel{
		ObjectMeta: metav1.ObjectMeta{
			Name: modelName,
			Labels: map[string]string{
				shared.LabelKeyModelSource: source.Name,
				shared.LabelAutoCreated:    "true",
			},
			Annotations: map[string]string{
				"aim.eai.amd.com/source-registry":   img.Registry,
				"aim.eai.amd.com/source-repository": img.Repository,
				"aim.eai.amd.com/source-tag":        img.Tag,
			},
		},
		Spec: aimv1alpha1.AIMModelSpec{
			Image:            imageURI,
			ImagePullSecrets: source.Spec.ImagePullSecrets,
		},
	}
}

// RegistryImage represents a container image discovered in a registry.
type RegistryImage struct {
	Registry   string
	Repository string
	Tag        string
	Digest     string
}

// ToImageURI converts a RegistryImage to a full image URI.
// Special handling for docker.io which doesn't require the registry prefix.
func (ri RegistryImage) ToImageURI() string {
	// Docker Hub special case - no registry prefix
	if ri.Registry == DockerRegistry || ri.Registry == "" {
		return fmt.Sprintf("%s:%s", ri.Repository, ri.Tag)
	}
	return fmt.Sprintf("%s/%s:%s", ri.Registry, ri.Repository, ri.Tag)
}

// ExtractStaticImages converts filters that are exact image references (no wildcards, with tag)
// into RegistryImage objects. This allows bypassing registry queries for static image lists.
// When a filter has no explicit tag but spec.versions are specified, it generates static images
// for each version. This is especially useful for registries like ghcr.io that don't support catalog API.
// Returns only the filters that can be converted to static references.
func ExtractStaticImages(spec aimv1alpha1.AIMClusterModelSourceSpec) []RegistryImage {
	var images []RegistryImage

	for _, filter := range spec.Filters {
		// Parse the filter to check if it's a static reference
		parsed := parseImageFilter(filter.Image)

		// Skip filters with wildcards - these require registry queries
		if parsed.hasWildcard {
			continue
		}

		// Determine which registry to use
		registry := parsed.registry
		if registry == "" {
			registry = spec.Registry
			if registry == "" {
				registry = DockerRegistry
			}
		}

		// Determine which versions/tags to use
		var tags []string
		if parsed.tag != "" {
			// Filter has explicit tag - use it
			tags = []string{parsed.tag}
		} else if len(filter.Versions) > 0 {
			// Filter has per-filter versions - only use exact versions, not constraints
			for _, v := range filter.Versions {
				if !isVersionConstraint(v) {
					tags = append(tags, v)
				}
			}
		} else if len(spec.Versions) > 0 {
			// Use global versions from spec - only exact versions, not constraints
			for _, v := range spec.Versions {
				if !isVersionConstraint(v) {
					tags = append(tags, v)
				}
			}
		}

		// If no exact versions found, skip this filter (let it be handled by FetchImagesUsingTagsList)
		if len(tags) == 0 {
			continue
		}

		// Generate a RegistryImage for each tag
		for _, tag := range tags {
			images = append(images, RegistryImage{
				Registry:   registry,
				Repository: parsed.repository,
				Tag:        tag,
			})
		}
	}

	return images
}
