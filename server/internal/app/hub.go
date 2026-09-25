package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

const maxTouchBody = 1 << 20

type touchEvent struct {
	TMs    int64   `json:"t_ms"`
	Action string  `json:"action"`
	ID     int     `json:"id"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	PX     float64 `json:"px"`
	PY     float64 `json:"py"`
	ViewW  int     `json:"view_w"`
	ViewH  int     `json:"view_h"`
}

type touchBatch struct {
	Events []touchEvent `json:"events"`
}

// hub serves the current image on GET and records touch events on POST.
// Both use the request URL as-is; the tablet is configured with one address.
type hub struct {
	mu     sync.RWMutex
	body   []byte
	ctype  string
	imgW   int
	imgH   int
	gen    int64
	frames []frame
	cur    int
	log    *log.Logger
}

type frame struct {
	name  string
	body  []byte
	ctype string
	w     int
	h     int
}

func newHub(logger *log.Logger) *hub {
	if logger == nil {
		logger = log.New(io.Discard, "", 0)
	}
	return &hub{log: logger}
}

func (h *hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.serveImage(w, r)
	case http.MethodPost:
		h.serveTouch(w, r)
	default:
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *hub) serveImage(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	body, ctype, gen := h.body, h.ctype, h.gen
	h.mu.RUnlock()

	if body == nil {
		http.Error(w, "no image", http.StatusNotFound)
		return
	}
	etag := fmt.Sprintf("\"%d\"", gen)
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "no-cache")
	if etagMatch(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(body)
}

func (h *hub) serveTouch(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxTouchBody))
	if err != nil {
		http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
		return
	}
	var batch touchBatch
	if err := json.Unmarshal(raw, &batch); err != nil {
		http.Error(w, "expected JSON object with an events array", http.StatusBadRequest)
		return
	}
	if len(batch.Events) > 500 {
		http.Error(w, "too many events", http.StatusBadRequest)
		return
	}
	pressed := false
	for _, ev := range batch.Events {
		h.logPress(ev)
		if ev.Action == "down" {
			pressed = true
		}
	}
	if pressed {
		h.toggle()
	}

	h.mu.RLock()
	gen := h.gen
	h.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_ = json.NewEncoder(w).Encode(struct {
		Generation int64 `json:"generation"`
	}{Generation: gen})
}

// showPNG replaces the served image and bumps the generation.
func (h *hub) showPNG(name string, data []byte) {
	w, ht := imageSize(data)
	h.mu.Lock()
	defer h.mu.Unlock()
	h.frames = nil
	h.body = append([]byte(nil), data...)
	h.ctype = "image/png"
	h.imgW, h.imgH = w, ht
	h.gen++
	//h.log.Printf("showing %s", name)
}

// useFrames serves the frames in order. A press toggles to the next one.
func (h *hub) useFrames(frames []frame) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.frames = frames
	if len(frames) == 0 {
		return
	}
	h.applyLocked(0)
}

// loadPair reads two images and serves the first until a press toggles them.
func (h *hub) loadPair(pathA, pathB string) error {
	a, err := readFrame(pathA)
	if err != nil {
		return err
	}
	b, err := readFrame(pathB)
	if err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.frames = []frame{a, b}
	h.applyLocked(0)
	return nil
}

// toggle swaps to the other image. One press advances one frame.
func (h *hub) toggle() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.frames) < 2 {
		return
	}
	h.applyLocked((h.cur + 1) % len(h.frames))
	h.log.Printf("showing %s", h.frames[h.cur].name)
}

func (h *hub) applyLocked(i int) {
	f := h.frames[i]
	h.cur = i
	h.body = f.body
	h.ctype = f.ctype
	h.imgW = f.w
	h.imgH = f.h
	h.gen++
}

func readFrame(path string) (frame, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return frame{}, err
	}
	if !imageReady(data) {
		return frame{}, fmt.Errorf("%s is not a complete png, jpeg, gif, or webp", path)
	}
	w, ht := imageSize(data)
	return frame{
		name:  path,
		body:  data,
		ctype: sniff(data),
		w:     w,
		h:     ht,
	}, nil
}

// set replaces the served image when the bytes change and bumps the generation.
func (h *hub) set(data []byte) {
	if !imageReady(data) {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if bytes.Equal(data, h.body) {
		return
	}
	next := make([]byte, len(data))
	copy(next, data)
	h.body = next
	h.ctype = sniff(next)
	h.imgW, h.imgH = imageSize(next)
	h.gen++
}

// clear drops the image and bumps the generation so clients refetch.
func (h *hub) clear() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.body == nil {
		return
	}
	h.body = nil
	h.ctype = ""
	h.imgW, h.imgH = 0, 0
	h.gen++
}

// logPress records a finger going down and where it landed on the served image.
// x and y are fractions of that image. Moves are not presses.
func (h *hub) logPress(ev touchEvent) {
	if ev.Action != "down" {
		return
	}
	h.mu.RLock()
	iw, ih := h.imgW, h.imgH
	h.mu.RUnlock()
	if iw > 0 && ih > 0 {
		h.log.Printf("press pointer=%d image=%.0f,%.0f norm=%.3f,%.3f view=%.0f,%.0f",
			ev.ID, ev.X*float64(iw), ev.Y*float64(ih), ev.X, ev.Y, ev.PX, ev.PY)
		return
	}
	h.log.Printf("press pointer=%d norm=%.3f,%.3f view=%.0f,%.0f",
		ev.ID, ev.X, ev.Y, ev.PX, ev.PY)
}

func imageSize(data []byte) (int, int) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}

// watchImage polls path until stop is closed. A nil stop channel watches until the process exits.
func watchImage(path string, h *hub, stop <-chan struct{}) {
	var mod time.Time
	var size int64 = -1
	load := func() {
		info, err := os.Stat(path)
		if err != nil {
			h.clear()
			mod = time.Time{}
			size = -1
			return
		}
		if info.ModTime().Equal(mod) && info.Size() == size {
			return
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return
		}
		if !imageReady(data) {
			return
		}
		h.set(data)
		mod = info.ModTime()
		size = info.Size()
	}
	load()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			load()
		}
	}
}

func etagMatch(header, etag string) bool {
	if header == "" || etag == "" {
		return false
	}
	for _, part := range strings.Split(header, ",") {
		p := strings.TrimSpace(part)
		if p == "*" || p == etag {
			return true
		}
	}
	return false
}

func sniff(b []byte) string {
	if len(b) >= 8 && bytes.Equal(b[:8], []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}) {
		return "image/png"
	}
	if len(b) >= 3 && b[0] == 0xff && b[1] == 0xd8 && b[2] == 0xff {
		return "image/jpeg"
	}
	if len(b) >= 6 && (bytes.HasPrefix(b, []byte("GIF87a")) || bytes.HasPrefix(b, []byte("GIF89a"))) {
		return "image/gif"
	}
	if len(b) >= 12 && bytes.Equal(b[:4], []byte("RIFF")) && bytes.Equal(b[8:12], []byte("WEBP")) {
		return "image/webp"
	}
	return "application/octet-stream"
}

// imageReady reports whether b looks like a finished image rather than a partial write.
func imageReady(b []byte) bool {
	switch sniff(b) {
	case "image/png":
		return bytes.Contains(b, []byte("IEND"))
	case "image/jpeg":
		return len(b) >= 4 && b[len(b)-2] == 0xff && b[len(b)-1] == 0xd9
	case "image/gif":
		return len(b) > 6 && b[len(b)-1] == ';'
	case "image/webp":
		return len(b) >= 12
	default:
		return false
	}
}
