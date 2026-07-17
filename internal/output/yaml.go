package output

import (
	"io"

	"sigs.k8s.io/yaml"
)

// writeYAML marshals v to YAML via its JSON tags (so YAML keys match JSON).
func writeYAML(w io.Writer, v any) error {
	b, err := yaml.Marshal(v)
	if err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}
