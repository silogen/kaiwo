/**
 * Copyright 2025 Advanced Micro Devices, Inc. All rights reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
**/

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

	// Fetch flags
	execFlags := GetExecFlags()
	metaFlags := GetMetaFlags()

	// Generate workload configuration
	workloadConfig, err := workload.GenerateTemplateContext(execFlags)
	if err != nil {
		return fmt.Errorf("error generating workload config: %w", err)
	}

	// Load custom configuration, if provided
	var customConfig any
	if execFlags.CustomConfigPath != "" {
		customConfig, err = loadCustomConfig(execFlags.CustomConfigPath)
		if err != nil {
			return fmt.Errorf("error loading custom config: %w", err)
		}
	}

	// Finalize metadata flags
	if metaFlags.Name == "" {
		metaFlags.Name = makeWorkloadName(execFlags.Path, metaFlags.Image)
		logrus.Infof("No explicit name provided, using name: %s", metaFlags.Name)
	}

	// Parse environment variables
	envFilePath := filepath.Join(execFlags.Path, workloads.EnvFilename)
	if err := parseEnvFile(envFilePath, &metaFlags); err != nil {
		return fmt.Errorf("error parsing environment: %w", err)
	}

	// Prepare scheduling flags
	dynamicClient, err := k8s.GetDynamicClient()
	if err != nil {
		return fmt.Errorf("error fetching Kubernetes client: %w", err)
	}

	ctx := context.TODO()
	schedulingFlags := GetSchedulingFlags()
	if err := fillSchedulingFlags(ctx, dynamicClient, &schedulingFlags, execFlags.ResourceFlavorGpuNodeLabelKey); err != nil {
		return fmt.Errorf("error filling scheduling flags: %w", err)
	}
	logrus.Infof("Successfully loaded scheduling info from Kubernetes")

	// Create the workload template context
	templateContext := workloads.WorkloadTemplateConfig{
		WorkloadMeta: workloadMeta,
		Workload:     workloadConfig,
		Meta:         metaFlags,
		Scheduling:   schedulingFlags,
		Custom:       customConfig,
	}

	// Apply the workload
	if err := workloads.ApplyWorkload(ctx, dynamicClient, workload, execFlags, templateContext); err != nil {
		return fmt.Errorf("error applying workload: %w", err)
	}

	return nil
}

// loadCustomConfig loads custom configuration data from a file
func loadCustomConfig(path string) (any, error) {
	logrus.Debugln("Loading custom config")
	customConfigContents, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read custom config file: %w", err)
	}

	customConfig, err := yaml.Marshal(customConfigContents)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal custom config file: %w", err)
	}

	return customConfig, nil
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
