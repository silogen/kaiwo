package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/kueue/kueue/x-k8s.io/client/v1beta1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

func main() {
	// Load the kubeconfig from the default location (usually ~/.kube/config)
	kubeconfig := flag.String("kubeconfig", "", "location to your kubeconfig file")
	if *kubeconfig == "" && homedir.HomeDir() != "" {
		*kubeconfig = homedir.HomeDir() + "/.kube/config"
	}
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		log.Fatalf("Failed to create Kubernetes config: %v", err)
	}

	// Create a Kubernetes client
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("Failed to create Kubernetes client: %v", err)
	}

	// Apply the manifests (you can implement this logic in another function)
	// Example: ApplyNamespace(clientset)

	// Fetch ResourceFlavor
	resourceFlavor, err := fetchResourceFlavor(clientset, "base-gpu-flavour")
	if err != nil {
		log.Fatalf("Error fetching resource flavor: %v", err)
	}

	// Print the fetched ResourceFlavor
	fmt.Printf("Fetched ResourceFlavor: %v\n", resourceFlavor)
}

// ApplyNamespace applies a namespace (just as an example)
func ApplyNamespace(clientset *kubernetes.Clientset) {
	namespace := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "example-namespace",
		},
	}
	_, err := clientset.CoreV1().Namespaces().Create(context.TODO(), namespace, metav1.CreateOptions{})
	if err != nil {
		log.Fatalf("Failed to apply namespace: %v", err)
	}
	fmt.Println("Namespace applied successfully")
}

// FetchResourceFlavor fetches the ResourceFlavor resource from the cluster
func fetchResourceFlavor(clientset *kubernetes.Clientset, name string) (*v1beta1.ResourceFlavor, error) {
	// Create a dynamic client to fetch the custom resource (ResourceFlavor)
	dynamicClient, err := clientset.DynamicClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %v", err)
	}

	// Define the GVR (Group Version Resource) for ResourceFlavor
	gvr := metav1.GroupVersionResource{
		Group:    "kueue.x-k8s.io",
		Version:  "v1beta1",
		Resource: "resourceflavors",
	}

	// Fetch the resource using the dynamic client
	resourceFlavorObj, err := dynamicClient.Resource(gvr).Namespace("default").Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch ResourceFlavor: %v", err)
	}

	// Convert to ResourceFlavor struct
	var resourceFlavor v1beta1.ResourceFlavor
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(resourceFlavorObj.UnstructuredContent(), &resourceFlavor); err != nil {
		return nil, fmt.Errorf("failed to convert unstructured to ResourceFlavor: %v", err)
	}

	return &resourceFlavor, nil
}
