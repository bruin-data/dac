package dashboard

import (
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestResolveDateExpression_Today(t *testing.T) {
	got := ResolveDateExpression("TODAY")
	want := time.Now().Format("2006-01-02")
	if got != want {
		t.Errorf("TODAY: got %q, want %q", got, want)
	}
}

func TestResolveDateExpression_TodayMinusN(t *testing.T) {
	got := ResolveDateExpression("TODAY-30")
	want := time.Now().AddDate(0, 0, -30).Format("2006-01-02")
	if got != want {
		t.Errorf("TODAY-30: got %q, want %q", got, want)
	}
}

func TestResolveDateExpression_TodayPlusN(t *testing.T) {
	got := ResolveDateExpression("TODAY+7")
	want := time.Now().AddDate(0, 0, 7).Format("2006-01-02")
	if got != want {
		t.Errorf("TODAY+7: got %q, want %q", got, want)
	}
}

func TestResolveDateExpression_Literal(t *testing.T) {
	got := ResolveDateExpression("2024-01-15")
	if got != "2024-01-15" {
		t.Errorf("literal: got %q, want %q", got, "2024-01-15")
	}
}

func TestResolveDateExpression_Invalid(t *testing.T) {
	cases := []string{
		"today",
		"TODAY - 30",
		"TOMORROW",
		"YESTERDAY",
		"2024/01/15",
		"",
		"TODAY+",
		"TODAY-abc",
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			if got := ResolveDateExpression(c); got != "" {
				t.Errorf("expected empty for invalid input %q, got %q", c, got)
			}
		})
	}
}

func TestDefaultFilters_DateExpressionResolved(t *testing.T) {
	d := &Dashboard{
		Filters: []Filter{
			{Name: "as_of", Type: "date", Default: "TODAY"},
		},
	}
	got := d.DefaultFilters()
	want := time.Now().Format("2006-01-02")
	if got["as_of"] != want {
		t.Errorf("expected TODAY to resolve to %q, got %v", want, got["as_of"])
	}
}

func TestDefaultFilters_DateExpressionInvalidPassesThrough(t *testing.T) {
	d := &Dashboard{
		Filters: []Filter{
			{Name: "as_of", Type: "date", Default: "today"},
		},
	}
	got := d.DefaultFilters()
	if got["as_of"] != "today" {
		t.Errorf("expected pass-through %q, got %v", "today", got["as_of"])
	}
}

func TestValidate_NumberFilterAccepted(t *testing.T) {
	d := &Dashboard{
		Name: "t",
		Rows: []Row{{Widgets: []Widget{{Name: "w", Type: WidgetTypeText, Content: "hi"}}}},
		Filters: []Filter{
			{Name: "threshold", Type: "number", Default: 100},
		},
	}
	if err := Validate(d); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestValidate_DateFilterAccepted(t *testing.T) {
	d := &Dashboard{
		Name: "t",
		Rows: []Row{{Widgets: []Widget{{Name: "w", Type: WidgetTypeText, Content: "hi"}}}},
		Filters: []Filter{
			{Name: "as_of", Type: "date", Default: "TODAY-30"},
		},
	}
	if err := Validate(d); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestValidate_DateFilterRejectsBadExpression(t *testing.T) {
	cases := []string{"today", "TOMORROW", "TODAY - 30", "2024/01/15"}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			d := &Dashboard{
				Name: "t",
				Rows: []Row{{Widgets: []Widget{{Name: "w", Type: WidgetTypeText, Content: "hi"}}}},
				Filters: []Filter{
					{Name: "as_of", Type: "date", Default: c},
				},
			}
			err := Validate(d)
			if err == nil || !strings.Contains(err.Error(), "invalid date default") {
				t.Fatalf("expected invalid-date-default error for %q, got: %v", c, err)
			}
		})
	}
}

func TestValidate_UnknownFilterTypeRejected(t *testing.T) {
	d := &Dashboard{
		Name: "t",
		Rows: []Row{{Widgets: []Widget{{Name: "w", Type: WidgetTypeText, Content: "hi"}}}},
		Filters: []Filter{
			{Name: "x", Type: "boolean"},
		},
	}
	err := Validate(d)
	if err == nil || !strings.Contains(err.Error(), "unknown filter type") {
		t.Fatalf("expected unknown-filter-type error, got: %v", err)
	}
}

func TestValidate_DateRangeStillWorks(t *testing.T) {
	d := &Dashboard{
		Name: "t",
		Rows: []Row{{Widgets: []Widget{{Name: "w", Type: WidgetTypeText, Content: "hi"}}}},
		Filters: []Filter{
			{Name: "period", Type: "date-range", Default: "last_30_days"},
		},
	}
	if err := Validate(d); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestResolveDateExpression_RejectsImpossibleDate(t *testing.T) {
	cases := []string{"9999-99-99", "2024-13-01", "2024-02-30", "0000-00-00"}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			if got := ResolveDateExpression(c); got != "" {
				t.Errorf("expected empty for impossible date %q, got %q", c, got)
			}
		})
	}
}

func TestValidate_DateFilterRejectsImpossibleDate(t *testing.T) {
	d := &Dashboard{
		Name: "t",
		Rows: []Row{{Widgets: []Widget{{Name: "w", Type: WidgetTypeText, Content: "hi"}}}},
		Filters: []Filter{
			{Name: "as_of", Type: "date", Default: "9999-99-99"},
		},
	}
	err := Validate(d)
	if err == nil || !strings.Contains(err.Error(), "invalid date default") {
		t.Fatalf("expected invalid-date-default error, got: %v", err)
	}
}

func TestDefaultFilters_UnquotedYAMLDate(t *testing.T) {
	yamlBody := `
name: t
rows:
  - widgets:
      - { name: w, type: text, content: hi }
filters:
  - name: as_of
    type: date
    default: 2024-01-15
`
	var d Dashboard
	if err := yaml.Unmarshal([]byte(yamlBody), &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got := d.DefaultFilters()
	if got["as_of"] != "2024-01-15" {
		t.Errorf("expected %q, got %v (%T)", "2024-01-15", got["as_of"], got["as_of"])
	}
}

func TestValidate_UnquotedYAMLDateAccepted(t *testing.T) {
	yamlBody := `
name: t
rows:
  - widgets:
      - { name: w, type: text, content: hi }
filters:
  - name: as_of
    type: date
    default: 2024-01-15
`
	var d Dashboard
	if err := yaml.Unmarshal([]byte(yamlBody), &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if err := Validate(&d); err != nil {
		t.Fatalf("expected no error for unquoted YAML date, got: %v", err)
	}
}

func TestValidate_DateFilterRejectsNonStringNonTimeDefault(t *testing.T) {
	d := &Dashboard{
		Name: "t",
		Rows: []Row{{Widgets: []Widget{{Name: "w", Type: WidgetTypeText, Content: "hi"}}}},
		Filters: []Filter{
			{Name: "as_of", Type: "date", Default: 123},
		},
	}
	err := Validate(d)
	if err == nil || !strings.Contains(err.Error(), "invalid date default") {
		t.Fatalf("expected invalid-date-default error for int default, got: %v", err)
	}
}
