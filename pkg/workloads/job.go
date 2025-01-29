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

package workloads

import (
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
	kueuev1beta1 "sigs.k8s.io/kueue/apis/kueue/v1beta1"

	"github.com/silogen/kaiwo/pkg/k8s"
)

// JobFlags contain flags specific to job-workloads
type JobFlags struct {
	// The Kueue queue to use
	Queue string
}

func CreateLocalClusterQueueManifest(k8sClient client.Client, templateContext WorkloadTemplateConfig) (*kueuev1beta1.LocalQueue, error) {
	jobMeta, ok := templateContext.WorkloadMeta.(JobFlags)

	if !ok {
		return nil, fmt.Errorf("workload meta is not of type workloads.JobFlags")
	}

	// Handle jobs local queue
	localQueue, err := k8s.PrepareLocalClusterQueue(jobMeta.Queue, templateContext.Meta.Namespace, k8sClient)
	if err != nil {
		return nil, fmt.Errorf("error preparing local cluster queue: %v", err)
	}

	return localQueue, nil
}
