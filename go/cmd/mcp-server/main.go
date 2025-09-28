package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rvazquez/ai-assisted-observability-poc/go/internal/config"
	"github.com/rvazquez/ai-assisted-observability-poc/go/internal/mcp"
)

func main() {
	config.Init(nil)

	srv := mcp.New(mcp.DefaultConfig())

	host := os.Getenv("MCP_SERVER_HOST")
	if host == "" {
		host = "0.0.0.0"
	}

	port := os.Getenv("MCP_SERVER_PORT")
	if port == "" {
		port = "8000"
	}

	addr := host + ":" + port

	httpServer := &http.Server{
		Addr:    addr,
		Handler: newLoggingMiddleware(srv.Handler),
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("MCP server listening on %s", addr)
		errCh <- httpServer.ListenAndServe()
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-stop:
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(ctx); err != nil {
			log.Fatalf("shutdown error: %v", err)
		}
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}
}

type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func newLoggingResponseWriter(w http.ResponseWriter) *loggingResponseWriter {
	return &loggingResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}

func newLoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lrw := newLoggingResponseWriter(w)
		next.ServeHTTP(lrw, r)
		elapsed := time.Since(start)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, lrw.statusCode, elapsed)
	})
}
