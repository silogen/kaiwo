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
	RunSpecs(t, "ConvertMultilineEntrypointToSingleLine Suite")
}

var _ = Describe("ConvertMultilineEntrypointToSingleLine", func() {
	It("should return single-line command when input is already one line", func() {
		input := `/bin/bash -c 'echo "Hello World"'`
		Expect(ConvertMultilineEntrypointToSingleLine(input)).To(Equal(input))
	})

	It("should remove line continuation backslashes", func() {
		input := `apt update && \
		apt install abc \
		xyz`
		expected := `/bin/bash -c 'apt update && apt install abc xyz'`
		Expect(ConvertMultilineEntrypointToSingleLine(input)).To(Equal(expected))
	})

	It("should replace multiple spaces and newlines with a single space", func() {
		input := `apt   update  
		apt   install   abc   xyz`
		expected := `/bin/bash -c 'apt update && apt install abc xyz'`
		Expect(ConvertMultilineEntrypointToSingleLine(input)).To(Equal(expected))
	})

	It("should detect and use shebang (e.g., #!/bin/sh)", func() {
		input := `#!/bin/sh
		echo "Starting"
		apt update
		apt install -y vim
		echo "Done"`
		expected := `/bin/sh -c 'echo "Starting" && apt update && apt install -y vim && echo "Done"'`
		Expect(ConvertMultilineEntrypointToSingleLine(input)).To(Equal(expected))
	})

	It("should remove empty lines and comments", func() {
		input := `#!/bin/bash
		# This is a comment
		apt update

		# Another comment
		apt install -y nano`
		expected := `/bin/bash -c 'apt update && apt install -y nano'`
		Expect(ConvertMultilineEntrypointToSingleLine(input)).To(Equal(expected))
	})

	It("should safely handle single quotes inside script", func() {
		input := `sleep 1
		echo 'Hello World'`
		expected := `/bin/bash -c 'sleep 1 && echo '"'"'Hello World'"'"''`
		Expect(ConvertMultilineEntrypointToSingleLine(input)).To(Equal(expected))
	})

	It("should handle a complex script with various cases", func() {
		input := `#!/bin/bash
		# Update system
		apt update && \
		apt install -y curl \
		git
		# Final message
		echo "Setup complete"`
		expected := `/bin/bash -c 'apt update && apt install -y curl git && echo "Setup complete"'`
		Expect(ConvertMultilineEntrypointToSingleLine(input)).To(Equal(expected))
	})
})
