package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/rvazquez/ai-assisted-observability-poc/go/internal/config"
	"github.com/rvazquez/ai-assisted-observability-poc/go/internal/mcp"
)

func main() {
	root := &cobra.Command{
		Use:   "mcp-server",
		Short: "ARO-HCP MCP server",
		RunE:  run,
	}

	root.PersistentFlags().String("postgres-url", "", "Postgres connection URL")
	root.PersistentFlags().String("ollama-url", "", "Ollama base URL")
	root.PersistentFlags().String("auth-file", "", "Docker auth file for skopeo")
	root.PersistentFlags().String("cache-dir", "", "Cache directory path")
	root.PersistentFlags().Int("max-new-prs", 100, "Maximum PRs to fetch per run")
	root.PersistentFlags().String("pr-start-date", "", "PR start date (ISO-8601)")
	root.PersistentFlags().Int("port", 8000, "HTTP port")
	root.PersistentFlags().String("host", "0.0.0.0", "HTTP host")

	config.Init(root)

	if err := root.Execute(); err != nil {
		log.Fatalf("command failed: %v", err)
	}
}

func run(cmd *cobra.Command, args []string) error {
	srv := mcp.New(mcp.DefaultConfig())

	host, _ := cmd.Flags().GetString("host")
	port, _ := cmd.Flags().GetInt("port")
	addr := host + ":" + strconv.Itoa(port)

	httpServer := &http.Server{
		Addr:    addr,
		Handler: srv.Handler,
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
		return httpServer.Shutdown(ctx)
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	}
}
