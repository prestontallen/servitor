// servitor GUI — vanilla JS, no build step, no dependencies.
// All data via the API; live updates via SSE with watermark resync.
"use strict";

const $ = (sel) => document.querySelector(sel);
const api = {
  token: localStorage.getItem("servitor_token") || "",
  auth(path) {
    if (!this.token) return path;
    return path + (path.includes("?") ? "&" : "?") + "token=" + encodeURIComponent(this.token);
  },
  async get(path) {
    const r = await fetch(this.auth(path));
    if (r.status === 401) {
      const t = prompt("API token:");
      if (t) { this.token = t; localStorage.setItem("servitor_token", t); return this.get(path); }
    }
    if (!r.ok) throw new Error(`${r.status} ${await r.text()}`);
    return r.json();
  },
};

// ---------- routing ----------
let head = 0; // per-connection watermark: max ledger event id seen
document.querySelectorAll("nav button").forEach((b) =>
  b.addEventListener("click", () => show(b.dataset.view))
);
function show(name) {
  document.querySelectorAll("nav button").forEach((b) =>
    b.classList.toggle("active", b.dataset.view === name)
  );
  for (const v of ["board", "ticket", "analytics"]) {
    const el = document.getElementById(`view-${v}`);
    if (el) el.hidden = v !== name;
  }
  if (name === "analytics") loadAnalytics(currentDays);
}

// ---------- board ----------
async function loadBoard() {
  const cards = await api.get("/api/board");
  const lanes = {};
  for (const lane of document.querySelectorAll(".lane")) {
    lane.querySelector(".cards").innerHTML = "";
    lanes[lane.dataset.status] = lane.querySelector(".cards");
  }
  for (const c of cards) {
    const el = document.createElement("div");
    el.className = "card";
    el.dataset.ulid = c.ulid;
    el.innerHTML = `
      <div class="title">${esc(c.title || c.slug)}</div>
      <div class="meta">
        <span>${esc(c.slug)}</span>
        ${c.card_word ? `<span class="badge">${esc(c.card_word)}</span>` : ""}
        ${c.blocked_on ? `<span class="badge blocked_on">on ${esc(c.blocked_on)}</span>` : ""}
      </div>`;
    el.addEventListener("click", () => openTicket(c.ulid));
    (lanes[c.status] || lanes.queued).appendChild(el);
  }
}

async function openTicket(ref) {
  const doc = await api.get(`/api/ticket/${ref}`);
  const el = document.getElementById("ticket-doc");
  el.innerHTML = `
    <div class="kv">
      <span>${esc(doc.slug)}</span>
      <span>${esc(doc.status)}</span>
      ${doc.card_word ? `<span class="badge">${esc(doc.card_word)}</span>` : ""}
      ${doc.pr !== null && doc.pr !== undefined ? `<span>pr: ${esc(doc.pr) || "(empty)"}</span>` : ""}
    </div>
    <h2>${esc(doc.title || doc.slug)}</h2>
    ${section("gates", doc.gates.map((g) =>
      `<li>${esc(g.gate)} <span class="meta-inline">${esc(g.actor)} · ${new Date(g.ts).toLocaleString()}</span></li>`))}
    ${section("criteria", doc.criteria.map((c) =>
      `<li><span class="${c.state === "pass" ? "state-pass" : c.state === "fail" ? "state-fail" : ""}">${c.state ? "●" : "○"}</span> ${esc(c.body)}</li>`))}
    ${section("plan", doc.plan.map((p) => `<li>${esc(p.body)}</li>`))}
    ${section("decisions", doc.decisions.map((d) =>
      `<li>${esc(d.what)} <span class="meta-inline">— ${esc(d.why || "")}</span></li>`))}
    ${section("questions", doc.questions.map((q) => `<li>${esc(q.body)}</li>`))}
    ${section("links", doc.links.map((l) =>
      `<li><a href="${esc(l.url)}" target="_blank">${esc(l.url)}</a></li>`))}
    ${section("notes", doc.notes.slice(0, 20).map((n) =>
      `<li>${esc(n.body)} <span class="meta-inline">${new Date(n.ts).toLocaleString()}</span></li>`))}`;
  show("ticket");
}
document.querySelector(".back").addEventListener("click", () => show("board"));

function section(name, items) {
  if (!items || items.length === 0) return "";
  return `<div class="section"><h3>${name}</h3><ul>${items.join("")}</ul></div>`;
}

// ---------- analytics (the time-series demo) ----------
let currentDays = 30;
document.querySelectorAll(".range button").forEach((b) =>
  b.addEventListener("click", () => {
    currentDays = +b.dataset.days;
    document.querySelectorAll(".range button").forEach((x) =>
      x.classList.toggle("active", x === b));
    loadAnalytics(currentDays);
  })
);

async function loadAnalytics(days) {
  const buckets = await api.get(`/api/analytics?days=${days}`);
  renderChart(buckets);
}

// dependency-free bar chart: events/day, hover breaks the day down by kind
function renderChart(buckets) {
  const chart = document.getElementById("chart");
  chart.innerHTML = "";
  const max = Math.max(1, ...buckets.map((b) => b.events));
  for (const b of buckets) {
    const bar = document.createElement("div");
    bar.className = "bar";
    bar.style.height = `${(b.events / max) * 100}%`;
    const kinds = Object.entries(b.by_kind || {})
      .sort((a, x) => x[1] - a[1])
      .map(([k, n]) => `${esc(k)}: ${n}`)
      .join("<br>");
    bar.innerHTML = `<span class="tip"><b>${b.day}</b><br>${b.events} events${kinds ? "<br>" + kinds : ""}</span>`;
    bar.title = `${b.day}: ${b.events}`;
    chart.appendChild(bar);
  }
  const kinds = {};
  for (const b of buckets) for (const [k, n] of Object.entries(b.by_kind || {})) kinds[k] = (kinds[k] || 0) + n;
  document.getElementById("kindlegend").innerHTML = Object.entries(kinds)
    .sort((a, x) => x[1] - a[1])
    .map(([k, n]) => `<span>${esc(k)} — ${n}</span>`)
    .join("");
}

// ---------- live updates: SSE + watermark resync ----------
function connect() {
  const conn = document.getElementById("conn");
  const es = new EventSource("/api/events/stream");
  es.onopen = () => { conn.textContent = "live"; conn.className = "conn live"; };
  es.addEventListener("change", (e) => {
    const c = JSON.parse(e.data);
    if (c.event_id > head) head = c.event_id;
    if (!document.getElementById("view-ticket").hidden) {
      // cheap: reload whatever is open
      const slug = document.querySelector("#ticket-doc .kv span");
      if (slug) openTicket(slug.textContent);
    } else {
      loadBoard();
    }
  });
  es.addEventListener("resync", reconnect);
  es.onerror = () => {
    conn.textContent = "reconnecting…";
    conn.className = "conn down";
    es.close();
    setTimeout(reconnect, 2000);
  };
}
async function reconnect() {
  // watermark resync: anything past `head` is refetched by the reloads below
  await loadBoard();
  connect();
}

// ---------- utils ----------
function esc(s) {
  return String(s ?? "").replace(/[&<>"']/g, (c) =>
    ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
}

loadBoard();
connect();
