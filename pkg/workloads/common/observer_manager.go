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

package common

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

// ObserverManager manages all observers for a workload
type ObserverManager struct {
	observers []Observer
}

func NewObserverManager() *ObserverManager {
	return &ObserverManager{
		observers: make([]Observer, 0),
	}
}

func (om *ObserverManager) AddObserver(observer Observer) {
	om.observers = append(om.observers, observer)
}

func (om *ObserverManager) ObserveAll(ctx context.Context, c client.Client) ([]UnitStatus, error) {
	var units []UnitStatus
	for _, observer := range om.observers {
		unit, err := observer.Observe(ctx, c)
		if err != nil {
			return nil, fmt.Errorf("failed to observe unit: %w", err)
		}
		units = append(units, unit)
	}
	return units, nil
}

// BuildObserversForWorkload creates observers for a given workload
func BuildObserversForWorkload(workload KaiwoWorkload) *ObserverManager {
	manager := NewObserverManager()
	commonSpec := workload.GetCommonSpec()
	objKey := client.ObjectKeyFromObject(workload.GetKaiwoWorkloadObject())

	// Add storage observers (prereqs)
	if commonSpec.Storage != nil && commonSpec.Storage.StorageEnabled {
		if commonSpec.Storage.HasData() {
			manager.AddObserver(NewPVCObserver(
				types.NamespacedName{
					Name:      baseutils.FormatNameWithPostfix(objKey.Name, DataStoragePostfix),
					Namespace: objKey.Namespace,
				},
				"Prereqs",
			))
		}
		if commonSpec.Storage.HasHfDownloads() {
			manager.AddObserver(NewPVCObserver(
				types.NamespacedName{
					Name:      baseutils.FormatNameWithPostfix(objKey.Name, HfStoragePostfix),
					Namespace: objKey.Namespace,
				},
				"Prereqs",
			))
		}
		if commonSpec.Storage.HasDownloads() {
			// For download jobs, we need a generic observer since the download job
			// will be a regular Kubernetes Job
			manager.AddObserver(&DownloadJobObserver{
				NamespacedName: types.NamespacedName{
					Name:      baseutils.FormatNameWithPostfix(objKey.Name, "download"),
					Namespace: objKey.Namespace,
				},
				Group: "Prereqs",
			})
		}
	}

	// Note: For now, we can create observers for the main resource types
	// In the future, this could be enhanced to dynamically determine the correct observer
	// based on the workload handler configuration

	// Add workload observers based on workload type
	gvk := workload.GetKaiwoWorkloadObject().GetObjectKind().GroupVersionKind()

	switch gvk.Kind {
	case "KaiwoJob":
		// For now, add a generic Job observer
		// The actual reconciler will determine whether it's a BatchJob or RayJob
		manager.AddObserver(&GenericJobObserver{
			NamespacedName: objKey,
			Group:          "Workload",
		})
	case "KaiwoService":
		// For now, add a generic Service observer
		// The actual reconciler will determine whether it's a Deployment or RayService
		manager.AddObserver(&GenericServiceObserver{
			NamespacedName: objKey,
			Group:          "Workload",
		})
	}

	return manager
}

// ObserveWorkloadWithNewPattern provides the new observation pattern as an alternative
// to the existing ObserveOverallStatus function
func ObserveWorkloadWithNewPattern(ctx context.Context, c client.Client, workload KaiwoWorkload) (WorkloadPhase, []UnitStatus, error) {
	// Build observers for this workload
	manager := BuildObserversForWorkload(workload)

	// Observe all units
	units, err := manager.ObserveAll(ctx, c)
	if err != nil {
		return "", nil, fmt.Errorf("failed to observe units: %w", err)
	}

	// Aggregate results
	agg := Reduce(units)

	// Decide phase and conditions
	phase, _ := Decide(workload.GetKaiwoWorkloadObject(), agg, units)

	return phase, units, nil
}

// GenericJobObserver can observe either BatchJob or RayJob
type GenericJobObserver struct {
	NamespacedName types.NamespacedName
	Group          string
}

func (o *GenericJobObserver) Observe(ctx context.Context, c client.Client) (UnitStatus, error) {
	// Try to get as RayJob first
	var rayJob v1alpha1.KaiwoJob
	if err := c.Get(ctx, o.NamespacedName, &rayJob); err == nil {
		if rayJob.Spec.IsRayJob() {
			// Observe RayJob - for now return a simple status
			return UnitStatus{
				Name:  o.NamespacedName.Name,
				Kind:  "RayJob",
				Group: o.Group,
				Phase: UnitProgressing, // Simplified for now
			}, nil
		} else {
			// Observe BatchJob - for now return a simple status
			return UnitStatus{
				Name:  o.NamespacedName.Name,
				Kind:  "Job",
				Group: o.Group,
				Phase: UnitProgressing, // Simplified for now
			}, nil
		}
	}

	return UnitStatus{
		Name:  o.NamespacedName.Name,
		Kind:  "Job",
		Group: o.Group,
		Phase: UnitPending,
	}, nil
}

// GenericServiceObserver can observe either Deployment or RayService
type GenericServiceObserver struct {
	NamespacedName types.NamespacedName
	Group          string
}

func (o *GenericServiceObserver) Observe(ctx context.Context, c client.Client) (UnitStatus, error) {
	// Try to get as KaiwoService first
	var kaiwoService v1alpha1.KaiwoService
	if err := c.Get(ctx, o.NamespacedName, &kaiwoService); err == nil {
		if kaiwoService.Spec.IsRayService() {
			// Observe RayService - for now return a simple status
			return UnitStatus{
				Name:  o.NamespacedName.Name,
				Kind:  "RayService",
				Group: o.Group,
				Phase: UnitProgressing, // Simplified for now
			}, nil
		} else {
			// Observe Deployment - for now return a simple status
			return UnitStatus{
				Name:  o.NamespacedName.Name,
				Kind:  "Deployment",
				Group: o.Group,
				Phase: UnitProgressing, // Simplified for now
			}, nil
		}
	}

	return UnitStatus{
		Name:  o.NamespacedName.Name,
		Kind:  "Service",
		Group: o.Group,
		Phase: UnitPending,
	}, nil
}

// DownloadJobObserver observes download Jobs
type DownloadJobObserver struct {
	NamespacedName types.NamespacedName
	Group          string
}

func (o *DownloadJobObserver) Observe(ctx context.Context, c client.Client) (UnitStatus, error) {
	// This would observe a regular Kubernetes Job used for downloading
	// For now, return a simple status
	return UnitStatus{
		Name:  o.NamespacedName.Name,
		Kind:  "Job",
		Group: o.Group,
		Phase: UnitProgressing, // Simplified for now
	}, nil
}
