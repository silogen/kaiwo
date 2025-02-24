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

package v1

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	batchv1 "k8s.io/api/batch/v1"
)

var _ = Describe("Job Webhook", func() {
	var (
		obj  *batchv1.Job
		hook *JobWebhook
	)

	BeforeEach(func() {
		obj = &batchv1.Job{}
		hook = &JobWebhook{}

		Expect(hook).NotTo(BeNil(), "Expected JobWebhook to be initialized")
		Expect(obj).NotTo(BeNil(), "Expected Job object to be initialized")
	})

	Context("When mutating a Job under the Defaulting Webhook", func() {
		It("Should not modify jobs without the special label", func() {
			obj.Labels = map[string]string{
				"some-other-label": "true",
			}

			err := hook.Default(context.TODO(), obj)
			Expect(err).ToNot(HaveOccurred(), "Expected no error in mutation")
		})

		It("Should mutate jobs with the special label into KaiwoJobs", func() {
			obj.Labels = map[string]string{
				"kaiwo.silogen.ai/managed": "true",
			}

			err := hook.Default(context.TODO(), obj)
			Expect(err).ToNot(HaveOccurred(), "Expected successful mutation")

			// Expect the mutated object to match a KaiwoJob spec
			Expect(obj.Labels["kaiwo.silogen.ai/managed"]).To(Equal("true"))
		})
	})
})
