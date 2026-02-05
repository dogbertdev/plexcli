package outfmt

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/olekukonko/tablewriter"
)

var formatContextKey = struct{}{}

type Format string

const (
	Table Format = "table"
	JSON  Format = "json"
	TSV   Format = "tsv"
)

type Formatter interface {
	Format(w io.Writer, header []string, rows [][]string, data interface{}) error
}

type TableFormatter struct{}

func (f *TableFormatter) Format(w io.Writer, header []string, rows [][]string, data interface{}) error {
	table := tablewriter.NewWriter(w)
	table.Header(header)
	for _, row := range rows {
		table.Append(row)
	}
	return table.Render()
}

type JSONFormatter struct{}

func (f *JSONFormatter) Format(w io.Writer, header []string, rows [][]string, data interface{}) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(data)
}

type TSVFormatter struct{}

func (f *TSVFormatter) Format(w io.Writer, header []string, rows [][]string, data interface{}) error {
	for _, row := range rows {
		fmt.Fprintln(w, strings.Join(row, "\t"))
	}
	return nil
}

func NewFormatter(f Format) Formatter {
	switch f {
	case JSON:
		return &JSONFormatter{}
	case TSV:
		return &TSVFormatter{}
	case Table:
		fallthrough
	default:
		return &TableFormatter{}
	}
}

func WithMode(ctx context.Context, format Format) context.Context {
	return context.WithValue(ctx, formatContextKey, format)
}

func FromContext(ctx context.Context) Format {
	if f, ok := ctx.Value(formatContextKey).(Format); ok {
		return f
	}
	return Table
}
