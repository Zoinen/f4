package iosfs

import (
	"context"
	"errors"
	"testing"

	goios "github.com/danielpaulus/go-ios/ios"
)

func canceledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func TestConnectServiceCanceled(t *testing.T) {
	_, err := connectService(canceledContext(), goios.DeviceEntry{}, "service")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("connectService() error = %v, want context.Canceled", err)
	}
}

func TestOpenHouseArrestCanceled(t *testing.T) {
	_, err := openHouseArrest(canceledContext(), goios.DeviceEntry{}, "bundle", vendContainer)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("openHouseArrest() error = %v, want context.Canceled", err)
	}
}

func TestOpenCrashReportServiceCanceled(t *testing.T) {
	_, err := openCrashReportService(canceledContext(), goios.DeviceEntry{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("openCrashReportService() error = %v, want context.Canceled", err)
	}
}
