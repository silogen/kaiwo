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
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/silogen/kaiwo/pkg/workloads"
)

const defaultGpuNodeLabelKey = "beta.amd.com/gpu.family.AI"

// Exec flags
var (
	dryRun           bool
	createNamespace  bool
	template         string
	path             string
	gpuNodeLabelKey  string
	customConfigPath string
	envFilePath      string
)

// AddExecFlags adds flags that are needed for the execution of apply functions
func AddExecFlags(cmd *cobra.Command) {
	cmd.Flags().BoolVarP(&createNamespace, "create-namespace", "", false, "Create namespace if it does not exist")
	cmd.Flags().BoolVarP(&dryRun, "dry-run", "d", false, "Print the generated workload manifest without submitting it")
	cmd.Flags().StringVarP(&path, "path", "p", "", "Path to directory for workload code, entrypoint/serveconfig, env-file, etc. Either image or path is mandatory")
	cmd.Flags().StringVarP(&gpuNodeLabelKey, "gpu-node-label-key", "", defaultGpuNodeLabelKey, fmt.Sprintf("Optional node label key used to specify the resource flavor GPU count. Defaults to %s", defaultGpuNodeLabelKey))
	cmd.Flags().StringVarP(&template, "template", "t", "", "Optional path to a custom template to use for the workload. If not provided, a default template will be used unless template file found in workload directory")
	cmd.Flags().StringVarP(&customConfigPath, "custom-config", "c", "", "Optional path to a custom YAML configuration file whose contents are made available in the template")
	cmd.Flags().StringVarP(&envFilePath, "env-file", "", "", "Optional path to env file. Defaults to 'env' in workload code directory")
}

func GetExecFlags() workloads.ExecFlags {
	return workloads.ExecFlags{
		CreateNamespace:               createNamespace,
		DryRun:                        dryRun,
		Template:                      template,
		Path:                          path,
		ResourceFlavorGpuNodeLabelKey: gpuNodeLabelKey,
		CustomConfigPath:              customConfigPath,
		EnvFilePath:                   envFilePath,
	}
}

// Kubernetes meta flags

const (
	defaultNamespace = "kaiwo"
	defaultImage     = "ghcr.io/silogen/rocm-ray:v0.5"
)

var (
	name            string
	namespace       string
	image           string
	imagePullSecret string
	version         string
)

// AddMetaFlags adds flags that are needed for basic Kubernetes metadata
func AddMetaFlags(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&name, "name", "", "", "Optional name for the workload")
	cmd.Flags().StringVarP(&namespace, "namespace", "n", defaultNamespace, fmt.Sprintf("Namespace of the workload. Defaults to %s", defaultNamespace))
	cmd.Flags().StringVarP(&image, "image", "i", defaultImage, fmt.Sprintf("Optional Image to use for the workload. Defaults to %s. Either image or workload path is mandatory", defaultImage))
	cmd.Flags().StringVarP(&imagePullSecret, "imagepullsecret", "", "", "ImagePullSecret name for job/deployment if private registry")
	cmd.Flags().StringVarP(&version, "version", "", "", "Optional version for job/deployment")
}

func GetMetaFlags() workloads.MetaFlags {
	return workloads.MetaFlags{
		Name:            name,
		Namespace:       namespace,
		Image:           image,
		ImagePullSecret: imagePullSecret,
		Version:         version,
	}
}

// Scheduling flags

var (
	gpus           int
	replicas       int
	gpusPerReplica int
	storage        string
	noStorage      bool
)

const (
	defaultStorageAmount = "10Gi"
)

// AddSchedulingFlags adds flags related to (Kueue) scheduling
func AddSchedulingFlags(cmd *cobra.Command) {
	cmd.Flags().IntVarP(&gpus, "gpus", "g", 0, "Number of GPUs requested for the workload")
	cmd.Flags().IntVarP(&replicas, "replicas", "", 0, "Number of replicas requested for the workload")
	cmd.Flags().IntVarP(&gpusPerReplica, "gpus-per-replica", "", 0, "Number of GPUs requested per replica")
	cmd.Flags().StringVarP(
		&storage,
		"storage",
		"",
		"",
		"Storage requested for the workload, use: --storage=10Gi,storageClassName or --storage=10Gi to use the default storage class. "+
			fmt.Sprintf("If not provided, --storage=%s is implied. If you do not want to include storage, use the --no-storage flag", defaultStorageAmount),
	)
	cmd.Flags().BoolVarP(&noStorage, "no-storage", "", false, "Don't use storage for the workload")
}

// GetSchedulingFlags initializes the scheduling flags with the number of GPUs requested
func GetSchedulingFlags() (*workloads.SchedulingFlags, error) {
	flags := &workloads.SchedulingFlags{
		TotalRequestedGPUs:      gpus,
		RequestedReplicas:       replicas,
		RequestedGPUsPerReplica: gpusPerReplica,
	}

	if noStorage {
		logrus.Info("No storage (PVC) requested for workload")
		return flags, nil
	}

	requestedStorage := defaultStorageAmount
	storageClassName := ""

	split := strings.Split(storage, ",")

	if len(split) > 2 {
		return nil, fmt.Errorf("invalid storage specifier %s", storage)
	}
	if len(split) > 1 {
		storageClassName = split[1]
		logrus.Infof("Requested storage class name %s", storageClassName)
	} else {
		logrus.Info("You did not pass a storage class name, the default storage class will be used if it exists")
	}
	if len(split) > 0 {
		requestedStorage = split[0]

		if _, err := resource.ParseQuantity(requestedStorage); err != nil {
			return nil, fmt.Errorf("invalid storage quantity %s", requestedStorage)
		}

		logrus.Infof("Requested storage %s", requestedStorage)
	} else {
		logrus.Infof("You did not pass a storage quantity, the default amount (%s) will be used", requestedStorage)
	}

	flags.Storage = &workloads.StorageSchedulingFlags{
		RequestedStorage: requestedStorage,
		StorageClassName: storageClassName,
	}

	return flags, nil
}

type Config struct {
	DryRun                  bool   `yaml:"dryRun"`
	CreateNamespace         bool   `yaml:"createNamespace"`
	Path                    string `yaml:"path"`
	GpuNodeLabelKey         string `yaml:"gpuNodeLabelKey"`
	Template                string `yaml:"template"`
	CustomConfig            string `yaml:"customConfig"`
	EnvFile                 string `yaml:"envFile"`
	Name                    string `yaml:"name"`
	Namespace               string `yaml:"namespace"`
	Image                   string `yaml:"image"`
	ImagePullSecret         string `yaml:"imagePullSecret"`
	Version                 string `yaml:"version"`
	Gpus                    int    `yaml:"gpus"`
	RequestedReplicas       int    `yaml:"requestedReplicas"`
	RequestedGPUsPerReplica int    `yaml:"requestedGPUsPerReplica"`
}

func LoadConfigFromPath(path string) (*Config, error) {
	configPath := filepath.Join(path, workloads.KaiwoconfigFilename)
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", configPath, err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %w", configPath, err)
	}

	return &config, nil
}

func ApplyConfigToFlags(cmd *cobra.Command, config *Config) {
	if config == nil {
		return
	}
	setFlag := func(name, value string) {
		if value != "" {
			if err := cmd.Flags().Set(name, value); err != nil {
				logrus.Errorf("Failed to set flag %s: %v", name, err)
			}
		}
	}

	// ExecFlags
	setFlag("dry-run", fmt.Sprintf("%v", config.DryRun))
	setFlag("create-namespace", fmt.Sprintf("%v", config.CreateNamespace))
	setFlag("path", config.Path)
	setFlag("gpu-node-label-key", config.GpuNodeLabelKey)
	setFlag("template", config.Template)
	setFlag("custom-config", config.CustomConfig)
	setFlag("env-file", config.EnvFile)

	// MetaFlags
	setFlag("name", config.Name)
	setFlag("namespace", config.Namespace)
	setFlag("image", config.Image)
	setFlag("imagepullsecret", config.ImagePullSecret)
	setFlag("version", config.Version)

	// SchedulingFlags
	setFlag("gpus", fmt.Sprintf("%d", config.Gpus))
	setFlag("replicas", fmt.Sprintf("%d", config.RequestedReplicas))
	setFlag("gpus-per-replica", fmt.Sprintf("%d", config.RequestedGPUsPerReplica))
}

func PreRunLoadConfig(cmd *cobra.Command, _ []string) error {
	if path == "" {
		return nil
	}

	config, err := LoadConfigFromPath(path)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if config != nil {
		ApplyConfigToFlags(cmd, config)
		logrus.Infof("Configuration loaded from %s", filepath.Join(path, workloads.KaiwoconfigFilename))
	} else {
		logrus.Debugf("No configuration file found in %s", path)
	}

	return nil
}
