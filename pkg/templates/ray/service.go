package ray

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
)

//go:embed service.yaml.tmpl
var RayServiceTemplate []byte

const SERVECONFIG_FILENAME = "serveconfig"

type RayServiceLoader struct {
	Serveconfig string
}

func (r *RayServiceLoader) Load(path string) error {
	logrus.Infof("Loading Ray service from %s", path)

	// Read the serveconfig file
	contents, err := os.ReadFile(filepath.Join(path, SERVECONFIG_FILENAME))

	if err != nil {
		return fmt.Errorf("failed to read serveconfig file: %w", err)
	}

	r.Serveconfig = string(contents)

	return nil
}

func (r *RayServiceLoader) DefaultTemplate() []byte {
	return RayServiceTemplate
}

func (r *RayServiceLoader) IgnoreFiles() []string {
	return []string{SERVECONFIG_FILENAME}
}
