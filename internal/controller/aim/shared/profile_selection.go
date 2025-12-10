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

// SelectionDiagnostics provides detailed information about why template selection failed.
type SelectionDiagnostics struct {
	TotalCandidates                  int
	AfterAvailabilityFilter          int
	AfterUnoptimizedFilter           int
	AfterOverridesFilter             int
	AfterGPUAvailabilityFilter       int
	UnoptimizedTemplatesWereFiltered bool
}

// CandidateEvaluation captures why a specific candidate was chosen or rejected.
type CandidateEvaluation struct {
	Candidate TemplateCandidate
	Status    string // "chosen" or "rejected"
	Reason    string // CamelCase reason
	Rank      int    // For candidates that passed all filters
}

// QualifiedName returns a human-readable identifier for logging/debugging.
func (c TemplateCandidate) QualifiedName() string {
	if c.Scope == TemplateScopeNamespace && c.Namespace != "" {
		return c.Namespace + "/" + c.Name
	}
	return c.Name
}

type rejectionStage string

const (
	stageAvailability rejectionStage = "availability"
	stageUnoptimized  rejectionStage = "unoptimized"
	stageOverrides    rejectionStage = "overrides"
	stageGPU          rejectionStage = "gpu"
)

const (
	statusChosen   = "chosen"
	statusRejected = "rejected"

	reasonBestMatch           = "BestMatch"
	reasonLowerPreferenceRank = "LowerPreferenceRank"

	reasonUnoptimizedFiltered = "UnoptimizedTemplateFiltered"
	reasonOverridesNotMatched = "ServiceOverridesNotMatched"
	reasonGPUNotInCluster     = "RequiredGPUNotInCluster"
)

// appendRejections emits CandidateEvaluation entries for all rejected candidates.
func appendRejections(
	evals *[]CandidateEvaluation,
	rejectedByStage map[rejectionStage][]TemplateCandidate,
) {
	// availability uses per-template status-derived reason
	for _, c := range rejectedByStage[stageAvailability] {
		*evals = append(*evals, CandidateEvaluation{
			Candidate: c,
			Status:    statusRejected,
			Reason:    getRejectionReasonForStatus(c.Status.Status),
		})
	}

	addWithReason := func(stage rejectionStage, reason string) {
		for _, c := range rejectedByStage[stage] {
			*evals = append(*evals, CandidateEvaluation{
				Candidate: c,
				Status:    statusRejected,
				Reason:    reason,
			})
		}
	}

	addWithReason(stageUnoptimized, reasonUnoptimizedFiltered)
	addWithReason(stageOverrides, reasonOverridesNotMatched)
	addWithReason(stageGPU, reasonGPUNotInCluster)
}

// SelectBestTemplate selects the best template candidate from the provided list.
// The heuristic is:
// 1. Consider only templates that are Available.
// 2. Filter by service overrides when provided.
// 3. Filter by GPUs that exist in the cluster.
// 4. Prefer namespace-scoped templates over cluster-scoped templates.
// 5. Prefer higher-tier GPUs, then latency over throughput, then lower precision.
// Returns (selected template, count of templates with identical preference scores, diagnostics, per-candidate evaluations).
// If count > 1, the templates are ambiguous (identical in all preference dimensions).
func SelectBestTemplate(
	candidates []TemplateCandidate,
	overrides *aimv1alpha1.AIMServiceOverrides,
	availableGPUs []string,
	allowUnoptimized bool,
) (*TemplateCandidate, int, SelectionDiagnostics, []CandidateEvaluation) {
	diag := SelectionDiagnostics{
		TotalCandidates: len(candidates),
	}

	evaluations := make([]CandidateEvaluation, 0, len(candidates))
	rejectedByStage := make(map[rejectionStage][]TemplateCandidate)

	// --- Stage 1: Availability filter ---

	var filtered []TemplateCandidate
	for _, c := range candidates {
		if c.Status.Status == aimv1alpha1.AIMTemplateStatusReady {
			filtered = append(filtered, c)
		} else {
			rejectedByStage[stageAvailability] = append(rejectedByStage[stageAvailability], c)
		}
	}

	diag.AfterAvailabilityFilter = len(filtered)
	if len(filtered) == 0 {
		appendRejections(&evaluations, rejectedByStage)
		return nil, 0, diag, evaluations
	}

	// --- Stage 2: Unoptimized filter ---

	beforeUnoptimized := filtered
	filtered = filtered[:0] // reuse backing array
	for _, c := range beforeUnoptimized {
		if c.Status.Profile.Metadata.Type == aimv1alpha1.AIMProfileTypeOptimized || allowUnoptimized {
			filtered = append(filtered, c)
		} else {
			rejectedByStage[stageUnoptimized] = append(rejectedByStage[stageUnoptimized], c)
		}
	}

	diag.AfterUnoptimizedFilter = len(filtered)
	diag.UnoptimizedTemplatesWereFiltered = len(rejectedByStage[stageUnoptimized]) > 0

	if len(filtered) == 0 {
		appendRejections(&evaluations, rejectedByStage)
		return nil, 0, diag, evaluations
	}

	// --- Stage 3: Overrides filter ---

	beforeOverrides := filtered
	filtered = filterTemplatesByOverrides(beforeOverrides, overrides)
	diag.AfterOverridesFilter = len(filtered)

	if len(filtered) == 0 {
		// everything that made it to this stage but wasn’t kept is an overrides rejection
		rejectedByStage[stageOverrides] = append(rejectedByStage[stageOverrides], beforeOverrides...)
		appendRejections(&evaluations, rejectedByStage)
		return nil, 0, diag, evaluations
	}

	// --- Stage 4: GPU availability filter ---

	beforeGPU := filtered
	filtered = filterTemplatesByGPUAvailability(beforeGPU, availableGPUs)
	diag.AfterGPUAvailabilityFilter = len(filtered)

	if len(filtered) == 0 {
		// everything that made it to this stage but wasn’t kept is a GPU rejection
		rejectedByStage[stageGPU] = append(rejectedByStage[stageGPU], beforeGPU...)
		appendRejections(&evaluations, rejectedByStage)
		return nil, 0, diag, evaluations
	}

	// --- Stage 5: namespace-scoped vs cluster-scoped ---

	filtered = preferNamespaceTemplates(filtered)

	if len(filtered) == 1 {
		// All rejected candidates (from all stages)
		appendRejections(&evaluations, rejectedByStage)

		// The chosen one
		evaluations = append(evaluations, CandidateEvaluation{
			Candidate: filtered[0],
			Status:    statusChosen,
			Reason:    reasonBestMatch,
			Rank:      1,
		})

		return &filtered[0], 1, diag, evaluations
	}

	// --- Stage 6: preference scoring among remaining candidates ---

	selected, count := choosePreferredTemplate(filtered)

	// First, record all stage-level rejections
	appendRejections(&evaluations, rejectedByStage)

	// Build preference maps once
	gpuPref := makePreferenceMap(GPUPreferenceOrder)
	metricPref := makePreferenceMap(MetricPreferenceOrder)
	precisionPref := makePreferenceMap(PrecisionPreferenceOrder)
	profileTypePref := makePreferenceMap(ProfileTypePreferenceOrder)

	// Precompute the "best" scores once for the selected candidate
	bestGPUScore := getPreferenceScore(candidateGPUModel(*selected), gpuPref)
	bestMetricScore := getPreferenceScore(candidateMetric(*selected), metricPref)
	bestPrecisionScore := getPreferenceScore(candidatePrecision(*selected), precisionPref)
	bestProfileTypeScore := getPreferenceScore(candidateProfileType(*selected), profileTypePref)

	for i, c := range filtered {
		gpuScore := getPreferenceScore(candidateGPUModel(c), gpuPref)
		metricScore := getPreferenceScore(candidateMetric(c), metricPref)
		precisionScore := getPreferenceScore(candidatePrecision(c), precisionPref)
		profileTypeScore := getPreferenceScore(candidateProfileType(c), profileTypePref)

		switch {
		case c.Name == selected.Name:
			evaluations = append(evaluations, CandidateEvaluation{
				Candidate: c,
				Status:    statusChosen,
				Reason:    reasonBestMatch,
				Rank:      1,
			})

		case gpuScore == bestGPUScore &&
			metricScore == bestMetricScore &&
			precisionScore == bestPrecisionScore &&
			profileTypeScore == bestProfileTypeScore:
			// Same preference scores as the selected candidate but not chosen (tie-breaking elsewhere)
			evaluations = append(evaluations, CandidateEvaluation{
				Candidate: c,
				Status:    statusRejected,
				Reason:    reasonLowerPreferenceRank, // keep existing reason string for compatibility
				Rank:      i + 1,
			})

		default:
			evaluations = append(evaluations, CandidateEvaluation{
				Candidate: c,
				Status:    statusRejected,
				Reason:    reasonLowerPreferenceRank,
				Rank:      i + 1,
			})
		}
	}

	return selected, count, diag, evaluations
}

// getRejectionReasonForStatus converts a template status to a rejection reason.
func getRejectionReasonForStatus(status aimv1alpha1.AIMTemplateStatusEnum) string {
	switch status {
	case aimv1alpha1.AIMTemplateStatusPending:
		return "TemplatePending"
	case aimv1alpha1.AIMTemplateStatusProgressing:
		return "TemplateProgressing"
	case aimv1alpha1.AIMTemplateStatusNotAvailable:
		return "TemplateNotAvailable"
	case aimv1alpha1.AIMTemplateStatusDegraded:
		return "TemplateDegraded"
	case aimv1alpha1.AIMTemplateStatusFailed:
		return "TemplateFailed"
	default:
		return "TemplateNotReady"
	}
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
	profileTypePref := makePreferenceMap(ProfileTypePreferenceOrder)

	bestIndex := 0
	bestGPUScore := getPreferenceScore(candidateGPUModel(candidates[0]), gpuPref)
	bestMetricScore := getPreferenceScore(candidateMetric(candidates[0]), metricPref)
	bestPrecisionScore := getPreferenceScore(candidatePrecision(candidates[0]), precisionPref)
	bestProfileTypeScore := getPreferenceScore(candidateProfileType(candidates[0]), profileTypePref)

	for i := 1; i < len(candidates); i++ {
		currentGPUScore := getPreferenceScore(candidateGPUModel(candidates[i]), gpuPref)
		currentMetricScore := getPreferenceScore(candidateMetric(candidates[i]), metricPref)
		currentPrecisionScore := getPreferenceScore(candidatePrecision(candidates[i]), precisionPref)
		currentProfileTypeScore := getPreferenceScore(candidateProfileType(candidates[i]), profileTypePref)

		if currentProfileTypeScore < bestProfileTypeScore {
			bestIndex = i
			bestGPUScore = currentGPUScore
			bestMetricScore = currentMetricScore
			bestPrecisionScore = currentPrecisionScore
			bestProfileTypeScore = currentProfileTypeScore
		}
		if currentProfileTypeScore > bestProfileTypeScore {
			continue
		}

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

func candidateProfileType(candidate TemplateCandidate) string {
	return string(candidate.Status.Profile.Metadata.Type)
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

var ProfileTypePreferenceOrder = []string{
	string(aimv1alpha1.AIMProfileTypeOptimized),
	string(aimv1alpha1.AIMProfileTypePreview),
	string(aimv1alpha1.AIMProfileTypeUnoptimized),
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
