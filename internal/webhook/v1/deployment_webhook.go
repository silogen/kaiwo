/*
MIT License

Copyright (c) 2025 Advanced Micro Devices, Inc.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package v1

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// nolint:unused
// log is for logging in this package.
var deploymentlog = logf.Log.WithName("deployment-resource")

// SetupDeploymentWebhookWithManager registers the webhook for Deployment in the manager.
func SetupDeploymentWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&appsv1.Deployment{}).
		WithDefaulter(&DeploymentCustomDefaulter{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:path=/mutate-apps-v1-deployment,mutating=true,failurePolicy=fail,sideEffects=None,groups=apps,resources=deployments,verbs=create;update,versions=v1,name=mdeployment-v1.kb.io,admissionReviewVersions=v1

// DeploymentCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind Deployment when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type DeploymentCustomDefaulter struct {
	// TODO(user): Add more fields as needed for defaulting
}

var _ webhook.CustomDefaulter = &DeploymentCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind Deployment.
func (d *DeploymentCustomDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	deployment, ok := obj.(*appsv1.Deployment)

	if !ok {
		return fmt.Errorf("expected an Deployment object but got %T", obj)
	}
	deploymentlog.Info("Defaulting for Deployment", "name", deployment.GetName())

	// TODO(user): fill in your defaulting logic.

	return nil
}

// TODO: refactor the following for Deployments (removed from job_webhook.go)
// func (j *JobWebhook) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
// 	newJob, ok := newObj.(*batchv1.Job)
// 	if !ok {
// 		return nil, fmt.Errorf("expected a Job object but got %T", newObj)
// 	}
// 	oldJob, ok := oldObj.(*batchv1.Job)
// 	if !ok {
// 		return nil, fmt.Errorf("expected a Job object but got %T", oldObj)
// 	}

// 	// Allow modification only if the update is from Kueue
// 	req, err := admission.RequestFromContext(ctx)
// 	if err == nil {
// 		if strings.HasPrefix(req.UserInfo.Username, "system:serviceaccount:kueue-") {
// 			return nil, nil
// 		}
// 	}

// 	if len(newJob.Spec.Template.Spec.Containers) != len(oldJob.Spec.Template.Spec.Containers) {
// 		return nil, fmt.Errorf("changing the number of containers is not allowed")
// 	}
// 	// Prevent increasing GPU requests/limits
// 	for i, newContainer := range newJob.Spec.Template.Spec.Containers {
// 		oldContainer := oldJob.Spec.Template.Spec.Containers[i]

// 		for _, gpuKey := range gpuKeys {
// 			if newLimit, newExists := newContainer.Resources.Limits[gpuKey]; newExists {
// 				if oldLimit, oldExists := oldContainer.Resources.Limits[gpuKey]; oldExists {
// 					if newLimit.Cmp(oldLimit) != 0 {
// 						return nil, fmt.Errorf("increasing/decreasing GPU limits is not allowed")
// 					}
// 				}
// 			}

// 			if newRequest, newExists := newContainer.Resources.Requests[gpuKey]; newExists {
// 				if oldRequest, oldExists := oldContainer.Resources.Requests[gpuKey]; oldExists {
// 					if newRequest.Cmp(oldRequest) != 0 {
// 						return nil, fmt.Errorf("increasing/decreasing GPU requests is not allowed")
// 					}
// 				}
// 			}
// 		}
// 	}

// 	joblog.Info("Job update validation passed", "JobName", newJob.Description)
// 	return nil, nil
// }
