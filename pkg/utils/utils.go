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
	"regexp"
	"strings"
)

const (
	DefaultNamespace = "kaiwo"
	DefaultRayImage  = "ghcr.io/silogen/rocm-ray:v0.7"
)

func SanitizeStringForKubernetes(str string) string {
	replacer := strings.NewReplacer(
		":", "-",
		"/", "-",
		"\\", "-",
		"_", "-",
		".", "-",
	)
	str = strings.ToLower(replacer.Replace(str))
	str = MakeRFC1123Compliant(str)
	return str
}

func MakeRFC1123Compliant(input string) string {
	input = strings.ToLower(input)

	rfc1123Regex := regexp.MustCompile(`[^a-z0-9.-]+`)
	input = rfc1123Regex.ReplaceAllString(input, "-")

	input = strings.Trim(input, "-.")

	input = regexp.MustCompile(`[-.]{2,}`).ReplaceAllString(input, "-")

	if len(input) > 63 {
		input = input[:63]
	}

	return input
}
