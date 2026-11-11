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
// It is for non-SQL values; use RenderSQL for SQL text or expressions.
func Render(templateStr string, filters map[string]any) (out string, err error) {
	if !strings.Contains(templateStr, "{{") && !strings.Contains(templateStr, "{%") {
		return templateStr, nil
	}
	defer recoverTemplatePanic(&err)

	tpl, err := gonja.FromString(templateStr)
	if err != nil {
		return "", fmt.Errorf("parsing template: %w", err)
	}

	ctx := renderContext(filters)

	var buf bytes.Buffer
	if err := tpl.Execute(&buf, ctx); err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}

	return buf.String(), nil
}

// recoverTemplatePanic turns a gonja panic (for example on an empty
// {% raw %} block) into an error. Widget queries render on their own
// goroutines, where a panic would stop the whole server.
func recoverTemplatePanic(err *error) {
	if r := recover(); r != nil {
		*err = fmt.Errorf("rendering template: %v", r)
	}
}

func renderContext(filters map[string]any) *exec.Context {
	return exec.NewContext(map[string]interface{}{
		"filters": filters,
		"bruin": map[string]any{
			// In Bruin Cloud this is the signed-in viewer's email, injected
			// per request. Locally there's no viewer, so it comes from
			// BRUIN_USER_EMAIL (empty if unset) — set it to preview a
			// dashboard as a specific user.
			"user_email": os.Getenv("BRUIN_USER_EMAIL"),
		},
	})
}
