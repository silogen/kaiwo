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
	for j := 0; j < len(i.Commands); j++ {

		obj, err := i.Commands[j].Build()
		if err != nil {
			return nil, fmt.Errorf("error building object: %v", err)
		}
		i.Commands[j].SetObject(obj)

		localContext := ctx // .WithValue(ctx, "object", obj.GetObjectKind())
		logger := log.FromContext(localContext)

		isOwner := false
		if obj == i.Commands[j].GetOwner() {
			isOwner = true
		}

		exists, err := i.Commands[j].ResourceExists(localContext, k8sClient)
		if err != nil {
			logger.Error(err, "failed to check if resource exists")
			return nil, err
		}

		if !exists && !isOwner {
			// Create if object doesn't exist, and it is a dependent object (don't try to recreate the owner custom resource)
			if err := i.Commands[j].Create(localContext, k8sClient); err != nil {
				logger.Error(err, "failed to create resource")
				return nil, err
			}

			if err := ctrl.SetControllerReference(i.Commands[j].GetOwner(), obj, scheme); err != nil {
				return nil, fmt.Errorf("failed to set controller reference on resource: %v", err)
			}
		} else if exists {
			// Update all objects
			if err := i.Commands[j].Update(localContext, k8sClient); err != nil {
				logger.Error(err, "failed to update resource")
				return nil, err
			}
		}

		currentResult := i.Commands[j].GetCurrentReconcileResult()
		if currentResult != nil {
			// If a command needs to interrupt with a result, return it
			return currentResult, nil
		}
	}
	return nil, nil
}

func (i *CommandInvoker) BuildAllResources() ([]client.Object, error) {
	var resources []client.Object
	for j := 0; j < len(i.Commands); j++ {
		obj, err := i.Commands[j].Build()
		if err != nil {
			return resources, fmt.Errorf("error building: %v", err)
		}
		if obj == nil {
			continue
		}
		i.Commands[j].SetObject(obj)
		resources = append(resources, obj)
	}
	return resources, nil
}
