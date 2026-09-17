package plughost

import "context"

func (c *extUiMediaConn) handleDirectoryPreview(ctx context.Context, requestID string, message map[string]any) {
	resourceID := extUiString(message, "resourceId")
	response := map[string]any{"type": "response", "requestId": requestID, "ok": true}
	var leaseID string
	var err error
	switch extUiString(message, "op") {
	case "enumerateDirectoryPreview":
		items, lease, readErr := c.broker.EnumerateDirectoryPreview(ctx, resourceID)
		leaseID, err = lease, readErr
		entries := make([]map[string]any, 0, len(items))
		for _, item := range items {
			entries = append(entries, map[string]any{"name": item.Name, "size": item.Size, "sizeKnown": item.SizeKnown, "mtime": item.MTime.UnixMilli(), "revision": item.Revision})
		}
		response["entries"] = entries
	case "resolveDirectoryPreview":
		var names []string
		switch values := message["names"].(type) {
		case []any:
			for _, value := range values {
				name, ok := value.(string)
				if !ok {
					c.respondError(requestID, errMediaUnknownResource)
					return
				}
				names = append(names, name)
			}
		case []string:
			names = values
		default:
			c.respondError(requestID, errMediaUnknownResource)
			return
		}
		images, lease, resolveErr := c.broker.ResolveDirectoryPreview(ctx, resourceID, extUiString(message, "listingLeaseId"), names)
		leaseID, err = lease, resolveErr
		entries := make([]map[string]any, 0, len(images))
		for _, image := range images {
			source := image.Source
			entries = append(entries, map[string]any{"name": image.Name, "source": map[string]any{
				"resourceId": source.ResourceID, "sourceKey": source.SourceKey, "version": source.Version, "versionStrength": source.VersionStrength,
				"size": source.Size, "sizeKnown": source.SizeKnown, "accessProfile": source.AccessProfile, "storageClass": source.StorageClass,
			}})
		}
		response["entries"] = entries
	}
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		c.broker.Release(resourceID, leaseID)
		c.respondError(requestID, err)
		return
	}
	response["leaseId"] = leaseID
	provisional := c.provision(requestID, resourceID, leaseID)
	if provisional == nil {
		return
	}
	if err := c.send(response); err != nil {
		c.dropProvisional(provisional)
	}
}
