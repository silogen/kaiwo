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

	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
)

// GetDefaultClusterConfig fetches the "default" AIMClusterConfig.
// Returns nil if not found (without error), error only on API failures.
func GetDefaultClusterConfig(ctx context.Context, k8sClient client.Client) (*aimv1alpha1.AIMClusterConfig, error) {
	var config aimv1alpha1.AIMClusterConfig
	key := client.ObjectKey{Name: DefaultConfigName}

	if err := k8sClient.Get(ctx, key, &config); err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	return &config, nil
}
