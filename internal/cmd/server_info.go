package cmd

import (
	"fmt"
	"io"

	"github.com/alecthomas/kong"

	"github.com/dogbertdev/plexcli/internal/config"
	"github.com/dogbertdev/plexcli/internal/outfmt"
	"github.com/dogbertdev/plexcli/internal/ui"
)

type ServerInfoCmd struct {
	Output string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type ServerInfoItem struct {
	ServerURL      string `json:"server_url"`
	ServerUUID     string `json:"server_uuid"`
	LibraryCount   int    `json:"library_count"`
	MovieCount     int    `json:"movie_count"`
	ShowCount      int    `json:"show_count"`
	MusicCount     int    `json:"music_count"`
	PhotoCount     int    `json:"photo_count"`
	RequestTimeout int    `json:"request_timeout_seconds"`
}

func (c *ServerInfoCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	cc, err := NewClientContext(cfg)
	if err != nil {
		return err
	}
	defer cc.Cancel()

	uuid, err := cc.Client.GetServerUUID(cc.Ctx)
	if err != nil {
		return fmt.Errorf("failed to get server UUID: %w", err)
	}

	sections, err := cc.Client.GetSections(cc.Ctx)
	if err != nil {
		return fmt.Errorf("failed to get sections: %w", err)
	}

	item := ServerInfoItem{
		ServerURL:      cc.Client.GetServerURL(),
		ServerUUID:     uuid,
		LibraryCount:   len(sections),
		RequestTimeout: int(cc.Timeout.Seconds()),
	}

	for _, section := range sections {
		switch section.Type {
		case "movie":
			item.MovieCount++
		case "show":
			item.ShowCount++
		case "artist":
			item.MusicCount++
		case "photo":
			item.PhotoCount++
		}
	}

	return c.output(u.Out(), item)
}

func (c *ServerInfoCmd) output(w io.Writer, item ServerInfoItem) error {
	formatter := outfmt.NewFormatter(outfmt.Format(c.Output))

	header := []string{"FIELD", "VALUE"}
	rows := [][]string{
		{"Server URL", item.ServerURL},
		{"Server UUID", item.ServerUUID},
		{"Libraries", fmt.Sprintf("%d", item.LibraryCount)},
		{"Movie Libraries", fmt.Sprintf("%d", item.MovieCount)},
		{"TV Libraries", fmt.Sprintf("%d", item.ShowCount)},
		{"Music Libraries", fmt.Sprintf("%d", item.MusicCount)},
		{"Photo Libraries", fmt.Sprintf("%d", item.PhotoCount)},
		{"Timeout (seconds)", fmt.Sprintf("%d", item.RequestTimeout)},
	}

	return formatter.Format(w, header, rows, item)
}
