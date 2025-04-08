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

package baseutils

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestConvertEntrypoint(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ConvertMultilineEntrypoint Suite")
}

var _ = Describe("ConvertMultilineEntrypoint", func() {
	DescribeTable("should correctly convert multi-line entrypoints",
		func(input string, isRayJob bool, expected interface{}) {
			result := ConvertMultilineEntrypoint(input, isRayJob)
			Expect(result).To(Equal(expected))
		},

		Entry("Single-line Bash command (K8s)",
			`/bin/bash -c 'echo "Hello World"'`, false,
			[]string{"/bin/sh", "-c", `/bin/bash -c 'echo "Hello World"'`}),

		Entry("Multi-line Python script with arguments (RayJob)",
			`python mounted/main.py
--model-name=meta-llama/Llama-3.1-8B-Instruct
--ds-config=./mounted/zero_3_offload_optim_param.json
--bucket=silogen-dev
--num-epochs=2
--num-devices=$NUM_GPUS
--batch-size-per-device=32
--eval-batch-size-per-device=32
--ctx-len=1024`, true,
			`python mounted/main.py
--model-name=meta-llama/Llama-3.1-8B-Instruct
--ds-config=./mounted/zero_3_offload_optim_param.json
--bucket=silogen-dev
--num-epochs=2
--num-devices=$NUM_GPUS
--batch-size-per-device=32
--eval-batch-size-per-device=32
--ctx-len=1024`),

		Entry("Detects and uses shebang for Bash (K8s)",
			`#!/bin/bash
 echo "Starting"
 apt update
 apt install -y vim
 echo "Done"`, false,
			[]string{"/bin/bash", "-c", ` echo "Starting"
 apt update
 apt install -y vim
 echo "Done"`}),

		Entry("Detects and uses shebang for SH (K8s)",
			`#!/bin/sh
 echo "Starting"
 apt update
 apt install -y vim
 echo "Done"`, false,
			[]string{"/bin/sh", "-c", ` echo "Starting"
 apt update
 apt install -y vim
 echo "Done"`}),

		Entry("Removes empty lines and comments (K8s)",
			`#!/bin/bash
 # This is a comment
 apt update

 # Another comment
 apt install -y nano`, false,
			[]string{"/bin/bash", "-c", ` # This is a comment
 apt update

 # Another comment
 apt install -y nano`}),

		Entry("Handles single and double quotes correctly (K8s)",
			`sleep 1
 echo "Hello 'World'"`, false,
			[]string{"/bin/sh", "-c", `sleep 1
 echo "Hello 'World'"`}),

		Entry("Preserves environment variables (K8s)",
			`#!/bin/bash
 export VAR1="Hello"
 echo $VAR1`, false,
			[]string{"/bin/bash", "-c", ` export VAR1="Hello"
 echo $VAR1`}),
	)
})
