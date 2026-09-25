package app

import (
	"context"
	"time"

	"github.com/gogpu/ui/primitives"
	"github.com/gogpu/ui/theme/material3"
	"github.com/gogpu/ui/widget"
)

// timeTempScreen is a 1920×1200 dark layout. The clock, weekday, and date
// come from now. Temperatures come from view, which stays empty until a
// weather fetch has succeeded.
func timeTempScreen(theme *material3.Theme, now time.Time, view weatherView) widget.Widget {
	ink := theme.Colors.OnSurface
	muted := theme.Colors.OnSurfaceVariant
	current, high, low, condition := "—", "—", "—", "—"
	days := make([]dayReading, 5)
	for i := range days {
		days[i] = dayReading{name: now.AddDate(0, 0, i+1).Format("Mon"), high: "—", low: "—"}
	}
	if view.ready {
		current, high, low = view.current, view.todayHigh, view.todayLow
		if view.condition != "" {
			condition = view.condition
		}
		for i, day := range view.days {
			if i >= len(days) {
				break
			}
			days[i] = day
		}
	}
	tiles := make([]widget.Widget, len(days))
	for i, day := range days {
		tiles[i] = dayTile(theme, day.name, day.high, day.low, day.condition)
	}
	return primitives.VBox(
		primitives.HBox(
			primitives.HBox(
				primitives.Text(now.Format("3:04")).FontSize(168).Bold(),
				primitives.Text(now.Format("PM")).FontSize(48).Color(muted),
			).Gap(20),
			primitives.Expanded(primitives.Box()),
			primitives.VBox(
				primitives.Text(now.Format("Monday")).FontSize(88),
				primitives.Text(now.Format("January 2")).FontSize(64).Color(muted),
			).Gap(8).CrossAlign(primitives.CrossAxisEnd),
		),
		primitives.HBox(
			tempFigure(current, "", 168, 0, theme.Colors.Primary, muted),
			primitives.VBox(
				primitives.Box().Height(83),
				primitives.HBox(
					tempFigure(high, "High", 64, 0, ink, muted),
					tempFigure(low, "Low", 64, 0, ink, muted),
				).Gap(40),
			),
		).Gap(64),
		primitives.Text(condition).FontSize(72).Color(ink),
		primitives.Expanded(primitives.Box()),
		primitives.Text("5 day forecast").FontSize(64).Color(muted),
		primitives.HBox(tiles...).Gap(24),
	).Width(screenW).Height(screenH).Padding(64).Gap(28).Background(theme.Colors.Background)
}

func tempFigure(value, label string, valueSize, padTop float32, valueColor, labelColor widget.Color) *primitives.BoxWidget {
	children := []widget.Widget{}
	if padTop > 0 {
		children = append(children, primitives.Box().Height(padTop))
	}
	children = append(children, primitives.Text(value).FontSize(valueSize).Bold().Color(valueColor))
	if label != "" {
		labelSize := float32(28)
		if valueSize >= 120 {
			labelSize = 32
		}
		children = append(children, primitives.Text(label).FontSize(labelSize).Color(labelColor))
	}
	return primitives.VBox(children...).Gap(8)
}

func dayTile(theme *material3.Theme, day, high, low, condition string) *primitives.BoxWidget {
	if condition == "" {
		condition = "—"
	}
	return primitives.VBox(
		primitives.Text(day).FontSize(54).Color(theme.Colors.OnSurfaceVariant),
		primitives.Text(high).FontSize(72).Bold(),
		primitives.Text(low).FontSize(40).Color(theme.Colors.OnSurfaceVariant),
		primitives.Text(condition).FontSize(32).Color(theme.Colors.OnSurface),
	).Width(336).Padding(28).Gap(12).
		Background(theme.Colors.SurfaceContainer).
		Rounded(24).
		CrossAlign(primitives.CrossAxisCenter)
}

// timeTempDemo redraws the clock each minute and refreshes temperatures
// on the half hour from 6:00 AM through 11:00 PM.
type timeTempDemo struct {
	theme     *material3.Theme
	view      weatherView
	nextFetch time.Time
}

func newTimeTempDemo() *timeTempDemo {
	return &timeTempDemo{theme: material3.NewDark(widget.Hex(0x8AB4F8))}
}

func (d *timeTempDemo) renderAt(now time.Time) ([]byte, error) {
	return renderScreen(timeTempScreen(d.theme, now, d.view), d.theme)
}

func (d *timeTempDemo) considerWeather(h *hub, now time.Time) {
	if !weatherAllowed(now) {
		return
	}
	if !d.nextFetch.IsZero() && now.Before(d.nextFetch) {
		return
	}
	view, err := fetchWeather(now)
	if err != nil {
		h.log.Printf("weather: %v", err)
		d.nextFetch = nextHalfHour(now)
		return
	}
	d.view = view
	d.nextFetch = nextHalfHour(now)
	h.log.Printf("weather updated, next fetch %s", d.nextFetch.Format("15:04"))
}

// publish redraws the screen and, when a weather slot is due, refreshes temps first.
func (d *timeTempDemo) publish(h *hub, now time.Time) error {
	d.considerWeather(h, now)
	raw, err := d.renderAt(now)
	if err != nil {
		return err
	}
	h.showPNG(now.Format("3:04 PM"), raw)
	return nil
}

// tick waits until the top of each minute, then publishes a new frame.
// A cancelled context ends the wait and returns before the next publish.
func (d *timeTempDemo) tick(ctx context.Context, h *hub) {
	for {
		now := time.Now()
		timer := time.NewTimer(time.Until(now.Truncate(time.Minute).Add(time.Minute)))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		if err := d.publish(h, time.Now()); err != nil {
			h.log.Printf("render: %v", err)
		}
	}
}
