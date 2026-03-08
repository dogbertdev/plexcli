package cmd

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/alecthomas/kong"

	"github.com/dogbertdev/plexcli/internal/config"
	"github.com/dogbertdev/plexcli/internal/plexclient"
	"github.com/dogbertdev/plexcli/internal/ui"
)

type LibrarySectionCmd struct {
	Show    LibrarySectionShowCmd    `cmd:"" help:"Show details for a library section"`
	Create  LibrarySectionCreateCmd  `cmd:"" help:"Create a new library section"`
	Edit    LibrarySectionEditCmd    `cmd:"" help:"Edit a library section"`
	Delete  LibrarySectionDeleteCmd  `cmd:"" help:"Delete a library section"`
	Prefs   LibrarySectionPrefsCmd   `cmd:"" help:"View or update section preferences"`
	Filters LibrarySectionFiltersCmd `cmd:"" help:"List available filters for a section"`
	Sorts   LibrarySectionSortsCmd   `cmd:"" help:"List available sorts for a section"`
	Letters LibrarySectionLettersCmd `cmd:"" help:"List first-character buckets for a section"`
	Image   LibrarySectionImageCmd   `cmd:"" help:"Download the section composite image"`
}

type LibrarySectionShowCmd struct {
	Section string `arg:"" name:"section" help:"Library section ID"`
	Output  string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type LibrarySectionCreateCmd struct {
	Name      string   `arg:"" help:"Library section name"`
	Type      string   `help:"Library type: movie, show, music, photo" required:"" enum:"movie,show,music,photo"`
	Agent     string   `help:"Metadata agent identifier" required:""`
	Scanner   string   `help:"Scanner identifier"`
	Language  string   `help:"Section language (for example en-US)" default:"en-US"`
	Locations []string `help:"Library location on disk" required:"" name:"location"`
	Prefs     []string `help:"Preference override in key=value form" name:"pref"`
	Output    string   `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type LibrarySectionEditCmd struct {
	Section   string   `arg:"" name:"section" help:"Library section ID"`
	Name      string   `help:"Updated section name"`
	Agent     string   `help:"Updated metadata agent identifier"`
	Scanner   string   `help:"Updated scanner identifier"`
	Language  string   `help:"Updated section language"`
	Locations []string `help:"Updated library locations" name:"location"`
	Prefs     []string `help:"Preference override in key=value form" name:"pref"`
	Output    string   `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type LibrarySectionDeleteCmd struct {
	Section string `arg:"" name:"section" help:"Library section ID"`
	Yes     bool   `help:"Confirm deletion" name:"yes"`
	Output  string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type LibrarySectionPrefsCmd struct {
	List     LibrarySectionPrefsListCmd     `cmd:"" help:"List section preferences"`
	Set      LibrarySectionPrefsSetCmd      `cmd:"" help:"Set section preferences"`
	Defaults LibrarySectionPrefsDefaultsCmd `cmd:"" help:"List default prefs for a metadata type"`
}

type LibrarySectionPrefsListCmd struct {
	Section string `arg:"" name:"section" help:"Library section ID"`
	Output  string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type LibrarySectionPrefsSetCmd struct {
	Section string   `arg:"" name:"section" help:"Library section ID"`
	Prefs   []string `help:"Preference override in key=value form" name:"pref" required:""`
	Output  string   `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type LibrarySectionPrefsDefaultsCmd struct {
	Type   string `help:"Metadata type: movie, show, music, photo" required:"" enum:"movie,show,music,photo"`
	Agent  string `help:"Metadata agent identifier"`
	Output string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type LibrarySectionFiltersCmd struct {
	Section string `arg:"" name:"section" help:"Library section ID"`
	Output  string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type LibrarySectionSortsCmd struct {
	Section string `arg:"" name:"section" help:"Library section ID"`
	Output  string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type LibrarySectionLettersCmd struct {
	Section string `arg:"" name:"section" help:"Library section ID"`
	Output  string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type LibrarySectionImageCmd struct {
	Section   string `arg:"" name:"section" help:"Library section ID"`
	UpdatedAt int64  `help:"Section updated-at timestamp used for cache busting" default:"0"`
	Output    string `help:"Destination file path" type:"path" required:""`
}

type LibraryRefreshCmd struct {
	Cancel  LibraryRefreshCancelCmd  `cmd:"" help:"Cancel an active section refresh"`
	StopAll LibraryRefreshStopAllCmd `cmd:"" name:"stop-all" help:"Stop all active refreshes"`
}

type LibraryRefreshCancelCmd struct {
	Section string `arg:"" name:"section" help:"Library section ID"`
	Output  string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type LibraryRefreshStopAllCmd struct {
	Yes    bool   `help:"Confirm stopping all refreshes" name:"yes"`
	Output string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

func (c *LibrarySectionShowCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	return runLibraryJSONCommand(u, cfg, c.Output, "GetLibraryDetails", fmt.Sprintf("library/sections/%s", url.PathEscape(c.Section)), nil)
}

func (c *LibrarySectionCreateCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	prefs, err := parseKeyValueFlags(c.Prefs)
	if err != nil {
		return err
	}

	cc, err := NewClientContext(cfg)
	if err != nil {
		return err
	}
	defer cc.Cancel()

	if err := cc.Client.CreateSection(cc.Ctx, plexclient.SectionMutationInput{
		Name:      c.Name,
		Type:      librarySectionTypeID(c.Type),
		Agent:     c.Agent,
		Scanner:   c.Scanner,
		Language:  c.Language,
		Locations: c.Locations,
		Prefs:     prefs,
	}); err != nil {
		return err
	}

	return runLibraryMutationSummary(u, c.Output, plexclient.LibraryMutationSummary{
		Action:  "section-create",
		Target:  c.Name,
		Outcome: "section created",
	})
}

func (c *LibrarySectionEditCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	prefs, err := parseKeyValueFlags(c.Prefs)
	if err != nil {
		return err
	}
	if c.Name == "" && c.Agent == "" && c.Scanner == "" && c.Language == "" && len(c.Locations) == 0 && len(prefs) == 0 {
		return fmt.Errorf("provide at least one change flag")
	}

	cc, err := NewClientContext(cfg)
	if err != nil {
		return err
	}
	defer cc.Cancel()

	if err := cc.Client.EditSection(cc.Ctx, c.Section, plexclient.SectionMutationInput{
		Name:      c.Name,
		Agent:     c.Agent,
		Scanner:   c.Scanner,
		Language:  c.Language,
		Locations: c.Locations,
		Prefs:     prefs,
	}); err != nil {
		return err
	}

	return runLibraryMutationSummary(u, c.Output, plexclient.LibraryMutationSummary{
		Action:  "section-edit",
		Target:  c.Section,
		Outcome: "section updated",
	})
}

func (c *LibrarySectionDeleteCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	if err := c.validate(); err != nil {
		return err
	}

	cc, err := NewClientContext(cfg)
	if err != nil {
		return err
	}
	defer cc.Cancel()

	if err := cc.Client.DeleteSection(cc.Ctx, c.Section); err != nil {
		return err
	}

	return runLibraryMutationSummary(u, c.Output, plexclient.LibraryMutationSummary{
		Action:  "section-delete",
		Target:  c.Section,
		Outcome: "section deleted",
	})
}

func (c *LibrarySectionPrefsListCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	return runLibraryJSONCommand(u, cfg, c.Output, "GetSectionPreferences", fmt.Sprintf("library/sections/%s/prefs", url.PathEscape(c.Section)), nil)
}

func (c *LibrarySectionPrefsSetCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	prefs, err := parseKeyValueFlags(c.Prefs)
	if err != nil {
		return err
	}

	cc, err := NewClientContext(cfg)
	if err != nil {
		return err
	}
	defer cc.Cancel()

	if err := cc.Client.SetSectionPreferencesDynamic(cc.Ctx, c.Section, prefs); err != nil {
		return err
	}

	return runLibraryMutationSummary(u, c.Output, plexclient.LibraryMutationSummary{
		Action:  "section-prefs-set",
		Target:  c.Section,
		Outcome: "preferences updated",
	})
}

func (c *LibrarySectionPrefsDefaultsCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	cc, err := NewClientContext(cfg)
	if err != nil {
		return err
	}
	defer cc.Cancel()

	payload, err := cc.Client.GetSectionsPrefs(cc.Ctx, librarySectionTypeID(c.Type), c.Agent)
	if err != nil {
		return err
	}
	return outputGenericPayload(u.Out(), c.Output, payload)
}

func (c *LibrarySectionFiltersCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	return runLibraryJSONCommand(u, cfg, c.Output, "GetSectionFilters", fmt.Sprintf("library/sections/%s/filters", url.PathEscape(c.Section)), nil)
}

func (c *LibrarySectionSortsCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	return runLibraryJSONCommand(u, cfg, c.Output, "GetAvailableSorts", fmt.Sprintf("library/sections/%s/sorts", url.PathEscape(c.Section)), nil)
}

func (c *LibrarySectionLettersCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	return runLibraryJSONCommand(u, cfg, c.Output, "GetFirstCharacters", fmt.Sprintf("library/sections/%s/firstCharacter", url.PathEscape(c.Section)), nil)
}

func (c *LibrarySectionImageCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	if err := c.validate(); err != nil {
		return err
	}

	cc, err := NewClientContext(cfg)
	if err != nil {
		return err
	}
	defer cc.Cancel()

	result, err := cc.Client.LibraryDownload(
		cc.Ctx,
		"GetSectionImage",
		"GET",
		fmt.Sprintf("library/sections/%s/composite/%d", url.PathEscape(c.Section), c.UpdatedAt),
		nil,
		c.Output,
	)
	if err != nil {
		return err
	}
	return outputBinaryResult(u.Out(), "json", result)
}

func (c *LibraryRefreshCancelCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	cc, err := NewClientContext(cfg)
	if err != nil {
		return err
	}
	defer cc.Cancel()

	if err := cc.Client.CancelRefresh(cc.Ctx, c.Section); err != nil {
		return err
	}
	return runLibraryMutationSummary(u, c.Output, plexclient.LibraryMutationSummary{
		Action:  "refresh-cancel",
		Target:  c.Section,
		Outcome: "refresh canceled",
	})
}

func (c *LibraryRefreshStopAllCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	if err := c.validate(); err != nil {
		return err
	}
	cc, err := NewClientContext(cfg)
	if err != nil {
		return err
	}
	defer cc.Cancel()

	if err := cc.Client.StopAllRefreshes(cc.Ctx); err != nil {
		return err
	}
	return runLibraryMutationSummary(u, c.Output, plexclient.LibraryMutationSummary{
		Action:  "refresh-stop-all",
		Target:  "all",
		Outcome: "all refreshes stopped",
	})
}

func librarySectionTypeID(value string) int64 {
	switch strings.ToLower(value) {
	case "movie":
		return 1
	case "show":
		return 2
	case "music":
		return 8
	case "photo":
		return 13
	default:
		return 1
	}
}

func (c *LibrarySectionDeleteCmd) validate() error {
	return requireConfirmed(c.Yes, "section delete")
}

func (c *LibrarySectionImageCmd) validate() error {
	return requireOutputPath(c.Output)
}

func (c *LibraryRefreshStopAllCmd) validate() error {
	return requireConfirmed(c.Yes, "refresh stop-all")
}

func runWithClient(cfg *config.Config, fn func(context.Context, *plexclient.Client) error) error {
	cc, err := NewClientContext(cfg)
	if err != nil {
		return err
	}
	defer cc.Cancel()
	return fn(cc.Ctx, cc.Client)
}
