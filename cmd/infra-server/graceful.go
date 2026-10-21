package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"
)

func serveUntilCancelled(ctx context.Context, server *http.Server, listener net.Listener) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		<-ctx.Done()
		shutdown, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		err := server.Shutdown(shutdown)
		if err != nil {
			_ = server.Close()
		}
		done <- err
	}()
	err := server.Serve(listener)
	cancel()
	shutdownErr := <-done
	if errors.Is(err, http.ErrServerClosed) {
		return shutdownErr
	}
	return err
}
