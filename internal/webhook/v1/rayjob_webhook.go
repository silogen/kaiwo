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
var rayjoblog = logf.Log.WithName("rayjob-resource")

// SetupRayJobWebhookWithManager registers the webhook for RayJob in the manager.
func SetupRayJobWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&rayiov1.RayJob{}).
		WithDefaulter(&RayJobCustomDefaulter{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:path=/mutate-ray-io-ray-io-v1-rayjob,mutating=true,failurePolicy=fail,sideEffects=None,groups=ray.io.ray.io,resources=rayjobs,verbs=create;update,versions=v1,name=mrayjob-v1.kb.io,admissionReviewVersions=v1

// RayJobCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind RayJob when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type RayJobCustomDefaulter struct {
	// TODO(user): Add more fields as needed for defaulting
}

var _ webhook.CustomDefaulter = &RayJobCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind RayJob.
func (d *RayJobCustomDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	rayjob, ok := obj.(*rayiov1.RayJob)

	if !ok {
		return fmt.Errorf("expected an RayJob object but got %T", obj)
	}
	rayjoblog.Info("Defaulting for RayJob", "name", rayjob.GetName())

	// TODO(user): fill in your defaulting logic.

	return nil
}
