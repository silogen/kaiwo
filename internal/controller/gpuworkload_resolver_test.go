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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("GpuWorkload Resolver", func() {
	Context("GpuWorkloadName", func() {
		It("should generate deterministic name with UID prefix", func() {
			name := GpuWorkloadName("Deployment", "my-training", "a1b2c3d4-e5f6-7890-abcd-ef1234567890")
			Expect(name).To(Equal("deployment-my-training-a1b2c3d4"))
		})

		It("should handle short UIDs", func() {
			name := GpuWorkloadName("Job", "test", types.UID("abc"))
			Expect(name).To(Equal("job-test-abc"))
		})

		It("should lowercase the kind", func() {
			name := GpuWorkloadName("KaiwoJob", "my-job", "12345678-abcd")
			Expect(name).To(Equal("kaiwojob-my-job-12345678"))
		})
	})

	Context("gvkFromAPIVersionKind", func() {
		It("should parse core API version", func() {
			gvk := gvkFromAPIVersionKind("v1", "Pod")
			Expect(gvk.Group).To(Equal(""))
			Expect(gvk.Version).To(Equal("v1"))
			Expect(gvk.Kind).To(Equal("Pod"))
		})

		It("should parse group API version", func() {
			gvk := gvkFromAPIVersionKind("batch/v1", "Job")
			Expect(gvk.Group).To(Equal("batch"))
			Expect(gvk.Version).To(Equal("v1"))
			Expect(gvk.Kind).To(Equal("Job"))
		})

		It("should parse ray.io API version", func() {
			gvk := gvkFromAPIVersionKind("ray.io/v1", "RayJob")
			Expect(gvk.Group).To(Equal("ray.io"))
			Expect(gvk.Version).To(Equal("v1"))
			Expect(gvk.Kind).To(Equal("RayJob"))
		})
	})

	Context("gvkFromAPIVersionKind with invalid input", func() {
		It("should fall back to kind-only GVK for malformed apiVersion", func() {
			gvk := gvkFromAPIVersionKind("///invalid", "Widget")
			Expect(gvk.Kind).To(Equal("Widget"))
			Expect(gvk.Group).To(Equal(""))
			Expect(gvk.Version).To(Equal(""))
		})
	})

	Context("getControllerOwnerRef", func() {
		It("should find the controller owner ref", func() {
			trueVal := true
			falseVal := false
			refs := []metav1.OwnerReference{
				{Name: "non-controller", Controller: &falseVal},
				{Name: "controller-owner", Controller: &trueVal, APIVersion: "batch/v1", Kind: "Job", UID: "uid-123"},
			}
			result := getControllerOwnerRef(refs)
			Expect(result).NotTo(BeNil())
			Expect(result.Name).To(Equal("controller-owner"))
		})

		It("should return nil when no controller owner", func() {
			falseVal := false
			refs := []metav1.OwnerReference{
				{Name: "non-controller", Controller: &falseVal},
			}
			result := getControllerOwnerRef(refs)
			Expect(result).To(BeNil())
		})

		It("should return nil for empty refs", func() {
			result := getControllerOwnerRef(nil)
			Expect(result).To(BeNil())
		})
	})

	Context("ResolveRootOwner", func() {
		ctx := context.Background()

		It("should resolve a bare pod (no owner refs) to itself", func() {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "resolve-bare-pod",
					Namespace: "default",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "c", Image: "busybox"}},
				},
			}
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, pod) }()

			result, err := ResolveRootOwner(ctx, k8sClient, "default", pod.Name, "Pod", "v1", pod.UID)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Ref.Kind).To(Equal("Pod"))
			Expect(result.Ref.Name).To(Equal("resolve-bare-pod"))
			Expect(result.Ref.UID).To(Equal(pod.UID))
		})

		It("should resolve single-hop Job owner", func() {
			isController := true
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "resolve-test-job",
					Namespace: "default",
				},
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers:    []corev1.Container{{Name: "c", Image: "busybox"}},
							RestartPolicy: corev1.RestartPolicyNever,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, job)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, job) }()

			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "resolve-test-pod",
					Namespace: "default",
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: "batch/v1",
						Kind:       "Job",
						Name:       job.Name,
						UID:        job.UID,
						Controller: &isController,
					}},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "c", Image: "busybox"}},
				},
			}
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, pod) }()

			result, err := ResolveRootOwner(ctx, k8sClient, "default", pod.Name, "Pod", "v1", pod.UID)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Ref.Kind).To(Equal("Job"))
			Expect(result.Ref.Name).To(Equal("resolve-test-job"))
			Expect(result.Ref.UID).To(Equal(job.UID))
		})

		It("should return error when owner is not found", func() {
			isController := true
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "resolve-orphan-pod",
					Namespace: "default",
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: "batch/v1",
						Kind:       "Job",
						Name:       "nonexistent-job",
						UID:        "fake-uid",
						Controller: &isController,
					}},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "c", Image: "busybox"}},
				},
			}
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, pod) }()

			_, err := ResolveRootOwner(ctx, k8sClient, "default", pod.Name, "Pod", "v1", pod.UID)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})
	})
})
