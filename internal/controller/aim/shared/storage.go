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

package shared

import (
	"k8s.io/apimachinery/pkg/api/resource"

	aimv1alpha1 "github.com/silogen/kaiwo/apis/aim/v1alpha1"
)

// ApplyHeadroomAndRound applies headroom percentage to a base size and rounds up to the nearest Gi.
// This ensures PVC sizes are clean, human-readable values (e.g., "421Gi" instead of "451936812032").
//
// Parameters:
//   - baseSizeBytes: The original size in bytes
//   - headroomPercent: Percentage of extra space to add (0-100, e.g., 10 means 10% extra)
//
// Returns:
//   - The final size in bytes, rounded up to the nearest Gi boundary
//
// Example:
//   - Input: 9,094,593,249 bytes with 10% headroom
//   - With headroom: 10,004,052,573 bytes (9.31 Gi)
//   - Rounded: 10,737,418,240 bytes (10 Gi)
func ApplyHeadroomAndRound(baseSizeBytes int64, headroomPercent int32) int64 {
	// Apply headroom: convert percentage to multiplier (e.g., 10% -> 1.10)
	headroomMultiplier := 1.0 + (float64(headroomPercent) / 100.0)
	sizeWithHeadroom := int64(float64(baseSizeBytes) * headroomMultiplier)

	// Round up to nearest Gi for cleaner display
	// Uses ceiling division: (x + n - 1) / n gives ceiling(x/n)
	const giInBytes int64 = 1024 * 1024 * 1024
	return ((sizeWithHeadroom + giInBytes - 1) / giInBytes) * giInBytes
}

// QuantityWithHeadroom creates a resource.Quantity with headroom applied and rounded to the nearest Gi.
// This is a convenience wrapper around ApplyHeadroomAndRound that returns a Kubernetes Quantity.
//
// The returned Quantity uses BinarySI format (Ki, Mi, Gi, Ti suffixes) for compatibility
// with Kubernetes storage resources.
//
// Parameters:
//   - baseSizeBytes: The original size in bytes
//   - headroomPercent: Percentage of extra space to add (0-100)
//
// Returns:
//   - A resource.Quantity representing the size with headroom, formatted cleanly
func QuantityWithHeadroom(baseSizeBytes int64, headroomPercent int32) resource.Quantity {
	roundedSize := ApplyHeadroomAndRound(baseSizeBytes, headroomPercent)
	return *resource.NewQuantity(roundedSize, resource.BinarySI)
}

// ResolveStorageClass determines the effective storage class using fallback logic:
//  1. Use explicit storage class if provided (non-empty)
//  2. Fall back to runtime config's defaultStorageClassName if explicit is empty
//  3. Empty string means use the cluster's default StorageClass
//
// This implements consistent storage class resolution across all PVC creation paths.
//
// Parameters:
//   - explicitStorageClass: Storage class explicitly specified in the resource spec
//   - runtimeConfigSpec: The resolved runtime configuration spec
//
// Returns:
//   - The effective storage class name (may be empty to use cluster default)
func ResolveStorageClass(explicitStorageClass string, runtimeConfigSpec aimv1alpha1.AIMRuntimeConfigSpec) string {
	if explicitStorageClass != "" {
		return explicitStorageClass
	}
	return runtimeConfigSpec.DefaultStorageClassName
}
