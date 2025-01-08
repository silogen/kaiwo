package ray

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
)

//go:embed job.yaml.tmpl
var RayJobTemplate []byte

const ENTRYPOINT_FILENAME = "entrypoint"

type RayJobLoader struct {
	Entrypoint string
}

func (r *RayJobLoader) Load(path string) error {

	logrus.Infof("Loading Ray job from %s", path)

	// Read entrypoint file
	contents, err := os.ReadFile(filepath.Join(path, ENTRYPOINT_FILENAME))

	if err != nil {
		return fmt.Errorf("failed to read entrypoint file: %w", err)
	}

	r.Entrypoint = string(contents)

	return nil
}

func (r *RayJobLoader) DefaultTemplate() []byte {
	return RayJobTemplate
}

func (r *RayJobLoader) IgnoreFiles() []string {
	return []string{ENTRYPOINT_FILENAME}
}
