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

		localContext := context.WithValue(ctx, "object", obj.GetObjectKind())
		logger := log.FromContext(localContext)

		exists, err := i.Commands[j].ResourceExists(localContext, k8sClient)
		if err != nil {
			logger.Error(err, "failed to check if resource exists")
			return nil, err
		}

		if !exists {
			if err := i.Commands[j].Create(localContext, k8sClient); err != nil {
				logger.Error(err, "failed to create resource")
				return nil, err
			}

			if obj != nil {
				if err := ctrl.SetControllerReference(i.Commands[j].GetOwner(), obj, scheme); err != nil {
				} else {
					if err := i.Commands[j].Update(localContext, k8sClient); err != nil {
						logger.Error(err, "failed to update resource")
						return nil, err
					}
				}
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
