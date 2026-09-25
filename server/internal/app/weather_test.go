package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/briandowns/openweathermap"
)

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func TestLoadWeatherEnv(t *testing.T) {
	root := moduleRoot(t)
	if _, err := os.Stat(filepath.Join(root, ".env")); err != nil {
		t.Skip(".env is not in the module root")
	}
	t.Chdir(root)
	if err := loadEnv(); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("OWM_KEY") == "" || os.Getenv("OWM_ZIP") == "" {
		t.Fatal("OWM_KEY and OWM_ZIP are empty after loading .env")
	}
}

func TestWeatherAllowed(t *testing.T) {
	loc := time.Local
	cases := []struct {
		h, m int
		ok   bool
	}{
		{5, 59, false},
		{6, 0, true},
		{15, 30, true},
		{23, 0, true},
		{23, 1, false},
		{23, 30, false},
	}
	for _, c := range cases {
		at := time.Date(2026, 9, 24, c.h, c.m, 0, 0, loc)
		if got := weatherAllowed(at); got != c.ok {
			t.Errorf("%02d:%02d allowed=%v want %v", c.h, c.m, got, c.ok)
		}
	}
}

func TestSummarizeWeather(t *testing.T) {
	loc := time.FixedZone("CST", -6*3600)
	now := time.Date(2026, 9, 24, 15, 0, 0, 0, loc)
	list := []openweathermap.Forecast5WeatherList{
		slot(time.Date(2026, 9, 24, 18, 0, 0, 0, loc), 70, 78, "Clouds"),
		slot(time.Date(2026, 9, 24, 21, 0, 0, 0, loc), 64, 72, "Clear"),
		slot(time.Date(2026, 9, 25, 15, 0, 0, 0, loc), 55, 68, "Rain"),
		slot(time.Date(2026, 9, 25, 21, 0, 0, 0, loc), 50, 60, "Clouds"),
	}
	view := summarizeWeather(now, 80, "Clear", list)
	if view.todayHigh != "80°" || view.todayLow != "64°" {
		t.Fatalf("today %s / %s", view.todayHigh, view.todayLow)
	}
	if view.current != "80°" || view.condition != "Clear" {
		t.Fatalf("current %s %s", view.current, view.condition)
	}
	if len(view.days) != 1 || view.days[0].name != "Fri" || view.days[0].high != "68°" || view.days[0].low != "50°" || view.days[0].condition != "Rain" {
		t.Fatalf("days %+v", view.days)
	}
}

func slot(at time.Time, lo, hi float64, condition string) openweathermap.Forecast5WeatherList {
	return openweathermap.Forecast5WeatherList{
		Dt: int(at.Unix()),
		Main: openweathermap.Main{
			TempMin: lo,
			TempMax: hi,
		},
		Weather: []openweathermap.Weather{{Main: condition}},
	}
}
