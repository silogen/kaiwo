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

// DiscoveryResult represents the output from a discovery job
type DiscoveryResult struct {
	Filename string  `json:"filename"`
	Profile  Profile `json:"profile"`
}

// Profile represents the discovered runtime profile
type Profile struct {
	Model          string            `json:"model"`
	QuantizedModel string            `json:"quantized_model"`
	Metadata       ProfileMetadata   `json:"metadata"`
	EngineArgs     map[string]any    `json:"engine_args"`
	EnvVars        map[string]string `json:"env_vars"`
	Models         []ProfileModel    `json:"models"`
}

// ProfileMetadata contains metadata about the runtime profile
type ProfileMetadata struct {
	Engine    string `json:"engine"`
	GPU       string `json:"gpu"`
	Precision string `json:"precision"`
	GPUCount  int    `json:"gpu_count"`
	Metric    string `json:"metric"`
}

// ProfileModel represents a model in the profile
type ProfileModel struct {
	Name   string  `json:"name"`
	Source string  `json:"source"`
	SizeGB float64 `json:"size_gb"`
}

// ParsedDiscovery holds the parsed discovery result
type ParsedDiscovery struct {
	ModelSources []aimv1alpha1.AIMModelSource
	Profile      *apiextensionsv1.JSON
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
	OwnerRef         metav1.OwnerReference
}

// BuildDiscoveryJob creates a Job that runs model discovery dry-run
func BuildDiscoveryJob(spec DiscoveryJobSpec) *batchv1.Job {
	// Create deterministic job name with hash of key parameters
	hash := sha256.Sum256([]byte(spec.ModelID + spec.Image))
	jobName := fmt.Sprintf("discover-%s-%x", spec.TemplateName, hash[:4])

	backoffLimit := int32(3)
	ttlSeconds := int32(60) // Clean up after 1 minute (enough time to fetch status)

	// Add AIM environmental variables

	var env []corev1.EnvVar
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

	job := &batchv1.Job{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "batch/v1",
			Kind:       "Job",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: spec.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "aim-discovery",
				"app.kubernetes.io/component":  "discovery-job",
				"app.kubernetes.io/managed-by": "aim-controller",
				"aim.silogen.ai/template":      spec.TemplateName,
			},
			OwnerReferences: []metav1.OwnerReference{spec.OwnerRef},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoffLimit,
			TTLSecondsAfterFinished: &ttlSeconds,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy:    corev1.RestartPolicyNever,
					ImagePullSecrets: spec.ImagePullSecrets,
					Containers: []corev1.Container{
						{
							Name:  "discovery",
							Image: spec.Image,
							Args: []string{
								"dry-run",
								"--format=json",
							},
							Env: env,
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
		"aim.silogen.ai/template": templateName,
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

// ParseDiscoveryLogs parses the discovery job output to extract model sources and profile.
// Reads pod logs from the completed job and parses the JSON output.
func ParseDiscoveryLogs(ctx context.Context, k8sClient client.Client, clientset kubernetes.Interface, job *batchv1.Job) (*ParsedDiscovery, error) {
	if !IsJobSucceeded(job) {
		return nil, fmt.Errorf("job has not succeeded yet")
	}

	// List pods for this job
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
	var successfulPod *corev1.Pod
	for i := range podList.Items {
		pod := &podList.Items[i]
		if pod.Status.Phase == corev1.PodSucceeded {
			successfulPod = pod
			break
		}
	}

	if successfulPod == nil {
		return nil, fmt.Errorf("no successful pod found for job %s", job.Name)
	}

	// Get pod logs
	req := clientset.CoreV1().Pods(successfulPod.Namespace).GetLogs(successfulPod.Name, &corev1.PodLogOptions{
		Container: "discovery",
	})

	logs, err := req.Stream(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get pod logs: %w", err)
	}
	defer func() { _ = logs.Close() }()

	// Read logs
	logBytes, err := io.ReadAll(logs)
	if err != nil {
		return nil, fmt.Errorf("failed to read pod logs: %w", err)
	}

	// Parse JSON output
	var result DiscoveryResult
	if err := json.Unmarshal(logBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to parse discovery JSON output: %w", err)
	}

	// Convert profile to JSON
	profileBytes, err := json.Marshal(result.Profile)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal profile: %w", err)
	}

	profileJSON := &apiextensionsv1.JSON{Raw: profileBytes}

	// Extract model sources from profile
	var modelSources []aimv1alpha1.AIMModelSource
	for _, model := range result.Profile.Models {
		// Convert GB to bytes for resource.Quantity
		sizeBytes := int64(model.SizeGB * 1024 * 1024 * 1024)
		size := resource.NewQuantity(sizeBytes, resource.BinarySI)

		modelSources = append(modelSources, aimv1alpha1.AIMModelSource{
			Name:      model.Name,
			SourceURI: model.Source,
			Size:      *size,
		})
	}

	return &ParsedDiscovery{
		ModelSources: modelSources,
		Profile:      profileJSON,
	}, nil
}
