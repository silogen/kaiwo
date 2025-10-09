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

package shared

import (
	"fmt"
	"strings"

	"github.com/kserve/kserve/pkg/constants"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	gatewayapiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// InferenceServiceRouteSpec describes the HTTPRoute that fronts an InferenceService.
type InferenceServiceRouteSpec struct {
	Name             string
	Namespace        string
	OwnerRef         metav1.OwnerReference
	ParentRef        *gatewayapiv1.ParentReference
	BackendNamespace string
	BackendService   string
	PathPrefix       string
	TemplateName     string
	ModelID          string
}

// InferenceServiceRouteName returns the canonical HTTPRoute name for an InferenceService.
func InferenceServiceRouteName(serviceName string) string {
	return fmt.Sprintf("%s-route", serviceName)
}

// BuildInferenceServiceHTTPRoute creates an HTTPRoute that exposes the predictor service via the provided gateway parent.
func BuildInferenceServiceHTTPRoute(spec InferenceServiceRouteSpec) *gatewayapiv1.HTTPRoute {
	route := &gatewayapiv1.HTTPRoute{
		TypeMeta: metav1.TypeMeta{
			APIVersion: gatewayapiv1.GroupVersion.String(),
			Kind:       "HTTPRoute",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      spec.Name,
			Namespace: spec.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       LabelValueServiceName,
				"app.kubernetes.io/component":  LabelValueServiceComponent,
				"app.kubernetes.io/managed-by": LabelValueManagedBy,
				LabelKeyTemplate:               spec.TemplateName,
				LabelKeyModelID:                sanitizeLabelValue(spec.ModelID),
			},
			OwnerReferences: []metav1.OwnerReference{spec.OwnerRef},
		},
		Spec: gatewayapiv1.HTTPRouteSpec{},
	}

	if spec.ParentRef != nil {
		parent := spec.ParentRef.DeepCopy()
		if parent.Group == nil || *parent.Group == "" {
			parent.Group = ptr.To(gatewayapiv1.Group(gatewayapiv1.GroupVersion.Group))
		}
		if parent.Kind == nil || *parent.Kind == "" {
			parent.Kind = ptr.To(gatewayapiv1.Kind(constants.GatewayKind))
		}
		if parent.Namespace == nil || *parent.Namespace == "" {
			ns := gatewayapiv1.Namespace(spec.Namespace)
			parent.Namespace = &ns
		}
		route.Spec.ParentRefs = []gatewayapiv1.ParentReference{*parent}
	}

	pathPrefix := spec.PathPrefix
	if !strings.HasPrefix(pathPrefix, "/") {
		pathPrefix = "/" + pathPrefix
	}
	pathPrefix = strings.TrimRight(pathPrefix, "/")

	port := gatewayapiv1.PortNumber(constants.CommonDefaultHttpPort)
	backend := gatewayapiv1.HTTPBackendRef{
		BackendRef: gatewayapiv1.BackendRef{
			BackendObjectReference: gatewayapiv1.BackendObjectReference{
				Kind:      ptr.To(gatewayapiv1.Kind(constants.ServiceKind)),
				Name:      gatewayapiv1.ObjectName(spec.BackendService),
				Namespace: (*gatewayapiv1.Namespace)(&spec.BackendNamespace),
				Port:      &port,
			},
		},
	}

	route.Spec.Rules = []gatewayapiv1.HTTPRouteRule{
		{
			Matches: []gatewayapiv1.HTTPRouteMatch{
				{
					Path: &gatewayapiv1.HTTPPathMatch{
						Type:  ptr.To(gatewayapiv1.PathMatchPathPrefix),
						Value: ptr.To(pathPrefix),
					},
				},
			},
			Filters: []gatewayapiv1.HTTPRouteFilter{
				{
					Type: gatewayapiv1.HTTPRouteFilterURLRewrite,
					URLRewrite: &gatewayapiv1.HTTPURLRewriteFilter{
						Path: &gatewayapiv1.HTTPPathModifier{
							ReplacePrefixMatch: ptr.To("/"),
						},
					},
				},
			},
			BackendRefs: []gatewayapiv1.HTTPBackendRef{backend},
		},
	}

	return route
}
