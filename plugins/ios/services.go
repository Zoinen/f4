package iosfs

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	goios "github.com/danielpaulus/go-ios/ios"
)

const (
	afcServiceName              = "com.apple.afc"
	houseArrestServiceName      = "com.apple.mobile.house_arrest"
	crashReportMoverServiceName = "com.apple.crashreportmover"
	crashReportCopyServiceName  = "com.apple.crashreportcopymobile"

	vendContainer = "VendContainer"
	vendDocuments = "VendDocuments"
)

func connectService(ctx context.Context, device goios.DeviceEntry, service string) (goios.DeviceConnectionInterface, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	conn, err := goios.ConnectToService(device, service)
	if err != nil {
		return nil, fmt.Errorf("ios: connect %s: %w", service, err)
	}
	if err := ctx.Err(); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

func openHouseArrest(ctx context.Context, device goios.DeviceEntry, bundleID, command string) (io.ReadWriteCloser, error) {
	conn, err := connectService(ctx, device, houseArrestServiceName)
	if err != nil {
		return nil, err
	}
	keep := false
	defer func() {
		if !keep {
			_ = conn.Close()
		}
	}()

	codec := goios.NewPlistCodec()
	request, err := codec.Encode(map[string]interface{}{
		"Command":    command,
		"Identifier": bundleID,
	})
	if err != nil {
		return nil, fmt.Errorf("ios: encode House Arrest request: %w", err)
	}
	if err := conn.Send(request); err != nil {
		return nil, fmt.Errorf("ios: send House Arrest request: %w", err)
	}
	responseBytes, err := codec.Decode(conn.Reader())
	if err != nil {
		return nil, fmt.Errorf("ios: read House Arrest response: %w", err)
	}
	response, err := goios.ParsePlist(responseBytes)
	if err != nil {
		return nil, fmt.Errorf("ios: decode House Arrest response: %w", err)
	}
	if status, _ := response["Status"].(string); status != "Complete" {
		remoteError, _ := response["Error"].(string)
		if remoteError == "" {
			remoteError = fmt.Sprintf("unexpected status %q", status)
		}
		return nil, fmt.Errorf("ios: %s for %s failed: %s", command, bundleID, remoteError)
	}
	keep = true
	return conn, nil
}

// openCrashReportService asks iOS to move pending reports into the AFC export
// before connecting to that export. It is deliberately read-only at the VFS
// layer even though the protocol itself also exposes deletion.
func openCrashReportService(ctx context.Context, device goios.DeviceEntry) (io.ReadWriteCloser, error) {
	mover, err := connectService(ctx, device, crashReportMoverServiceName)
	if err != nil {
		return nil, err
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = mover.Conn().SetDeadline(deadline)
	} else {
		_ = mover.Conn().SetDeadline(time.Now().Add(10 * time.Second))
	}
	ping := make([]byte, 4)
	_, pingErr := io.ReadFull(mover, ping)
	_ = mover.Close()
	if pingErr != nil {
		return nil, fmt.Errorf("ios: wait for crash report mover: %w", pingErr)
	}
	if !strings.EqualFold(string(ping), "ping") {
		return nil, fmt.Errorf("ios: crash report mover returned %q instead of ping", ping)
	}
	return connectService(ctx, device, crashReportCopyServiceName)
}
