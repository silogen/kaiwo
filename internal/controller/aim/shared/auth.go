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

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/authn/k8schain"
	kauth "github.com/google/go-containerregistry/pkg/authn/kubernetes"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
)

// BuildKeychain creates an authentication keychain for accessing container registries.
// It supports Kubernetes image pull secrets and falls back to the default keychain
// (which uses docker config, credential helpers, etc.) when no secrets are provided.
//
// Parameters:
//   - ctx: Context for the operation
//   - clientset: Kubernetes clientset for accessing secrets
//   - namespace: Namespace where the secrets are located
//   - imagePullSecrets: Kubernetes image pull secrets for authentication
//
// Returns:
//   - authn.Keychain: The configured keychain for registry authentication
//   - error: Any error encountered while creating the keychain
func BuildKeychain(
	ctx context.Context,
	clientset kubernetes.Interface,
	namespace string,
	imagePullSecrets []corev1.LocalObjectReference,
) (authn.Keychain, error) {
	logger := ctrl.LoggerFrom(ctx)

	// If we have a clientset, namespace, and secrets, use k8schain
	if clientset != nil && namespace != "" && len(imagePullSecrets) > 0 {
		// Convert LocalObjectReference to secret names
		secretNames := make([]string, len(imagePullSecrets))
		for i, secret := range imagePullSecrets {
			secretNames[i] = secret.Name
		}

		logger.V(1).Info("Building keychain with image pull secrets", "secrets", secretNames, "namespace", namespace)

		// Create k8s keychain with the provided secrets
		kc, err := k8schain.New(ctx, clientset, k8schain.Options{
			Namespace:          namespace,
			ImagePullSecrets:   secretNames,
			ServiceAccountName: kauth.NoServiceAccount,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create k8s keychain: %w", err)
		}
		return kc, nil
	}

	// Fall back to default keychain (uses docker config, credential helpers, etc.)
	logger.V(1).Info("Using default keychain (no image pull secrets provided)")
	return authn.DefaultKeychain, nil
}
