package slides

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dustin/go-humanize"

	"github.com/bruin-data/dac/pkg/dashboard"
	"github.com/bruin-data/dac/pkg/query"
	"github.com/bruin-data/dac/pkg/server"
	driveapi "google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
	slidesapi "google.golang.org/api/slides/v1"
)

// Slide dimensions in EMU (English Metric Units). 1 inch = 914400 EMU.
const (
	slideW   int64 = 9144000 // 10"
	slideH   int64 = 5143500 // 5.625"
	padX     int64 = 457200  // 0.5"
	padY     int64 = 457200  // 0.5"
	contentW       = slideW - 2*padX
	contentH       = slideH - 2*padY

	titleBarH int64 = 365760 // 0.4" for widget name
	gapV      int64 = 91440  // 0.1" vertical gap
)

// Config holds configuration for the Google Slides export.
type Config struct {
	DashboardDir string
	Dashboard    string
	Credentials  string // path to Google OAuth credentials.json
	Filters      map[string]any
	ConfigFile   string
	Environment  string
}

// Export creates a Google Slides presentation from a dashboard and returns its URL.
func Export(ctx context.Context, cfg Config) (string, error) {
	// Load dashboard.
	dashboards, err := dashboard.LoadDir(cfg.DashboardDir)
	if err != nil {
		return "", fmt.Errorf("loading dashboards: %w", err)
	}
	if err := dashboard.ValidateAll(dashboards); err != nil {
		return "", fmt.Errorf("validating dashboards: %w", err)
	}
	d := dashboard.FindByName(dashboards, cfg.Dashboard)
	if d == nil {
		return "", fmt.Errorf("dashboard not found: %q", cfg.Dashboard)
	}

	// Execute queries.
	backend := &query.BruinCLIBackend{
		ConfigFile:  cfg.ConfigFile,
		Environment: cfg.Environment,
	}
	filters := d.DefaultFilters()
	for k, v := range cfg.Filters {
		filters[k] = v
	}

	jobs, err := server.ResolveWidgetJobs(d, filters)
	if err != nil {
		return "", fmt.Errorf("resolving queries: %w", err)
	}

	widgetData := executeJobs(ctx, backend, jobs, d)

	// Authenticate with Google.
	creds, err := authorize(ctx, cfg.Credentials)
	if err != nil {
		return "", fmt.Errorf("google auth: %w", err)
	}

	slidesSvc, err := slidesapi.NewService(ctx, option.WithCredentials(creds))
	if err != nil {
		return "", fmt.Errorf("slides service: %w", err)
	}
	driveSvc, err := driveapi.NewService(ctx, option.WithCredentials(creds))
	if err != nil {
		return "", fmt.Errorf("drive service: %w", err)
	}

	// Create presentation.
	pres, err := slidesSvc.Presentations.Create(&slidesapi.Presentation{
		Title: d.Name,
	}).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("creating presentation: %w", err)
	}
	presID := pres.PresentationId

	// Set up title slide + create blank content slides.
	var setupReqs []*slidesapi.Request
	setupReqs = append(setupReqs, titleSlideReqs(pres.Slides[0].ObjectId, d)...)
	for i := range d.Rows {
		setupReqs = append(setupReqs, &slidesapi.Request{
			CreateSlide: &slidesapi.CreateSlideRequest{
				ObjectId: fmt.Sprintf("slide_%d", i),
				SlideLayoutReference: &slidesapi.LayoutReference{
					PredefinedLayout: "BLANK",
				},
			},
		})
	}

	if _, err := slidesSvc.Presentations.BatchUpdate(presID, &slidesapi.BatchUpdatePresentationRequest{
		Requests: setupReqs,
	}).Context(ctx).Do(); err != nil {
		return "", fmt.Errorf("setting up slides: %w", err)
	}

	// Populate each content slide with widgets.
	var driveCleanup []string
	for i, row := range d.Rows {
		slideID := fmt.Sprintf("slide_%d", i)
		reqs, fileIDs, err := widgetRequests(ctx, driveSvc, slideID, i, row, d, widgetData)
		if err != nil {
			log.Printf("Warning: slide %d: %v", i, err)
			continue
		}
		driveCleanup = append(driveCleanup, fileIDs...)
		if len(reqs) > 0 {
			if _, err := slidesSvc.Presentations.BatchUpdate(presID, &slidesapi.BatchUpdatePresentationRequest{
				Requests: reqs,
			}).Context(ctx).Do(); err != nil {
				log.Printf("Warning: populating slide %d: %v", i, err)
			}
		}
	}

	// Clean up temporary Drive images.
	for _, fid := range driveCleanup {
		_ = driveSvc.Files.Delete(fid).Context(ctx).Do()
	}

	return fmt.Sprintf("https://docs.google.com/presentation/d/%s/edit", presID), nil
}

func executeJobs(ctx context.Context, backend query.Backend, jobs []server.WidgetJob, d *dashboard.Dashboard) map[string]*server.WidgetQueryResult {
	results := make(map[string]*server.WidgetQueryResult)
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)

	for _, j := range jobs {
		wg.Add(1)
		go func(j server.WidgetJob) {
			defer wg.Done()
			sem <- struct{}{}
			wr := server.ExecuteWidgetQuery(ctx, backend, j)
			<-sem
			mu.Lock()
			if j.MetricFanout != nil {
				server.FanoutMetricResults(results, wr, j, d)
			} else {
				results[j.ID] = wr
			}
			mu.Unlock()
		}(j)
	}
	wg.Wait()
	return results
}

// --- Title slide ---

func titleSlideReqs(slideID string, d *dashboard.Dashboard) []*slidesapi.Request {
	reqs := styledTextBox("title_box", slideID, d.Name, padX, slideH/2-600000, contentW, 800000, 36, true, "CENTER")
	if d.Description != "" {
		reqs = append(reqs, styledTextBox("desc_box", slideID, d.Description, padX, slideH/2+200000, contentW, 500000, 16, false, "CENTER")...)
	}
	return reqs
}

// --- Widget renderers ---

func widgetRequests(ctx context.Context, driveSvc *driveapi.Service, slideID string, rowIdx int, row dashboard.Row, d *dashboard.Dashboard, widgetData map[string]*server.WidgetQueryResult) ([]*slidesapi.Request, []string, error) {
	var reqs []*slidesapi.Request
	var fileIDs []string

	colUnit := float64(contentW) / 12.0
	xOff := int64(0)

	for j, w := range row.Widgets {
		cols := w.Col
		if cols <= 0 {
			cols = 12 / len(row.Widgets)
		}

		wWidth := int64(colUnit * float64(cols))
		wID := server.WidgetID(rowIdx, j)
		data := widgetData[wID]
		prefix := fmt.Sprintf("r%d_w%d", rowIdx, j)
		x := padX + xOff

		switch w.Type {
		case dashboard.WidgetTypeMetric:
			reqs = append(reqs, metricReqs(prefix, slideID, &w, data, x, padY, wWidth)...)

		case dashboard.WidgetTypeChart:
			cr, fid, err := chartImageReqs(ctx, driveSvc, prefix, slideID, &w, data, x, padY, wWidth)
			if err != nil {
				log.Printf("Chart %q: %v, using placeholder", w.Name, err)
				reqs = append(reqs, placeholderReqs(prefix, slideID, &w, x, padY, wWidth)...)
			} else {
				reqs = append(reqs, cr...)
				if fid != "" {
					fileIDs = append(fileIDs, fid)
				}
			}

		case dashboard.WidgetTypeTable:
			reqs = append(reqs, tableReqs(prefix, slideID, &w, data, x, padY, wWidth)...)

		case dashboard.WidgetTypeText:
			reqs = append(reqs, textContentReqs(prefix, slideID, &w, x, padY, wWidth)...)
		}

		xOff += wWidth
	}

	return reqs, fileIDs, nil
}

func metricReqs(prefix, slideID string, w *dashboard.Widget, data *server.WidgetQueryResult, x, y, width int64) []*slidesapi.Request {
	value := formatMetricValue(w, data)

	// Center the metric card vertically on the slide.
	cardH := int64(1800000) // 2" total card height
	cardY := y + (contentH-cardH)/2
	labelH := int64(400000)  // 0.44" for label
	valueH := cardH - labelH // remaining for value

	reqs := styledTextBox(prefix+"_title", slideID, w.Name, x, cardY, width, labelH, 12, false, "CENTER")
	// Set title text color to gray.
	reqs = append(reqs, &slidesapi.Request{
		UpdateTextStyle: &slidesapi.UpdateTextStyleRequest{
			ObjectId: prefix + "_title",
			Style: &slidesapi.TextStyle{
				ForegroundColor: &slidesapi.OptionalColor{
					OpaqueColor: &slidesapi.OpaqueColor{
						RgbColor: &slidesapi.RgbColor{Red: 0.45, Green: 0.45, Blue: 0.45},
					},
				},
			},
			TextRange: &slidesapi.Range{Type: "ALL"},
			Fields:    "foregroundColor",
		},
	})

	reqs = append(reqs, styledTextBox(prefix+"_value", slideID, value, x, cardY+labelH, width, valueH, 28, true, "CENTER")...)
	reqs = append(reqs, &slidesapi.Request{
		UpdateShapeProperties: &slidesapi.UpdateShapePropertiesRequest{
			ObjectId: prefix + "_value",
			ShapeProperties: &slidesapi.ShapeProperties{
				ContentAlignment: "MIDDLE",
			},
			Fields: "contentAlignment",
		},
	})
	return reqs
}

func chartImageReqs(ctx context.Context, driveSvc *driveapi.Service, prefix, slideID string, w *dashboard.Widget, data *server.WidgetQueryResult, x, y, width int64) ([]*slidesapi.Request, string, error) {
	pngData, err := renderChart(w, data)
	if err != nil {
		return nil, "", err
	}

	fileID, err := uploadImage(ctx, driveSvc, fmt.Sprintf("dac-%s.png", w.Name), pngData)
	if err != nil {
		return nil, "", fmt.Errorf("uploading chart: %w", err)
	}

	if _, err := driveSvc.Permissions.Create(fileID, &driveapi.Permission{
		Type: "anyone",
		Role: "reader",
	}).Context(ctx).Do(); err != nil {
		return nil, fileID, fmt.Errorf("setting permissions: %w", err)
	}

	imgURL := fmt.Sprintf("https://drive.google.com/uc?export=download&id=%s", fileID)
	chartH := contentH - titleBarH - gapV

	reqs := styledTextBox(prefix+"_title", slideID, w.Name, x, y, width, titleBarH, 14, false, "START")
	reqs = append(reqs, &slidesapi.Request{
		CreateImage: &slidesapi.CreateImageRequest{
			ObjectId: prefix + "_img",
			Url:      imgURL,
			ElementProperties: &slidesapi.PageElementProperties{
				PageObjectId: slideID,
				Size: &slidesapi.Size{
					Width:  emu(width),
					Height: emu(chartH),
				},
				Transform: &slidesapi.AffineTransform{
					ScaleX:     1,
					ScaleY:     1,
					TranslateX: float64(x),
					TranslateY: float64(y + titleBarH + gapV),
					Unit:       "EMU",
				},
			},
		},
	})

	return reqs, fileID, nil
}

func tableReqs(prefix, slideID string, w *dashboard.Widget, data *server.WidgetQueryResult, x, y, width int64) []*slidesapi.Request {
	if data == nil || data.Error != "" || len(data.Rows) == 0 || len(data.Columns) == 0 {
		return placeholderReqs(prefix, slideID, w, x, y, width)
	}

	maxRows := 10
	if len(data.Rows) < maxRows {
		maxRows = len(data.Rows)
	}
	numCols := int64(len(data.Columns))
	numRows := int64(maxRows + 1) // +1 for header
	tableH := contentH - titleBarH - gapV
	tableID := prefix + "_table"

	reqs := styledTextBox(prefix+"_title", slideID, w.Name, x, y, width, titleBarH, 14, false, "START")
	reqs = append(reqs, &slidesapi.Request{
		CreateTable: &slidesapi.CreateTableRequest{
			ObjectId: tableID,
			Rows:     numRows,
			Columns:  numCols,
			ElementProperties: &slidesapi.PageElementProperties{
				PageObjectId: slideID,
				Size: &slidesapi.Size{
					Width:  emu(width),
					Height: emu(tableH),
				},
				Transform: &slidesapi.AffineTransform{
					ScaleX:     1,
					ScaleY:     1,
					TranslateX: float64(x),
					TranslateY: float64(y + titleBarH + gapV),
					Unit:       "EMU",
				},
			},
		},
	})

	// Header row.
	for c, col := range data.Columns {
		label := col.Name
		for _, wc := range w.Columns {
			if wc.Name == col.Name && wc.Label != "" {
				label = wc.Label
				break
			}
		}
		reqs = append(reqs, &slidesapi.Request{
			InsertText: &slidesapi.InsertTextRequest{
				ObjectId:     tableID,
				Text:         label,
				CellLocation: &slidesapi.TableCellLocation{RowIndex: 0, ColumnIndex: int64(c)},
			},
		})
	}

	// Data rows.
	for r := 0; r < maxRows; r++ {
		for c := range data.Columns {
			reqs = append(reqs, &slidesapi.Request{
				InsertText: &slidesapi.InsertTextRequest{
					ObjectId:     tableID,
					Text:         fmt.Sprint(data.Rows[r][c]),
					CellLocation: &slidesapi.TableCellLocation{RowIndex: int64(r + 1), ColumnIndex: int64(c)},
				},
			})
		}
	}

	return reqs
}

func textContentReqs(prefix, slideID string, w *dashboard.Widget, x, y, width int64) []*slidesapi.Request {
	return styledTextBox(prefix+"_text", slideID, w.Content, x, y, width, contentH, 14, false, "START")
}

func placeholderReqs(prefix, slideID string, w *dashboard.Widget, x, y, width int64) []*slidesapi.Request {
	text := w.Name
	if w.Chart != "" {
		text = fmt.Sprintf("[%s chart] %s", w.Chart, w.Name)
	}
	reqs := styledTextBox(prefix+"_ph", slideID, text, x, y, width, contentH, 14, false, "CENTER")
	reqs = append(reqs, &slidesapi.Request{
		UpdateShapeProperties: &slidesapi.UpdateShapePropertiesRequest{
			ObjectId: prefix + "_ph",
			ShapeProperties: &slidesapi.ShapeProperties{
				ContentAlignment: "MIDDLE",
			},
			Fields: "contentAlignment",
		},
	})
	return reqs
}

// --- Helpers ---

func styledTextBox(id, pageID, text string, x, y, w, h int64, fontSize float64, bold bool, align string) []*slidesapi.Request {
	fields := "fontSize,fontFamily"
	style := &slidesapi.TextStyle{
		FontSize:   &slidesapi.Dimension{Magnitude: fontSize, Unit: "PT"},
		FontFamily: "Google Sans",
	}
	if bold {
		fields += ",bold"
		style.Bold = true
	}

	return []*slidesapi.Request{
		{
			CreateShape: &slidesapi.CreateShapeRequest{
				ObjectId:  id,
				ShapeType: "TEXT_BOX",
				ElementProperties: &slidesapi.PageElementProperties{
					PageObjectId: pageID,
					Size: &slidesapi.Size{
						Width:  emu(w),
						Height: emu(h),
					},
					Transform: &slidesapi.AffineTransform{
						ScaleX:     1,
						ScaleY:     1,
						TranslateX: float64(x),
						TranslateY: float64(y),
						Unit:       "EMU",
					},
				},
			},
		},
		{InsertText: &slidesapi.InsertTextRequest{ObjectId: id, Text: text}},
		{
			UpdateTextStyle: &slidesapi.UpdateTextStyleRequest{
				ObjectId:  id,
				Style:     style,
				TextRange: &slidesapi.Range{Type: "ALL"},
				Fields:    fields,
			},
		},
		{
			UpdateParagraphStyle: &slidesapi.UpdateParagraphStyleRequest{
				ObjectId:  id,
				Style:     &slidesapi.ParagraphStyle{Alignment: align},
				TextRange: &slidesapi.Range{Type: "ALL"},
				Fields:    "alignment",
			},
		},
	}
}

func emu(v int64) *slidesapi.Dimension {
	return &slidesapi.Dimension{Magnitude: float64(v), Unit: "EMU"}
}

func uploadImage(ctx context.Context, driveSvc *driveapi.Service, name string, data []byte) (string, error) {
	f, err := driveSvc.Files.Create(&driveapi.File{
		Name:     name,
		MimeType: "image/png",
	}).Media(bytes.NewReader(data)).Context(ctx).Do()
	if err != nil {
		return "", err
	}
	return f.Id, nil
}

func formatMetricValue(w *dashboard.Widget, data *server.WidgetQueryResult) string {
	if data == nil || data.Error != "" || len(data.Rows) == 0 || len(data.Rows[0]) == 0 {
		return "\u2014" // em dash
	}

	ci := 0
	if field := w.ValueField(); field != "" {
		for i, c := range data.Columns {
			if c.Name == field {
				ci = i
				break
			}
		}
	}

	raw := data.Rows[0][ci]

	var format, typ string
	if w.Value != nil {
		format, typ = w.Value.Format, w.Value.Type
	}

	// Date-typed values render with the d3-time-format string.
	if typ == "date" {
		if t, ok := parseMetricTime(raw); ok {
			if layout := strftimeToGo(format); layout != "" {
				return t.Format(layout)
			}
		}
		return fmt.Sprintf("%v", raw)
	}

	num, ok := toFloat64OK(raw)
	if !ok {
		return fmt.Sprintf("%v", raw)
	}

	if format == "" {
		// No explicit format: comma-group, trimming to whole numbers when exact.
		if num == float64(int64(num)) {
			return humanize.Comma(int64(num))
		}
		return humanize.CommafWithDigits(num, 2)
	}

	return formatD3Number(format, num)
}

// formatD3Number renders val using the subset of d3-format specifiers dac
// dashboards rely on: an optional "$" currency prefix, "," group separators, a
// ".N" precision, and a type of f (fixed), % (percent), or d/"," (integer).
// Unrecognized specs fall back to a comma-grouped, two-decimal number.
func formatD3Number(format string, val float64) string {
	spec := format

	prefix := ""
	if strings.HasPrefix(spec, "$") {
		prefix = "$"
		spec = spec[1:]
	}

	group := strings.HasPrefix(spec, ",")
	if group {
		spec = spec[1:]
	}

	prec := -1
	if strings.HasPrefix(spec, ".") {
		rest := spec[1:]
		j := 0
		for j < len(rest) && rest[j] >= '0' && rest[j] <= '9' {
			j++
		}
		if j > 0 {
			prec, _ = strconv.Atoi(rest[:j])
		}
		spec = rest[j:]
	}

	var typ byte
	if len(spec) > 0 {
		typ = spec[len(spec)-1]
	}

	switch typ {
	case '%':
		if prec < 0 {
			prec = 0
		}
		s := strconv.FormatFloat(val*100, 'f', prec, 64)
		if group {
			s = groupInteger(s)
		}
		return prefix + s + "%"
	case 'd', ',':
		return prefix + groupInteger(strconv.FormatFloat(val, 'f', 0, 64))
	default: // 'f' and anything unrecognized
		if prec < 0 {
			prec = 2
		}
		s := strconv.FormatFloat(val, 'f', prec, 64)
		if group {
			s = groupInteger(s)
		}
		return prefix + s
	}
}

// groupInteger inserts thousands separators into the integer part of a numeric
// string, preserving any sign and fractional part.
func groupInteger(s string) string {
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}

	intPart, frac := s, ""
	if i := strings.IndexByte(s, '.'); i >= 0 {
		intPart, frac = s[:i], s[i:]
	}

	n := len(intPart)
	if n > 3 {
		var b strings.Builder
		pre := n % 3
		if pre > 0 {
			b.WriteString(intPart[:pre])
			b.WriteByte(',')
		}
		for i := pre; i < n; i += 3 {
			b.WriteString(intPart[i : i+3])
			if i+3 < n {
				b.WriteByte(',')
			}
		}
		intPart = b.String()
	}

	out := intPart + frac
	if neg {
		out = "-" + out
	}
	return out
}

// strftimeToGo converts a d3-time-format (strftime-style) string into a Go
// reference layout. Returns "" when nothing was converted.
func strftimeToGo(f string) string {
	out := strings.NewReplacer(
		"%Y", "2006", "%y", "06",
		"%m", "01", "%d", "02",
		"%H", "15", "%M", "04", "%S", "05",
		"%b", "Jan", "%B", "January",
		"%a", "Mon", "%A", "Monday",
		"%p", "PM",
	).Replace(f)
	if out == f {
		return ""
	}
	return out
}

// parseMetricTime coerces a query value into a time.Time for date-typed metrics.
func parseMetricTime(v any) (time.Time, bool) {
	switch t := v.(type) {
	case time.Time:
		return t, true
	case string:
		for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02", "2006-01"} {
			if parsed, err := time.Parse(layout, t); err == nil {
				return parsed, true
			}
		}
	}
	return time.Time{}, false
}

// toFloat64OK parses a query value as a float, reporting whether it is numeric.
func toFloat64OK(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case string:
		f, err := strconv.ParseFloat(n, 64)
		return f, err == nil
	default:
		return 0, false
	}
}
