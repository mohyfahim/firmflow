package devotcp

import (
	"context"
	"errors"
	"net"
	"sync"

	"github.com/sirupsen/logrus"
)

// Serve listens on addr until ctx is cancelled, then closes the listener.
func Serve(ctx context.Context, log *logrus.Logger, addr string, h *Handler) error {
	if h == nil {
		return errors.New("devotcp: nil handler")
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	var wg sync.WaitGroup
	defer wg.Wait()

	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	if log != nil {
		log.WithField("addr", addr).Info("device OTA tcp listening")
	}

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		wg.Add(1)
		go func(c net.Conn) {
			defer wg.Done()
			h.ServeConn(ctx, c)
		}(conn)
	}
}
