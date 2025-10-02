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
