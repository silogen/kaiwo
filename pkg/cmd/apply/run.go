package cli

import (
	"context"
	"fmt"
	"github.com/silogen/kaiwo/pkg/k8s"
	"github.com/silogen/kaiwo/pkg/workloads"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"
	"os"
	"os/user"
	"path/filepath"
	"sigs.k8s.io/yaml"
	"strings"
)

// RunApply prepares the workload and applies it
func RunApply(workload workloads.Workload, workloadMeta any) error {
	logrus.Debugln("Applying workload")

	// Fetch all CLI flags to generate the template context

	execFlags := GetExecFlags()
	workloadConfig, err := workload.GenerateTemplateContext(execFlags)
	if err != nil {
		return fmt.Errorf("failed to generate template context: %w", err)
	}

	var customConfig any = nil

	// Load custom config data, if any
	if execFlags.CustomConfigPath != "" {
		logrus.Debugln("Loading custom config")
		customConfigContents, err := os.ReadFile(execFlags.CustomConfigPath)
		if err != nil {
			return fmt.Errorf("failed to read custom config file: %w", err)
		}
		customConfig, err = yaml.Marshal(customConfigContents)
		if err != nil {
			return fmt.Errorf("failed to marshal custom config file: %w", err)
		}
	} else {
		logrus.Debugln("No custom config to load")
	}

	metaFlags := GetMetaFlags()

	if metaFlags.Name == "" {
		metaFlags.Name = makeWorkloadName(execFlags.Path, metaFlags.Image)
		logrus.Infof("No explicit name provided, using name: %s", metaFlags.Name)
	}

	// Load env vars
	logrus.Debugln("Attempting to read env file")
	envFilePath := filepath.Join(execFlags.Path, workloads.EnvFilename)
	if err := parseEnvFile(envFilePath, &metaFlags); err != nil {
		return fmt.Errorf("failed to parse env file: %w", err)
	}

	schedulingFlags := GetSchedulingFlags()

	dynamicClient, err := k8s.GetDynamicClient()

	if err != nil {
		return fmt.Errorf("error fetching Kubernetes client: %v", err)
	}

	ctx := context.TODO()

	logrus.Debugf("Loading scheduling info from Kubernetes...")
	if err := fillSchedulingFlags(ctx, dynamicClient, &schedulingFlags, execFlags.ResourceFlavorGpuNodeLabelKey); err != nil {
		return fmt.Errorf("error filling scheduling flags: %v", err)
	}
	logrus.Infof("Succesfully loaded scheduling info from Kubernetes")

	templateContext := workloads.WorkloadTemplateConfig{
		WorkloadMeta: workloadMeta,
		Workload:     workloadConfig,
		Meta:         metaFlags,
		Scheduling:   schedulingFlags,
		Custom:       customConfig,
	}

	if err := workloads.ApplyWorkload(ctx, dynamicClient, workload, execFlags, templateContext); err != nil {
		return fmt.Errorf("error applying workload: %v", err)
	}

	return nil
}

func makeWorkloadName(path string, image string) string {
	currentUser, err := user.Current()
	if err != nil {
		panic(fmt.Sprintf("Failed to fetch the current user: %v", err))
	}

	var appendix string

	if path != "" {
		appendix = sanitizeStringForKubernetes(filepath.Base(path))
	} else {
		appendix = sanitizeStringForKubernetes(image)
	}
	return strings.Join([]string{currentUser.Username, appendix}, "-")

}

func sanitizeStringForKubernetes(path string) string {
	replacer := strings.NewReplacer(
		":", "-",
		"/", "-",
		"\\", "-",
		"_", "-",
		".", "-",
	)
	return strings.ToLower(replacer.Replace(path))
}

// fillSchedulingFlags fills in the GPU scheduling flags based on the Kubernetes cluster state
func fillSchedulingFlags(
	ctx context.Context,
	client dynamic.Interface,
	schedulingFlags *workloads.SchedulingFlags,
	resourceFlavorGpuNodeLabelKey string,
) error {
	gpuCount, err := k8s.GetDefaultResourceFlavorGpuCount(ctx, client, resourceFlavorGpuNodeLabelKey)

	schedulingFlags.GPUsAvailablePerNode = gpuCount

	if err != nil {
		return err
	}

	numReplicas, nodeGpuRequest := k8s.CalculateNumberOfReplicas(schedulingFlags.TotalRequestedGPUs, gpuCount, nil)

	schedulingFlags.CalculatedNumReplicas = numReplicas
	schedulingFlags.CalculatedGPUsPerReplica = nodeGpuRequest

	return nil
}

// parseEnvFile parses values from an environmental file and adds them to the meta flags
func parseEnvFile(envFilePath string, flags *workloads.MetaFlags) error {
	var envVars []corev1.EnvVar
	var secretVolumes []k8s.SecretVolume
	if _, err := os.Stat(envFilePath); err == nil {
		logrus.Infof("Found env file at %s, parsing environment variables and secret volumes", envFilePath)
		envVars, secretVolumes, err = k8s.ReadEnvFile(envFilePath)
		if err != nil {
			return fmt.Errorf("failed to parse env file: %w", err)
		}
		logrus.Infof("Parsed %d environment variables and %d secret volumes from env file", len(envVars), len(secretVolumes))
		flags.EnvVars = envVars
		flags.SecretVolumes = secretVolumes
	}
	return nil
}
