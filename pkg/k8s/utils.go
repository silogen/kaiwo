package k8s

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/sirupsen/logrus"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"

	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
)

func CreateNamespace(namespace string) *v1.Namespace {
	return &v1.Namespace{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Namespace",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}
}

func isBinaryFile(content []byte) bool {
	return bytes.Contains(content, []byte{0})
}

// Generate ConfigMap from a directory
func GenerateConfigMapFromDir(dir string, configmap_name string, namespace string, skipFiles []string) (v1.ConfigMap, error) {
	files, err := os.ReadDir(dir)

	configMap := v1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      configmap_name,
			Namespace: namespace,
		},
		Data: make(map[string]string),
	}

	if err != nil {
		return configMap, fmt.Errorf("failed to read directory: %w", err)
	}

	data := make(map[string]string)

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		if slices.Contains(skipFiles, file.Name()) {
			continue
		}

		filePath := filepath.Join(dir, file.Name())
		content, err := os.ReadFile(filePath)
		if err != nil {
			return configMap, fmt.Errorf("failed to read file %s: %w", filePath, err)
		}

		// Skip binary files
		if isBinaryFile(content) {
			logrus.Warnf("Skipping binary file: %s", file.Name())
			continue
		}
		data[file.Name()] = string(content)
	}

	configMap.Data = data

	return configMap, nil
}

func DecodeYAMLToObjects(yamlData string) ([]runtime.Object, error) {
	// Create a new scheme and add all known types to it
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = rayv1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)

	// Create a YAML decoder
	decoder := serializer.NewCodecFactory(scheme).UniversalDeserializer()

	// Split the YAML into individual documents
	documents := strings.Split(yamlData, "---")

	var objects []runtime.Object
	for _, doc := range documents {
		if strings.TrimSpace(doc) == "" {
			continue
		}

		// Decode each document into a runtime.Object
		obj, _, err := decoder.Decode([]byte(doc), nil, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to decode YAML: %w", err)
		}

		objects = append(objects, obj)
	}

	return objects, nil
}
