// SPDX-License-Identifier: GPL-3.0-or-later
// AXLink web interface: reads /api/status every 3 s and draws the page.
// Whatever comes from the radio is set as text, never as HTML.
"use strict";

const T = JSON.parse(document.getElementById("texts").textContent);
const GROUPS = ["radio", "ip", "beacons", "internet", "gateway"];

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
  if (n < 1024) return n + " B";
  if (n < 1024 * 1024) return (n / 1024).toFixed(1) + " KB";
  return (n / 1024 / 1024).toFixed(1) + " MB";
}

function badges(tags) {
  const f = document.createDocumentFragment();
  for (const tag of tags || []) f.append(el("span", { class: "badge " + tag, text: tag }));
  return f;
}

function row(dl, key, value) {
  dl.append(el("dt", { text: t(key) }), el("dd", {}, value));
}

function drawStation(s) {
  const dl = document.getElementById("station");
  dl.replaceChildren();
  row(dl, "station.call", el("span", { class: "mono", text: s.call }));
  row(dl, "station.ip", el("span", { class: "mono", text: s.ip }));
  row(dl, "station.tags", s.tags && s.tags.length ? badges(s.tags) : el("span", { class: "muted", text: t("station.none") }));
  row(dl, "station.beacon", s.beacon_s ? t("station.every", { n: Math.round(s.beacon_s / 60) }) : t("station.beacon.off"));
  row(dl, "station.adapter", t("station.mtu", { name: s.adapter.name, a: s.adapter.mtu, r: s.adapter.radio_mtu }));
  row(dl, "station.tnc", s.tnc.name + " · " + s.tnc.profile);
  row(dl, "station.running", duration(s.uptime_s));
  document.getElementById("tnc-dot").className = "dot" + (s.tnc.connected ? " on" : "");
  document.getElementById("tnc-text").textContent = s.tnc.connected
    ? t("tnc.on", { t: duration(s.tnc.for_s) }) : t("tnc.off");
}

function drawInternet(s) {
  const box = document.getElementById("internet");
  const net = s.internet;
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
  document.getElementById("stations").hidden = list.length === 0;
  document.getElementById("stations-empty").hidden = list.length !== 0;
  for (const st of list) {
    const chosen = s.internet.mode === "station" && s.internet.call === st.call;
    let action = null;
    if (st.proxy) {
      action = chosen ? el("span", { class: "muted small", text: t("in.use") })
        : el("button", { text: t("btn.use"), onclick: () => choose("station", st.call) });
    }
    body.append(el("tr", { class: s.internet.using === st.call ? "chosen" : "" },
      el("td", { class: "mono", text: st.call }),
      el("td", { class: "mono", text: st.ip }),
      el("td", {}, badges(st.tags)),
      el("td", { class: "mono", text: st.proxy || "–" }),
      el("td", { text: ago(st.heard_s) }),
      el("td", {}, action)));
  }
}

function drawGateway(s) {
  const card = document.getElementById("gateway-card");
  card.hidden = !s.gateway;
  if (!s.gateway) return;
  const dl = document.getElementById("gateway");
  dl.replaceChildren();
  row(dl, "gateway.listen", el("span", { class: "mono", text: s.gateway.listen }));
  row(dl, "gateway.lan", t(s.gateway.lan ? "lan.open" : "lan.closed"));
}

function drawCounters(s) {
  const box = document.getElementById("counters");
  box.replaceChildren();
  for (const g of GROUPS) {
    if (g === "internet" && s.gateway && s.internet.mode === "off") continue;
    const items = (s.counters || []).filter((c) => c.group === g);
    if (!items.length) continue;
    const tiles = el("div", { class: "tiles" });
    for (const c of items) {
      tiles.append(el("div", { class: "tile" },
        el("div", { class: "v", text: c.bytes ? size(c.value) : c.value.toLocaleString(document.documentElement.lang) }),
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
    document.getElementById("offline").hidden = true;
    drawStation(s);
    drawInternet(s);
    drawStations(s);
    drawGateway(s);
    drawCounters(s);
  } catch (e) {
    document.getElementById("offline").hidden = false;
  }
}

refresh();
setInterval(refresh, 3000);
