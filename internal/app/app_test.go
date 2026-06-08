package app

import (
	"testing"

	"github.com/aohoyd/aku/internal/theme"
)

func TestThemeColorOrNil(t *testing.T) {
	if got := themeColorOrNil(theme.Color("")); got != nil {
		t.Fatalf("empty theme.Color should map to nil, got %v", got)
	}
	if got := themeColorOrNil(theme.Color("#0000AA")); got == nil {
		t.Fatal("non-empty theme.Color should map to a non-nil color.Color")
	}
}

func TestViewBackgroundUnsetByDefault(t *testing.T) {
	saved := theme.Background
	defer func() { theme.Background = saved }()

	theme.Background = ""
	a := newTestApp()
	if v := a.View(); v.BackgroundColor != nil {
		t.Fatalf("View().BackgroundColor should be nil when theme.Background is empty, got %v", v.BackgroundColor)
	}
}

func TestViewBackgroundSetWhenThemed(t *testing.T) {
	saved := theme.Background
	defer func() { theme.Background = saved }()

	theme.Background = theme.Color("#0000AA")
	a := newTestApp()
	v := a.View()
	if v.BackgroundColor == nil {
		t.Fatal("View().BackgroundColor should be non-nil when theme.Background is set")
	}
	wantR, wantG, wantB, wantA := theme.Color("#0000AA").RGBA()
	gotR, gotG, gotB, gotA := v.BackgroundColor.RGBA()
	if gotR != wantR || gotG != wantG || gotB != wantB || gotA != wantA {
		t.Fatalf("View().BackgroundColor RGBA = (%d,%d,%d,%d), want (%d,%d,%d,%d)",
			gotR, gotG, gotB, gotA, wantR, wantG, wantB, wantA)
	}
}

func TestViewForegroundUnsetByDefault(t *testing.T) {
	saved := theme.Foreground
	defer func() { theme.Foreground = saved }()

	theme.Foreground = ""
	a := newTestApp()
	if v := a.View(); v.ForegroundColor != nil {
		t.Fatalf("View().ForegroundColor should be nil when theme.Foreground is empty, got %v", v.ForegroundColor)
	}
}

func TestViewForegroundSetWhenThemed(t *testing.T) {
	saved := theme.Foreground
	defer func() { theme.Foreground = saved }()

	theme.Foreground = theme.Color("#C8C093")
	a := newTestApp()
	v := a.View()
	if v.ForegroundColor == nil {
		t.Fatal("View().ForegroundColor should be non-nil when theme.Foreground is set")
	}
	wantR, wantG, wantB, wantA := theme.Color("#C8C093").RGBA()
	gotR, gotG, gotB, gotA := v.ForegroundColor.RGBA()
	if gotR != wantR || gotG != wantG || gotB != wantB || gotA != wantA {
		t.Fatalf("View().ForegroundColor RGBA = (%d,%d,%d,%d), want (%d,%d,%d,%d)",
			gotR, gotG, gotB, gotA, wantR, wantG, wantB, wantA)
	}
}
