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

package gpu

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/silogen/kaiwo/apis/kaiwo/v1alpha1"
)

const (
	AmdGpuResourceName    = corev1.ResourceName("amd.com/gpu")
	NvidiaGpuResourceName = corev1.ResourceName("nvidia.com/gpu")
)

var (
	DefaultMemory = resource.MustParse("16Gi")
	DefaultCPU    = resource.MustParse("2")
)

func VendorToResourceName(vendor v1alpha1.GpuVendor) corev1.ResourceName {
	switch vendor {
	case v1alpha1.GpuVendorAmd:
		return AmdGpuResourceName
	case v1alpha1.GpuVendorNvidia:
		return NvidiaGpuResourceName
	}
	panic(fmt.Sprintf("unknown vendor: %v", vendor))
}
