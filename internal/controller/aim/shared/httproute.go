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
	"strings"

	"github.com/kserve/kserve/pkg/constants"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	gatewayapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	aimstate "github.com/silogen/kaiwo/internal/controller/aim/state"
)

// InferenceServiceRouteName returns the canonical HTTPRoute name for an InferenceService.
func InferenceServiceRouteName(serviceName string) string {
	return serviceName
	// return fmt.Sprintf("%s-route", serviceName)
}

// BuildInferenceServiceHTTPRoute creates an HTTPRoute that exposes the predictor service via the provided gateway parent.
func BuildInferenceServiceHTTPRoute(serviceState aimstate.ServiceState, ownerRef metav1.OwnerReference) *gatewayapiv1.HTTPRoute {
	annotations := make(map[string]string)
	if len(serviceState.Routing.Annotations) > 0 {
		for k, v := range serviceState.Routing.Annotations {
			annotations[k] = v
		}
	}

	route := &gatewayapiv1.HTTPRoute{
		TypeMeta: metav1.TypeMeta{
			APIVersion: gatewayapiv1.GroupVersion.String(),
			Kind:       "HTTPRoute",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      InferenceServiceRouteName(serviceState.Name),
			Namespace: serviceState.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       LabelValueServiceName,
				"app.kubernetes.io/component":  LabelValueServiceComponent,
				"app.kubernetes.io/managed-by": LabelValueManagedBy,
				LabelKeyTemplate:               serviceState.Template.Name,
				LabelKeyModelID:                SanitizeLabelValue(serviceState.ModelID),
				LabelKeyImageName:              SanitizeLabelValue(serviceState.Template.SpecCommon.ModelName),
				LabelKeyServiceName:            SanitizeLabelValue(serviceState.Name),
			},
			Annotations:     annotations,
			OwnerReferences: []metav1.OwnerReference{ownerRef},
		},
		Spec: gatewayapiv1.HTTPRouteSpec{},
	}

	if serviceState.Routing.GatewayRef != nil {
		parent := serviceState.Routing.GatewayRef.DeepCopy()
		if parent.Group == nil || *parent.Group == "" {
			parent.Group = ptr.To(gatewayapiv1.Group(gatewayapiv1.GroupVersion.Group))
		}
		if parent.Kind == nil || *parent.Kind == "" {
			parent.Kind = ptr.To(gatewayapiv1.Kind(constants.GatewayKind))
		}
		if parent.Namespace == nil || *parent.Namespace == "" {
			ns := gatewayapiv1.Namespace(serviceState.Namespace)
			parent.Namespace = &ns
		}
		route.Spec.ParentRefs = []gatewayapiv1.ParentReference{*parent}
	}

	pathPrefix := serviceState.Routing.PathPrefix
	if !strings.HasPrefix(pathPrefix, "/") {
		pathPrefix = "/" + pathPrefix
	}
	pathPrefix = strings.TrimRight(pathPrefix, "/")
	if pathPrefix == "" {
		pathPrefix = "/"
	}

	port := gatewayapiv1.PortNumber(constants.CommonDefaultHttpPort)

	// Use the generated InferenceService name (which may be shortened for DNS compliance)
	isvcName := GenerateInferenceServiceName(serviceState.Name, serviceState.Namespace)

	backend := gatewayapiv1.HTTPBackendRef{
		BackendRef: gatewayapiv1.BackendRef{
			BackendObjectReference: gatewayapiv1.BackendObjectReference{
				Kind:      ptr.To(gatewayapiv1.Kind(constants.ServiceKind)),
				Name:      gatewayapiv1.ObjectName(constants.PredictorServiceName(isvcName)),
				Namespace: (*gatewayapiv1.Namespace)(&serviceState.Namespace),
				Port:      &port,
			},
		},
	}

	rule := gatewayapiv1.HTTPRouteRule{
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
						Type:               gatewayapiv1.PrefixMatchHTTPPathModifier,
						ReplacePrefixMatch: ptr.To("/"),
					},
				},
			},
		},
		BackendRefs: []gatewayapiv1.HTTPBackendRef{backend},
	}

	// Set request timeout if configured (either from service or runtime config)
	if serviceState.Routing.RequestTimeout != nil {
		timeout := gatewayapiv1.Duration(*serviceState.Routing.RequestTimeout)
		rule.Timeouts = &gatewayapiv1.HTTPRouteTimeouts{
			Request: &timeout,
		}
	}

	route.Spec.Rules = []gatewayapiv1.HTTPRouteRule{rule}

	return route
}
