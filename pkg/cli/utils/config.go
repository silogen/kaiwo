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

package cliutils

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type KaiwoCliConfig struct {
	User         string `yaml:"user"`
	ClusterQueue string `yaml:"clusterQueue"`
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
