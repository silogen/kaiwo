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

package controllerutils

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/silogen/kaiwo/pkg/api/v1alpha1"
	baseutils "github.com/silogen/kaiwo/pkg/utils"
)

var configName = baseutils.GetEnv("CONFIG_NAME", "kaiwo")

type cfgKey struct{}

// GetContextWithConfig fetches the latest config and attaches it to a given context
func GetContextWithConfig(ctx context.Context, k8sClient client.Client) (context.Context, error) {
	logger := log.FromContext(ctx)
	config := &v1alpha1.KaiwoConfig{}
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

func createDefaultConfig(ctx context.Context, k8sClient client.Client) (*v1alpha1.KaiwoConfig, error) {
	config := &v1alpha1.KaiwoConfig{}
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
	v1alpha1.KaiwoConfigSpec
}
