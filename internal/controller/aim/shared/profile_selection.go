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
	"strings"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
)

// GPUPreferenceOrder defines the preference order for GPU models when selecting profiles.
// GPUs earlier in the list are preferred over later ones.
// TODO: Fill in the complete preference order based on performance characteristics.
var GPUPreferenceOrder = []string{
	"MI325X",
	"MI300X",
	"MI250X",
	"MI210",
	"A100",
	"H100",
	// TODO: Add more GPU models in preference order
}

// MetricPreferenceOrder defines preference for optimization metrics.
// "latency" is preferred over "throughput" by default.
var MetricPreferenceOrder = []string{
	"latency",
	"throughput",
}

// PrecisionPreferenceOrder defines preference for precision levels.
// Lower precision (more optimized) is preferred.
var PrecisionPreferenceOrder = []string{
	"fp8",
	"fp16",
	"bf16",
	"fp32",
}

// SelectBestProfile selects the best profile from a list of discovered profiles
// based on the following heuristic:
// 1. Filter by user overrides (if provided)
// 2. Filter by GPU availability in the cluster
// 3. Apply preference heuristic: GPU model > metric > precision
//
// Returns nil if no suitable profile is found.
func SelectBestProfile(
	profiles []aimv1alpha1.ResolvedProfile,
	overrides *aimv1alpha1.AIMServiceOverrides,
	availableGPUs []string,
) *aimv1alpha1.ResolvedProfile {
	if len(profiles) == 0 {
		return nil
	}

	// Step 1: Filter by user overrides
	filtered := filterByOverrides(profiles, overrides)
	if len(filtered) == 0 {
		return nil
	}

	// Step 2: Filter by GPU availability
	filtered = filterByGPUAvailability(filtered, availableGPUs)
	if len(filtered) == 0 {
		return nil
	}

	// Step 3: Apply preference heuristic
	return applyPreferenceHeuristic(filtered)
}

// filterByOverrides filters profiles based on user-provided overrides.
func filterByOverrides(profiles []aimv1alpha1.ResolvedProfile, overrides *aimv1alpha1.AIMServiceOverrides) []aimv1alpha1.ResolvedProfile {
	if overrides == nil {
		return profiles
	}

	var filtered []aimv1alpha1.ResolvedProfile

	for _, profile := range profiles {
		metric := resolvedProfileMetric(profile)
		precision := resolvedProfilePrecision(profile)
		gpuModel := resolvedProfileGPUModel(profile)

		// Filter by metric if specified
		if overrides.Metric != nil && !strings.EqualFold(metric, string(*overrides.Metric)) {
			continue
		}

		// Filter by precision if specified
		if overrides.Precision != nil && !strings.EqualFold(precision, string(*overrides.Precision)) {
			continue
		}

		// Filter by GPU selector if specified
		if overrides.GpuSelector != nil && !strings.EqualFold(gpuModel, overrides.GpuSelector.Model) {
			continue
		}

		filtered = append(filtered, profile)
	}

	return filtered
}

// filterByGPUAvailability filters profiles to only include those with GPU models
// available in the cluster.
func filterByGPUAvailability(profiles []aimv1alpha1.ResolvedProfile, availableGPUs []string) []aimv1alpha1.ResolvedProfile {
	if len(availableGPUs) == 0 {
		// If no GPUs are available, return empty list
		return nil
	}

	// Create a map for fast lookup
	gpuMap := make(map[string]bool)
	for _, gpu := range availableGPUs {
		gpuMap[strings.ToUpper(gpu)] = true
	}

	var filtered []aimv1alpha1.ResolvedProfile
	for _, profile := range profiles {
		gpuModel := resolvedProfileGPUModel(profile)
		if gpuModel == "" || gpuMap[strings.ToUpper(gpuModel)] {
			filtered = append(filtered, profile)
		}
	}

	return filtered
}

// applyPreferenceHeuristic selects the best profile based on preference ordering.
// Preference order: GPU model > metric > precision
func applyPreferenceHeuristic(profiles []aimv1alpha1.ResolvedProfile) *aimv1alpha1.ResolvedProfile {
	if len(profiles) == 0 {
		return nil
	}

	if len(profiles) == 1 {
		return &profiles[0]
	}

	// Create preference maps for fast lookup
	gpuPref := makePreferenceMap(GPUPreferenceOrder)
	metricPref := makePreferenceMap(MetricPreferenceOrder)
	precisionPref := makePreferenceMap(PrecisionPreferenceOrder)

	best := &profiles[0]
	bestGPUScore := getPreferenceScore(resolvedProfileGPUModel(*best), gpuPref)
	bestMetricScore := getPreferenceScore(resolvedProfileMetric(*best), metricPref)
	bestPrecisionScore := getPreferenceScore(resolvedProfilePrecision(*best), precisionPref)

	for i := 1; i < len(profiles); i++ {
		current := &profiles[i]
		currentGPUScore := getPreferenceScore(resolvedProfileGPUModel(*current), gpuPref)
		currentMetricScore := getPreferenceScore(resolvedProfileMetric(*current), metricPref)
		currentPrecisionScore := getPreferenceScore(resolvedProfilePrecision(*current), precisionPref)

		// Compare by GPU preference first
		if currentGPUScore < bestGPUScore {
			best = current
			bestGPUScore = currentGPUScore
			bestMetricScore = currentMetricScore
			bestPrecisionScore = currentPrecisionScore
			continue
		} else if currentGPUScore > bestGPUScore {
			continue
		}

		// GPU scores are equal, compare by metric
		if currentMetricScore < bestMetricScore {
			best = current
			bestMetricScore = currentMetricScore
			bestPrecisionScore = currentPrecisionScore
			continue
		} else if currentMetricScore > bestMetricScore {
			continue
		}

		// Metric scores are equal, compare by precision
		if currentPrecisionScore < bestPrecisionScore {
			best = current
			bestPrecisionScore = currentPrecisionScore
		}
	}

	return best
}

func resolvedProfileMetric(profile aimv1alpha1.ResolvedProfile) string {
	if profile.Runtime.Metric == nil {
		return ""
	}
	return string(*profile.Runtime.Metric)
}

func resolvedProfilePrecision(profile aimv1alpha1.ResolvedProfile) string {
	if profile.Runtime.Precision == nil {
		return ""
	}
	return string(*profile.Runtime.Precision)
}

func resolvedProfileGPUModel(profile aimv1alpha1.ResolvedProfile) string {
	if profile.Runtime.GpuSelector == nil {
		return ""
	}
	return profile.Runtime.GpuSelector.Model
}

// makePreferenceMap creates a map from preference list to index (lower index = higher preference).
func makePreferenceMap(preferences []string) map[string]int {
	m := make(map[string]int)
	for i, pref := range preferences {
		m[strings.ToUpper(pref)] = i
	}
	return m
}

// getPreferenceScore returns the preference score for a value.
// Lower score means higher preference. Values not in the map get a high score (low preference).
func getPreferenceScore(value string, prefMap map[string]int) int {
	if score, ok := prefMap[strings.ToUpper(value)]; ok {
		return score
	}
	// Unknown values get lowest preference (highest score)
	return len(prefMap) + 1000
}
