// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !linux

package main

import (
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"runtime"
	"strings"

	"github.com/TechnikWeber/AXLink/internal/setup"
)

// startStation starts "axlinkd run" with administrator rights, which the
// network adapter needs: on Windows after the UAC prompt in a window of its
// own, on macOS after the password prompt in the background. Neither is a
// service yet; after a restart of the computer, open AXLink again.
// Not tested on a real Windows or Mac yet.
func startStation(configPath string, log *slog.Logger) (setup.Result, error) {
	exe, err := executable()
	if err != nil {
		return setup.Result{}, err
	}
	switch runtime.GOOS {
	case "windows":
		manual := fmt.Sprintf(`"%s" run -config "%s"`, exe, configPath)
		ps := fmt.Sprintf(`Start-Process -FilePath '%s' -ArgumentList 'run -config "%s"' -Verb RunAs`,
			psQuote(exe), psQuote(configPath))
		if out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", ps).CombinedOutput(); err != nil {
			return setup.Result{Manual: manual}, fmt.Errorf("%v %s", err, strings.TrimSpace(string(out)))
		}
		return setup.Result{Wait: true}, nil
	case "darwin":
		run := shellQuote(exe, "run", "-config", configPath) + " >>/tmp/axlinkd.log 2>&1 &"
		manual := "sudo " + shellQuote(exe, "run", "-config", configPath)
		script := `do shell script "` + asQuote(run) + `" with administrator privileges`
		if out, err := exec.Command("osascript", "-e", script).CombinedOutput(); err != nil {
			return setup.Result{Manual: manual}, fmt.Errorf("%v %s", err, strings.TrimSpace(string(out)))
		}
		log.Info("AXLink started in the background", "log", "/tmp/axlinkd.log")
		return setup.Result{Wait: true}, nil
	}
	return setup.Result{Manual: "sudo " + shellQuote(exe, "run", "-config", configPath), Wait: true}, nil
}

// psQuote escapes s for a single-quoted PowerShell string.
func psQuote(s string) string { return strings.ReplaceAll(s, "'", "''") }

// asQuote escapes s for a double-quoted AppleScript string.
func asQuote(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s)
}

func installService(_ []string, _, stderr io.Writer) int {
	fmt.Fprintln(stderr, "install-service: only on Linux so far")
	return 1
}

func uninstallService(_ []string, _, stderr io.Writer) int {
	fmt.Fprintln(stderr, "uninstall-service: only on Linux so far")
	return 1
}
