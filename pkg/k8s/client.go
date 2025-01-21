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

package k8s

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"os"
	"path/filepath"
	"sync"
)

var (
	dynamicClient dynamic.Interface
	//typedClient   *kubernetes.Clientset

	dynamicInitErr error
	//typedInitErr   error

	dynamicOnce sync.Once
	//typedOnce   sync.Once
)

// getKubeConfig loads the kubeconfig file path
func getKubeConfig() (string, error) {
	kubeConfigPath := os.Getenv("KUBECONFIG")
	if kubeConfigPath != "" {
		logrus.Infof("Using KUBECONFIG environment variable: %s", kubeConfigPath)
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
	kubeConfigPath, err := getKubeConfig()
	if err != nil {
		return nil, err
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to build kubeconfig: %v", err)
	}

	return config, nil
}

// InitializeDynamicClient initializes the dynamic Kubernetes client
func InitializeDynamicClient() (dynamic.Interface, error) {
	config, err := buildKubeConfig()
	if err != nil {
		return nil, err
	}

	client, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic Kubernetes client: %v", err)
	}

	return client, nil
}

// GetDynamicClient provides a singleton for the dynamic client
func GetDynamicClient() (dynamic.Interface, error) {
	dynamicOnce.Do(func() {
		client, err := InitializeDynamicClient()
		if err != nil {
			logrus.Fatalf("failed to initialize dynamic Kubernetes client: %v", err)
			dynamicInitErr = err
			return
		}
		dynamicClient = client
	})
	return dynamicClient, dynamicInitErr
}

//// InitializeTypedClient initializes the typed Kubernetes client
//func InitializeTypedClient() (*kubernetes.Clientset, error) {
//	config, err := buildKubeConfig()
//	if err != nil {
//		return nil, err
//	}
//
//	client, err := kubernetes.NewForConfig(config)
//	if err != nil {
//		return nil, fmt.Errorf("failed to create typed Kubernetes client: %v", err)
//	}
//
//	return client, nil
//}
//
//// GetTypedClient provides a singleton for the typed client
//func GetTypedClient() (*kubernetes.Clientset, error) {
//	typedOnce.Do(func() {
//		client, err := InitializeTypedClient()
//		if err != nil {
//			logrus.Fatalf("failed to initialize typed Kubernetes client: %v", err)
//			typedInitErr = err
//			return
//		}
//		typedClient = client
//	})
//	return typedClient, typedInitErr
//}
