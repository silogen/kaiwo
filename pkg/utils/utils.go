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
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	DebugLogLevel = 1
	TraceLogLevel = 2
)

func Debug(logger logr.Logger, fmt string, keysAndValues ...any) {
	logger.V(DebugLogLevel).Info(fmt, keysAndValues...)
}

func Trace(logger logr.Logger, fmt string, keysAndValues ...any) {
	logger.V(TraceLogLevel).Info(fmt, keysAndValues...)
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

	input = strings.TrimRight(input, "-")

	return input
}

func FormatNameWithPostfix(name string, postfix string) string {
	return MakeRFC1123Compliant(fmt.Sprintf("%s-%s", name, postfix))
}

func GetEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

func Pointer[T any](d T) *T {
	return &d
}

func ValueOrDefault[T any](d *T) T {
	if d == nil {
		return *new(T)
	}
	return *d
}

// LogErrorf takes care of logging the error message with logr, as well as creating the error object to return
func LogErrorf(logger logr.Logger, message string, err error) error {
	logger.Error(err, message)
	return fmt.Errorf("%s: %w", message, err)
}

func GetGVK(scheme runtime.Scheme, object client.Object) (schema.GroupVersionKind, error) {
	gvks, _, err := scheme.ObjectKinds(object)
	if err != nil {
		return schema.GroupVersionKind{}, fmt.Errorf("failed to determine GVK for %T: %w", object, err)
	}
	if len(gvks) == 0 {
		return schema.GroupVersionKind{}, fmt.Errorf("no GVK found for object type %T", object)
	}
	return gvks[0], nil
}

func ContainsString(slice []string, str string) bool {
	for _, v := range slice {
		if v == str {
			return true
		}
	}
	return false
}

func RemoveString(slice []string, str string) []string {
	var newSlice []string
	for _, v := range slice {
		if v != str {
			newSlice = append(newSlice, v)
		}
	}
	return newSlice
}

func ConvertMultilineEntrypoint(entrypoint string, isRayJob bool) interface{} {
	entrypoint = strings.TrimSpace(entrypoint)

	// Handle line continuation backslashes
	entrypoint = regexp.MustCompile(`\\\s*\n`).ReplaceAllString(entrypoint, " ")

	// If RayJob, return raw command (Ray handles it as-is)
	if isRayJob {
		return entrypoint
	}

	// Detect shebang
	shell := "/bin/sh"
	lines := strings.Split(entrypoint, "\n")
	if len(lines) > 0 && strings.HasPrefix(lines[0], "#!") {
		shell = strings.TrimSpace(lines[0][2:])
		entrypoint = strings.Join(lines[1:], "\n") // remove shebang line
	}

	return []string{shell, "-c", entrypoint}
}
