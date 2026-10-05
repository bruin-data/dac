package dashboard

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	sem "github.com/bruin-data/bruin/semantic-engine"
	semschemas "github.com/bruin-data/bruin/semantic-engine/schemas"
	"gopkg.in/yaml.v3"

	"github.com/bruin-data/dac/schemas"
)

// ValidateDefinition checks YAML/JSON and all supplied models.
func ValidateDefinition(data []byte, rawModels map[string][]byte) error {
	d, err := LoadDefinition(data, nil)
	if err != nil {
		return err
	}

	models := make(map[string]*sem.Model, len(rawModels))
	var problems []string
	for _, name := range slices.Sorted(maps.Keys(rawModels)) {
		raw := rawModels[name]
		if err := semschemas.ValidateYAML(semschemas.SemanticModelV1ID, raw); err != nil {
			problems = append(problems, fmt.Sprintf("semantic model %q: %s", name, strings.Join(schemas.FormatErrors(err.Error(), raw), "; ")))
			continue
		}
		var model sem.Model
		if err := yaml.Unmarshal(raw, &model); err != nil {
			problems = append(problems, fmt.Sprintf("semantic model %q: %v", name, err))
			continue
		}
		if model.Name != name {
			problems = append(problems, fmt.Sprintf("semantic model %q: name must match its key, got %q", name, model.Name))
			continue
		}
		if model.Schema == "" {
			model.Schema = semschemas.SemanticModelV1ID
		}
		if _, err := sem.NewEngine(&model); err != nil {
			problems = append(problems, fmt.Sprintf("semantic model %q: %v", name, err))
			continue
		}
		models[name] = &model
	}
	if len(problems) > 0 {
		return &ValidationError{Dashboard: d.Name, Errors: problems}
	}

	d.SetProjectContext("", models, nil)
	return Validate(d)
}
