package workloads

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type Workload interface {

	// GenerateTemplateContext creates the workload-specific context that can be passed to the template
	GenerateTemplateContext() (any, error)

	// GenerateName provides a name that can be used to describe the workload
	GenerateName() string

	// DefaultTemplate returns a default template to use for this workload
	DefaultTemplate() ([]byte, error)

	// IgnoreFiles lists the files that should be ignored in the ConfigMap
	IgnoreFiles() []string

	// AdditionalResources adds additional resources needed by the workload
	AdditionalResources(resources *[]*unstructured.Unstructured) error

	// GetPods returns a list of pods that are associated with this workload
	GetPods() ([]corev1.Pod, error)

	// GetServices returns a list of services that are associated with this workload
	GetServices() ([]corev1.Service, error)
}

// ExecFlags contain flags that are shared by all workloads during the resource-creation process
type ExecFlags struct {
	// Run without modifying resources
	DryRun bool

	// Create namespace if it doesn't already exist
	CreateNamespace bool
}

// SharedFlags contain flags that are shared by all workloads
type SharedFlags struct {
	// The name of the resource
	Name string

	// The namespace of the resource
	Namespace string

	// Number of GPUs requested
	GPUs int

	// Custom template, if any
	Template string

	// Path to workload folder
	Path string
}

// JobFlags contain flags specific to job-workloads
type JobFlags struct {
	// The base image to use
	Image string

	// The Kueue queue to use
	Queue string

	// Cleanup finished jobs after minutes
	TtlMinAfterFinished int
}

// DeploymentFlags contain flags specific to deployment-workloads
type DeploymentFlags struct {
	// The base image to use
	Image string

	// The model to use
	Model string
}
