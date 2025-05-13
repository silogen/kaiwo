// Copyright 2025 Advanced Micro Devices, Inc.  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package common

import (
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/silogen/kaiwo/apis/kaiwo/utils"

	kaiwo "github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"

	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

type KaiwoLabelContext struct {
	User    string
	Name    string
	Type    string
	RunId   string
	Managed string
}

// SetKaiwoSystemLabels sets the Kaiwo system labels on an ObjectMeta instance
func SetKaiwoSystemLabels(kaiwoLabelContext KaiwoLabelContext, objectMeta *v1.ObjectMeta) {
	if objectMeta.Labels == nil {
		objectMeta.Labels = make(map[string]string)
	}
	objectMeta.Labels[kaiwo.KaiwoUserLabel] = baseutils.MakeRFC1123Compliant(kaiwoLabelContext.User)
	objectMeta.Labels[kaiwo.KaiwoNameLabel] = kaiwoLabelContext.Name
	objectMeta.Labels[kaiwo.KaiwoTypeLabel] = kaiwoLabelContext.Type
	objectMeta.Labels[kaiwo.KaiwoRunIdLabel] = kaiwoLabelContext.RunId
	if kaiwoLabelContext.Managed != "" {
		objectMeta.Labels[kaiwo.KaiwoManagedLabel] = kaiwoLabelContext.Managed
	}
}

// CopyLabels copies labels from kaiwoLabels to objectMeta.Labels, skipping keys that already exist
func CopyLabels(kaiwoLabels map[string]string, objectMeta *v1.ObjectMeta) {
	for kaiwoLabelKey, kaiwoLabelValue := range kaiwoLabels {
		if kaiwoLabelKey == kaiwo.QueueLabel {
			continue
		}
		if _, exists := objectMeta.Labels[kaiwoLabelKey]; !exists {
			objectMeta.Labels[kaiwoLabelKey] = kaiwoLabelValue
		}
	}
}

func GetKaiwoLabelContext(k utils.KaiwoWorkload) KaiwoLabelContext {
	objectMeta := k.GetObjectMeta()
	return KaiwoLabelContext{
		User:    k.GetUser(),
		Name:    objectMeta.Name,
		Type:    k.GetType(),
		RunId:   string(objectMeta.UID),
		Managed: objectMeta.Labels[kaiwo.KaiwoManagedLabel],
	}
}
