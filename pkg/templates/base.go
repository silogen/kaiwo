package templates

import (
	_ "embed"
	"fmt"
)

// WorkloadArgs is a struct that holds the high-level arguments used for all workloads
type WorkloadArgs struct {
	Path            string
	Image           string
	Name            string
	Namespace       string
	Type            string
	GPUs            int
	TemplatePath    string
	DryRun          bool
	CreateNamespace bool
	NoUploadFolder  bool
}

func ValidateWorkloadArgs(args WorkloadArgs) error {
	if args.Path == "" || args.GPUs <= 0 {
		return fmt.Errorf("invalid flags: ensure --path, --type, and --gpus are provided")
	}
	return nil
}

type WorkloadLoader interface {
	// Load loads a workload from a path
	Load(path string) error

	// DefaultTemplate returns the default template for the workloader
	DefaultTemplate() []byte

	// IgnoreFiles lists the files that should be ignored in the ConfigMap
	IgnoreFiles() []string
}
