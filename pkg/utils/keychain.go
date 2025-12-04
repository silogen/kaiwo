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

package baseutils

import (
	"context"
	"fmt"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/authn/k8schain"
	kauth "github.com/google/go-containerregistry/pkg/authn/kubernetes"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)

// BuildKeychain creates an authn.Keychain for authenticating to container registries.
// It uses Kubernetes image pull secrets if provided, otherwise falls back to the default keychain.
//
// Parameters:
//   - ctx: Context for the operation
//   - clientset: Kubernetes clientset for accessing secrets (can be nil)
//   - secretNamespace: Namespace where secrets are located
//   - imagePullSecrets: List of secret references for authentication
//
// Returns:
//   - authn.Keychain: Configured keychain for authentication
//   - error: Any error encountered during keychain creation
func BuildKeychain(
	ctx context.Context,
	clientset kubernetes.Interface,
	secretNamespace string,
	imagePullSecrets []corev1.LocalObjectReference,
) (authn.Keychain, error) {
	// If no clientset or secrets provided, use default keychain
	if clientset == nil || secretNamespace == "" || len(imagePullSecrets) == 0 {
		return authn.DefaultKeychain, nil
	}

	// Convert LocalObjectReference to secret names
	secretNames := make([]string, len(imagePullSecrets))
	for i, secret := range imagePullSecrets {
		secretNames[i] = secret.Name
	}

	// Create k8s keychain with the provided secrets
	kc, err := k8schain.New(ctx, clientset, k8schain.Options{
		Namespace:          secretNamespace,
		ImagePullSecrets:   secretNames,
		ServiceAccountName: kauth.NoServiceAccount,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create k8s keychain: %w", err)
	}

	return kc, nil
}
