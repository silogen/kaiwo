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

package unit

import (
	"testing"

	baseutils "github.com/silogen/kaiwo/pkg/utils"

	"github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"

	"github.com/silogen/kaiwo/pkg/workloads/common"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestBatchJob(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Workload defaults suite")
}

var _ = Describe("Workload defaults", func() {
	podTemplateSpec := corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"foo": "bar",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "first",
					Image: "busybox",
				},
			},
		},
	}
	originalPodTemplateSpec := podTemplateSpec.DeepCopy()

	var kaiwoCommonMetaSpec v1alpha1.CommonMetaSpec

	replicas := 1
	gpusPerReplica := 1

	labelContext := common.KaiwoLabelContext{
		User:  "test-user",
		Type:  "test-type",
		RunId: "test-run-id",
		Name:  "test-name",
	}
	name := "test-name"

	BeforeEach(func() {
		podTemplateSpec = *originalPodTemplateSpec.DeepCopy()
		kaiwoCommonMetaSpec = v1alpha1.CommonMetaSpec{
			PodTemplateSpecLabels: map[string]string{
				"kaiwo-foo": "kaiwo-bar",
			},
		}
	})

	JustBeforeEach(func() {
		resourceConfig := &common.GpuSchedulingResult{
			Replicas:           baseutils.Pointer(replicas),
			GpuCountPerReplica: gpusPerReplica,
			GpuResourceName:    common.AmdGpuResourceName,
		}
		workload := &v1alpha1.KaiwoJob{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Spec: v1alpha1.KaiwoJobSpec{
				CommonMetaSpec: kaiwoCommonMetaSpec,
			},
		}
		common.UpdatePodSpec(common.KaiwoConfigContext{}, workload, resourceConfig, &podTemplateSpec, false)
	})

	When("a workload pod spec is being updated", func() {
		It("sets the kaiwo system flags", func() {
			Expect(podTemplateSpec.Labels[common.KaiwoTypeLabel]).To(Equal(labelContext.Type))
			Expect(podTemplateSpec.Labels[common.KaiwoUserLabel]).To(Equal(labelContext.User))
			Expect(podTemplateSpec.Labels[common.KaiwoNameLabel]).To(Equal(labelContext.Name))
			Expect(podTemplateSpec.Labels[common.KaiwoRunIdLabel]).To(Equal(labelContext.RunId))
		})

		It("keeps any existing flags", func() {
			Expect(podTemplateSpec.Labels["foo"]).To(Equal("bar"))
		})

		Context("and extra labels are present", func() {
			It("adds the extra flags", func() {
				Expect(podTemplateSpec.Labels["kaiwo-foo"]).To(Equal("kaiwo-bar"))
			})
		})
	})

	When("images are being checked", func() {
		Context("and an image has been given", func() {
			BeforeEach(func() {
				kaiwoCommonMetaSpec.Image = "ubuntu"
			})
			Context("and there is no image set in the container", func() {
				BeforeEach(func() {
					podTemplateSpec.Spec.Containers[0].Image = ""
				})
				It("sets the image if none is set in the spec", func() {
					Expect(podTemplateSpec.Spec.Containers[0].Image).To(Equal(kaiwoCommonMetaSpec.Image))
				})
			})

			It("does not set the image if one is set in the spec", func() {
				Expect(podTemplateSpec.Spec.Containers[0].Image).To(Equal(originalPodTemplateSpec.Spec.Containers[0].Image))
			})
		})
		Context("and no image is set in the spec", func() {
			BeforeEach(func() {
				podTemplateSpec.Spec.Containers[0].Image = ""
			})
			//It("uses the default ray image if an image is not set", func() {
			//	Expect(podTemplateSpec.Spec.Containers[0].Image).To(Equal(baseutils.DefaultRayImage))
			//})
		})
	})
})
