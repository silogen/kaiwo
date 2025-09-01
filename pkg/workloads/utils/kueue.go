package workloadutils

import (
	"context"
	"fmt"

	"github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"

	"github.com/silogen/kaiwo/pkg/api"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kueuev1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"
)

// GetKaiwoForWorkload resolves: Workload -> (controller owner) -> Kaiwo owner.
// Returns (cr, true, nil) on success, (nil, false, nil) if not found, or error on API errors.
func GetKaiwoForWorkload(ctx context.Context, c client.Client, wl *kueuev1beta1.Workload) (api.KaiwoWorkload, error) {
	// Pick the controller owner of the Workload (fallback: first owner)
	var ownerRef *metav1.OwnerReference
	for i := range wl.OwnerReferences {
		if wl.OwnerReferences[i].Controller != nil && *wl.OwnerReferences[i].Controller {
			ownerRef = &wl.OwnerReferences[i]
			break
		}
	}
	if ownerRef == nil {
		if len(wl.OwnerReferences) == 0 {
			return nil, fmt.Errorf("no owner reference found for workload %q", wl.GetName())
		}
		ownerRef = &wl.OwnerReferences[0]
	}

	// 2) Fetch the immediate owner (usually namespaced; use WL namespace)
	gv, err := schema.ParseGroupVersion(ownerRef.APIVersion)
	if err != nil {
		return nil, fmt.Errorf("parse owner apiversion: %w", err)
	}
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(schema.GroupVersionKind{Group: gv.Group, Version: gv.Version, Kind: ownerRef.Kind})

	if err := c.Get(ctx, types.NamespacedName{Namespace: wl.Namespace, Name: ownerRef.Name}, u); err != nil {
		return nil, fmt.Errorf("get owner: %w", err)
	}

	// Verify UID matches to avoid stale references
	if uid := string(u.GetUID()); uid != "" && ownerRef.UID != "" && uid != string(ownerRef.UID) {
		return nil, fmt.Errorf("owner uid mismatch: have %s, want %s", uid, ownerRef.UID)
	}

	// Find a Kaiwo owner on that object and fetch it
	wantGV := v1alpha1.GroupVersion.String()
	for _, or := range u.GetOwnerReferences() {
		if or.APIVersion == wantGV {
			var obj client.Object

			if or.Kind == "KaiwoJob" {
				obj = &v1alpha1.KaiwoJob{}
			} else if or.Kind == "KaiwoService" {
				obj = &v1alpha1.KaiwoService{}
			} else {
				return nil, fmt.Errorf("unknown Kaiwo workload type: %s", or.Kind)
			}
			if err := c.Get(ctx, types.NamespacedName{Namespace: wl.Namespace, Name: or.Name}, obj); err != nil {
				return nil, err
			}
			return obj.(api.KaiwoWorkload), nil
		}
	}

	return nil, fmt.Errorf("kueue workload owner does not point to a Kaiwo workload")
}
