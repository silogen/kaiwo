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
	"os"
	"sync"
)

const (
	// operatorNamespaceEnvVar is the environment variable the operator uses to determine its namespace.
	operatorNamespaceEnvVar = "AIM_OPERATOR_NAMESPACE"

	// DefaultRuntimeConfigName is the name of the default AIM runtime config
	DefaultRuntimeConfigName = "default"

	// AimLabelDomain is the base domain used for AIM-specific labels.
	AimLabelDomain = "aim.silogen.ai"

	// AIM label keys.
	LabelKeyTemplate        = AimLabelDomain + "/template"
	LabelKeyModelID         = AimLabelDomain + "/model-id"
	LabelKeyDerivedTemplate = AimLabelDomain + "/derived-template"

	// AIM label values.
	LabelValueRuntimeName        = "aim-runtime"
	LabelValueRuntimeComponent   = "serving-runtime"
	LabelValueManagedBy          = "aim-controller"
	LabelValueDiscoveryName      = "aim-discovery"
	LabelValueDiscoveryComponent = "discovery-job"
	LabelValueServiceName        = "aim-service"
	LabelValueServiceComponent   = "inference-service"
	LabelValueDerivedTemplate    = "true"
)

var (
	operatorNamespaceOnce sync.Once
	operatorNamespace     string
)

// GetOperatorNamespace returns the namespace where the AIM operator runs.
// It reads the AIM_OPERATOR_NAMESPACE environment variable; if unset, it defaults to "kaiwo-system".
func GetOperatorNamespace() string {
	operatorNamespaceOnce.Do(func() {
		if ns := os.Getenv(operatorNamespaceEnvVar); ns != "" {
			operatorNamespace = ns
			return
		}
		operatorNamespace = "kaiwo-system"
	})
	return operatorNamespace
}
