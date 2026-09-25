package app

import (
	"bytes"
	"image"
	"image/color"
	"image/png"

	"github.com/gogpu/ui/core/button"
	"github.com/gogpu/ui/event"
	"github.com/gogpu/ui/geometry"
	"github.com/gogpu/ui/offscreen"
	"github.com/gogpu/ui/primitives"
	"github.com/gogpu/ui/theme/material3"
	"github.com/gogpu/ui/widget"
)

const (
	screenW = 1920
	screenH = 1200
)

// sampleFrames renders the two dark example screens shown on the tablet.
// A press swaps between them. The controls are drawn only; touches are not
// routed into individual widgets.
func sampleFrames() ([]frame, error) {
	theme := material3.NewDark(widget.Hex(0x8AB4F8))
	overview, err := renderScreen(overviewScreen(theme), theme)
	if err != nil {
		return nil, err
	}
	schedule, err := renderScreen(scheduleScreen(theme), theme)
	if err != nil {
		return nil, err
	}
	return []frame{
		pngFrame("overview", overview),
		pngFrame("schedule", schedule),
	}, nil
}

func renderScreen(root widget.Widget, theme *material3.Theme) ([]byte, error) {
	renderer := offscreen.NewRenderer(screenW, screenH,
		offscreen.WithTheme(theme),
		offscreen.WithBackground(theme.Colors.Background),
	)
	renderer.Render(root)
	var buf bytes.Buffer
	if err := png.Encode(&buf, renderer.Image()); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func overviewScreen(theme *material3.Theme) widget.Widget {
	return page(theme,
		primitives.Text("Andscreen").FontSize(64).Bold(),
		primitives.Text("Overview").FontSize(32).Color(theme.Colors.OnSurfaceVariant),
		buttonRow(
			m3Button(theme, "Arm", button.Filled),
			m3Button(theme, "Home", button.Tonal),
			m3Button(theme, "Lights", button.Outlined),
		),
		primitives.HBox(
			primitives.VBox(
				primitives.Text("Front room").FontSize(36).Bold(),
				primitives.Text("The picture sits in this layout.").FontSize(26),
				primitives.Text("A touch swaps to the other sample.").FontSize(26),
				primitives.Text("The buttons are drawn, not live controls.").FontSize(26),
			).Width(860).Gap(16),
			newPlacedImage(nightPicture()),
		).Gap(48).CrossAlign(primitives.CrossAxisCenter),
	)
}

func scheduleScreen(theme *material3.Theme) widget.Widget {
	return page(theme,
		primitives.Text("Tonight").FontSize(64).Bold(),
		primitives.Text("Schedule").FontSize(32).Color(theme.Colors.OnSurfaceVariant),
		buttonRow(
			m3Button(theme, "Earlier", button.Outlined),
			m3Button(theme, "Later", button.Filled),
			m3Button(theme, "Cancel", button.TextOnly),
		),
		primitives.HBox(
			newPlacedImage(dialPicture()),
			primitives.VBox(
				primitives.Text("After 18:00").FontSize(36).Bold(),
				primitives.Text("Lights ease down. The entry stays on. Touch anywhere to return to Overview.").FontSize(26),
			).Width(860).Gap(16),
		).Gap(48).CrossAlign(primitives.CrossAxisCenter),
	)
}

func page(theme *material3.Theme, children ...widget.Widget) widget.Widget {
	return primitives.VBox(children...).
		Width(screenW).
		Height(screenH).
		Padding(56).
		Gap(28).
		Background(theme.Colors.Background)
}

func buttonRow(buttons ...widget.Widget) widget.Widget {
	return primitives.HBox(buttons...).Gap(20).CrossAlign(primitives.CrossAxisCenter)
}

func m3Button(theme *material3.Theme, label string, variant button.Variant) widget.Widget {
	return button.New(
		button.TextOpt(label),
		button.VariantOpt(variant),
		button.SizeOpt(button.Large),
		button.PainterOpt(material3.ButtonPainter{Theme: theme}),
	)
}

// placedImage draws a raster through the toolkit canvas. The stock image
// widget still paints a placeholder, so this one calls DrawImage.
type placedImage struct {
	widget.WidgetBase
	img *image.RGBA
}

func newPlacedImage(img *image.RGBA) *placedImage {
	p := &placedImage{img: img}
	p.SetVisible(true)
	p.SetEnabled(true)
	return p
}

func (p *placedImage) Layout(_ widget.Context, constraints geometry.Constraints) geometry.Size {
	b := p.img.Bounds()
	size := constraints.Constrain(geometry.Sz(float32(b.Dx()), float32(b.Dy())))
	p.SetBounds(geometry.FromPointSize(p.Position(), size))
	return size
}

func (p *placedImage) Draw(_ widget.Context, canvas widget.Canvas) {
	if p.img == nil || !p.IsVisible() {
		return
	}
	canvas.DrawImage(p.img, p.Bounds().Min)
}

func (p *placedImage) Event(widget.Context, event.Event) bool { return false }

func (p *placedImage) Children() []widget.Widget { return nil }

func nightPicture() *image.RGBA {
	const w, h = 720, 420
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			sky := uint8(18 + y/8)
			c := color.RGBA{R: 12, G: 18, B: sky, A: 255}
			if y > h*2/3 {
				c = color.RGBA{R: 42, G: 28, B: 18, A: 255}
			}
			img.SetRGBA(x, y, c)
		}
	}
	for _, s := range [][2]int{{80, 60}, {180, 110}, {310, 40}, {470, 90}, {600, 50}, {140, 160}} {
		img.SetRGBA(s[0], s[1], color.RGBA{R: 230, G: 230, B: 210, A: 255})
		img.SetRGBA(s[0]+1, s[1], color.RGBA{R: 230, G: 230, B: 210, A: 255})
	}
	horizon := h * 2 / 3
	for x := 0; x < w; x++ {
		img.SetRGBA(x, horizon, color.RGBA{R: 210, G: 120, B: 48, A: 255})
		img.SetRGBA(x, horizon+1, color.RGBA{R: 210, G: 120, B: 48, A: 255})
	}
	return img
}

func dialPicture() *image.RGBA {
	const w, h = 420, 420
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	bg := color.RGBA{R: 28, G: 32, B: 38, A: 255}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, bg)
		}
	}
	cx, cy := w/2, h/2
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dx, dy := x-cx, y-cy
			r2 := dx*dx + dy*dy
			if r2 > 150*150 && r2 < 168*168 {
				img.SetRGBA(x, y, color.RGBA{R: 138, G: 180, B: 248, A: 255})
			}
			if r2 < 8*8 {
				img.SetRGBA(x, y, color.RGBA{R: 232, G: 234, B: 237, A: 255})
			}
		}
	}
	for i := 0; i < 120; i++ {
		img.SetRGBA(cx+i, cy-40, color.RGBA{R: 232, G: 234, B: 237, A: 255})
	}
	return img
}

func pngFrame(name string, data []byte) frame {
	w, h := imageSize(data)
	return frame{
		name:  name,
		body:  data,
		ctype: "image/png",
		w:     w,
		h:     h,
	}
}
