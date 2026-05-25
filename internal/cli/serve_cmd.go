package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/tzone85/themis/internal/api"
)

func newServeCmd() *cobra.Command {
	var base, addr string
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the read-only REST API server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServe(cmd, base, addr)
		},
	}
	cmd.Flags().StringVar(&base, "base", "", "base state directory")
	cmd.Flags().StringVar(&addr, "addr", ":8787", "listen address")
	_ = cmd.MarkFlagRequired("base")
	return cmd
}

// runServe binds and serves, then waits for SIGINT/SIGTERM for clean
// shutdown. Kept as a separate function so tests can drive it without
// installing signal handlers.
func runServe(cmd *cobra.Command, base, addr string) error {
	mux := api.NewMux(base)
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "themis: serving on %s\n", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}
