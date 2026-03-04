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
	"os"
	"time"

	configapi "github.com/silogen/kaiwo/apis/config/v1alpha1"
	kaiwo "github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
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
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: record.NewFakeRecorder(10),
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
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: record.NewFakeRecorder(10),
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
				AnnotationGracePeriod: "5m",
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
			Expect(spec.GracePeriod).NotTo(BeNil())
			Expect(spec.GracePeriod.Duration).To(Equal(5 * time.Minute))
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
				AnnotationGracePeriod: "invalid",
				AnnotationPolicy:      "InvalidPolicy",
				AnnotationAggregation: "InvalidAgg",
			}
			spec := &kaiwo.GpuWorkloadSpec{
				GpuResources: map[string]int{"amd.com/gpu": 1},
			}
			parseAnnotationsIntoSpec(annotations, spec)

			Expect(spec.UtilizationThreshold).To(BeNil())
			Expect(spec.GracePeriod).To(BeNil())
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

		It("should return true with only grace-period (implicit enable)", func() {
			annotations := map[string]string{
				AnnotationGracePeriod: "5m",
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
			result := reconciler.computeAggregatedUtilization(gw, configapi.KaiwoGpuPreemptionConfig{})
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
			result := reconciler.computeAggregatedUtilization(gw, configapi.KaiwoGpuPreemptionConfig{})
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
			result := reconciler.computeAggregatedUtilization(gw, configapi.KaiwoGpuPreemptionConfig{})
			Expect(result).NotTo(BeNil())
			// pod-a avg = 15, pod-b avg = 50; avg = 32.5
			Expect(*result).To(BeNumerically("~", 32.5))
		})

		It("should return nil for empty utilizations", func() {
			gw := &kaiwo.GpuWorkload{
				Status: kaiwo.GpuWorkloadStatus{},
			}
			reconciler := &GpuWorkloadReconciler{}
			result := reconciler.computeAggregatedUtilization(gw, configapi.KaiwoGpuPreemptionConfig{})
			Expect(result).To(BeNil())
		})
	})

	Context("hasGpuResources", func() {
		It("should return true for pod requesting amd.com/gpu", func() {
			pod := &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{"amd.com/gpu": resource.MustParse("1")},
						},
					}},
				},
			}
			Expect(hasGpuResources(pod)).To(BeTrue())
		})

		It("should return true when GPU is in limits only", func() {
			pod := &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{"amd.com/gpu": resource.MustParse("1")},
						},
					}},
				},
			}
			Expect(hasGpuResources(pod)).To(BeTrue())
		})

		It("should return false for CPU-only pod", func() {
			pod := &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{"cpu": resource.MustParse("4")},
						},
					}},
				},
			}
			Expect(hasGpuResources(pod)).To(BeFalse())
		})
	})

	Context("computeTotalGpuResources", func() {
		It("should sum GPU resources across multiple pods", func() {
			pods := []corev1.Pod{
				{Spec: corev1.PodSpec{Containers: []corev1.Container{{
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{"amd.com/gpu": resource.MustParse("1")},
					},
				}}}},
				{Spec: corev1.PodSpec{Containers: []corev1.Container{{
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{"amd.com/gpu": resource.MustParse("1")},
					},
				}}}},
				{Spec: corev1.PodSpec{Containers: []corev1.Container{{
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{"amd.com/gpu": resource.MustParse("2")},
					},
				}}}},
			}
			total := computeTotalGpuResources(pods)
			Expect(total).To(HaveKeyWithValue("amd.com/gpu", 4))
		})

		It("should handle multiple GPU resource types", func() {
			pods := []corev1.Pod{
				{Spec: corev1.PodSpec{Containers: []corev1.Container{{
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							"amd.com/gpu":   resource.MustParse("2"),
							"amd.com/gpu-0": resource.MustParse("1"),
						},
					},
				}}}},
				{Spec: corev1.PodSpec{Containers: []corev1.Container{{
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							"amd.com/gpu-0": resource.MustParse("3"),
						},
					},
				}}}},
			}
			total := computeTotalGpuResources(pods)
			Expect(total).To(HaveKeyWithValue("amd.com/gpu", 2))
			Expect(total).To(HaveKeyWithValue("amd.com/gpu-0", 4))
		})

		It("should return empty map for no pods", func() {
			total := computeTotalGpuResources(nil)
			Expect(total).To(BeEmpty())
		})
	})

	Context("gpuResourcesEqual", func() {
		It("should return true for equal maps", func() {
			a := map[string]int{"amd.com/gpu": 4}
			b := map[string]int{"amd.com/gpu": 4}
			Expect(gpuResourcesEqual(a, b)).To(BeTrue())
		})

		It("should return false for different values", func() {
			a := map[string]int{"amd.com/gpu": 4}
			b := map[string]int{"amd.com/gpu": 2}
			Expect(gpuResourcesEqual(a, b)).To(BeFalse())
		})

		It("should return false for different keys", func() {
			a := map[string]int{"amd.com/gpu": 4}
			b := map[string]int{"amd.com/gpu": 4, "amd.com/gpu-0": 1}
			Expect(gpuResourcesEqual(a, b)).To(BeFalse())
		})

		It("should return true for both empty", func() {
			Expect(gpuResourcesEqual(map[string]int{}, map[string]int{})).To(BeTrue())
		})
	})

	Context("isPodOwnedByWorkload", func() {
		ctx := context.Background()

		It("should match bare pod by UID", func() {
			podUID := types.UID("bare-pod-uid-12345")
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{UID: podUID},
			}
			gw := &kaiwo.GpuWorkload{
				Spec: kaiwo.GpuWorkloadSpec{
					WorkloadRef: kaiwo.WorkloadReference{Kind: "Pod", UID: podUID},
				},
			}
			reconciler := &GpuWorkloadReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			Expect(reconciler.isPodOwnedByWorkload(ctx, pod, gw)).To(BeTrue())
		})

		It("should match single-hop Job owner", func() {
			jobUID := types.UID("job-uid-12345")
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: "batch/v1",
						Kind:       "Job",
						Name:       "my-job",
						UID:        jobUID,
					}},
				},
			}
			gw := &kaiwo.GpuWorkload{
				Spec: kaiwo.GpuWorkloadSpec{
					WorkloadRef: kaiwo.WorkloadReference{Kind: "Job", UID: jobUID},
				},
			}
			reconciler := &GpuWorkloadReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			Expect(reconciler.isPodOwnedByWorkload(ctx, pod, gw)).To(BeTrue())
		})

		It("should match multi-hop Deployment owner via ReplicaSet chain", func() {
			isController := true

			deploy := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-chain-deploy",
					Namespace: "default",
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "test-chain"}},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "test-chain"}},
						Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "busybox"}}},
					},
				},
			}
			Expect(k8sClient.Create(ctx, deploy)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, deploy) }()

			rs := &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-chain-rs",
					Namespace: "default",
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       deploy.Name,
						UID:        deploy.UID,
						Controller: &isController,
					}},
				},
				Spec: appsv1.ReplicaSetSpec{
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "test-chain"}},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "test-chain"}},
						Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "busybox"}}},
					},
				},
			}
			Expect(k8sClient.Create(ctx, rs)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, rs) }()

			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: "apps/v1",
						Kind:       "ReplicaSet",
						Name:       rs.Name,
						UID:        rs.UID,
						Controller: &isController,
					}},
				},
				Spec: corev1.PodSpec{Containers: []corev1.Container{{
					Name:  "c",
					Image: "busybox",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{"amd.com/gpu": resource.MustParse("1")},
					},
				}}},
			}

			gw := &kaiwo.GpuWorkload{
				Spec: kaiwo.GpuWorkloadSpec{
					WorkloadRef: kaiwo.WorkloadReference{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       deploy.Name,
						UID:        deploy.UID,
					},
				},
			}

			reconciler := &GpuWorkloadReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			Expect(reconciler.isPodOwnedByWorkload(ctx, pod, gw)).To(BeTrue())
		})

		It("should NOT match unrelated pod", func() {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					UID: "unrelated-pod-uid",
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: "batch/v1",
						Kind:       "Job",
						Name:       "other-job",
						UID:        "other-job-uid",
					}},
				},
			}
			gw := &kaiwo.GpuWorkload{
				Spec: kaiwo.GpuWorkloadSpec{
					WorkloadRef: kaiwo.WorkloadReference{Kind: "Job", UID: "my-job-uid"},
				},
			}
			reconciler := &GpuWorkloadReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			Expect(reconciler.isPodOwnedByWorkload(ctx, pod, gw)).To(BeFalse())
		})
	})

	Context("extractGpuResources with InitContainers", func() {
		It("should extract GPU resources from init containers only", func() {
			pod := &corev1.Pod{
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									"amd.com/gpu": resource.MustParse("4"),
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									"cpu": resource.MustParse("2"),
								},
							},
						},
					},
				},
			}
			result := extractGpuResources(pod)
			Expect(result).To(HaveKeyWithValue("amd.com/gpu", 4))
		})

		It("should take max of init container and regular container", func() {
			pod := &corev1.Pod{
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									"amd.com/gpu": resource.MustParse("8"),
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									"amd.com/gpu": resource.MustParse("2"),
								},
							},
						},
					},
				},
			}
			result := extractGpuResources(pod)
			Expect(result).To(HaveKeyWithValue("amd.com/gpu", 8))
		})
	})

	Context("config fallback chains", func() {
		var reconciler *GpuWorkloadReconciler
		var gw *kaiwo.GpuWorkload
		var emptyCfg configapi.KaiwoGpuPreemptionConfig

		BeforeEach(func() {
			reconciler = &GpuWorkloadReconciler{}
			gw = &kaiwo.GpuWorkload{}
			emptyCfg = configapi.KaiwoGpuPreemptionConfig{}
		})

		Context("getThreshold", func() {
			It("should use spec value when set", func() {
				threshold := 42.0
				gw.Spec.UtilizationThreshold = &threshold
				Expect(reconciler.getThreshold(gw, emptyCfg)).To(Equal(42.0))
			})

			It("should use config value when spec is nil", func() {
				threshold := 15.0
				cfg := configapi.KaiwoGpuPreemptionConfig{DefaultThreshold: &threshold}
				Expect(reconciler.getThreshold(gw, cfg)).To(Equal(15.0))
			})

			It("should use env var when spec and config are nil", func() {
				os.Setenv(EnvDefaultThreshold, "25.5")
				defer os.Unsetenv(EnvDefaultThreshold)
				Expect(reconciler.getThreshold(gw, emptyCfg)).To(BeNumerically("~", 25.5))
			})

			It("should fall back to default when nothing is set", func() {
				os.Unsetenv(EnvDefaultThreshold)
				Expect(reconciler.getThreshold(gw, emptyCfg)).To(Equal(DefaultUtilizationThreshold))
			})
		})

		Context("getGracePeriod", func() {
			It("should use spec value when set", func() {
				dur := metav1.Duration{Duration: 30 * time.Minute}
				gw.Spec.GracePeriod = &dur
				Expect(reconciler.getGracePeriod(gw, emptyCfg)).To(Equal(30 * time.Minute))
			})

			It("should use config value when spec is nil", func() {
				cfg := configapi.KaiwoGpuPreemptionConfig{DefaultGracePeriod: "20m"}
				Expect(reconciler.getGracePeriod(gw, cfg)).To(Equal(20 * time.Minute))
			})

			It("should fall back to default when nothing is set", func() {
				os.Unsetenv(EnvDefaultGracePeriod)
				Expect(reconciler.getGracePeriod(gw, emptyCfg)).To(Equal(DefaultGracePeriod))
			})
		})

		Context("getPreemptionPolicy", func() {
			It("should use spec value when set", func() {
				p := kaiwo.PreemptionPolicyAlways
				gw.Spec.PreemptionPolicy = &p
				Expect(reconciler.getPreemptionPolicy(gw, emptyCfg)).To(Equal(kaiwo.PreemptionPolicyAlways))
			})

			It("should use config value when spec is nil", func() {
				cfg := configapi.KaiwoGpuPreemptionConfig{DefaultPolicy: "Always"}
				Expect(reconciler.getPreemptionPolicy(gw, cfg)).To(Equal(kaiwo.PreemptionPolicyAlways))
			})

			It("should fall back to OnPressure by default", func() {
				os.Unsetenv(EnvDefaultPolicy)
				Expect(reconciler.getPreemptionPolicy(gw, emptyCfg)).To(Equal(kaiwo.PreemptionPolicyOnPressure))
			})
		})

		Context("getAggregationPolicy", func() {
			It("should use spec value when set", func() {
				a := kaiwo.AggregationPolicyMin
				gw.Spec.AggregationPolicy = &a
				Expect(reconciler.getAggregationPolicy(gw, emptyCfg)).To(Equal(kaiwo.AggregationPolicyMin))
			})

			It("should use config value when spec is nil", func() {
				cfg := configapi.KaiwoGpuPreemptionConfig{DefaultAggregation: "Avg"}
				Expect(reconciler.getAggregationPolicy(gw, cfg)).To(Equal(kaiwo.AggregationPolicyAvg))
			})

			It("should fall back to Max by default", func() {
				os.Unsetenv(EnvDefaultAggregation)
				Expect(reconciler.getAggregationPolicy(gw, emptyCfg)).To(Equal(kaiwo.AggregationPolicyMax))
			})
		})

		Context("getTTL", func() {
			It("should use spec value when set", func() {
				dur := metav1.Duration{Duration: 48 * time.Hour}
				gw.Spec.TTLAfterFinished = &dur
				Expect(reconciler.getTTL(gw, emptyCfg)).To(Equal(48 * time.Hour))
			})

			It("should use config value when spec is nil", func() {
				cfg := configapi.KaiwoGpuPreemptionConfig{DefaultTTL: "12h"}
				Expect(reconciler.getTTL(gw, cfg)).To(Equal(12 * time.Hour))
			})

			It("should fall back to default when nothing is set", func() {
				os.Unsetenv(EnvDefaultTTL)
				Expect(reconciler.getTTL(gw, emptyCfg)).To(Equal(DefaultTTL))
			})
		})
	})
})
