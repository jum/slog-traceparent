# slog-traceparent

The slog-traceparent package contains a simple middleware for the slog
package to parse an incoming traceparent header and put the
traceinfomation into current context. The
[slog-context](github.com/veqryn/slog-context) package can be used to
append or prepend the trace information to the log record.

An example how to use this in a go app:

```go
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	traceparent "github.com/jum/slog-traceparent"
	slogctx "github.com/veqryn/slog-context"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

func main() {
	debug := os.Getenv("NODE_ENV") == "development"
	level := new(slog.LevelVar) // Info by default
	if debug {
		level.Set(slog.LevelDebug)
	}
	shandler := slogctx.NewHandler(
		slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
			Level: level,
		}),
		&slogctx.HandlerOptions{
			// Prependers stays as default (leaving as nil would accomplish the same)
			Prependers: []slogctx.AttrExtractor{
				slogctx.ExtractPrepended,
			},
			// Appenders first appends anything added with slogctx.Append,
			// then appends our custom ctx value
			Appenders: []slogctx.AttrExtractor{
				slogctx.ExtractAppended,
				traceparent.TraceParentExtractor,
			},
		},
	)
	logger := slog.New(shandler)
	slog.SetDefault(logger)
	port := os.Getenv("PORT")
	if port == "" {
		port = "8089"
		slog.Debug("Defaulting", "port", port)
	}
	var network string
	var addr string
	if strings.HasPrefix(port, "/") {
		network = "unix"
		addr = port
		err := os.Remove(addr)
		if err != nil && !os.IsNotExist(err) {
			slog.Error("remove unix socket", "err", err)
		}
		defer os.Remove(addr)
		slog.Info("Listening", "addr", addr)
	} else {
		network = "tcp"
		addr = fmt.Sprintf(":%s", port)
		// Output somthing to cmd-click on
		slog.Info("Listening", "port", port, "url", fmt.Sprintf("http://localhost:%s/", port))
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		slog.DebugContext(ctx, "incoming", "parent", r.Header.Get("traceparent"))
		fmt.Fprintf(w, "Hello, World!\n")
	})
	h2s := &http2.Server{}
	srv := http.Server{
		Addr:    addr,
		Handler: h2c.NewHandler(traceparent.New(mux), h2s),
	}
	listener, err := net.Listen(network, addr)
	if err != nil {
		slog.Error("init listen", "err", err)
		os.Exit(1)
	}
	if network == "unix" {
		err := os.Chmod(addr, 0666)
		if err != nil {
			slog.Error("chmod", "addr", addr, "err", err)
		}
	}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		if err := srv.Serve(listener); err != nil {
			if err != http.ErrServerClosed {
				slog.Error("Serve", "err", err)
			}
		}
	}()
	<-stop
	slog.Debug("Shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("Shutdown", "err", err)
	}
}
```
