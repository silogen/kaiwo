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

	"k8s.io/apimachinery/pkg/api/errors"

	"sigs.k8s.io/controller-runtime/pkg/client"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
)

// LookupImageForClusterTemplate looks up the container image for a cluster-scoped template.
// It searches only in AIMClusterImage resources.
func LookupImageForClusterTemplate(ctx context.Context, k8sClient client.Client, modelName string) (string, error) {
	clusterImage := &aimv1alpha1.AIMClusterImage{}

	if err := k8sClient.Get(ctx, client.ObjectKey{Name: modelName}, clusterImage); err == nil {
		return clusterImage.Spec.Image, nil
	} else if !errors.IsNotFound(err) {
		return "", fmt.Errorf("failed to lookup AIMClusterImage: %w", err)
	}

	// TODO Handle propagating the error that no image was found, so that it can be distinguished from other errors, and ends up in the condition

	return "", nil
}

// LookupImageForNamespaceTemplate looks up the container image for a namespace-scoped template.
// It searches AIMImage resources in the specified namespace first, then falls back to
// cluster-scoped AIMClusterImage resources.
func LookupImageForNamespaceTemplate(ctx context.Context, k8sClient client.Client, namespace, modelName string) (string, error) {
	// Try namespace-scoped AIMImage first
	nsImage := &aimv1alpha1.AIMImage{}

	if err := k8sClient.Get(ctx, client.ObjectKey{Name: modelName, Namespace: namespace}, nsImage); err == nil {
		return nsImage.Spec.Image, nil
	} else if !errors.IsNotFound(err) {
		return "", fmt.Errorf("failed to lookup AIMImage: %w", err)
	}

	// Fall back to cluster-scoped namespace
	return LookupImageForClusterTemplate(ctx, k8sClient, modelName)
}
