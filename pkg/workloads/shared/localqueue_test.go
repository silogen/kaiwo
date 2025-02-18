package workloadshared

import (
	"context"
	"testing"

	workloadutils "github.com/silogen/kaiwo/pkg/workloads2/utils"
)

func TestKaiwoLocalQueueCommand_Build(t *testing.T) {
	t.Run("Test", func(t *testing.T) {
		command := KaiwoLocalQueueCommand{CommandBase: workloadutils.CommandBase[workloadutils.CommandStateBase]{}, Namespace: "test-namespace", Name: "test-name"}

		builtObject, err := command.Build(context.TODO(), nil)
		if err != nil {
			t.Fatalf("failed to build command: %v", err)
		}
		if command.Name != builtObject.GetName() {
			t.Errorf("name does not match")
		}
		if command.Namespace != builtObject.GetNamespace() {
			t.Errorf("namespace does not match")
		}
	})
}
