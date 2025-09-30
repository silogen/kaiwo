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

func FormatNameWithPostfix(name string, postfix ...string) string {
	builtName := name
	for _, postfix := range postfix {
		builtName += fmt.Sprintf("-%s", postfix)
	}
	return MakeRFC1123Compliant(builtName)
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
