// This file contains the structs used to create the config map for Kubernetes and the Kaiwo downloader

package utils

import (
	"fmt"
	"os"

	"sigs.k8s.io/controller-runtime/pkg/client"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"gopkg.in/yaml.v3"

	corev1 "k8s.io/api/core/v1"
)

// DownloadTaskConfig describes all the items that should be downloaded into a Kubernetes pod (via an init container)
type DownloadTaskConfig struct {
	// DownloadRoot is hardcoded in the configuration below
	DownloadRoot string                         `yaml:"download_root"`
	S3           []S3DownloadItem               `yaml:"s3"`
	GCS          []GCSDownloadItem              `yaml:"gcs"`
	HF           []HuggingFaceDownloadItem      `yaml:"hf"`
	AzureBlob    []AzureBlobStorageDownloadItem `yaml:"azure_blob"`
}

type ValueReference struct {
	File       string `yaml:"file,omitempty"`
	SecretName string `yaml:"secret_name,omitempty"`
	SecretKey  string `yaml:"secret_key,omitempty"`
}

type S3DownloadItem struct {
	EndpointUrl ValueReference        `yaml:"endpoint_url,omitempty"`
	AccessKeyId ValueReference        `yaml:"access_key_id,omitempty"`
	SecretKey   ValueReference        `yaml:"secret_key,omitempty"`
	Buckets     []CloudDownloadBucket `yaml:"buckets"`
}

type GCSDownloadItem struct {
	ApplicationCredentials ValueReference        `yaml:"application_credentials,omitempty"`
	Buckets                []CloudDownloadBucket `yaml:"buckets"`
}

type AzureBlobStorageDownloadItem struct {
	ConnectionString ValueReference        `yaml:"connection_string,omitempty"`
	Containers       []CloudDownloadBucket `yaml:"containers"`
}

type HuggingFaceDownloadItem struct {
	RepoID string   `yaml:"repo_id"`
	Files  []string `yaml:"files"`
}

type CloudDownloadBucket struct {
	Name    string                `yaml:"name"`
	Files   []CloudDownloadFile   `yaml:"files"`
	Folders []CloudDownloadFolder `yaml:"folders"`
}

type CloudDownloadFolder struct {
	Folder     string `yaml:"folder"`
	TargetPath string `yaml:"target_path"`
	Glob       string `yaml:"glob,omitempty"`
}

type CloudDownloadFile struct {
	File       string `yaml:"file"`
	TargetPath string `yaml:"target_path"`
}

func LoadDownloadConfig(path string) (*DownloadTaskConfig, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading config file: %w", err)
	}

	config := &DownloadTaskConfig{}
	err = yaml.Unmarshal(contents, config)
	if err != nil {
		return nil, fmt.Errorf("error parsing config file: %w", err)
	}

	return config, nil
}

type DownloadWrapper struct {
	Container         corev1.Container
	ConfigMap         *corev1.ConfigMap
	AdditionalVolumes []*corev1.Volume
}

func (config *DownloadTaskConfig) ToKubernetesContainer(workloadVolume corev1.Volume, kaiwoPythonImage string, hfHome string, key client.ObjectKey) (*DownloadWrapper, error) {
	secretVolumes := map[string]*corev1.Volume{}
	secretsMount := "/app/secrets"

	// Closure to add a new secret volume if required, and to add the specific secret to the secret volume items list
	setSecretPath := func(ref *ValueReference) {
		if _, ok := secretVolumes[ref.SecretName]; !ok {
			secretVolumes[ref.SecretName] = &corev1.Volume{
				Name: "secret-" + ref.SecretName,
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: ref.SecretName,
					},
				},
			}
		}

		path := ref.SecretKey

		secretVolumes[ref.SecretName].Secret.Items = append(secretVolumes[ref.SecretName].Secret.Items, corev1.KeyToPath{
			Key:  ref.SecretKey,
			Path: path,
		})

		// Path where the secret will be mounted on in the primary container
		ref.File = secretsMount + "/" + path
	}

	// Set secret paths

	for i := range config.S3 {
		setSecretPath(&config.S3[i].EndpointUrl)
		setSecretPath(&config.S3[i].AccessKeyId)
		setSecretPath(&config.S3[i].SecretKey)
	}
	for i := range config.GCS {
		setSecretPath(&config.GCS[i].ApplicationCredentials)
	}
	for i := range config.AzureBlob {
		setSecretPath(&config.AzureBlob[i].ConnectionString)
	}

	// Hard-code the root within the init container
	config.DownloadRoot = "/workload"

	// Marshal the config that is to be passed to the Python downloader
	yamlData, err := yaml.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("error serializing config: %w", err)
	}

	// This config map holds
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      key.Name + "-download",
			Namespace: key.Namespace,
		},
		Data: map[string]string{
			"config.yaml": string(yamlData),
		},
	}

	downloadConfigVolume := &corev1.Volume{
		Name: "download-config",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: configMap.Name,
				},
			},
		},
	}

	additionalVolumes := []*corev1.Volume{
		downloadConfigVolume,
	}

	container := corev1.Container{
		Image: kaiwoPythonImage,
		Env: []corev1.EnvVar{
			// Set HF_HOME to download the cache to the right place
			{
				Name:  "HF_HOME",
				Value: hfHome,
			},
		},
		Command: []string{
			"python",
			"-m",
			"kaiwo.downloader",
		},
		WorkingDir: "/app/python",
		Args: []string{
			"/app/config/config.yaml",
		},
		VolumeMounts: []corev1.VolumeMount{
			// Config map
			{
				Name:      downloadConfigVolume.Name,
				MountPath: "/app/config",
			},
			// PVC for data
			{
				Name:      workloadVolume.Name,
				MountPath: config.DownloadRoot,
			},
		},
	}

	// Add secret volumes
	for secretName, secretVolume := range secretVolumes {
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      secretVolume.Name,
			MountPath: secretsMount + "/" + secretName,
		})
		additionalVolumes = append(additionalVolumes, secretVolume)
	}

	return &DownloadWrapper{
		Container:         container,
		ConfigMap:         configMap,
		AdditionalVolumes: additionalVolumes,
	}, nil
}
