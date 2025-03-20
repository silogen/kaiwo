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

package baseutils

import (
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/silogen/kaiwo/pkg/api/v1alpha1"
)

const (
	KaiwoLabelBase    = "kaiwo.silogen.ai"
	KaiwoUserLabel    = KaiwoLabelBase + "/user"
	KaiwoNameLabel    = KaiwoLabelBase + "/name"
	KaiwoTypeLabel    = KaiwoLabelBase + "/type"
	KaiwoRunIdLabel   = KaiwoLabelBase + "/run-id"
	KaiwoManagedLabel = KaiwoLabelBase + "/managed"
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
	objectMeta.Labels[KaiwoUserLabel] = MakeRFC1123Compliant(kaiwoLabelContext.User)
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
		if kaiwoLabelKey == v1alpha1.QueueLabel {
			continue
		}
		if _, exists := objectMeta.Labels[kaiwoLabelKey]; !exists {
			objectMeta.Labels[kaiwoLabelKey] = kaiwoLabelValue
		}
	}
}

type KaiwoObject interface {
	GetName() string
	GetUID() types.UID
	GetLabels() map[string]string
	GetUser() *string
	ResourceType() string
}

func GetKaiwoLabelContext(k KaiwoObject) KaiwoLabelContext {
	return KaiwoLabelContext{
		User:    ValueOrDefault(k.GetUser()),
		Name:    k.GetName(),
		Type:    k.ResourceType(),
		RunId:   string(k.GetUID()),
		Managed: k.GetLabels()[KaiwoManagedLabel],
	}
}
