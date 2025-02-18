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
	"fmt"

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
	for j := 0; j < len(i.Commands); j++ {
		kind, err := i.Commands[j].GetGVK(scheme)
		if err != nil {
			return nil, fmt.Errorf("error getting GVK: %w", err)
		}

		localLogger := logger.WithValues(
			"kind", kind.Kind,
			"version", kind.Version,
			"group", kind.Group,
		)

		localCtx := log.IntoContext(ctx, localLogger)

		localLogger.Info("Building")

		obj, err := i.Commands[j].Build(localCtx, k8sClient)
		i.Commands[j].SetDesired(obj)

		localLogger = logger.WithValues(
			"kind", kind.Kind,
			"version", kind.Version,
			"group", kind.Group,
			"name", obj.GetName(),
			"namespace", obj.GetNamespace(),
		)

		localLogger.Info("Built")

		if err != nil {
			return nil, fmt.Errorf("error building object: %w", err)
		}
		i.Commands[j].SetDesired(obj)

		isOwner := false
		if obj == i.Commands[j].GetOwner() {
			isOwner = true
		}

		localCtx = log.IntoContext(ctx, localLogger)

		exists, err := i.Commands[j].ResourceExists(localCtx, k8sClient)
		if err != nil {
			return nil, fmt.Errorf("error checking if resource exists: %w", err)
		}

		if !exists && !isOwner {
			// Create if object doesn't exist, and it is a dependent object (don't try to recreate the owner custom resource)
			if err := i.Commands[j].Create(localCtx, k8sClient); err != nil {
				localLogger.Error(err, "failed to create resource")
				return nil, err
			}

			if err := ctrl.SetControllerReference(i.Commands[j].GetOwner(), obj, scheme); err != nil {
				return nil, fmt.Errorf("failed to set controller reference on resource: %w", err)
			}
		} else if exists {
			// Update all objects
			if err := i.Commands[j].Update(localCtx, k8sClient); err != nil {
				return nil, fmt.Errorf("failed to update resource: %w", err)
			}
		}

		currentResult := i.Commands[j].GetCurrentReconcileResult(ctx)
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

		kind, err := i.Commands[j].GetGVK(scheme)
		if err != nil {
			return nil, fmt.Errorf("error getting GVK: %w", err)
		}

		localLogger := logger.WithValues(
			"gvk", kind,
			"objectKey", i.Commands[j].GetObjectKey(),
		)

		localCtx := log.IntoContext(ctx, localLogger)

		localLogger.Info("Building")

		obj, err := i.Commands[j].Build(localCtx, k8sClient)
		if err != nil {
			return resources, fmt.Errorf("error building: %w", err)
		}
		if obj == nil {
			continue
		}

		localLogger.Info("Built successfully")

		i.Commands[j].SetDesired(obj)
		resources = append(resources, obj)
	}
	return resources, nil
}
