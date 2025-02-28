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

package workloadjob

import (
	"context"
	"fmt"
	"testing"

	batchv1 "k8s.io/api/batch/v1"

	controllerutils "github.com/silogen/kaiwo/internal/controller/utils"

	"github.com/silogen/kaiwo/pkg/k8s"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/silogen/kaiwo/pkg/api/v1alpha1"
)

func TestBatchJob(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Batch job suite")
}

var _ = Describe("Batch job suite", func() {
	scheme, err := k8s.GetScheme()
	if err != nil {
		panic(err)
	}

	kaiwoJobSpec := v1alpha1.KaiwoJobSpec{
		Resources: &v1.ResourceRequirements{
			Requests: v1.ResourceList{v1.ResourceMemory: resource.MustParse("15Gi")},
			Limits:   v1.ResourceList{v1.ResourceCPU: resource.MustParse("125m")},
		},
	}

	dryReconcile := func(job *batchv1.Job) KaiwoJobReconciler {
		spec := kaiwoJobSpec.DeepCopy()
		spec.Job = job
		kaiwoJob := &v1alpha1.KaiwoJob{
			Spec: *spec,
		}
		reconciler := NewKaiwoJobReconciler(kaiwoJob)
		_, _, err = reconciler.Reconcile(context.Background(), nil, &scheme, true)
		Expect(err).NotTo(HaveOccurred())
		return reconciler
	}

	When("the resources are specified in the kaiwo job spec", func() {
		Context("and the job does not specify resources", func() {
			reconciler := dryReconcile(nil)

			It("sets all of the given resources", func() {
				Expect(reconciler.BatchJob.Desired.Spec.Template.Spec.Containers[0].Resources.Requests[v1.ResourceMemory]).To(Equal((*kaiwoJobSpec.Resources).Requests[v1.ResourceMemory]))
				Expect(reconciler.BatchJob.Desired.Spec.Template.Spec.Containers[0].Resources.Limits[v1.ResourceCPU]).To(Equal((*kaiwoJobSpec.Resources).Limits[v1.ResourceCPU]))
			})

			It("uses the default values for those fields that are not set", func() {
				Expect(reconciler.BatchJob.Desired.Spec.Template.Spec.Containers[0].Resources.Requests[v1.ResourceCPU]).To(Equal(controllerutils.DefaultCPU))
				Expect(reconciler.BatchJob.Desired.Spec.Template.Spec.Containers[0].Resources.Limits[v1.ResourceMemory]).To(Equal(controllerutils.DefaultMemory))
			})
		})

		Context("and the job includes resources that do not overlap", func() {
			memoryLimit := resource.MustParse("14Gi")
			cpuRequest := resource.MustParse("250m")

			reconciler := dryReconcile(&batchv1.Job{
				Spec: batchv1.JobSpec{
					Template: v1.PodTemplateSpec{
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Resources: v1.ResourceRequirements{
										Limits: v1.ResourceList{
											v1.ResourceMemory: memoryLimit,
										},
										Requests: v1.ResourceList{
											v1.ResourceCPU: cpuRequest,
										},
									},
								},
							},
						},
					},
				},
			})

			It("sets all of the given resources", func() {
				Expect(reconciler.BatchJob.Desired.Spec.Template.Spec.Containers[0].Resources.Requests[v1.ResourceMemory]).To(Equal((*kaiwoJobSpec.Resources).Requests[v1.ResourceMemory]))
				Expect(reconciler.BatchJob.Desired.Spec.Template.Spec.Containers[0].Resources.Limits[v1.ResourceCPU]).To(Equal((*kaiwoJobSpec.Resources).Limits[v1.ResourceCPU]))
			})
			It("does not use defaults for values that are given in the job spec", func() {
				Expect(reconciler.BatchJob.Desired.Spec.Template.Spec.Containers[0].Resources.Requests[v1.ResourceCPU]).To(Equal(cpuRequest))
				Expect(reconciler.BatchJob.Desired.Spec.Template.Spec.Containers[0].Resources.Limits[v1.ResourceMemory]).To(Equal(memoryLimit))
			})
		})

		Context("and the job includes resources that overlap", func() {
			memoryRequest := resource.MustParse("14Gi")
			cpuLimit := resource.MustParse("500m")

			reconciler := dryReconcile(&batchv1.Job{
				Spec: batchv1.JobSpec{
					Template: v1.PodTemplateSpec{
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Resources: v1.ResourceRequirements{
										Requests: v1.ResourceList{
											v1.ResourceMemory: memoryRequest,
										},
										Limits: v1.ResourceList{
											v1.ResourceCPU: cpuLimit,
										},
									},
								},
							},
						},
					},
				},
			})

			It("does not override the resources that were set in the job spec", func() {
				Expect(reconciler.BatchJob.Desired.Spec.Template.Spec.Containers[0].Resources.Requests[v1.ResourceMemory]).To(Equal(memoryRequest))
				Expect(reconciler.BatchJob.Desired.Spec.Template.Spec.Containers[0].Resources.Limits[v1.ResourceCPU]).To(Equal(cpuLimit))
			})

			It("values that were not set are still unset", func() {
				fmt.Println(reconciler.BatchJob.Desired.Spec.Template.Spec.Containers[0].Resources)
				requestedCpu := reconciler.BatchJob.Desired.Spec.Template.Spec.Containers[0].Resources.Requests[v1.ResourceCPU]
				Expect(requestedCpu.Value()).To(Equal(int64(0)))

				limitedMemory := reconciler.BatchJob.Desired.Spec.Template.Spec.Containers[0].Resources.Limits[v1.ResourceMemory]
				Expect(limitedMemory.Value()).To(Equal(int64(0)))
			})
		})
	})
})
