package app

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/kardianos/service"
	"github.com/mlctrez/servicego"
)

// Service is the long-running process. Start draws the first frame, binds the
// listen address, and returns while the HTTP server and the minute loop run.
type Service struct {
	servicego.Defaults

	hub      *hub
	demo     *timeTempDemo
	listener net.Listener
	server   *http.Server
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

var _ servicego.Service = (*Service)(nil)

// Config names the installed unit after the executable and describes it.
func (s *Service) Config() *service.Config {
	cfg := s.DefaultConfig.Config()
	cfg.Description = "Nexus 7 time and temperature screen"
	return cfg
}

// Start loads the development .env, publishes the first frame, and listens.
func (s *Service) Start(service.Service) error {
	if err := loadEnv(); err != nil {
		s.Errorf("env: %v", err)
	}
	addr := listenAddr()
	s.hub = newHub(log.New(serviceWriter{s}, "", 0))
	s.demo = newTimeTempDemo()
	if err := s.demo.publish(s.hub, time.Now()); err != nil {
		return err
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	s.listener = ln
	s.server = &http.Server{
		Handler:           s.hub,
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.wg.Add(2)
	go s.serve()
	go s.loop(ctx)
	s.Infof("listening on %s", ln.Addr().String())
	return nil
}

// Stop cancels the minute loop and shuts the listener down.
func (s *Service) Stop(service.Service) error {
	if s.cancel != nil {
		s.cancel()
	}
	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := s.server.Shutdown(ctx)
		cancel()
		if err != nil {
			s.Errorf("http shutdown: %v", err)
		}
	}
	s.wg.Wait()
	return nil
}

func (s *Service) serve() {
	defer s.wg.Done()
	err := s.server.Serve(s.listener)
	if err != nil && err != http.ErrServerClosed {
		s.Errorf("http: %v", err)
	}
}

func (s *Service) loop(ctx context.Context) {
	defer s.wg.Done()
	s.demo.tick(ctx, s.hub)
}

// listenAddr is LISTEN, or :8080 when that variable is empty.
func listenAddr() string {
	if v := os.Getenv("LISTEN"); v != "" {
		return v
	}
	return ":8080"
}

type serviceWriter struct{ s *Service }

func (w serviceWriter) Write(p []byte) (int, error) {
	msg := strings.TrimRight(string(p), "\n")
	if msg != "" {
		w.s.Infof("%s", msg)
	}
	return len(p), nil
}
