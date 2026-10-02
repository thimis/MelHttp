//go:build !windows

package main

import (
	"context"
	"errors"
)

func isWindowsService() bool { return false }

func runAsService(string, func(context.Context) int) error {
	return errors.New("not a Windows service")
}

func controlService(string, string, []string) error {
	return errors.New("-service is for Windows; on Linux use the systemd unit and on macOS the launchd plist in deploy/ (see docs/install.md)")
}
