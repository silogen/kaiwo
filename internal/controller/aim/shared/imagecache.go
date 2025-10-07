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
	"fmt"
	"sort"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
)

const (
	// BaseCacherDaemonSetName is the name of the DaemonSet that caches base images
	BaseCacherDaemonSetName = "aim-base-cacher"
)

// ImageCacheObservation holds observed state for image caching reconciliation
type ImageCacheObservation struct {
	Config            *aimv1alpha1.AIMClusterConfig
	ExistingDaemonSet *appsv1.DaemonSet
	BaseImages        []string
}

// ExtractBaseImage extracts the base image name from a full AIM image reference.
// Example: ghcr.io/silogen/aim:0.4.0-meta-llama-llama-3.1-8b-instruct-v20251006
//
//	-> ghcr.io/silogen/aim-base:0.4.0
func ExtractBaseImage(image string) (string, error) {
	// Split on last colon to separate registry/repo from tag
	lastColon := strings.LastIndex(image, ":")
	if lastColon == -1 {
		return "", fmt.Errorf("image %q has no tag", image)
	}

	registryAndRepo := image[:lastColon]
	tag := image[lastColon+1:]

	// Extract version from tag (everything before first hyphen)
	hyphenIndex := strings.Index(tag, "-")
	if hyphenIndex == -1 {
		// No hyphen means this might already be a base image or just a version
		return "", fmt.Errorf("image %q tag has no version-model separator", image)
	}

	version := tag[:hyphenIndex]

	// Replace "aim" with "aim-base" in the repo name
	if !strings.HasSuffix(registryAndRepo, "/aim") {
		return "", fmt.Errorf("image %q does not end with /aim", registryAndRepo)
	}

	baseRepo := strings.TrimSuffix(registryAndRepo, "/aim") + "/aim-base"

	return fmt.Sprintf("%s:%s", baseRepo, version), nil
}

// ObserveImageCaching gathers all information needed for image caching decisions
func ObserveImageCaching(ctx context.Context, k8sClient client.Client) (*ImageCacheObservation, error) {
	obs := &ImageCacheObservation{}

	// Get the default AIMClusterConfig
	config, err := GetDefaultClusterConfig(ctx, k8sClient)
	if err != nil {
		return nil, fmt.Errorf("failed to get default AIMClusterConfig: %w", err)
	}
	obs.Config = config

	// Get existing DaemonSet if it exists
	var ds appsv1.DaemonSet
	dsKey := client.ObjectKey{
		Name:      BaseCacherDaemonSetName,
		Namespace: OperatorNamespace,
	}
	if err := k8sClient.Get(ctx, dsKey, &ds); err != nil {
		if !errors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to get DaemonSet: %w", err)
		}
		// DaemonSet doesn't exist yet, that's fine
	} else {
		obs.ExistingDaemonSet = &ds
	}

	// List all AIMImage resources (all namespaces)
	var aimImages aimv1alpha1.AIMImageList
	if err := k8sClient.List(ctx, &aimImages); err != nil {
		return nil, fmt.Errorf("failed to list AIMImage resources: %w", err)
	}

	// List all AIMClusterImage resources
	var aimClusterImages aimv1alpha1.AIMClusterImageList
	if err := k8sClient.List(ctx, &aimClusterImages); err != nil {
		return nil, fmt.Errorf("failed to list AIMClusterImage resources: %w", err)
	}

	// Extract base images from all resources
	baseImageSet := make(map[string]bool)

	for _, img := range aimImages.Items {
		if img.Spec.Image != "" {
			baseImage, err := ExtractBaseImage(img.Spec.Image)
			if err != nil {
				// Log but don't fail - skip invalid images
				continue
			}
			baseImageSet[baseImage] = true
		}
	}

	for _, img := range aimClusterImages.Items {
		if img.Spec.Image != "" {
			baseImage, err := ExtractBaseImage(img.Spec.Image)
			if err != nil {
				// Log but don't fail - skip invalid images
				continue
			}
			baseImageSet[baseImage] = true
		}
	}

	// Convert to sorted slice for deterministic ordering
	baseImages := make([]string, 0, len(baseImageSet))
	for img := range baseImageSet {
		baseImages = append(baseImages, img)
	}
	sort.Strings(baseImages)
	obs.BaseImages = baseImages

	return obs, nil
}

// PlanImageCaching determines what resources should exist based on observation
func PlanImageCaching(obs *ImageCacheObservation) ([]client.Object, error) {
	var desired []client.Object

	// If no config or caching is disabled, return empty (no DaemonSet)
	if obs == nil || obs.Config == nil || !obs.Config.Spec.Caching.CacheAimImageBase {
		return desired, nil
	}

	// If there are no base images to cache, return empty
	if len(obs.BaseImages) == 0 {
		return desired, nil
	}

	// Build DaemonSet with all base images
	ds := BuildBaseCacherDaemonSet(obs.BaseImages, obs.Config.Spec.ImagePullSecrets)
	desired = append(desired, ds)

	return desired, nil
}

// BuildBaseCacherDaemonSet constructs a DaemonSet that caches base images on all nodes
func BuildBaseCacherDaemonSet(baseImages []string, imagePullSecrets []corev1.LocalObjectReference) *appsv1.DaemonSet {
	// Build containers - one per base image
	containers := make([]corev1.Container, 0, len(baseImages))
	for i, image := range baseImages {
		// Create container name from image, replacing invalid characters
		containerName := fmt.Sprintf("cache-%d", i)

		containers = append(containers, corev1.Container{
			Name:    containerName,
			Image:   image,
			Command: []string{"/bin/sh", "-c", "sleep infinity"},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("10m"),
					corev1.ResourceMemory: resource.MustParse("32Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("64Mi"),
				},
			},
		})
	}

	return &appsv1.DaemonSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "DaemonSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      BaseCacherDaemonSetName,
			Namespace: OperatorNamespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":      "aim-base-cacher",
				"app.kubernetes.io/component": "image-cacher",
				"app.kubernetes.io/part-of":   "kaiwo",
			},
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/name": "aim-base-cacher",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/name":      "aim-base-cacher",
						"app.kubernetes.io/component": "image-cacher",
						"app.kubernetes.io/part-of":   "kaiwo",
					},
				},
				Spec: corev1.PodSpec{
					Containers:       containers,
					ImagePullSecrets: imagePullSecrets,
				},
			},
		},
	}
}
