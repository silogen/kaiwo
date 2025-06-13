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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestResources(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Resources suite")
}

var _ = Describe("Resources calculation", func() {
	//ctx := context.Background()
	//ctx = common.SetContextConfig(ctx, &configapi.KaiwoConfig{
	//	Spec: configapi.KaiwoConfigSpec{
	//		Nodes: configapi.KaiwoNodeConfig{
	//			ExcludeMasterNodesFromNodePools: true,
	//		},
	//	},
	//})
	//clusterCtx := common.ClusterContext{
	//	Nodes: []common.NodeInfo{
	//		{
	//			Node: corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "test-cpu-node"}},
	//		},
	//		{
	//			Node: corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "test-gpu-node-partitioned-large"}},
	//			GpuInfo: &common.NodeGpuInfo{
	//				PhysicalCount:     8,
	//				LogicalCount:      64,
	//				LogicalVramPerGpu: resource.MustParse("24G"),
	//				ResourceName:      common.AmdGpuResourceName,
	//			},
	//		},
	//		{
	//			Node: corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "test-gpu-node-partitioned-small"}},
	//			GpuInfo: &common.NodeGpuInfo{
	//				PhysicalCount:     4,
	//				LogicalCount:      32,
	//				LogicalVramPerGpu: resource.MustParse("24G"),
	//				ResourceName:      common.AmdGpuResourceName,
	//			},
	//		},
	//		{
	//			Node: corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "test-gpu-node-physical-large"}},
	//			GpuInfo: &common.NodeGpuInfo{
	//				PhysicalCount:     8,
	//				LogicalCount:      8,
	//				LogicalVramPerGpu: resource.MustParse("192G"),
	//				ResourceName:      common.AmdGpuResourceName,
	//			},
	//		},
	//		{
	//			Node: corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "test-gpu-node-physical-small"}},
	//			GpuInfo: &common.NodeGpuInfo{
	//				PhysicalCount:     4,
	//				LogicalCount:      4,
	//				LogicalVramPerGpu: resource.MustParse("192G"),
	//				ResourceName:      common.AmdGpuResourceName,
	//			},
	//		},
	//	},
	//}

	//makeResourceConfig := func(gpuResourceRequirements *v1alpha1.GpuResourceRequirements, replicas *int) (*common.GpuSchedulingResult, error) {
	//	workload := &v1alpha1.KaiwoJob{
	//		Spec: v1alpha1.KaiwoJobSpec{
	//			CommonMetaSpec: v1alpha1.CommonMetaSpec{
	//				GpuResources: gpuResourceRequirements,
	//				Replicas:     replicas,
	//			},
	//		},
	//	}
	//	return common.CalculateGpuRequirements(ctx, clusterCtx, workload.Spec.GpuResources, replicas)
	//}

	//When("a workload defines physical GPUs that fit onto a single node", func() {
	//	resourceConfig, err := makeResourceConfig(&v1alpha1.GpuResourceRequirements{
	//		Count: baseutils.Pointer(8),
	//	}, baseutils.Pointer(1))
	//
	//	It("calculates the correct values", func() {
	//		Expect(err).NotTo(HaveOccurred())
	//		Expect(resourceConfig.Replicas).To(Equal(1))
	//		Expect(resourceConfig.GetTotalGpus()).To(Equal(8))
	//		Expect(resourceConfig.GpuCountPerReplica).To(Equal(8))
	//	})
	//})
	//
	//When("a workload defines partitioned GPUs that fit onto a single node", func() {
	//	resourceConfig, err := makeResourceConfig(&v1alpha1.GpuResourceRequirements{
	//		Count: baseutils.Pointer(54),
	//		Partitioned: true,
	//	}, baseutils.Pointer(1))
	//
	//	It("calculates the correct values", func() {
	//		Expect(err).NotTo(HaveOccurred())
	//		Expect(resourceConfig.Replicas).To(Equal(1))
	//		Expect(resourceConfig.GetTotalGpus()).To(Equal(54))
	//		Expect(resourceConfig.GpuCountPerReplica).To(Equal(54))
	//	})
	//})
	//
	//When("a workload defines GPU memory that fits into a partition", func() {
	//	resourceConfig, err := makeResourceConfig(corev1.ResourceList{
	//		common.AmdGpuMemoryResourceName: resource.MustParse("20Gi"),
	//	}, baseutils.Pointer(1), v1alpha1.GpuNodeTypeAny)
	//
	//	It("calculates the correct values", func() {
	//		Expect(err).NotTo(HaveOccurred())
	//		Expect(resourceConfig.Replicas).To(Equal(1))
	//		Expect(resourceConfig.TotalGpus).To(Equal(1))
	//		Expect(resourceConfig.GpusPerReplica).To(Equal(1))
	//		Expect(resourceConfig.GpuNodeType).To(Equal(v1alpha1.GpuNodeTypePartitioned))
	//	})
	//})
	//
	//When("a workload defines GPU memory that fits into a physical GPU", func() {
	//	resourceConfig, err := makeResourceConfig(corev1.ResourceList{
	//		common.AmdGpuMemoryResourceName: resource.MustParse("180Gi"),
	//	}, baseutils.Pointer(1), v1alpha1.GpuNodeTypeAny)
	//
	//	It("calculates the correct values", func() {
	//		Expect(err).NotTo(HaveOccurred())
	//		Expect(resourceConfig.Replicas).To(Equal(1))
	//		Expect(resourceConfig.TotalGpus).To(Equal(1))
	//		Expect(resourceConfig.GpusPerReplica).To(Equal(1))
	//		Expect(resourceConfig.GpuNodeType).To(Equal(v1alpha1.GpuNodeTypePhysical))
	//	})
	//})
	//
	//When("a workload defines GPU memory that fits into two partitions", func() {
	//	resourceConfig, err := makeResourceConfig(corev1.ResourceList{
	//		common.AmdGpuMemoryResourceName: resource.MustParse("30Gi"),
	//	}, baseutils.Pointer(1), v1alpha1.GpuNodeTypeAny)
	//
	//	It("calculates the correct values", func() {
	//		Expect(err).NotTo(HaveOccurred())
	//		Expect(resourceConfig.Replicas).To(Equal(1))
	//		Expect(resourceConfig.TotalGpus).To(Equal(2))
	//		Expect(resourceConfig.GpusPerReplica).To(Equal(2))
	//		Expect(resourceConfig.GpuNodeType).To(Equal(v1alpha1.GpuNodeTypePartitioned))
	//	})
	//})
	//
	//When("a workload defines GPU memory that fits into two physical GPUs", func() {
	//	resourceConfig, err := makeResourceConfig(corev1.ResourceList{
	//		common.AmdGpuMemoryResourceName: resource.MustParse("200Gi"),
	//	}, baseutils.Pointer(1), v1alpha1.GpuNodeTypeAny)
	//
	//	It("calculates the correct values", func() {
	//		Expect(err).NotTo(HaveOccurred())
	//		Expect(resourceConfig.Replicas).To(Equal(1))
	//		Expect(resourceConfig.TotalGpus).To(Equal(2))
	//		Expect(resourceConfig.GpusPerReplica).To(Equal(2))
	//		Expect(resourceConfig.GpuNodeType).To(Equal(v1alpha1.GpuNodeTypePhysical))
	//	})
	//})
	//
	//When("a workload defines GPU memory that fits into a partition, but physical is requested", func() {
	//	resourceConfig, err := makeResourceConfig(corev1.ResourceList{
	//		common.AmdGpuMemoryResourceName: resource.MustParse("1Gi"),
	//	}, baseutils.Pointer(1), v1alpha1.GpuNodeTypePhysical)
	//
	//	It("calculates the correct values", func() {
	//		Expect(err).NotTo(HaveOccurred())
	//		Expect(resourceConfig.Replicas).To(Equal(1))
	//		Expect(resourceConfig.TotalGpus).To(Equal(1))
	//		Expect(resourceConfig.GpusPerReplica).To(Equal(1))
	//		Expect(resourceConfig.GpuNodeType).To(Equal(v1alpha1.GpuNodeTypePhysical))
	//	})
	//})
	//
	//When("a workload defines GPU memory that fits into a physical GPU, but partitions are requested", func() {
	//	resourceConfig, err := makeResourceConfig(corev1.ResourceList{
	//		common.AmdGpuMemoryResourceName: resource.MustParse("192Gi"),
	//	}, baseutils.Pointer(1), v1alpha1.GpuNodeTypePartitioned)
	//
	//	It("calculates the correct values", func() {
	//		Expect(err).NotTo(HaveOccurred())
	//		Expect(resourceConfig.Replicas).To(Equal(1))
	//		Expect(resourceConfig.TotalGpus).To(Equal(8))
	//		Expect(resourceConfig.GpusPerReplica).To(Equal(8))
	//		Expect(resourceConfig.GpuNodeType).To(Equal(v1alpha1.GpuNodeTypePartitioned))
	//	})
	//})
})
