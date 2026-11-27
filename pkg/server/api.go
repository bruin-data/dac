package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync"

	"github.com/bruin-data/dac/pkg/dashboard"
	"github.com/bruin-data/dac/pkg/query"
	tmpl "github.com/bruin-data/dac/pkg/template"
)

// WidgetJob represents a single SQL query to execute for a dashboard widget.
type WidgetJob struct {
	// GrandTotalSQL runs on Connection, like the detail query.
	GrandTotalSQL string
	ID            string
	SQL           string
	Connection    string
	// InlineData, when set, is returned directly without executing SQL — the
	// widget carries static data and needs no connection.
	InlineData *dashboard.WidgetData
	// ColumnRenames maps a lowercase executed result column name to the name the
	// dashboard references it by. Semantic joins sanitize qualified dimension
	// names (e.g. "customers.country" -> "customers_country"); this restores
	// the dashboard-facing name so widgets resolve the column.
	ColumnRenames map[string]string
	Series        []SeriesJob
}

// SeriesJob is model sparkline columns sharing one x; empty SQL means merged.
type SeriesJob struct {
	SQL, TotalSQL string
	Keys          int
	Window        int // index of x among the table's dimensions, or -1
	Metrics       []string
	Columns       []SeriesColumn
}

type SeriesColumn struct {
	Name, X, Y string
	Limit      int
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
	GrandTotal *WidgetQueryResult `json:"grandTotal,omitempty"`
	Columns    []struct {
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

	jobs, err := ResolveWidgetJobs(d, filters, s.backend)
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
			results[j.ID] = wr
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
// in a dashboard, including semantic metric and dimensional widgets that
// reference a semantic model.
func ResolveWidgetJobs(d *dashboard.Dashboard, filters map[string]any, backends ...query.Backend) ([]WidgetJob, error) {
	filters = d.NormalizeFilterValues(filters)
	if err := d.ValidateFilterValues(filters); err != nil {
		return nil, err
	}
	if err := tmpl.ValidateSemanticValue(filters); err != nil {
		return nil, err
	}
	connectionType := query.NewConnectionTypeResolver(backends...)
	var jobs []WidgetJob
	for i, row := range d.Rows {
		for j := range row.Widgets {
			widget := row.Widgets[j]

			// One job per tab, keyed r{i}-w{j}::{tab name}.
			if widget.HasTabs() {
				for k := range widget.Tabs {
					tab := widget.ResolvedTab(k)
					job, err := resolveWidgetJob(d, filters, WidgetTabID(i, j, tab.Name), &tab, connectionType)
					if err != nil {
						return nil, err
					}
					if job != nil {
						jobs = append(jobs, *job)
					}
				}
				continue
			}

			job, err := resolveWidgetJob(d, filters, WidgetID(i, j), &widget, connectionType)
			if err != nil {
				return nil, err
			}
			if job != nil {
				jobs = append(jobs, *job)
			}
		}
	}

	return jobs, nil
}

// resolveWidgetJob builds the job for one widget (or tab) under the given id, or
// nil when it carries no data (text/divider) or resolves to empty SQL.
func resolveWidgetJob(d *dashboard.Dashboard, filters map[string]any, id string, widget *dashboard.Widget, connectionType func(string) (string, error)) (*WidgetJob, error) {
	if err := widget.ValidateGrandTotal(d); err != nil {
		return nil, fmt.Errorf("widget %q: %w", widget.Name, err)
	}
	if widget.Type == dashboard.WidgetTypeText || widget.Type == dashboard.WidgetTypeDivider {
		return nil, nil
	}

	if widget.HasInlineData() {
		return &WidgetJob{ID: id, InlineData: widget.Data}, nil
	}

	if semanticJob, handled, err := d.ResolveWidgetSemanticJob(widget); err != nil {
		return nil, fmt.Errorf("widget %q: %w", widget.Name, err)
	} else if handled {
		dialect, err := connectionType(semanticJob.Connection)
		if err != nil {
			return nil, err
		}
		sql, conn, renames, err := compileSemanticJob(semanticJob, filters, dialect)
		if err != nil {
			return nil, fmt.Errorf("widget %q: %w", widget.Name, err)
		}
		job := &WidgetJob{ID: id, SQL: sql, Connection: conn, ColumnRenames: renames}
		if widget.GrandTotalEnabled() {
			grandTotalSQL, err := compileSemanticGrandTotalJob(semanticJob, filters, dialect)
			if err != nil {
				return nil, fmt.Errorf("widget %q grandTotal: %w", widget.Name, err)
			}
			job.GrandTotalSQL = grandTotalSQL
		}
		if err := compileSeriesJobs(semanticJob, widget, filters, dialect, job); err != nil {
			return nil, fmt.Errorf("widget %q: %w", widget.Name, err)
		}
		return job, nil
	}

	sql, conn, err := widget.ResolvedQuery(d)
	if err != nil {
		return nil, err
	}
	if sql == "" {
		return nil, nil
	}

	// Always render: even with no filters, SQL may reference the `bruin`
	// namespace (e.g. {{ bruin.user_email }}). RenderSQL() short-circuits
	// templates with no placeholders.
	dialect, err := connectionType(conn)
	if err != nil {
		return nil, err
	}
	sql, err = tmpl.RenderSQL(sql, filters, dialect)
	if err != nil {
		return nil, fmt.Errorf("template error: %w", err)
	}
	return &WidgetJob{ID: id, SQL: sql, Connection: conn}, nil
}

// ExecuteWidgetQuery runs a single widget SQL query against the given backend.
// Widgets carrying inline static data are returned directly without a backend call.
func ExecuteWidgetQuery(ctx context.Context, backend query.Backend, j WidgetJob) *WidgetQueryResult {
	if j.InlineData != nil {
		wr := &WidgetQueryResult{Rows: j.InlineData.Rows}
		for _, name := range j.InlineData.Columns {
			wr.Columns = append(wr.Columns, struct {
				Name string `json:"name"`
				Type string `json:"type,omitempty"`
			}{Name: name})
		}
		if wr.Rows == nil {
			wr.Rows = [][]any{}
		}
		return wr
	}

	qr, err := backend.Execute(ctx, j.Connection, j.SQL)
	if err != nil {
		return &WidgetQueryResult{Query: j.SQL, Error: err.Error()}
	}

	wr := &WidgetQueryResult{
		Rows:  make([][]any, len(qr.Rows)),
		Query: j.SQL,
	}
	for _, col := range qr.Columns {
		name := col.Name
		// Keys are lowercase: warehouses such as Snowflake uppercase unquoted aliases.
		if renamed, ok := j.ColumnRenames[strings.ToLower(name)]; ok {
			name = renamed
		}
		wr.Columns = append(wr.Columns, struct {
			Name string `json:"name"`
			Type string `json:"type,omitempty"`
		}{Name: name, Type: col.Type})
	}
	for i, row := range qr.Rows {
		wr.Rows[i] = slices.Clone(row) // cells are filled below; the backend may cache qr
	}
	if j.GrandTotalSQL != "" {
		grandTotal := ExecuteWidgetQuery(ctx, backend, WidgetJob{SQL: j.GrandTotalSQL, Connection: j.Connection, ColumnRenames: j.ColumnRenames})
		if grandTotal.Error != "" {
			return grandTotal
		}
		if len(grandTotal.Rows) > 1 {
			return &WidgetQueryResult{Query: j.GrandTotalSQL, Error: "expected at most one grand-total row"}
		}
		wr.GrandTotal = grandTotal
	}
	for _, s := range j.Series {
		rows, err := seriesPoints(ctx, backend, j.Connection, s)
		if err != nil {
			return &WidgetQueryResult{Query: s.SQL, Error: err.Error()}
		}
		total, err := seriesPoints(ctx, backend, j.Connection, SeriesJob{SQL: s.TotalSQL, Window: -1})
		if err != nil {
			return &WidgetQueryResult{Query: s.TotalSQL, Error: err.Error()}
		}
		fillSeries(s, rows, total, wr)
	}
	return wr
}

// WidgetID returns the canonical widget identifier for a given row and widget index.
func WidgetID(rowIdx, widgetIdx int) string {
	return fmt.Sprintf("r%d-w%d", rowIdx, widgetIdx)
}

// WidgetTabID returns the id for one tab of a widget, keyed by the tab's name
// (unique within the widget) so reordering tabs never mixes up their data. The
// frontend keys per-tab data by the same id.
func WidgetTabID(rowIdx, widgetIdx int, tabName string) string {
	return fmt.Sprintf("r%d-w%d::%s", rowIdx, widgetIdx, tabName)
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

	jobs, err := ResolveWidgetJobs(d, filters, s.backend)
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
	ch := make(chan streamResult, len(jobs))

	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for _, j := range jobs {
		wg.Add(1)
		go func(j WidgetJob) {
			defer wg.Done()
			sem <- struct{}{}
			wr := ExecuteWidgetQuery(r.Context(), s.backend, j)
			<-sem
			ch <- streamResult{ID: j.ID, Data: wr}
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
	jobs, err := ResolveWidgetJobs(d, filters, s.backend)
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
	}

	writeError(w, http.StatusNotFound, "widget not found: "+widgetID)
}

// seriesPoints runs a series query and groups its rows by row key.
func seriesPoints(ctx context.Context, backend query.Backend, connection string, s SeriesJob) (map[string][][]any, error) {
	if s.SQL == "" {
		return nil, nil
	}
	qr, err := backend.Execute(ctx, connection, s.SQL)
	if err != nil {
		return nil, err
	}
	byIdx, first := s.Keys, s.Keys+1
	if s.Window >= 0 {
		byIdx, first = s.Window, s.Keys
	}
	out := map[string][][]any{}
	for _, row := range qr.Rows {
		if len(row) > first && row[byIdx] != nil {
			key := seriesKey(row, s.Keys, s.Window)
			out[key] = append(out[key], append([]any{row[byIdx]}, row[first:]...))
		}
	}
	return out, nil
}

// fillSeries writes each row's last Limit points; a window row stops at its own x.
func fillSeries(s SeriesJob, points, total map[string][][]any, wr *WidgetQueryResult) {
	for _, c := range s.Columns {
		m := slices.Index(s.Metrics, c.Y) + 1
		read := func(row []any, idx int, rows [][]any) []map[string]any {
			if s.SQL == "" {
				var list []map[string]any
				if idx < len(row) {
					list = cellPoints(row[idx], c)
				}
				return append([]map[string]any{}, list[max(0, len(list)-c.Limit):]...)
			}
			// The last Limit points with a value; a NULL y isn't a point, as in the merged lists.
			out := []map[string]any{}
			for _, p := range slices.Backward(rows) {
				if len(out) == c.Limit {
					break
				}
				if p[m] != nil {
					out = append(out, map[string]any{c.X: p[0], c.Y: p[m]})
				}
			}
			slices.Reverse(out)
			return out
		}
		idx := seriesColumn(wr, c.Name)
		for i, row := range wr.Rows {
			rows := points[seriesKey(row, s.Keys, s.Window)]
			if s.Window >= 0 {
				rows = rows[:slices.IndexFunc(rows, func(p []any) bool { return reflect.DeepEqual(p[0], row[s.Window]) })+1]
			}
			wr.Rows[i] = setCell(row, idx, read(row, idx, rows))
		}
		if gt := wr.GrandTotal; gt != nil && len(gt.Rows) == 1 {
			idx := seriesColumn(gt, c.Name)
			gt.Rows[0] = setCell(gt.Rows[0], idx, read(gt.Rows[0], idx, total["[]"]))
		}
	}
}

// cellPoints reads a merged cell's newest-first {x, y} or [x, y] points, oldest first.
func cellPoints(cell any, c SeriesColumn) []map[string]any {
	items, _ := cell.([]any)
	var out []map[string]any
	for _, item := range slices.Backward(items) {
		switch p := item.(type) {
		case map[string]any:
			out = append(out, map[string]any{c.X: p["x"], c.Y: p["y"]})
		case []any:
			if len(p) == 2 {
				out = append(out, map[string]any{c.X: p[0], c.Y: p[1]})
			}
		}
	}
	return out
}

// seriesKey is the row's key values as JSON, without the window column.
func seriesKey(row []any, keys, window int) string {
	values := []any{}
	for i, v := range row[:min(keys, len(row))] {
		if i != window {
			values = append(values, v)
		}
	}
	data, _ := json.Marshal(values)
	return string(data)
}

func seriesColumn(wr *WidgetQueryResult, name string) int {
	for i, col := range wr.Columns {
		if strings.EqualFold(col.Name, name) {
			return i
		}
	}
	wr.Columns = append(wr.Columns, struct {
		Name string `json:"name"`
		Type string `json:"type,omitempty"`
	}{Name: name})
	return len(wr.Columns) - 1
}

func setCell(row []any, idx int, value any) []any {
	for len(row) <= idx {
		row = append(row, nil)
	}
	row[idx] = value
	return row
}
