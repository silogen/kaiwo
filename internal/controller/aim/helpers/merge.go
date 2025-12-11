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
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
)

// MergeEnvVars combines default env vars with service-specific overrides.
// Service env vars take precedence over defaults when env var names match.
// If jsonMergeKeys is provided, env vars with those names are deep-merged as JSON objects.
func MergeEnvVars(defaults, overrides []corev1.EnvVar, jsonMergeKeys ...string) []corev1.EnvVar {
	// Build set of JSON-mergeable keys
	jsonMerge := make(map[string]bool, len(jsonMergeKeys))
	for _, key := range jsonMergeKeys {
		jsonMerge[key] = true
	}

	// Create a map for quick lookup of overrides
	overrideMap := make(map[string]corev1.EnvVar)
	for _, env := range overrides {
		overrideMap[env.Name] = env
	}

	// Start with defaults, replacing or merging any that are overridden
	merged := make([]corev1.EnvVar, 0, len(defaults)+len(overrides))
	for _, env := range defaults {
		if override, exists := overrideMap[env.Name]; exists {
			// Check if this is a JSON-mergeable env var
			if jsonMerge[env.Name] {
				mergedValue := MergeJSONEnvVarValues(env.Value, override.Value)
				merged = append(merged, corev1.EnvVar{
					Name:  env.Name,
					Value: mergedValue,
				})
			} else {
				merged = append(merged, override)
			}
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

// MergeJSONEnvVarValues deep-merges two JSON object strings.
// The higher precedence value (from) takes priority in case of key conflicts.
// Non-JSON values or parsing errors result in the higher precedence value being used directly.
func MergeJSONEnvVarValues(base, higher string) string {
	if base == "" {
		return higher
	}
	if higher == "" {
		return base
	}

	var baseObj, higherObj map[string]any
	if err := json.Unmarshal([]byte(base), &baseObj); err != nil {
		// base is not valid JSON, use higher precedence value
		return higher
	}
	if err := json.Unmarshal([]byte(higher), &higherObj); err != nil {
		// higher is not valid JSON, use it as-is (overwrite)
		return higher
	}

	// Deep merge: higher takes precedence
	DeepMergeMap(baseObj, higherObj)

	result, err := json.Marshal(baseObj)
	if err != nil {
		// Merge failed, use higher precedence value
		return higher
	}

	return string(result)
}

// DeepMergeMap recursively merges src into dst.
// Values from src take precedence. Nested maps are merged recursively.
func DeepMergeMap(dst, src map[string]any) {
	for key, srcVal := range src {
		if dstVal, exists := dst[key]; exists {
			// Both have this key, check if both are maps for recursive merge
			srcMap, srcIsMap := srcVal.(map[string]any)
			dstMap, dstIsMap := dstVal.(map[string]any)
			if srcIsMap && dstIsMap {
				DeepMergeMap(dstMap, srcMap)
				continue
			}
		}
		// Not both maps, or key doesn't exist in dst - use src value
		dst[key] = srcVal
	}
}
