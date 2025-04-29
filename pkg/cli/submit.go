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

package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"github.com/charmbracelet/huh"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cliutils "github.com/silogen/kaiwo/pkg/cli/utils"
	k8sUtils "github.com/silogen/kaiwo/pkg/k8s"
)

var (
	queue     string
	user      string
	namespace string

	config     string
	file       string
	emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
)

const (
	promptAccessible = false
)

func BuildSubmitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "submit",
		Short: "Submit a job or service",
		Long:  "Submit a Kaiwo Job or a Kaiwo Service",
		RunE: func(cmd *cobra.Command, args []string) error {
			if file == "" {
				return fmt.Errorf("--file is required")
			}
			obj, err := readManifest(file)
			if err != nil {
				return fmt.Errorf("failed to read manifest: %v", err)
			}

			existingKaiwoCliConfigPath, err := cliutils.GetKaiwoCliConfigPath(config)
			if err != nil {
				return fmt.Errorf("failed to get Kaiwo config path: %v", err)
			}

			var kaiwoConfig cliutils.KaiwoCliConfig

			configPath, err := cliutils.GetDefaultKaiwoCliConfigPath()
			if err != nil {
				return fmt.Errorf("failed to get default Kaiwo config path: %w", err)
			}

			if existingKaiwoCliConfigPath == "" && user == "" {
				// No config file was found and no user was given, allow user to create the config dynamically
				fmt.Printf("Kaiwo config file cannot be resolved and user is not given. Would you like to create a kaiwoconfig file?")
				configCreated, err := promptUserForConfig()
				if err != nil {
					return fmt.Errorf("failed to prompt user: %v", err)
				}

				if configCreated {
					existingKaiwoCliConfigPath = configPath
				}
			}

			if existingKaiwoCliConfigPath != "" {
				// Config file exists
				kaiwoConfig, err = readKaiwoCliConfig(existingKaiwoCliConfigPath)
				if err != nil {
					return fmt.Errorf("failed to read Kaiwo config file: %v", err)
				}
			} else if user == "" {
				// No config file exists and the user was not explicitly provided
				return fmt.Errorf("kaiwo cli config file was not given, ensure you pass it as a CLI flag, via the env var %s or place it in %s, or provide the user / queue via CLI flags", cliutils.KaiwoCliConfigPathEnv, configPath)
			}

			if user != "" {
				// Override the user
				kaiwoConfig.User = user
			}
			if queue != "" {
				// Override the cluster queue
				kaiwoConfig.ClusterQueue = queue
			}

			ctx := context.Background()
			clients, err := k8sUtils.GetKubernetesClients()
			if err != nil {
				return fmt.Errorf("failed to get Kubernetes clients: %v", err)
			}

			if err := ensureObjectNestedStringField(&obj, kaiwoConfig.User, "spec", "user"); err != nil {
				return fmt.Errorf("failed to ensure user field: %v", err)
			}
			if err := ensureObjectNestedStringField(&obj, kaiwoConfig.ClusterQueue, "spec", "clusterQueue"); err != nil {
				return fmt.Errorf("failed to ensure queue field: %v", err)
			}

			return Apply(ctx, clients.Client, &obj)
		},
	}

	cmd.Flags().StringVarP(&user, "user", "", "", "The user to run as")
	cmd.Flags().StringVarP(&queue, "queue", "", "", "The cluster queue to use")
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "The namespace to use, if none is given")

	cmd.Flags().StringVarP(&file, "file", "f", "", "The Kaiwo manifest file to apply")
	cmd.Flags().StringVarP(&config, "config", "c", "", "The path to the kaiwoconfig configuration file")

	return cmd
}

func readKaiwoCliConfig(path string) (cliutils.KaiwoCliConfig, error) {
	yamlFile, err := os.ReadFile(path)
	config := cliutils.KaiwoCliConfig{}
	if err != nil {
		return config, fmt.Errorf("failed to read file %s: %w", path, err)
	}
	err = yaml.Unmarshal(yamlFile, &config)
	if err != nil {
		return config, fmt.Errorf("failed to unmarshal file %s: %w", path, err)
	}
	return config, nil
}

// readManifest reads a Kubernetes manifest from a given path or from stdin if path is "-" or "--"
func readManifest(path string) (unstructured.Unstructured, error) {
	var obj unstructured.Unstructured
	var reader io.Reader
	var yamlBytes []byte
	var err error

	if path == "-" || path == "--" {
		reader = os.Stdin
		yamlBytes, err = io.ReadAll(os.Stdin)
		if err != nil {
			return obj, fmt.Errorf("failed to read from stdin: %w", err)
		}
	} else {
		file, err := os.Open(path)
		if err != nil {
			return obj, fmt.Errorf("failed to open file: %w", err)
		}
		defer func(file *os.File) {
			err := file.Close()
			if err != nil {
				panic(err)
			}
		}(file)

		reader = file
		yamlBytes, err = os.ReadFile(path)
		if err != nil {
			return obj, fmt.Errorf("failed to read file: %w", err)
		}
	}

	// Decode YAML into unstructured.Unstructured
	decoder := k8syaml.NewYAMLOrJSONDecoder(reader, 4096)
	err = decoder.Decode(&obj)
	if err != nil {
		// Retry decoding from raw bytes if the decoder failed
		err = k8syaml.Unmarshal(yamlBytes, &obj)
		if err != nil {
			return obj, fmt.Errorf("failed to decode manifest: %w", err)
		}
	}

	return obj, nil
}

// promptUserForConfig allows the user to dynamically create a kaiwoconfig if one does not exist
func promptUserForConfig() (bool, error) {
	configPath, err := cliutils.GetDefaultKaiwoCliConfigPath()
	if err != nil {
		return false, fmt.Errorf("failed to get default Kaiwo config path: %w", err)
	}

	create := false
	err = huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Create Kaiwo config?").
				Value(&create).
				Description(fmt.Sprintf("Create Kaiwo config in %s", configPath)),
		),
	).WithAccessible(promptAccessible).Run()
	if err != nil {
		return false, err
	}

	if !create {
		return false, nil
	}

	queueValue := "kaiwo"
	userEmail := ""

	for {
		form := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("User email").
					Description("The user email to run your jobs and services with").
					Placeholder("user@email.com").
					Value(&userEmail),
				huh.NewInput().
					Title("Cluster queue").
					Description("The cluster queue name to run your jobs and services in").
					Value(&queueValue),
			),
		).WithAccessible(promptAccessible)

		if err := form.Run(); err != nil {
			return false, err
		}

		if !emailRegex.MatchString(userEmail) {
			fmt.Println("Please enter a valid email address (e.g., user@example.com)")
			continue
		}

		break
	}

	config := &cliutils.KaiwoCliConfig{
		User:         userEmail,
		ClusterQueue: queueValue,
	}

	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false, fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	file, err := os.Create(configPath)
	if err != nil {
		fmt.Println("Failed to create file:", err)
		return false, fmt.Errorf("failed to create Kaiwo config file: %w", err)
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			panic(err)
		}
	}(file)

	encoder := yaml.NewEncoder(file)
	encoder.SetIndent(2)
	if err := encoder.Encode(config); err != nil {
		return false, fmt.Errorf("failed to encode Kaiwo config file: %w", err)
	}

	return true, nil
}

func ensureObjectNestedStringField(obj *unstructured.Unstructured, value string, fields ...string) error {
	if _, found, err := unstructured.NestedFieldCopy(obj.Object, fields...); err != nil {
		return fmt.Errorf("failed to get nested fields: %w", err)
	} else if !found {
		if err := unstructured.SetNestedField(obj.Object, value, fields...); err != nil {
			return fmt.Errorf("failed to set nested field: %w", err)
		}
	}
	return nil
}

func Apply(ctx context.Context, k8sClient client.Client, manifest *unstructured.Unstructured) error {
	obj := manifest.DeepCopy()

	if namespace == "" {
		namespace = "default"
	}

	if err := ensureObjectNestedStringField(obj, namespace, "metadata", "namespace"); err != nil {
		return fmt.Errorf("failed to ensure object namespace: %w", err)
	}

	if obj.GetNamespace() != namespace && namespace != "default" {
		return fmt.Errorf("expected namespace to be %s, got %s", namespace, obj.GetNamespace())
	}

	key := client.ObjectKeyFromObject(obj)
	gvk := obj.GroupVersionKind()
	logrus.Infof("Submitting resource: %s/%s %s %s/%s", gvk.Group, gvk.Version, gvk.Kind, key.Namespace, key.Name)

	if err := k8sClient.Get(ctx, key, obj); err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to get object: %w", err)
		}
	} else {
		logrus.Infof("Resource already exists, updating...")
		if err := k8sClient.Update(ctx, obj); err != nil {
			return fmt.Errorf("failed to update object: %w", err)
		}
		logrus.Infof("Resource successfully updated")
		return nil
	}

	logrus.Info("Creating resource...")

	if err := k8sClient.Create(ctx, obj); err != nil {
		return fmt.Errorf("failed to create resource: %w", err)
	}

	logrus.Infof("Resource successfully created")

	return nil
}
