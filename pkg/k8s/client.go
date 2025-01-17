package k8s

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"os"
	"path/filepath"
	"sync"
)

var (
	dynamicClient dynamic.Interface
	initErr       error
	once          sync.Once
)

func InitializeClient() (dynamic.Interface, error) {
	kubeConfigPath := os.Getenv("KUBECONFIG")
	if kubeConfigPath != "" {
		logrus.Infof("Using KUBECONFIG environment variable: %s", kubeConfigPath)
	} else {
		kubeConfigPath = filepath.Join(homedir.HomeDir(), ".kube", "config")
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to build kubeconfig: %v", err)
	}

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %v", err)
	}

	return dynamicClient, nil
}

// GetDynamicClient provides a way to access the client without initializing it multiple times
func GetDynamicClient() (dynamic.Interface, error) {
	once.Do(func() {
		client, err := InitializeClient()
		if err != nil {
			logrus.Fatalf("failed to initialize kubernetes client: %v", err)
		}
		dynamicClient = client
	})

	return dynamicClient, initErr
}
