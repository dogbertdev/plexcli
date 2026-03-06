package cmd

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"github.com/LukeHagar/plexgo/models/components"
	"github.com/alecthomas/kong"

	"github.com/dogbertdev/plexcli/internal/config"
	"github.com/dogbertdev/plexcli/internal/outfmt"
	"github.com/dogbertdev/plexcli/internal/plexclient"
	"github.com/dogbertdev/plexcli/internal/ui"
)

type EditionsCmd struct {
	Section string `help:"Library section ID to check" short:"s"`
	Title   string `help:"Filter by title" short:"t"`
	Issues  bool   `help:"Only show items with edition issues" short:"i"`
	Output  string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type EditionInfo struct {
	Title           string `json:"title"`
	Year            int    `json:"year,omitempty"`
	RatingKey       string `json:"rating_key"`
	Section         string `json:"section"`
	EditionTitle    string `json:"edition_title,omitempty"`
	FileEdition     string `json:"file_edition,omitempty"`
	RawEditionTag   string `json:"raw_edition_tag,omitempty"`
	FilePath        string `json:"file_path"`
	HasMetaEdition  bool   `json:"has_meta_edition"`
	HasFileEdition  bool   `json:"has_file_edition"`
	EditionMismatch bool   `json:"edition_mismatch,omitempty"`
	Issue           string `json:"issue,omitempty"`
}

type editionItemWithSection struct {
	item    *components.Metadata
	section string
}

var editionRegex = regexp.MustCompile(`\{[Ee]dition[-\s]?([^}]+)\}`)
var rawEditionTagRegex = regexp.MustCompile(`(\{[Ee]dition[-\s]?[^}]+\})`)

func (c *EditionsCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	cc, err := NewClientContext(cfg)
	if err != nil {
		return err
	}
	defer cc.Cancel()

	items, err := c.fetchItems(cc.Ctx, cc.Client)
	if err != nil {
		return err
	}

	editions := c.extractEditions(items)

	if c.Issues {
		editions = c.filterIssues(editions)
	}

	if len(editions) == 0 {
		if c.Issues {
			fmt.Fprintln(u.Err(), "No edition issues found")
		} else {
			fmt.Fprintln(u.Err(), "No editions found")
		}
		return nil
	}

	return c.outputResults(u.Out(), editions)
}

func (c *EditionsCmd) fetchItems(ctx context.Context, client *plexclient.Client) ([]editionItemWithSection, error) {
	var items []editionItemWithSection

	sections, err := client.GetSections(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch sections: %w", err)
	}

	for _, section := range sections {
		if c.Section != "" && section.ID != c.Section {
			continue
		}

		// Only check movie libraries
		if section.Type != "movie" {
			continue
		}

		sectionName := ""
		if section.Title != nil {
			sectionName = *section.Title
		}

		sectionItems, err := client.GetAllLibraryItems(ctx, section.ID)
		if err != nil {
			continue
		}

		for _, item := range sectionItems {
			if c.Title != "" && !strings.Contains(strings.ToLower(anyToString(item.Title)), strings.ToLower(c.Title)) {
				continue
			}
			items = append(items, editionItemWithSection{item: item, section: sectionName})
		}
	}

	return items, nil
}

func (c *EditionsCmd) extractEditions(items []editionItemWithSection) []EditionInfo {
	var editions []EditionInfo

	for _, iws := range items {
		item := iws.item
		if item.Media == nil {
			continue
		}

		for _, media := range item.Media {
			if media.Part == nil {
				continue
			}

			for _, part := range media.Part {
				filePath := anyToString(part.File)
				if filePath == "" {
					continue
				}

				// Extract edition from file path
				fileEdition := ""
				rawTag := ""
				if matches := editionRegex.FindStringSubmatch(filePath); len(matches) > 1 {
					fileEdition = strings.TrimSpace(matches[1])
				}
				if matches := rawEditionTagRegex.FindStringSubmatch(filePath); len(matches) > 1 {
					rawTag = matches[1]
				}

				// Get edition from metadata
				metaEdition := ""
				if item.AdditionalProperties != nil {
					if ed, ok := item.AdditionalProperties["editionTitle"]; ok {
						metaEdition = anyToString(ed)
					}
				}

				// Skip if no edition info at all
				if fileEdition == "" && metaEdition == "" {
					continue
				}

				info := EditionInfo{
					Title:          anyToString(item.Title),
					RatingKey:      anyToString(item.RatingKey),
					Section:        iws.section,
					EditionTitle:   metaEdition,
					FileEdition:    fileEdition,
					RawEditionTag:  rawTag,
					FilePath:       filePath,
					HasMetaEdition: metaEdition != "",
					HasFileEdition: fileEdition != "",
				}

				if item.Year != nil {
					info.Year = *item.Year
				}

				// Check for mismatch
				if fileEdition != "" && metaEdition != "" {
					// Normalize for comparison
					normalizedFile := strings.ToLower(strings.ReplaceAll(fileEdition, " ", ""))
					normalizedMeta := strings.ToLower(strings.ReplaceAll(metaEdition, " ", ""))
					info.EditionMismatch = normalizedFile != normalizedMeta
				}

				// Diagnose issues
				info.Issue = diagnoseEditionIssue(rawTag, fileEdition, metaEdition, info.EditionMismatch)

				editions = append(editions, info)
			}
		}
	}

	// Sort by title, then edition
	sort.Slice(editions, func(i, j int) bool {
		if editions[i].Title != editions[j].Title {
			return editions[i].Title < editions[j].Title
		}
		return editions[i].FileEdition < editions[j].FileEdition
	})

	return editions
}

func (c *EditionsCmd) filterIssues(editions []EditionInfo) []EditionInfo {
	var issues []EditionInfo
	for _, e := range editions {
		if e.Issue != "" {
			issues = append(issues, e)
		}
	}
	return issues
}

// diagnoseEditionIssue analyzes the edition tag and returns a human-readable explanation
func diagnoseEditionIssue(rawTag, fileEdition, metaEdition string, mismatch bool) string {
	if fileEdition == "" && metaEdition != "" {
		return "" // No issue - metadata edition without file tag is fine
	}

	if fileEdition != "" && metaEdition != "" && !mismatch {
		return "" // No issue - both match
	}

	if mismatch {
		return fmt.Sprintf("Mismatch: file has '%s' but metadata has '%s'", fileEdition, metaEdition)
	}

	// File has edition but metadata doesn't - diagnose why
	if rawTag == "" {
		return "Edition tag not found in filename"
	}

	// Check for common format issues
	issues := []string{}

	// Check for correct format: {edition-Name}
	correctFormat := regexp.MustCompile(`^\{edition-[^}]+\}$`)
	if !correctFormat.MatchString(rawTag) {
		// Diagnose specific issues
		if strings.Contains(rawTag, "Edition") {
			issues = append(issues, "use lowercase 'edition'")
		}
		if strings.Contains(rawTag, "{edition ") || strings.Contains(rawTag, "{Edition ") {
			issues = append(issues, "use dash not space after 'edition'")
		}
		if strings.Contains(rawTag, "{edition--") || strings.Contains(rawTag, "{Edition--") {
			issues = append(issues, "remove extra dash")
		}
		if strings.HasPrefix(rawTag, "-{") || strings.HasSuffix(rawTag, "}-") {
			issues = append(issues, "edition tag should not have adjacent dashes")
		}
	}

	if len(issues) > 0 {
		return fmt.Sprintf("Bad format %s: %s. Use {edition-Name}", rawTag, strings.Join(issues, ", "))
	}

	// Format looks OK but still not recognized
	return fmt.Sprintf("Tag %s not recognized - try refreshing metadata or re-scanning", rawTag)
}

func (c *EditionsCmd) outputResults(w io.Writer, editions []EditionInfo) error {
	formatter := outfmt.NewFormatter(outfmt.Format(c.Output))

	header := []string{"TITLE", "YEAR", "SECTION", "META EDITION", "FILE EDITION", "ISSUE"}
	rows := make([][]string, len(editions))

	for i, e := range editions {
		yearStr := ""
		if e.Year > 0 {
			yearStr = fmt.Sprintf("%d", e.Year)
		}

		rows[i] = []string{
			e.Title,
			yearStr,
			e.Section,
			e.EditionTitle,
			e.FileEdition,
			e.Issue,
		}
	}

	return formatter.Format(w, header, rows, editions)
}
