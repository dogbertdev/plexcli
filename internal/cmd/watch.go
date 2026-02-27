package cmd

import (
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/alecthomas/kong"

	"github.com/dogbertdev/plexcli/internal/config"
	"github.com/dogbertdev/plexcli/internal/outfmt"
	"github.com/dogbertdev/plexcli/internal/plexclient"
	"github.com/dogbertdev/plexcli/internal/ui"
)

type WatchCmd struct {
	Now     WatchNowCmd     `cmd:"" help:"Show what's currently being watched"`
	History WatchHistoryCmd `cmd:"" help:"Show watch history"`
	Stats   WatchStatsCmd   `cmd:"" help:"Show watch statistics (most watched, by user, etc.)"`
}

// WatchNowCmd shows current active sessions
type WatchNowCmd struct {
	Output string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

// WatchHistoryCmd shows watch history
type WatchHistoryCmd struct {
	Limit  int    `help:"Maximum number of items to show" default:"50"`
	Days   int    `help:"Only show items from the last N days" default:"30"`
	User   string `help:"Filter by user name" short:"u"`
	Type   string `help:"Filter by type: movie, episode, or all" default:"all" enum:"movie,episode,all"`
	Output string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

// WatchStatsCmd shows watch statistics
type WatchStatsCmd struct {
	Days   int    `help:"Analyze items from the last N days" default:"30"`
	Top    int    `help:"Number of top items to show" default:"10"`
	By     string `help:"Group by: title, user, show" default:"title" enum:"title,user,show"`
	Type   string `help:"Filter by type: movie, episode, or all" default:"all" enum:"movie,episode,all"`
	Output string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type ActiveSession struct {
	User     string `json:"user"`
	Title    string `json:"title"`
	Type     string `json:"type"`
	Show     string `json:"show,omitempty"`
	Progress string `json:"progress"`
	Device   string `json:"device"`
	State    string `json:"state"`
}

type HistoryItem struct {
	Title     string    `json:"title"`
	Type      string    `json:"type"`
	Show      string    `json:"show,omitempty"`
	Season    int       `json:"season,omitempty"`
	Episode   int       `json:"episode,omitempty"`
	User      string    `json:"user"`
	WatchedAt time.Time `json:"watched_at"`
}

type WatchStat struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
	Type  string `json:"type,omitempty"`
}

func (c *WatchNowCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	cc, err := NewClientContext(cfg)
	if err != nil {
		return err
	}
	defer cc.Cancel()

	sessions, err := cc.Client.GetActiveSessions(cc.Ctx)
	if err != nil {
		return err
	}

	if len(sessions) == 0 {
		fmt.Fprintln(u.Out(), "No active sessions")
		return nil
	}

	return c.outputResults(u.Out(), sessions)
}

func (c *WatchNowCmd) outputResults(w io.Writer, sessions []plexclient.ActiveSession) error {
	formatter := outfmt.NewFormatter(outfmt.Format(c.Output))

	header := []string{"USER", "TITLE", "TYPE", "SHOW", "PROGRESS", "DEVICE", "STATE"}
	rows := make([][]string, len(sessions))

	for i, s := range sessions {
		rows[i] = []string{
			s.User,
			s.Title,
			s.Type,
			s.Show,
			s.Progress,
			s.Device,
			s.State,
		}
	}

	return formatter.Format(w, header, rows, sessions)
}

func (c *WatchHistoryCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	cc, err := NewClientContext(cfg)
	if err != nil {
		return err
	}
	defer cc.Cancel()

	history, err := cc.Client.GetWatchHistory(cc.Ctx)
	if err != nil {
		return err
	}

	// Get accounts for user name lookup
	accounts, _ := cc.Client.GetAccounts(cc.Ctx)
	accountNameByID := buildAccountNameByID(accounts)

	cutoff := time.Now().AddDate(0, 0, -c.Days)
	var filtered []HistoryItem

	for _, h := range history {
		if !watchHistoryMatchesFilters(h, cutoff, c.Type) {
			continue
		}

		userName := accountDisplayName(accountNameByID, h.AccountID)

		// Filter by user
		if c.User != "" && userName != c.User {
			continue
		}

		filtered = append(filtered, HistoryItem{
			Title:     h.Title,
			Type:      h.Type,
			Show:      h.GrandparentTitle,
			Season:    h.ParentIndex,
			Episode:   h.Index,
			User:      userName,
			WatchedAt: h.ViewedAt,
		})

		if c.Limit > 0 && len(filtered) >= c.Limit {
			break
		}
	}

	if len(filtered) == 0 {
		fmt.Fprintln(u.Err(), "No watch history found")
		return nil
	}

	return c.outputResults(u.Out(), filtered)
}

func (c *WatchHistoryCmd) outputResults(w io.Writer, history []HistoryItem) error {
	formatter := outfmt.NewFormatter(outfmt.Format(c.Output))

	header := []string{"TITLE", "TYPE", "SHOW", "S", "E", "USER", "WATCHED"}
	rows := make([][]string, len(history))

	for i, h := range history {
		seasonStr := ""
		if h.Season > 0 {
			seasonStr = fmt.Sprintf("%d", h.Season)
		}
		episodeStr := ""
		if h.Episode > 0 {
			episodeStr = fmt.Sprintf("%d", h.Episode)
		}

		rows[i] = []string{
			h.Title,
			h.Type,
			h.Show,
			seasonStr,
			episodeStr,
			h.User,
			h.WatchedAt.Format("2006-01-02 15:04"),
		}
	}

	return formatter.Format(w, header, rows, history)
}

func (c *WatchStatsCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	cc, err := NewClientContext(cfg)
	if err != nil {
		return err
	}
	defer cc.Cancel()

	history, err := cc.Client.GetWatchHistory(cc.Ctx)
	if err != nil {
		return err
	}

	// Get accounts for user name lookup
	accounts, _ := cc.Client.GetAccounts(cc.Ctx)
	accountNameByID := buildAccountNameByID(accounts)

	cutoff := time.Now().AddDate(0, 0, -c.Days)

	// Count by grouping
	counts := make(map[string]*WatchStat)

	for _, h := range history {
		if !watchHistoryMatchesFilters(h, cutoff, c.Type) {
			continue
		}

		var key string
		var name string
		var itemType string

		switch c.By {
		case "user":
			userName := accountDisplayName(accountNameByID, h.AccountID)
			key = fmt.Sprintf("user:%d", h.AccountID)
			name = userName
			itemType = "user"
		case "show":
			if h.Type == "episode" && h.GrandparentTitle != "" {
				key = "show:" + h.GrandparentTitle
				name = h.GrandparentTitle
				itemType = "show"
			} else if h.Type == "movie" {
				key = "movie:" + h.Title
				name = h.Title
				itemType = "movie"
			} else {
				continue
			}
		default: // title
			if h.Type == "episode" && h.GrandparentTitle != "" {
				key = "show:" + h.GrandparentTitle
				name = h.GrandparentTitle
				itemType = "show"
			} else {
				key = h.Type + ":" + h.Title
				name = h.Title
				itemType = h.Type
			}
		}

		if _, ok := counts[key]; !ok {
			counts[key] = &WatchStat{Name: name, Type: itemType}
		}
		counts[key].Count++
	}

	// Convert to slice and sort
	var stats []WatchStat
	for _, s := range counts {
		stats = append(stats, *s)
	}
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].Count > stats[j].Count
	})

	// Limit results
	if c.Top > 0 && len(stats) > c.Top {
		stats = stats[:c.Top]
	}

	if len(stats) == 0 {
		fmt.Fprintln(u.Err(), "No watch statistics found")
		return nil
	}

	return c.outputResults(u.Out(), stats)
}

func buildAccountNameByID(accounts []plexclient.Account) map[int]string {
	accountNameByID := make(map[int]string, len(accounts))
	for _, account := range accounts {
		accountNameByID[account.ID] = account.Name
	}
	return accountNameByID
}

func accountDisplayName(accountNameByID map[int]string, accountID int) string {
	accountName := accountNameByID[accountID]
	if accountName != "" {
		return accountName
	}
	return fmt.Sprintf("Account %d", accountID)
}

func watchHistoryMatchesFilters(historyEntry plexclient.HistoryEntry, cutoff time.Time, mediaType string) bool {
	if historyEntry.ViewedAt.Before(cutoff) {
		return false
	}
	if mediaType != "all" && historyEntry.Type != mediaType {
		return false
	}
	return true
}

func (c *WatchStatsCmd) outputResults(w io.Writer, stats []WatchStat) error {
	formatter := outfmt.NewFormatter(outfmt.Format(c.Output))

	header := []string{"NAME", "TYPE", "PLAYS"}
	rows := make([][]string, len(stats))

	for i, s := range stats {
		rows[i] = []string{
			s.Name,
			s.Type,
			fmt.Sprintf("%d", s.Count),
		}
	}

	return formatter.Format(w, header, rows, stats)
}
