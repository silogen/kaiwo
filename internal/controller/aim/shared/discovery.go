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

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
								"--dry-run",
								"--model-id", spec.ModelID,
								"--output", "json",
							},
							Env: spec.Env,
						},
					},
				},
			},
		},
	}

	return job
}

// GetDiscoveryJob fetches the discovery job for a template
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

	// Return the most recent job
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
// For now, this returns mocked data. In production, this would read pod logs.
func ParseDiscoveryLogs(ctx context.Context, k8sClient client.Client, job *batchv1.Job) (*ParsedDiscovery, error) {
	// TODO: In production, fetch pod logs and parse JSON output
	// For now, return mocked discovery result

	if !IsJobSucceeded(job) {
		return nil, fmt.Errorf("job has not succeeded yet")
	}

	// Mocked result matching the expected format
	mockedResult := DiscoveryResult{
		Filename: "vllm-mi300x-fp8-tp1-latency.yaml",
		Profile: Profile{
			Model:          "meta-llama/Llama-3.1-8B-Instruct",
			QuantizedModel: "amd/Llama-3.1-8B-Instruct-FP8-KV",
			Metadata: ProfileMetadata{
				Engine:    "vllm",
				GPU:       "MI300X",
				Precision: "fp8",
				GPUCount:  1,
				Metric:    "latency",
			},
			EngineArgs: map[string]any{
				"gpu-memory-utilization":       0.95,
				"distributed_executor_backend": "mp",
				"no-enable-chunked-prefill":    nil,
				"tensor-parallel-size":         1,
			},
			EnvVars: map[string]string{
				"VLLM_DO_NOT_TRACK":           "1",
				"VLLM_USE_V1":                 "0",
				"VLLM_USE_TRITON_FLASH_ATTN":  "0",
				"HIP_FORCE_DEV_KERNARG":       "1",
				"NCCL_MIN_NCHANNELS":          "112",
				"TORCH_BLAS_PREFER_HIPBLASLT": "1",
				"PYTORCH_TUNABLEOP_ENABLED":   "1",
				"PYTORCH_TUNABLEOP_VERBOSE":   "1",
				"PYTORCH_TUNABLEOP_TUNING":    "0",
			},
			Models: []ProfileModel{
				{
					Name:   "amd/Llama-3.1-8B-Instruct-FP8-KV",
					Source: "hf://amd/Llama-3.1-8B-Instruct-FP8-KV",
					SizeGB: 8.47,
				},
			},
		},
	}

	// Convert profile to JSON
	profileBytes, err := json.Marshal(mockedResult.Profile)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal profile: %w", err)
	}

	profileJSON := &apiextensionsv1.JSON{Raw: profileBytes}

	// Extract model sources from profile
	var modelSources []aimv1alpha1.AIMModelSource
	for _, model := range mockedResult.Profile.Models {
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

// MockDiscoveryLogsFromTemplate creates mocked discovery output based on template
func MockDiscoveryLogsFromTemplate(modelID string) string {
	result := DiscoveryResult{
		Filename: "mock-profile.yaml",
		Profile: Profile{
			Model:          modelID,
			QuantizedModel: modelID,
			Metadata: ProfileMetadata{
				Engine:    "vllm",
				GPU:       "MI300X",
				Precision: "fp16",
				GPUCount:  1,
				Metric:    "latency",
			},
			EngineArgs: map[string]any{
				"gpu-memory-utilization": 0.9,
			},
			EnvVars: map[string]string{},
			Models: []ProfileModel{
				{
					Name:   modelID,
					Source: fmt.Sprintf("hf://%s", modelID),
					SizeGB: 16,
				},
			},
		},
	}

	data, _ := json.Marshal(result)
	return string(data)
}
