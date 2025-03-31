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

package common

import (
	"context"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestKaiwoLocalQueueCommand_Build(t *testing.T) {
	t.Run("Test", func(t *testing.T) {
		objectKey := client.ObjectKey{
			Namespace: "test",
			Name:      "test-name",
		}
		reconciler := NewLocalQueueReconciler(objectKey)

		builtObject, err := reconciler.Build(context.TODO(), nil)
		if err != nil {
			t.Fatalf("failed to build command: %v", err)
		}
		if objectKey.Name != builtObject.GetName() {
			t.Errorf("name does not match")
		}
		if objectKey.Namespace != builtObject.GetNamespace() {
			t.Errorf("namespace does not match")
		}
		if objectKey.Name != string(builtObject.Spec.ClusterQueue) {
			t.Errorf("cluster queue name does not match")
		}
	})
}
