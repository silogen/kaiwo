package k8s

import (
	"context"
	"fmt"
	"github.com/silogen/ai-workload-orchestrator/pkg/templates"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"strconv"

	kueuev1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type KueueArgs struct {
	NodeGpuCount int
}

func ListResourceFlavors(ctx context.Context, client dynamic.Interface) ([]kueuev1beta1.ResourceFlavor, error) {
	gvr := schema.GroupVersionResource{
		Group:    "kueue.x-k8s.io",
		Version:  "v1beta1",
		Resource: "resourceflavors",
	}
	resourceList, err := client.Resource(gvr).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list resource flavors: %v", err)
	}

	var resourceFlavors []kueuev1beta1.ResourceFlavor
	for _, item := range resourceList.Items {
		resourceFlavor := kueuev1beta1.ResourceFlavor{}
		err = runtime.DefaultUnstructuredConverter.FromUnstructured(item.UnstructuredContent(), &resourceFlavor)
		if err != nil {
			return nil, fmt.Errorf("failed to convert unstructured item: %v", err)
		}
		resourceFlavors = append(resourceFlavors, resourceFlavor)
	}

	return resourceFlavors, nil
}

func GetResourceFlavorGpuCount(resourceFlavor kueuev1beta1.ResourceFlavor) (int, error) {
	gpuLabel, found := resourceFlavor.Spec.NodeLabels["beta.amd.com/gpu.family.AI"]
	if !found {
		return 0, fmt.Errorf("GPU label not found in ResourceFlavor")
	}

	gpuCount, err := strconv.Atoi(gpuLabel)
	if err != nil {
		return 0, fmt.Errorf("failed to parse GPU count: %v", err)
	}

	return gpuCount, nil
}

func GetDefaultResourceFlavorGpuCount(ctx context.Context, client dynamic.Interface) (int, error) {
	resourceFlavors, err := ListResourceFlavors(ctx, client)
	if err != nil {
		return 0, fmt.Errorf("failed to list resource flavors: %v", err)
	}
	if len(resourceFlavors) == 1 {
		return GetResourceFlavorGpuCount(resourceFlavors[0])
	} else {
		return 0, fmt.Errorf("zero or more than one resource flavor found, expected just one")
	}
}

func ListClusterQueues(ctx context.Context, client dynamic.Interface) ([]kueuev1beta1.ClusterQueue, error) {
	gvr := schema.GroupVersionResource{
		Group:    "kueue.x-k8s.io",
		Version:  "v1beta1",
		Resource: "clusterqueues",
	}
	resourceList, err := client.Resource(gvr).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list cluster queues: %v", err)
	}

	var clusterQueues []kueuev1beta1.ClusterQueue
	for _, item := range resourceList.Items {
		clusterQueue := kueuev1beta1.ClusterQueue{}
		err = runtime.DefaultUnstructuredConverter.FromUnstructured(item.UnstructuredContent(), &clusterQueue)
		if err != nil {
			return nil, fmt.Errorf("failed to convert unstructured item: %v", err)
		}
		clusterQueues = append(clusterQueues, clusterQueue)
	}

	return clusterQueues, nil
}

func GetDefaultClusterQueue(ctx context.Context, client dynamic.Interface) (*kueuev1beta1.ClusterQueue, error) {
	clusterQueues, err := ListClusterQueues(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("failed to list cluster queues: %v", err)
	}
	if len(clusterQueues) == 0 {
		return nil, fmt.Errorf("no cluster queues found")
	}
	// FIXME determine how to choose
	//if len(clusterQueues) > 1 {
	//	return nil, fmt.Errorf("multiple cluster queues found")
	//}
	return &clusterQueues[0], nil
}

func PrepareLocalClusterQueue(args templates.WorkloadArgs, c dynamic.Interface) (*unstructured.Unstructured, error) {
	clusterQueue, err := GetDefaultClusterQueue(context.TODO(), c)
	if err != nil {
		return nil, fmt.Errorf("failed to get default cluster queue: %w", err)
	}
	localClusterQueue := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "kueue.x-k8s.io/v1beta1",
			"kind":       "LocalQueue",
			"metadata": map[string]interface{}{
				"namespace": args.Namespace,
				"name":      clusterQueue.Name,
			},
			"spec": map[string]interface{}{
				"clusterQueue": clusterQueue.Name,
			},
		},
	}
	return localClusterQueue, nil
}
