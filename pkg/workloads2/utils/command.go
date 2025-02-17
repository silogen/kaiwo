package workloadutils

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type CommandBase[T any] struct {
	// Object is the object which would be created by running this command
	Object client.Object

	// Owner is object that owns the object that would be created by this command.
	Owner client.Object

	// State holds a shared state struct that each command can use
	State *T

	Context context.Context
	Scheme  *runtime.Scheme
}

func (cb *CommandBase[T]) SetObject(object client.Object) {
	cb.Object = object
}

func (cb *CommandBase[T]) Create(ctx context.Context, k8sClient client.Client) error {
	log.FromContext(ctx).Info("Creating object")
	return k8sClient.Create(ctx, cb.Object)
}

func (cb *CommandBase[T]) Update(ctx context.Context, k8sClient client.Client) error {
	logger := log.FromContext(ctx)
	logger.Info("Object exists, skipping update")
	return nil
}

func (cb *CommandBase[T]) ResourceExists(ctx context.Context, k8sClient client.Client) (bool, error) {
	existingObject := cb.Object.DeepCopyObject().(client.Object)
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cb.Object), existingObject); err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (cb *CommandBase[T]) GetOwner() client.Object {
	return cb.Owner
}

func (cb *CommandBase[T]) GetCurrentReconcileResult() *ctrl.Result {
	return nil
}

type Command interface {
	// Build creates the Kubernetes manifest that this command would create. If nil is returned, assuming nothing would be created
	Build() (client.Object, error)

	SetObject(object client.Object)

	// ResourceExists checks if the resource exists. Must be run after Build().
	ResourceExists(ctx context.Context, k8sClient client.Client) (bool, error)

	// Create creates the object inside the Kubernetes cluster. Must be run after Build().
	Create(ctx context.Context, k8sClient client.Client) error

	// Update updates the object inside the Kubernetes cluster. Must be run after Build().
	Update(ctx context.Context, k8sClient client.Client) error

	// GetOwner returns the object that owns the object that would be created by this command.
	GetOwner() client.Object

	// GetCurrentReconcileResult returns an optional reconciliation result, which can be used to force a reconciliation
	// request in the middle of invoking a list of commands, in which case the rest of the commands do not get invoked.
	GetCurrentReconcileResult() *ctrl.Result
}
