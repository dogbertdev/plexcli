package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/dogbertdev/plexcli/internal/config"
	"github.com/dogbertdev/plexcli/internal/outfmt"
	"github.com/dogbertdev/plexcli/internal/plexclient"
	"github.com/dogbertdev/plexcli/internal/ui"
)

type BinaryDownloadItem struct {
	Action      string `json:"action"`
	Target      string `json:"target"`
	OutputPath  string `json:"output_path"`
	ContentType string `json:"content_type"`
	Bytes       int64  `json:"bytes"`
}

func parseKeyValueFlags(values []string) (map[string]string, error) {
	if len(values) == 0 {
		return map[string]string{}, nil
	}
	return plexclient.ParseKeyValuePairs(values)
}

func requireConfirmed(yes bool, action string) error {
	return plexclient.RequireConfirmation(yes, action)
}

func requireOutputPath(output string) error {
	return plexclient.RequireOutputPath(output)
}

func outputBinaryResult(w io.Writer, format string, result *plexclient.LibraryBinaryResult) error {
	if result == nil {
		return fmt.Errorf("binary result is required")
	}

	item := BinaryDownloadItem{
		Action:      result.Action,
		Target:      result.Target,
		OutputPath:  result.OutputPath,
		ContentType: result.ContentType,
		Bytes:       result.Bytes,
	}

	header := []string{"ACTION", "TARGET", "OUTPUT", "CONTENT TYPE", "BYTES"}
	rows := [][]string{{
		item.Action,
		item.Target,
		item.OutputPath,
		item.ContentType,
		strconv.FormatInt(item.Bytes, 10),
	}}

	return outfmt.NewFormatter(outfmt.Format(format)).Format(w, header, rows, []BinaryDownloadItem{item})
}

func outputGenericPayload(w io.Writer, format string, data any) error {
	formatter := outfmt.NewFormatter(outfmt.Format(format))
	header, rows := genericRows(data)
	return formatter.Format(w, header, rows, data)
}

func genericRows(data any) ([]string, [][]string) {
	records := extractRecords(data)
	if len(records) == 0 {
		return []string{"VALUE"}, [][]string{}
	}

	keys := orderedKeys(records)
	rows := make([][]string, 0, len(records))
	for _, record := range records {
		row := make([]string, 0, len(keys))
		for _, key := range keys {
			row = append(row, record[key])
		}
		rows = append(rows, row)
	}

	header := make([]string, 0, len(keys))
	for _, key := range keys {
		header = append(header, strings.ToUpper(strings.ReplaceAll(key, "_", " ")))
	}
	return header, rows
}

func extractRecords(data any) []map[string]string {
	switch v := data.(type) {
	case map[string]any:
		if container, ok := v["MediaContainer"]; ok {
			return extractRecords(container)
		}
		for _, key := range []string{"Metadata", "Directory", "Setting", "Media", "Part", "Stream", "Marker"} {
			if list, ok := v[key]; ok {
				return extractRecords(list)
			}
		}
		return []map[string]string{flattenRecord(v)}
	case []any:
		out := make([]map[string]string, 0, len(v))
		for _, item := range v {
			switch entry := item.(type) {
			case map[string]any:
				out = append(out, flattenRecord(entry))
			default:
				out = append(out, map[string]string{"value": compactJSON(entry)})
			}
		}
		return out
	default:
		return []map[string]string{{"value": compactJSON(v)}}
	}
}

func flattenRecord(record map[string]any) map[string]string {
	if len(record) == 0 {
		return map[string]string{"value": "{}"}
	}

	ordered := []string{
		"ratingKey", "key", "id", "tag", "title", "type", "year", "index",
		"parentTitle", "grandparentTitle", "summary", "value", "default",
		"label", "group", "filter", "thumb", "art", "updatedAt",
	}
	flat := map[string]string{}
	for _, key := range ordered {
		if value, ok := record[key]; ok {
			flat[normalizeKey(key)] = stringifyValue(value)
		}
	}

	if len(flat) == 0 {
		flat["value"] = compactJSON(record)
	}
	return flat
}

func orderedKeys(records []map[string]string) []string {
	priority := []string{
		"rating_key", "key", "id", "tag", "title", "type", "year", "index",
		"parent_title", "grandparent_title", "label", "value", "default",
		"summary", "filter", "thumb", "art", "updated_at",
	}

	seen := map[string]struct{}{}
	keys := make([]string, 0)
	for _, key := range priority {
		for _, record := range records {
			if _, ok := record[key]; ok {
				if _, exists := seen[key]; !exists {
					keys = append(keys, key)
					seen[key] = struct{}{}
				}
				break
			}
		}
	}

	extras := make([]string, 0)
	for _, record := range records {
		for key := range record {
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			extras = append(extras, key)
		}
	}
	sort.Strings(extras)
	return append(keys, extras...)
}

func normalizeKey(key string) string {
	var builder strings.Builder
	for i, r := range key {
		if i > 0 && r >= 'A' && r <= 'Z' {
			builder.WriteByte('_')
		}
		builder.WriteRune(r)
	}
	return strings.ToLower(builder.String())
}

func stringifyValue(v any) string {
	switch value := v.(type) {
	case nil:
		return ""
	case string:
		return value
	case bool:
		return strconv.FormatBool(value)
	case float64:
		if value == float64(int64(value)) {
			return strconv.FormatInt(int64(value), 10)
		}
		return strconv.FormatFloat(value, 'f', -1, 64)
	case []any:
		parts := make([]string, 0, len(value))
		for _, item := range value {
			parts = append(parts, stringifyValue(item))
		}
		return strings.Join(parts, ", ")
	case map[string]any:
		return compactJSON(value)
	default:
		return fmt.Sprintf("%v", value)
	}
}

func compactJSON(v any) string {
	body, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(body)
}

func encodeCSVArg(input string) (string, error) {
	return plexclient.EncodeMetadataInput(input)
}

func runLibraryJSONCommand(u *ui.UI, cfg *config.Config, output string, op, path string, query url.Values) error {
	cc, err := NewClientContext(cfg)
	if err != nil {
		return err
	}
	defer cc.Cancel()

	payload, err := cc.Client.LibraryJSON(cc.Ctx, op, http.MethodGet, path, query)
	if err != nil {
		return err
	}
	return outputGenericPayload(u.Out(), output, payload)
}

func runLibraryMutationSummary(u *ui.UI, output string, summary plexclient.LibraryMutationSummary) error {
	results := []LibraryActionResult{{
		Action:  summary.Action,
		ID:      summary.Target,
		Title:   summary.Target,
		Outcome: summary.Outcome,
	}}
	return outputLibraryActionResults(u.Out(), output, results)
}

func sectionTypeIDForLibrary(ctx context.Context, client *plexclient.Client, sectionID string) (int64, error) {
	sections, err := client.GetSections(ctx)
	if err != nil {
		return 0, fmt.Errorf("resolve section %s type: %w", sectionID, err)
	}

	for _, section := range sections {
		if section.ID == sectionID {
			sectionTypeID, ok := librarySectionTypeIDValue(section.Type)
			if !ok {
				return 0, fmt.Errorf("resolve section %s type: unsupported section type %q", sectionID, section.Type)
			}
			return sectionTypeID, nil
		}
	}

	return 0, fmt.Errorf("resolve section %s type: section not found", sectionID)
}
