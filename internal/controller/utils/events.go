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

package controllerutils

import (
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Event type constants
const (
	EventTypeNormal  = "Normal"
	EventTypeWarning = "Warning"
)

// EmitEvent emits a Kubernetes event if a recorder is available
func EmitEvent(recorder record.EventRecorder, obj client.Object, eventType, reason, message string) {
	if recorder != nil {
		recorder.Event(obj, eventType, reason, message)
	}
}

// EmitNormalEvent emits a Normal type event
func EmitNormalEvent(recorder record.EventRecorder, obj client.Object, reason, message string) {
	EmitEvent(recorder, obj, EventTypeNormal, reason, message)
}

// EmitWarningEvent emits a Warning type event
func EmitWarningEvent(recorder record.EventRecorder, obj client.Object, reason, message string) {
	EmitEvent(recorder, obj, EventTypeWarning, reason, message)
}
