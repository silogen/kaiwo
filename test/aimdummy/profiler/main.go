package main

import (
	"encoding/json"
	"fmt"
)

// discoveryResult represents the raw output from a discovery job.
// This is an internal type used only for parsing the JSON output.
type discoveryResult struct {
	Filename string                 `json:"filename"`
	Profile  discoveryProfileResult `json:"profile"`
	Models   []discoveryModelResult `json:"models"`
}

// discoveryProfileResult is the raw profile format from discovery job output
type discoveryProfileResult struct {
	Model          string            `json:"model"`
	QuantizedModel string            `json:"quantized_model"`
	Metadata       profileMetadata   `json:"metadata"`
	EngineArgs     map[string]any    `json:"engine_args"`
	EnvVars        map[string]string `json:"env_vars"`
}

// profileMetadata is the raw metadata format from discovery job output
type profileMetadata struct {
	Engine    string `json:"engine"`
	GPU       string `json:"gpu"`
	Precision string `json:"precision"`
	GPUCount  int32  `json:"gpu_count"`
	Metric    string `json:"metric"`
}

// discoveryModelResult represents a model in the raw discovery output
type discoveryModelResult struct {
	Name   string  `json:"name"`
	Source string  `json:"source"`
	SizeGB float64 `json:"size_gb"`
}

func main() {
	qwen := discoveryModelResult{Name: "smol2-135m", Source: "hf://HuggingFaceTB/SmolLM2-135M", SizeGB: 1}
	fakeprofilemeta := profileMetadata{Engine: "vllm", GPU: "AMD", Precision: "bf16", GPUCount: 0, Metric: "latency"}
	fakeprofileresult := discoveryProfileResult{
		Model:          "smol2-135m",
		QuantizedModel: "smol2-135m-bf16",
		Metadata:       fakeprofilemeta,
		EngineArgs:     map[string]any{},
		EnvVars:        map[string]string{}}
	fakediscoveryresult := []discoveryResult{{Filename: "smol2-135m", Profile: fakeprofileresult, Models: []discoveryModelResult{qwen}}}

	discoveryJson, _ := json.Marshal(fakediscoveryresult)

	fmt.Println(string(discoveryJson))

}
