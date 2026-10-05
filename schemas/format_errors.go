package schemas

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	dacSchemaProblemLineRe = regexp.MustCompile(`(?m)^\s*- at '([^']*)':\s*(.*)$`)
	dacKeywordNoiseRe      = regexp.MustCompile(`^'(?:allOf|anyOf|oneOf|not)' failed$`)
	dacWidgetPathRe        = regexp.MustCompile(`^/rows/\d+/widgets/\d+$`)
)

// FormatErrors formats schema errors, using names from an optional YAML/JSON definition.
func FormatErrors(raw string, definition ...[]byte) []string {
	matches := dacSchemaProblemLineRe.FindAllStringSubmatch(raw, -1)
	if len(matches) == 0 {
		return []string{raw}
	}
	var value any
	if len(definition) > 0 {
		_ = yaml.Unmarshal(definition[0], &value)
	}
	problems := make([]string, 0, len(matches))
	seen := map[string]bool{}
	add := func(path, msg string) {
		s := msg
		if path != "" {
			s = path + ": " + msg
		}
		if !seen[s] {
			seen[s] = true
			problems = append(problems, s)
		}
	}
	for _, m := range matches {
		ptr, msg := m[1], strings.TrimSpace(m[2])
		if msg == "" || strings.EqualFold(msg, "validation failed") {
			continue
		}
		if dacKeywordNoiseRe.MatchString(msg) {
			// Keep childless `not` errors; skip other wrappers.
			if msg == "'not' failed" {
				if dacWidgetPathRe.MatchString(ptr) {
					add(prettyDACPath(ptr, value), "pivot is only valid on pivot_table widgets")
				} else {
					add(prettyDACPath(ptr, value), "value is not allowed here")
				}
			}
			continue
		}
		add(prettyDACPath(ptr, value), msg)
	}
	if len(problems) == 0 {
		return []string{raw}
	}
	return problems
}

// prettyDACPath uses unique item names, falling back to array indices.
func prettyDACPath(pointer string, value any) string {
	var b strings.Builder
	for _, seg := range strings.Split(strings.Trim(pointer, "/"), "/") {
		if seg == "" {
			continue
		}
		seg = strings.ReplaceAll(strings.ReplaceAll(seg, "~1", "/"), "~0", "~")
		var siblings []any
		switch node := value.(type) {
		case map[string]any:
			value = node[seg]
		case []any:
			siblings = node
			value = nil
			if i, err := strconv.Atoi(seg); err == nil && i >= 0 && i < len(node) {
				value = node[i]
			}
		default:
			value = nil
		}
		if _, err := strconv.Atoi(seg); err == nil {
			if item, ok := value.(map[string]any); ok {
				if name, ok := item["name"].(string); ok && strings.TrimSpace(name) != "" {
					count := 0
					for _, sibling := range siblings {
						if other, ok := sibling.(map[string]any); ok && other["name"] == name {
							count++
						}
					}
					if count == 1 {
						b.WriteString(fmt.Sprintf("[%q]", name))
						continue
					}
				}
			}
			b.WriteString("[" + seg + "]")
			continue
		}
		if b.Len() > 0 {
			b.WriteString(" › ")
		}
		b.WriteString(seg)
	}
	return b.String()
}
