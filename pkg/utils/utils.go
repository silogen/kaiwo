package utils

import (
	"fmt"
	"os/user"
	"path/filepath"
	"strings"
)

const KaiwoconfigFilename = "kaiwoconfig"
const EnvFilename = "env"

func SanitizeStringForKubernetes(path string) string {
	replacer := strings.NewReplacer(
		":", "-",
		"/", "-",
		"_", "-",
		".", "-",
	)
	return strings.ToLower(replacer.Replace(path))
}

func BuildWorkloadName(defaultName string, path string, image string) string {
	if defaultName == "" {
		currentUser, err := user.Current()
		if err != nil {
			panic(fmt.Sprintf("Failed to fetch the current user: %v", err))
		}

		var appendix string

		if path != "" {
			appendix = SanitizeStringForKubernetes(filepath.Base(path))
		} else {
			appendix = SanitizeStringForKubernetes(image)
		}
		return strings.Join([]string{currentUser.Username, appendix}, "-")
	}
	return defaultName
}
