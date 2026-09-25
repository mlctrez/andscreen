package app

import (
	"io"
	"net/http"
	"testing"

	"github.com/kardianos/service"
)

func TestListenAddr(t *testing.T) {
	t.Setenv("LISTEN", ":9090")
	if got := listenAddr(); got != ":9090" {
		t.Fatalf("env %s", got)
	}
	t.Setenv("LISTEN", "")
	if got := listenAddr(); got != ":8080" {
		t.Fatalf("default %s", got)
	}
}

func TestServiceStartStop(t *testing.T) {
	t.Setenv("OWM_KEY", "")
	t.Setenv("OWM_ZIP", "")
	t.Setenv("LISTEN", "127.0.0.1:0")
	s := &Service{}
	s.Logger(service.ConsoleLogger)
	if err := s.Start(nil); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := s.Stop(nil); err != nil {
			t.Fatal(err)
		}
	}()

	res, err := http.Get("http://" + s.listener.Addr().String() + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	if res.Header.Get("Content-Type") != "image/png" {
		t.Fatalf("content type %q", res.Header.Get("Content-Type"))
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) == 0 {
		t.Fatal("empty image")
	}
}
