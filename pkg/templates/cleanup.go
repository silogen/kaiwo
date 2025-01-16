package templates

import (
	"context"
	"fmt"
	"github.com/silogen/ai-workload-orchestrator/pkg/k8s"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Cleanup removes a specified Kubernetes resource and optionally its associated ConfigMap.
func Cleanup(ctx context.Context, resource string, name string, namespace string, removeConfigMap bool) error {
	// Validate inputs
	if resource == "" || name == "" || namespace == "" {
		return fmt.Errorf("invalid input: resource, name, and namespace must not be empty")
	}

	client, err := k8s.GetDynamicClient()
	if err != nil {
		return fmt.Errorf("failed to get dynamic client: %w", err)
	}
	if client == nil {
		return fmt.Errorf("dynamic client is nil")
	}

	logrus.Infof("Removing workload %s/%s/%s", namespace, resource, name)

	// Define the GVR based on resource type
	var gvr schema.GroupVersionResource
	switch resource {
	case "job":
		gvr = schema.GroupVersionResource{
			Group:    "batch",
			Version:  "v1",
			Resource: "jobs",
		}
	case "rayjob":
		gvr = schema.GroupVersionResource{
			Group:    "ray.io",
			Version:  "v1",
			Resource: "rayjobs",
		}
	case "rayservice":
		gvr = schema.GroupVersionResource{
			Group:    "ray.io",
			Version:  "v1",
			Resource: "rayservices",
		}
	default:
		logrus.Errorf("unknown resource type: %s", resource)
		return fmt.Errorf("unknown resource %s", resource)
	}

	err = client.Resource(gvr).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete %s/%s %s in namespace %s: %w", gvr.Group, gvr.Resource, name, namespace, err)
	}

	logrus.Infof("Resource %s/%s (%s/%s) deleted successfully", gvr.Group, gvr.Resource, namespace, name)

	if !removeConfigMap {
		return nil
	}

	configMapGvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "configmaps",
	}

	// Delete the associated ConfigMap
	err = client.Resource(configMapGvr).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			logrus.Info("Config map not found, skipping delete")
		} else {
			return fmt.Errorf("failed to delete config map %s in namespace %s: %w", name, namespace, err)
		}

	} else {
		logrus.Infof("ConfigMap (%s/%s) deleted successfully", namespace, name)
	}

	return nil
}
