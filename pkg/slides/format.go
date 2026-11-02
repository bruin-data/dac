package slides

import (
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/bruin-data/dac/pkg/dashboard"
	"golang.org/x/text/currency"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

// English separators and narrow symbols; fall back to the currency code.
// Compact output shortens axis ticks like the browser (₺1.5M) and ignores decimals.
func formatSemanticNumber(value float64, format dashboard.SemanticFormat, compact bool) (string, bool) {
	typ := format.Type
	switch {
	case typ == "" && format.Currency != "":
		typ = "currency"
	case typ == "" || typ == "currency" && format.Currency == "":
		typ = "decimal"
	}
	if typ != "currency" && typ != "number" && typ != "decimal" && typ != "percentage" {
		return "", false
	}
	decimals, symbol := 0, ""
	trim := format.Decimals == nil && (typ == "number" || typ == "decimal")
	if trim {
		decimals = 3
	}
	if typ == "currency" {
		decimals, symbol = 2, format.Currency
		if unit, err := currency.ParseISO(format.Currency); err == nil {
			symbol = message.NewPrinter(language.AmericanEnglish).Sprintf("%v", currency.NarrowSymbol(unit))
			decimals, _ = currency.Standard.Rounding(unit)
		}
	}
	if format.Decimals != nil {
		decimals = *format.Decimals
	}
	if decimals < 0 || decimals > 100 || math.IsNaN(value) || math.IsInf(value, 0) {
		text := strconv.FormatFloat(value, 'f', -1, 64)
		if typ == "currency" {
			text = format.Currency + " " + text
		}
		return text, true
	}
	var text string
	if compact {
		text = compactDecimal(value, typ == "percentage")
	} else {
		text = fixedDecimal(value, decimals, typ == "percentage")
		if trim {
			text = strings.TrimRight(strings.TrimRight(text, "0"), ".")
		}
	}
	if typ == "percentage" {
		return text + "%", true
	}
	if symbol != "" {
		// Space letter-based symbols (kr, CHF) from digits.
		if last, _ := utf8.DecodeLastRuneInString(symbol); !unicode.IsSymbol(last) && !unicode.IsSpace(last) {
			symbol += " "
		}
		if strings.HasPrefix(text, "-") {
			symbol, text = "-"+symbol, text[1:]
		}
		text = symbol + text
	}
	return text, true
}

// Match Intl rounding without float artifacts (1.005 -> 1.01).
func fixedDecimal(value float64, decimals int, percentage bool) string {
	r, _ := new(big.Rat).SetString(strconv.FormatFloat(math.Abs(value), 'f', -1, 64))
	if percentage {
		r.Mul(r, big.NewRat(100, 1))
	}
	text := r.FloatString(decimals)
	if math.Signbit(value) {
		text = "-" + text
	}
	return groupInteger(text)
}

// Match Intl's English compact notation with three significant digits
// (1.23K, 1.5M), rounding halves away from zero like fixedDecimal.
func compactDecimal(value float64, percentage bool) string {
	r, _ := new(big.Rat).SetString(strconv.FormatFloat(math.Abs(value), 'f', -1, 64))
	if percentage {
		r.Mul(r, big.NewRat(100, 1))
	}
	units := []string{"", "K", "M", "B", "T"}
	unit := 0
	thousand := big.NewRat(1000, 1)
	for unit < len(units)-1 && r.Cmp(thousand) >= 0 {
		r.Quo(r, thousand)
		unit++
	}
	// Keep two digits after the first significant one: 123, 12.3, 1.23, 0.00123.
	decimals := 0
	switch {
	case r.Sign() == 0 || r.Cmp(big.NewRat(100, 1)) >= 0:
	case r.Cmp(big.NewRat(10, 1)) >= 0:
		decimals = 1
	default:
		decimals = 2
		for scaled := new(big.Rat).Set(r); scaled.Cmp(big.NewRat(1, 1)) < 0; scaled.Mul(scaled, big.NewRat(10, 1)) {
			decimals++
		}
	}
	text := r.FloatString(decimals)
	// Rounding can reach the next unit: 999,999 is 1M, not 1000K.
	if text == "1000" && unit < len(units)-1 {
		text, unit = "1", unit+1
	}
	if strings.Contains(text, ".") {
		text = strings.TrimRight(strings.TrimRight(text, "0"), ".")
	}
	text = groupInteger(text) + units[unit]
	if math.Signbit(value) {
		text = "-" + text
	}
	return text
}

func formatTableValue(raw any, column dashboard.TableColumn) string {
	if raw == nil {
		return "—"
	}
	value, ok := toFloat64OK(raw)
	if !ok || math.IsNaN(value) || math.IsInf(value, 0) {
		return fmt.Sprint(raw)
	}
	spec, semantic := dashboard.ParseDisplayFormat(column.Number, column.Name)
	if semantic != nil {
		if text, ok := formatSemanticNumber(value, *semantic, false); ok {
			return text
		}
	}
	switch spec {
	case "currency":
		// Legacy dollars with 2–3 decimals.
		return "$" + strings.TrimSuffix(fixedDecimal(value, 3, false), "0")
	case "number":
		text := fixedDecimal(value, 3, false)
		return strings.TrimRight(strings.TrimRight(text, "0"), ".")
	case "":
		return fmt.Sprint(raw)
	default:
		return formatD3Number(spec, value)
	}
}
