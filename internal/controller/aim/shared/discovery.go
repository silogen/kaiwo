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
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
)

// discoveryResult represents the raw output from a discovery job.
// This is an internal type used only for parsing the JSON output.
type discoveryResult struct {
	Filename string                 `json:"filename"`
	Profile  discoveryProfileResult `json:"profile"`
	Models   []discoveryModelResult `json:"models"`
}

// discoveryProfileResult is the raw profile format from discovery job output
type discoveryProfileResult struct {
	Model          string            `json:"model"`
	QuantizedModel string            `json:"quantized_model"`
	Metadata       profileMetadata   `json:"metadata"`
	EngineArgs     map[string]any    `json:"engine_args"`
	EnvVars        map[string]string `json:"env_vars"`
}

// profileMetadata is the raw metadata format from discovery job output
type profileMetadata struct {
	Engine    string `json:"engine"`
	GPU       string `json:"gpu"`
	Precision string `json:"precision"`
	GPUCount  int32  `json:"gpu_count"`
	Metric    string `json:"metric"`
	Type      string `json:"type"`
}

// discoveryModelResult represents a model in the raw discovery output
type discoveryModelResult struct {
	Name   string  `json:"name"`
	Source string  `json:"source"`
	SizeGB float64 `json:"size_gb"`
}

// convertToAIMProfile converts the raw discovery profile to AIMProfile API type
func convertToAIMProfile(raw discoveryProfileResult) (*aimv1alpha1.AIMProfile, error) {
	// Marshal engine args to JSON
	engineArgsBytes, err := json.Marshal(raw.EngineArgs)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal engine args: %w", err)
	}

	return &aimv1alpha1.AIMProfile{
		EngineArgs: &apiextensionsv1.JSON{Raw: engineArgsBytes},
		EnvVars:    raw.EnvVars,
		Metadata: aimv1alpha1.AIMProfileMetadata{
			Engine:    raw.Metadata.Engine,
			GPU:       raw.Metadata.GPU,
			GPUCount:  raw.Metadata.GPUCount,
			Metric:    aimv1alpha1.AIMMetric(raw.Metadata.Metric),
			Precision: aimv1alpha1.AIMPrecision(raw.Metadata.Precision),
			Type:      aimv1alpha1.AIMProfileType(raw.Metadata.Type),
		},
	}, nil
}

// convertToAIMModelSources converts raw discovery models to AIMModelSource API types
func convertToAIMModelSources(models []discoveryModelResult) []aimv1alpha1.AIMModelSource {
	var modelSources []aimv1alpha1.AIMModelSource
	for _, model := range models {
		// Convert GB to bytes for resource.Quantity
		sizeBytes := int64(model.SizeGB * 1024 * 1024 * 1024)
		size := resource.NewQuantity(sizeBytes, resource.BinarySI)

		modelSources = append(modelSources, aimv1alpha1.AIMModelSource{
			Name:      model.Name,
			SourceURI: model.Source,
			Size:      *size,
		})
	}
	return modelSources
}

// ParsedDiscovery holds the parsed discovery result
type ParsedDiscovery struct {
	ModelSources []aimv1alpha1.AIMModelSource
	Profile      *aimv1alpha1.AIMProfile
}

// DiscoveryJobSpec defines parameters for creating a discovery job
type DiscoveryJobSpec struct {
	TemplateName     string
	TemplateSpec     aimv1alpha1.AIMServiceTemplateSpecCommon
	Namespace        string
	ModelID          string
	Image            string
	Env              []corev1.EnvVar
	ImagePullSecrets []corev1.LocalObjectReference
	ServiceAccount   string
	OwnerRef         metav1.OwnerReference
}

const (
	// Kubernetes name limit
	kubernetesNameMaxLength = 63

	// Job name components
	discoveryJobPrefix = "discover-"
	discoveryJobSuffix = "-"

	// Hash length for job uniqueness (4 bytes = 8 hex chars)
	discoveryJobHashLength = 4
	discoveryJobHashHexLen = 8

	// DiscoveryJobBackoffLimit is the number of retries before marking the discovery job as failed
	DiscoveryJobBackoffLimit = 3

	// DiscoveryJobTTLSeconds defines how long completed discovery jobs persist
	// before automatic cleanup. This allows time for status inspection and log retrieval.
	DiscoveryJobTTLSeconds = 60
)

// BuildDiscoveryJob creates a Job that runs model discovery dry-run
func BuildDiscoveryJob(spec DiscoveryJobSpec) *batchv1.Job {
	// Create deterministic job name with hash of ALL parameters that affect the Job spec
	// This ensures that any change to the spec results in a new Job instead of an update attempt
	hashInput := spec.ModelID + spec.Image + spec.ServiceAccount

	// Include env vars in hash (sorted for determinism)
	for _, env := range spec.Env {
		hashInput += env.Name + env.Value
	}

	// Include image pull secrets in hash
	for _, secret := range spec.ImagePullSecrets {
		hashInput += secret.Name
	}

	// Include template spec fields that affect env vars
	if spec.TemplateSpec.Metric != nil {
		hashInput += string(*spec.TemplateSpec.Metric)
	}
	if spec.TemplateSpec.Precision != nil {
		hashInput += string(*spec.TemplateSpec.Precision)
	}
	if spec.TemplateSpec.GpuSelector != nil {
		hashInput += spec.TemplateSpec.GpuSelector.Model + strconv.Itoa(int(spec.TemplateSpec.GpuSelector.Count))
	}

	hash := sha256.Sum256([]byte(hashInput))
	hashHex := fmt.Sprintf("%x", hash[:discoveryJobHashLength])

	// Calculate max template name length to keep total <= 63 chars
	// Format: "discover-<template>-<hash>"
	reservedLength := len(discoveryJobPrefix) + len(discoveryJobSuffix) + discoveryJobHashHexLen
	maxTemplateNameLength := kubernetesNameMaxLength - reservedLength

	// Truncate template name if necessary
	templateName := spec.TemplateName
	if len(templateName) > maxTemplateNameLength {
		templateName = templateName[:maxTemplateNameLength]
	}

	jobName := fmt.Sprintf("%s%s%s%s", discoveryJobPrefix, templateName, discoveryJobSuffix, hashHex)

	backoffLimit := int32(DiscoveryJobBackoffLimit)
	ttlSeconds := int32(DiscoveryJobTTLSeconds)

	// Add AIM environmental variables

	// Silence logging to produce clean JSON output
	env := []corev1.EnvVar{
		{
			Name:  "AIM_LOG_LEVEL_ROOT",
			Value: "CRITICAL",
		},
		{
			Name:  "AIM_LOG_LEVEL",
			Value: "CRITICAL",
		},
	}
	env = append(env, spec.Env...)

	if spec.TemplateSpec.Metric != nil {
		env = append(env, corev1.EnvVar{
			Name:  "AIM_METRIC",
			Value: string(*spec.TemplateSpec.Metric),
		})
	}

	if spec.TemplateSpec.Precision != nil {
		env = append(env, corev1.EnvVar{
			Name:  "AIM_PRECISION",
			Value: string(*spec.TemplateSpec.Precision),
		})
	}

	if spec.TemplateSpec.GpuSelector != nil {
		if spec.TemplateSpec.GpuSelector.Model != "" {
			env = append(env, corev1.EnvVar{
				Name:  "AIM_GPU_MODEL",
				Value: spec.TemplateSpec.GpuSelector.Model,
			})
		}
		if spec.TemplateSpec.GpuSelector.Count > 0 {
			env = append(env, corev1.EnvVar{
				Name:  "AIM_GPU_COUNT",
				Value: strconv.Itoa(int(spec.TemplateSpec.GpuSelector.Count)),
			})
		}
	}

	// If a profile ID is set, propagate it
	if profileId := spec.TemplateSpec.ProfileId; profileId != "" {
		env = append(env, corev1.EnvVar{
			Name:  "AIM_PROFILE_ID",
			Value: profileId,
		})
	}

	// Security context for pod security standards compliance
	allowPrivilegeEscalation := false
	runAsNonRoot := true
	runAsUser := int64(65532) // Standard non-root user ID (commonly used in distroless images)
	seccompProfile := &corev1.SeccompProfile{
		Type: corev1.SeccompProfileTypeRuntimeDefault,
	}

	job := &batchv1.Job{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "batch/v1",
			Kind:       "Job",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: spec.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       LabelValueDiscoveryName,
				"app.kubernetes.io/component":  LabelValueDiscoveryComponent,
				"app.kubernetes.io/managed-by": LabelValueManagedBy,
				LabelKeyTemplate:               spec.TemplateName,
			},
			OwnerReferences: []metav1.OwnerReference{spec.OwnerRef},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoffLimit,
			TTLSecondsAfterFinished: &ttlSeconds,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy:      corev1.RestartPolicyNever,
					ImagePullSecrets:   spec.ImagePullSecrets,
					ServiceAccountName: spec.ServiceAccount,
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot:   &runAsNonRoot,
						RunAsUser:      &runAsUser,
						SeccompProfile: seccompProfile,
					},
					Containers: []corev1.Container{
						{
							Name:  "discovery",
							Image: spec.Image,
							Args: []string{
								"dry-run",
								"--format=json",
							},
							Env: env,
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: &allowPrivilegeEscalation,
								RunAsNonRoot:             &runAsNonRoot,
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
							},
						},
					},
				},
			},
		},
	}

	return job
}

// GetDiscoveryJob fetches the discovery job for a template.
// Returns the newest job (by CreationTimestamp) if multiple exist.
func GetDiscoveryJob(ctx context.Context, k8sClient client.Client, namespace, templateName string) (*batchv1.Job, error) {
	var jobList batchv1.JobList
	if err := k8sClient.List(ctx, &jobList, client.InNamespace(namespace), client.MatchingLabels{
		LabelKeyTemplate: templateName,
	}); err != nil {
		return nil, err
	}

	if len(jobList.Items) == 0 {
		return nil, nil
	}

	// Sort by CreationTimestamp descending (newest first)
	sort.Slice(jobList.Items, func(i, j int) bool {
		return jobList.Items[i].CreationTimestamp.After(jobList.Items[j].CreationTimestamp.Time)
	})

	// Return the newest job
	return &jobList.Items[0], nil
}

// IsJobComplete returns true if the job has completed (successfully or failed)
func IsJobComplete(job *batchv1.Job) bool {
	if job == nil {
		return false
	}

	for _, condition := range job.Status.Conditions {
		if condition.Type == batchv1.JobComplete && condition.Status == corev1.ConditionTrue {
			return true
		}
		if condition.Type == batchv1.JobFailed && condition.Status == corev1.ConditionTrue {
			return true
		}
	}

	return false
}

// IsJobSucceeded returns true if the job completed successfully
func IsJobSucceeded(job *batchv1.Job) bool {
	if job == nil {
		return false
	}

	for _, condition := range job.Status.Conditions {
		if condition.Type == batchv1.JobComplete && condition.Status == corev1.ConditionTrue {
			return true
		}
	}

	return false
}

// IsJobFailed returns true if the job failed
func IsJobFailed(job *batchv1.Job) bool {
	if job == nil {
		return false
	}

	for _, condition := range job.Status.Conditions {
		if condition.Type == batchv1.JobFailed && condition.Status == corev1.ConditionTrue {
			return true
		}
	}

	return false
}

// findSuccessfulPodForJob locates a successfully completed pod for the given job.
func findSuccessfulPodForJob(ctx context.Context, k8sClient client.Client, job *batchv1.Job) (*corev1.Pod, error) {
	var podList corev1.PodList
	if err := k8sClient.List(ctx, &podList, client.InNamespace(job.Namespace), client.MatchingLabels{
		"job-name": job.Name,
	}); err != nil {
		return nil, fmt.Errorf("failed to list pods for job: %w", err)
	}

	if len(podList.Items) == 0 {
		return nil, fmt.Errorf("no pods found for job %s", job.Name)
	}

	// Find a successful pod
	for i := range podList.Items {
		pod := &podList.Items[i]
		if pod.Status.Phase == corev1.PodSucceeded {
			return pod, nil
		}
	}

	return nil, fmt.Errorf("no successful pod found for job %s", job.Name)
}

// streamPodLogs retrieves logs from the specified pod's discovery container.
func streamPodLogs(ctx context.Context, clientset kubernetes.Interface, pod *corev1.Pod) ([]byte, error) {
	req := clientset.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{
		Container: "discovery",
	})

	logs, err := req.Stream(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get pod logs: %w", err)
	}
	defer func() {
		if closeErr := logs.Close(); closeErr != nil {
			// Log the error but don't fail the operation since we may have already read the logs
			// Note: In production, this should use a proper logger from the context
			fmt.Fprintf(os.Stderr, "warning: failed to close log stream: %v\n", closeErr)
		}
	}()

	logBytes, err := io.ReadAll(logs)
	if err != nil {
		return nil, fmt.Errorf("failed to read pod logs: %w", err)
	}

	return logBytes, nil
}

// extractLastValidJSONArray attempts to find and extract a valid JSON array from mixed log output.
// Returns the JSON bytes if found, or an error if extraction fails.
func extractLastValidJSONArray(logBytes []byte) ([]byte, error) {
	lastStartIdx := -1
	lastEndIdx := -1

	// Find the last occurrence of '[' that starts a valid JSON array
	for i := len(logBytes) - 1; i >= 0; i-- {
		if logBytes[i] == ']' && lastEndIdx == -1 {
			lastEndIdx = i
		}
		if logBytes[i] == '[' && lastEndIdx != -1 {
			// Try parsing from this '[' to the found ']'
			testBytes := logBytes[i : lastEndIdx+1]
			var testResults []discoveryResult
			if json.Unmarshal(testBytes, &testResults) == nil && len(testResults) > 0 {
				// Valid JSON array found
				lastStartIdx = i
				break
			}
		}
	}

	if lastStartIdx == -1 || lastEndIdx == -1 {
		return nil, fmt.Errorf("no valid JSON array found in logs")
	}

	return logBytes[lastStartIdx : lastEndIdx+1], nil
}

// parseDiscoveryJSON parses discovery results from log bytes, with fallback for mixed output.
func parseDiscoveryJSON(ctx context.Context, logBytes []byte) ([]discoveryResult, error) {
	// Check for cancellation before expensive parsing
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled before parsing JSON: %w", err)
	}

	var results []discoveryResult
	if err := json.Unmarshal(logBytes, &results); err == nil {
		return results, nil
	}

	// Try extracting the last valid JSON array from mixed stdout/stderr
	jsonBytes, err := extractLastValidJSONArray(logBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse discovery JSON: %w", err)
	}

	if err := json.Unmarshal(jsonBytes, &results); err != nil {
		return nil, fmt.Errorf("failed to parse extracted JSON array: %w", err)
	}

	return results, nil
}

// ParseDiscoveryLogs parses the discovery job output to extract model sources and profile.
// Reads pod logs from the completed job and parses the JSON output.
func ParseDiscoveryLogs(ctx context.Context, k8sClient client.Client, clientset kubernetes.Interface, job *batchv1.Job) (*ParsedDiscovery, error) {
	// Check for cancellation early
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled before parsing discovery logs: %w", err)
	}

	if !IsJobSucceeded(job) {
		return nil, fmt.Errorf("job has not succeeded yet")
	}

	// Find successful pod
	successfulPod, err := findSuccessfulPodForJob(ctx, k8sClient, job)
	if err != nil {
		return nil, err
	}

	// Stream pod logs
	logBytes, err := streamPodLogs(ctx, clientset, successfulPod)
	if err != nil {
		return nil, err
	}

	// Parse discovery JSON
	results, err := parseDiscoveryJSON(ctx, logBytes)
	if err != nil {
		return nil, err
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("discovery output contains empty array")
	}

	// Use the first result
	result := results[0]

	// Convert raw discovery profile to AIMProfile
	profile, err := convertToAIMProfile(result.Profile)
	if err != nil {
		return nil, fmt.Errorf("failed to convert profile: %w", err)
	}

	// Convert raw models to AIMModelSource
	modelSources := convertToAIMModelSources(result.Models)

	return &ParsedDiscovery{
		ModelSources: modelSources,
		Profile:      profile,
	}, nil
}
