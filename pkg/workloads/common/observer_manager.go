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

	"golang.org/x/sync/errgroup"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	units := make([]UnitStatus, len(om.observers))
	g, ctx := errgroup.WithContext(ctx)

	for i := range om.observers {
		i := i // capture loop variable
		g.Go(func() error {
			u, err := om.observers[i].Observe(ctx, c)
			if err != nil {
				// Keep the entry with Unknown + error details, don't fail the whole operation
				units[i] = UnitStatus{
					Name:    getObserverName(om.observers[i]),
					Kind:    om.observers[i].Kind(),
					Group:   getObserverGroup(om.observers[i]),
					Phase:   UnitUnknown,
					Reason:  ReasonObserveError,
					Message: err.Error(),
				}
				// Don't return err: continue collecting
				return nil
			}
			// Set the common fields at this level
			u.Name = getObserverName(om.observers[i])
			u.Kind = om.observers[i].Kind()
			u.Group = getObserverGroup(om.observers[i])
			units[i] = u
			return nil
		})
	}

	_ = g.Wait() // Always returns nil since we don't return errors from goroutines
	return units, nil
}

// Helper functions to extract name and group from observer structs using interface methods
func getObserverName(observer Observer) string {
	// Use type assertions to access NamespacedName field
	switch o := observer.(type) {
	case interface{ GetNamespacedName() types.NamespacedName }:
		return o.GetNamespacedName().Name
	default:
		// For observers that follow the NamespacedName field pattern
		if hasNamespacedName, ok := observer.(interface{ NamespacedName types.NamespacedName }); ok {
			// This won't work directly, need field access through reflection or type switches
			_ = hasNamespacedName
		}
		
		// Fallback to type switches for known observer types
		switch obs := observer.(type) {
		case *PVCObserver:
			return obs.NamespacedName.Name
		case *DownloadJobObserver:
			return obs.NamespacedName.Name
		}
		
		// Try to get from external packages through interface pattern
		if nameProvider, ok := observer.(interface{ GetName() string }); ok {
			return nameProvider.GetName()
		}
		return "unknown"
	}
}

func getObserverGroup(observer Observer) UnitGroup {
	// Similar pattern for Group
	switch o := observer.(type) {
	case interface{ GetGroup() UnitGroup }:
		return o.GetGroup()
	default:
		// Fallback to type switches for known observer types
		switch obs := observer.(type) {
		case *PVCObserver:
			return obs.Group
		case *DownloadJobObserver:
			return obs.Group
		}
		return GroupWorkload
	}
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
				GroupPrereqs,
			))
		}
		if commonSpec.Storage.HasHfDownloads() {
			manager.AddObserver(NewPVCObserver(
				types.NamespacedName{
					Name:      baseutils.FormatNameWithPostfix(objKey.Name, HfStoragePostfix),
					Namespace: objKey.Namespace,
				},
				GroupPrereqs,
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
				Group: GroupPrereqs,
			})
		}
	}

	// Add workload observers based on workload type and spec
	// Note: These constructors are defined in their respective packages:
	// - NewJobObserver and NewRayJobObserver from pkg/workloads/job
	// - NewDeploymentObserver and NewRayServiceObserver from pkg/workloads/service
	switch wl := workload.GetKaiwoWorkloadObject().(type) {
	case *v1alpha1.KaiwoJob:
		if wl.Spec.IsRayJob() {
			// TODO: Import NewRayJobObserver from pkg/workloads/job and add observer
			// manager.AddObserver(NewRayJobObserver(objKey, GroupWorkload))
		} else {
			// TODO: Import NewJobObserver from pkg/workloads/job and add observer
			// manager.AddObserver(NewJobObserver(objKey, GroupWorkload))
		}
	case *v1alpha1.KaiwoService:
		if wl.Spec.IsRayService() {
			// TODO: Import NewRayServiceObserver from pkg/workloads/service and add observer
			// manager.AddObserver(NewRayServiceObserver(objKey, GroupWorkload))
		} else {
			// TODO: Import NewDeploymentObserver from pkg/workloads/service and add observer
			// manager.AddObserver(NewDeploymentObserver(objKey, GroupWorkload))
		}
	}

	return manager
}

// ObserveWorkloadWithNewPattern provides the new observation pattern as an alternative
// to the existing ObserveOverallStatus function
func ObserveWorkloadWithNewPattern(ctx context.Context, c client.Client, workload KaiwoWorkload) (WorkloadPhase, []UnitStatus, error) {
	// Use the unified observation function
	observation, err := ObserveWorkload(ctx, c, workload)
	if err != nil {
		return "", nil, fmt.Errorf("failed to observe workload: %w", err)
	}

	return observation.Phase, observation.Units, nil
}


// DownloadJobObserver observes download Jobs
type DownloadJobObserver struct {
	NamespacedName types.NamespacedName
	Group          UnitGroup
}

func (o *DownloadJobObserver) Kind() string {
	return "Job"
}

func (o *DownloadJobObserver) Observe(ctx context.Context, c client.Client) (UnitStatus, error) {
	// Observe a regular Kubernetes Job used for downloading
	var j batchv1.Job
	if err := c.Get(ctx, o.NamespacedName, &j); apierrors.IsNotFound(err) {
		return UnitStatus{
			Phase: UnitPending,
		}, nil
	} else if err != nil {
		return UnitStatus{
			Phase:   UnitUnknown,
			Reason:  ReasonGetError,
			Message: err.Error(),
		}, nil
	}

	// Check for terminal conditions
	for _, condition := range j.Status.Conditions {
		switch condition.Type {
		case batchv1.JobComplete:
			if condition.Status == corev1.ConditionTrue {
				return UnitStatus{
					Phase:              UnitSucceeded,
					Ready:              true,
					Reason:             condition.Reason,
					Message:            condition.Message,
					ObservedGeneration: j.Status.ObservedGeneration,
				}, nil
			}
		case batchv1.JobFailed:
			if condition.Status == corev1.ConditionTrue {
				return UnitStatus{
					Phase:              UnitFailed,
					Reason:             condition.Reason,
					Message:            condition.Message,
					ObservedGeneration: j.Status.ObservedGeneration,
				}, nil
			}
		}
	}

	// Check if job is actively running
	if j.Status.Active > 0 {
		return UnitStatus{
			Phase:              UnitProgressing,
			ObservedGeneration: j.Status.ObservedGeneration,
		}, nil
	}

	// Default to pending
	return UnitStatus{
		Phase:              UnitPending,
		ObservedGeneration: j.Status.ObservedGeneration,
	}, nil
}
