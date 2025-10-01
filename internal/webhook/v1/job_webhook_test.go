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
