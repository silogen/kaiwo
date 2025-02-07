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
	"os/user"
	"regexp"
	"strings"

	"github.com/sirupsen/logrus"
)

func GetCurrentUser() (string, error) {
	userEmail := os.Getenv("KAIWO_USER_EMAIL")

	if userEmail != "" {
		emailRegex := `^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`
		matched, err := regexp.MatchString(emailRegex, userEmail)
		if err != nil {
			return "", fmt.Errorf("failed to validate KAIWO_USER_EMAIL: %w", err)
		}
		if !matched {
			return "", fmt.Errorf("invalid email format: %s", userEmail)
		}

		parts := strings.Split(userEmail, "@")
		username := strings.Split(parts[0], "-")[0]
		domain := strings.ReplaceAll(parts[1], ".", "-")
		return MakeRFC1123Compliant(fmt.Sprintf("%s-%s", username, domain)), nil
	}

	logrus.Warn("KAIWO_USER_EMAIL not set. Falling back to UNIX username and hostname")
	currentUser, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("failed to retrieve current user: %w", err)
	}
	hostname, err := os.Hostname()
	if err != nil {
		return "", fmt.Errorf("failed to retrieve hostname: %w", err)
	}

	k8sCompatibleHostname := strings.ReplaceAll(hostname, ".", "-")

	return MakeRFC1123Compliant(fmt.Sprintf("%s-%s", currentUser.Username, k8sCompatibleHostname)), nil
}
