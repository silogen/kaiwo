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

package common

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	configapi "github.com/silogen/kaiwo/apis/config/v1alpha1"

	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	configName = "kaiwo"
)

type cfgKey struct{}

// EnsureConfig ensures that a Kaiwo configuration exists within the cluster. It provides a way to either create
// one automatically, or to give some time for another system to create one while the controller is starting.
func EnsureConfig(ctx context.Context, k8sClient client.Client) error {
	logger := log.FromContext(ctx)
	config := &configapi.KaiwoConfig{}
	if err := k8sClient.Get(ctx, client.ObjectKey{Name: configName}, config); err == nil {
		logger.Info("Kaiwo Config exists", "configName", configName)
	} else {
		if _, err := createDefaultConfig(ctx, k8sClient); err != nil {
			return fmt.Errorf("could not create default Kaiwo Config: %w", err)
		} else {
			logger.Info("Default Kaiwo Config created", "configName", configName)
		}
	}
	return nil
}

// GetContextWithConfig fetches the latest config and attaches it to a given context
func GetContextWithConfig(ctx context.Context, k8sClient client.Client) (context.Context, error) {
	logger := log.FromContext(ctx)
	config := &configapi.KaiwoConfig{}
	if err := k8sClient.Get(ctx, client.ObjectKey{Name: configName}, config); err != nil {
		return ctx, fmt.Errorf("could not get Kaiwo Config with name '%s': %w", configName, err)
	} else if errors.IsNotFound(err) {
		logger.Info("Default Kaiwo Config not found, creating", "defaultKaiwoConfigName", configName)
		if defaultConfig, err := createDefaultConfig(ctx, k8sClient); err != nil {
			return ctx, fmt.Errorf("could not create default Kaiwo Config: %w", err)
		} else {
			config = defaultConfig
		}
	}
	return context.WithValue(ctx, cfgKey{}, KaiwoConfigContext{
		KaiwoConfigSpec: config.Spec,
	}), nil
}

func createDefaultConfig(ctx context.Context, k8sClient client.Client) (*configapi.KaiwoConfig, error) {
	config := &configapi.KaiwoConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: configName,
		},
	}
	if err := k8sClient.Create(ctx, config); err != nil {
		return nil, err
	}
	return config, nil
}

// ConfigFromContext extracts the cfg (or panics if missing).
func ConfigFromContext(ctx context.Context) KaiwoConfigContext {
	v := ctx.Value(cfgKey{})
	if v == nil {
		panic("config not found in context")
	}
	return v.(KaiwoConfigContext)
}

// KaiwoConfigContext holds the config that is relevant for a particular context
// In the future it can be extended with namespace-specific configuration as well
type KaiwoConfigContext struct {
	configapi.KaiwoConfigSpec
}
