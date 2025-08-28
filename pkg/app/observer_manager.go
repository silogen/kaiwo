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

package app

import (
	"context"

	jobs "github.com/silogen/kaiwo/pkg/workloads/job"
	services "github.com/silogen/kaiwo/pkg/workloads/service"

	"github.com/silogen/kaiwo/pkg/api"
	"github.com/silogen/kaiwo/pkg/observe"
	"github.com/silogen/kaiwo/pkg/storage"

	"golang.org/x/sync/errgroup"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

// ObserverManager manages all observers for a workload
type ObserverManager struct {
	observers []observe.Observer
}

func NewObserverManager() *ObserverManager {
	return &ObserverManager{
		observers: make([]observe.Observer, 0),
	}
}

func (om *ObserverManager) AddObserver(observer observe.Observer) {
	om.observers = append(om.observers, observer)
}

func (om *ObserverManager) ObserveAll(ctx context.Context, c client.Client) ([]observe.UnitStatus, error) {
	units := make([]observe.UnitStatus, len(om.observers))
	g, ctx := errgroup.WithContext(ctx)

	for i := range om.observers {
		// capture loop variable
		g.Go(func() error {
			u, err := om.observers[i].Observe(ctx, c)
			if err != nil {
				// Keep the entry with Unknown + error details, don't fail the whole operation
				units[i] = observe.UnitStatus{
					Name:    getObserverName(om.observers[i]),
					Kind:    om.observers[i].Kind(),
					Group:   getObserverGroup(om.observers[i]),
					Phase:   observe.UnitUnknown,
					Reason:  observe.ReasonObserveError,
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
func getObserverName(observer observe.Observer) string {
	if id, ok := observer.(observe.HasIdentity); ok {
		return id.GetNamespacedName().Name
	}
	panic("observer does not have identity")
}

func getObserverGroup(observer observe.Observer) observe.UnitGroup {
	if id, ok := observer.(observe.HasIdentity); ok {
		return id.GetGroup()
	}
	panic("observer does not have identity")
}

// BuildObserversForWorkload creates observers for a given workload
func BuildObserversForWorkload(workload api.KaiwoWorkload) *ObserverManager {
	manager := NewObserverManager()
	commonSpec := workload.GetCommonSpec()
	objKey := client.ObjectKeyFromObject(workload.GetKaiwoWorkloadObject())

	// Add storage observers (prereqs)
	if commonSpec.Storage != nil && commonSpec.Storage.StorageEnabled {
		if commonSpec.Storage.HasData() {
			manager.AddObserver(storage.NewPVCObserver(
				types.NamespacedName{
					Name:      baseutils.FormatNameWithPostfix(objKey.Name, storage.DataStoragePostfix),
					Namespace: objKey.Namespace,
				},
				observe.GroupPrereqs,
			))
		}
		if commonSpec.Storage.HasHfDownloads() {
			manager.AddObserver(storage.NewPVCObserver(
				types.NamespacedName{
					Name:      baseutils.FormatNameWithPostfix(objKey.Name, storage.HfStoragePostfix),
					Namespace: objKey.Namespace,
				},
				observe.GroupPrereqs,
			))
		}
		if commonSpec.Storage.HasDownloads() {
			key := client.ObjectKey{
				Name:      baseutils.FormatNameWithPostfix(objKey.Name, storage.DownloadPostfix),
				Namespace: objKey.Namespace,
			}
			manager.AddObserver(observe.NewJobObserver(key, observe.GroupPrereqs, false)) // Download jobs don't use Kueue
		}
	}

	switch wl := workload.GetKaiwoWorkloadObject().(type) {
	case *v1alpha1.KaiwoJob:
		if wl.Spec.IsRayJob() {
			manager.AddObserver(jobs.NewRayJobObserver(objKey, observe.GroupWorkload))
		} else {
			manager.AddObserver(observe.NewJobObserver(objKey, observe.GroupWorkload, true)) // Workload jobs use Kueue
		}
	case *v1alpha1.KaiwoService:
		if wl.Spec.IsRayService() {
			manager.AddObserver(services.NewRayServiceObserver(objKey, observe.GroupWorkload))
		} else {
			manager.AddObserver(services.NewDeploymentObserver(objKey, observe.GroupWorkload))
		}
	}

	return manager
}

// ObserveWorkload provides a unified observation function that returns phase + conditions + units
func ObserveWorkload(ctx context.Context, c client.Client, workload api.KaiwoWorkload) (observe.Observation, error) {
	mgr := BuildObserversForWorkload(workload)
	units, _ := mgr.ObserveAll(ctx, c)
	agg := observe.Reduce(units)
	phase, conditions := observe.Decide(workload.GetKaiwoWorkloadObject(), agg, units)
	return observe.Observation{
		Phase:      phase,
		Units:      units,
		Conditions: conditions,
	}, nil
}
