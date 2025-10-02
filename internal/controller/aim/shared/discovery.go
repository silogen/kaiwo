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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
)

// ModelSource represents a discovered model source
type ModelSource struct {
	SourceURI string `json:"sourceUri"`
	Size      string `json:"size"`
}

// DiscoveryResult represents the output from a discovery job
type DiscoveryResult struct {
	ModelSources []ModelSource `json:"modelSources"`
}

// DiscoveryJobSpec defines parameters for creating a discovery job
type DiscoveryJobSpec struct {
	TemplateName string
	Namespace    string
	ModelID      string
	Image        string
	Env          []corev1.EnvVar
	OwnerRef     metav1.OwnerReference
}

// BuildDiscoveryJob creates a Job that runs model discovery dry-run
func BuildDiscoveryJob(spec DiscoveryJobSpec) *batchv1.Job {
	// Create deterministic job name with hash of key parameters
	hash := sha256.Sum256([]byte(spec.ModelID + spec.Image))
	jobName := fmt.Sprintf("discover-%s-%x", spec.TemplateName, hash[:4])

	backoffLimit := int32(3)
	ttlSeconds := int32(300) // Clean up after 5 minutes (enough time to fetch status)

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
					RestartPolicy: corev1.RestartPolicyNever,
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

// ParseDiscoveryLogs parses the discovery job output to extract model sources.
// For now, this returns mocked data. In production, this would read pod logs.
func ParseDiscoveryLogs(ctx context.Context, k8sClient client.Client, job *batchv1.Job) ([]aimv1alpha1.AIMModelSource, error) {
	// TODO: In production, fetch pod logs and parse JSON output
	// For now, return mocked discovery result

	if !IsJobSucceeded(job) {
		return nil, fmt.Errorf("job has not succeeded yet")
	}

	// Mocked result
	mockedResult := DiscoveryResult{
		ModelSources: []ModelSource{
			{
				SourceURI: "hf://meta/llama-3-8b",
				Size:      "16Gi",
			},
		},
	}

	// Convert to API type
	var modelSources []aimv1alpha1.AIMModelSource
	for _, ms := range mockedResult.ModelSources {
		size, err := resource.ParseQuantity(ms.Size)
		if err != nil {
			return nil, fmt.Errorf("failed to parse size %q: %w", ms.Size, err)
		}
		modelSources = append(modelSources, aimv1alpha1.AIMModelSource{
			SourceURI: ms.SourceURI,
			Size:      size,
		})
	}

	return modelSources, nil
}

// MockDiscoveryLogsFromTemplate creates mocked discovery output based on template
func MockDiscoveryLogsFromTemplate(modelID string) string {
	result := DiscoveryResult{
		ModelSources: []ModelSource{
			{
				SourceURI: fmt.Sprintf("hf://%s", modelID),
				Size:      "16Gi",
			},
		},
	}

	data, _ := json.Marshal(result)
	return string(data)
}
