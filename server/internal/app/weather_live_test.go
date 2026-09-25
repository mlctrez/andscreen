//go:build live

package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gogpu/ui/theme/material3"
	"github.com/gogpu/ui/widget"
)

func TestLiveWeatherPreview(t *testing.T) {
	if err := loadEnv(); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	view, err := fetchWeather(now)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("current %s %s today %s/%s days %d", view.current, view.condition, view.todayHigh, view.todayLow, len(view.days))
	for _, d := range view.days {
		t.Logf("%s %s %s", d.name, d.high, d.low)
	}
	theme := material3.NewDark(widget.Hex(0x8AB4F8))
	raw, err := renderScreen(timeTempScreen(theme, now, view), theme)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(moduleRoot(t), "time-temp.png"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
}
