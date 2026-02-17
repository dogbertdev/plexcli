package cmd

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/LukeHagar/plexgo/models/components"
	"github.com/alecthomas/kong"

	"github.com/dogbertdev/plexcli/internal/config"
	"github.com/dogbertdev/plexcli/internal/outfmt"
	"github.com/dogbertdev/plexcli/internal/ui"
)

// DuplicatesCmd represents the duplicates command
type DuplicatesCmd struct {
	SectionID             string `help:"Library section ID to scan (empty = all sections)" default:""`
	Type                  string `help:"Filter by type: movie, episode, or all" default:"all" enum:"movie,episode,all"`
	MinCount              int    `help:"Minimum number of duplicates to report" default:"2"`
	Output                string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
	EditionsAreDuplicates bool   `help:"Treat different editions (Director's Cut, etc.) as duplicates" default:"false"`
}

// DuplicateGroup represents a group of duplicate items
type DuplicateGroup struct {
	Key        string   `json:"key"`
	Title      string   `json:"title"`
	Year       int      `json:"year,omitempty"`
	Show       string   `json:"show,omitempty"`
	Edition    string   `json:"edition,omitempty"`
	Type       string   `json:"type"`
	Count      int      `json:"count"`
	RatingKeys []string `json:"rating_keys"`
}

// Run executes the duplicates command
func (c *DuplicatesCmd) Run(ctx *kong.Context, ui *ui.UI, cfg *config.Config) error {
	cc, err := NewClientContext(cfg)
	if err != nil {
		return err
	}
	defer cc.Cancel()

	var items []*components.Metadata
	if c.SectionID != "" {
		items, err = cc.Client.GetAllLibraryItems(cc.Ctx, c.SectionID)
		if err != nil {
			return fmt.Errorf("failed to fetch library items: %w", err)
		}
	} else {
		sections, err := cc.Client.GetSections(cc.Ctx)
		if err != nil {
			return fmt.Errorf("failed to get sections: %w", err)
		}
		for _, section := range sections {
			sectionItems, err := cc.Client.GetAllLibraryItems(cc.Ctx, section.ID)
			if err != nil {
				return fmt.Errorf("failed to get items from section %s: %w", section.ID, err)
			}
			items = append(items, sectionItems...)
		}
	}

	duplicates := c.findDuplicates(items)

	var filtered []DuplicateGroup
	for _, d := range duplicates {
		if d.Count >= c.MinCount {
			filtered = append(filtered, d)
		}
	}

	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].Count != filtered[j].Count {
			return filtered[i].Count > filtered[j].Count
		}
		return filtered[i].Title < filtered[j].Title
	})

	return c.outputResults(ui.Out(), filtered)
}

func (c *DuplicatesCmd) findDuplicates(items []*components.Metadata) []DuplicateGroup {
	// Group items by key
	groups := make(map[string]*DuplicateGroup)

	for _, item := range items {
		if item == nil {
			continue
		}

		// Filter by type
		itemType := anyToString(item.GetType())
		if c.Type != "all" && itemType != c.Type {
			continue
		}

		// Generate grouping key
		key := c.generateKey(item)
		if key == "" {
			continue
		}

		// Get or create group
		group, exists := groups[key]
		if !exists {
			group = &DuplicateGroup{
				Key:        key,
				Title:      anyToString(item.GetTitle()),
				Type:       itemType,
				RatingKeys: []string{},
			}

			// Add year for movies
			if year := item.GetYear(); year != nil {
				group.Year = int(*year)
			}

			// Add show for episodes
			if itemType == "episode" {
				if show := item.GetGrandparentTitle(); show != nil {
					group.Show = *show
				}
			}

			if itemType == "movie" {
				group.Edition = c.getEditionTitle(item)
			}

			groups[key] = group
		}

		// Add rating key to group
		ratingKey := anyToString(item.GetRatingKey())
		if ratingKey != "" {
			group.RatingKeys = append(group.RatingKeys, ratingKey)
			group.Count = len(group.RatingKeys)
		}
	}

	// Convert map to slice (only include groups with duplicates)
	var result []DuplicateGroup
	for _, group := range groups {
		if group.Count >= c.MinCount {
			result = append(result, *group)
		}
	}

	return result
}

func (c *DuplicatesCmd) generateKey(item *components.Metadata) string {
	if item == nil {
		return ""
	}

	title := anyToString(item.GetTitle())
	if title == "" {
		return ""
	}

	itemType := anyToString(item.GetType())

	switch itemType {
	case "movie":
		// Group by title + year
		year := 0
		if y := item.GetYear(); y != nil {
			year = int(*y)
		}

		// When EditionsAreDuplicates is false (default), include edition in key
		// so different editions are NOT considered duplicates
		if !c.EditionsAreDuplicates {
			edition := c.getEditionTitle(item)
			if edition != "" {
				return fmt.Sprintf("movie:%s:%d:%s", strings.ToLower(title), year, strings.ToLower(edition))
			}
		}
		return fmt.Sprintf("movie:%s:%d", strings.ToLower(title), year)

	case "episode":
		// Group by show + title (episode name)
		show := ""
		if s := item.GetGrandparentTitle(); s != nil {
			show = *s
		}
		return fmt.Sprintf("episode:%s:%s", strings.ToLower(show), strings.ToLower(title))

	default:
		// For other types, group by title only
		return fmt.Sprintf("%s:%s", itemType, strings.ToLower(title))
	}
}

func (c *DuplicatesCmd) getEditionTitle(item *components.Metadata) string {
	if item == nil {
		return ""
	}

	props := item.GetAdditionalProperties()
	if props == nil {
		return ""
	}

	if edition, ok := props["editionTitle"]; ok {
		if editionStr, ok := edition.(string); ok {
			return editionStr
		}
	}
	return ""
}

// outputResults outputs the results in the specified format
func (c *DuplicatesCmd) outputResults(w io.Writer, groups []DuplicateGroup) error {
	formatter := outfmt.NewFormatter(outfmt.Format(c.Output))

	header := []string{"Title", "Year", "Edition", "Show", "Type", "Count", "Rating Keys"}
	rows := make([][]string, len(groups))

	for i, group := range groups {
		yearStr := ""
		if group.Year > 0 {
			yearStr = strconv.Itoa(group.Year)
		}

		showStr := group.Show
		if showStr == "" && group.Type == "episode" {
			showStr = "-"
		}

		editionStr := group.Edition
		if editionStr == "" {
			editionStr = "-"
		}

		ratingKeysStr := strings.Join(group.RatingKeys, ", ")

		rows[i] = []string{
			group.Title,
			yearStr,
			editionStr,
			showStr,
			group.Type,
			strconv.Itoa(group.Count),
			ratingKeysStr,
		}
	}

	return formatter.Format(w, header, rows, groups)
}
