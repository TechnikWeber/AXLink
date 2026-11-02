// SPDX-License-Identifier: GPL-3.0-or-later
// AXLink web interface. The overview reads /api/status every 2 s and draws
// the page; the settings view reads /api/settings once and saves it back.
// Whatever comes from the radio is set as text, never as HTML.
"use strict";

const T = JSON.parse(document.getElementById("texts").textContent);
const GROUPS = ["radio", "ip", "beacons", "internet", "gateway"];
const SVG = "http://www.w3.org/2000/svg";
const LANG = document.documentElement.lang;
let settings = null; // the settings as last read or saved

function t(key, vars) {
  let s = T[key] || key;
  for (const [k, v] of Object.entries(vars || {})) s = s.split("{" + k + "}").join(String(v));
  return s;
}

function el(tag, props, ...children) {
  const e = document.createElement(tag);
  for (const [k, v] of Object.entries(props || {})) {
    if (k === "class") e.className = v;
    else if (k === "text") e.textContent = v;
    else if (k.startsWith("on")) e.addEventListener(k.slice(2), v);
    else e.setAttribute(k, v);
  }
  for (const c of children) if (c != null) e.append(c);
  return e;
}

function svg(tag, attrs, text) {
  const e = document.createElementNS(SVG, tag);
  for (const [k, v] of Object.entries(attrs || {})) e.setAttribute(k, v);
  if (text != null) e.textContent = text;
  return e;
}

const $ = (id) => document.getElementById(id);

function ago(s) {
  if (s < 90) return t("ago.s", { n: s });
  if (s < 90 * 60) return t("ago.min", { n: Math.round(s / 60) });
  return t("ago.h", { n: Math.round(s / 3600) });
}

function duration(s) {
  if (s < 90) return s + " s";
  if (s < 90 * 60) return Math.round(s / 60) + " min";
  const h = Math.floor(s / 3600), m = Math.round((s % 3600) / 60);
  return m ? h + " h " + m + " min" : h + " h";
}

function size(n) {
  if (n < 1024) return Math.round(n) + " B";
  if (n < 1024 * 1024) return (n / 1024).toFixed(1) + " KB";
  return (n / 1024 / 1024).toFixed(1) + " MB";
}

function rate(n) {
  return (n < 10 ? n.toFixed(1) : Math.round(n)) + " B/s";
}

function pct(x) {
  return Math.round(x * 100) + "\u00a0%"; // a number and its unit stay on one line
}

function badges(tags) {
  const f = document.createDocumentFragment();
  for (const tag of tags || []) f.append(el("span", { class: "badge " + tag, text: tag }));
  return f;
}

function row(dl, key, value) {
  dl.append(el("dt", { text: t(key) }), el("dd", {}, value));
}

// ---- live monitor ----

function gauge(box, value, ticks) {
  const r = 70, cx = 90, cy = 88;
  const point = (f) => [cx - r * Math.cos(Math.PI * f), cy - r * Math.sin(Math.PI * f)];
  const arc = (to) => {
    const [x0, y0] = point(0), [x1, y1] = point(to);
    return `M ${x0} ${y0} A ${r} ${r} 0 0 1 ${x1} ${y1}`;
  };
  const v = Math.max(0, Math.min(value, 1));
  const level = v < 0.5 ? "ok" : v < 0.8 ? "warn" : "high";
  const g = svg("svg", { viewBox: "0 0 180 108", width: "180", height: "108", class: "gauge", role: "img" });
  g.append(svg("path", { d: arc(1), class: "track" }));
  if (v > 0.002) g.append(svg("path", { d: arc(Math.min(v, 0.999)), class: "fill " + level }));
  g.append(svg("text", { x: cx, y: cy - 8, "text-anchor": "middle", class: "big" }, ticks.center));
  g.append(svg("text", { x: cx - r, y: cy + 16, "text-anchor": "middle", class: "tick" }, ticks.low));
  g.append(svg("text", { x: cx + r, y: cy + 16, "text-anchor": "middle", class: "tick" }, ticks.high));
  box.replaceChildren(g);
}

function niceMax(v) {
  for (const m of [20, 50, 100, 150, 200, 300, 500, 1000, 2000, 5000, 10000]) if (v <= m) return m;
  return Math.ceil(v / 10000) * 10000;
}

function chart(box, samples, interval) {
  const W = 640, H = 180, L = 56, R = 10, TOP = 10, B = 24;
  const n = Math.round(300 / interval); // five minutes
  const pw = W - L - R, ph = H - TOP - B;
  // Each frame is about 2 s on the air: a mean over three samples (6 s)
  // reads better than the raw steps.
  const smooth = (key) => samples.map((_, i) => {
    const w = samples.slice(Math.max(0, i - 2), i + 1);
    return w.reduce((a, s) => a + s[key], 0) / w.length;
  });
  const sin = smooth("in"), sout = smooth("out");
  const max = niceMax(Math.max(...sin, ...sout, 1));
  const y = (v) => TOP + ph - (v / max) * ph;
  const x = (i) => L + pw - ((samples.length - 1 - i) / (n - 1)) * pw;
  const g = svg("svg", { viewBox: `0 0 ${W} ${H}`, class: "plot", role: "img" });
  for (const f of [0, 0.5, 1]) {
    g.append(svg("line", { x1: L, x2: W - R, y1: y(max * f), y2: y(max * f), class: "grid" }));
    g.append(svg("text", { x: L - 6, y: y(max * f) + 4, "text-anchor": "end", class: "axis" }, rate(max * f)));
  }
  for (let m = 0; m <= 5; m++) {
    const xm = L + pw - (m * 60 / interval / (n - 1)) * pw;
    g.append(svg("line", { x1: xm, x2: xm, y1: TOP, y2: TOP + ph, class: "grid" }));
    g.append(svg("text", { x: xm, y: H - 6, "text-anchor": m === 0 ? "end" : "middle", class: "axis" },
      m === 0 ? t("monitor.now") : "−" + m + " min"));
  }
  if (samples.length > 1) {
    const pts = (series) => series.map((v, i) => `${x(i).toFixed(1)},${y(v).toFixed(1)}`).join(" ");
    const first = x(0).toFixed(1), last = x(samples.length - 1).toFixed(1), base = y(0).toFixed(1);
    g.append(svg("polygon", { points: `${first},${base} ${pts(sin)} ${last},${base}`, class: "in-area" }));
    g.append(svg("polyline", { points: pts(sin), class: "in" }));
    g.append(svg("polyline", { points: pts(sout), class: "out" }));
  } else {
    g.append(svg("text", { x: L + pw / 2, y: TOP + ph / 2, "text-anchor": "middle", class: "axis" }, t("monitor.wait")));
  }
  box.replaceChildren(g);
}

function drawMonitor(s) {
  const m = s.monitor, samples = m.samples || [];
  const minute = samples.slice(-Math.round(60 / m.interval_s));
  const avg = (list, f) => (list.length ? list.reduce((a, x) => a + f(x), 0) / list.length : 0);
  const load = avg(minute, (x) => x.own + x.heard), own = avg(minute, (x) => x.own);
  gauge($("gauge-load"), load, { center: pct(load), low: "0", high: "100 %" });
  $("load-sub").textContent = t("monitor.load.sub", { p: pct(own) });
  const queue = samples.length ? samples[samples.length - 1].queue : 0;
  gauge($("gauge-queue"), queue / m.queue_cap, { center: String(queue), low: "0", high: String(m.queue_cap) });
  $("queue-sub").textContent = t("monitor.queue.sub", { n: queue, m: m.queue_cap });
  chart($("chart"), samples, m.interval_s);
  const recent = samples.slice(-Math.round(10 / m.interval_s));
  $("rate-now").textContent = t("monitor.avg", { in: rate(avg(recent, (x) => x.in)), out: rate(avg(recent, (x) => x.out)) });
}

// ---- overview ----

function drawStation(s) {
  const dl = $("station");
  dl.replaceChildren();
  row(dl, "station.call", el("span", { class: "mono", text: s.call }));
  row(dl, "station.ip", el("span", { class: "mono", text: s.ip }));
  row(dl, "station.tags", s.tags && s.tags.length ? badges(s.tags) : el("span", { class: "muted", text: t("station.none") }));
  row(dl, "station.beacon", s.beacon_s ? t("station.every", { n: Math.round(s.beacon_s / 60) }) : t("station.beacon.off"));
  row(dl, "station.welcome", s.welcome
    ? el("a", { href: "/welcome/", target: "_blank", rel: "noopener", class: "mono", text: s.welcome })
    : el("span", { class: "muted", text: t("station.welcome.off") }));
  row(dl, "station.adapter", t("station.mtu", { name: s.adapter.name, a: s.adapter.mtu, r: s.adapter.radio_mtu }));
  row(dl, "station.tnc", s.tnc.name + " · " + s.tnc.profile);
  row(dl, "station.running", duration(s.uptime_s));
  $("tnc-dot").className = "dot" + (s.tnc.connected ? " on" : "");
  $("tnc-text").textContent = s.tnc.connected ? t("tnc.on", { t: duration(s.tnc.for_s) }) : t("tnc.off");
}

function drawInternet(s) {
  const box = $("internet"), net = s.internet;
  box.replaceChildren();
  if (s.gateway && net.mode === "off") { // a gateway uses its own connection
    box.append(el("p", { class: "state", text: t("internet.own") }));
    return;
  }
  const choices = el("div", { class: "choices" },
    el("button", { class: net.mode === "off" ? "on" : "", text: t("internet.off"), onclick: () => choose("off", "") }),
    el("button", { class: net.mode === "auto" ? "on" : "", text: t("internet.auto"), onclick: () => choose("auto", "") }));
  if (net.mode === "station") choices.append(el("button", { class: "on mono", text: net.call }));
  box.append(choices);
  let state, ok = false;
  if (net.mode === "off") state = t("internet.is.off");
  else if (net.using) { state = t("internet.using", { call: net.using, addr: net.upstream }); ok = true; }
  else if (net.mode === "station") state = t("internet.gone", { call: net.call });
  else state = t("internet.none");
  box.append(el("p", { class: "state" + (ok ? " ok" : ""), text: state }));
  if (net.local_proxy) {
    box.append(
      el("p", { class: "small", text: t("internet.how", { addr: net.local_proxy }) }),
      el("pre", { text: "curl -x http://" + net.local_proxy + " https://example.org/" }),
      el("p", { class: "small muted", text: t("internet.slow") }));
  } else {
    box.append(el("p", { class: "small muted", text: t("internet.nolocal") }));
  }
}

function drawStations(s) {
  const list = s.stations || [];
  const body = document.querySelector("#stations tbody");
  body.replaceChildren();
  $("stations").hidden = list.length === 0;
  $("stations-empty").hidden = list.length !== 0;
  for (const st of list) {
    const chosen = s.internet.mode === "station" && s.internet.call === st.call;
    let action = null;
    if (st.proxy && !s.gateway) {
      action = chosen ? el("span", { class: "muted small", text: t("in.use") })
        : el("button", { text: t("btn.use"), onclick: () => choose("station", st.call) });
    }
    body.append(el("tr", { class: s.internet.using === st.call ? "chosen" : "" },
      el("td", { class: "mono", text: st.call }),
      el("td", { class: "mono", text: st.ip }),
      el("td", {}, badges(st.tags)),
      el("td", { class: "mono", text: st.proxy || "–" }),
      el("td", { text: ago(st.heard_s) }),
      el("td", {}, el("a", { href: st.page, target: "_blank", rel: "noopener", text: t("page.open") })),
      el("td", {}, action)));
  }
}

function drawGateway(s) {
  const card = $("gateway-card");
  card.hidden = !s.gateway;
  if (!s.gateway) return;
  const dl = $("gateway");
  dl.replaceChildren();
  row(dl, "gateway.listen", el("span", { class: "mono", text: s.gateway.listen }));
  row(dl, "gateway.lan", t(s.gateway.lan ? "lan.open" : "lan.closed"));
  row(dl, "gateway.only", s.gateway.only && s.gateway.only.length
    ? el("span", { class: "mono", text: s.gateway.only.join(", ") }) : t("gateway.all"));
}

function drawFirewall(s) {
  const box = $("firewall"), fw = s.firewall;
  box.replaceChildren(
    el("p", { class: "state", text: t(fw.mode === "open" ? "fw.open" : "fw.protect") }),
    el("p", { class: "small muted", text: t(fw.silent ? "fw.counts" : "fw.counts.answered",
      { p: fw.passed, b: fw.blocked, a: fw.answered }) }));
  const recent = fw.recent || [];
  if (!recent.length) {
    box.append(el("p", { class: "small muted", text: t("fw.none") }));
    return;
  }
  const list = el("ul", { class: "recent" });
  for (const b of recent) {
    const when = new Date(b.at).toLocaleTimeString(LANG);
    list.append(el("li", {}, el("span", { class: "mono", text: b.call }), " → ",
      el("span", { class: "mono", text: b.proto + (b.port ? " " + b.port : "") }), el("span", { class: "muted", text: " · " + when })));
  }
  box.append(el("h3", { text: t("fw.recent") }), list);
}

function drawCounters(s) {
  const box = $("counters");
  box.replaceChildren();
  for (const g of GROUPS) {
    if (g === "internet" && s.gateway && s.internet.mode === "off") continue;
    const items = (s.counters || []).filter((c) => c.group === g);
    if (!items.length) continue;
    const tiles = el("div", { class: "tiles" });
    for (const c of items) {
      tiles.append(el("div", { class: "tile" },
        el("div", { class: "v", text: c.bytes ? size(c.value) : c.value.toLocaleString(LANG) }),
        el("div", { class: "k", text: t("c." + c.key) })));
    }
    box.append(el("div", { class: "group" }, el("h3", { text: t("group." + g) }), tiles));
  }
}

async function choose(mode, call) {
  try {
    await fetch("/api/gateway", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ mode, call }),
    });
  } finally {
    refresh();
  }
}

async function refresh() {
  try {
    const r = await fetch("/api/status", { cache: "no-store" });
    if (!r.ok) throw new Error(String(r.status));
    const s = await r.json();
    $("offline").hidden = true;
    drawStation(s); // also the header
    $("settings-file").textContent = s.settings_file ? t("set.file", { path: s.settings_file }) : t("set.memory");
    if ($("view-overview").hidden) return;
    drawMonitor(s);
    drawInternet(s);
    drawStations(s);
    drawGateway(s);
    drawFirewall(s);
    drawCounters(s);
  } catch (e) {
    $("offline").hidden = false;
  }
}

// ---- settings ----

function calls(text) {
  return text.split(",").map((c) => c.trim()).filter((c) => c);
}

function fillSettings(s) {
  settings = s;
  $("share-enabled").checked = s.share.enabled;
  $("share-port").value = s.share.port;
  $("share-lan").checked = s.share.lan;
  $("share-only").value = (s.share.only || []).join(", ");
  const w = s.welcome;
  $("wel-enabled").checked = w.enabled;
  $("wel-port").value = w.port;
  for (const k of ["name", "locator", "qth", "equipment", "text"]) $("wel-" + k).value = w[k] || "";
  $("fw-mode").value = s.firewall.mode;
  $("fw-ping").checked = s.firewall.ping;
  $("fw-answer").checked = !s.firewall.silent;
  const body = document.querySelector("#fw-rules tbody");
  body.replaceChildren();
  for (const r of s.firewall.rules || []) addRule(r);
  showNoRules();
}

function addRule(r) {
  const proto = el("select", {}, el("option", { value: "tcp", text: "TCP" }), el("option", { value: "udp", text: "UDP" }));
  proto.value = r.proto || "tcp";
  const tr = el("tr", {},
    el("td", {}, proto),
    el("td", {}, el("input", { type: "number", min: "1", max: "65535", value: r.port || "", class: "port" })),
    el("td", {}, el("input", { type: "text", value: (r.calls || []).join(", "), class: "calls mono", spellcheck: "false" })),
    el("td", {}, el("input", { type: "text", value: r.note || "", class: "rule-note", maxlength: "80" })),
    el("td", {}, el("button", { type: "button", text: t("fws.remove"), onclick: () => { tr.remove(); showNoRules(); } })));
  document.querySelector("#fw-rules tbody").append(tr);
  showNoRules();
}

function showNoRules() {
  const n = document.querySelectorAll("#fw-rules tbody tr").length;
  $("fw-rules").hidden = n === 0;
  $("fw-none").hidden = n !== 0;
}

function collect() {
  const s = structuredClone(settings);
  s.share = { enabled: $("share-enabled").checked, port: Number($("share-port").value), lan: $("share-lan").checked,
    only: calls($("share-only").value) };
  s.welcome = { enabled: $("wel-enabled").checked, port: Number($("wel-port").value) };
  for (const k of ["name", "locator", "qth", "equipment", "text"]) s.welcome[k] = $("wel-" + k).value;
  s.firewall = { mode: $("fw-mode").value, ping: $("fw-ping").checked, silent: !$("fw-answer").checked, rules: [] };
  for (const tr of document.querySelectorAll("#fw-rules tbody tr")) {
    s.firewall.rules.push({ proto: tr.querySelector("select").value, port: Number(tr.querySelector(".port").value),
      calls: calls(tr.querySelector(".calls").value), note: tr.querySelector(".rule-note").value });
  }
  return s;
}

async function loadSettings() {
  const r = await fetch("/api/settings", { cache: "no-store" });
  if (r.ok) fillSettings(await r.json());
}

async function save(form) {
  if (!settings) return; // not loaded yet
  const msg = form.querySelector(".msg");
  msg.className = "msg";
  msg.textContent = "…";
  try {
    const r = await fetch("/api/settings", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(collect()),
    });
    const body = await r.json();
    if (!r.ok) throw new Error(body.error || String(r.status));
    fillSettings(body);
    msg.className = "msg ok";
    msg.textContent = t("set.saved");
  } catch (e) {
    msg.className = "msg err";
    msg.textContent = t("set.failed", { e: e.message });
  }
}

for (const id of ["form-share", "form-welcome", "form-firewall"]) {
  $(id).addEventListener("submit", (ev) => { ev.preventDefault(); save(ev.target); });
}
$("fw-add").addEventListener("click", () => addRule({ proto: "tcp" }));

// ---- views ----

function show() {
  const view = location.hash === "#settings" ? "settings" : "overview";
  $("view-overview").hidden = view !== "overview";
  $("view-settings").hidden = view !== "settings";
  $("tab-overview").className = view === "overview" ? "on" : "";
  $("tab-settings").className = view === "settings" ? "on" : "";
  if (view === "settings") loadSettings();
  refresh();
}

window.addEventListener("hashchange", show);
show();
setInterval(refresh, 2000);
