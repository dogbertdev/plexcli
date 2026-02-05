package ui

import (
	"bytes"
	"testing"

	"github.com/muesli/termenv"
)

func TestNew(t *testing.T) {
	out := &bytes.Buffer{}
	err := &bytes.Buffer{}

	t.Run("Default options", func(t *testing.T) {
		ui := New(Options{Out: out, Err: err})
		if ui.Out() != out {
			t.Errorf("Expected Out to be %v, got %v", out, ui.Out())
		}
		if ui.Err() != err {
			t.Errorf("Expected Err to be %v, got %v", err, ui.Err())
		}
	})

	t.Run("Color Always", func(t *testing.T) {
		ui := New(Options{ColorMode: ColorAlways})
		if ui.Profile() != termenv.TrueColor {
			t.Errorf("Expected profile to be TrueColor, got %v", ui.Profile())
		}
	})

	t.Run("Color Never", func(t *testing.T) {
		ui := New(Options{ColorMode: ColorNever})
		if ui.Profile() != termenv.Ascii {
			t.Errorf("Expected profile to be Ascii, got %v", ui.Profile())
		}
	})
}
