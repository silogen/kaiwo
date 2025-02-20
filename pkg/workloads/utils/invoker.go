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

package workloadutils

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"

	baseutils "github.com/silogen/kaiwo/pkg/utils"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type CommandInvoker struct {
	Commands []Command
}

func (i *CommandInvoker) AddCommand(cmd Command) {
	i.Commands = append(i.Commands, cmd)
}

func (i *CommandInvoker) Run(ctx context.Context, k8sClient client.Client, scheme *runtime.Scheme) (*ctrl.Result, error) {
	logger := log.FromContext(ctx)
	var owner client.Object

	for j := 0; j < len(i.Commands); j++ {

		//kind, err := i.Commands[j].GetGVK(scheme)
		//if err != nil {
		//	return nil, baseutils.LogErrorf(logger, "error getting GVK: %w", err)
		//}

		//localLogger := logger.WithValues(
		//	"kind", kind.Kind,
		//	"version", kind.Version,
		//	"group", kind.Group,
		//)

		// localCtx := log.IntoContext(ctx, localLogger)

		desired, err := i.Commands[j].Build(ctx, k8sClient)
		if err != nil {
			return nil, baseutils.LogErrorf(logger, "error building object", err)
		}
		if desired == nil {
			return nil, baseutils.LogErrorf(logger, "error building object", err)
		}

		descriptor, err := baseutils.GetObjectDescriptor(*scheme, desired)
		if err != nil {
			return nil, baseutils.LogErrorf(logger, "error getting object descriptor", err)
		}

		// localLogger = logger.WithValues("object", descriptor)

		objectKey := i.Commands[j].GetObjectKey()

		actual := i.Commands[j].GetEmptyObject()
		exists := false

		if err := k8sClient.Get(ctx, objectKey, actual); err != nil {
			// Try to fetch the object to retrieve the latest actual state
			if !errors.IsNotFound(err) {
				return nil, baseutils.LogErrorf(logger, "error getting object", err)
			}
		} else {
			exists = true
		}

		if exists {
			// Update the object
			retries := 0
			for {
				if err := i.Commands[j].Update(ctx, k8sClient, actual); err == nil {
					// Updated successfully
					break
				} else if !errors.IsConflict(err) || retries > 3 {
					return nil, baseutils.LogErrorf(logger, "failed to update resource", err)
				} else {
					logger.Error(err, "Failed to update, retrying")
					retries += 1
					if err := k8sClient.Get(ctx, objectKey, actual); err != nil {
						// Try to fetch the object to retrieve the latest actual state
						if !errors.IsNotFound(err) {
							return nil, baseutils.LogErrorf(logger, "error getting object", err)
						}
					}
				}
			}
		} else {
			// Create the object
			if owner != nil {
				// Set the owner
				if err := ctrl.SetControllerReference(owner, desired, scheme); err != nil {
					return nil, baseutils.LogErrorf(logger, "failed to set controller reference on resource", err)
				}
			}

			logger.Info("Creating resource", "resource", descriptor)
			if err := k8sClient.Create(ctx, desired); err != nil {
				return nil, baseutils.LogErrorf(logger, "error creating object", err)
			}
		}

		if owner == nil && j == 0 {
			owner = actual
		}
		currentResult, _ := i.Commands[j].GetCurrentReconcileResult(ctx, k8sClient)
		if currentResult != nil {
			// If a command needs to interrupt with a result, return it
			return currentResult, nil
		}
	}
	return nil, nil
}

func (i *CommandInvoker) BuildAllResources(ctx context.Context, scheme *runtime.Scheme, k8sClient client.Client) ([]client.Object, error) {
	var resources []client.Object
	logger := log.FromContext(ctx)
	for j := 0; j < len(i.Commands); j++ {

		obj, err := i.Commands[j].Build(ctx, k8sClient)
		if err != nil {
			return resources, baseutils.LogErrorf(logger, "error building", err)
		}
		if obj == nil {
			continue
		}

		resources = append(resources, obj)
	}
	return resources, nil
}
