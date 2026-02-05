package ui

import (
	"context"
	"io"
	"os"

	"github.com/muesli/termenv"
)

// ColorMode defines how colors should be handled
type ColorMode int

const (
	ColorAuto ColorMode = iota
	ColorAlways
	ColorNever
)

// Options configures the UI
type Options struct {
	Out       io.Writer
	Err       io.Writer
	ColorMode ColorMode
}

// UI handles output and color support
type UI struct {
	out     io.Writer
	err     io.Writer
	profile termenv.Profile
}

// New creates a new UI with the given options
func New(opts Options) *UI {
	if opts.Out == nil {
		opts.Out = os.Stdout
	}
	if opts.Err == nil {
		opts.Err = os.Stderr
	}

	ui := &UI{
		out: opts.Out,
		err: opts.Err,
	}

	switch opts.ColorMode {
	case ColorAlways:
		ui.profile = termenv.TrueColor
	case ColorNever:
		ui.profile = termenv.Ascii
	default:
		// Auto-detect
		ui.profile = termenv.DefaultOutput().Profile
	}

	return ui
}

// Out returns the stdout writer
func (u *UI) Out() io.Writer {
	return u.out
}

// Err returns the stderr writer
func (u *UI) Err() io.Writer {
	return u.err
}

// Profile returns the termenv profile for color support
func (u *UI) Profile() termenv.Profile {
	return u.profile
}

var uiContextKey = struct{}{}

// WithUI adds the UI to the context
func WithUI(ctx context.Context, u *UI) context.Context {
	return context.WithValue(ctx, uiContextKey, u)
}

// FromContext retrieves the UI from the context
func FromContext(ctx context.Context) *UI {
	if u, ok := ctx.Value(uiContextKey).(*UI); ok {
		return u
	}
	return nil
}
