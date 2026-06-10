package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/bruin-data/dac/pkg/dashboard"
	"github.com/bruin-data/dac/pkg/query"
	tmpl "github.com/bruin-data/dac/pkg/template"
)

// WidgetJob represents a single SQL query to execute for a dashboard widget.
type WidgetJob struct {
	ID         string
	SQL        string
	Connection string
	// MetricFanout is set on the merged metrics job: maps widget ID -> metric name.
	// When set, the single query result is fanned out to multiple widget results.
	MetricFanout map[string]string
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("error encoding JSON response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// DashboardSummary is a lightweight representation of a dashboard for listing.
type DashboardSummary struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Connection  string `json:"connection,omitempty"`
	WidgetCount int    `json:"widget_count"`
	FilterCount int    `json:"filter_count"`
	RowCount    int    `json:"row_count"`
}

// MakeDashboardSummary creates a DashboardSummary from a Dashboard.
func MakeDashboardSummary(d *dashboard.Dashboard) DashboardSummary {
	widgetCount := 0
	for _, row := range d.Rows {
		widgetCount += len(row.Widgets)
	}
	return DashboardSummary{
		Name:        d.Name,
		Description: d.Description,
		Connection:  d.Connection,
		WidgetCount: widgetCount,
		FilterCount: len(d.Filters),
		RowCount:    len(d.Rows),
	}
}

func (s *Server) handleListDashboards(w http.ResponseWriter, r *http.Request) {
	dashboards, err := s.loader.LoadMeta()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	summaries := make([]DashboardSummary, 0, len(dashboards))
	for _, d := range dashboards {
		summaries = append(summaries, MakeDashboardSummary(d))
	}
	writeJSON(w, http.StatusOK, map[string]any{"dashboards": summaries})
}

// resolveDashboard loads a single dashboard by name.
func (s *Server) resolveDashboard(name string) (*dashboard.Dashboard, error) {
	return s.loader.LoadOne(name)
}

func (s *Server) handleGetDashboard(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	d, err := s.resolveDashboard(name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if d == nil {
		writeError(w, http.StatusNotFound, "dashboard not found: "+name)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleGetDashboardRaw(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	dashboards, err := s.loader.LoadMeta()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	for _, d := range dashboards {
		if d.Name == name {
			data, err := os.ReadFile(d.FilePath)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "failed to read dashboard file: "+err.Error())
				return
			}
			ct := "text/yaml; charset=utf-8"
			if d.FileType == "tsx" {
				ct = "text/typescript; charset=utf-8"
			}
			w.Header().Set("Content-Type", ct)
			w.Write(data)
			return
		}
	}
	writeError(w, http.StatusNotFound, "dashboard not found: "+name)
}

type batchQueryRequest struct {
	Filters map[string]any `json:"filters"`
}

// WidgetQueryResult holds the result of executing a widget's SQL query.
type WidgetQueryResult struct {
	Columns []struct {
		Name string `json:"name"`
		Type string `json:"type,omitempty"`
	} `json:"columns"`
	Rows  [][]any `json:"rows"`
	Query string  `json:"query,omitempty"`
	Error string  `json:"error,omitempty"`
}

func (s *Server) handleBatchQuery(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var req batchQueryRequest
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
			return
		}
	}

	// Find the dashboard.
	d, err := s.loader.LoadOne(name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if d == nil {
		writeError(w, http.StatusNotFound, "dashboard not found: "+name)
		return
	}

	// Merge request filters over dashboard defaults so unset filters
	// still have values for query templating.
	filters := d.DefaultFilters()
	for k, v := range req.Filters {
		filters[k] = v
	}

	jobs, err := ResolveWidgetJobs(d, filters)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Execute all widget queries concurrently with a concurrency limit.
	results := make(map[string]*WidgetQueryResult)
	var mu sync.Mutex
	var wg sync.WaitGroup

	sem := make(chan struct{}, 8)
	for _, j := range jobs {
		wg.Add(1)
		go func(j WidgetJob) {
			defer wg.Done()
			sem <- struct{}{}
			wr := ExecuteWidgetQuery(r.Context(), s.backend, j)
			<-sem
			mu.Lock()
			if j.MetricFanout != nil {
				FanoutMetricResults(results, wr, j, d)
			} else {
				results[j.ID] = wr
			}
			mu.Unlock()
		}(j)
	}

	wg.Wait()
	writeJSON(w, http.StatusOK, map[string]any{"widgets": results})
}

func (s *Server) handleSingleQuery(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Connection string `json:"connection"`
		SQL        string `json:"sql"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	result, err := s.backend.Execute(r.Context(), req.Connection, req.SQL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	templateName := s.config.TemplateName
	resp := map[string]any{
		"template":      templateName,
		"admin_enabled": s.config.AdminPassword != "",
	}

	// If the template is a user-defined theme (not a built-in), include its tokens
	// so the frontend can apply them even without having the template's components.
	if t, ok := s.themes.Get(templateName); ok {
		resp["tokens"] = t.Tokens
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleListThemes(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"themes": s.themes.List()})
}

func (s *Server) handleGetTheme(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	t, ok := s.themes.Get(name)
	if !ok {
		writeError(w, http.StatusNotFound, "theme not found: "+name)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// ResolveWidgetJobs builds the list of SQL jobs for all data-bearing widgets
// in a dashboard, including declarative metric and dimensional widgets.
//
// Metric-ref widgets are merged into a single query to avoid redundant table
// scans. The merged result is then split into per-widget results via
// metricsJobGroup.
func ResolveWidgetJobs(d *dashboard.Dashboard, filters map[string]any) ([]WidgetJob, error) {
	// Extract date filter values for declarative SQL generation.
	var dateFilter map[string]any
	if dfName := d.DateRangeFilterName(); dfName != "" {
		if df, ok := filters[dfName]; ok {
			if m, ok := df.(map[string]any); ok {
				dateFilter = m
			}
		}
	}

	// First pass: collect metric-ref widgets to merge into one query.
	type metricWidget struct {
		id        string
		metricRef string
	}
	var metricWidgets []metricWidget

	var jobs []WidgetJob
	for i, row := range d.Rows {
		for j, widget := range row.Widgets {
			if widget.Type == dashboard.WidgetTypeText || widget.Type == dashboard.WidgetTypeDivider || widget.Type == dashboard.WidgetTypeImage {
				continue
			}

			var sql, conn string
			var err error

			if semanticJob, handled, err := d.ResolveWidgetSemanticJob(&widget); err != nil {
				return nil, fmt.Errorf("widget %q: %w", widget.Name, err)
			} else if handled {
				sql, conn, err = compileSemanticJob(semanticJob, filters)
				if err != nil {
					return nil, fmt.Errorf("widget %q: %w", widget.Name, err)
				}
				jobs = append(jobs, WidgetJob{ID: WidgetID(i, j), SQL: sql, Connection: conn})
				continue
			}

			switch {
			case widget.MetricRef != "" && d.SemanticSource() != nil:
				// Collect for merging — don't create individual jobs yet.
				metricWidgets = append(metricWidgets, metricWidget{
					id:        WidgetID(i, j),
					metricRef: widget.MetricRef,
				})
				continue

			case len(widget.MetricRefs) > 0 && widget.Dimension != "" && d.SemanticSource() != nil:
				dims := d.SemanticDimensions()
				dim, ok := dims[widget.Dimension]
				if !ok {
					return nil, fmt.Errorf("widget %q: dimension %q not found", widget.Name, widget.Dimension)
				}
				sql, err = dashboard.GenerateDimensionalSQL(d.SemanticSource(), d.SemanticMetrics(), widget.MetricRefs, &dim, dateFilter, widget.Limit)
				if err != nil {
					return nil, fmt.Errorf("widget %q: %w", widget.Name, err)
				}
				conn = d.SourceConnection()

			default:
				sql, conn, err = widget.ResolvedQuery(d)
				if err != nil {
					return nil, err
				}
				if sql == "" {
					continue
				}
			}

			if len(filters) > 0 {
				sql, err = tmpl.Render(sql, filters)
				if err != nil {
					return nil, fmt.Errorf("template error: %w", err)
				}
			}
			jobs = append(jobs, WidgetJob{ID: WidgetID(i, j), SQL: sql, Connection: conn})
		}
	}

	// Merge all metric-ref widgets into a single query.
	if len(metricWidgets) > 0 && d.SemanticSource() != nil {
		sql, err := dashboard.GenerateMetricsSQL(d.SemanticSource(), d.SemanticMetrics(), dateFilter)
		if err != nil {
			return nil, fmt.Errorf("merged metrics query: %w", err)
		}
		if len(filters) > 0 {
			sql, err = tmpl.Render(sql, filters)
			if err != nil {
				return nil, fmt.Errorf("template error: %w", err)
			}
		}

		// Build mapping from widget ID to its metric ref / expression.
		widgetMetrics := make(map[string]string, len(metricWidgets))
		for _, mw := range metricWidgets {
			widgetMetrics[mw.id] = mw.metricRef
		}

		jobs = append(jobs, WidgetJob{
			ID:           MetricsJobID,
			SQL:          sql,
			Connection:   d.SourceConnection(),
			MetricFanout: widgetMetrics,
		})
	}

	return jobs, nil
}

// MetricsJobID is the sentinel job ID used for the merged metrics query.
const MetricsJobID = "__metrics__"

// ExecuteWidgetQuery runs a single widget SQL query against the given backend.
func ExecuteWidgetQuery(ctx context.Context, backend query.Backend, j WidgetJob) *WidgetQueryResult {
	qr, err := backend.Execute(ctx, j.Connection, j.SQL)
	if err != nil {
		return &WidgetQueryResult{Query: j.SQL, Error: err.Error()}
	}

	wr := &WidgetQueryResult{
		Rows:  make([][]any, len(qr.Rows)),
		Query: j.SQL,
	}
	for _, col := range qr.Columns {
		wr.Columns = append(wr.Columns, struct {
			Name string `json:"name"`
			Type string `json:"type,omitempty"`
		}{Name: col.Name, Type: col.Type})
	}
	for i, row := range qr.Rows {
		wr.Rows[i] = row
	}
	return wr
}

// WidgetID returns the canonical widget identifier for a given row and widget index.
func WidgetID(rowIdx, widgetIdx int) string {
	return fmt.Sprintf("r%d-w%d", rowIdx, widgetIdx)
}

// handleStreamQuery is the streaming variant of handleBatchQuery.
// It writes each widget result as a newline-delimited JSON line
// as soon as the query completes, so the frontend can render
// widgets incrementally.
func (s *Server) handleStreamQuery(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	name := r.PathValue("name")

	var req batchQueryRequest
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
			return
		}
	}

	d, err := s.loader.LoadOne(name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if d == nil {
		writeError(w, http.StatusNotFound, "dashboard not found: "+name)
		return
	}

	filters := d.DefaultFilters()
	for k, v := range req.Filters {
		filters[k] = v
	}

	jobs, err := ResolveWidgetJobs(d, filters)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Set up streaming response.
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// Results channel — each goroutine sends its result here.
	type streamResult struct {
		ID   string             `json:"id"`
		Data *WidgetQueryResult `json:"data"`
	}
	// Buffer size accounts for metric fanout producing multiple results per job.
	bufSize := len(jobs)
	for _, j := range jobs {
		if j.MetricFanout != nil {
			bufSize += len(j.MetricFanout) - 1
		}
	}
	ch := make(chan streamResult, bufSize)

	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for _, j := range jobs {
		wg.Add(1)
		go func(j WidgetJob) {
			defer wg.Done()
			sem <- struct{}{}
			wr := ExecuteWidgetQuery(r.Context(), s.backend, j)
			<-sem
			if j.MetricFanout != nil {
				// Fan out the merged metrics result to individual widget results.
				for wid, metricRef := range j.MetricFanout {
					wr := FanoutSingleMetric(wr, metricRef, j.SQL, d)
					ch <- streamResult{ID: wid, Data: wr}
				}
			} else {
				ch <- streamResult{ID: j.ID, Data: wr}
			}
		}(j)
	}

	// Close channel when all goroutines complete.
	go func() {
		wg.Wait()
		close(ch)
	}()

	enc := json.NewEncoder(w)
	for result := range ch {
		if err := enc.Encode(result); err != nil {
			return // client disconnected
		}
		flusher.Flush()
	}
}

// FanoutMetricResults splits a merged metrics query result into individual
// widget results and adds them to the results map.
func FanoutMetricResults(results map[string]*WidgetQueryResult, merged *WidgetQueryResult, j WidgetJob, d *dashboard.Dashboard) {
	for wid, metricRef := range j.MetricFanout {
		results[wid] = FanoutSingleMetric(merged, metricRef, j.SQL, d)
	}
}

// FanoutSingleMetric extracts a single metric's value from a merged query
// result. For aggregate metrics, it picks the column by name. For expression
// metrics, it evaluates the expression client-side from the aggregate values.
func FanoutSingleMetric(merged *WidgetQueryResult, metricRef string, sql string, d *dashboard.Dashboard) *WidgetQueryResult {
	if merged.Error != "" {
		return &WidgetQueryResult{Query: sql, Error: merged.Error}
	}

	m, ok := d.SemanticMetrics()[metricRef]
	if !ok {
		return &WidgetQueryResult{Query: sql, Error: fmt.Sprintf("metric %q not found", metricRef)}
	}

	if m.IsExpression() {
		// Evaluate expression from aggregate values in the merged result.
		values := make(map[string]float64)
		if len(merged.Rows) > 0 {
			for ci, col := range merged.Columns {
				if ci < len(merged.Rows[0]) {
					if v, ok := toFloat64(merged.Rows[0][ci]); ok {
						values[col.Name] = v
					}
				}
			}
		}
		result, err := dashboard.EvaluateExpression(m.Expression, values)
		if err != nil {
			return &WidgetQueryResult{Query: sql, Error: err.Error()}
		}
		return &WidgetQueryResult{
			Columns: []struct {
				Name string `json:"name"`
				Type string `json:"type,omitempty"`
			}{{Name: metricRef}},
			Rows:  [][]any{{result}},
			Query: sql,
		}
	}

	// Find the column index for this metric.
	colIdx := -1
	for i, col := range merged.Columns {
		if col.Name == metricRef {
			colIdx = i
			break
		}
	}
	if colIdx < 0 {
		return &WidgetQueryResult{Query: sql, Error: fmt.Sprintf("column %q not found in merged result", metricRef)}
	}

	// Extract just this column.
	var value any
	if len(merged.Rows) > 0 && colIdx < len(merged.Rows[0]) {
		value = merged.Rows[0][colIdx]
	}
	return &WidgetQueryResult{
		Columns: []struct {
			Name string `json:"name"`
			Type string `json:"type,omitempty"`
		}{{Name: metricRef, Type: merged.Columns[colIdx].Type}},
		Rows:  [][]any{{value}},
		Query: sql,
	}
}

// handleWidgetQuery executes a single widget's query and returns the result.
// This allows the frontend to fetch data per-widget (lazy, on-demand).
func (s *Server) handleWidgetQuery(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	widgetID := r.PathValue("widgetId")

	var req batchQueryRequest
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
			return
		}
	}

	d, err := s.resolveDashboard(name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if d == nil {
		writeError(w, http.StatusNotFound, "dashboard not found: "+name)
		return
	}

	filters := d.DefaultFilters()
	for k, v := range req.Filters {
		filters[k] = v
	}

	// Resolve all jobs and find the one matching this widget ID.
	// For metric-ref widgets the merged job's MetricFanout map contains the widget ID.
	jobs, err := ResolveWidgetJobs(d, filters)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	for _, j := range jobs {
		if j.ID == widgetID {
			wr := ExecuteWidgetQuery(r.Context(), s.backend, j)
			writeJSON(w, http.StatusOK, wr)
			return
		}
		// Check if this widget is part of a merged metrics job.
		if j.MetricFanout != nil {
			if _, ok := j.MetricFanout[widgetID]; ok {
				wr := ExecuteWidgetQuery(r.Context(), s.backend, j)
				result := FanoutSingleMetric(wr, j.MetricFanout[widgetID], j.SQL, d)
				writeJSON(w, http.StatusOK, result)
				return
			}
		}
	}

	writeError(w, http.StatusNotFound, "widget not found: "+widgetID)
}

func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}
