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

package controllerutils

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"k8s.io/client-go/tools/record"

	utilerrors "k8s.io/apimachinery/pkg/util/errors"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"k8s.io/apimachinery/pkg/runtime"

	baseutils "github.com/silogen/kaiwo/pkg/utils"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	kueuev1alpha1 "sigs.k8s.io/kueue/apis/kueue/v1alpha1"
	kueuev1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"

	kaiwo "github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"
	"github.com/silogen/kaiwo/pkg/workloads/common"
)

// SyncQueueResource syncs a list of resources with the Kubernetes API
func SyncQueueResource[T any, U client.Object](
	ctx context.Context,
	k8sClient client.Client,
	recorder record.EventRecorder,
	kaiwoQueueConfig *kaiwo.KaiwoQueueConfig,
	desired []T,
	converter KaiwoQueueResourceConverter[T, U],
	opts ...SyncOption,
) error {
	cfg := SyncOptions{
		ShouldDelete: func(_ client.Object) bool { return true },
		Owner:        nil,
		Scheme:       nil,
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	kind := converter.CreateList().GetObjectKind().GroupVersionKind().Kind
	kind = strings.TrimSuffix(kind, "List")
	baseLog := log.FromContext(ctx).
		WithName("syncResource").
		WithValues("controller", "KaiwoQueue", "kind", kind)

	existing := converter.CreateList()
	if err := k8sClient.List(ctx, existing); err != nil {
		return fmt.Errorf("failed to list resources: %w", err)
	}

	items, err := meta.ExtractList(existing)
	if err != nil {
		// Should never happen, but just it's not silently ignored if it does
		panic("failed to extract list of resources")
	}
	toDelete := make(map[string]U, len(items))

	for _, item := range items {
		obj := item.(U)
		key := obj.GetNamespace() + "/" + obj.GetName()
		toDelete[key] = obj
	}

	var syncErrors []error

	for _, item := range desired {

		desiredObj, err := converter.Convert(item)
		if err != nil {
			syncErrors = append(syncErrors, fmt.Errorf("error converting %v: %w", item, err))
			continue
		}

		key := desiredObj.GetNamespace() + "/" + desiredObj.GetName()

		working := desiredObj.DeepCopyObject().(U)

		// Determine owner
		owner := cfg.Owner
		if cfg.Owner == nil && cfg.OwnerFunc == nil {
			// Should never happen, but just it's not silently ignored if it does
			panic("an owner or owner func must be passed to this method")
		} else if cfg.Scheme == nil {
			// Should never happen, but just it's not silently ignored if it does
			panic("a scheme must be passed to this method")
		}
		if owner == nil && cfg.OwnerFunc != nil {
			owner = cfg.OwnerFunc(working)
		}

		if err := controllerutil.SetControllerReference(owner, working, cfg.Scheme); err != nil {
			syncErrors = append(syncErrors, fmt.Errorf("error setting owner %s/%s: %w",
				working.GetNamespace(), working.GetName(), err))
			// Ensure we don't delete if encountered an error
			delete(toDelete, key)
			continue
		}

		op, err := controllerutil.CreateOrPatch(ctx, k8sClient, working, func() error {
			converter.Mutate(desiredObj, working)
			return nil
		})
		if err != nil {
			syncErrors = append(syncErrors, fmt.Errorf("error applying %s/%s: %w",
				working.GetNamespace(), working.GetName(), err))
			// Ensure we don't delete if encountered an error
			delete(toDelete, key)
			continue
		}

		ns, name := working.GetNamespace(), working.GetName()
		entryLog := baseLog.WithValues("namespace", ns, "name", name, "operation", op)

		switch op {
		case controllerutil.OperationResultNone:
			baseutils.Debug(entryLog, "no-op, up-to-date")
		case controllerutil.OperationResultCreated:
			entryLog.Info("created")
			recorder.Event(
				kaiwoQueueConfig, corev1.EventTypeNormal, "Created",
				fmt.Sprintf("Created %s %s/%s", kind, ns, name),
			)
		case controllerutil.OperationResultUpdated:
			entryLog.Info("updated")
			recorder.Event(
				kaiwoQueueConfig, corev1.EventTypeNormal, "Updated",
				fmt.Sprintf("Updated %s %s/%s", kind, ns, name),
			)
		}

		delete(toDelete, key)
	}

	for key, obj := range toDelete {
		if cfg.ShouldDelete != nil && !cfg.ShouldDelete(obj) {
			continue
		}
		ns, name := obj.GetNamespace(), obj.GetName()
		if err := k8sClient.Delete(ctx, obj); err != nil && !errors.IsNotFound(err) {
			syncErrors = append(syncErrors, fmt.Errorf("delete %s: %w", key, err))
			continue
		}
		baseLog.WithValues("namespace", ns, "name", name).
			Info("deleted stale")
		recorder.Event(
			kaiwoQueueConfig, corev1.EventTypeNormal, "Deleted",
			fmt.Sprintf("Deleted stale %s %s/%s", kind, ns, name),
		)
	}

	if len(syncErrors) > 0 {
		return utilerrors.NewAggregate(syncErrors)
	}

	return nil
}

type SyncOptions struct {
	// if non-nil, called for each stale object; delete only if it returns true
	ShouldDelete func(obj client.Object) bool

	// Scheme is used when setting the owner reference
	Scheme *runtime.Scheme

	// if non-nil, used as the owner reference on every created/updated obj
	Owner client.Object

	// OwnerFunc, if non-nil, is called for each object to determine its owner
	// Only used if Owner is non-nil
	OwnerFunc func(obj client.Object) client.Object
}

type SyncOption func(*SyncOptions)

// WithStaleDeleteCheck takes a U-typed predicate and wraps it
func WithStaleDeleteCheck[U client.Object](fn func(obj U) bool) SyncOption {
	return func(o *SyncOptions) {
		o.ShouldDelete = func(obj client.Object) bool {
			u, ok := obj.(U)
			if !ok {
				return false
			}
			return fn(u)
		}
	}
}

// WithOwnerReference is now non-generic, since owner is just client.Object
func WithOwnerReference(owner client.Object, scheme *runtime.Scheme) SyncOption {
	return func(o *SyncOptions) {
		o.Owner = owner
		o.Scheme = scheme
	}
}

// WithDynamicOwnerReference allows the syncer to set the owner reference dynamically to something else
func WithDynamicOwnerReference(scheme *runtime.Scheme, fn func(obj client.Object) client.Object) SyncOption {
	return func(o *SyncOptions) {
		o.OwnerFunc = fn
		o.Scheme = scheme
	}
}

type KaiwoQueueResourceConverter[T any, U client.Object] interface {
	// Convert turns the Kaiwo resources T into the Kueue resource U
	Convert(input T) (U, error)

	// CreateList creates a list to use to fetch the Kueue resources of type U
	CreateList() client.ObjectList

	Mutate(desired U, existing U)
}

// ClusterQueue

type ClusterQueueConverter struct{}

func (c ClusterQueueConverter) Convert(input kaiwo.ClusterQueue) (*kueuev1beta1.ClusterQueue, error) {
	in := input.Spec
	clusterQueue := &kueuev1beta1.ClusterQueue{
		ObjectMeta: metav1.ObjectMeta{
			Name: input.Name,
		},
		Spec: kueuev1beta1.ClusterQueueSpec{
			ResourceGroups:          in.ResourceGroups,
			Cohort:                  in.Cohort,
			QueueingStrategy:        in.QueueingStrategy,
			NamespaceSelector:       in.NamespaceSelector,
			FlavorFungibility:       in.FlavorFungibility,
			Preemption:              in.Preemption,
			AdmissionChecks:         in.AdmissionChecks,
			AdmissionChecksStrategy: in.AdmissionChecksStrategy,
			StopPolicy:              in.StopPolicy,
			FairSharing:             in.FairSharing,
		},
	}

	// TODO check if this does something?
	for i, resourceGroup := range clusterQueue.Spec.ResourceGroups {
		for j, flavor := range resourceGroup.Flavors {
			resourceMap := make(map[string]kueuev1beta1.ResourceQuota)
			for _, flavorResource := range flavor.Resources {
				resourceMap[flavorResource.Name.String()] = flavorResource
			}
			var resources []kueuev1beta1.ResourceQuota
			for _, resourceName := range resourceGroup.CoveredResources {
				if res, exists := resourceMap[resourceName.String()]; exists {
					resources = append(resources, res)
				}
			}
			clusterQueue.Spec.ResourceGroups[i].Flavors[j].Resources = resources
		}
	}

	return clusterQueue, nil
}

func (c ClusterQueueConverter) CreateList() client.ObjectList {
	return &kueuev1beta1.ClusterQueueList{
		TypeMeta: metav1.TypeMeta{
			APIVersion: kueuev1beta1.SchemeGroupVersion.String(),
			Kind:       "ClusterQueueList",
		},
	}
}

func (c ClusterQueueConverter) Mutate(desired, actual *kueuev1beta1.ClusterQueue) {
	actual.Spec = desired.Spec
}

// LocalQueue

type LocalQueueConverter struct{}

func (c LocalQueueConverter) Convert(input client.ObjectKey) (*kueuev1beta1.LocalQueue, error) {
	return &kueuev1beta1.LocalQueue{
		ObjectMeta: metav1.ObjectMeta{
			Name:      input.Name,
			Namespace: input.Namespace,
		},
		Spec: kueuev1beta1.LocalQueueSpec{
			ClusterQueue: kueuev1beta1.ClusterQueueReference(input.Name),
		},
	}, nil
}

func (c LocalQueueConverter) CreateList() client.ObjectList {
	return &kueuev1beta1.LocalQueueList{
		TypeMeta: metav1.TypeMeta{
			APIVersion: kueuev1beta1.SchemeGroupVersion.String(),
			Kind:       "LocalQueueList",
		},
	}
}

func (c LocalQueueConverter) Mutate(desired, actual *kueuev1beta1.LocalQueue) {
	// Nothing updated at the moment
}

// WorkloadPriorityClass

type WorkloadPriorityClassConverter struct{}

func (c WorkloadPriorityClassConverter) Convert(input kueuev1beta1.WorkloadPriorityClass) (*kueuev1beta1.WorkloadPriorityClass, error) {
	return input.DeepCopy(), nil
}

func (c WorkloadPriorityClassConverter) CreateList() client.ObjectList {
	return &kueuev1beta1.WorkloadPriorityClassList{
		TypeMeta: metav1.TypeMeta{
			APIVersion: kueuev1beta1.SchemeGroupVersion.String(),
			Kind:       "WorkloadPriorityClassList",
		},
	}
}

func (c WorkloadPriorityClassConverter) Mutate(desired, actual *kueuev1beta1.WorkloadPriorityClass) {
	actual.Value = desired.Value
}

// Topology

type TopologyConverter struct{}

func (c TopologyConverter) Convert(input kaiwo.Topology) (*kueuev1alpha1.Topology, error) {
	levels := make([]kueuev1alpha1.TopologyLevel, len(input.Spec.Levels))
	for i, l := range input.Spec.Levels {
		levels[i] = kueuev1alpha1.TopologyLevel{
			NodeLabel: l.NodeLabel,
		}
	}

	return &kueuev1alpha1.Topology{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Topology",
			APIVersion: "kueue.x-k8s.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: input.Name,
		},
		Spec: kueuev1alpha1.TopologySpec{
			Levels: levels,
		},
	}, nil
}

func (c TopologyConverter) CreateList() client.ObjectList {
	return &kueuev1alpha1.TopologyList{
		TypeMeta: metav1.TypeMeta{
			APIVersion: kueuev1alpha1.SchemeGroupVersion.String(),
			Kind:       "TopologyList",
		},
	}
}

func (c TopologyConverter) Mutate(desired, actual *kueuev1alpha1.Topology) {
	actual.Spec = desired.Spec
}

// ResourceFlavor

type ResourceFlavorConverter struct{}

func (c ResourceFlavorConverter) Convert(input kaiwo.ResourceFlavorSpec) (*kueuev1beta1.ResourceFlavor, error) {
	nodeLabels := make(map[string]string, len(input.NodeLabels))
	for k, v := range input.NodeLabels {
		nodeLabels[k] = v
	}

	var topologyRef *kueuev1beta1.TopologyReference
	if input.TopologyName != "" {
		ref := kueuev1beta1.TopologyReference(input.TopologyName)
		topologyRef = &ref
	} else {
		ref := kueuev1beta1.TopologyReference(common.DefaultTopologyName)
		topologyRef = &ref
		nodeLabels[common.DefaultKaiwoWorkerLabel] = "true"
	}

	// Only schedule on nodes with KaiwoNode status = Ready
	nodeLabels[common.NodeStatusLabelKey] = string(kaiwo.KaiwoNodeStatusReady)

	return &kueuev1beta1.ResourceFlavor{
		ObjectMeta: metav1.ObjectMeta{Name: input.Name},
		Spec: kueuev1beta1.ResourceFlavorSpec{
			NodeLabels:   nodeLabels,
			TopologyName: topologyRef,
		},
	}, nil
}

func (c ResourceFlavorConverter) CreateList() client.ObjectList {
	return &kueuev1beta1.ResourceFlavorList{
		TypeMeta: metav1.TypeMeta{
			APIVersion: kueuev1beta1.SchemeGroupVersion.String(),
			Kind:       "ResourceFlavorList",
		},
	}
}

func (c ResourceFlavorConverter) Mutate(desired *kueuev1beta1.ResourceFlavor, actual *kueuev1beta1.ResourceFlavor) {
	actual.Spec = desired.Spec
}

func ConstructDefaultResourceFlavors(ctx context.Context, clusterContext common.ClusterContext) ([]kaiwo.ResourceFlavorSpec, map[string]kueuev1beta1.FlavorQuotas, error) {
	logger := log.FromContext(ctx)
	config := common.ConfigFromContext(ctx)

	var resourceFlavors []kaiwo.ResourceFlavorSpec
	nodePoolResources := make(map[string]kueuev1beta1.FlavorQuotas)
	nodePools := make(map[string][]kaiwo.KaiwoNode)

	resourceAggregates := make(map[string]map[corev1.ResourceName]*resource.Quantity)

	if config.Nodes.ExcludeMasterNodesFromNodePools {
		logger.Info("Excluding master/control-plane nodes from nodepools. Set config.nodes.excludeMasterNodesFromNodePools=false to include them")
	} else {
		logger.Info("Including master/control-plane nodes in nodepools. Set config.nodes.excludeMasterNodesFromNodePools=true to exclude them")
	}

	for _, node := range clusterContext.Nodes {
		if node.IsUnschedulable() {
			logger.Info("Skipping cordoned node", "node", node.Name)
			continue
		}
		if config.Nodes.ExcludeMasterNodesFromNodePools && node.Status.IsControlPlane {
			logger.Info("Skipping control plane node", "node", node.Name)
			continue
		}

		flavorName := node.Status.KueueFlavorName

		// Track node membership in the nodepool
		nodePools[flavorName] = append(nodePools[flavorName], node)
		logger.Info("Node added to node pool", "node", node.Name, "nodePool", flavorName)

		gpuInfo := node.Status.Resources.Gpus

		nodeHasGpus := gpuInfo != nil && gpuInfo.LogicalCount > 0

		if _, exists := resourceAggregates[flavorName]; !exists {
			resourceAggregates[flavorName] = map[corev1.ResourceName]*resource.Quantity{
				corev1.ResourceCPU:    resource.NewQuantity(0, resource.DecimalSI),
				corev1.ResourceMemory: resource.NewQuantity(0, resource.BinarySI),
			}
			if nodeHasGpus {
				resourceAggregates[flavorName][gpuInfo.ResourceName] = resource.NewQuantity(0, resource.DecimalSI)
			}
		}

		resourceAggregates[flavorName][corev1.ResourceCPU].Add(*node.Status.Resources.Cpu.Nominal)
		resourceAggregates[flavorName][corev1.ResourceMemory].Add(*node.Status.Resources.Memory.Nominal)

		if nodeHasGpus {
			resourceAggregates[flavorName][gpuInfo.ResourceName].Add(*resource.NewQuantity(int64(gpuInfo.LogicalCount), resource.DecimalSI))
		}
	}

	for flavorName := range nodePools {
		resourceQuotas := []kueuev1beta1.ResourceQuota{
			{
				Name:         corev1.ResourceCPU,
				NominalQuota: *resourceAggregates[flavorName][corev1.ResourceCPU],
			},
			{
				Name:         corev1.ResourceMemory,
				NominalQuota: *resourceAggregates[flavorName][corev1.ResourceMemory],
			},
		}

		for resourceName, quota := range resourceAggregates[flavorName] {
			if resourceName == corev1.ResourceCPU || resourceName == corev1.ResourceMemory {
				continue
			}
			resourceQuotas = append(resourceQuotas, kueuev1beta1.ResourceQuota{
				Name:         resourceName,
				NominalQuota: *quota,
			})
		}

		// Store in nodePoolResources
		nodePoolResources[flavorName] = kueuev1beta1.FlavorQuotas{
			Name:      kueuev1beta1.ResourceFlavorReference(flavorName),
			Resources: resourceQuotas,
		}
		node := nodePools[flavorName][0]
		gpuInfo := node.Status.Resources.Gpus
		flavor := kaiwo.ResourceFlavorSpec{
			Name: flavorName,
			NodeLabels: map[string]string{
				common.DefaultNodePoolLabelKey: flavorName,
			},
			TopologyName: common.DefaultTopologyName,
		}
		if !node.IsCpuOnlyNode() {
			if logicalVramPerGpu := gpuInfo.LogicalVramPerGpu; logicalVramPerGpu != nil {
				flavor.NodeLabels[common.NodeGpuLogicalVramLabelKey] = baseutils.QuantityToGi(*logicalVramPerGpu)
			}
			flavor.NodeLabels[common.NodeGpuModelLabelKey] = gpuInfo.Model
			if partitioned := gpuInfo.IsPartitioned; partitioned != nil {
				flavor.NodeLabels[common.NodeGpusPartitionedLabelKey] = strconv.FormatBool(*partitioned)
			}
		}

		// TODO: Look into why automatic scheduling is not working
		// At the moment, we are adding toleration to pod spec ourselves
		// https://kueue.sigs.k8s.io/docs/concepts/resource_flavor/#resourceflavor-tolerations-for-automatic-scheduling
		// if addTaints && !strings.HasPrefix(flavorName, "cpu-only") {
		// 	flavor.Tolerations = []corev1.Toleration{GPUToleration}
		// }

		resourceFlavors = append(resourceFlavors, flavor)
	}

	resourceFlavors = RemoveDuplicateResourceFlavors(resourceFlavors)

	// logger.Info("Final generated node pools", "nodePools", nodePools)
	logger.Info("Final generated node pool resources", "nodePoolResources", nodePoolResources)
	logger.Info("Final generated resource flavors", "resourceFlavors", resourceFlavors)

	return resourceFlavors, nodePoolResources, nil
}

func RemoveDuplicateResourceFlavors(flavors []kaiwo.ResourceFlavorSpec) []kaiwo.ResourceFlavorSpec {
	uniqueMap := make(map[string]kaiwo.ResourceFlavorSpec)
	for _, flavor := range flavors {
		uniqueMap[flavor.Name] = flavor
	}

	uniqueFlavors := make([]kaiwo.ResourceFlavorSpec, 0, len(uniqueMap))
	for _, flavor := range uniqueMap {
		uniqueFlavors = append(uniqueFlavors, flavor)
	}
	return uniqueFlavors
}

func ConstructClusterQueue(nodePoolResources map[string]kueuev1beta1.FlavorQuotas, name string, cohort string) kaiwo.ClusterQueue {
	var resourceGroups []kueuev1beta1.ResourceGroup
	coveredResources := make(map[corev1.ResourceName]struct{})

	// Convert map to slice
	var flavorQuotas []kueuev1beta1.FlavorQuotas
	for _, quota := range nodePoolResources {
		flavorQuotas = append(flavorQuotas, quota)

		// Extract resources dynamically
		for _, quotaResource := range quota.Resources {
			resourceName := quotaResource.Name
			normalizedResourceName := corev1.ResourceName(strings.TrimSpace(string(resourceName)))
			coveredResources[normalizedResourceName] = struct{}{}
		}
	}

	// Sort flavors: CPU-only nodes first, then GPU nodes by GPU count, then CPU, then memory
	sort.Slice(flavorQuotas, func(i, j int) bool {
		gpuCountI := getGPUCount(string(flavorQuotas[i].Name))
		gpuCountJ := getGPUCount(string(flavorQuotas[j].Name))
		cpuI := getCPUCount(string(flavorQuotas[i].Name))
		cpuJ := getCPUCount(string(flavorQuotas[j].Name))
		memoryI := getMemoryCount(string(flavorQuotas[i].Name))
		memoryJ := getMemoryCount(string(flavorQuotas[j].Name))

		if gpuCountI == 0 && gpuCountJ > 0 {
			return true
		}
		if gpuCountJ == 0 && gpuCountI > 0 {
			return false
		}
		if gpuCountI != gpuCountJ {
			return gpuCountI < gpuCountJ
		}
		if cpuI != cpuJ {
			return cpuI < cpuJ
		}
		return memoryI < memoryJ
	})

	// Convert collected resources to a slice for `CoveredResources`
	var coveredResourcesSlice []corev1.ResourceName
	for coveredResource := range coveredResources {
		coveredResourcesSlice = append(coveredResourcesSlice, coveredResource)
	}

	// Ensure every flavor has the same set of resources as coveredResources
	for i, flavor := range flavorQuotas {
		var missingResources []corev1.ResourceName

		for _, covered := range coveredResourcesSlice {
			found := false
			for _, quota := range flavor.Resources {
				if quota.Name == covered {
					found = true
					break
				}
			}
			if !found {
				missingResources = append(missingResources, covered)
			}
		}

		// If a resource is missing from a flavor, add it with zero quota
		for _, missing := range missingResources {
			flavorQuotas[i].Resources = append(flavorQuotas[i].Resources, kueuev1beta1.ResourceQuota{
				Name:         missing,
				NominalQuota: *resource.NewQuantity(0, resource.DecimalSI), // Set to zero
			})
		}
	}

	// Define the resource group dynamically based on collected resources
	resourceGroups = append(resourceGroups, kueuev1beta1.ResourceGroup{
		CoveredResources: coveredResourcesSlice,
		Flavors:          flavorQuotas,
	})

	// Create ClusterQueue
	return kaiwo.ClusterQueue{
		Name:       name,
		Namespaces: []string{},
		Spec: kaiwo.ClusterQueueSpec{
			NamespaceSelector: &metav1.LabelSelector{},
			Cohort:            kueuev1beta1.CohortReference(cohort),
			ResourceGroups:    resourceGroups,
		},
	}
}

func CreateDefaultTopology(ctx context.Context, c client.Client) ([]kaiwo.Topology, error) {
	defaultTopology := kaiwo.Topology{
		ObjectMeta: metav1.ObjectMeta{
			Name: common.DefaultTopologyName,
		},
		Spec: kaiwo.TopologySpec{
			Levels: []kueuev1alpha1.TopologyLevel{
				{NodeLabel: common.DefaultTopologyBlockLabel},
				{NodeLabel: common.DefaultTopologyRackLabel},
				{NodeLabel: common.DefaultTopologyHostLabel},
			},
		},
	}
	return []kaiwo.Topology{defaultTopology}, nil
}
