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

package config

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

func SetContextConfig(ctx context.Context, config *configapi.KaiwoConfig) context.Context {
	return context.WithValue(ctx, cfgKey{}, KaiwoConfigContext{KaiwoConfigSpec: config.Spec})
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
