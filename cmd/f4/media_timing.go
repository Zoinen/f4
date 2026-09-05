package main

import (
	"context"
	"encoding/json"
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

func mediaTimingWithCorrelation(ctx context.Context, requestID, traceID string) context.Context {
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
func mediaTimingDetachedContext(ctx context.Context) context.Context {
	correlation, _ := ctx.Value(mediaTimingContextKey{}).(mediaTimingCorrelation)
	return mediaTimingWithCorrelation(
		context.Background(), correlation.requestID, correlation.traceID)
}

func mediaTimingEmit(ctx context.Context, event, thread string, fields ...any) {
	mediaTimingEmitAt(ctx, event, thread, navigationBenchmarkMonotonicNs(), fields...)
}

func mediaTimingEmitAt(ctx context.Context, event, thread string, monotonicNs int64, fields ...any) {
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

func mediaTimingError(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func mediaTimingResourceFields(resource *extUiMediaResource, fields ...any) []any {
	if resource == nil {
		return fields
	}
	base := []any{
		"resourceId", resource.id,
		"sourceKey", resource.sourceKey,
		"path", resource.path,
		"sourceBytes", resource.size,
		"sizeKnown", resource.sizeKnown,
		"accessProfile", resource.accessProfile.String(),
		"storageClass", resource.storageClass.String(),
	}
	return append(base, fields...)
}

func mediaTimingResourceEmit(ctx context.Context, event, thread string,
	resource *extUiMediaResource, fields ...any) {
	mediaTimingEmit(ctx, event, thread,
		mediaTimingResourceFields(resource, fields...)...)
}

func mediaTimingResourceEmitAt(ctx context.Context, event, thread string,
	monotonicNs int64, resource *extUiMediaResource, fields ...any) {
	mediaTimingEmitAt(ctx, event, thread, monotonicNs,
		mediaTimingResourceFields(resource, fields...)...)
}
