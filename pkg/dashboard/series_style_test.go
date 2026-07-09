package dashboard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFile_SeriesStylesYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dashboard.yml")
	assertNoErr(t, os.WriteFile(path, []byte(`schema: https://getbruin.com/schemas/dac/dashboard/v1
name: Pattern Styles
rows:
  - widgets:
      - name: Revenue vs Forecast
        type: chart
        chart: line
        sql: SELECT month, actual_revenue, forecast_revenue FROM revenue_by_month
        x: { field: month }
        y: { field: [actual_revenue, forecast_revenue] }
        seriesStyles:
          actual_revenue:
            lineStyle: solid
          forecast_revenue:
            lineStyle: dashed
            fillStyle: hatched
`), 0o644))

	d, err := LoadFile(path)
	assertNoErr(t, err)

	styles := d.Rows[0].Widgets[0].SeriesStyles
	if styles["actual_revenue"].LineStyle != "solid" {
		t.Fatalf("expected actual_revenue lineStyle solid, got %+v", styles["actual_revenue"])
	}
	if styles["forecast_revenue"].LineStyle != "dashed" || styles["forecast_revenue"].FillStyle != "hatched" {
		t.Fatalf("expected forecast_revenue dashed + hatched, got %+v", styles["forecast_revenue"])
	}
}

func TestEvalTSX_SeriesStyles(t *testing.T) {
	source := `
export default (
  <Dashboard name="Pattern Styles" connection="duckdb">
    <Row>
      <Chart name="Regional Mix" chart="bar" col={12}
        sql="SELECT region, current_share, prior_share FROM regional_mix"
        x={{ field: "region" }}
        y={{ field: ["current_share", "prior_share"] }}
        seriesStyles={{
          current_share: { fillStyle: "solid" },
          prior_share: { fillStyle: "striped" },
        }} />
    </Row>
  </Dashboard>
)
`
	d, err := evalTSX(source, "test.tsx", &tsxConfig{})
	assertNoErr(t, err)

	styles := d.Rows[0].Widgets[0].SeriesStyles
	if styles["current_share"].FillStyle != "solid" {
		t.Fatalf("expected current_share fillStyle solid, got %+v", styles["current_share"])
	}
	if styles["prior_share"].FillStyle != "striped" {
		t.Fatalf("expected prior_share fillStyle striped, got %+v", styles["prior_share"])
	}
}

func TestWidgetSeriesStyles_JSONRoundTrip(t *testing.T) {
	w := Widget{
		Name:  "Revenue",
		Type:  WidgetTypeChart,
		Chart: "line",
		SeriesStyles: map[string]SeriesStyle{
			"forecast": {LineStyle: "dotted"},
		},
	}

	data, err := json.Marshal(w)
	assertNoErr(t, err)

	var got Widget
	assertNoErr(t, json.Unmarshal(data, &got))

	if got.SeriesStyles["forecast"].LineStyle != "dotted" {
		t.Fatalf("expected forecast lineStyle dotted after JSON round trip, got %+v", got.SeriesStyles["forecast"])
	}
}

func TestValidate_SeriesStylesRejectUnknownValues(t *testing.T) {
	d := &Dashboard{
		Name: "Invalid Pattern Styles",
		Rows: []Row{{Widgets: []Widget{{
			Name:  "Revenue",
			Type:  WidgetTypeChart,
			Chart: "line",
			SQL:   "SELECT month, forecast FROM revenue_by_month",
			X:     &AxisEncoding{Field: "month"},
			Y:     &AxisEncoding{Field: "forecast"},
			SeriesStyles: map[string]SeriesStyle{
				"forecast": {LineStyle: "dash", FillStyle: "checkerboard"},
			},
		}}}},
	}

	err := Validate(d)
	assertErr(t, err)
	assertValidationContains(t, err, "lineStyle must be one of solid, dashed, or dotted")
	assertValidationContains(t, err, "fillStyle must be one of solid, striped, or hatched")
}
