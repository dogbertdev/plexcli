package cmd

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/alecthomas/kong"

	"github.com/dogbertdev/plexcli/internal/config"
	"github.com/dogbertdev/plexcli/internal/outfmt"
	"github.com/dogbertdev/plexcli/internal/plexclient"
	"github.com/dogbertdev/plexcli/internal/ui"
)

type LibraryCmd struct {
	List   LibrariesCmd     `cmd:"" help:"List library sections"`
	Update LibraryUpdateCmd `cmd:"" help:"Refresh one library section or all sections"`
	Clean  LibraryCleanCmd  `cmd:"" help:"Empty trash for one library section or all sections"`
	Status LibraryStatusCmd `cmd:"" help:"Show active server tasks and activities"`
}

type LibraryUpdateCmd struct {
	Section string `arg:"" optional:"" name:"section" help:"Library section ID or 'all' (default)" default:"all"`
	Output  string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type LibraryCleanCmd struct {
	Section string `arg:"" optional:"" name:"section" help:"Library section ID or 'all' (default)" default:"all"`
	Output  string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type LibraryStatusCmd struct {
	Output string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type LibraryActionResult struct {
	Action  string `json:"action"`
	ID      string `json:"id"`
	Title   string `json:"title"`
	Outcome string `json:"outcome"`
}

type LibraryStatusItem struct {
	Source      string `json:"source"`
	Type        string `json:"type"`
	Title       string `json:"title"`
	Progress    string `json:"progress"`
	Details     string `json:"details"`
	Cancellable bool   `json:"cancellable"`
}

func (c *LibraryUpdateCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	selector := strings.TrimSpace(c.Section)
	if selector == "" || strings.EqualFold(selector, "all") {
		cc, err := NewClientContext(cfg)
		if err != nil {
			return err
		}
		defer cc.Cancel()

		if err := cc.Client.RefreshAllSections(cc.Ctx); err != nil {
			return fmt.Errorf("failed to update all sections: %w", err)
		}

		results := []LibraryActionResult{
			{
				Action:  "update",
				ID:      "all",
				Title:   "All Sections",
				Outcome: "refresh requested",
			},
		}
		return outputLibraryActionResults(u.Out(), c.Output, results)
	}

	return runLibrarySectionAction(c.Section, c.Output, u, cfg, "update", "refresh requested", func(runCtx context.Context, client *plexclient.Client, sectionID string) error {
		return client.RefreshSection(runCtx, sectionID)
	})
}

func (c *LibraryCleanCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	cc, err := NewClientContext(cfg)
	if err != nil {
		return err
	}
	defer cc.Cancel()

	sections, err := cc.Client.GetSections(cc.Ctx)
	if err != nil {
		return fmt.Errorf("failed to get sections: %w", err)
	}

	targets, err := targetLibrarySections(sections, c.Section)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		fmt.Fprintln(u.Err(), "No libraries found")
		return nil
	}

	results := make([]LibraryActionResult, 0, len(targets)+3)
	for _, section := range targets {
		if err := cc.Client.EmptyTrash(cc.Ctx, section.ID); err != nil {
			return fmt.Errorf("failed to clean section %s: %w", section.ID, err)
		}

		results = append(results, LibraryActionResult{
			Action:  "clean",
			ID:      section.ID,
			Title:   sectionTitle(section),
			Outcome: "empty trash requested",
		})
	}

	if err := cc.Client.CleanBundles(cc.Ctx); err != nil {
		return fmt.Errorf("failed to clean bundles: %w", err)
	}
	results = append(results, LibraryActionResult{
		Action:  "clean",
		ID:      "-",
		Title:   "Global",
		Outcome: "clean bundles requested",
	})

	if err := cc.Client.DeleteCaches(cc.Ctx); err != nil {
		return fmt.Errorf("failed to delete caches: %w", err)
	}
	results = append(results, LibraryActionResult{
		Action:  "clean",
		ID:      "-",
		Title:   "Global",
		Outcome: "delete caches requested",
	})

	if err := cc.Client.OptimizeDatabase(cc.Ctx); err != nil {
		return fmt.Errorf("failed to optimize database: %w", err)
	}
	results = append(results, LibraryActionResult{
		Action:  "clean",
		ID:      "-",
		Title:   "Global",
		Outcome: "optimize database requested",
	})

	return outputLibraryActionResults(u.Out(), c.Output, results)
}

func (c *LibraryStatusCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	cc, err := NewClientContext(cfg)
	if err != nil {
		return err
	}
	defer cc.Cancel()

	activities, err := cc.Client.GetActivities(cc.Ctx)
	if err != nil {
		return fmt.Errorf("failed to get activities: %w", err)
	}

	backgroundTasks, err := cc.Client.GetBackgroundTasks(cc.Ctx)
	if err != nil {
		return fmt.Errorf("failed to get background tasks: %w", err)
	}

	items := make([]LibraryStatusItem, 0, len(activities)+len(backgroundTasks))
	for _, activity := range activities {
		items = append(items, LibraryStatusItem{
			Source:      "activity",
			Type:        defaultString(activity.Type, "unknown"),
			Title:       firstNonEmpty(activity.Title, activity.Subtitle, "-"),
			Progress:    formatTaskProgress(activity.Progress),
			Details:     defaultString(activity.Subtitle, "-"),
			Cancellable: activity.Cancellable,
		})
	}
	for _, task := range backgroundTasks {
		items = append(items, LibraryStatusItem{
			Source:      "background",
			Type:        defaultString(task.Type, "unknown"),
			Title:       defaultString(task.Title, "-"),
			Progress:    formatTaskProgress(task.Progress),
			Details:     formatBackgroundDetails(task.Remaining, task.Speed),
			Cancellable: false,
		})
	}

	return c.output(u.Out(), items)
}

func (c *LibraryStatusCmd) output(w io.Writer, items []LibraryStatusItem) error {
	formatter := outfmt.NewFormatter(outfmt.Format(c.Output))

	header := []string{"SOURCE", "TYPE", "TITLE", "PROGRESS", "DETAILS", "CANCELLABLE"}
	rows := make([][]string, 0, len(items))
	for _, item := range items {
		rows = append(rows, []string{
			item.Source,
			item.Type,
			item.Title,
			item.Progress,
			item.Details,
			fmt.Sprintf("%t", item.Cancellable),
		})
	}

	return formatter.Format(w, header, rows, items)
}

func runLibrarySectionAction(
	sectionSelector string,
	output string,
	u *ui.UI,
	cfg *config.Config,
	action string,
	outcome string,
	runAction func(ctx context.Context, client *plexclient.Client, sectionID string) error,
) error {
	cc, err := NewClientContext(cfg)
	if err != nil {
		return err
	}
	defer cc.Cancel()

	sections, err := cc.Client.GetSections(cc.Ctx)
	if err != nil {
		return fmt.Errorf("failed to get sections: %w", err)
	}

	targets, err := targetLibrarySections(sections, sectionSelector)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		fmt.Fprintln(u.Err(), "No libraries found")
		return nil
	}

	results := make([]LibraryActionResult, 0, len(targets))
	for _, section := range targets {
		if err := runAction(cc.Ctx, cc.Client, section.ID); err != nil {
			return fmt.Errorf("failed to %s section %s: %w", action, section.ID, err)
		}

		results = append(results, LibraryActionResult{
			Action:  action,
			ID:      section.ID,
			Title:   sectionTitle(section),
			Outcome: outcome,
		})
	}

	return outputLibraryActionResults(u.Out(), output, results)
}

func formatTaskProgress(progress *float64) string {
	if progress == nil {
		return "-"
	}
	if *progress < 0 {
		return "indeterminate"
	}
	return fmt.Sprintf("%.0f%%", *progress)
}

func formatBackgroundDetails(remaining *int64, speed *float64) string {
	parts := make([]string, 0, 2)
	if remaining != nil {
		parts = append(parts, fmt.Sprintf("remaining=%ds", *remaining))
	}
	if speed != nil {
		parts = append(parts, fmt.Sprintf("speed=%.2fx", *speed))
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, ", ")
}

func defaultString(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func targetLibrarySections(sections []plexclient.Library, sectionSelector string) ([]plexclient.Library, error) {
	selector := strings.TrimSpace(sectionSelector)
	if selector == "" || strings.EqualFold(selector, "all") {
		return sections, nil
	}

	for _, section := range sections {
		if section.ID == selector {
			return []plexclient.Library{section}, nil
		}
	}

	return nil, fmt.Errorf("section %q not found", selector)
}

func sectionTitle(section plexclient.Library) string {
	if section.Title != nil && *section.Title != "" {
		return *section.Title
	}
	return "Unknown"
}

func outputLibraryActionResults(w io.Writer, output string, results []LibraryActionResult) error {
	formatter := outfmt.NewFormatter(outfmt.Format(output))

	header := []string{"ACTION", "ID", "TITLE", "OUTCOME"}
	rows := make([][]string, 0, len(results))
	for _, result := range results {
		rows = append(rows, []string{result.Action, result.ID, result.Title, result.Outcome})
	}

	return formatter.Format(w, header, rows, results)
}
