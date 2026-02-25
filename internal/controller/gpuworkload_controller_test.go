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

package controller

import (
	"context"
	"time"

	kaiwo "github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("GpuWorkload Controller", func() {
	const (
		timeout  = 10 * time.Second
		interval = 250 * time.Millisecond
	)

	Context("Reconcile lifecycle", func() {
		const gwName = "test-gpuworkload"
		ctx := context.Background()

		namespacedName := types.NamespacedName{
			Name:      gwName,
			Namespace: "default",
		}

		BeforeEach(func() {
			gw := &kaiwo.GpuWorkload{}
			err := k8sClient.Get(ctx, namespacedName, gw)
			if err != nil && errors.IsNotFound(err) {
				resource := &kaiwo.GpuWorkload{
					ObjectMeta: metav1.ObjectMeta{
						Name:      gwName,
						Namespace: "default",
					},
					Spec: kaiwo.GpuWorkloadSpec{
						WorkloadRef: kaiwo.WorkloadReference{
							APIVersion: "batch/v1",
							Kind:       "Job",
							Name:       "test-job",
							UID:        "test-uid-12345678",
						},
						GpuResources: map[string]int{
							"amd.com/gpu": 2,
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &kaiwo.GpuWorkload{}
			err := k8sClient.Get(ctx, namespacedName, resource)
			if err == nil {
				By("Cleanup the GpuWorkload resource")
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should reconcile without error when owner does not exist", func() {
			reconciler := &GpuWorkloadReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())

			// Owner doesn't exist, so it should transition to Deleted
			Eventually(func() kaiwo.GpuWorkloadPhase {
				gw := &kaiwo.GpuWorkload{}
				if err := k8sClient.Get(ctx, namespacedName, gw); err != nil {
					return ""
				}
				return gw.Status.Phase
			}, timeout, interval).Should(Equal(kaiwo.GpuWorkloadPhaseDeleted))
			_ = result
		})

		It("should handle terminal phase with TTL", func() {
			gw := &kaiwo.GpuWorkload{}
			Expect(k8sClient.Get(ctx, namespacedName, gw)).To(Succeed())

			now := metav1.Now()
			gw.Status.Phase = kaiwo.GpuWorkloadPhaseDeleted
			gw.Status.FinishedAt = &now
			Expect(k8sClient.Status().Update(ctx, gw)).To(Succeed())

			reconciler := &GpuWorkloadReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
			Expect(err).NotTo(HaveOccurred())
			// TTL default 24h, should requeue
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))
		})
	})

	Context("extractGpuResources", func() {
		It("should extract AMD GPU resources from pod containers", func() {
			pod := &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									"amd.com/gpu": resource.MustParse("2"),
									"cpu":         resource.MustParse("4"),
								},
							},
						},
						{
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									"amd.com/gpu": resource.MustParse("1"),
								},
							},
						},
					},
				},
			}
			result := extractGpuResources(pod)
			Expect(result).To(HaveKeyWithValue("amd.com/gpu", 3))
			Expect(result).NotTo(HaveKey("cpu"))
		})

		It("should extract multiple GPU resource types", func() {
			pod := &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									"amd.com/gpu":   resource.MustParse("2"),
									"amd.com/gpu-0": resource.MustParse("4"),
								},
							},
						},
					},
				},
			}
			result := extractGpuResources(pod)
			Expect(result).To(HaveKeyWithValue("amd.com/gpu", 2))
			Expect(result).To(HaveKeyWithValue("amd.com/gpu-0", 4))
		})

		It("should return empty map for pods without GPU resources", func() {
			pod := &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									"cpu":    resource.MustParse("4"),
									"memory": resource.MustParse("8Gi"),
								},
							},
						},
					},
				},
			}
			result := extractGpuResources(pod)
			Expect(result).To(BeEmpty())
		})
	})

	Context("parseAnnotationsIntoSpec", func() {
		It("should parse all annotation fields", func() {
			annotations := map[string]string{
				AnnotationEnabled:     "true",
				AnnotationThreshold:   "10.5",
				AnnotationIfIdleAfter: "5m",
				AnnotationPolicy:      "Always",
				AnnotationAggregation: "Avg",
				AnnotationTTL:         "48h",
			}
			spec := &kaiwo.GpuWorkloadSpec{
				GpuResources: map[string]int{"amd.com/gpu": 1},
			}
			parseAnnotationsIntoSpec(annotations, spec)

			Expect(spec.UtilizationThreshold).NotTo(BeNil())
			Expect(*spec.UtilizationThreshold).To(BeNumerically("~", 10.5))
			Expect(spec.IfIdleAfter).NotTo(BeNil())
			Expect(spec.IfIdleAfter.Duration).To(Equal(5 * time.Minute))
			Expect(spec.PreemptionPolicy).NotTo(BeNil())
			Expect(*spec.PreemptionPolicy).To(Equal(kaiwo.PreemptionPolicyAlways))
			Expect(spec.AggregationPolicy).NotTo(BeNil())
			Expect(*spec.AggregationPolicy).To(Equal(kaiwo.AggregationPolicyAvg))
			Expect(spec.TTLAfterFinished).NotTo(BeNil())
			Expect(spec.TTLAfterFinished.Duration).To(Equal(48 * time.Hour))
		})

		It("should ignore invalid annotations", func() {
			annotations := map[string]string{
				AnnotationThreshold:   "notanumber",
				AnnotationIfIdleAfter: "invalid",
				AnnotationPolicy:      "InvalidPolicy",
				AnnotationAggregation: "InvalidAgg",
			}
			spec := &kaiwo.GpuWorkloadSpec{
				GpuResources: map[string]int{"amd.com/gpu": 1},
			}
			parseAnnotationsIntoSpec(annotations, spec)

			Expect(spec.UtilizationThreshold).To(BeNil())
			Expect(spec.IfIdleAfter).To(BeNil())
			Expect(spec.PreemptionPolicy).To(BeNil())
			Expect(spec.AggregationPolicy).To(BeNil())
		})
	})

	Context("isGpuPreemptionAnnotated", func() {
		It("should return true with explicit enabled annotation", func() {
			annotations := map[string]string{
				AnnotationEnabled: "true",
			}
			Expect(isGpuPreemptionAnnotated(annotations)).To(BeTrue())
		})

		It("should return true with only if-idle-after (implicit enable)", func() {
			annotations := map[string]string{
				AnnotationIfIdleAfter: "5m",
			}
			Expect(isGpuPreemptionAnnotated(annotations)).To(BeTrue())
		})

		It("should return true with only policy annotation (implicit enable)", func() {
			annotations := map[string]string{
				AnnotationPolicy: "Always",
			}
			Expect(isGpuPreemptionAnnotated(annotations)).To(BeTrue())
		})

		It("should return false with nil annotations", func() {
			Expect(isGpuPreemptionAnnotated(nil)).To(BeFalse())
		})

		It("should return false with no preemption annotations", func() {
			annotations := map[string]string{
				"some-other-annotation": "value",
			}
			Expect(isGpuPreemptionAnnotated(annotations)).To(BeFalse())
		})
	})

	Context("isPendingDueToGPU", func() {
		It("should return true when pod is unschedulable due to GPU", func() {
			pod := &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodPending,
					Conditions: []corev1.PodCondition{
						{
							Type:    corev1.PodScheduled,
							Status:  corev1.ConditionFalse,
							Reason:  "Unschedulable",
							Message: "0/4 nodes are available: 4 Insufficient amd.com/gpu. preemption: 0/4 nodes are available",
						},
					},
				},
			}
			gpuResources := map[string]int{"amd.com/gpu": 2}
			Expect(isPendingDueToGPU(pod, gpuResources)).To(BeTrue())
		})

		It("should return false when pod is pending for other reasons", func() {
			pod := &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodPending,
					Conditions: []corev1.PodCondition{
						{
							Type:    corev1.PodScheduled,
							Status:  corev1.ConditionFalse,
							Reason:  "Unschedulable",
							Message: "0/4 nodes are available: 4 Insufficient cpu.",
						},
					},
				},
			}
			gpuResources := map[string]int{"amd.com/gpu": 2}
			Expect(isPendingDueToGPU(pod, gpuResources)).To(BeFalse())
		})

		It("should return false when pod is scheduled", func() {
			pod := &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodPending,
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.PodScheduled,
							Status: corev1.ConditionTrue,
						},
					},
				},
			}
			gpuResources := map[string]int{"amd.com/gpu": 2}
			Expect(isPendingDueToGPU(pod, gpuResources)).To(BeFalse())
		})
	})

	Context("computeAggregatedUtilization", func() {
		It("should compute Max aggregation correctly", func() {
			maxPolicy := kaiwo.AggregationPolicyMax
			gw := &kaiwo.GpuWorkload{
				Spec: kaiwo.GpuWorkloadSpec{
					AggregationPolicy: &maxPolicy,
				},
				Status: kaiwo.GpuWorkloadStatus{
					PodUtilizations: []kaiwo.PodGpuUtilization{
						{PodName: "pod-a", GpuID: "0", Utilization: 10.0},
						{PodName: "pod-a", GpuID: "1", Utilization: 20.0},
						{PodName: "pod-b", GpuID: "0", Utilization: 50.0},
					},
				},
			}
			reconciler := &GpuWorkloadReconciler{}
			result := reconciler.computeAggregatedUtilization(gw)
			Expect(result).NotTo(BeNil())
			// pod-a avg = 15, pod-b avg = 50; max = 50
			Expect(*result).To(BeNumerically("~", 50.0))
		})

		It("should compute Min aggregation correctly", func() {
			minPolicy := kaiwo.AggregationPolicyMin
			gw := &kaiwo.GpuWorkload{
				Spec: kaiwo.GpuWorkloadSpec{
					AggregationPolicy: &minPolicy,
				},
				Status: kaiwo.GpuWorkloadStatus{
					PodUtilizations: []kaiwo.PodGpuUtilization{
						{PodName: "pod-a", GpuID: "0", Utilization: 10.0},
						{PodName: "pod-a", GpuID: "1", Utilization: 20.0},
						{PodName: "pod-b", GpuID: "0", Utilization: 50.0},
					},
				},
			}
			reconciler := &GpuWorkloadReconciler{}
			result := reconciler.computeAggregatedUtilization(gw)
			Expect(result).NotTo(BeNil())
			// pod-a avg = 15, pod-b avg = 50; min = 15
			Expect(*result).To(BeNumerically("~", 15.0))
		})

		It("should compute Avg aggregation correctly", func() {
			avgPolicy := kaiwo.AggregationPolicyAvg
			gw := &kaiwo.GpuWorkload{
				Spec: kaiwo.GpuWorkloadSpec{
					AggregationPolicy: &avgPolicy,
				},
				Status: kaiwo.GpuWorkloadStatus{
					PodUtilizations: []kaiwo.PodGpuUtilization{
						{PodName: "pod-a", GpuID: "0", Utilization: 10.0},
						{PodName: "pod-a", GpuID: "1", Utilization: 20.0},
						{PodName: "pod-b", GpuID: "0", Utilization: 50.0},
					},
				},
			}
			reconciler := &GpuWorkloadReconciler{}
			result := reconciler.computeAggregatedUtilization(gw)
			Expect(result).NotTo(BeNil())
			// pod-a avg = 15, pod-b avg = 50; avg = 32.5
			Expect(*result).To(BeNumerically("~", 32.5))
		})

		It("should return nil for empty utilizations", func() {
			gw := &kaiwo.GpuWorkload{
				Status: kaiwo.GpuWorkloadStatus{},
			}
			reconciler := &GpuWorkloadReconciler{}
			result := reconciler.computeAggregatedUtilization(gw)
			Expect(result).To(BeNil())
		})
	})
})
