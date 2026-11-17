// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/TechnikWeber/AXLink/internal/settings"
	"github.com/TechnikWeber/AXLink/internal/setup"
	"github.com/TechnikWeber/AXLink/internal/tnc"
)

// defaultUI is the address of the web interface "axlinkd run" opens.
const defaultUI = "127.0.0.1:7373"

// openOrSetup is what a double click (no arguments) does: open the web
// interface if AXLink runs, the setup page otherwise.
func openOrSetup(stdout, stderr io.Writer) int {
	if uiRunning(defaultUI) {
		url := "http://" + defaultUI + "/"
		fmt.Fprintln(stdout, "AXLink is running:", url)
		if err := openBrowser(url); err != nil {
			fmt.Fprintln(stderr, "could not open a browser:", err)
		}
		return 0
	}
	return runSetup(nil, stdout, stderr)
}

// runSetup serves the setup page and opens it in the browser. It ends when
// the page has seen AXLink start, after 30 idle minutes, or on Ctrl+C.
func runSetup(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	fs.SetOutput(stderr)
	addr := fs.String("listen", setup.DefaultAddr, "address of the setup page")
	configPath := fs.String("config", settings.DefaultPath(), "file for the settings")
	noBrowser := fs.Bool("no-browser", false, "do not open a browser, only print the address")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *configPath == "" {
		fmt.Fprintln(stderr, "setup: -config: the setup page needs a settings file")
		return 2
	}
	abs, err := filepath.Abs(*configPath)
	if err != nil {
		fmt.Fprintln(stderr, "setup:", err)
		return 1
	}
	store, err := settings.Open(abs)
	if err != nil {
		fmt.Fprintln(stderr, "setup:", err)
		return 1
	}
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		url := "http://" + *addr + "/"
		if setupOpen(*addr) { // the setup page is open already
			fmt.Fprintln(stdout, "The setup page is open already:", url)
			if !*noBrowser {
				_ = openBrowser(url)
			}
			return 0
		}
		fmt.Fprintln(stderr, "setup:", err)
		return 1
	}
	log := slog.New(slog.NewTextHandler(stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	idle := time.NewTimer(30 * time.Minute)
	page := setup.New(setup.Config{
		Store: store, Ports: tnc.ListPorts, Version: version, Logger: log,
		UIURL:   "http://" + defaultUI + "/",
		Running: func() bool { return uiRunning(defaultUI) },
		Start:   func(st settings.Station) (setup.Result, error) { return startStation(abs, log) },
		Done:    stop,
	})
	srv := &http.Server{ReadHeaderTimeout: time.Minute, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		idle.Reset(30 * time.Minute)
		page.ServeHTTP(w, r)
	})}
	go func() {
		select {
		case <-ctx.Done():
		case <-idle.C:
		}
		srv.Close()
	}()
	url := "http://" + ln.Addr().String() + "/"
	fmt.Fprintln(stdout, "AXLink setup:", url)
	if !*noBrowser {
		if err := openBrowser(url); err != nil {
			fmt.Fprintln(stderr, "could not open a browser, open the address yourself:", err)
		}
	}
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintln(stderr, "setup:", err)
		return 1
	}
	return 0
}

// uiRunning reports whether AXLink's web interface answers at addr.
func uiRunning(addr string) bool {
	c := http.Client{Timeout: 800 * time.Millisecond}
	resp, err := c.Get("http://" + addr + "/api/status")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// setupOpen reports whether a setup page answers at addr.
func setupOpen(addr string) bool {
	c := http.Client{Timeout: 800 * time.Millisecond}
	resp, err := c.Get("http://" + addr + "/setup.js")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// openBrowser opens url in the desktop's browser.
func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	cmd.Stdout, cmd.Stderr = nil, nil
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait() //nolint:errcheck // the browser's exit code says nothing
	return nil
}

// executable is the path of the running program with symlinks resolved.
func executable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

// shellQuote joins args for a POSIX shell.
func shellQuote(args ...string) string {
	q := make([]string, len(args))
	for i, a := range args {
		if a != "" && !strings.ContainsAny(a, " \t\n'\"\\$`!*?[]{}()<>|&;#~") {
			q[i] = a
			continue
		}
		q[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
	}
	return strings.Join(q, " ")
}
