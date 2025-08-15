package observe

import "sigs.k8s.io/controller-runtime/pkg/client"

type HasIdentity interface {
	GetNamespacedName() client.ObjectKey
	GetGroup() UnitGroup
}

type Identified struct {
	NamespacedName client.ObjectKey // or types.NamespacedName
	Group          UnitGroup
}

func (i Identified) GetNamespacedName() client.ObjectKey { return i.NamespacedName }
func (i Identified) GetGroup() UnitGroup                 { return i.Group }
