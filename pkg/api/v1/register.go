package v1

import (
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/apimachinery/pkg/runtime/schema"
)
var GroupVersion = schema.GroupVersion{Group: "example.com", Version: "v1"}

var SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)

var AddToScheme = SchemeBuilder.AddToScheme

func addKnownTypes(scheme *runtime.Scheme) error {
    scheme.AddKnownTypes(GroupVersion,
        &KaiwoJob{},
        &KaiwoJobList{},
        &KaiwoService{},
        &KaiwoServiceList{},
    )
    metav1.AddToGroupVersion(scheme, GroupVersion)
    return nil
}
