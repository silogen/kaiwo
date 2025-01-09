package jobs

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

//go:embed job.yaml.tmpl
var JobTemplate []byte

const ENTRYPOINT_FILENAME = "entrypoint"

type JobLoader struct {
	Entrypoint string
}

func (r *JobLoader) Load(path string) error {

	contents, err := os.ReadFile(filepath.Join(path, ENTRYPOINT_FILENAME))

	if err != nil {
		return fmt.Errorf("failed to read entrypoint file: %w", err)
	}

	r.Entrypoint = strings.ReplaceAll(string(contents), "\n", " ") // Flatten multiline string
	r.Entrypoint = strings.ReplaceAll(r.Entrypoint, "\"", "\\\"") // Escape double quotes
	r.Entrypoint = fmt.Sprintf("\"%s\"", r.Entrypoint) // Wrap the entire command in quotes



	return nil
}

func (r *JobLoader) DefaultTemplate() []byte {
	return JobTemplate
}

func (r *JobLoader) IgnoreFiles() []string {
	return []string{ENTRYPOINT_FILENAME}
}
