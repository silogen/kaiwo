package nodeutils

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type UpdateKaiwoNodeTask struct {
	Client client.Client
}

func (t *UpdateKaiwoNodeTask) Name() string {
	return "EnsureKaiwoNode"
}

func (t *UpdateKaiwoNodeTask) Run(ctx context.Context, wrapper *KaiwoNodeWrapper) (*ctrl.Result, error) {
	return nil, nil
}
