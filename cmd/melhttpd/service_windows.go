//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

// isWindowsService reports whether the Service Control Manager started us.
func isWindowsService() bool {
	ok, err := svc.IsWindowsService()
	return err == nil && ok
}

// runAsService runs melhttpd under the Service Control Manager: Stop and
// Shutdown requests cancel the server's context for a graceful shutdown.
func runAsService(name string, serve func(ctx context.Context) int) error {
	return svc.Run(name, &serviceHandler{serve: serve})
}

type serviceHandler struct {
	serve func(ctx context.Context) int
}

func (h *serviceHandler) Execute(_ []string, req <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.StartPending}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan int, 1)
	go func() { done <- h.serve(ctx) }()
	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for {
		select {
		case code := <-done:
			status <- svc.Status{State: svc.Stopped}
			return false, uint32(code)
		case c := <-req:
			switch c.Cmd {
			case svc.Interrogate:
				status <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending, WaitHint: 20000}
				cancel()
				code := <-done
				status <- svc.Status{State: svc.Stopped}
				return false, uint32(code)
			}
		}
	}
}

// controlService installs, removes, starts or stops the Windows service.
// args are the melhttpd flags to store in the service definition.
func controlService(action, name string, args []string) error {
	m, err := mgr.Connect()
	if err != nil {
		if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			return fmt.Errorf("access denied: run this from an elevated (Administrator) prompt")
		}
		return err
	}
	defer m.Disconnect()
	switch action {
	case "install":
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		s, err := m.CreateService(name, exe, mgr.Config{
			DisplayName: "MelHttp (" + name + ")",
			Description: "MelHttp web server: pages printed by Malbolge programs.",
			StartType:   mgr.StartAutomatic,
		}, serviceArgs(args)...)
		if err != nil {
			return err
		}
		defer s.Close()
		// Restart after crashes: 5s, 10s, then every 30s.
		return s.SetRecoveryActions([]mgr.RecoveryAction{
			{Type: mgr.ServiceRestart, Delay: 5 * time.Second},
			{Type: mgr.ServiceRestart, Delay: 10 * time.Second},
			{Type: mgr.ServiceRestart, Delay: 30 * time.Second},
		}, 86400)
	case "uninstall", "start", "stop":
		s, err := m.OpenService(name)
		if err != nil {
			return fmt.Errorf("service %q: %w", name, err)
		}
		defer s.Close()
		switch action {
		case "uninstall":
			s.Control(svc.Stop) // ignore "not running"
			return s.Delete()
		case "start":
			return s.Start()
		case "stop":
			_, err := s.Control(svc.Stop)
			return err
		}
	}
	return fmt.Errorf("unknown -service action %q (use install, uninstall, start or stop)", action)
}
