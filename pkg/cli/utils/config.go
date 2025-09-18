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

package cliutils

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type KaiwoCliConfig struct {
	User string `yaml:"user"`
}

const (
	KaiwoCliConfigPathEnv = "KAIWOCONFIG"
)

func GetDefaultKaiwoCliConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config/kaiwo/kaiwoconfig.yaml"), nil
}

func GetKaiwoCliConfigPath(cliFlagsPath string) (string, error) {
	configPath := ""
	if cliFlagsPath != "" {
		configPath = cliFlagsPath
	} else if envPath := os.Getenv(KaiwoCliConfigPathEnv); envPath != "" {
		configPath = envPath
	} else if defaultFileExists, err := DefaultKaiwoCliConfigFileExists(); err == nil && defaultFileExists {
		configPath, err = GetDefaultKaiwoCliConfigPath()
		if err != nil {
			return "", err
		}
	} else if err != nil {
		return "", fmt.Errorf("failed to determine default Kaiwo cli config file path: %w", err)
	}
	return configPath, nil
}

func DefaultKaiwoCliConfigFileExists() (bool, error) {
	configPath, err := GetDefaultKaiwoCliConfigPath()
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(configPath); err == nil {
		return true, nil
	} else if errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else {
		return false, fmt.Errorf("unable to stat kaiwo cli config file: %w", err)
	}
}
