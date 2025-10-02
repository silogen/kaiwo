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
