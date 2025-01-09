package templates

import (
	_ "embed"
	"fmt"
)

// WorkloadArgs is a struct that holds the high-level arguments used for all workloads
type WorkloadArgs struct {
	Path         string
	Image        string
	Name         string
	Namespace    string
	Type         string
	GPUs         int
	TemplatePath string
	DryRun       bool
}

func ValidateWorkloadArgs(args WorkloadArgs) error {
	if args.Path == "" || args.GPUs <= 0 {
		return fmt.Errorf("invalid flags: ensure --path, --type, and --gpus are provided")
	}
	return nil
}

type WorkloadLoader interface {
	// Loads a workload from a path
	Load(path string) error

	// Returns the default teplate for the workloader
	DefaultTemplate() []byte

	// Lists the files that should be ignored in the ConfigMap
	IgnoreFiles() []string
}
