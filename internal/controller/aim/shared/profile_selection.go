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

// TemplateCandidate captures the information needed to evaluate a template during selection.
type TemplateCandidate struct {
	Name      string
	Namespace string
	Scope     TemplateScope
	Spec      aimv1alpha1.AIMServiceTemplateSpecCommon
	Status    aimv1alpha1.AIMServiceTemplateStatus
}

// QualifiedName returns a human-readable identifier for logging/debugging.
func (c TemplateCandidate) QualifiedName() string {
	if c.Scope == TemplateScopeNamespace && c.Namespace != "" {
		return c.Namespace + "/" + c.Name
	}
	return c.Name
}

// SelectBestTemplate selects the best template candidate from the provided list.
// The heuristic is:
// 1. Consider only templates that are Available.
// 2. Filter by service overrides when provided.
// 3. Filter by GPUs that exist in the cluster.
// 4. Prefer namespace-scoped templates over cluster-scoped templates.
// 5. Prefer higher-tier GPUs, then latency over throughput, then lower precision.
// Returns (selected template, count of templates with identical preference scores).
// If count > 1, the templates are ambiguous (identical in all preference dimensions).
func SelectBestTemplate(
	candidates []TemplateCandidate,
	overrides *aimv1alpha1.AIMServiceOverrides,
	availableGPUs []string,
) (*TemplateCandidate, int) {
	filtered := filterAvailableTemplates(candidates)
	if len(filtered) == 0 {
		return nil, 0
	}

	filtered = filterTemplatesByOverrides(filtered, overrides)
	if len(filtered) == 0 {
		return nil, 0
	}

	filtered = filterTemplatesByGPUAvailability(filtered, availableGPUs)
	if len(filtered) == 0 {
		return nil, 0
	}

	// Prefer namespace-scoped templates over cluster-scoped templates
	filtered = preferNamespaceTemplates(filtered)

	if len(filtered) == 1 {
		return &filtered[0], 1
	}

	return choosePreferredTemplate(filtered)
}

func filterAvailableTemplates(candidates []TemplateCandidate) []TemplateCandidate {
	result := make([]TemplateCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Status.Status == aimv1alpha1.AIMTemplateStatusReady {
			result = append(result, candidate)
		}
	}
	return result
}

func filterTemplatesByOverrides(candidates []TemplateCandidate, overrides *aimv1alpha1.AIMServiceOverrides) []TemplateCandidate {
	if overrides == nil {
		return candidates
	}

	result := make([]TemplateCandidate, 0, len(candidates))

	for _, candidate := range candidates {
		templateMetric := candidateMetric(candidate)
		templatePrecision := candidatePrecision(candidate)
		templateGPU := candidateGPUModel(candidate)

		if overrides.Metric != nil && !strings.EqualFold(templateMetric, string(*overrides.Metric)) {
			continue
		}

		if overrides.Precision != nil && !strings.EqualFold(templatePrecision, string(*overrides.Precision)) {
			continue
		}

		if overrides.GpuSelector != nil {
			overrideGPU := strings.TrimSpace(overrides.GpuSelector.Model)
			if overrideGPU != "" && !strings.EqualFold(templateGPU, overrideGPU) {
				continue
			}
		}

		result = append(result, candidate)
	}

	return result
}

func preferNamespaceTemplates(candidates []TemplateCandidate) []TemplateCandidate {
	// If there are any namespace-scoped templates, filter out cluster-scoped ones
	hasNamespaceTemplate := false
	for _, candidate := range candidates {
		if candidate.Scope == TemplateScopeNamespace {
			hasNamespaceTemplate = true
			break
		}
	}

	if !hasNamespaceTemplate {
		return candidates
	}

	// Return only namespace-scoped templates
	result := make([]TemplateCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Scope == TemplateScopeNamespace {
			result = append(result, candidate)
		}
	}
	return result
}

func filterTemplatesByGPUAvailability(candidates []TemplateCandidate, availableGPUs []string) []TemplateCandidate {
	gpuMap := make(map[string]struct{}, len(availableGPUs))
	for _, gpu := range availableGPUs {
		normalized := normalizeGPUModel(strings.TrimSpace(gpu))
		if normalized != "" {
			gpuMap[normalized] = struct{}{}
		}
	}

	result := make([]TemplateCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		model := strings.TrimSpace(candidateGPUModel(candidate))
		if model == "" {
			result = append(result, candidate)
			continue
		}

		normalized := normalizeGPUModel(model)
		if _, ok := gpuMap[normalized]; ok {
			result = append(result, candidate)
		}
	}

	return result
}

func choosePreferredTemplate(candidates []TemplateCandidate) (*TemplateCandidate, int) {
	if len(candidates) == 0 {
		return nil, 0
	}

	if len(candidates) == 1 {
		return &candidates[0], 1
	}

	gpuPref := makePreferenceMap(GPUPreferenceOrder)
	metricPref := makePreferenceMap(MetricPreferenceOrder)
	precisionPref := makePreferenceMap(PrecisionPreferenceOrder)

	bestIndex := 0
	bestGPUScore := getPreferenceScore(candidateGPUModel(candidates[0]), gpuPref)
	bestMetricScore := getPreferenceScore(candidateMetric(candidates[0]), metricPref)
	bestPrecisionScore := getPreferenceScore(candidatePrecision(candidates[0]), precisionPref)

	for i := 1; i < len(candidates); i++ {
		currentGPUScore := getPreferenceScore(candidateGPUModel(candidates[i]), gpuPref)
		currentMetricScore := getPreferenceScore(candidateMetric(candidates[i]), metricPref)
		currentPrecisionScore := getPreferenceScore(candidatePrecision(candidates[i]), precisionPref)

		if currentGPUScore < bestGPUScore {
			bestIndex = i
			bestGPUScore = currentGPUScore
			bestMetricScore = currentMetricScore
			bestPrecisionScore = currentPrecisionScore
			continue
		}
		if currentGPUScore > bestGPUScore {
			continue
		}

		if currentMetricScore < bestMetricScore {
			bestIndex = i
			bestMetricScore = currentMetricScore
			bestPrecisionScore = currentPrecisionScore
			continue
		}
		if currentMetricScore > bestMetricScore {
			continue
		}

		if currentPrecisionScore < bestPrecisionScore {
			bestIndex = i
			bestPrecisionScore = currentPrecisionScore
		}
	}

	// Count how many templates have the exact same preference scores as the best one
	// If multiple templates have identical scores, they are ambiguous
	identicalCount := 0
	for i := range candidates {
		gpuScore := getPreferenceScore(candidateGPUModel(candidates[i]), gpuPref)
		metricScore := getPreferenceScore(candidateMetric(candidates[i]), metricPref)
		precisionScore := getPreferenceScore(candidatePrecision(candidates[i]), precisionPref)

		if gpuScore == bestGPUScore && metricScore == bestMetricScore && precisionScore == bestPrecisionScore {
			identicalCount++
		}
	}

	return &candidates[bestIndex], identicalCount
}

func candidateMetric(candidate TemplateCandidate) string {
	if metric := candidate.Status.Profile.Metadata.Metric; metric != "" {
		return string(metric)
	}
	if candidate.Spec.Metric != nil {
		return string(*candidate.Spec.Metric)
	}
	return ""
}

func candidatePrecision(candidate TemplateCandidate) string {
	if precision := candidate.Status.Profile.Metadata.Precision; precision != "" {
		return string(precision)
	}
	if candidate.Spec.Precision != nil {
		return string(*candidate.Spec.Precision)
	}
	return ""
}

func candidateGPUModel(candidate TemplateCandidate) string {
	if candidate.Spec.GpuSelector != nil {
		model := strings.TrimSpace(candidate.Spec.GpuSelector.Model)
		if model != "" {
			return model
		}
	}
	if gpu := strings.TrimSpace(candidate.Status.Profile.Metadata.GPU); gpu != "" {
		return gpu
	}
	return ""
}

// GPUPreferenceOrder defines the preference order for GPU models when selecting templates.
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
	return len(prefMap) + 1000
}
