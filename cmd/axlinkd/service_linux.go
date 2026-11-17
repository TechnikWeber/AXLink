// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/TechnikWeber/AXLink/internal/setup"
)

const (
	serviceName = "axlinkd.service"
	serviceFile = "/etc/systemd/system/" + serviceName
	installedAt = "/usr/local/bin/axlinkd"
)

// startStation installs and starts the service. The setup page runs as the
// user, so it asks for the administrator password once with pkexec; without
// pkexec it returns the command to run by hand.
func startStation(configPath string, log *slog.Logger) (setup.Result, error) {
	exe, err := executable()
	if err != nil {
		return setup.Result{}, err
	}
	u, err := user.Current()
	if err != nil {
		return setup.Result{}, err
	}
	args := []string{exe, "install-service", "-user", u.Username, "-config", configPath}
	manual := "sudo " + shellQuote(args...)
	pkexec, err := exec.LookPath("pkexec")
	if err != nil {
		return setup.Result{Manual: manual, Wait: true}, nil
	}
	out, err := exec.Command(pkexec, args...).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return setup.Result{Manual: manual, Wait: false}, errors.New(msg)
	}
	if err := writeDesktopEntry(); err != nil {
		log.Warn("start menu entry not written", "err", err)
	}
	return setup.Result{Wait: true}, nil
}

// installService copies the program to /usr/local/bin and installs a systemd
// service that runs it as the given user, with only the rights a network
// adapter needs (CAP_NET_ADMIN; CAP_NET_BIND_SERVICE for the welcome page on
// port 80), and access to serial ports. Run as root.
func installService(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("install-service", flag.ContinueOnError)
	fs.SetOutput(stderr)
	name := fs.String("user", "", "the user the service runs as (required)")
	configPath := fs.String("config", "", "the user's settings file with the station (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if os.Geteuid() != 0 {
		fmt.Fprintln(stderr, "install-service: run as root (sudo)")
		return 1
	}
	u, err := user.Lookup(*name)
	if *name == "" || err != nil || u.Uid == "0" {
		fmt.Fprintln(stderr, "install-service: -user: want an existing user other than root")
		return 2
	}
	if !filepath.IsAbs(*configPath) || strings.ContainsAny(*configPath, "\"\\\n%$") {
		fmt.Fprintln(stderr, "install-service: -config: want an absolute path without quotes, backslashes, % or $")
		return 2
	}
	if _, err := os.Stat(*configPath); err != nil {
		fmt.Fprintln(stderr, "install-service:", err)
		return 1
	}
	exe, err := executable()
	if err != nil {
		fmt.Fprintln(stderr, "install-service:", err)
		return 1
	}
	if exe != installedAt {
		if err := copyFile(exe, installedAt); err != nil {
			fmt.Fprintln(stderr, "install-service:", err)
			return 1
		}
	}
	unit := serviceUnit(u.Username, *configPath, serialGroups())
	if err := os.WriteFile(serviceFile, []byte(unit), 0o644); err != nil {
		fmt.Fprintln(stderr, "install-service:", err)
		return 1
	}
	for _, c := range [][]string{{"daemon-reload"}, {"enable", serviceName}, {"restart", serviceName}} {
		if out, err := exec.Command("systemctl", c...).CombinedOutput(); err != nil {
			fmt.Fprintf(stderr, "install-service: systemctl %s: %v %s\n", strings.Join(c, " "), err, out)
			return 1
		}
	}
	fmt.Fprintf(stdout, "AXLink runs as the service %s (user %s); it starts with the computer.\n", serviceName, u.Username)
	fmt.Fprintln(stdout, "Web interface: http://"+defaultUI+"/   Log: journalctl -u axlinkd")
	return 0
}

// uninstallService stops and removes the service and the installed program.
// The settings stay. Run as root.
func uninstallService(_ []string, stdout, stderr io.Writer) int {
	if os.Geteuid() != 0 {
		fmt.Fprintln(stderr, "uninstall-service: run as root (sudo)")
		return 1
	}
	_ = exec.Command("systemctl", "disable", "--now", serviceName).Run()
	for _, p := range []string{serviceFile, installedAt} {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintln(stderr, "uninstall-service:", err)
			return 1
		}
	}
	_ = exec.Command("systemctl", "daemon-reload").Run()
	fmt.Fprintln(stdout, "The service is removed; the settings are kept.")
	return 0
}

func serviceUnit(userName, configPath string, groups []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, `[Unit]
Description=AXLink: IPv4 over AX.25 radio
After=network.target

[Service]
User=%s
ExecStart=%s run -config "%s"
AmbientCapabilities=CAP_NET_ADMIN CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_BIND_SERVICE
`, userName, installedAt, configPath)
	if len(groups) > 0 {
		fmt.Fprintf(&b, "SupplementaryGroups=%s\n", strings.Join(groups, " "))
	}
	b.WriteString(`Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
`)
	return b.String()
}

// serialGroups are the groups that own serial ports on this system:
// dialout on Fedora, Debian, Ubuntu and Raspberry Pi OS, uucp on Arch.
func serialGroups() []string {
	var out []string
	for _, g := range []string{"dialout", "uucp"} {
		if _, err := user.LookupGroup(g); err == nil {
			out = append(out, g)
		}
	}
	return out
}

// copyFile writes a new file, so that it gets the labels of its folder
// (SELinux: a moved file would keep the label of the home folder).
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".new"
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

// writeDesktopEntry puts "AXLink" into the user's start menu: it opens the
// web interface, or the setup page when AXLink does not run.
func writeDesktopEntry() error {
	dir := os.Getenv("XDG_DATA_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		dir = filepath.Join(home, ".local", "share")
	}
	dir = filepath.Join(dir, "applications")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "axlink.desktop"), []byte(desktopEntry), 0o644)
}

const desktopEntry = `[Desktop Entry]
Type=Application
Name=AXLink
GenericName=IP over packet radio
Comment=Open AXLink: stations, chat, internet gateway and settings
Comment[de]=AXLink öffnen: Stationen, Chat, Internet-Gateway und Einstellungen
Exec=` + installedAt + `
Icon=network-wireless
Terminal=false
Categories=Network;HamRadio;
Keywords=AX.25;KISS;packet;radio;ham;
`
