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

package common

import (
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"

	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

var (
	DefaultNodePoolLabel      = "kaiwo/nodepool"
	DefaultKaiwoWorkerLabel   = "kaiwo/worker"
	GPUModelLabel             = "kaiwo/gpu-model"
	DefaultTopologyBlockLabel = "kaiwo/topology-block"
	DefaultTopologyRackLabel  = "kaiwo/topology-rack"
	DefaultTopologyHostLabel  = "kubernetes.io/hostname"
	DefaultTopologyName       = "default-topology"
)

type KaiwoLabelContext struct {
	User    string
	Name    string
	Type    string
	RunId   string
	Managed string
}

// UpdateLabels propagates the Kaiwo workload's labels to the objectMeta, and sets the Kaiwo system labels
func UpdateLabels(workload KaiwoWorkload, objectMeta *v1.ObjectMeta) {
	if objectMeta.Labels == nil {
		objectMeta.Labels = map[string]string{}
	}

	sourceLabels := workload.GetKaiwoWorkloadObject().GetLabels()
	CopyLabels(sourceLabels, objectMeta)

	SetKaiwoSystemLabels(GetKaiwoLabelContext(workload), objectMeta)
}

// SetKaiwoSystemLabels sets the Kaiwo system labels on an ObjectMeta instance
func SetKaiwoSystemLabels(kaiwoLabelContext KaiwoLabelContext, objectMeta *v1.ObjectMeta) {
	if objectMeta.Labels == nil {
		objectMeta.Labels = make(map[string]string)
	}
	objectMeta.Labels[KaiwoUserLabel] = baseutils.MakeRFC1123Compliant(kaiwoLabelContext.User)
	objectMeta.Labels[KaiwoNameLabel] = kaiwoLabelContext.Name
	objectMeta.Labels[KaiwoTypeLabel] = kaiwoLabelContext.Type
	objectMeta.Labels[KaiwoRunIdLabel] = kaiwoLabelContext.RunId
	if kaiwoLabelContext.Managed != "" {
		objectMeta.Labels[KaiwoManagedLabel] = kaiwoLabelContext.Managed
	}
}

// CopyLabels copies labels from kaiwoLabels to objectMeta.Labels, skipping keys that already exist
func CopyLabels(kaiwoLabels map[string]string, objectMeta *v1.ObjectMeta) {
	for kaiwoLabelKey, kaiwoLabelValue := range kaiwoLabels {
		if kaiwoLabelKey == QueueLabel {
			continue
		}
		if _, exists := objectMeta.Labels[kaiwoLabelKey]; !exists {
			objectMeta.Labels[kaiwoLabelKey] = kaiwoLabelValue
		}
	}
}

func GetKaiwoLabelContext(k KaiwoWorkload) KaiwoLabelContext {
	obj := k.GetKaiwoWorkloadObject()
	spec := k.GetCommonSpec()

	workloadType := "job"
	if _, isService := obj.(*v1alpha1.KaiwoService); isService {
		workloadType = "service"
	}

	return KaiwoLabelContext{
		User:    spec.User,
		Name:    obj.GetName(),
		Type:    workloadType,
		RunId:   string(obj.GetUID()),
		Managed: obj.GetLabels()[KaiwoManagedLabel],
	}
}
