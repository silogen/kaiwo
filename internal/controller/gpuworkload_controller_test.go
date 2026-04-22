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
	batchv1 "k8s.io/api/batch/v1"
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

	Context("mergePreemptionAnnotations", func() {
		It("should return empty map when both inputs are nil", func() {
			result := mergePreemptionAnnotations(nil, nil)
			Expect(result).To(BeEmpty())
		})

		It("should use namespace annotations when workload has none", func() {
			ns := map[string]string{
				AnnotationGracePeriod: "15m",
				AnnotationPolicy:      "Always",
				"unrelated/key":       "ignored",
			}
			result := mergePreemptionAnnotations(ns, nil)
			Expect(result).To(HaveLen(2))
			Expect(result[AnnotationGracePeriod]).To(Equal("15m"))
			Expect(result[AnnotationPolicy]).To(Equal("Always"))
		})

		It("should use workload annotations when namespace has none", func() {
			workload := map[string]string{
				AnnotationThreshold: "20",
				"other/annotation":  "ignored",
			}
			result := mergePreemptionAnnotations(nil, workload)
			Expect(result).To(HaveLen(1))
			Expect(result[AnnotationThreshold]).To(Equal("20"))
		})

		It("should let workload annotations override namespace annotations", func() {
			ns := map[string]string{
				AnnotationGracePeriod: "15m",
				AnnotationPolicy:      "OnPressure",
				AnnotationThreshold:   "10",
			}
			workload := map[string]string{
				AnnotationPolicy:    "Always",
				AnnotationThreshold: "25",
			}
			result := mergePreemptionAnnotations(ns, workload)
			Expect(result).To(HaveLen(3))
			Expect(result[AnnotationGracePeriod]).To(Equal("15m"))
			Expect(result[AnnotationPolicy]).To(Equal("Always"))
			Expect(result[AnnotationThreshold]).To(Equal("25"))
		})

		It("should filter out non-preemption annotations from both sources", func() {
			ns := map[string]string{
				AnnotationEnabled:          "true",
				"kubernetes.io/some-label": "value",
			}
			workload := map[string]string{
				AnnotationTTL:            "12h",
				"app.kubernetes.io/name": "test",
			}
			result := mergePreemptionAnnotations(ns, workload)
			Expect(result).To(HaveLen(2))
			Expect(result).To(HaveKey(AnnotationEnabled))
			Expect(result).To(HaveKey(AnnotationTTL))
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
					TrackedPods: []kaiwo.TrackedPod{
						{PodName: "pod-a", GpuMetrics: []kaiwo.GpuMetric{
							{GpuID: "0", Utilization: 10.0},
							{GpuID: "1", Utilization: 20.0},
						}},
						{PodName: "pod-b", GpuMetrics: []kaiwo.GpuMetric{
							{GpuID: "0", Utilization: 50.0},
						}},
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
					TrackedPods: []kaiwo.TrackedPod{
						{PodName: "pod-a", GpuMetrics: []kaiwo.GpuMetric{
							{GpuID: "0", Utilization: 10.0},
							{GpuID: "1", Utilization: 20.0},
						}},
						{PodName: "pod-b", GpuMetrics: []kaiwo.GpuMetric{
							{GpuID: "0", Utilization: 50.0},
						}},
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
					TrackedPods: []kaiwo.TrackedPod{
						{PodName: "pod-a", GpuMetrics: []kaiwo.GpuMetric{
							{GpuID: "0", Utilization: 10.0},
							{GpuID: "1", Utilization: 20.0},
						}},
						{PodName: "pod-b", GpuMetrics: []kaiwo.GpuMetric{
							{GpuID: "0", Utilization: 50.0},
						}},
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
				Expect(os.Setenv(EnvDefaultThreshold, "25.5")).To(Succeed())
				defer func() { Expect(os.Unsetenv(EnvDefaultThreshold)).To(Succeed()) }()
				Expect(reconciler.getThreshold(gw, emptyCfg)).To(BeNumerically("~", 25.5))
			})

			It("should fall back to default when nothing is set", func() {
				Expect(os.Unsetenv(EnvDefaultThreshold)).To(Succeed())
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
				dur := &metav1.Duration{Duration: 20 * time.Minute}
				cfg := configapi.KaiwoGpuPreemptionConfig{DefaultGracePeriod: dur}
				Expect(reconciler.getGracePeriod(gw, cfg)).To(Equal(20 * time.Minute))
			})

			It("should fall back to default when nothing is set", func() {
				Expect(os.Unsetenv(EnvDefaultGracePeriod)).To(Succeed())
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
				Expect(os.Unsetenv(EnvDefaultPolicy)).To(Succeed())
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
				Expect(os.Unsetenv(EnvDefaultAggregation)).To(Succeed())
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
				dur := &metav1.Duration{Duration: 12 * time.Hour}
				cfg := configapi.KaiwoGpuPreemptionConfig{DefaultTTL: dur}
				Expect(reconciler.getTTL(gw, cfg)).To(Equal(12 * time.Hour))
			})

			It("should fall back to default when nothing is set", func() {
				Expect(os.Unsetenv(EnvDefaultTTL)).To(Succeed())
				Expect(reconciler.getTTL(gw, emptyCfg)).To(Equal(DefaultTTL))
			})
		})
	})

	Context("podToGpuWorkload namespace annotation discovery", func() {
		ctx := context.Background()

		createNamespace := func(name string, annotations map[string]string) *corev1.Namespace {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:        name,
					Annotations: annotations,
				},
			}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())
			return ns
		}

		createJobAndPod := func(namespace string, jobAnnotations map[string]string) (*batchv1.Job, *corev1.Pod) {
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-job",
					Namespace:   namespace,
					Annotations: jobAnnotations,
				},
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{
								Name:  "gpu",
								Image: "busybox",
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{"amd.com/gpu": resource.MustParse("2")},
									Limits:   corev1.ResourceList{"amd.com/gpu": resource.MustParse("2")},
								},
							}},
							RestartPolicy: corev1.RestartPolicyNever,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, job)).To(Succeed())

			isController := true
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-job-pod",
					Namespace: namespace,
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: "batch/v1",
						Kind:       "Job",
						Name:       job.Name,
						UID:        job.UID,
						Controller: &isController,
					}},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "gpu",
						Image: "busybox",
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{"amd.com/gpu": resource.MustParse("2")},
							Limits:   corev1.ResourceList{"amd.com/gpu": resource.MustParse("2")},
						},
					}},
					RestartPolicy: corev1.RestartPolicyNever,
				},
			}
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())
			return job, pod
		}

		It("should create GpuWorkload from namespace annotations when workload has none", func() {
			ns := createNamespace("ns-annotated", map[string]string{
				AnnotationEnabled:     "true",
				AnnotationGracePeriod: "20m",
				AnnotationPolicy:      "Always",
			})
			defer func() { _ = k8sClient.Delete(ctx, ns) }()

			job, pod := createJobAndPod("ns-annotated", nil)
			defer func() {
				_ = k8sClient.Delete(ctx, pod)
				_ = k8sClient.Delete(ctx, job)
			}()

			reconciler := &GpuWorkloadReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: record.NewFakeRecorder(10),
			}
			requests := reconciler.podToGpuWorkload(ctx, pod)
			Expect(requests).NotTo(BeEmpty())

			gwName := requests[0].Name
			var gw kaiwo.GpuWorkload
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "ns-annotated", Name: gwName}, &gw)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, &gw) }()

			Expect(gw.Spec.GpuResources).To(HaveKeyWithValue("amd.com/gpu", 2))
			Expect(gw.Spec.GracePeriod).NotTo(BeNil())
			Expect(gw.Spec.GracePeriod.Duration).To(Equal(20 * time.Minute))
			Expect(gw.Spec.PreemptionPolicy).NotTo(BeNil())
			Expect(*gw.Spec.PreemptionPolicy).To(Equal(kaiwo.PreemptionPolicyAlways))
		})

		It("should let workload annotations override namespace annotations", func() {
			ns := createNamespace("ns-override", map[string]string{
				AnnotationGracePeriod: "20m",
				AnnotationPolicy:      "OnPressure",
				AnnotationThreshold:   "10",
			})
			defer func() { _ = k8sClient.Delete(ctx, ns) }()

			job, pod := createJobAndPod("ns-override", map[string]string{
				AnnotationPolicy:    "Always",
				AnnotationThreshold: "30",
			})
			defer func() {
				_ = k8sClient.Delete(ctx, pod)
				_ = k8sClient.Delete(ctx, job)
			}()

			reconciler := &GpuWorkloadReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: record.NewFakeRecorder(10),
			}
			requests := reconciler.podToGpuWorkload(ctx, pod)
			Expect(requests).NotTo(BeEmpty())

			gwName := requests[0].Name
			var gw kaiwo.GpuWorkload
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "ns-override", Name: gwName}, &gw)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, &gw) }()

			Expect(gw.Spec.GracePeriod).NotTo(BeNil())
			Expect(gw.Spec.GracePeriod.Duration).To(Equal(20 * time.Minute))
			Expect(gw.Spec.PreemptionPolicy).NotTo(BeNil())
			Expect(*gw.Spec.PreemptionPolicy).To(Equal(kaiwo.PreemptionPolicyAlways))
			Expect(gw.Spec.UtilizationThreshold).NotTo(BeNil())
			Expect(*gw.Spec.UtilizationThreshold).To(BeNumerically("~", 30.0))
		})

		It("should not create GpuWorkload when neither namespace nor workload is annotated", func() {
			ns := createNamespace("ns-plain", nil)
			defer func() { _ = k8sClient.Delete(ctx, ns) }()

			job, pod := createJobAndPod("ns-plain", nil)
			defer func() {
				_ = k8sClient.Delete(ctx, pod)
				_ = k8sClient.Delete(ctx, job)
			}()

			reconciler := &GpuWorkloadReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: record.NewFakeRecorder(10),
			}
			requests := reconciler.podToGpuWorkload(ctx, pod)
			Expect(requests).To(BeEmpty())
		})
	})

	Context("preemption evaluation", func() {
		var (
			reconciler *GpuWorkloadReconciler
			ns         *corev1.Namespace
			gpuCfg     configapi.KaiwoGpuPreemptionConfig
		)

		makeGW := func(name string, phase kaiwo.GpuWorkloadPhase, gpus int, opts ...func(*kaiwo.GpuWorkload)) *kaiwo.GpuWorkload {
			onPressure := kaiwo.PreemptionPolicyOnPressure
			gracePeriod := metav1.Duration{Duration: 5 * time.Second}
			gw := &kaiwo.GpuWorkload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: ns.Name,
				},
				Spec: kaiwo.GpuWorkloadSpec{
					WorkloadRef: kaiwo.WorkloadReference{
						APIVersion: "batch/v1",
						Kind:       "Job",
						Name:       name,
						UID:        types.UID(name + "-uid"),
					},
					GpuResources:     map[string]int{"amd.com/gpu": gpus},
					PreemptionPolicy: &onPressure,
					GracePeriod:      &gracePeriod,
				},
			}
			for _, fn := range opts {
				fn(gw)
			}
			ExpectWithOffset(1, k8sClient.Create(ctx, gw)).To(Succeed())

			gw.Status.Phase = phase
			if phase == kaiwo.GpuWorkloadPhaseIdle {
				idle := metav1.NewTime(time.Now().Add(-30 * time.Second))
				gw.Status.IdleSince = &idle
			}
			ExpectWithOffset(1, k8sClient.Status().Update(ctx, gw)).To(Succeed())
			return gw
		}

		BeforeEach(func() {
			ns = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "preempt-test-"}}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())

			reconciler = &GpuWorkloadReconciler{
				Client:   k8sClient,
				Scheme:   k8sClient.Scheme(),
				Recorder: record.NewFakeRecorder(10),
			}
			gpuCfg = configapi.KaiwoGpuPreemptionConfig{}
		})

		AfterEach(func() {
			_ = k8sClient.Delete(ctx, ns)
		})

		It("should preempt a single victim when it satisfies demand", func() {
			victim := makeGW("victim", kaiwo.GpuWorkloadPhaseIdle, 5)
			demand := makeGW("demand", kaiwo.GpuWorkloadPhasePendingGpu, 5)

			state, err := reconciler.classifyWorkloads(ctx, []kaiwo.GpuWorkload{*victim, *demand}, gpuCfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(reconciler.matchAndMarkVictims(ctx, state)).To(Succeed())

			var updated kaiwo.GpuWorkload
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns.Name, Name: "victim"}, &updated)).To(Succeed())
			Expect(updated.Status.Phase).To(Equal(kaiwo.GpuWorkloadPhasePreempting))
			Expect(updated.Status.PreemptedFor).To(Equal("demand"))
		})

		It("should accumulate multiple victims when one is insufficient", func() {
			victimA := makeGW("victim-a", kaiwo.GpuWorkloadPhaseIdle, 5)
			victimB := makeGW("victim-b", kaiwo.GpuWorkloadPhaseIdle, 5)
			demand := makeGW("demand", kaiwo.GpuWorkloadPhasePendingGpu, 6)

			state, err := reconciler.classifyWorkloads(ctx, []kaiwo.GpuWorkload{*victimA, *victimB, *demand}, gpuCfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(reconciler.matchAndMarkVictims(ctx, state)).To(Succeed())

			var updatedA, updatedB kaiwo.GpuWorkload
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns.Name, Name: "victim-a"}, &updatedA)).To(Succeed())
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns.Name, Name: "victim-b"}, &updatedB)).To(Succeed())
			Expect(updatedA.Status.Phase).To(Equal(kaiwo.GpuWorkloadPhasePreempting))
			Expect(updatedB.Status.Phase).To(Equal(kaiwo.GpuWorkloadPhasePreempting))
			Expect(updatedA.Status.PreemptedFor).To(Equal("demand"))
			Expect(updatedB.Status.PreemptedFor).To(Equal("demand"))
		})

		It("should not preempt when idle capacity is insufficient (all-or-nothing)", func() {
			victim := makeGW("small-victim", kaiwo.GpuWorkloadPhaseIdle, 3)
			makeGW("big-demand", kaiwo.GpuWorkloadPhasePendingGpu, 10)

			state, err := reconciler.classifyWorkloads(ctx, []kaiwo.GpuWorkload{*victim}, gpuCfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(reconciler.matchAndMarkVictims(ctx, state)).To(Succeed())

			var updated kaiwo.GpuWorkload
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns.Name, Name: "small-victim"}, &updated)).To(Succeed())
			Expect(updated.Status.Phase).To(Equal(kaiwo.GpuWorkloadPhaseIdle))
		})

		It("should subtract in-flight capacity from demand", func() {
			victim := makeGW("idle-victim", kaiwo.GpuWorkloadPhaseIdle, 3)
			demand := makeGW("inflight-demand", kaiwo.GpuWorkloadPhasePendingGpu, 5)

			inFlight := makeGW("already-freeing", kaiwo.GpuWorkloadPhaseIdle, 3)
			inFlight.Status.Phase = kaiwo.GpuWorkloadPhasePreempting
			inFlight.Status.PreemptedFor = "inflight-demand"
			Expect(k8sClient.Status().Update(ctx, inFlight)).To(Succeed())

			state, err := reconciler.classifyWorkloads(ctx, []kaiwo.GpuWorkload{*victim, *demand, *inFlight}, gpuCfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(reconciler.matchAndMarkVictims(ctx, state)).To(Succeed())

			var updated kaiwo.GpuWorkload
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns.Name, Name: "idle-victim"}, &updated)).To(Succeed())
			Expect(updated.Status.Phase).To(Equal(kaiwo.GpuWorkloadPhasePreempting))
		})

		It("should not double-claim a victim across two pending workloads", func() {
			victim := makeGW("shared-victim", kaiwo.GpuWorkloadPhaseIdle, 5)
			makeGW("demand-1", kaiwo.GpuWorkloadPhasePendingGpu, 5)
			demand2 := makeGW("demand-2", kaiwo.GpuWorkloadPhasePendingGpu, 5)

			state, err := reconciler.classifyWorkloads(ctx, []kaiwo.GpuWorkload{*victim, *demand2}, gpuCfg)
			Expect(err).NotTo(HaveOccurred())

			state.pendingByResource["amd.com/gpu"] = append(
				[]workloadEntry{{gw: demand2, gpuCount: demand2.Spec.GpuResources}},
				state.pendingByResource["amd.com/gpu"]...,
			)
			Expect(reconciler.matchAndMarkVictims(ctx, state)).To(Succeed())

			var updated kaiwo.GpuWorkload
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns.Name, Name: "shared-victim"}, &updated)).To(Succeed())
			Expect(updated.Status.Phase).To(Equal(kaiwo.GpuWorkloadPhasePreempting))
			Expect(updated.Status.PreemptedFor).To(Equal("demand-2"))
		})

		It("should skip victims still within grace period", func() {
			gw := &kaiwo.GpuWorkload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "young-victim",
					Namespace: ns.Name,
				},
				Spec: kaiwo.GpuWorkloadSpec{
					WorkloadRef: kaiwo.WorkloadReference{
						APIVersion: "batch/v1",
						Kind:       "Job",
						Name:       "young-victim",
						UID:        "young-uid",
					},
					GpuResources:     map[string]int{"amd.com/gpu": 5},
					PreemptionPolicy: func() *kaiwo.PreemptionPolicy { p := kaiwo.PreemptionPolicyOnPressure; return &p }(),
					GracePeriod:      &metav1.Duration{Duration: 10 * time.Minute},
				},
			}
			Expect(k8sClient.Create(ctx, gw)).To(Succeed())
			gw.Status.Phase = kaiwo.GpuWorkloadPhaseIdle
			recent := metav1.NewTime(time.Now().Add(-30 * time.Second))
			gw.Status.IdleSince = &recent
			Expect(k8sClient.Status().Update(ctx, gw)).To(Succeed())

			demand := makeGW("grace-demand", kaiwo.GpuWorkloadPhasePendingGpu, 5)

			state, err := reconciler.classifyWorkloads(ctx, []kaiwo.GpuWorkload{*gw, *demand}, gpuCfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(state.idleByResource["amd.com/gpu"]).To(BeEmpty())
		})

		It("should mark Always-policy workload as Preempting immediately after grace period", func() {
			always := kaiwo.PreemptionPolicyAlways
			victim := makeGW("always-victim", kaiwo.GpuWorkloadPhaseIdle, 5, func(gw *kaiwo.GpuWorkload) {
				gw.Spec.PreemptionPolicy = &always
			})

			state, err := reconciler.classifyWorkloads(ctx, []kaiwo.GpuWorkload{*victim}, gpuCfg)
			Expect(err).NotTo(HaveOccurred())

			var updated kaiwo.GpuWorkload
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns.Name, Name: "always-victim"}, &updated)).To(Succeed())
			Expect(updated.Status.Phase).To(Equal(kaiwo.GpuWorkloadPhasePreempting))
			Expect(state.idleByResource["amd.com/gpu"]).To(BeEmpty())
		})
	})
})
