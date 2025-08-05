package nodeutils

import (
	"context"

	"github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"
	corev1 "k8s.io/api/core/v1"

	ctrl "sigs.k8s.io/controller-runtime"
)

type ReconcileTask[T any] interface {
	Name() string
	Run(ctx context.Context, obj T) (*ctrl.Result, error)
}

type KaiwoNodeWrapper struct {
	Node      *corev1.Node
	KaiwoNode *v1alpha1.KaiwoNode
}
