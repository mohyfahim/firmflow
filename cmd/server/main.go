package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"firmflow/internal/bootstrap"
)

func main() {
	app, err := bootstrap.New()
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = app.Close()
	}()

	go func() {
		app.Logger.WithField("addr", app.HTTPServer.Addr).Info("http server starting")
		if err := app.HTTPServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			app.Logger.WithError(err).Fatal("http server failed")
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), app.Config.HTTP.ShutdownTimeout)
	defer cancel()

	app.Logger.Info("shutting down http server")
	app.StopSchedulers()
	app.StopOTA()
	if err := app.HTTPServer.Shutdown(ctx); err != nil {
		app.Logger.WithError(err).Error("http server graceful shutdown failed")
		return
	}
	app.Logger.Info("http server stopped cleanly")
}
