package kueue

import (
	_ "embed"
	"fmt"
	"github.com/silogen/ai-workload-orchestrator/pkg/k8s"
	"github.com/silogen/ai-workload-orchestrator/pkg/utils"
	"github.com/silogen/ai-workload-orchestrator/pkg/workloads"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"os"
	"path/filepath"
	"strings"
)

//go:embed job.yaml.tmpl
var JobTemplate []byte

const EntrypointFilename = "entrypoint"

type Job struct {
	Shared     workloads.SharedFlags
	Job        workloads.JobFlags
	Entrypoint string
}

func (job Job) GenerateTemplateContext() (any, error) {

	contents, err := os.ReadFile(filepath.Join(job.Shared.Path, EntrypointFilename))

	if contents == nil {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("failed to read entrypoint file: %w", err)
	}

	job.Entrypoint = strings.ReplaceAll(string(contents), "\n", " ")  // Flatten multiline string
	job.Entrypoint = strings.ReplaceAll(job.Entrypoint, "\"", "\\\"") // Escape double quotes
	job.Entrypoint = fmt.Sprintf("\"%s\"", job.Entrypoint)            // Wrap the entire command in quotes

	job.Shared.Name = job.GenerateName()

	context := struct {
		Shared     workloads.SharedFlags
		Job        workloads.JobFlags
		Entrypoint string
	}{
		Shared:     job.Shared,
		Job:        job.Job,
		Entrypoint: job.Entrypoint,
	}
	return context, nil
}

func (job Job) DefaultTemplate() ([]byte, error) {
	if JobTemplate == nil {
		return nil, fmt.Errorf("job template is empty")
	}
	return JobTemplate, nil
}

func (job Job) GenerateName() string {
	return utils.BuildWorkloadName(job.Shared.Name, job.Shared.Path, job.Job.Image)
}

func (job Job) IgnoreFiles() []string {
	return []string{EntrypointFilename, utils.KaiwoconfigFilename}
}

func (job Job) GetPods() ([]corev1.Pod, error) {
	return []corev1.Pod{}, nil
}

func (job Job) GetServices() ([]corev1.Service, error) {
	return []corev1.Service{}, nil
}

func (job Job) AdditionalResources(resources *[]*unstructured.Unstructured) error {
	c, err := k8s.GetDynamicClient()
	if err != nil {
		return err
	}

	// Handle kueue local queue
	localQueue, err := k8s.PrepareLocalClusterQueue(job.Job.Queue, job.Shared.Namespace, c)
	if err != nil {
		return err
	}

	*resources = append(*resources, localQueue)

	return nil
}
