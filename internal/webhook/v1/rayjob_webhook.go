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
