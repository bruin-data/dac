package template

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/nikolalohinski/gonja/v2"
	"github.com/nikolalohinski/gonja/v2/exec"
	"github.com/nikolalohinski/gonja/v2/loaders"
	"github.com/nikolalohinski/gonja/v2/tokens"
)

// ValidateSQLValue rejects characters that cannot be interpolated portably
// across the SQL dialects supported by the CLI backend. That backend has no
// bind-parameter API; guessing a dialect's escaping rules is unsafe.
func ValidateSQLValue(value any) error {
	switch v := value.(type) {
	case string:
		if strings.ContainsAny(v, "'\\") || strings.ContainsFunc(v, unicode.IsControl) {
			return fmt.Errorf("SQL filter values cannot contain quotes, backslashes, or control characters")
		}
	case []string:
		for _, item := range v {
			if err := ValidateSQLValue(item); err != nil {
				return err
			}
		}
	case []any:
		for _, item := range v {
			if err := ValidateSQLValue(item); err != nil {
				return err
			}
		}
	case map[string]any:
		for _, item := range v {
			if err := ValidateSQLValue(item); err != nil {
				return err
			}
		}
	}
	return nil
}

var sqlNumber = regexp.MustCompile(`^-?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?$`)
var dollarQuote = regexp.MustCompile(`^\$(?:[A-Za-z_][A-Za-z_0-9]*)?\$`)

type sqlOutput struct {
	text   string
	number bool
	list   bool
}

// RenderSQL evaluates Jinja normally (including conditions and loops), but
// captures every output before it can enter SQL. Outputs are substituted only
// inside ordinary single-quoted literals, or as numbers outside literals.
// Never use Render for SQL fragments containing request-controlled values.
func RenderSQL(source string, filters map[string]any) (string, error) {
	if !strings.Contains(source, "{{") && !strings.Contains(source, "{%") {
		return source, nil
	}
	if err := ValidateSQLValue(filters); err != nil {
		return "", err
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	prefix := fmt.Sprintf("DACVALUE%x_", nonce)
	captureName := prefix + "capture"
	listName := prefix + "list"
	rewritten, err := captureSQLExpressions(source, captureName, listName)
	if err != nil {
		return "", err
	}
	var values []sqlOutput
	capture := func(v *exec.Value) string {
		values = append(values, sqlOutput{text: v.String(), number: v.IsNumber()})
		return fmt.Sprintf("%s%dEND", prefix, len(values)-1)
	}
	captureList := func(v *exec.Value) (string, error) {
		var items []string
		switch list := v.Interface().(type) {
		case []string:
			items = list
		case []any:
			for _, item := range list {
				text, ok := item.(string)
				if !ok {
					return "", fmt.Errorf("SQL selections must be strings")
				}
				items = append(items, text)
			}
		default:
			return "", fmt.Errorf("SQL selections must be a list")
		}
		if err := ValidateSQLValue(items); err != nil {
			return "", err
		}
		values = append(values, sqlOutput{text: strings.Join(items, "','"), list: true})
		return fmt.Sprintf("%s%dEND", prefix, len(values)-1), nil
	}
	// Included/imported filesystem templates would bypass output capture.
	// Resolve SQL files at the dashboard layer and render only this source.
	root := "/" + prefix
	loader := loaders.MustNewMemoryLoader(map[string]string{root: rewritten})
	tpl, err := exec.NewTemplate(root, gonja.DefaultConfig, loader, gonja.DefaultEnvironment)
	if err != nil {
		return "", fmt.Errorf("parsing SQL template: %w", err)
	}
	ctx := renderContext(filters)
	ctx.Set(captureName, capture)
	ctx.Set(listName, captureList)
	var out bytes.Buffer
	if err := tpl.Execute(&out, ctx); err != nil {
		return "", fmt.Errorf("rendering SQL template: %w", err)
	}
	return substituteSQLValues(out.String(), prefix, values)
}

// Use the Jinja lexer, not a regexp: delimiters inside strings and comments
// must not be mistaken for output boundaries. Preserve whitespace control.
func captureSQLExpressions(source, capture, list string) (string, error) {
	stream := tokens.Lex(source, gonja.DefaultConfig)
	var out strings.Builder
	last, start := 0, -1
	var expression []*tokens.Token
	for !stream.End() {
		token := stream.Current()
		switch token.Type {
		case tokens.BlockBegin:
			// {% filter %} transforms rendered text, placeholders included,
			// so its output would reach SQL without being captured.
			if next := stream.Peek(); next != nil && next.Type == tokens.Name && next.Val == "filter" {
				return "", fmt.Errorf("{%% filter %%} blocks are not supported in SQL templates")
			}
		case tokens.VariableBegin:
			start = token.Pos + len(token.Val)
			out.WriteString(source[last:start])
			expression = nil
		case tokens.VariableEnd:
			if start < 0 {
				return "", fmt.Errorf("invalid SQL template output")
			}
			end := token.Pos
			function := capture
			// Preserve the documented IN ('{{ list | join("','") }}') idiom,
			// validating each element separately from the trusted separator.
			n := len(expression)
			if n >= 5 && expression[n-5].Type == tokens.Pipe && expression[n-4].Val == "join" &&
				expression[n-3].Type == tokens.LeftParenthesis && expression[n-2].Type == tokens.String &&
				expression[n-2].Val == "','" && expression[n-1].Type == tokens.RightParenthesis {
				function = list
				end = expression[n-5].Pos
			}
			out.WriteString(function + "((" + source[start:end] + "))")
			last = token.Pos
			start = -1
		default:
			if start >= 0 {
				expression = append(expression, token)
			}
		}
		stream.Next()
	}
	if stream.IsError() {
		return "", fmt.Errorf("invalid SQL template")
	}
	if start >= 0 {
		return "", fmt.Errorf("unclosed SQL template output")
	}
	out.WriteString(source[last:])
	return out.String(), nil
}

func substituteSQLValues(sql, prefix string, values []sqlOutput) (string, error) {
	var out strings.Builder
	// A deliberately conservative lexer: only ordinary single-quoted strings
	// support text interpolation. Identifiers, comments, dollar/triple quoted
	// strings and dialect-specific escaped literals cannot contain outputs.
	state, delimiter := "", ""
	depth := 0
	// Past the last output nothing more is substituted, so dialect syntax the
	// lexer cannot follow (backslash escapes, unknown dollar tags) is harmless.
	last := strings.LastIndex(sql, prefix)
	// '#>' and non-identifier '[' read differently across dialects (MySQL
	// comment vs Postgres operator, T-SQL identifier vs array literal). Both
	// readings agree again at the end of the construct only if this lexer is
	// outside any literal there; otherwise later outputs cannot be placed.
	var syncs []int
	ambiguous := false
	for i := 0; i < len(sql); {
		if i > last {
			out.WriteString(sql[i:])
			break
		}
		syncs = slices.DeleteFunc(syncs, func(at int) bool {
			if i < at {
				return false
			}
			ambiguous = ambiguous || state != ""
			return true
		})
		if strings.HasPrefix(sql[i:], prefix) {
			if ambiguous {
				return "", fmt.Errorf("SQL template outputs cannot follow quotes inside '#>' or '[...]' syntax that other dialects read differently")
			}
			end := strings.Index(sql[i:], "END")
			if end < 0 {
				return "", fmt.Errorf("invalid SQL placeholder")
			}
			index, err := strconv.Atoi(sql[i+len(prefix) : i+end])
			if err != nil || index < 0 || index >= len(values) {
				return "", fmt.Errorf("invalid SQL placeholder")
			}
			value := values[index]
			if state == "'" {
				if !value.list {
					if err := ValidateSQLValue(value.text); err != nil {
						return "", err
					}
				}
			} else if state != "" || value.list || !value.number || !sqlNumber.MatchString(value.text) {
				return "", fmt.Errorf("SQL template outputs must be numbers or appear inside single-quoted string literals; dynamic SQL identifiers and fragments are not supported")
			}
			if state == "" {
				out.WriteByte(' ')
			}
			out.WriteString(value.text)
			if state == "" {
				out.WriteByte(' ')
			}
			i += end + 3
			continue
		}
		c := sql[i]
		switch state {
		case "line":
			if c == '\n' || c == '\r' {
				state = ""
			}
		case "block":
			if strings.HasPrefix(sql[i:], "/*") {
				depth++
				out.WriteString("/*")
				i += 2
				continue
			}
			if strings.HasPrefix(sql[i:], "*/") {
				depth--
				if depth == 0 {
					state = ""
				}
				out.WriteString("*/")
				i += 2
				continue
			}
		case "dollar", "triple":
			if strings.HasPrefix(sql[i:], delimiter) {
				state = ""
				out.WriteString(delimiter)
				i += len(delimiter)
				continue
			}
		case "'", "\"", "`", "]":
			if c == '\\' {
				// Backslash escaping varies by dialect. Fail closed instead of guessing
				// which subsequent quote closes a literal.
				return "", fmt.Errorf("backslash-escaped SQL literals cannot be combined with template outputs")
			}
			if c == state[0] {
				if i+1 < len(sql) && sql[i+1] == c {
					out.WriteByte(c)
					i++
				} else {
					state = ""
				}
			}
		default:
			switch {
			case c == '#' && strings.HasPrefix(sql[i+1:], ">"):
				// Postgres JSON path operator ('#>', '#>>'), or a MySQL comment.
				syncs = append(syncs, lineEnd(sql, i)+1)
			case strings.HasPrefix(sql[i:], "--") || c == '#':
				// '#' starts a MySQL/BigQuery comment.
				state = "line"
			case strings.HasPrefix(sql[i:], "/*"):
				state = "block"
				depth = 1
				out.WriteString("/*")
				i += 2
				continue
			case strings.HasPrefix(sql[i:], "'''") || strings.HasPrefix(sql[i:], `"""`):
				state = "triple"
				delimiter = sql[i : i+3]
				out.WriteString(delimiter)
				i += 3
				continue
			case c == '$':
				if match := dollarQuote.FindString(sql[i:]); match != "" {
					state = "dollar"
					delimiter = match
					out.WriteString(match)
					i += len(match)
					continue
				}
				// Dollar tags can use dialect-specific identifier characters
				// (including Unicode). Do not mistake quotes inside an unknown
				// dollar literal for ordinary string delimiters.
				return "", fmt.Errorf("unsupported dollar syntax in SQL template")
			case c == '\'' || c == '"' || c == '`':
				state = string(c)
			case c == '[' && bracketIdentifier(sql[i+1:], prefix):
				state = "]"
			case c == '[':
				syncs = append(syncs, bracketEnd(sql, i+1))
			}
		}
		out.WriteByte(c)
		i++
	}
	// A `{% set %}` block can feed one output's placeholder into another, which
	// is then emitted verbatim instead of substituted. Fail rather than run it.
	if strings.Contains(strings.ToLower(out.String()), strings.ToLower(prefix)) {
		return "", fmt.Errorf("SQL template outputs cannot be nested or transformed by blocks")
	}
	return out.String(), nil
}

func lineEnd(sql string, i int) int {
	if end := strings.IndexAny(sql[i:], "\r\n"); end >= 0 {
		return i + end
	}
	return len(sql)
}

// bracketEnd returns the index just past the ']' that would close a T-SQL
// [identifier] opened before start, where ']]' is an escaped ']'.
func bracketEnd(sql string, start int) int {
	for i := start; i < len(sql); i++ {
		if sql[i] != ']' {
			continue
		}
		if i+1 < len(sql) && sql[i+1] == ']' {
			i++
			continue
		}
		return i + 1
	}
	return len(sql)
}

// bracketIdentifier reports whether '[' opens a SQL Server style [identifier]
// rather than an array literal or subscript such as ARRAY['a'] or [1, 2].
func bracketIdentifier(rest, prefix string) bool {
	rest = strings.TrimLeft(rest, " \t\r\n")
	if rest == "" || strings.HasPrefix(rest, prefix) {
		return false
	}
	c := rest[0]
	return c == '_' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= 0x80
}
