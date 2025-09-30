/*
MIT License

Copyright (c) 2025 Advanced Micro Devices, Inc.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package k8s

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	kaiwo "github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"

	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	"github.com/sirupsen/logrus"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	kueuev1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"
)

var (
	dynamicClient dynamic.Interface
	// typedClient   *kubernetes.Clientset

	dynamicInitErr error
	// typedInitErr   error

	dynamicOnce sync.Once
	// typedOnce   sync.Once

	scheme2       runtime.Scheme
	schemeInitErr error
	schemeOnce    sync.Once
)

// GetKubeConfig loads the kubeconfig file path
func GetKubeConfig() (string, error) {
	kubeConfigPath := os.Getenv("KUBECONFIG")
	if kubeConfigPath != "" {
		logrus.Debugf("Using KUBECONFIG environment variable: %s", kubeConfigPath)
	} else {
		kubeConfigPath = filepath.Join(homedir.HomeDir(), ".kube", "config")
		if _, err := os.Stat(kubeConfigPath); os.IsNotExist(err) {
			return "", fmt.Errorf("kubeconfig file not found: %s", kubeConfigPath)
		}
	}
	return kubeConfigPath, nil
}

// buildKubeConfig creates a REST config from the kubeconfig file
func buildKubeConfig() (*rest.Config, error) {
	kubeConfigPath, err := GetKubeConfig()
	if err != nil {
		return nil, err
	}

	kubeconfig, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to build kubeconfig: %v", err)
	}

	return kubeconfig, nil
}

// InitializeDynamicClient initializes the dynamic Kubernetes client
func InitializeDynamicClient() (dynamic.Interface, error) {
	config_, err := buildKubeConfig()
	if err != nil {
		return nil, err
	}

	client_, err := dynamic.NewForConfig(config_)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic Kubernetes client: %v", err)
	}

	return client_, nil
}

// GetDynamicClient provides a singleton for the dynamic client
func GetDynamicClient() (dynamic.Interface, error) {
	dynamicOnce.Do(func() {
		c, err := InitializeDynamicClient()
		if err != nil {
			logrus.Fatalf("failed to initialize dynamic Kubernetes client: %v", err)
			dynamicInitErr = err
			return
		}
		dynamicClient = c
	})
	return dynamicClient, dynamicInitErr
}

func buildScheme() (*runtime.Scheme, error) {
	scheme := runtime.NewScheme()
	// Add core Kubernetes API types
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to add core Kubernetes types to scheme: %v", err)
	}

	// Add batch API types
	if err := batchv1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to add batch Kubernetes types to scheme: %v", err)
	}

	// Add Kueue API types
	if err := kueuev1beta1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to add kueue Kubernetes types to scheme: %v", err)
	}

	// Add RayService custom resource API types
	if err := rayv1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to add RayService types to scheme: %v", err)
	}

	if err := kaiwo.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to add Kaiwo types to scheme: %v", err)
	}

	return scheme, nil
}

func GetScheme() (runtime.Scheme, error) {
	schemeOnce.Do(func() {
		s, err := buildScheme()
		if err != nil {
			logrus.Fatalf("failed to build scheme: %v", err)
			schemeInitErr = fmt.Errorf("failed to build scheme: %v", err)
			return
		}
		scheme2 = *s
	})
	return scheme2, schemeInitErr
}

func GetClient() (client.Client, error) {
	cfg, err := config.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get Kubernetes config: %v", err)
	}

	s, err := GetScheme()
	if err != nil {
		return nil, fmt.Errorf("failed to get Kubernetes scheme: %v", err)
	}

	k8sClient, err := client.New(cfg, client.Options{Scheme: &s})
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %v", err)
	}
	return k8sClient, err
}

func GetClientset() (*kubernetes.Clientset, error) {
	kubeconfig, _ := GetKubeConfig()
	config_, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to get Kubernetes config: %v", err)
	}

	// Create Kubernetes clientset
	clientset, err := kubernetes.NewForConfig(config_)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %v", err)
	}
	return clientset, nil
}

type KubernetesClients struct {
	Client     client.Client
	Clientset  *kubernetes.Clientset
	Kubeconfig *rest.Config
}

func GetKubernetesClients() (*KubernetesClients, error) {
	k8sClient, err := GetClient()
	if err != nil {
		return nil, fmt.Errorf("failed to get Kubernetes client: %v", err)
	}

	clientset, err := GetClientset()
	if err != nil {
		return nil, fmt.Errorf("failed to get Kubernetes clientset: %v", err)
	}

	kubeconfig, err := buildKubeConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get Kubernetes config: %v", err)
	}

	return &KubernetesClients{
		Client:     k8sClient,
		Clientset:  clientset,
		Kubeconfig: kubeconfig,
	}, nil
}
