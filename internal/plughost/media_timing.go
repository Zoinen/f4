package plughost

import (
	"context"
	"github.com/unxed/f4/internal/mediatiming"
)

func mediaTimingResourceFields(resource *ExtUiMediaResource, fields ...any) []any {
	if resource == nil {
		return fields
	}
	base := []any{
		"resourceId", resource.Id,
		"sourceKey", resource.SourceKey,
		"path", resource.Path,
		"sourceBytes", resource.Size,
		"sizeKnown", resource.SizeKnown,
		"accessProfile", resource.AccessProfile.String(),
		"storageClass", resource.StorageClass.String(),
	}
	return append(base, fields...)
}

func MediaTimingResourceEmit(ctx context.Context, event, thread string,
	resource *ExtUiMediaResource, fields ...any) {
	mediatiming.MediaTimingEmit(ctx, event, thread,
		mediaTimingResourceFields(resource, fields...)...)
}

func MediaTimingResourceEmitAt(ctx context.Context, event, thread string,
	monotonicNs int64, resource *ExtUiMediaResource, fields ...any) {
	mediatiming.MediaTimingEmitAt(ctx, event, thread, monotonicNs,
		mediaTimingResourceFields(resource, fields...)...)
}
