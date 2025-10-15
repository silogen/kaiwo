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
	"reflect"
	"regexp"
	"strings"

	"k8s.io/client-go/util/jsonpath"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

const (
	// MaxRoutePathLength is the maximum allowed length for a route path.
	// This prevents excessively long paths that could cause issues with gateways or proxies.
	MaxRoutePathLength = 200
)

var (
	routeTemplatePattern      = regexp.MustCompile(`\{([^{}]+)\}`)
	labelAccessPattern        = regexp.MustCompile(`^\.metadata\.labels\[['"]([^'"]+)['"]\]$`)
	annotationAccessPattern   = regexp.MustCompile(`^\.metadata\.annotations\[['"]([^'"]+)['"]\]$`)
	singleQuoteBracketPattern = regexp.MustCompile(`\['([^']*)'\]`)
)

// ResolveServiceRoutePath renders the HTTP route prefix using service and runtime config context.
func ResolveServiceRoutePath(service *aimv1alpha1.AIMService, runtimeConfig aimv1alpha1.AIMRuntimeConfigSpec) (string, error) {
	template := ""
	if service.Spec.Routing != nil && service.Spec.Routing.RouteTemplate != "" {
		template = service.Spec.Routing.RouteTemplate
	} else if runtimeConfig.Routing != nil && runtimeConfig.Routing.RouteTemplate != "" {
		template = runtimeConfig.Routing.RouteTemplate
	}

	if template == "" {
		return DefaultRoutePath(service), nil
	}

	rendered, err := renderRouteTemplate(template, service)
	if err != nil {
		return "", err
	}

	return normalizeRoutePath(rendered)
}

// DefaultRoutePath returns the default HTTP route prefix.
func DefaultRoutePath(service *aimv1alpha1.AIMService) string {
	path, err := normalizeRoutePath(fmt.Sprintf("/%s/%s", service.Namespace, string(service.UID)))
	if err != nil {
		return "/"
	}
	return path
}

func renderRouteTemplate(template string, service *aimv1alpha1.AIMService) (string, error) {
	matches := routeTemplatePattern.FindAllStringSubmatchIndex(template, -1)
	if len(matches) == 0 {
		return template, nil
	}

	var builder strings.Builder
	last := 0
	for _, m := range matches {
		start, end := m[0], m[1]
		exprStart, exprEnd := m[2], m[3]

		builder.WriteString(template[last:start])

		expr := strings.TrimSpace(template[exprStart:exprEnd])
		value, err := evaluateJSONPath(expr, service)
		if err != nil {
			return "", fmt.Errorf("failed to evaluate route template %q: %w", expr, err)
		}
		builder.WriteString(applyTemplateValueModifiers(expr, value))

		last = end
	}
	builder.WriteString(template[last:])

	return builder.String(), nil
}

func evaluateJSONPath(expr string, obj interface{}) (string, error) {
	if expr == "" {
		return "", fmt.Errorf("jsonpath expression is empty")
	}

	if service, ok := obj.(*aimv1alpha1.AIMService); ok {
		if match := labelAccessPattern.FindStringSubmatch(expr); len(match) == 2 {
			value, ok := service.Labels[match[1]]
			if !ok {
				return "", fmt.Errorf("jsonpath evaluation error: label %q not found", match[1])
			}
			return value, nil
		}
		if match := annotationAccessPattern.FindStringSubmatch(expr); len(match) == 2 {
			value, ok := service.Annotations[match[1]]
			if !ok {
				return "", fmt.Errorf("jsonpath evaluation error: annotation %q not found", match[1])
			}
			return value, nil
		}
	}

	parsed := fmt.Sprintf("{%s}", normalizeBracketKeys(expr))
	jp := jsonpath.New("route")
	jp.AllowMissingKeys(false)
	if err := jp.Parse(parsed); err != nil {
		return "", fmt.Errorf("invalid jsonpath expression: %w", err)
	}

	results, err := jp.FindResults(obj)
	if err != nil {
		return "", fmt.Errorf("jsonpath evaluation error: %w", err)
	}

	if len(results) == 0 || len(results[0]) == 0 {
		return "", fmt.Errorf("jsonpath returned no results")
	}
	if len(results) > 1 {
		return "", fmt.Errorf("jsonpath returned multiple results")
	}
	if len(results[0]) > 1 {
		return "", fmt.Errorf("jsonpath returned multiple results")
	}

	val := results[0][0]
	if !val.IsValid() {
		return "", fmt.Errorf("jsonpath returned invalid value")
	}
	if val.Kind() == reflect.Ptr && val.IsNil() {
		return "", fmt.Errorf("jsonpath returned nil value")
	}

	value := val.Interface()
	switch typed := value.(type) {
	case string:
		return typed, nil
	case fmt.Stringer:
		return typed.String(), nil
	default:
		return fmt.Sprint(value), nil
	}
}

func normalizeRoutePath(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("route template produced an empty path")
	}
	if !strings.HasPrefix(raw, "/") {
		raw = "/" + raw
	}

	normalized := strings.TrimRight(raw, "/")
	if normalized == "" {
		normalized = "/"
	}
	if len(normalized) > MaxRoutePathLength {
		return "", fmt.Errorf("route path %q exceeds %d characters", normalized, MaxRoutePathLength)
	}

	segments := strings.Split(normalized, "/")
	encoded := make([]string, 0, len(segments))
	for i, segment := range segments {
		if i == 0 {
			encoded = append(encoded, "")
			continue
		}
		if segment == "" {
			continue
		}
		encodedSegment := encodeRouteSegment(segment)
		if encodedSegment == "" {
			continue
		}
		encoded = append(encoded, encodedSegment)
	}

	path := "/"
	if len(encoded) > 1 {
		path = "/" + strings.Join(encoded[1:], "/")
	}

	path = strings.TrimRight(path, "/")
	if path == "" {
		path = "/"
	}

	if len(path) > MaxRoutePathLength {
		return "", fmt.Errorf("route path %q exceeds %d characters", path, MaxRoutePathLength)
	}

	return path, nil
}

func encodeRouteSegment(segment string) string {
	segment = strings.TrimSpace(segment)
	if segment == "" {
		return ""
	}
	return baseutils.MakeRFC1123Compliant(segment)
}

func applyTemplateValueModifiers(expr, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return value
	}
	if annotationAccessPattern.MatchString(expr) {
		return value
	}
	return baseutils.MakeRFC1123Compliant(value)
}

func normalizeBracketKeys(expr string) string {
	return singleQuoteBracketPattern.ReplaceAllStringFunc(expr, func(match string) string {
		groups := singleQuoteBracketPattern.FindStringSubmatch(match)
		if len(groups) != 2 {
			return match
		}
		key := groups[1]
		key = strings.ReplaceAll(key, `\`, `\\`)
		key = strings.ReplaceAll(key, `"`, `\\"`)
		return fmt.Sprintf(`["%s"]`, key)
	})
}
