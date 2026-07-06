package template

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/nikolalohinski/gonja/v2"
	"github.com/nikolalohinski/gonja/v2/exec"
)

// Render processes a Jinja template string with the given filter values.
func Render(templateStr string, filters map[string]any) (string, error) {
	if !strings.Contains(templateStr, "{{") && !strings.Contains(templateStr, "{%") {
		return templateStr, nil
	}

	tpl, err := gonja.FromString(templateStr)
	if err != nil {
		return "", fmt.Errorf("parsing template: %w", err)
	}

	ctx := exec.NewContext(map[string]interface{}{
		"filters": filters,
		"bruin": map[string]any{
			// In Bruin Cloud this is the signed-in viewer's email, injected
			// per request. Locally there's no viewer, so it comes from
			// BRUIN_USER_EMAIL (empty if unset) — set it to preview a
			// dashboard as a specific user.
			"user_email": os.Getenv("BRUIN_USER_EMAIL"),
		},
	})

	var buf bytes.Buffer
	if err := tpl.Execute(&buf, ctx); err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}

	return buf.String(), nil
}
