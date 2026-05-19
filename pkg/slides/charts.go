package slides

import (
	"bytes"
	"fmt"
	"strconv"

	chart "github.com/wcharczuk/go-chart/v2"

	"github.com/bruin-data/dac/pkg/dashboard"
	"github.com/bruin-data/dac/pkg/server"
)

// renderChart renders a chart widget to PNG bytes.
func renderChart(w *dashboard.Widget, data *server.WidgetQueryResult) ([]byte, error) {
	if data == nil || data.Error != "" || len(data.Rows) == 0 {
		return nil, fmt.Errorf("no data")
	}

	switch w.Chart {
	case "pie", "funnel":
		return renderPieChart(w, data)
	case "bar":
		return renderBarChart(w, data)
	case "line", "area", "sparkline":
		return renderLineChart(w, data)
	default:
		return renderBarChart(w, data)
	}
}

func renderPieChart(w *dashboard.Widget, data *server.WidgetQueryResult) ([]byte, error) {
	labelCol := colIdx(data, w.Label)
	valueCol := colIdx(data, w.Value)
	if labelCol < 0 || valueCol < 0 {
		return nil, fmt.Errorf("label/value columns not found")
	}

	var values []chart.Value
	for _, row := range data.Rows {
		values = append(values, chart.Value{
			Value: toFloat64(row[valueCol]),
			Label: fmt.Sprint(row[labelCol]),
		})
	}

	pie := chart.PieChart{
		Width:  800,
		Height: 500,
		Values: values,
	}

	var buf bytes.Buffer
	if err := pie.Render(chart.PNG, &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func renderBarChart(w *dashboard.Widget, data *server.WidgetQueryResult) ([]byte, error) {
	xCol, yCol := chartXY(w, data)
	if xCol < 0 || yCol < 0 {
		return nil, fmt.Errorf("x/y columns not found")
	}

	var bars []chart.Value
	for _, row := range data.Rows {
		bars = append(bars, chart.Value{
			Value: toFloat64(row[yCol]),
			Label: fmt.Sprint(row[xCol]),
		})
	}

	bc := chart.BarChart{
		Width:  800,
		Height: 500,
		Bars:   bars,
	}

	var buf bytes.Buffer
	if err := bc.Render(chart.PNG, &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func renderLineChart(w *dashboard.Widget, data *server.WidgetQueryResult) ([]byte, error) {
	xCol, yCol := chartXY(w, data)
	if xCol < 0 || yCol < 0 {
		return nil, fmt.Errorf("x/y columns not found")
	}

	xVals := make([]float64, len(data.Rows))
	yVals := make([]float64, len(data.Rows))
	var ticks []chart.Tick

	for i, row := range data.Rows {
		xVals[i] = float64(i)
		yVals[i] = toFloat64(row[yCol])
		label := fmt.Sprint(row[xCol])
		if len(data.Rows) <= 12 || i%(len(data.Rows)/6+1) == 0 {
			ticks = append(ticks, chart.Tick{Value: float64(i), Label: label})
		}
	}

	series := chart.ContinuousSeries{
		XValues: xVals,
		YValues: yVals,
	}
	if w.Chart == "area" {
		series.Style = chart.Style{
			FillColor: chart.ColorBlue.WithAlpha(64),
		}
	}

	graph := chart.Chart{
		Width:  800,
		Height: 500,
		XAxis:  chart.XAxis{Ticks: ticks},
		Series: []chart.Series{series},
	}

	var buf bytes.Buffer
	if err := graph.Render(chart.PNG, &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// chartXY returns the x and first y column indices for a chart widget.
func chartXY(w *dashboard.Widget, data *server.WidgetQueryResult) (int, int) {
	xCol := colIdx(data, w.XField())
	if xCol < 0 && w.Label != "" {
		xCol = colIdx(data, w.Label)
	}

	yCol := -1
	if ys := w.YFields(); len(ys) > 0 {
		yCol = colIdx(data, ys[0])
	} else if w.Value != "" {
		yCol = colIdx(data, w.Value)
	}

	return xCol, yCol
}

func colIdx(data *server.WidgetQueryResult, name string) int {
	for i, c := range data.Columns {
		if c.Name == name {
			return i
		}
	}
	return -1
}

func toFloat64(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case string:
		f, _ := strconv.ParseFloat(n, 64)
		return f
	default:
		return 0
	}
}
