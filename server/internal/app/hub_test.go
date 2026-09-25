package app

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gogpu/ui/theme/material3"
	"github.com/gogpu/ui/widget"
)

func testPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestGetAndTouch(t *testing.T) {
	var logs bytes.Buffer
	h := newHub(log.New(&logs, "", 0))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("empty GET status %d", rec.Code)
	}

	pngBytes := testPNG(t)
	h.set(pngBytes)

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://tablet/screen", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status %d", rec.Code)
	}
	if rec.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("content type %q", rec.Header().Get("Content-Type"))
	}
	etag := rec.Header().Get("ETag")
	if etag != `"1"` {
		t.Fatalf("etag %q", etag)
	}
	if !bytes.Equal(rec.Body.Bytes(), pngBytes) {
		t.Fatal("body mismatch")
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("If-None-Match", etag)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotModified {
		t.Fatalf("conditional GET status %d", rec.Code)
	}

	body := []byte(`{"events":[{"t_ms":10,"action":"down","id":0,"x":0.5,"y":0.25,"px":100,"py":50,"view_w":1920,"view_h":1200},{"t_ms":20,"action":"move","id":0,"x":0.6,"y":0.3,"px":110,"py":60,"view_w":1920,"view_h":1200}]}`)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("POST status %d body %s", rec.Code, rec.Body.Bytes())
	}
	var resp struct {
		Generation int64 `json:"generation"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Generation != 1 {
		t.Fatalf("generation %d", resp.Generation)
	}
	if strings.Count(logs.String(), "press ") != 1 {
		t.Fatalf("log %q", logs.String())
	}
	// 4×4 test image: the press is halfway across and a quarter of the way down.
	if !strings.Contains(logs.String(), "image=2,1") || !strings.Contains(logs.String(), "norm=0.500,0.250") {
		t.Fatalf("log %q", logs.String())
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader("nope")))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad POST status %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("PUT status %d", rec.Code)
	}
}

func TestGenerationAdvancesWhenFileChanges(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "screen.png")
	if err := os.WriteFile(path, testPNG(t), 0o644); err != nil {
		t.Fatal(err)
	}
	h := newHub(nil)
	stop := make(chan struct{})
	defer close(stop)
	go watchImage(path, h, stop)

	deadline := time.Now().Add(2 * time.Second)
	for h.generation() == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if h.generation() != 1 {
		t.Fatalf("generation after create %d", h.generation())
	}

	img := image.NewRGBA(image.Rect(0, 0, 8, 2))
	img.Set(1, 0, color.RGBA{G: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	// Rename onto the watched path so the server never reads a partial file.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, path); err != nil {
		t.Fatal(err)
	}

	deadline = time.Now().Add(2 * time.Second)
	for h.generation() == 1 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if h.generation() != 2 {
		t.Fatalf("generation after replace %d", h.generation())
	}
}

func (h *hub) generation() int64 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.gen
}

func TestPressTogglesImages(t *testing.T) {
	dir := t.TempDir()
	aPath := filepath.Join(dir, "a.png")
	bPath := filepath.Join(dir, "b.png")
	if err := os.WriteFile(aPath, solidPNG(t, color.RGBA{R: 255, A: 255}), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bPath, solidPNG(t, color.RGBA{G: 255, A: 255}), 0o644); err != nil {
		t.Fatal(err)
	}

	var logs bytes.Buffer
	h := newHub(log.New(&logs, "", 0))
	if err := h.loadPair(aPath, bPath); err != nil {
		t.Fatal(err)
	}

	first := getBody(t, h)
	postEvents(t, h, `{"events":[{"t_ms":1,"action":"move","id":0,"x":0.1,"y":0.1,"px":1,"py":1,"view_w":4,"view_h":4}]}`)
	if afterMove := getBody(t, h); !bytes.Equal(afterMove, first) {
		t.Fatal("a move swapped the image")
	}

	postEvents(t, h, `{"events":[{"t_ms":2,"action":"down","id":0,"x":0.5,"y":0.5,"px":2,"py":2,"view_w":4,"view_h":4},{"t_ms":3,"action":"move","id":0,"x":0.6,"y":0.6,"px":3,"py":3,"view_w":4,"view_h":4}]}`)
	second := getBody(t, h)
	if bytes.Equal(second, first) {
		t.Fatal("press did not swap the image")
	}
	if strings.Count(logs.String(), "showing ") != 1 {
		t.Fatalf("log %q", logs.String())
	}

	postEvents(t, h, `{"events":[{"t_ms":4,"action":"down","id":1,"x":0.2,"y":0.2,"px":1,"py":1,"view_w":4,"view_h":4}]}`)
	if back := getBody(t, h); !bytes.Equal(back, first) {
		t.Fatal("second press did not restore the first image")
	}
}

func solidPNG(t *testing.T, c color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func getBody(t *testing.T, h *hub) []byte {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status %d", rec.Code)
	}
	return rec.Body.Bytes()
}

func postEvents(t *testing.T, h *hub, body string) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("POST status %d body %s", rec.Code, rec.Body.Bytes())
	}
}

func TestSampleScreensToggle(t *testing.T) {
	frames, err := sampleFrames()
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 2 {
		t.Fatalf("frames %d", len(frames))
	}
	if frames[0].w != screenW || frames[0].h != screenH || frames[1].w != screenW || frames[1].h != screenH {
		t.Fatalf("sizes %dx%d %dx%d", frames[0].w, frames[0].h, frames[1].w, frames[1].h)
	}
	if bytes.Equal(frames[0].body, frames[1].body) {
		t.Fatal("sample screens are identical")
	}

	h := newHub(nil)
	h.useFrames(frames)
	if got := getBody(t, h); !bytes.Equal(got, frames[0].body) {
		t.Fatal("first screen was not overview")
	}
	postEvents(t, h, `{"events":[{"t_ms":1,"action":"down","id":0,"x":0.5,"y":0.5,"px":960,"py":600,"view_w":1920,"view_h":1200}]}`)
	if got := getBody(t, h); !bytes.Equal(got, frames[1].body) {
		t.Fatal("press did not show the schedule screen")
	}
}

func TestTimeTempScreen(t *testing.T) {
	theme := material3.NewDark(widget.Hex(0x8AB4F8))
	at := time.Date(2026, 9, 23, 18, 42, 10, 0, time.Local)
	raw, err := renderScreen(timeTempScreen(theme, at, weatherView{}), theme)
	if err != nil {
		t.Fatal(err)
	}
	w, h := imageSize(raw)
	if w != screenW || h != screenH {
		t.Fatalf("size %dx%d", w, h)
	}
	again, err := renderScreen(timeTempScreen(theme, at, weatherView{}), theme)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, again) {
		t.Fatal("the same minute rendered two different images")
	}
	next, err := renderScreen(timeTempScreen(theme, at.Add(time.Minute), weatherView{}), theme)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(raw, next) {
		t.Fatal("advancing a minute did not change the image")
	}
	tomorrow := at.AddDate(0, 0, 1)
	rolled, err := renderScreen(timeTempScreen(theme, tomorrow, weatherView{}), theme)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(raw, rolled) {
		t.Fatal("advancing a day did not change the image")
	}
}

func TestImageReadyRejectsPartialPNG(t *testing.T) {
	pngBytes := testPNG(t)
	if !imageReady(pngBytes) {
		t.Fatal("complete png rejected")
	}
	if imageReady(pngBytes[:8]) {
		t.Fatal("header-only png accepted")
	}
	if imageReady([]byte("hello")) {
		t.Fatal("plain text accepted")
	}
}
