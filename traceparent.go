// Package traceparent implements a simple middleware to parse the
// HTTP traceparent header [W3C] into its parts without using OTEL or
// other big tracing packages. The trace context will be output via
// [log/slog] if using the log functions that include a [context]
// parameter as the first argument.
//
// [W3C]: https://www.w3.org/TR/trace-context/
package traceparent

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Trace contains tracing information used in logging.
type Trace struct {
	ID      string
	SpanID  string
	Sampled bool
}

// Context returns a Context that stores the Trace.
func (trace Trace) Context(ctx context.Context) context.Context {
	return context.WithValue(ctx, traceContextKeyT{}, trace)
}

type traceContextKeyT struct{}

// New creates a middleware function that will inject the
// [Trace] structure into the current requests context. To
// make this context available to the [log/slog] logging functions, be
// sure to the the variants including a [context] argument.
func New(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		traceparent := strings.Split(r.Header.Get("traceparent"), "-")
		if len(traceparent) == 4 && traceparent[0] == "00" {
			flags, err := strconv.ParseInt(traceparent[3], 16, 8)
			if err == nil {
				trace := Trace{
					ID:      traceparent[1],
					SpanID:  traceparent[2],
					Sampled: (flags & 1) != 0,
				}
				ctx := trace.Context(r.Context())
				next.ServeHTTP(w, r.WithContext(ctx))
			} else {
				next.ServeHTTP(w, r)
			}
		} else {
			next.ServeHTTP(w, r)
		}
	}
	return http.HandlerFunc(fn)
}

// TraceParentExtractor is function suitable for use as an extractor
// function for the [github.com/veqryn/slog-context] package to prepend
// or append the trace information from the context.
func TraceParentExtractor(ctx context.Context, recordT time.Time, recordLvl slog.Level, recordMsg string) []slog.Attr {
	trace, ok := ctx.Value(traceContextKeyT{}).(Trace)
	if !ok || trace.ID == "" {
		return nil
	}
	attrs := []slog.Attr{
		slog.String("traceID", trace.ID),
		slog.Bool("traceSampled", trace.Sampled),
	}
	if trace.SpanID != "" {
		attrs = append(attrs, slog.String("spanID", trace.ID))
	}
	return attrs
}
