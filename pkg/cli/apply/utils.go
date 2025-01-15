package cli

import (
	"fmt"
	"github.com/modern-go/reflect2"
	"github.com/silogen/ai-workload-orchestrator/pkg/workloads"
	"github.com/spf13/cobra"
)

const defaultNamespace = "kaiwo"

// Exec flags

var (
	dryRun          bool
	createNamespace bool
)

// AddExecFlags adds flags that are needed for the execution of apply functions
func AddExecFlags(cmd *cobra.Command) {
	cmd.Flags().BoolVarP(&createNamespace, "create-namespace", "", false, "Create namespace if it does not exist")
	cmd.Flags().BoolVarP(&dryRun, "dry-run", "d", false, "Print the generated workload manifest without submitting it")
}

func GetExecFlags() workloads.ExecFlags {
	return workloads.ExecFlags{
		CreateNamespace: createNamespace,
		DryRun:          dryRun,
	}
}

// Shared apply flags

var (
	name      string
	namespace string
	gpus      int
	template  string
	path      string
)

func AddSharedFlags(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&name, "name", "", "", "Name of the workload")
	cmd.Flags().StringVarP(&namespace, "namespace", "n", defaultNamespace, "Namespace of the workload")
	cmd.Flags().IntVarP(&gpus, "gpus", "g", 0, "Number of GPUs to use")
	cmd.Flags().StringVarP(&template, "template", "t", "", "Path to a custom template to use for the workload. If not provided, a default template will be used")
	cmd.Flags().StringVarP(&path, "path", "p", "", "absolute or relative path to workload code and entrypoint/serveconfig directory")
}

func GetSharedFlags() workloads.SharedFlags {
	return workloads.SharedFlags{
		Name:      name,
		Namespace: namespace,
		GPUs:      gpus,
		Template:  template,
		Path:      path,
	}
}

func RunApply(workload workloads.Workload) {
	// TODO Load

	fmt.Println("Applying workload...")
	fmt.Println(reflect2.TypeOf(workload))

	//
}
