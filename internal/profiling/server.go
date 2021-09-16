package profiling

import (
	"context"
	"errors"
	"net/http"
	"net/http/pprof"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"
)

// RunServer runs a server with pprof debugging endpoints
func RunServer(ctx context.Context, logger hclog.Logger, address string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	server := &http.Server{
		Addr:    address,
		Handler: mux,
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			// graceful shutdown failed, exit
			logger.Error("error shutting down metrics server", "error", err)
		}
	}()
	defer wg.Wait()

	if err := server.ListenAndServe(); err != nil {
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}
	return nil
}
