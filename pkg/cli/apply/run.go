// Copyright 2025 Advanced Micro Devices, Inc.  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/silogen/kaiwo/pkg/k8s"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
	"github.com/silogen/kaiwo/pkg/workloads"
)

// RunApply prepares the workload and applies it
func RunApply(workload workloads.Workload, workloadMeta any) error {
	logrus.Debugln("Applying workload")

	// Fetch flags
	execFlags := GetExecFlags()
	metaFlags := GetMetaFlags()

	if execFlags.Path == "" && metaFlags.Image == defaultImage {
		logrus.Error("Cannot run workload without image or path")
		return nil
	}

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

	metaFlags.User, err = baseutils.GetCurrentUser()
	if err != nil {
		return fmt.Errorf("failed to fetch the current user: %v", err)
	}

	if metaFlags.Name == "" {
		metaFlags.Name = makeWorkloadName(execFlags.Path, metaFlags.Image, metaFlags.Version, metaFlags.User)
		logrus.Debugf("No explicit name provided, using name: %s", metaFlags.Name)
	}

	// Parse environment variables
	if execFlags.EnvFilePath == "" {
		envFilePath = filepath.Join(execFlags.Path, workloads.EnvFilename)
	} else {
		envFilePath = execFlags.EnvFilePath
	}

	if err := parseEnvFile(envFilePath, &metaFlags); err != nil {
		return fmt.Errorf("error parsing environment: %w", err)
	}

	// Prepare scheduling flags
	clients, err := k8s.GetKubernetesClients()
	if err != nil {
		return fmt.Errorf("error getting k8s clients: %w", err)
	}

	ctx := context.TODO()
	schedulingFlags, err := GetSchedulingFlags()
	if err != nil {
		return fmt.Errorf("error getting scheduling flags: %w", err)
	}

	if schedulingFlags.Storage != nil {
		if schedulingFlags.Storage.StorageClassName == "" || schedulingFlags.Storage.Quantity == "" {
			logrus.Info("Storage requested but storage class name and / or quantity not provided, checking namespace labels for defaults")

			defaultStorageFlags, err := findDefaultStorageFlags(ctx, *clients.Clientset, metaFlags.Namespace)
			if err != nil {
				return fmt.Errorf("error checking for storage defaults: %w", err)
			}

			if schedulingFlags.Storage.StorageClassName == "" {
				if defaultStorageFlags.StorageClassName == "" {
					return fmt.Errorf("storage requested, but no storage class name provided and no default exists in the namespace label '%s'", workloads.KaiwoDefaultStorageClassNameLabel)
				}
				schedulingFlags.Storage.StorageClassName = defaultStorageFlags.StorageClassName
			}
			if schedulingFlags.Storage.Quantity == "" {
				if defaultStorageFlags.Quantity == "" {
					return fmt.Errorf("storage requested, but no quantity provided and no default exists in the namespace label '%s'", workloads.KaiwoDefaultStorageQuantityLabel)
				}
				schedulingFlags.Storage.Quantity = defaultStorageFlags.Quantity
			}
		}

		storageClassExists, err := doesStorageClassExist(ctx, *clients.Clientset, schedulingFlags.Storage.StorageClassName)
		if err != nil {
			return fmt.Errorf("error checking if storage class exists: %w", err)
		}
		if !storageClassExists {
			logrus.Warnf("Requested storage class '%s' does not exist, applying this workload will likely fail", schedulingFlags.Storage.StorageClassName)
		}
	}

	if err := fillSchedulingFlags(ctx, clients.Client, schedulingFlags, execFlags.ResourceFlavorGpuNodeLabelKey, metaFlags.EnvVars); err != nil {
		return fmt.Errorf("error filling scheduling flags: %w", err)
	}
	logrus.Debugf("Successfully loaded scheduling info from Kubernetes")

	metaFlags.EnvVars = append(metaFlags.EnvVars, corev1.EnvVar{Name: "NUM_GPUS", Value: strconv.Itoa(schedulingFlags.TotalRequestedGPUs)})
	metaFlags.EnvVars = append(metaFlags.EnvVars, corev1.EnvVar{Name: "NUM_REPLICAS", Value: strconv.Itoa(schedulingFlags.CalculatedNumReplicas)})
	metaFlags.EnvVars = append(metaFlags.EnvVars, corev1.EnvVar{Name: "NUM_GPUS_PER_REPLICA", Value: strconv.Itoa(schedulingFlags.CalculatedGPUsPerReplica)})

	// Create the workload template context
	templateContext := workloads.WorkloadTemplateConfig{
		WorkloadMeta: workloadMeta,
		Workload:     workloadConfig,
		Meta:         metaFlags,
		Scheduling:   *schedulingFlags,
		Custom:       customConfig,
	}

	// Apply the workload
	if err := workloads.ApplyWorkload(ctx, clients.Client, workload, execFlags, templateContext); err != nil {
		return fmt.Errorf("error applying workload: %w", err)
	}

	return nil
}

func findDefaultStorageFlags(ctx context.Context, clientset kubernetes.Clientset, namespace string) (*workloads.StorageSchedulingFlags, error) {
	namespaceObject, err := clientset.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			logrus.Warnf("Namespace does not exist, cannot check for storage defaults. Either ensure that the namespace exists and has default values, specify the storage class and amount explicitly, or specify --no-storage to skip adding storage.")
			return nil, fmt.Errorf("failed to find default storage class or quantity for namespace that does not exist: %s", namespace)
		}
		return nil, fmt.Errorf("error getting namespace: %w", err)
	}

	flags := &workloads.StorageSchedulingFlags{}

	defaultStorageClassName, ok := namespaceObject.Labels[workloads.KaiwoDefaultStorageClassNameLabel]
	if ok {
		logrus.Debugf("Default storage class discovered: %s", defaultStorageClassName)
		flags.StorageClassName = defaultStorageClassName
	} else {
		logrus.Debugf("Default storage class not found")
	}
	defaultStorageQuantity, ok := namespaceObject.Labels[workloads.KaiwoDefaultStorageQuantityLabel]
	if ok {
		logrus.Debugf("Default storage quantity discovered: %s", defaultStorageQuantity)
		flags.Quantity = defaultStorageQuantity
	} else {
		logrus.Debugf("Default storage quantity not found")
	}

	return flags, nil
}

func doesStorageClassExist(ctx context.Context, clientset kubernetes.Clientset, storageClassName string) (bool, error) {
	_, err := clientset.StorageV1().StorageClasses().Get(ctx, storageClassName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("error getting storage class %s: %w", storageClassName, err)
	}
	return true, nil
}

// loadCustomConfig loads custom configuration data from a file
func loadCustomConfig(path string) (any, error) {
	logrus.Debugln("Loading custom config")
	customConfigContents, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read custom config file: %w", err)
	}

	customConfig := make(map[string]any)

	if err := yaml.Unmarshal(customConfigContents, &customConfig); err != nil {
		return nil, fmt.Errorf("failed to marshal custom config file: %w", err)
	}

	return customConfig, nil
}

func makeWorkloadName(path string, image string, version string, currentUser string) string {
	var appendix string

	if path != "" {
		appendix = sanitizeStringForKubernetes(filepath.Base(path))
	} else {
		appendix = sanitizeStringForKubernetes(image)
	}

	// Calculate the max allowed length for the appendix
	separatorCount := 1 // At least one "-" between username and appendix
	if version != "" {
		version = sanitizeStringForKubernetes(version)
		separatorCount = 2 // Include one more "-" for the version
	}
	maxAppendixLength := 45 - len(currentUser) - len(version) - separatorCount

	// Truncate appendix if necessary
	if len(appendix) > maxAppendixLength {
		appendix = appendix[:maxAppendixLength]
	}

	// Combine components
	components := []string{currentUser, appendix}
	if version != "" {
		components = append(components, version)
	}

	return strings.Join(components, "-")
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
	client client.Client,
	schedulingFlags *workloads.SchedulingFlags,
	resourceFlavorGpuNodeLabelKey string,
	envVars []corev1.EnvVar,
) error {
	logrus.Debugf("Connecting to Kubernetes cluster to fetch resource flavor")
	gpuCount, err := k8s.GetDefaultResourceFlavorGpuCount(ctx, client, resourceFlavorGpuNodeLabelKey)
	logrus.Debugf("Found a resource flavor with %d GPUs per node", gpuCount)
	schedulingFlags.GPUsAvailablePerNode = gpuCount

	if err != nil {
		return err
	}

	if schedulingFlags.RequestedReplicas > 0 && schedulingFlags.RequestedGPUsPerReplica > 0 {
		if schedulingFlags.RequestedGPUsPerReplica > schedulingFlags.GPUsAvailablePerNode {
			return fmt.Errorf("you requested %d GPUs per replica, but there are only %d GPUs available per node",
				schedulingFlags.RequestedGPUsPerReplica, schedulingFlags.GPUsAvailablePerNode)
		}
		if schedulingFlags.TotalRequestedGPUs > 0 {
			return fmt.Errorf("cannot set requested gpus with --gpus when --replicas and --gpus-per-replica are set")
		}
		schedulingFlags.CalculatedNumReplicas = schedulingFlags.RequestedReplicas
		schedulingFlags.CalculatedGPUsPerReplica = schedulingFlags.RequestedGPUsPerReplica
		schedulingFlags.TotalRequestedGPUs = schedulingFlags.RequestedReplicas * schedulingFlags.RequestedGPUsPerReplica
		return nil
	}

	numReplicas, nodeGpuRequest := k8s.CalculateNumberOfReplicas(schedulingFlags.TotalRequestedGPUs, gpuCount, envVars)
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
