// MIT License
//
// Copyright (c) 2025 Advanced Micro Devices, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package shared

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
)

func TestBuildDerivedTemplateAddsDerivedLabelAndClearsOwners(t *testing.T) {
	service := &aimv1alpha1.AIMService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example",
			Namespace: "default",
		},
		Spec: aimv1alpha1.AIMServiceSpec{
			AIMModelName: "image",
		},
	}

	template := BuildDerivedTemplate(service, "example-ovr-12345678", nil)

	if template == nil {
		t.Fatal("expected template to be constructed")
	}

	if template.Labels[LabelKeyDerivedTemplate] != LabelValueDerivedTemplate {
		t.Fatalf("expected derived template label %q to be %q, got %q",
			LabelKeyDerivedTemplate, LabelValueDerivedTemplate, template.Labels[LabelKeyDerivedTemplate])
	}

	if managedBy := template.Labels["app.kubernetes.io/managed-by"]; managedBy != LabelValueManagedBy {
		t.Fatalf("expected managed-by label to be %q, got %q", LabelValueManagedBy, managedBy)
	}

	if len(template.OwnerReferences) != 0 {
		t.Fatalf("expected no owner references, got %d", len(template.OwnerReferences))
	}
}
