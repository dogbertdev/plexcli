package cmd

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/alecthomas/kong"

	"github.com/dogbertdev/plexcli/internal/config"
	"github.com/dogbertdev/plexcli/internal/plexclient"
	"github.com/dogbertdev/plexcli/internal/ui"
)

type LibraryItemCmd struct {
	Delete     LibraryItemDeleteCmd     `cmd:"" help:"Delete one or more metadata items"`
	Edit       LibraryItemEditCmd       `cmd:"" help:"Edit metadata fields for one or more items"`
	Refresh    LibraryItemRefreshCmd    `cmd:"" help:"Refresh metadata for one or more items"`
	Analyze    LibraryItemAnalyzeCmd    `cmd:"" help:"Analyze one or more metadata items"`
	Unmatch    LibraryItemUnmatchCmd    `cmd:"" help:"Unmatch one or more metadata items"`
	Split      LibraryItemSplitCmd      `cmd:"" help:"Split a merged metadata item"`
	Merge      LibraryItemMergeCmd      `cmd:"" help:"Merge multiple metadata items"`
	Prefs      LibraryItemPrefsCmd      `cmd:"" help:"View or update item preferences"`
	BulkUpdate LibraryItemBulkUpdateCmd `cmd:"" name:"bulk-update" help:"Bulk update items in a section using a filter expression"`
}

type LibraryItemDeleteCmd struct {
	IDs    string `arg:"" name:"ids" help:"Comma-separated metadata IDs"`
	Yes    bool   `help:"Confirm deletion" name:"yes"`
	Output string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type LibraryItemEditCmd struct {
	IDs    string   `arg:"" name:"ids" help:"Comma-separated metadata IDs"`
	Set    []string `help:"Field assignment in key=value form" name:"set" required:""`
	Lock   []string `help:"Metadata field to lock" name:"lock"`
	Unlock []string `help:"Metadata field to unlock" name:"unlock"`
	Output string   `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type LibraryItemRefreshCmd struct {
	IDs    string `arg:"" name:"ids" help:"Comma-separated metadata IDs"`
	Output string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type LibraryItemAnalyzeCmd struct {
	IDs    string `arg:"" name:"ids" help:"Comma-separated metadata IDs"`
	Output string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type LibraryItemUnmatchCmd struct {
	IDs    string `arg:"" name:"ids" help:"Comma-separated metadata IDs"`
	Yes    bool   `help:"Confirm unmatch" name:"yes"`
	Output string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type LibraryItemSplitCmd struct {
	IDs    string `arg:"" name:"ids" help:"Comma-separated metadata IDs"`
	Yes    bool   `help:"Confirm split" name:"yes"`
	Output string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type LibraryItemMergeCmd struct {
	IDs    string `arg:"" name:"ids" help:"Comma-separated metadata IDs to merge"`
	Yes    bool   `help:"Confirm merge" name:"yes"`
	Output string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type LibraryItemPrefsCmd struct {
	List LibraryItemPrefsListCmd `cmd:"" help:"List item preferences"`
	Set  LibraryItemPrefsSetCmd  `cmd:"" help:"Set item preferences"`
}

type LibraryItemPrefsListCmd struct {
	IDs    string `arg:"" name:"ids" help:"Comma-separated metadata IDs"`
	Output string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type LibraryItemPrefsSetCmd struct {
	IDs    string   `arg:"" name:"ids" help:"Comma-separated metadata IDs"`
	Prefs  []string `help:"Preference override in key=value form" name:"pref" required:""`
	Output string   `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type LibraryItemBulkUpdateCmd struct {
	Section   string   `help:"Library section ID" short:"s" required:""`
	Filter    string   `help:"Raw Plex filter expression" required:""`
	Set       []string `help:"Field assignment in key=value form" name:"set"`
	Lock      []string `help:"Metadata field to lock" name:"lock"`
	AddTags   []string `help:"Tag assignment in type=value form" name:"add-tag"`
	RemoveTag []string `help:"Tag removal in type=value form" name:"remove-tag"`
	Output    string   `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type LibrarySubtitleCmd struct {
	Add LibrarySubtitleAddCmd `cmd:"" help:"Attach subtitles to a metadata item from a URL or local file"`
}

type LibrarySubtitleAddCmd struct {
	IDs             string `arg:"" name:"ids" help:"Comma-separated metadata IDs"`
	URL             string `help:"Subtitle URL"`
	File            string `help:"Local subtitle file to upload" type:"path"`
	Language        string `help:"Subtitle language code"`
	Title           string `help:"Subtitle title"`
	MediaItemID     int64  `help:"Specific media item ID for attachment" default:"0"`
	Format          string `help:"Subtitle format"`
	Forced          bool   `help:"Mark subtitles as forced"`
	HearingImpaired bool   `help:"Mark subtitles as hearing impaired"`
	Output          string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type LibraryArtworkCmd struct {
	Get    LibraryArtworkGetCmd    `cmd:"" help:"Download artwork for a metadata item"`
	Set    LibraryArtworkSetCmd    `cmd:"" help:"Set artwork for a metadata item from a URL"`
	Update LibraryArtworkUpdateCmd `cmd:"" help:"Update artwork for a metadata item from a URL"`
}

type LibraryArtworkGetCmd struct {
	IDs       string `arg:"" name:"ids" help:"Comma-separated metadata IDs"`
	Element   string `help:"Artwork element" default:"poster" enum:"thumb,art,clearLogo,banner,poster,theme"`
	Timestamp int64  `help:"Artwork timestamp used for cache busting" default:"0"`
	Output    string `help:"Destination file path" type:"path" required:""`
}

type LibraryArtworkSetCmd struct {
	IDs     string `arg:"" name:"ids" help:"Comma-separated metadata IDs"`
	Element string `help:"Artwork element" default:"poster" enum:"thumb,art,clearLogo,banner,poster,theme"`
	URL     string `help:"Artwork URL" required:""`
	Output  string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type LibraryArtworkUpdateCmd struct {
	IDs     string `arg:"" name:"ids" help:"Comma-separated metadata IDs"`
	Element string `help:"Artwork element" default:"poster" enum:"thumb,art,clearLogo,banner,poster,theme"`
	URL     string `help:"Artwork URL" required:""`
	Output  string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type LibraryDetectCmd struct {
	Ads          LibraryDetectAdsCmd          `cmd:"" help:"Detect ads for metadata items"`
	Credits      LibraryDetectCreditsCmd      `cmd:"" help:"Detect credits for metadata items"`
	Intros       LibraryDetectIntrosCmd       `cmd:"" help:"Detect intros for metadata items"`
	DeleteIntros LibraryDetectDeleteIntrosCmd `cmd:"" name:"delete-intros" help:"Delete section intros"`
	Voice        LibraryDetectVoiceCmd        `cmd:"" help:"Detect voice activity for metadata items"`
	BIF          LibraryDetectBIFCmd          `cmd:"" name:"bif" help:"Generate BIF previews for metadata items"`
	Status       LibraryDetectStatusCmd       `cmd:"" help:"Show augmentation status"`
}

type LibraryDetectAdsCmd struct {
	IDs    string `arg:"" name:"ids" help:"Comma-separated metadata IDs"`
	Output string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}
type LibraryDetectCreditsCmd struct {
	IDs    string `arg:"" name:"ids" help:"Comma-separated metadata IDs"`
	Force  bool   `help:"Force detection to rerun"`
	Manual bool   `help:"Mark detection as a manual request"`
	Output string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}
type LibraryDetectIntrosCmd struct {
	IDs       string   `arg:"" name:"ids" help:"Comma-separated metadata IDs"`
	Force     bool     `help:"Force detection to rerun"`
	Threshold *float64 `help:"Override the intro detection threshold"`
	Output    string   `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}
type LibraryDetectDeleteIntrosCmd struct {
	Section string `arg:"" name:"section" help:"Library section ID"`
	Yes     bool   `help:"Confirm intro deletion" name:"yes"`
	Output  string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}
type LibraryDetectVoiceCmd struct {
	IDs    string `arg:"" name:"ids" help:"Comma-separated metadata IDs"`
	Force  bool   `help:"Force detection to rerun"`
	Manual bool   `help:"Mark detection as a manual request"`
	Output string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}
type LibraryDetectBIFCmd struct {
	IDs    string `arg:"" name:"ids" help:"Comma-separated metadata IDs"`
	Force  bool   `help:"Force detection to rerun"`
	Output string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}
type LibraryDetectStatusCmd struct {
	AugmentationID string `arg:"" name:"augmentation-id" help:"Augmentation ID"`
	Output         string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type LibraryDiscoverCmd struct {
	Tags         LibraryDiscoverTagsCmd         `cmd:"" help:"List library tags"`
	Collections  LibraryDiscoverCollectionsCmd  `cmd:"" help:"List section collections"`
	Common       LibraryDiscoverCommonCmd       `cmd:"" help:"List common items in a section"`
	Related      LibraryDiscoverRelatedCmd      `cmd:"" help:"List items related to metadata IDs"`
	Similar      LibraryDiscoverSimilarCmd      `cmd:"" help:"List similar items for metadata IDs"`
	SonicSimilar LibraryDiscoverSonicCmd        `cmd:"" name:"sonic-similar" help:"List sonically similar items"`
	Autocomplete LibraryDiscoverAutocompleteCmd `cmd:"" help:"Autocomplete within a library section"`
	Matches      LibraryDiscoverMatchesCmd      `cmd:"" help:"List metadata matches for one or more items"`
	RandomArt    LibraryDiscoverRandomArtCmd    `cmd:"" name:"random-artwork" help:"List random artwork candidates"`
}

type LibraryDiscoverTagsCmd struct {
	Type   string `help:"Metadata type: movie, show, music, photo" default:"movie" enum:"movie,show,music,photo"`
	Output string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}
type LibraryDiscoverCollectionsCmd struct {
	Section string `arg:"" name:"section" help:"Library section ID"`
	Output  string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}
type LibraryDiscoverCommonCmd struct {
	Section string `arg:"" name:"section" help:"Library section ID"`
	Output  string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}
type LibraryDiscoverRelatedCmd struct {
	RatingKey string `arg:"" name:"ids" help:"Metadata rating key"`
	Compact   bool   `help:"Return a flat compact record set"`
	Output    string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}
type LibraryDiscoverSimilarCmd struct {
	RatingKey string `arg:"" name:"ids" help:"Metadata rating key"`
	Limit     int    `help:"Maximum number of results" default:"50"`
	Compact   bool   `help:"Return a flat compact record set"`
	Output    string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}
type LibraryDiscoverSonicCmd struct {
	IDs    string `arg:"" name:"ids" help:"Comma-separated metadata IDs"`
	Output string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}
type LibraryDiscoverAutocompleteCmd struct {
	Section string `arg:"" name:"section" help:"Library section ID"`
	Field   string `help:"Field to autocomplete" default:"title"`
	Query   string `help:"Autocomplete query" required:""`
	Output  string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}
type LibraryDiscoverMatchesCmd struct {
	RatingKey string `arg:"" name:"ids" help:"Metadata rating key"`
	Compact   bool   `help:"Return a flat compact record set"`
	Output    string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}
type LibraryDiscoverRandomArtCmd struct {
	Output string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type LibraryPersonCmd struct {
	Show  LibraryPersonShowCmd  `cmd:"" help:"Show person details"`
	Media LibraryPersonMediaCmd `cmd:"" help:"List media for a person"`
}

type LibraryPersonShowCmd struct {
	Person string `arg:"" name:"person" help:"Person ID"`
	Output string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type LibraryPersonMediaCmd struct {
	Person string `arg:"" name:"person" help:"Person ID"`
	Output string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type LibraryMediaCmd struct {
	Extras       LibraryMediaExtrasCmd       `cmd:"" help:"List extras for a metadata item"`
	File         LibraryMediaFileCmd         `cmd:"" help:"Download a bundled file for a metadata item"`
	Tree         LibraryMediaTreeCmd         `cmd:"" help:"Show the tree for a metadata item"`
	Leaves       LibraryMediaLeavesCmd       `cmd:"" help:"List all leaves for a metadata item"`
	Part         LibraryMediaPartCmd         `cmd:"" help:"Download a specific media part"`
	PartIndex    LibraryMediaPartIndexCmd    `cmd:"" name:"part-index" help:"Download a BIF index for a media part"`
	Stream       LibraryMediaStreamCmd       `cmd:"" help:"Download or manage a stream"`
	Levels       LibraryMediaLevelsCmd       `cmd:"" help:"Show stream levels"`
	Loudness     LibraryMediaLoudnessCmd     `cmd:"" help:"Show stream loudness"`
	Marker       LibraryMediaMarkerCmd       `cmd:"" help:"Create, edit, or delete markers"`
	ChapterImage LibraryMediaChapterImageCmd `cmd:"" name:"chapter-image" help:"Download a chapter image"`
	ItemArtwork  LibraryMediaItemArtworkCmd  `cmd:"" name:"item-artwork" help:"Download item artwork"`
	SectionImage LibraryMediaSectionImageCmd `cmd:"" name:"section-image" help:"Download section artwork"`
	BIFImage     LibraryMediaBIFImageCmd     `cmd:"" name:"bif-image" help:"Download an image from a BIF index"`
}

type LibraryMediaExtrasCmd struct {
	IDs    string `arg:"" name:"ids" help:"Comma-separated metadata IDs"`
	Output string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}
type LibraryMediaFileCmd struct {
	IDs    string `arg:"" optional:"" name:"ids" help:"Comma-separated metadata IDs for bundle-style file fetches"`
	URL    string `help:"Bundle URL to fetch"`
	Output string `help:"Destination file path" type:"path" required:""`
}
type LibraryMediaTreeCmd struct {
	IDs    string `arg:"" name:"ids" help:"Comma-separated metadata IDs"`
	Output string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}
type LibraryMediaLeavesCmd struct {
	IDs    string `arg:"" name:"ids" help:"Comma-separated metadata IDs"`
	Output string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}
type LibraryMediaPartCmd struct {
	PartID      int64  `arg:"" name:"part-id" help:"Part ID"`
	Changestamp int64  `arg:"" name:"changestamp" help:"Part changestamp"`
	Filename    string `arg:"" name:"filename" help:"Part filename"`
	Download    bool   `help:"Request a file download"`
	Output      string `help:"Destination file path" type:"path" required:""`
}
type LibraryMediaPartIndexCmd struct {
	PartID   int64  `arg:"" name:"part-id" help:"Part ID"`
	Index    string `arg:"" name:"index" help:"Index type" default:"sd" enum:"sd"`
	Interval int64  `help:"Interval between BIF images in milliseconds" default:"0"`
	Output   string `help:"Destination file path" type:"path" required:""`
}
type LibraryMediaStreamCmd struct {
	Get    LibraryMediaStreamGetCmd    `cmd:"" help:"Download a specific stream"`
	Delete LibraryMediaStreamDeleteCmd `cmd:"" help:"Delete a downloaded or sidecar stream"`
	Offset LibraryMediaStreamOffsetCmd `cmd:"" help:"Set subtitle stream offset"`
}
type LibraryMediaStreamGetCmd struct {
	StreamID   int64  `arg:"" name:"stream-id" help:"Stream ID"`
	Ext        string `arg:"" name:"ext" help:"Stream extension"`
	Encoding   string `help:"Requested encoding"`
	Format     string `help:"Requested format"`
	AutoAdjust bool   `help:"Auto-adjust subtitle timing"`
	Output     string `help:"Destination file path" type:"path" required:""`
}
type LibraryMediaStreamDeleteCmd struct {
	StreamID int64  `arg:"" name:"stream-id" help:"Stream ID"`
	Ext      string `arg:"" name:"ext" help:"Stream extension"`
	Yes      bool   `help:"Confirm deletion" name:"yes"`
	Output   string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}
type LibraryMediaStreamOffsetCmd struct {
	StreamID int64  `arg:"" name:"stream-id" help:"Stream ID"`
	Ext      string `arg:"" name:"ext" help:"Stream extension"`
	Offset   int64  `help:"Subtitle offset in milliseconds" required:""`
	Output   string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}
type LibraryMediaLevelsCmd struct {
	StreamID int64  `arg:"" name:"stream-id" help:"Stream ID"`
	Output   string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}
type LibraryMediaLoudnessCmd struct {
	StreamID int64  `arg:"" name:"stream-id" help:"Stream ID"`
	Output   string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}
type LibraryMediaMarkerCmd struct {
	Create LibraryMediaMarkerCreateCmd `cmd:"" help:"Create a marker"`
	Edit   LibraryMediaMarkerEditCmd   `cmd:"" help:"Edit a marker"`
	Delete LibraryMediaMarkerDeleteCmd `cmd:"" help:"Delete a marker"`
}
type LibraryMediaMarkerCreateCmd struct {
	IDs        string   `arg:"" name:"ids" help:"Comma-separated metadata IDs"`
	Type       string   `help:"Marker type" required:"" enum:"bookmark,intro,commercial,resume,credit"`
	Start      int64    `help:"Start offset in milliseconds" required:""`
	End        int64    `help:"End offset in milliseconds"`
	Attributes []string `help:"Marker attribute in key=value form" name:"attr"`
	Output     string   `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}
type LibraryMediaMarkerEditCmd struct {
	IDs        string   `arg:"" name:"ids" help:"Comma-separated metadata IDs"`
	Marker     string   `arg:"" name:"marker" help:"Marker ID"`
	Type       string   `help:"Marker type" required:"" enum:"bookmark,intro,commercial,resume,credit"`
	Start      int64    `help:"Start offset in milliseconds" required:""`
	End        int64    `help:"End offset in milliseconds"`
	Attributes []string `help:"Marker attribute in key=value form" name:"attr"`
	Output     string   `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}
type LibraryMediaMarkerDeleteCmd struct {
	IDs    string `arg:"" name:"ids" help:"Comma-separated metadata IDs"`
	Marker string `arg:"" name:"marker" help:"Marker ID"`
	Yes    bool   `help:"Confirm deletion" name:"yes"`
	Output string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}
type LibraryMediaChapterImageCmd struct {
	MediaID int64  `arg:"" name:"media-id" help:"Media ID"`
	Chapter int64  `arg:"" name:"chapter" help:"Chapter number"`
	Output  string `help:"Destination file path" type:"path" required:""`
}
type LibraryMediaItemArtworkCmd struct {
	IDs       string `arg:"" name:"ids" help:"Comma-separated metadata IDs"`
	Element   string `help:"Artwork element" default:"poster" enum:"thumb,art,clearLogo,banner,poster,theme"`
	Timestamp int64  `help:"Artwork timestamp used for cache busting" default:"0"`
	Output    string `help:"Destination file path" type:"path" required:""`
}
type LibraryMediaSectionImageCmd struct {
	Section   string `arg:"" name:"section" help:"Library section ID"`
	UpdatedAt int64  `help:"Section updated-at timestamp used for cache busting" default:"0"`
	Output    string `help:"Destination file path" type:"path" required:""`
}
type LibraryMediaBIFImageCmd struct {
	PartID int64  `arg:"" name:"part-id" help:"Part ID"`
	Index  string `arg:"" name:"index" help:"BIF index"`
	Offset string `arg:"" name:"offset" help:"BIF offset"`
	Output string `help:"Destination file path" type:"path" required:""`
}

func (c *LibraryItemDeleteCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	if err := requireConfirmed(c.Yes, "item delete"); err != nil {
		return err
	}
	return runSimpleClientAction(cfg, u, c.Output, "item-delete", c.IDs, "items deleted", func(runCtx context.Context, client *plexclient.Client) error {
		return client.DeleteMetadata(runCtx, c.IDs)
	})
}

func (c *LibraryItemEditCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	setValues, err := parseKeyValueFlags(c.Set)
	if err != nil {
		return err
	}
	return runSimpleClientAction(cfg, u, c.Output, "item-edit", c.IDs, "items updated", func(runCtx context.Context, client *plexclient.Client) error {
		return client.EditMetadataDynamic(runCtx, c.IDs, plexclient.MetadataEditInput{Set: setValues, Lock: c.Lock, Unlock: c.Unlock})
	})
}

func (c *LibraryItemRefreshCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	return runMetadataPathAction(cfg, u, c.Output, c.IDs, "RefreshItemsMetadata", http.MethodPut, "/refresh", "item-refresh", "refresh requested")
}

func (c *LibraryItemAnalyzeCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	return runMetadataPathAction(cfg, u, c.Output, c.IDs, "AnalyzeMetadata", http.MethodPut, "/analyze", "item-analyze", "analysis requested")
}

func (c *LibraryItemUnmatchCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	if err := requireConfirmed(c.Yes, "item unmatch"); err != nil {
		return err
	}
	return runSimpleClientAction(cfg, u, c.Output, "item-unmatch", c.IDs, "items unmatched", func(runCtx context.Context, client *plexclient.Client) error {
		return client.UnmatchMetadata(runCtx, c.IDs)
	})
}

func (c *LibraryItemSplitCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	if err := requireConfirmed(c.Yes, "item split"); err != nil {
		return err
	}
	return runSimpleClientAction(cfg, u, c.Output, "item-split", c.IDs, "item split requested", func(runCtx context.Context, client *plexclient.Client) error {
		return client.SplitMetadata(runCtx, c.IDs)
	})
}

func (c *LibraryItemMergeCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	if err := requireConfirmed(c.Yes, "item merge"); err != nil {
		return err
	}
	return runSimpleClientAction(cfg, u, c.Output, "item-merge", c.IDs, "merge requested", func(runCtx context.Context, client *plexclient.Client) error {
		return client.MergeMetadata(runCtx, c.IDs)
	})
}

func (c *LibraryItemPrefsListCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	if _, err := encodeCSVArg(c.IDs); err != nil {
		return err
	}
	return fmt.Errorf("Plex does not expose a readable item preferences endpoint; use `library item prefs set` to update preferences")
}

func (c *LibraryItemPrefsSetCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	prefs, err := parseKeyValueFlags(c.Prefs)
	if err != nil {
		return err
	}
	return runSimpleClientAction(cfg, u, c.Output, "item-prefs-set", c.IDs, "preferences updated", func(runCtx context.Context, client *plexclient.Client) error {
		return client.SetItemPreferencesDynamic(runCtx, c.IDs, prefs)
	})
}

func (c *LibraryItemBulkUpdateCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	setValues, err := parseKeyValueFlags(c.Set)
	if err != nil && len(c.Set) > 0 {
		return err
	}
	addTags, err := parseMultiValueFlags(c.AddTags)
	if err != nil && len(c.AddTags) > 0 {
		return err
	}
	removeTags, err := parseMultiValueFlags(c.RemoveTag)
	if err != nil && len(c.RemoveTag) > 0 {
		return err
	}
	return runSimpleClientAction(cfg, u, c.Output, "item-bulk-update", c.Section, "bulk update requested", func(runCtx context.Context, client *plexclient.Client) error {
		sectionTypeID, err := sectionTypeIDForLibrary(runCtx, client, c.Section)
		if err != nil {
			return err
		}
		return client.UpdateItemsDynamic(runCtx, plexclient.BulkUpdateInput{
			SectionID:  c.Section,
			MediaType:  sectionTypeID,
			Filter:     c.Filter,
			Set:        setValues,
			Lock:       c.Lock,
			AddTags:    addTags,
			RemoveTags: removeTags,
		})
	})
}

func (c *LibrarySubtitleAddCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	if err := c.validate(); err != nil {
		return err
	}
	return runSimpleClientAction(cfg, u, c.Output, "subtitle-add", c.IDs, "subtitle attached", func(runCtx context.Context, client *plexclient.Client) error {
		if c.File != "" {
			payload, err := os.ReadFile(c.File)
			if err != nil {
				return fmt.Errorf("failed to read subtitle file: %w", err)
			}
			title := c.Title
			if title == "" {
				title = filepath.Base(c.File)
			}
			return client.AddSubtitlesFromFile(runCtx, c.IDs, payload, c.Language, title, c.MediaItemID, c.Format, c.Forced, c.HearingImpaired)
		}
		return client.AddSubtitlesByURL(runCtx, c.IDs, c.URL, c.Language, c.Title, c.MediaItemID, c.Format, c.Forced, c.HearingImpaired)
	})
}

func (c *LibraryArtworkGetCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	if err := c.validate(); err != nil {
		return err
	}
	encodedIDs, err := encodeCSVArg(c.IDs)
	if err != nil {
		return err
	}
	return downloadLibraryFile(cfg, u, "GetItemArtwork", fmt.Sprintf("library/metadata/%s/%s/%d", encodedIDs, c.Element, c.Timestamp), nil, c.Output)
}

func (c *LibraryArtworkSetCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	return runSimpleClientAction(cfg, u, c.Output, "artwork-set", c.IDs, "artwork set", func(runCtx context.Context, client *plexclient.Client) error {
		return client.SetItemArtworkByURL(runCtx, c.IDs, c.Element, c.URL)
	})
}

func (c *LibraryArtworkUpdateCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	return runSimpleClientAction(cfg, u, c.Output, "artwork-update", c.IDs, "artwork updated", func(runCtx context.Context, client *plexclient.Client) error {
		return client.UpdateItemArtworkByURL(runCtx, c.IDs, c.Element, c.URL)
	})
}

func (c *LibraryDetectAdsCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	return runSimpleClientAction(cfg, u, c.Output, "detect-ads", c.IDs, "ad detection requested", func(runCtx context.Context, client *plexclient.Client) error {
		return client.DetectMetadataAds(runCtx, c.IDs)
	})
}

func (c *LibraryDetectCreditsCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	return runSimpleClientAction(cfg, u, c.Output, "detect-credits", c.IDs, "credit detection requested", func(runCtx context.Context, client *plexclient.Client) error {
		return client.DetectMetadataCredits(runCtx, c.IDs, c.Force, c.Manual)
	})
}

func (c *LibraryDetectIntrosCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	return runSimpleClientAction(cfg, u, c.Output, "detect-intros", c.IDs, "intro detection requested", func(runCtx context.Context, client *plexclient.Client) error {
		return client.DetectMetadataIntros(runCtx, c.IDs, c.Force, c.Threshold)
	})
}

func (c *LibraryDetectDeleteIntrosCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	if err := requireConfirmed(c.Yes, "delete-intros"); err != nil {
		return err
	}
	return runSimpleClientAction(cfg, u, c.Output, "delete-intros", c.Section, "section intros deleted", func(runCtx context.Context, client *plexclient.Client) error {
		return client.DeleteSectionIntros(runCtx, c.Section)
	})
}

func (c *LibraryDetectVoiceCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	return runSimpleClientAction(cfg, u, c.Output, "detect-voice", c.IDs, "voice activity detection requested", func(runCtx context.Context, client *plexclient.Client) error {
		return client.DetectMetadataVoiceActivity(runCtx, c.IDs, c.Force, c.Manual)
	})
}

func (c *LibraryDetectBIFCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	return runSimpleClientAction(cfg, u, c.Output, "detect-bif", c.IDs, "BIF generation requested", func(runCtx context.Context, client *plexclient.Client) error {
		return client.GenerateBIF(runCtx, c.IDs, c.Force)
	})
}

func (c *LibraryDetectStatusCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	return runLibraryJSONCommand(u, cfg, c.Output, "GetAugmentationStatus", fmt.Sprintf("library/metadata/augmentations/%s", url.PathEscape(c.AugmentationID)), nil)
}

func (c *LibraryDiscoverTagsCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	query := url.Values{}
	query.Set("type", strconv.FormatInt(librarySectionTypeID(c.Type), 10))
	return runLibraryJSONCommand(u, cfg, c.Output, "GetTags", "library/tags", query)
}

func (c *LibraryDiscoverCollectionsCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	return runLibraryJSONCommand(u, cfg, c.Output, "GetCollections", fmt.Sprintf("library/sections/%s/collection", url.PathEscape(c.Section)), nil)
}

func (c *LibraryDiscoverCommonCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	return runWithClient(cfg, func(runCtx context.Context, client *plexclient.Client) error {
		sectionTypeID, err := sectionTypeIDForLibrary(runCtx, client, c.Section)
		if err != nil {
			return err
		}
		query := url.Values{}
		query.Set("type", strconv.FormatInt(sectionTypeID, 10))

		payload, err := client.LibraryJSON(runCtx, "GetCommon", http.MethodGet, fmt.Sprintf("library/sections/%s/common", url.PathEscape(c.Section)), query)
		if err != nil {
			return err
		}
		return outputGenericPayload(u.Out(), c.Output, payload)
	})
}

func (c *LibraryDiscoverRelatedCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	cc, err := newLibraryDiscoverClientContext(cfg)
	if err != nil {
		return err
	}
	defer cc.Cancel()

	encodedIDs, err := encodeCSVArg(c.RatingKey)
	if err != nil {
		return err
	}
	results, err := cc.Client.GetRelatedItems(cc.Ctx, encodedIDs)
	if err != nil {
		return fmt.Errorf("failed to list related items: %w", err)
	}
	return outputDiscoveryResults(u.Out(), c.Output, c.Compact, results)
}

func (c *LibraryDiscoverSimilarCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	cc, err := newLibraryDiscoverClientContext(cfg)
	if err != nil {
		return err
	}
	defer cc.Cancel()

	encodedIDs, err := encodeCSVArg(c.RatingKey)
	if err != nil {
		return err
	}
	results, err := cc.Client.ListSimilar(cc.Ctx, encodedIDs, c.Limit)
	if err != nil {
		return fmt.Errorf("failed to list similar items: %w", err)
	}
	return outputDiscoveryResults(u.Out(), c.Output, c.Compact, results)
}

func (c *LibraryDiscoverSonicCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	return runMetadataJSON(u, cfg, c.Output, "ListSonicallySimilar", c.IDs, "/nearest", nil)
}

func (c *LibraryDiscoverAutocompleteCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	return runWithClient(cfg, func(runCtx context.Context, client *plexclient.Client) error {
		sectionTypeID, err := sectionTypeIDForLibrary(runCtx, client, c.Section)
		if err != nil {
			return err
		}
		query := url.Values{}
		query.Set("type", strconv.FormatInt(sectionTypeID, 10))
		query.Set(fmt.Sprintf("%s.query", c.Field), c.Query)

		payload, err := client.LibraryJSON(runCtx, "Autocomplete", http.MethodGet, fmt.Sprintf("library/sections/%s/autocomplete", url.PathEscape(c.Section)), query)
		if err != nil {
			return err
		}
		return outputGenericPayload(u.Out(), c.Output, payload)
	})
}

func (c *LibraryDiscoverMatchesCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	cc, err := newLibraryDiscoverClientContext(cfg)
	if err != nil {
		return err
	}
	defer cc.Cancel()

	ids, err := splitLibraryDiscoverIDs(c.RatingKey)
	if err != nil {
		return err
	}
	results := make([]plexclient.MatchResult, 0, len(ids))
	for _, id := range ids {
		matches, matchErr := cc.Client.SearchMatches(cc.Ctx, id, "", 0)
		if matchErr != nil {
			return fmt.Errorf("failed to list matches for %s: %w", id, matchErr)
		}
		results = append(results, matches...)
	}
	return outputDiscoveryMatches(u.Out(), c.Output, c.Compact, results)
}

func (c *LibraryDiscoverRandomArtCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	return runLibraryJSONCommand(u, cfg, c.Output, "GetRandomArtwork", "library/randomArtwork", nil)
}

func (c *LibraryPersonShowCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	return runLibraryJSONCommand(u, cfg, c.Output, "GetPerson", fmt.Sprintf("library/people/%s", url.PathEscape(c.Person)), nil)
}

func (c *LibraryPersonMediaCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	return runLibraryJSONCommand(u, cfg, c.Output, "ListPersonMedia", fmt.Sprintf("library/people/%s/media", url.PathEscape(c.Person)), nil)
}

func (c *LibraryMediaExtrasCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	return runMetadataJSON(u, cfg, c.Output, "GetExtras", c.IDs, "/extras", nil)
}

func (c *LibraryMediaFileCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	if err := c.validate(); err != nil {
		return err
	}
	if strings.HasPrefix(c.URL, "/") {
		return downloadLibraryFile(cfg, u, "GetFile", strings.TrimPrefix(c.URL, "/"), nil, c.Output)
	}
	encodedIDs, err := encodeCSVArg(c.IDs)
	if err != nil {
		return err
	}
	query := url.Values{}
	if c.URL != "" {
		query.Set("url", c.URL)
	}
	return downloadLibraryFile(cfg, u, "GetFile", fmt.Sprintf("library/metadata/%s/file", encodedIDs), query, c.Output)
}

func (c *LibraryMediaTreeCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	return runMetadataJSON(u, cfg, c.Output, "GetItemTree", c.IDs, "/tree", nil)
}

func (c *LibraryMediaLeavesCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	return runMetadataJSON(u, cfg, c.Output, "GetAllItemLeaves", c.IDs, "/allLeaves", nil)
}

func (c *LibraryMediaPartCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	query := url.Values{}
	if c.Download {
		query.Set("download", "1")
	}
	return downloadLibraryFile(cfg, u, "GetMediaPart", fmt.Sprintf("library/parts/%d/%d/%s", c.PartID, c.Changestamp, url.PathEscape(c.Filename)), query, c.Output)
}

func (c *LibraryMediaPartIndexCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	if err := c.validate(); err != nil {
		return err
	}
	return runWithClient(cfg, func(runCtx context.Context, client *plexclient.Client) error {
		var interval *int64
		if c.Interval > 0 {
			interval = &c.Interval
		}
		result, err := client.DownloadPartIndex(runCtx, c.PartID, c.Index, interval, c.Output)
		if err != nil {
			return err
		}
		return outputBinaryResult(u.Out(), "json", result)
	})
}

func (c *LibraryMediaStreamGetCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	if err := c.validate(); err != nil {
		return err
	}
	query := url.Values{}
	if c.Encoding != "" {
		query.Set("encoding", c.Encoding)
	}
	if c.Format != "" {
		query.Set("format", c.Format)
	}
	if c.AutoAdjust {
		query.Set("autoAdjustSubtitle", "1")
	}
	return downloadLibraryFile(cfg, u, "GetStream", fmt.Sprintf("library/streams/%d.%s", c.StreamID, c.Ext), query, c.Output)
}

func (c *LibraryMediaStreamDeleteCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	if err := requireConfirmed(c.Yes, "stream delete"); err != nil {
		return err
	}
	return runSimpleClientAction(cfg, u, c.Output, "stream-delete", strconv.FormatInt(c.StreamID, 10), "stream deleted", func(runCtx context.Context, client *plexclient.Client) error {
		return client.DeleteStreamByID(runCtx, c.StreamID, c.Ext)
	})
}

func (c *LibraryMediaStreamOffsetCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	return runSimpleClientAction(cfg, u, c.Output, "stream-offset", strconv.FormatInt(c.StreamID, 10), "stream offset updated", func(runCtx context.Context, client *plexclient.Client) error {
		return client.SetStreamOffsetByID(runCtx, c.StreamID, c.Ext, c.Offset)
	})
}

func (c *LibraryMediaLevelsCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	return runLibraryJSONCommand(u, cfg, c.Output, "GetStreamLevels", fmt.Sprintf("library/streams/%d/levels", c.StreamID), nil)
}

func (c *LibraryMediaLoudnessCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	return runLibraryJSONCommand(u, cfg, c.Output, "GetStreamLoudness", fmt.Sprintf("library/streams/%d/loudness", c.StreamID), nil)
}

func (c *LibraryMediaMarkerCreateCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	attrs, err := parseKeyValueFlags(c.Attributes)
	if err != nil && len(c.Attributes) > 0 {
		return err
	}
	var end *int64
	if c.End > 0 {
		end = &c.End
	}
	return runSimpleClientAction(cfg, u, c.Output, "marker-create", c.IDs, "marker created", func(runCtx context.Context, client *plexclient.Client) error {
		return client.CreateMarkerDynamic(runCtx, c.IDs, c.Type, c.Start, end, attrs)
	})
}

func (c *LibraryMediaMarkerEditCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	attrs, err := parseKeyValueFlags(c.Attributes)
	if err != nil && len(c.Attributes) > 0 {
		return err
	}
	var end *int64
	if c.End > 0 {
		end = &c.End
	}
	return runSimpleClientAction(cfg, u, c.Output, "marker-edit", c.Marker, "marker updated", func(runCtx context.Context, client *plexclient.Client) error {
		return client.EditMarkerDynamic(runCtx, c.IDs, c.Marker, c.Type, c.Start, end, attrs)
	})
}

func (c *LibraryMediaMarkerDeleteCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	if err := requireConfirmed(c.Yes, "marker delete"); err != nil {
		return err
	}
	return runSimpleClientAction(cfg, u, c.Output, "marker-delete", c.Marker, "marker deleted", func(runCtx context.Context, client *plexclient.Client) error {
		return client.DeleteMarkerByID(runCtx, c.IDs, c.Marker)
	})
}

func (c *LibraryMediaChapterImageCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	return downloadLibraryFile(cfg, u, "GetChapterImage", fmt.Sprintf("library/media/%d/chapterImages/%d", c.MediaID, c.Chapter), nil, c.Output)
}

func (c *LibraryMediaItemArtworkCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	encodedIDs, err := encodeCSVArg(c.IDs)
	if err != nil {
		return err
	}
	return downloadLibraryFile(cfg, u, "GetItemArtwork", fmt.Sprintf("library/metadata/%s/%s/%d", encodedIDs, c.Element, c.Timestamp), nil, c.Output)
}

func (c *LibraryMediaSectionImageCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	return downloadLibraryFile(cfg, u, "GetSectionImage", fmt.Sprintf("library/sections/%s/composite/%d", url.PathEscape(c.Section), c.UpdatedAt), nil, c.Output)
}

func (c *LibraryMediaBIFImageCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	return downloadLibraryFile(cfg, u, "GetImageFromBif", fmt.Sprintf("library/parts/%d/indexes/%s/%s", c.PartID, url.PathEscape(c.Index), url.PathEscape(c.Offset)), nil, c.Output)
}

func runSimpleClientAction(cfg *config.Config, u *ui.UI, output, action, target, outcome string, fn func(context.Context, *plexclient.Client) error) error {
	return runWithClient(cfg, func(runCtx context.Context, client *plexclient.Client) error {
		if err := fn(runCtx, client); err != nil {
			return err
		}
		return runLibraryMutationSummary(u, output, plexclient.LibraryMutationSummary{
			Action:  action,
			Target:  target,
			Outcome: outcome,
		})
	})
}

func runMetadataJSON(u *ui.UI, cfg *config.Config, output, op, ids, suffix string, query url.Values) error {
	encodedIDs, err := encodeCSVArg(ids)
	if err != nil {
		return err
	}
	return runLibraryJSONCommand(u, cfg, output, op, fmt.Sprintf("library/metadata/%s%s", encodedIDs, suffix), query)
}

func runMetadataPathAction(cfg *config.Config, u *ui.UI, output, ids, op string, method string, suffix string, action string, outcome string) error {
	encodedIDs, err := encodeCSVArg(ids)
	if err != nil {
		return err
	}
	return runSimpleClientAction(cfg, u, output, action, ids, outcome, func(runCtx context.Context, client *plexclient.Client) error {
		return client.LibraryAction(runCtx, op, method, fmt.Sprintf("library/metadata/%s%s", encodedIDs, suffix), nil)
	})
}

func downloadLibraryFile(cfg *config.Config, u *ui.UI, op, path string, query url.Values, outputPath string) error {
	if err := requireOutputPath(outputPath); err != nil {
		return err
	}
	return runWithClient(cfg, func(runCtx context.Context, client *plexclient.Client) error {
		result, err := client.LibraryDownload(runCtx, op, http.MethodGet, path, query, outputPath)
		if err != nil {
			return err
		}
		return outputBinaryResult(u.Out(), "json", result)
	})
}

func (c *LibrarySubtitleAddCmd) validate() error {
	if (c.URL == "" && c.File == "") || (c.URL != "" && c.File != "") {
		return fmt.Errorf("specify exactly one of --url or --file")
	}
	return nil
}

func (c *LibraryArtworkGetCmd) validate() error {
	return requireOutputPath(c.Output)
}

func (c *LibraryMediaFileCmd) validate() error {
	if err := requireOutputPath(c.Output); err != nil {
		return err
	}
	if strings.TrimSpace(c.URL) == "" {
		return fmt.Errorf("--url is required")
	}
	if strings.HasPrefix(c.URL, "/") {
		return nil
	}
	if strings.TrimSpace(c.IDs) == "" {
		return fmt.Errorf("ids are required unless --url is a direct Plex library path")
	}
	return nil
}

func (c *LibraryMediaPartIndexCmd) validate() error {
	return requireOutputPath(c.Output)
}

func (c *LibraryMediaStreamGetCmd) validate() error {
	return requireOutputPath(c.Output)
}
