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

package helpers

import (
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
)

// CopyPullSecrets returns a deep copy of the provided image pull secrets slice.
func CopyPullSecrets(in []corev1.LocalObjectReference) []corev1.LocalObjectReference {
	if len(in) == 0 {
		return nil
	}
	out := make([]corev1.LocalObjectReference, len(in))
	copy(out, in)
	return out
}

// CopyEnvVars returns a deep copy of the provided environment variables slice.
func CopyEnvVars(in []corev1.EnvVar) []corev1.EnvVar {
	if len(in) == 0 {
		return nil
	}
	out := make([]corev1.EnvVar, len(in))
	copy(out, in)
	return out
}

// mergeEnvVars combines default env vars with service-specific overrides.
// Service env vars take precedence over defaults when env var names match.
func MergeEnvVars(defaults []v1.EnvVar, overrides []v1.EnvVar) []v1.EnvVar {
	// Create a map for quick lookup of overrides
	overrideMap := make(map[string]v1.EnvVar)
	for _, env := range overrides {
		overrideMap[env.Name] = env
	}

	// Start with defaults, replacing any that are overridden
	merged := make([]v1.EnvVar, 0, len(defaults)+len(overrides))
	for _, env := range defaults {
		if override, exists := overrideMap[env.Name]; exists {
			merged = append(merged, override)
			delete(overrideMap, env.Name) // Mark as processed
		} else {
			merged = append(merged, env)
		}
	}

	// Add any remaining overrides that weren't in defaults
	for _, env := range overrides {
		if _, processed := overrideMap[env.Name]; !processed {
			continue // Already added in the loop above
		}
		merged = append(merged, env)
	}

	return merged
}
