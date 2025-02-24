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

package v1

import (
	"context"
	"fmt"

	rayiov1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// nolint:unused
// log is for logging in this package.
var rayservicelog = logf.Log.WithName("rayservice-resource")

// SetupRayServiceWebhookWithManager registers the webhook for RayService in the manager.
func SetupRayServiceWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&rayiov1.RayService{}).
		WithDefaulter(&RayServiceCustomDefaulter{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:path=/mutate-ray-io-ray-io-v1-rayservice,mutating=true,failurePolicy=fail,sideEffects=None,groups=ray.io.ray.io,resources=rayservices,verbs=create;update,versions=v1,name=mrayservice-v1.kb.io,admissionReviewVersions=v1

// RayServiceCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind RayService when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type RayServiceCustomDefaulter struct {
	// TODO(user): Add more fields as needed for defaulting
}

var _ webhook.CustomDefaulter = &RayServiceCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind RayService.
func (d *RayServiceCustomDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	rayservice, ok := obj.(*rayiov1.RayService)

	if !ok {
		return fmt.Errorf("expected an RayService object but got %T", obj)
	}
	rayservicelog.Info("Defaulting for RayService", "name", rayservice.GetName())

	// TODO(user): fill in your defaulting logic.

	return nil
}
