package utils

// WorkloadArgs is a struct that holds the high-level arguments used for all workloads
type WorkloadArgs struct {
	Path                string
	Image               string
	Queue               string
	Name                string
	Namespace           string
	Type                string
	GPUs                int
	TemplatePath        string
	DryRun              bool
	CreateNamespace     bool
	TtlMinAfterFinished int
	ConfigMap	bool
}
