// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"net/http"
	"strings"
)

// The page's texts. {n}, {t}, {call}, {addr}, {a}, {r} are filled in by the
// page's script. Both languages must have the same keys (a test checks).
var texts = map[string]map[string]string{
	"en": {
		"lang.label": "Language",
		"offline":    "No answer from axlinkd. Is it still running?",

		"station.title":      "This station",
		"station.call":       "Callsign",
		"station.ip":         "Address",
		"station.tags":       "Roles",
		"station.none":       "none",
		"station.beacon":     "Beacon",
		"station.every":      "every {n} min",
		"station.beacon.off": "off (listening only)",
		"station.adapter":    "Network adapter",
		"station.mtu":        "{name}, MTU {a} (radio {r})",
		"station.tnc":        "TNC",
		"station.running":    "Running for",
		"tnc.on":             "TNC connected for {t}",
		"tnc.off":            "TNC not connected",

		"stations.title": "Stations heard",
		"th.call":        "Callsign",
		"th.ip":          "Address",
		"th.roles":       "Roles",
		"th.gateway":     "Internet gateway",
		"th.heard":       "Last beacon",
		"stations.empty": "No station heard yet. Each station sends a beacon every few minutes; " +
			"a station that has just started asks the others for theirs.",
		"ago.s":   "{n} s ago",
		"ago.min": "{n} min ago",
		"ago.h":   "{n} h ago",
		"btn.use": "Use",
		"in.use":  "in use",

		"internet.title":   "Internet",
		"internet.off":     "Off",
		"internet.auto":    "Automatic",
		"internet.state":   "Gateway",
		"internet.is.off":  "No internet over the radio. Choose a station with the role INET below, or Automatic.",
		"internet.using":   "Through {call} ({addr})",
		"internet.none":    "No internet gateway heard yet.",
		"internet.gone":    "{call} is not heard at the moment.",
		"internet.how":     "Set {addr} as the HTTP and HTTPS proxy of your browser. Other programs, e.g.:",
		"internet.nolocal": "The local proxy is off (-local-proxy).",
		"internet.own": "This computer has its own internet connection and shares it with the radio network " +
			"(below). Its programs use the connection directly.",
		"internet.slow": "At 1200 bit/s a small page takes seconds, and setting up HTTPS alone " +
			"takes a minute or two.",

		"gateway.title":  "This station shares its internet connection",
		"gateway.listen": "Proxy for the radio network",
		"gateway.lan":    "Own network",
		"lan.closed":     "closed to radio users",
		"lan.open":       "open (-proxy-lan)",

		"traffic.title":  "Traffic",
		"group.radio":    "Radio",
		"group.ip":       "IP",
		"group.beacons":  "Beacons",
		"group.internet": "Internet through the gateway",
		"group.gateway":  "Gateway for others",

		"c.frames_out":      "Frames sent",
		"c.frames_in":       "Frames received",
		"c.queue_full":      "Queue full",
		"c.stale":           "Too old, dropped",
		"c.replaced":        "Merged in the queue",
		"c.repeats":         "Repeats dropped",
		"c.reconnects":      "TNC reconnects",
		"c.to_radio":        "Packets to the radio",
		"c.from_radio":      "Packets from the radio",
		"c.no_route":        "No route",
		"c.filtered":        "Broadcasts kept back",
		"c.not_for_us":      "For other stations",
		"c.bad_packet":      "Faulty",
		"c.clamped":         "TCP MSS lowered",
		"c.fragmented":      "Fragmented",
		"c.too_big":         "Too big (DF set)",
		"c.beacons_sent":    "Beacons sent",
		"c.beacons_heard":   "Beacons heard",
		"c.beacons_invalid": "Not usable",
		"c.requests":        "Requests",
		"c.tunnels":         "HTTPS tunnels",
		"c.no_gateway":      "Without a gateway",
		"c.failed":          "Failed",
		"c.refused":         "Refused (own network)",
		"c.bytes_in":        "Received",
		"c.bytes_out":       "Sent",
		"c.gw_bytes_in":     "From the internet",
		"c.gw_bytes_out":    "To the internet",

		"footer.operator": "Every operator is responsible for what is allowed on their frequency.",
	},
	"de": {
		"lang.label": "Sprache",
		"offline":    "Keine Antwort von axlinkd. Läuft es noch?",

		"station.title":      "Diese Station",
		"station.call":       "Rufzeichen",
		"station.ip":         "Adresse",
		"station.tags":       "Rollen",
		"station.none":       "keine",
		"station.beacon":     "Bake",
		"station.every":      "alle {n} min",
		"station.beacon.off": "aus (nur hören)",
		"station.adapter":    "Netzwerkadapter",
		"station.mtu":        "{name}, MTU {a} (Funk {r})",
		"station.tnc":        "TNC",
		"station.running":    "Läuft seit",
		"tnc.on":             "TNC seit {t} verbunden",
		"tnc.off":            "TNC nicht verbunden",

		"stations.title": "Gehörte Stationen",
		"th.call":        "Rufzeichen",
		"th.ip":          "Adresse",
		"th.roles":       "Rollen",
		"th.gateway":     "Internet-Gateway",
		"th.heard":       "Letzte Bake",
		"stations.empty": "Noch keine Station gehört. Jede Station sendet alle paar Minuten eine Bake; " +
			"eine gerade gestartete fragt die anderen nach ihren.",
		"ago.s":   "vor {n} s",
		"ago.min": "vor {n} min",
		"ago.h":   "vor {n} h",
		"btn.use": "Verwenden",
		"in.use":  "in Verwendung",

		"internet.title":   "Internet",
		"internet.off":     "Aus",
		"internet.auto":    "Automatisch",
		"internet.state":   "Gateway",
		"internet.is.off":  "Kein Internet über Funk. Unten eine Station mit der Rolle INET wählen oder Automatisch.",
		"internet.using":   "Über {call} ({addr})",
		"internet.none":    "Noch kein Internet-Gateway gehört.",
		"internet.gone":    "{call} ist gerade nicht zu hören.",
		"internet.how":     "Im Browser {addr} als HTTP- und HTTPS-Proxy eintragen. Andere Programme, z. B.:",
		"internet.nolocal": "Der lokale Proxy ist aus (-local-proxy).",
		"internet.own": "Dieser Rechner hat einen eigenen Internetzugang und teilt ihn mit dem Funknetz " +
			"(unten). Seine Programme nutzen den Zugang direkt.",
		"internet.slow": "Mit 1200 bit/s dauert eine kleine Seite Sekunden, allein der Aufbau von HTTPS " +
			"ein bis zwei Minuten.",

		"gateway.title":  "Diese Station teilt ihren Internetzugang",
		"gateway.listen": "Proxy für das Funknetz",
		"gateway.lan":    "Eigenes Netz",
		"lan.closed":     "für Funkteilnehmer gesperrt",
		"lan.open":       "offen (-proxy-lan)",

		"traffic.title":  "Verkehr",
		"group.radio":    "Funk",
		"group.ip":       "IP",
		"group.beacons":  "Baken",
		"group.internet": "Internet über das Gateway",
		"group.gateway":  "Gateway für andere",

		"c.frames_out":      "Frames gesendet",
		"c.frames_in":       "Frames empfangen",
		"c.queue_full":      "Warteschlange voll",
		"c.stale":           "Zu alt, verworfen",
		"c.replaced":        "In der Warteschlange zusammengefasst",
		"c.repeats":         "Wiederholungen verworfen",
		"c.reconnects":      "TNC neu verbunden",
		"c.to_radio":        "Pakete zum Funk",
		"c.from_radio":      "Pakete vom Funk",
		"c.no_route":        "Keine Route",
		"c.filtered":        "Broadcasts zurückgehalten",
		"c.not_for_us":      "Für andere Stationen",
		"c.bad_packet":      "Fehlerhaft",
		"c.clamped":         "TCP-MSS gesenkt",
		"c.fragmented":      "Fragmentiert",
		"c.too_big":         "Zu groß (DF gesetzt)",
		"c.beacons_sent":    "Baken gesendet",
		"c.beacons_heard":   "Baken gehört",
		"c.beacons_invalid": "Nicht verwendbar",
		"c.requests":        "Anfragen",
		"c.tunnels":         "HTTPS-Tunnel",
		"c.no_gateway":      "Ohne Gateway",
		"c.failed":          "Fehlgeschlagen",
		"c.refused":         "Abgelehnt (eigenes Netz)",
		"c.bytes_in":        "Empfangen",
		"c.bytes_out":       "Gesendet",
		"c.gw_bytes_in":     "Aus dem Internet",
		"c.gw_bytes_out":    "Ins Internet",

		"footer.operator": "Jeder Betreiber ist selbst dafür verantwortlich, was auf seiner Frequenz erlaubt ist.",
	},
}

const langCookie = "axlink_lang"

// HasText reports whether the page has a text for key in every language.
func HasText(key string) bool {
	for _, d := range texts {
		if _, ok := d[key]; !ok {
			return false
		}
	}
	return true
}

// language picks the page's language: an explicit choice (?lang= or the
// cookie it sets), else the browser's preference, else English (D7).
func language(r *http.Request) (lang string, chosen bool) {
	if l := r.URL.Query().Get("lang"); texts[l] != nil {
		return l, true
	}
	if c, err := r.Cookie(langCookie); err == nil && texts[c.Value] != nil {
		return c.Value, false
	}
	for _, part := range strings.Split(r.Header.Get("Accept-Language"), ",") {
		tag, _, _ := strings.Cut(strings.TrimSpace(part), ";")
		base, _, _ := strings.Cut(strings.ToLower(tag), "-")
		if texts[base] != nil {
			return base, false
		}
	}
	return "en", false
}
