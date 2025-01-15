package ray

import (
	"context"
	_ "embed"
	"fmt"
	"github.com/silogen/ai-workload-orchestrator/pkg/k8s"
	"github.com/silogen/ai-workload-orchestrator/pkg/utils"
	"github.com/silogen/ai-workload-orchestrator/pkg/workloads"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"os"
	"path/filepath"
)

type Deployment struct {
	Shared     workloads.SharedFlags
	Deployment workloads.DeploymentFlags
}

//go:embed service.yaml.tmpl
var ServiceTemplate []byte

const ServeconfigFilename = "serveconfig"

func (service Deployment) GenerateTemplateContext() (any, error) {
	logrus.Debugf("Loading ray service from %s", service.Shared.Path)

	contents, err := os.ReadFile(filepath.Join(service.Shared.Path, ServeconfigFilename))

	if err != nil {
		return nil, fmt.Errorf("failed to read serveconfig file: %w", err)
	}

	client, err := k8s.GetDynamicClient()

	if err != nil {
		return nil, err
	}

	logrus.Debug("Fetching GPU count")
	// TODO make labelKey dynamic
	gpuCount, err := k8s.GetDefaultResourceFlavorGpuCount(context.TODO(), client, "beta.amd.com/gpu.family.AI")

	if err != nil {
		return nil, err
	}

	logrus.Debugf("Fetched GPU count: %d", gpuCount)

	numReplicas, nodeGpuRequest := k8s.CalculateNumberOfReplicas(service.Shared.GPUs, gpuCount)

	templateContext := struct {
		Shared      workloads.SharedFlags
		Job         workloads.DeploymentFlags
		Kueue       k8s.KueueArgs
		Serveconfig string
	}{
		Shared:      service.Shared,
		Job:         service.Deployment,
		Serveconfig: string(contents),
		Kueue: k8s.KueueArgs{
			GPUsAvailablePerNode:    gpuCount,
			RequestedGPUsPerReplica: nodeGpuRequest,
			RequestedNumReplicas:    numReplicas,
		},
	}
	return templateContext, nil
}

func (service Deployment) DefaultTemplate() ([]byte, error) {
	if ServiceTemplate == nil {
		return nil, fmt.Errorf("job template is empty")
	}
	return ServiceTemplate, nil
}

func (service Deployment) GenerateName() string {
	return utils.BuildWorkloadName(service.Shared.Name, service.Shared.Path, service.Deployment.Image)
}

func (service Deployment) IgnoreFiles() []string {
	return []string{ServeconfigFilename, utils.KaiwoconfigFilename}
}

func (service Deployment) GetPods() ([]corev1.Pod, error) {
	return []corev1.Pod{}, nil
}

func (service Deployment) GetServices() ([]corev1.Service, error) {
	return []corev1.Service{}, nil
}

func (service Deployment) AdditionalResources(resources *[]*unstructured.Unstructured) error {
	return nil
}
