package mediatiming

import (
	"context"
	"encoding/json"
	"github.com/unxed/f4/internal/navtrace"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
)

const (
	mediaTimingEnv       = "F4_MEDIA_TIMING_TRACE"
	mediaTimingLogPrefix = "F4_MEDIA_TIMING_TRACE "
	mediaTimingSchema    = "f4.media.v1"
)

var (
	mediaTimingEnabled atomic.Bool
	mediaTimingOutput  = struct {
		sync.Mutex
		writer io.Writer
	}{writer: os.Stderr}
)

type mediaTimingContextKey struct{}

type mediaTimingCorrelation struct {
	requestID string
	traceID   string
}

func init() {
	_, enabled := os.LookupEnv(mediaTimingEnv)
	mediaTimingEnabled.Store(enabled)
}

func mediaTimingIsEnabled() bool {
	return mediaTimingEnabled.Load()
}

func MediaTimingWithCorrelation(ctx context.Context, requestID, traceID string) context.Context {
	if !mediaTimingIsEnabled() || (requestID == "" && traceID == "") {
		return ctx
	}
	return context.WithValue(ctx, mediaTimingContextKey{}, mediaTimingCorrelation{
		requestID: requestID,
		traceID:   traceID,
	})
}

// Shared broker flights intentionally outlive an individual waiter. Preserve
// only diagnostic correlation when detaching their cancellation lifetime.
func MediaTimingDetachedContext(ctx context.Context) context.Context {
	correlation, _ := ctx.Value(mediaTimingContextKey{}).(mediaTimingCorrelation)
	return MediaTimingWithCorrelation(
		context.Background(), correlation.requestID, correlation.traceID)
}

func MediaTimingEmit(ctx context.Context, event, thread string, fields ...any) {
	MediaTimingEmitAt(ctx, event, thread, navtrace.NavigationBenchmarkMonotonicNs(), fields...)
}

func MediaTimingEmitAt(ctx context.Context, event, thread string, monotonicNs int64, fields ...any) {
	if !mediaTimingIsEnabled() || monotonicNs == 0 {
		return
	}
	if !strings.HasPrefix(event, "go.") {
		event = "go." + event
	}
	record := make(map[string]any, 7+len(fields)/2)
	record["schema"] = mediaTimingSchema
	record["event"] = event
	record["monotonicNs"] = monotonicNs
	record["pid"] = os.Getpid()
	record["thread"] = thread
	if ctx != nil {
		if correlation, ok := ctx.Value(mediaTimingContextKey{}).(mediaTimingCorrelation); ok {
			if correlation.requestID != "" {
				record["requestId"] = correlation.requestID
			}
			if correlation.traceID != "" {
				record["traceId"] = correlation.traceID
			}
		}
	}
	for index := 0; index+1 < len(fields); index += 2 {
		key, ok := fields[index].(string)
		if ok && key != "" {
			record[key] = fields[index+1]
		}
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return
	}
	line := make([]byte, 0, len(mediaTimingLogPrefix)+len(payload)+1)
	line = append(line, mediaTimingLogPrefix...)
	line = append(line, payload...)
	line = append(line, '\n')

	mediaTimingOutput.Lock()
	_, _ = mediaTimingOutput.writer.Write(line)
	mediaTimingOutput.Unlock()
}

func MediaTimingError(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
