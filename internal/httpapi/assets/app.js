"use strict";

/* ═══════════════════════════════════════════════ state */
const LS = {
  get base() { return localStorage.getItem("wa.base") || location.origin; },
  set base(v) { localStorage.setItem("wa.base", v.replace(/\/+$/, "")); },
  get key() { return localStorage.getItem("wa.key") || ""; },
  set key(v) { localStorage.setItem("wa.key", v); },
  get theme() { return localStorage.getItem("wa.theme") || "dark"; },
  set theme(v) { localStorage.setItem("wa.theme", v); },
  get sess() { return localStorage.getItem("wa.sess") || ""; },
  set sess(v) { localStorage.setItem("wa.sess", v); },
};

/* ═══════════════════════════════════════════════ dom */
function h(tag, attrs, ...kids) {
  const e = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs || {})) {
    if (v == null || v === false) continue;
    if (k === "class") e.className = v;
    else if (k === "html") e.innerHTML = v;
    else if (k === "style") e.style.cssText = v;
    else if (k.startsWith("on") && typeof v === "function") e.addEventListener(k.slice(2), v);
    else if (v === true) e.setAttribute(k, "");
    else e.setAttribute(k, v);
  }
  for (const kid of kids.flat()) {
    if (kid == null || kid === false) continue;
    e.append(kid.nodeType ? kid : document.createTextNode(String(kid)));
  }
  return e;
}
const $ = (s, r = document) => r.querySelector(s);
const clear = (n) => { while (n && n.firstChild) n.removeChild(n.firstChild); };
const esc = (s) => String(s).replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c]));
const fmtTime = (s) => { if (!s) return "—"; const d = new Date(typeof s === "number" ? s * 1000 : s); return isNaN(d) ? "—" : d.toLocaleString(); };
const fmtRel = (s) => { if (!s) return ""; const d = new Date(typeof s === "number" ? s * 1000 : s), diff = (Date.now() - d) / 1000;
  if (Math.abs(diff) < 60) return "agora"; if (diff < 3600) return Math.floor(diff / 60) + "min"; if (diff < 86400) return Math.floor(diff / 3600) + "h"; return Math.floor(diff / 86400) + "d"; };
const trunc = (s, n = 18) => !s ? "" : (s.length > n ? s.slice(0, n) + "…" : s);

/* ═══════════════════════════════════════════════ api */
async function api(method, path, body) {
  const t0 = performance.now();
  const res = await fetch(LS.base + path, {
    method,
    headers: { "X-Api-Key": LS.key, ...(body !== undefined ? { "Content-Type": "application/json" } : {}) },
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  const ms = Math.round(performance.now() - t0);
  const text = await res.text();
  let data = null;
  try { data = text ? JSON.parse(text) : null; } catch { data = text; }
  if (!res.ok) {
    const e = new Error((data && (data.message || data.error)) || res.statusText || ("HTTP " + res.status));
    e.status = res.status; e.data = data; e.ms = ms;
    throw e;
  }
  return { data, ms, status: res.status };
}
const apiData = async (m, p, b) => (await api(m, p, b)).data;

/* ═══════════════════════════════════════════════ ui bits */
function toast(msg, kind = "", title) {
  const t = h("div", { class: "toast " + kind }, title ? h("b", {}, title) : null, msg);
  $("#toast-root").append(t);
  setTimeout(() => { t.style.opacity = "0"; setTimeout(() => t.remove(), 200); }, 4000);
}
const ok = (m) => toast(m, "ok");
const fail = (e) => toast(typeof e === "string" ? e : (e.message || "erro"), "err", typeof e === "object" && e.status ? "HTTP " + e.status : null);

function modal(title, sub, node) {
  const root = $("#modal-root"), close = () => clear(root);
  const box = h("div", { class: "modal" }, h("h3", {}, title), sub ? h("div", { class: "msub" }, sub) : null, node,
    h("div", { class: "btn-row", style: "margin-top:16px" }, h("button", { class: "btn ghost", onclick: close }, "Fechar")));
  const bg = h("div", { class: "modal-bg", onclick: (e) => { if (e.target === bg) close(); } }, box);
  clear(root); root.append(bg);
  return { close };
}

function hl(v, ind = 0) {
  const p = "  ".repeat(ind), p1 = "  ".repeat(ind + 1);
  if (v === null) return '<span class="j-null">null</span>';
  if (typeof v === "string") return '<span class="j-str">"' + esc(v) + '"</span>';
  if (typeof v === "number" || typeof v === "boolean") return `<span class="j-${typeof v === "number" ? "num" : "bool"}">${v}</span>`;
  if (Array.isArray(v)) return v.length ? "[\n" + v.map((x) => p1 + hl(x, ind + 1)).join(",\n") + "\n" + p + "]" : "[]";
  const ks = Object.keys(v);
  return ks.length ? "{\n" + ks.map((k) => p1 + `<span class="j-key">"${esc(k)}"</span>: ` + hl(v[k], ind + 1)).join(",\n") + "\n" + p + "}" : "{}";
}
function jsonOut(data, extra) {
  const raw = typeof data === "string" ? data : JSON.stringify(data, null, 2);
  const pre = h("pre", { class: "out" });
  pre.innerHTML = typeof data === "string" ? esc(data) : hl(data);
  return h("div", {},
    h("div", { class: "out-bar" },
      extra || null,
      h("span", { onclick: () => { navigator.clipboard.writeText(raw); ok("copiado"); } }, "copiar JSON")),
    pre);
}
const spinner = () => h("span", { class: "spinner" });
const loadingBox = () => h("div", { class: "loading" }, spinner());
const empty = (ico, txt) => h("div", { class: "empty" }, h("div", { class: "ico" }, ico), txt);
const badge = (t) => h("span", { class: "badge s-" + String(t || "?").toLowerCase() }, h("span", { class: "dot" }), String(t || "?"));

function table(cols, rows, emptyMsg) {
  if (!rows || !rows.length) return empty("∅", emptyMsg || "nada aqui");
  return h("div", { class: "tbl-wrap" }, h("div", { class: "tbl-scroll" }, h("table", {},
    h("thead", {}, h("tr", {}, cols.map((c) => h("th", {}, c.h)))),
    h("tbody", {}, rows.map((r) => h("tr", {}, cols.map((c) => {
      const val = c.get(r);
      return h("td", { class: c.cls || "" }, val && val.nodeType ? val : (val == null ? "" : String(val)));
    })))))));
}
function kv(k, v) {
  return h("div", { style: "display:flex;gap:14px;padding:7px 0;font-size:12.5px;border-bottom:1px solid var(--border-2)" },
    h("span", { style: "min-width:110px;color:var(--faint);font-weight:600" }, k),
    h("span", {}, v && v.nodeType ? v : String(v)));
}

/* session cache */
let SESSIONS = [];
async function loadSessions() { try { SESSIONS = (await apiData("GET", "/api/sessions")) || []; } catch { SESSIONS = []; } return SESSIONS; }
async function withLoad(root, fn) { const s = loadingBox(); root.append(s); try { await fn(); } finally { s.remove(); } }

/* ═══════════════════════════════════════════════ ENDPOINT REGISTRY */
/* field: [key, type, opts]  types: session|text|pass|area|num|bool|select|lines|json */
const GROUPS = ["Sessões", "Mensagens", "Grupos", "Contatos", "Fila & monitor"];
const ENDPOINTS = [
  // ---- sessões
  { g: "Sessões", m: "GET", path: "/api/sessions", title: "Listar sessões", fields: [] },
  { g: "Sessões", m: "POST", path: "/api/sessions", title: "Criar sessão", fields: [
    ["name", "text", { req: 1, ph: "default" }], ["start", "bool", { def: true }],
    ["config", "json", { ph: '{"webhooks":[{"url":"https://…","events":["message"]}]}' }] ] },
  { g: "Sessões", m: "GET", path: "/api/sessions/{session}", title: "Ver sessão", fields: [["session", "session", { req: 1 }]] },
  { g: "Sessões", m: "PUT", path: "/api/sessions/{session}", title: "Atualizar config", fields: [
    ["session", "session", { req: 1 }], ["config", "json", { req: 1, ph: '{"outbox":{"minIntervalMs":8000}}' }] ] },
  { g: "Sessões", m: "DELETE", path: "/api/sessions/{session}", title: "Apagar sessão", fields: [["session", "session", { req: 1 }]] },
  ...["start", "stop", "restart", "logout"].map((a) => ({
    g: "Sessões", m: "POST", path: "/api/sessions/{session}/" + a, title: a[0].toUpperCase() + a.slice(1),
    fields: [["session", "session", { req: 1 }]] })),
  { g: "Sessões", m: "GET", path: "/api/{session}/auth/qr", title: "QR (texto)", fields: [["session", "session", { req: 1 }]] },

  // ---- mensagens
  { g: "Mensagens", m: "POST", path: "/api/sendText", title: "Enviar texto", fields: [
    ["session", "session", { req: 1 }], ["chatId", "text", { req: 1, ph: "5517999999999@s.whatsapp.net" }],
    ["text", "area", { req: 1 }], ["enqueue", "bool", { hint: "fila com pacing anti-ban" }], ["delay", "text", { ph: "30s" }] ] },
  ...[["sendImage", "image"], ["sendFile", "documento"], ["sendVideo", "vídeo"], ["sendAudio", "áudio"]].map(([p, l]) => ({
    g: "Mensagens", m: "POST", path: "/api/" + p, title: "Enviar " + l, fields: [
      ["session", "session", { req: 1 }], ["chatId", "text", { req: 1, ph: "…@s.whatsapp.net" }],
      ["data", "area", { req: 1, ph: "base64 ou data:URI" }], ["mimetype", "text", {}],
      p === "sendFile" ? ["filename", "text", {}] : ["caption", "text", {}],
      p === "sendAudio" ? ["voice", "bool", { hint: "nota de voz (PTT)" }] : null,
      ["enqueue", "bool", {}], ["delay", "text", { ph: "30s" }] ].filter(Boolean) })),
  { g: "Mensagens", m: "POST", path: "/api/sendLocation", title: "Enviar localização", fields: [
    ["session", "session", { req: 1 }], ["chatId", "text", { req: 1 }],
    ["latitude", "num", { req: 1 }], ["longitude", "num", { req: 1 }], ["name", "text", {}], ["address", "text", {}] ] },
  { g: "Mensagens", m: "POST", path: "/api/sendContact", title: "Enviar contato", custom: "contact", fields: [
    ["session", "session", { req: 1 }], ["chatId", "text", { req: 1 }],
    ["name", "text", { req: 1, ph: "nome do contato" }], ["phone", "text", { ph: "+55…" }] ] },
  { g: "Mensagens", m: "POST", path: "/api/reaction", title: "Reagir", fields: [
    ["session", "session", { req: 1 }], ["chatId", "text", { req: 1 }], ["messageId", "text", { req: 1 }],
    ["emoji", "text", { ph: '👍  ("" remove)' }], ["fromMe", "bool", {}], ["senderId", "text", { ph: "grupo: jid do autor" }] ] },
  { g: "Mensagens", m: "POST", path: "/api/editMessage", title: "Editar mensagem", fields: [
    ["session", "session", { req: 1 }], ["chatId", "text", { req: 1 }], ["messageId", "text", { req: 1 }], ["text", "area", { req: 1 }], ["fromMe", "bool", { def: true }] ] },
  { g: "Mensagens", m: "POST", path: "/api/deleteMessage", title: "Apagar (revoke)", fields: [
    ["session", "session", { req: 1 }], ["chatId", "text", { req: 1 }], ["messageId", "text", { req: 1 }], ["fromMe", "bool", { def: true }], ["senderId", "text", {}] ] },
  { g: "Mensagens", m: "POST", path: "/api/sendSeen", title: "Marcar como lida", fields: [
    ["session", "session", { req: 1 }], ["chatId", "text", { req: 1 }], ["messageId", "text", { req: 1 }], ["fromMe", "bool", {}], ["senderId", "text", {}] ] },
  { g: "Mensagens", m: "POST", path: "/api/presence", title: "Presença (digitando…)", fields: [
    ["session", "session", { req: 1 }], ["chatId", "text", { req: 1 }], ["state", "select", { opts: ["typing", "recording", "paused"], req: 1 }] ] },

  // ---- grupos
  { g: "Grupos", m: "GET", path: "/api/groups", title: "Listar grupos", fields: [["session", "session", { req: 1 }]] },
  { g: "Grupos", m: "POST", path: "/api/groups", title: "Criar grupo", fields: [
    ["session", "session", { req: 1 }], ["name", "text", { req: 1 }], ["participants", "lines", { ph: "um jid por linha" }] ] },
  { g: "Grupos", m: "POST", path: "/api/groups/join", title: "Entrar por link", fields: [
    ["session", "session", { req: 1 }], ["code", "text", { req: 1, ph: "código do convite" }] ] },
  { g: "Grupos", m: "GET", path: "/api/groups/{jid}", title: "Info do grupo", fields: [["session", "session", { req: 1 }], ["jid", "text", { req: 1, ph: "…@g.us" }]] },
  { g: "Grupos", m: "POST", path: "/api/groups/{jid}/leave", title: "Sair do grupo", fields: [["session", "session", { req: 1 }], ["jid", "text", { req: 1 }]] },
  { g: "Grupos", m: "POST", path: "/api/groups/{jid}/participants", title: "Participantes", fields: [
    ["session", "session", { req: 1 }], ["jid", "text", { req: 1 }],
    ["action", "select", { opts: ["add", "remove", "promote", "demote"], req: 1 }], ["participants", "lines", { req: 1 }] ] },
  { g: "Grupos", m: "PUT", path: "/api/groups/{jid}/name", title: "Renomear grupo", fields: [["session", "session", { req: 1 }], ["jid", "text", { req: 1 }], ["name", "text", { req: 1 }]] },
  { g: "Grupos", m: "PUT", path: "/api/groups/{jid}/topic", title: "Tópico do grupo", fields: [["session", "session", { req: 1 }], ["jid", "text", { req: 1 }], ["topic", "text", {}]] },
  { g: "Grupos", m: "GET", path: "/api/groups/{jid}/invite-link", title: "Link de convite", fields: [["session", "session", { req: 1 }], ["jid", "text", { req: 1 }], ["reset", "bool", {}]] },

  // ---- contatos
  { g: "Contatos", m: "GET", path: "/api/contacts/check", title: "Número está no WhatsApp?", fields: [
    ["session", "session", { req: 1 }], ["phone", "text", { req: 1, ph: "+5517999999999,+55…" }] ] },
  { g: "Contatos", m: "GET", path: "/api/contacts/info", title: "Info de perfil", fields: [
    ["session", "session", { req: 1 }], ["jid", "text", { req: 1, ph: "…@s.whatsapp.net" }] ] },
  { g: "Contatos", m: "GET", path: "/api/contacts/profile-picture", title: "Foto de perfil", fields: [
    ["session", "session", { req: 1 }], ["jid", "text", { req: 1 }], ["preview", "bool", {}] ] },

  // ---- fila & monitor
  { g: "Fila & monitor", m: "GET", path: "/api/outbox", title: "Jobs da fila de saída", fields: [["session", "session", { req: 1 }], ["limit", "num", { ph: "50" }]] },
  { g: "Fila & monitor", m: "GET", path: "/api/deliveries", title: "Entregas de webhook", fields: [["session", "session", { req: 1 }], ["limit", "num", { ph: "100" }]] },
  { g: "Fila & monitor", m: "GET", path: "/api/stats", title: "Estatísticas", fields: [] },
  { g: "Fila & monitor", m: "GET", path: "/api/keys", title: "Listar API keys", fields: [] },
  { g: "Fila & monitor", m: "POST", path: "/api/keys", title: "Criar API key", fields: [["label", "text", {}], ["scopes", "lines", { ph: "* (uma por linha)" }]] },
];

/* ---- endpoint card ---- */
function fieldEl(f) {
  const [k, t, o = {}] = f;
  const id = "in-" + Math.random().toString(36).slice(2);
  let input;
  if (t === "session") {
    input = h("select", { id }, SESSIONS.length
      ? SESSIONS.map((s) => h("option", { value: s.name, selected: s.name === LS.sess || undefined }, `${s.name} · ${s.status}`))
      : h("option", { value: "" }, "— sem sessões —"));
  } else if (t === "bool") {
    input = h("input", { id, type: "checkbox", checked: o.def || undefined });
    return { k, t, get: () => input.checked, node: h("div", { class: "field" }, h("label", { class: "check" }, input, k, o.hint ? h("span", { class: "hint" }, "— " + o.hint) : null)) };
  } else if (t === "select") {
    input = h("select", { id }, o.opts.map((x) => h("option", { value: x }, x)));
  } else if (t === "area" || t === "json" || t === "lines") {
    input = h("textarea", { id, placeholder: o.ph || "" });
  } else {
    input = h("input", { id, type: t === "num" ? "number" : (t === "pass" ? "password" : "text"), step: t === "num" ? "any" : undefined, placeholder: o.ph || "" });
  }
  const get = () => {
    const v = input.value;
    if (t === "num") return v === "" ? undefined : Number(v);
    if (t === "json") { if (!v.trim()) return undefined; try { return JSON.parse(v); } catch { throw new Error(`campo "${k}": JSON inválido`); } }
    if (t === "lines") { const a = v.split("\n").map((x) => x.trim()).filter(Boolean); return a.length ? a : undefined; }
    return v === "" ? undefined : v;
  };
  const node = h("div", { class: "field" },
    h("label", {}, k, o.req ? h("span", { class: "req" }, "*") : null),
    input,
    o.hint ? h("span", { class: "hint" }, o.hint) : null);
  return { k, t, get, node };
}

function endpointCard(ep) {
  const card = h("div", { class: "ep" });
  const body = h("div", { class: "ep-body" });
  const head = h("div", { class: "ep-head", onclick: () => card.classList.toggle("open") },
    h("span", { class: "method " + ep.m.toLowerCase() }, ep.m),
    h("span", { class: "path" }, ep.path),
    h("span", { class: "ttl" }, ep.title));
  card.append(head, body);

  const fields = ep.fields.map(fieldEl);
  fields.forEach((f) => body.append(f.node));
  const respBox = h("div", {});
  const runBtn = h("button", { class: "btn run", onclick: run }, "Enviar requisição");
  body.append(h("div", { class: "btn-row", style: "margin-top:10px" }, runBtn), respBox);

  async function run() {
    let vals;
    try {
      vals = {};
      for (const f of fields) { const v = f.get(); if (v !== undefined) vals[f.k] = v; }
    } catch (e) { return fail(e.message); }
    for (const [k, , o = {}] of ep.fields) if (o.req && (vals[k] === undefined || vals[k] === "")) return fail(`campo obrigatório: ${k}`);
    if (vals.session) LS.sess = vals.session;

    // monta URL + body
    let path = ep.path;
    const pathKeys = (ep.path.match(/\{(\w+)\}/g) || []).map((s) => s.slice(1, -1));
    pathKeys.forEach((pk) => { path = path.replace(`{${pk}}`, encodeURIComponent(vals[pk] ?? "")); delete vals[pk]; });

    let body;
    if (ep.custom === "contact") {
      body = { session: vals.session, chatId: vals.chatId, contacts: [{ name: vals.name, phone: vals.phone }] };
    } else if (ep.m === "GET" || ep.m === "DELETE") {
      const qs = new URLSearchParams();
      Object.entries(vals).forEach(([k, v]) => qs.set(k, v));
      const s = qs.toString(); if (s) path += "?" + s;
    } else {
      body = vals;
    }

    runBtn.disabled = true; clear(respBox);
    respBox.append(h("div", { class: "resp" }, loadingBox()));
    try {
      const r = await api(ep.m, path, body);
      renderResp(r.status, r.ms, r.data, ep.m, path, body);
      ok(`${ep.m} ${ep.path} → ${r.status}`);
    } catch (e) {
      renderResp(e.status || 0, e.ms || 0, e.data ?? e.message, ep.m, path, body);
      fail(e);
    } finally { runBtn.disabled = false; }
  }
  function renderResp(code, ms, data, m, path, body) {
    clear(respBox);
    const good = code >= 200 && code < 300;
    const curl = `curl -X ${m} '${LS.base}${path}' \\\n  -H 'X-Api-Key: ***'` +
      (body ? ` \\\n  -H 'Content-Type: application/json' \\\n  -d '${JSON.stringify(body)}'` : "");
    respBox.append(h("div", { class: "resp" },
      h("div", { class: "meta" },
        h("span", { class: "code " + (good ? "ok" : "bad") }, code || "ERR"),
        h("span", { class: "ms" }, ms + " ms")),
      jsonOut(data, h("span", { onclick: () => { navigator.clipboard.writeText(curl); ok("cURL copiado"); } }, "copiar cURL"))));
  }
  return card;
}

/* ═══════════════════════════════════════════════ views */
const views = {};

views.overview = { title: "Visão geral", async render(root) {
  const page = h("div", { class: "page" }); root.append(page);
  page.append(h("div", { class: "page-head" }, h("h2", {}, "Visão geral"), h("p", {}, "Estado do gateway e das sessões.")));
  await withLoad(page, async () => {
    let stats, sessions;
    try { [stats, sessions] = await Promise.all([apiData("GET", "/api/stats"), loadSessions()]); }
    catch (e) { page.append(empty("⚠", e.message)); return; }
    const by = (stats.sessions && stats.sessions.byStatus) || {};
    const cards = [
      ["◉", "Sessões", stats.sessions ? stats.sessions.total : 0, 0],
      ["●", "Conectadas", by.WORKING || 0, !by.WORKING],
      ["◍", "Aguardando QR", by.SCAN_QR_CODE || 0, !by.SCAN_QR_CODE],
      ["○", "Paradas", (by.STOPPED || 0) + (by.LOGGED_OUT || 0), 1],
      ["✕", "Falhas", by.FAILED || 0, !by.FAILED],
    ];
    page.append(
      h("div", { class: "kgrid" }, cards.map(([i, k, v, dim]) =>
        h("div", { class: "kpi" }, h("div", { class: "k" }, h("span", {}, i), k), h("div", { class: "v" + (dim ? " dim" : "") }, v)))),
      h("div", { class: "sec-title" }, "Sessões"),
      (sessions && sessions.length) ? table([
        { h: "nome", get: (s) => h("b", {}, s.name) },
        { h: "status", get: (s) => badge(s.status) },
        { h: "jid", get: (s) => h("span", { class: "mono muted" }, s.jid || "—") },
        { h: "", get: () => h("a", { class: "btn ghost sm", href: "#/sessions" }, "gerenciar") },
      ], sessions) : empty("◉", "nenhuma sessão — crie em Sessões"),
      h("div", { class: "sec-title" }, "Ambiente"),
      h("div", { class: "card pad" },
        kv("versão", stats.version || "?"), kv("iniciado", fmtTime(stats.started)),
        kv("database", stats.database ? badge("ok") : badge("failed")),
        kv("base", h("span", { class: "mono muted" }, LS.base))),
    );
  });
}};

views.sessions = { title: "Sessões", async render(root) {
  const page = h("div", { class: "page" }); root.append(page);
  page.append(h("div", { class: "page-head" }, h("h2", {}, "Sessões"), h("p", {}, "Status atualiza sozinho. Sessões conectadas voltam automaticamente se o servidor reiniciar.")));
  const grid = h("div", { class: "sess-grid" });
  const refs = new Map();          // name -> { card, badgeSlot, qrSlot, status }
  const pendingQR = new Set();     // nomes onde o usuário pediu start e espera o QR
  let poll = null;
  const done = (s) => ["WORKING", "FAILED", "STOPPED", "LOGGED_OUT"].includes(s);

  const buildCard = (s) => {
    const badgeSlot = h("span", { class: "badge-slot" }, badge(s.status));
    const qrSlot = h("div", { class: "qr-slot" });
    const act = (verb, fn, dg) => h("button", { class: "btn ghost sm" + (dg ? " danger" : ""), onclick: async (e) => {
      e.target.disabled = true;
      try {
        await apiData("POST", `/api/sessions/${s.name}/${fn}`);
        if (fn === "start" || fn === "restart") pendingQR.add(s.name);
        ok(`${s.name}: ${verb}`); await refresh();
      } catch (err) { fail(err); } finally { e.target.disabled = false; }
    } }, verb);
    const card = h("div", { class: "card sess", "data-name": s.name },
      h("div", { class: "top" },
        h("div", {}, h("div", { class: "nm" }, s.name),
          h("div", { class: "sub" }, h("span", {}, "engine: " + s.engine), h("span", { class: "mono jid" }, s.jid || "sem JID"))),
        badgeSlot),
      qrSlot,
      h("div", { class: "acts" },
        act("start", "start"), act("stop", "stop"), act("restart", "restart"), act("logout", "logout"),
        h("button", { class: "btn ghost sm", onclick: () => cfgModal(s) }, "config"),
        h("button", { class: "btn ghost sm danger", onclick: async () => {
          if (!confirm(`apagar "${s.name}"?`)) return;
          try { await apiData("DELETE", `/api/sessions/${s.name}`); ok("apagada"); await refresh(); } catch (e) { fail(e); }
        } }, "apagar")));
    setQR(qrSlot, s.name, s.status);
    refs.set(s.name, { card, badgeSlot, qrSlot, status: s.status });
    return card;
  };
  const setQR = (slot, name, status) => {
    clear(slot);
    if (status === "SCAN_QR_CODE") slot.append(h("button", { class: "btn sm", onclick: () => qrModal(name) }, "📷 Escanear QR"));
  };
  const apply = (s) => {
    const r = refs.get(s.name); if (!r) return;
    r.card.querySelector(".jid").textContent = s.jid || "sem JID";
    if (s.status !== r.status) {
      clear(r.badgeSlot); r.badgeSlot.append(badge(s.status));
      setQR(r.qrSlot, s.name, s.status);
      r.status = s.status;
      if (s.status === "SCAN_QR_CODE" && pendingQR.has(s.name)) { pendingQR.delete(s.name); qrModal(s.name); }
      if (s.status === "WORKING" && pendingQR.has(s.name)) { pendingQR.delete(s.name); ok(s.name + " conectada ✓"); }
    }
  };
  const refresh = async () => {
    let list; try { list = await apiData("GET", "/api/sessions") || []; } catch { return; }
    SESSIONS = list;
    const now = list.map((s) => s.name).sort().join(",");
    const had = [...refs.keys()].sort().join(",");
    if (now !== had) { rebuild(list); return; }
    list.forEach(apply);
    if (poll && list.every((s) => done(s.status)) && pendingQR.size === 0) { /* mantém: status pode mudar por fora */ }
  };
  const rebuild = (list) => {
    clear(grid); refs.clear();
    if (!list.length) { grid.append(empty("◉", "nenhuma sessão ainda")); return; }
    list.forEach((s) => grid.append(buildCard(s)));
  };

  await withLoad(page, async () => {
    const list = await loadSessions();
    rebuild(list);
    poll = setInterval(refresh, 2500);
    window.addEventListener("hashchange", () => clearInterval(poll), { once: true });
    page.append(
      grid,
      h("div", { class: "sec-title" }, "Nova sessão"),
      h("div", { class: "card pad" },
        h("div", { class: "frow" },
          h("div", { class: "field" }, h("label", {}, "nome"), h("input", { id: "s-name", placeholder: "default" })),
          h("div", { class: "field narrow" }, h("label", {}, " "), h("label", { class: "check" }, h("input", { type: "checkbox", id: "s-start", checked: true }), "iniciar já"))),
        h("div", { class: "field" }, h("label", {}, "config (JSON, opcional)"),
          h("textarea", { id: "s-cfg", placeholder: '{\n  "webhooks": [{ "url": "https://…", "events": ["message"], "hmac": { "secret": "x" } }],\n  "outbox": { "minIntervalMs": 6000 }\n}' })),
        h("div", { class: "btn-row", style: "margin-top:10px" }, h("button", { class: "btn", onclick: async (e) => {
          const name = $("#s-name").value.trim(); if (!name) return fail("nome obrigatório");
          let config; const raw = $("#s-cfg").value.trim();
          if (raw) { try { config = JSON.parse(raw); } catch { return fail("config: JSON inválido"); } }
          e.target.disabled = true;
          try { await apiData("POST", "/api/sessions", { name, start: $("#s-start").checked, config }); ok("criada"); route(); }
          catch (err) { fail(err); e.target.disabled = false; }
        } }, "Criar sessão"))),
    );
  });
}};

function qrModal(name) {
  const box = h("div", { class: "qr wait" }, "gerando QR…");
  const sub = h("div", { class: "msub", style: "text-align:center;margin:12px 0 0" }, "");
  modal("Conectar " + name, "WhatsApp → Aparelhos conectados → Conectar aparelho", h("div", {}, box, sub));
  let stop = false;
  const tick = async () => {
    if (stop || !$("#modal-root .modal")) return;
    try {
      const s = await apiData("GET", "/api/sessions/" + name);
      sub.textContent = "status: " + s.status;
      if (s.status === "WORKING") { stop = true; ok(name + " conectada ✓"); clear($("#modal-root")); route(); return; }
      if (s.status === "SCAN_QR_CODE") {
        box.className = "qr"; clear(box);
        box.append(h("img", { alt: "QR", src: `${LS.base}/api/${encodeURIComponent(name)}/auth/qr.png?api_key=${encodeURIComponent(LS.key)}&_t=${Date.now()}` }));
      }
    } catch {}
    setTimeout(tick, 3000);
  };
  tick();
  const bg = $("#modal-root .modal-bg"); if (bg) bg.addEventListener("click", () => { stop = true; });
}
function cfgModal(s) {
  const ta = h("textarea", { style: "min-height:220px" }, JSON.stringify(s.config || {}, null, 2));
  const m = modal("Config — " + s.name, "webhooks · hmac · retries · outbox (pacing por sessão)",
    h("div", {}, ta, h("div", { class: "btn-row", style: "margin-top:12px" }, h("button", { class: "btn", onclick: async (e) => {
      let config; try { config = JSON.parse(ta.value); } catch { return fail("JSON inválido"); }
      e.target.disabled = true;
      try { await apiData("PUT", `/api/sessions/${s.name}`, { config }); ok("salva"); m.close(); route(); }
      catch (err) { fail(err); e.target.disabled = false; }
    } }, "Salvar"))));
}

views.playground = { title: "Playground", async render(root) {
  await loadSessions();
  const page = h("div", { class: "page" }); root.append(page);
  page.append(h("div", { class: "page-head" }, h("h2", {}, "Playground"), h("p", {}, "Todos os endpoints, prontos pra testar. A sessão escolhida fica lembrada.")));
  const search = h("input", { placeholder: "🔎 filtrar endpoint…", style: "margin-bottom:14px" });
  const nav = h("div", { class: "pg-nav" });
  const list = h("div", { class: "pg-list" });
  page.append(search, h("div", { class: "pg" }, nav, list));

  let active = GROUPS[0];
  const draw = () => {
    clear(nav); clear(list);
    GROUPS.forEach((g) => nav.append(h("button", { class: g === active ? "active" : "", onclick: () => { active = g; search.value = ""; draw(); } }, g)));
    const q = search.value.toLowerCase();
    const eps = ENDPOINTS.filter((e) => (q ? (e.path + e.title).toLowerCase().includes(q) : e.g === active));
    if (!eps.length) { list.append(empty("∅", "nada encontrado")); return; }
    eps.forEach((e) => list.append(endpointCard(e)));
  };
  search.addEventListener("input", draw);
  draw();
}};

views.events = { title: "Eventos ao vivo", async render(root) {
  await loadSessions();
  const page = h("div", { class: "page" }); root.append(page);
  page.append(h("div", { class: "page-head" }, h("h2", {}, "Eventos ao vivo"), h("p", {}, "Stream do WebSocket da sessão.")));
  const sel = h("select", {}, SESSIONS.length ? SESSIONS.map((s) => h("option", { value: s.name, selected: s.name === LS.sess || undefined }, s.name)) : h("option", { value: "" }, "— sem sessões —"));
  const filter = h("input", { value: "*", style: "max-width:220px" });
  const feed = h("div", { class: "feed" });
  const pill = h("span", { class: "pill" }, h("span", { class: "dot" }), "desligado");
  let sock = null, paused = false;
  const setPill = (t, c) => { pill.className = "pill " + (c || ""); pill.lastChild.textContent = t; };
  const stop = () => { if (sock) { sock.close(); sock = null; } setPill("desligado", ""); };
  const start = () => {
    stop(); const s = sel.value; if (!s) return fail("selecione uma sessão");
    LS.sess = s;
    setPill("conectando…", "warn");
    sock = new WebSocket(`${LS.base.replace(/^http/, "ws")}/ws?session=${encodeURIComponent(s)}&events=${encodeURIComponent(filter.value || "*")}&api_key=${encodeURIComponent(LS.key)}`);
    sock.onopen = () => setPill("ao vivo", "ok");
    sock.onclose = () => setPill("fechado", "bad");
    sock.onerror = () => setPill("erro", "bad");
    sock.onmessage = (m) => {
      if (paused) return;
      let o; try { o = JSON.parse(m.data); } catch { o = { event: "?", payload: m.data }; }
      const ev = o.event || o.name || "?", dom = ev.split(".")[0];
      feed.prepend(h("div", { class: "ln" },
        h("span", { class: "t" }, new Date().toLocaleTimeString()),
        h("span", { class: "e d-" + dom }, ev),
        h("span", { class: "p", html: hl(o.payload !== undefined ? o.payload : o) })));
      while (feed.childElementCount > 300) feed.lastChild.remove();
    };
  };
  window.addEventListener("hashchange", stop, { once: true });
  page.append(h("div", { class: "card pad", style: "margin-bottom:14px" }, h("div", { class: "frow", style: "align-items:flex-end" },
    h("div", { class: "field" }, h("label", {}, "sessão"), sel),
    h("div", { class: "field" }, h("label", {}, "eventos (wildcard: message.*, *)"), filter),
    h("div", { class: "field narrow" }, h("label", {}, " "), h("div", { class: "btn-row" },
      h("button", { class: "btn", onclick: start }, "Conectar"),
      h("button", { class: "btn ghost", onclick: stop }, "Parar"),
      h("button", { class: "btn ghost", onclick: (e) => { paused = !paused; e.target.textContent = paused ? "Retomar" : "Pausar"; } }, "Pausar"),
      h("button", { class: "btn ghost", onclick: () => clear(feed) }, "Limpar"), pill)))), feed);
}};

views.monitor = { title: "Monitoramento", async render(root) {
  await loadSessions();
  const page = h("div", { class: "page" }); root.append(page);
  page.append(h("div", { class: "page-head" }, h("h2", {}, "Monitoramento"), h("p", {}, "Fila de saída e entregas de webhook, por sessão.")));
  const sel = h("select", {}, SESSIONS.length ? SESSIONS.map((s) => h("option", { value: s.name, selected: s.name === LS.sess || undefined }, s.name)) : h("option", { value: "" }, "—"));
  const auto = h("input", { type: "checkbox" });
  const tabsEl = h("div", { class: "tabs" });
  const box = h("div", {});
  let tab = "outbox", timer = null;
  const cols = {
    outbox: [
      { h: "criado", get: (j) => h("span", { class: "muted", title: fmtTime(j.createdAt) }, fmtRel(j.createdAt)) },
      { h: "kind", get: (j) => j.kind }, { h: "status", get: (j) => badge(j.status) },
      { h: "runAt", get: (j) => h("span", { class: "muted" }, fmtTime(j.runAt)) },
      { h: "tent.", get: (j) => j.attempts },
      { h: "messageId", get: (j) => h("span", { class: "mono" }, trunc(j.messageId, 22)) },
      { h: "erro", get: (j) => h("span", { class: "muted wrap" }, j.lastError || "") } ],
    deliveries: [
      { h: "criado", get: (d) => h("span", { class: "muted", title: fmtTime(d.createdAt) }, fmtRel(d.createdAt)) },
      { h: "evento", get: (d) => d.event }, { h: "status", get: (d) => badge(d.status) },
      { h: "cód", get: (d) => d.responseCode || "" }, { h: "tent.", get: (d) => d.attempts },
      { h: "url", get: (d) => h("span", { class: "mono muted wrap" }, trunc(d.url, 44)) },
      { h: "erro", get: (d) => h("span", { class: "muted wrap" }, d.lastError || "") } ],
  };
  const load = async () => {
    const s = sel.value; if (!s) { clear(box); box.append(empty("⧗", "selecione uma sessão")); return; }
    LS.sess = s;
    try { const rows = await apiData("GET", `/api/${tab}?session=${encodeURIComponent(s)}&limit=100`); clear(box); box.append(table(cols[tab], rows, "nada ainda")); }
    catch (e) { clear(box); box.append(empty("⚠", e.message)); }
  };
  const drawTabs = () => { clear(tabsEl); [["outbox", "Fila de saída"], ["deliveries", "Webhooks"]].forEach(([t, l]) =>
    tabsEl.append(h("button", { class: t === tab ? "active" : "", onclick: () => { tab = t; drawTabs(); load(); } }, l))); };
  sel.addEventListener("change", load);
  auto.addEventListener("change", () => { clearInterval(timer); if (auto.checked) timer = setInterval(load, 4000); });
  window.addEventListener("hashchange", () => clearInterval(timer), { once: true });
  page.append(h("div", { class: "card pad", style: "margin-bottom:14px" }, h("div", { class: "frow", style: "align-items:flex-end" },
    h("div", { class: "field" }, h("label", {}, "sessão"), sel),
    h("div", { class: "field narrow" }, h("label", {}, " "), h("button", { class: "btn ghost", onclick: load }, "Carregar")),
    h("div", { class: "field narrow" }, h("label", {}, " "), h("label", { class: "check" }, auto, "auto 4s")))),
    tabsEl, box);
  drawTabs(); load();
}};

views.keys = { title: "API keys", async render(root) {
  const page = h("div", { class: "page" }); root.append(page);
  page.append(h("div", { class: "page-head" }, h("h2", {}, "API keys"), h("p", {}, "Chaves da tabela api_keys. A master do .env não aparece aqui.")));
  await withLoad(page, async () => {
    let list;
    try { list = await apiData("GET", "/api/keys"); }
    catch (e) { page.append(empty("⚿", e.status === 403 ? "precisa de uma chave com escopo * (a master serve)" : e.message)); return; }
    page.append(
      table([
        { h: "id", get: (k) => h("span", { class: "mono" }, k.id) },
        { h: "label", get: (k) => k.label || "—" },
        { h: "escopos", get: (k) => (k.scopes || []).join(", ") },
        { h: "criada", get: (k) => h("span", { class: "muted" }, fmtRel(k.createdAt)) },
        { h: "estado", get: (k) => k.revokedAt ? badge("failed") : badge("ok") },
        { h: "", get: (k) => k.revokedAt ? "" : h("button", { class: "btn ghost sm danger", onclick: async () => {
          if (!confirm("revogar " + k.id + "?")) return;
          try { await apiData("DELETE", "/api/keys/" + encodeURIComponent(k.id)); ok("revogada"); route(); } catch (e) { fail(e); }
        } }, "revogar") },
      ], list, "nenhuma chave"),
      h("div", { class: "sec-title" }, "Nova chave"),
      h("div", { class: "card pad" },
        h("div", { class: "frow" },
          h("div", { class: "field" }, h("label", {}, "label"), h("input", { id: "nk-l", placeholder: "app-x" })),
          h("div", { class: "field" }, h("label", {}, "escopos (vírgula, vazio = *)"), h("input", { id: "nk-s", placeholder: "*" }))),
        h("div", { class: "btn-row", style: "margin-top:10px" }, h("button", { class: "btn", onclick: async (e) => {
          const scopes = $("#nk-s").value.split(",").map((x) => x.trim()).filter(Boolean);
          e.target.disabled = true;
          try {
            const r = await apiData("POST", "/api/keys", { label: $("#nk-l").value, scopes });
            modal("Chave criada", "copie agora — não é exibida de novo", h("pre", { class: "out" }, r.token));
            route();
          } catch (err) { fail(err); } finally { e.target.disabled = false; }
        } }, "Gerar chave"))),
    );
  });
}};

views.settings = { title: "Conexão", async render(root) {
  const page = h("div", { class: "page" }); root.append(page);
  page.append(h("div", { class: "page-head" }, h("h2", {}, "Conexão"), h("p", {}, "Credenciais ficam só no localStorage deste navegador.")));
  const base = h("input", { value: LS.base, placeholder: "https://…" });
  const key = h("input", { value: LS.key, type: "password", placeholder: "X-Api-Key" });
  const out = h("div", { style: "margin-top:12px" });
  page.append(h("div", { class: "card pad" },
    h("div", { class: "field" }, h("label", {}, "base URL do gateway"), base),
    h("div", { class: "field" }, h("label", {}, "API key"), key, h("span", { class: "hint" }, "a master key do .env também funciona")),
    h("div", { class: "btn-row", style: "margin-top:10px" },
      h("button", { class: "btn", onclick: () => { LS.base = base.value; LS.key = key.value; ok("salvo"); renderFoot(); health(); } }, "Salvar"),
      h("button", { class: "btn ghost", onclick: async (e) => {
        clear(out); e.target.disabled = true;
        try { out.append(jsonOut(await apiData("GET", "/api/version"))); ok("conectou ✓"); } catch (err) { out.append(jsonOut(err.message)); fail(err); }
        finally { e.target.disabled = false; }
      } }, "Testar")),
    out));
}};

/* ═══════════════════════════════════════════════ shell */
const NAV = [
  ["Principal", [["overview", "▚", "Visão geral"], ["sessions", "◉", "Sessões"], ["playground", "◆", "Playground"]]],
  ["Observar", [["events", "⚡", "Eventos ao vivo"], ["monitor", "▤", "Monitoramento"]]],
  ["Config", [["keys", "⚿", "API keys"], ["settings", "⚙", "Conexão"]]],
];
function renderNav(active) {
  const nav = $("#nav"); clear(nav);
  NAV.forEach(([grp, items]) => {
    const g = h("div", { class: "nav-group" }, h("div", { class: "lbl" }, grp));
    items.forEach(([id, ico, label]) => g.append(h("a", {
      href: "#/" + id, class: id === active ? "active" : "",
      onclick: () => $("#sidebar").classList.remove("open"),
    }, h("span", { class: "ico" }, ico), label)));
    nav.append(g);
  });
}
function renderFoot() {
  const f = $("#foot"); clear(f);
  f.append(
    h("div", { class: "row" }, h("b", {}, "base"), h("span", {}, trunc(LS.base.replace(/^https?:\/\//, ""), 18))),
    h("div", { class: "row" }, h("b", {}, "key"), LS.key ? h("span", {}, trunc(LS.key, 7) + "…") : h("span", {}, "—")),
    h("button", { onclick: toggleTheme }, LS.theme === "dark" ? "☾  Tema escuro" : "☀  Tema claro"),
  );
}
function applyTheme() { document.documentElement.setAttribute("data-theme", LS.theme); }
function toggleTheme() { LS.theme = LS.theme === "dark" ? "light" : "dark"; applyTheme(); renderFoot(); }

async function health() {
  const p = $("#health");
  try { const hh = await apiData("GET", "/health"); p.className = "pill " + (hh.status === "ok" ? "ok" : "bad"); p.lastChild.textContent = hh.status === "ok" ? "online" : hh.status; }
  catch { p.className = "pill bad"; p.lastChild.textContent = "offline"; }
}

async function route() {
  const hash = (location.hash.replace(/^#\/?/, "") || "overview").split("?")[0];
  if (!LS.key && hash !== "settings") return onboard();
  const name = views[hash] ? hash : "overview";
  renderNav(name);
  $("#crumb").textContent = views[name].title;
  const view = $("#view"); clear(view);
  try { await views[name].render(view); }
  catch (e) { view.append(empty("⚠", "erro: " + e.message)); }
}
function onboard() {
  renderNav("settings");
  $("#crumb").textContent = "Bem-vindo";
  const view = $("#view"); clear(view);
  const base = h("input", { value: LS.base, placeholder: "https://…" });
  const key = h("input", { type: "password", placeholder: "X-Api-Key (a master do .env serve)" });
  view.append(h("div", { class: "onboard" },
    h("div", { class: "logo" }, "w"), h("h2", {}, "Conectar ao gateway"),
    h("p", {}, "Cole a API key para começar."),
    h("div", { class: "card pad", style: "text-align:left" },
      h("div", { class: "field" }, h("label", {}, "base URL"), base),
      h("div", { class: "field" }, h("label", {}, "API key"), key),
      h("div", { class: "btn-row", style: "margin-top:8px" }, h("button", { class: "btn block", onclick: async (e) => {
        if (!key.value.trim()) return fail("cole a API key");
        e.target.disabled = true; LS.base = base.value; LS.key = key.value;
        try { await apiData("GET", "/api/version"); ok("conectado ✓"); renderFoot(); location.hash = "#/overview"; route(); }
        catch (err) { fail(err); e.target.disabled = false; }
      } }, "Entrar")))));
}

$("#refresh").addEventListener("click", route);
$("#menu-btn").addEventListener("click", () => $("#sidebar").classList.toggle("open"));
window.addEventListener("hashchange", route);

applyTheme(); renderFoot(); route(); health(); setInterval(health, 15000);
