// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"strings"
	"testing"
)

func TestServiceUnit(t *testing.T) {
	unit := serviceUnit("op", "/home/op/.config/axlink/settings.json", []string{"dialout"})
	for _, want := range []string{
		"User=op\n",
		`ExecStart=/usr/local/bin/axlinkd run -config "/home/op/.config/axlink/settings.json"` + "\n",
		"AmbientCapabilities=CAP_NET_ADMIN CAP_NET_BIND_SERVICE\n",
		"CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_BIND_SERVICE\n",
		"SupplementaryGroups=dialout\n",
		"WantedBy=multi-user.target\n",
	} {
		if !strings.Contains(unit, want) {
			t.Errorf("unit lacks %q:\n%s", want, unit)
		}
	}
	if strings.Contains(serviceUnit("op", "/x", nil), "SupplementaryGroups") {
		t.Error("no serial group: no SupplementaryGroups line")
	}
}

func TestInstallServiceNeedsRoot(t *testing.T) {
	var out, errs strings.Builder
	if code := installService([]string{"-user", "op", "-config", "/x"}, &out, &errs); code == 0 {
		t.Fatal("must refuse without root")
	}
}
