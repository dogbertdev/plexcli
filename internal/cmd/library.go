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
}

type LibraryUpdateCmd struct {
	Section string `arg:"" optional:"" name:"section" help:"Library section ID or 'all' (default)" default:"all"`
	Output  string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type LibraryCleanCmd struct {
	Section string `arg:"" optional:"" name:"section" help:"Library section ID or 'all' (default)" default:"all"`
	Output  string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type LibraryActionResult struct {
	Action  string `json:"action"`
	ID      string `json:"id"`
	Title   string `json:"title"`
	Outcome string `json:"outcome"`
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
