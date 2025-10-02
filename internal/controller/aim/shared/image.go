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

	"sigs.k8s.io/controller-runtime/pkg/client"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
)

// LookupImageForClusterTemplate looks up the container image for a cluster-scoped template.
// It searches only in AIMClusterImage resources.
func LookupImageForClusterTemplate(ctx context.Context, k8sClient client.Client, modelID string) (string, error) {
	var imageList aimv1alpha1.AIMClusterImageList
	if err := k8sClient.List(ctx, &imageList); err != nil {
		return "", fmt.Errorf("failed to list AIMClusterImages: %w", err)
	}

	for _, img := range imageList.Items {
		if img.Spec.ModelID == modelID {
			return img.Spec.Image, nil
		}
	}

	return "", nil
}

// LookupImageForNamespaceTemplate looks up the container image for a namespace-scoped template.
// It searches AIMImage resources in the specified namespace first, then falls back to
// cluster-scoped AIMClusterImage resources.
func LookupImageForNamespaceTemplate(ctx context.Context, k8sClient client.Client, namespace, modelID string) (string, error) {
	// Try namespace-scoped AIMImage first
	var nsImageList aimv1alpha1.AIMImageList
	if err := k8sClient.List(ctx, &nsImageList, client.InNamespace(namespace)); err != nil {
		return "", fmt.Errorf("failed to list AIMImages: %w", err)
	}

	for _, img := range nsImageList.Items {
		if img.Spec.ModelID == modelID {
			return img.Spec.Image, nil
		}
	}

	// Fall back to cluster-scoped AIMClusterImage if not found
	var clusterImageList aimv1alpha1.AIMClusterImageList
	if err := k8sClient.List(ctx, &clusterImageList); err != nil {
		return "", fmt.Errorf("failed to list AIMClusterImages: %w", err)
	}

	for _, img := range clusterImageList.Items {
		if img.Spec.ModelID == modelID {
			return img.Spec.Image, nil
		}
	}

	return "", nil
}
