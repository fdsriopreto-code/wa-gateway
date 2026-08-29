"use strict";

/* ============================================================ state */
const LS = {
  get base() { return localStorage.getItem("wa.base") || location.origin; },
  set base(v) { localStorage.setItem("wa.base", v.replace(/\/+$/, "")); },
  get key() { return localStorage.getItem("wa.key") || ""; },
  set key(v) { localStorage.setItem("wa.key", v); },
  get theme() { return localStorage.getItem("wa.theme") || "dark"; },
  set theme(v) { localStorage.setItem("wa.theme", v); },
};

/* ============================================================ dom helpers */
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
const fmtTime = (s) => { if (!s) return "—"; const d = new Date(typeof s === "number" ? s * 1000 : s); return isNaN(d) ? "—" : d.toLocaleString(); };
const fmtRel = (s) => { if (!s) return ""; const d = new Date(typeof s === "number" ? s * 1000 : s), diff = (Date.now() - d) / 1000;
  if (Math.abs(diff) < 60) return "agora"; if (diff < 3600) return Math.floor(diff / 60) + "min"; if (diff < 86400) return Math.floor(diff / 3600) + "h"; return Math.floor(diff / 86400) + "d"; };
const trunc = (s, n = 16) => !s ? "" : (s.length > n ? s.slice(0, n) + "…" : s);
const esc = (s) => String(s).replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c]));

/* ============================================================ api */
async function api(method, path, body) {
  const res = await fetch(LS.base + path, {
    method,
    headers: { "X-Api-Key": LS.key, ...(body !== undefined ? { "Content-Type": "application/json" } : {}) },
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  const text = await res.text();
  let data = null;
  try { data = text ? JSON.parse(text) : null; } catch { data = text; }
  if (!res.ok) {
    const e = new Error((data && (data.message || data.error)) || res.statusText || ("HTTP " + res.status));
    e.status = res.status; e.data = data;
    throw e;
  }
  return data;
}

/* ============================================================ ui bits */
function toast(msg, kind = "", title) {
  const t = h("div", { class: "toast " + kind }, title ? h("b", {}, title) : null, msg);
  $("#toast-root").append(t);
  setTimeout(() => { t.style.opacity = "0"; setTimeout(() => t.remove(), 200); }, 4200);
}
const ok = (m) => toast(m, "ok");
const fail = (e) => toast(typeof e === "string" ? e : (e.message || "erro"), "err", typeof e === "object" && e.status ? "erro " + e.status : null);

function modal(title, sub, contentNode) {
  const root = $("#modal-root");
  const close = () => clear(root);
  const box = h("div", { class: "modal" },
    h("h2", {}, title),
    sub ? h("p", { class: "sub" }, sub) : null,
    contentNode,
    h("div", { class: "btn-row" }, h("button", { class: "btn ghost", onclick: close }, "Fechar")),
  );
  const bg = h("div", { class: "modal-bg", onclick: (e) => { if (e.target === bg) close(); } }, box);
  clear(root); root.append(bg);
  return { close, box };
}

function hl(v, ind = 0) {
  const pad = "  ".repeat(ind), pad1 = "  ".repeat(ind + 1);
  if (v === null) return '<span class="j-null">null</span>';
  if (typeof v === "string") return '<span class="j-str">"' + esc(v) + '"</span>';
  if (typeof v === "number") return '<span class="j-num">' + v + "</span>";
  if (typeof v === "boolean") return '<span class="j-bool">' + v + "</span>";
  if (Array.isArray(v)) {
    if (!v.length) return "[]";
    return "[\n" + v.map((x) => pad1 + hl(x, ind + 1)).join(",\n") + "\n" + pad + "]";
  }
  const keys = Object.keys(v);
  if (!keys.length) return "{}";
  return "{\n" + keys.map((k) => pad1 + '<span class="j-key">"' + esc(k) + '"</span>: ' + hl(v[k], ind + 1)).join(",\n") + "\n" + pad + "}";
}
function jsonOut(data) {
  const pre = h("pre", { class: "out" });
  const raw = typeof data === "string" ? data : JSON.stringify(data, null, 2);
  pre.innerHTML = (typeof data === "string" ? esc(data) : hl(data));
  const copy = h("span", { class: "copy", onclick: () => { navigator.clipboard.writeText(raw); ok("copiado"); } }, "copiar");
  return h("div", {}, copy, pre);
}
const spinner = () => h("div", { class: "loading" }, h("span", { class: "spinner" }));
const emptyState = (ico, text) => h("div", { class: "empty" }, h("div", { class: "ico" }, ico), text);

function badge(text) {
  const t = String(text || "?");
  return h("span", { class: "badge s-" + t.toLowerCase() }, h("span", { class: "dot" }), t);
}

function table(cols, rows, emptyMsg) {
  if (!rows || !rows.length) return emptyState("∅", emptyMsg || "nada por aqui");
  return h("div", { class: "table-wrap" }, h("div", { class: "table-scroll" }, h("table", {},
    h("thead", {}, h("tr", {}, cols.map((c) => h("th", {}, c.h)))),
    h("tbody", {}, rows.map((r) => h("tr", {}, cols.map((c) => {
      const val = c.get(r);
      return h("td", { class: c.cls || "" }, val && val.nodeType ? val : (val == null ? "" : String(val)));
    })))),
  )));
}

async function sessionNames() {
  try { return ((await api("GET", "/api/sessions")) || []).map((s) => s.name); }
  catch { return []; }
}
function sessionSelect(id, names, cur) {
  return h("select", { id }, names.length
    ? names.map((n) => h("option", { value: n, selected: n === cur || undefined }, n))
    : h("option", { value: "" }, "— sem sessões —"));
}
async function withLoading(root, fn) {
  const s = spinner(); root.append(s);
  try { await fn(); } finally { s.remove(); }
}

/* ============================================================ views */
const views = {};

/* ---- dashboard ---- */
views.dashboard = { title: "Dashboard", async render(root) {
  await withLoading(root, async () => {
    let stats, sessions = [];
    try { [stats, sessions] = await Promise.all([api("GET", "/api/stats"), api("GET", "/api/sessions")]); }
    catch (e) { root.append(emptyState("⚠", "falha ao carregar: " + e.message)); return; }
    const by = (stats.sessions && stats.sessions.byStatus) || {};
    const cards = [
      ["◉", "Sessões", stats.sessions ? stats.sessions.total : 0, false],
      ["●", "Conectadas", by.WORKING || 0, !(by.WORKING)],
      ["◍", "Aguardando QR", by.SCAN_QR_CODE || 0, !(by.SCAN_QR_CODE)],
      ["○", "Paradas", (by.STOPPED || 0) + (by.LOGGED_OUT || 0), true],
      ["✕", "Falhas", by.FAILED || 0, !(by.FAILED)],
    ];
    root.append(
      h("div", { class: "grid stats" }, cards.map(([ico, label, n, dim]) =>
        h("div", { class: "card stat" },
          h("div", { class: "label" }, h("span", {}, ico), label),
          h("div", { class: "value" + (dim ? " dim" : "") }, n)))),
      h("h2", { class: "section" }, "Sessões"),
      (sessions && sessions.length)
        ? table([
            { h: "nome", get: (s) => h("b", {}, s.name) },
            { h: "status", get: (s) => badge(s.status) },
            { h: "jid", get: (s) => h("span", { class: "mono muted" }, s.jid || "—") },
            { h: "", get: (s) => h("a", { class: "btn ghost sm", href: "#/sessions" }, "gerenciar") },
          ], sessions)
        : emptyState("◉", "nenhuma sessão ainda — crie em Sessões"),
      h("h2", { class: "section" }, "Ambiente"),
      h("div", { class: "panel" },
        kv("versão", stats.version || "?"),
        kv("iniciado", fmtTime(stats.started)),
        kv("database", stats.database ? badge("ok") : badge("failed")),
        kv("base", h("span", { class: "mono muted" }, LS.base)),
      ),
    );
  });
}};
function kv(k, v) {
  return h("div", { style: "display:flex;gap:12px;padding:6px 0;font-size:13px" },
    h("span", { style: "min-width:110px;color:var(--fg-3);font-weight:600" }, k),
    h("span", {}, v && v.nodeType ? v : String(v)));
}

/* ---- sessions ---- */
views.sessions = { title: "Sessões", async render(root) {
  await withLoading(root, async () => {
    const list = await api("GET", "/api/sessions").catch((e) => { fail(e); return []; });
    const grid = h("div", { class: "grid cards" });

    (list || []).forEach((s) => {
      const act = (verb, fn, danger) => h("button", { class: "btn ghost sm" + (danger ? " danger" : ""), onclick: async (e) => {
        e.target.disabled = true;
        try { await fn(); ok(s.name + ": " + verb); route(); } catch (err) { fail(err); e.target.disabled = false; }
      } }, verb);
      grid.append(h("div", { class: "card sess-card" },
        h("div", { class: "head" },
          h("div", {}, h("div", { class: "nm" }, s.name),
            h("div", { class: "meta", style: "margin-top:6px" },
              h("span", {}, "engine: " + s.engine),
              h("span", { class: "mono" }, s.jid || "sem JID"))),
          badge(s.status)),
        s.status === "SCAN_QR_CODE" ? h("button", { class: "btn sm", onclick: () => qrModal(s.name) }, "📷 Escanear QR") : null,
        h("div", { class: "foot" },
          act("start", () => api("POST", `/api/sessions/${s.name}/start`)),
          act("stop", () => api("POST", `/api/sessions/${s.name}/stop`)),
          act("restart", () => api("POST", `/api/sessions/${s.name}/restart`)),
          act("logout", () => api("POST", `/api/sessions/${s.name}/logout`)),
          h("button", { class: "btn ghost sm", onclick: () => cfgModal(s) }, "config"),
          h("button", { class: "btn ghost sm danger", onclick: async () => {
            if (!confirm(`apagar a sessão "${s.name}"?`)) return;
            try { await api("DELETE", `/api/sessions/${s.name}`); ok("apagada"); route(); } catch (e) { fail(e); }
          } }, "apagar")),
      ));
    });

    root.append(
      list && list.length ? grid : emptyState("◉", "nenhuma sessão — crie uma abaixo"),
      h("h2", { class: "section" }, "Nova sessão"),
      h("div", { class: "panel" },
        h("div", { class: "row" },
          h("div", { class: "field" }, h("label", {}, "nome"), h("input", { id: "s-name", placeholder: "default" })),
          h("div", { class: "field tight" }, h("label", {}, " "),
            h("label", { class: "check" }, h("input", { type: "checkbox", id: "s-start", checked: true }), "iniciar já"))),
        h("div", { class: "field" }, h("label", {}, "config (JSON opcional) — webhooks / outbox"),
          h("textarea", { id: "s-config", placeholder: '{\n  "webhooks": [\n    { "url": "https://…", "events": ["message","session.status"], "hmac": { "secret": "x" } }\n  ]\n}' })),
        h("div", { class: "btn-row" }, h("button", { class: "btn", onclick: createSession }, "Criar sessão"))),
    );

    async function createSession(ev) {
      const name = $("#s-name").value.trim();
      if (!name) return fail("nome obrigatório");
      let config;
      const raw = $("#s-config").value.trim();
      if (raw) { try { config = JSON.parse(raw); } catch { return fail("config não é JSON válido"); } }
      ev.target.disabled = true;
      try { await api("POST", "/api/sessions", { name, start: $("#s-start").checked, config }); ok("sessão criada"); route(); }
      catch (e) { fail(e); ev.target.disabled = false; }
    }
  });
}};

function qrModal(name) {
  const box = h("div", { class: "qr wait" }, "gerando QR…");
  const status = h("p", { class: "sub", style: "text-align:center;margin:12px 0 0" }, "");
  modal("Conectar " + name, "WhatsApp → Aparelhos conectados → Conectar aparelho",
    h("div", {}, box, status));
  let stop = false;
  const tick = async () => {
    if (stop || !$("#modal-root .modal")) return;
    try {
      const s = await api("GET", `/api/sessions/${name}`);
      status.textContent = "status: " + s.status;
      if (s.status === "WORKING") { stop = true; ok(name + " conectada! ✓"); clear($("#modal-root")); route(); return; }
      if (s.status === "SCAN_QR_CODE") {
        box.className = "qr";
        clear(box);
        box.append(h("img", { alt: "QR", src: `${LS.base}/api/${encodeURIComponent(name)}/auth/qr.png?api_key=${encodeURIComponent(LS.key)}&_t=${Date.now()}` }));
      }
    } catch { /* ignora */ }
    setTimeout(tick, 3000);
  };
  tick();
  const bg = $("#modal-root .modal-bg");
  if (bg) bg.addEventListener("click", () => { stop = true; });
}

function cfgModal(s) {
  const ta = h("textarea", { style: "min-height:220px" }, JSON.stringify(s.config || {}, null, 2));
  const m = modal("Config — " + s.name, "webhooks, hmac, retries, outbox (pacing por sessão)",
    h("div", {}, ta, h("div", { class: "btn-row" },
      h("button", { class: "btn", onclick: async (e) => {
        let config;
        try { config = JSON.parse(ta.value); } catch { return fail("JSON inválido"); }
        e.target.disabled = true;
        try { await api("PUT", `/api/sessions/${s.name}`, { config }); ok("config salva"); m.close(); route(); }
        catch (err) { fail(err); e.target.disabled = false; }
      } }, "Salvar"))));
}

/* ---- console (envio) ---- */
views.console = { title: "Console de envio", async render(root) {
  const names = await sessionNames();
  const fields = h("div", {});
  const out = h("div", {});

  const specs = {
    text: () => [tf("chatId", "chatId", "5517999999999@s.whatsapp.net"), ta("text", "texto")],
    image: () => mediaF("image"), file: () => mediaF("file"), video: () => mediaF("video"), audio: () => mediaF("audio", true),
    location: () => [tf("chatId"), rowF([nf("latitude"), nf("longitude")]), rowF([tf("name", "name (opc)"), tf("address", "address (opc)")])],
    contact: () => [tf("chatId"), tf("cname", "nome do contato"), tf("cphone", "telefone +55…")],
    reaction: () => [tf("chatId"), tf("messageId"), tf("emoji", 'emoji ("" = remover)'), cf("fromMe"), tf("senderId", "senderId (grupo)")],
    deleteMessage: () => [tf("chatId"), tf("messageId"), cf("fromMe"), tf("senderId", "senderId (opc)")],
    editMessage: () => [tf("chatId"), tf("messageId"), ta("text", "novo texto"), cf("fromMe")],
    sendSeen: () => [tf("chatId"), tf("messageId"), cf("fromMe"), tf("senderId", "senderId (opc)")],
    presence: () => [tf("chatId"), sf("state", ["typing", "recording", "paused"])],
  };
  const kindSel = h("select", { onchange: draw }, Object.keys(specs).map((k) => h("option", { value: k }, k)));
  function draw() { clear(fields); specs[kindSel.value]().forEach((f) => fields.append(f)); }

  const enq = h("input", { type: "checkbox" });
  const delay = h("input", { placeholder: "ex.: 30s", style: "max-width:110px" });

  root.append(h("div", { class: "panel" },
    h("div", { class: "row" },
      h("div", { class: "field" }, h("label", {}, "sessão"), sessionSelect("k-sess", names)),
      h("div", { class: "field" }, h("label", {}, "tipo"), kindSel)),
    fields,
    h("div", { class: "row", style: "align-items:flex-end;margin-top:4px" },
      h("div", { class: "field tight" }, h("label", {}, " "), h("label", { class: "check" }, enq, "enfileirar (pacing anti-ban)")),
      h("div", { class: "field tight" }, h("label", {}, "delay extra"), delay)),
    h("div", { class: "btn-row" }, h("button", { class: "btn", onclick: submit }, "Enviar")),
    out,
  ));
  draw();

  function v(id) { const e = $("#f-" + id); return e ? (e.type === "checkbox" ? e.checked : e.value) : undefined; }
  function mediaBody() { return { chatId: v("chatId"), data: v("data"), mimetype: v("mimetype") || undefined, filename: v("filename") || undefined, caption: v("caption") || undefined, voice: v("voice") || undefined }; }

  async function submit(ev) {
    const session = $("#k-sess").value;
    if (!session) return fail("selecione uma sessão");
    const k = kindSel.value;
    const q = { enqueue: enq.checked || undefined, delay: (enq.checked && delay.value) || undefined };
    let path, body = { session };
    if (k === "text") { path = "/api/sendText"; body = { ...body, chatId: v("chatId"), text: v("text"), ...q }; }
    else if (["image", "file", "video", "audio"].includes(k)) { path = "/api/send" + k[0].toUpperCase() + k.slice(1); body = { ...body, ...mediaBody(), ...q }; }
    else if (k === "location") { path = "/api/sendLocation"; body = { ...body, chatId: v("chatId"), latitude: parseFloat(v("latitude")), longitude: parseFloat(v("longitude")), name: v("name") || undefined, address: v("address") || undefined, ...q }; }
    else if (k === "contact") { path = "/api/sendContact"; body = { ...body, chatId: v("chatId"), contacts: [{ name: v("cname"), phone: v("cphone") || undefined }], ...q }; }
    else if (k === "reaction") { path = "/api/reaction"; body = { ...body, chatId: v("chatId"), messageId: v("messageId"), emoji: v("emoji") || "", fromMe: v("fromMe"), senderId: v("senderId") || undefined }; }
    else if (k === "deleteMessage") { path = "/api/deleteMessage"; body = { ...body, chatId: v("chatId"), messageId: v("messageId"), fromMe: v("fromMe"), senderId: v("senderId") || undefined }; }
    else if (k === "editMessage") { path = "/api/editMessage"; body = { ...body, chatId: v("chatId"), messageId: v("messageId"), text: v("text"), fromMe: v("fromMe") }; }
    else if (k === "sendSeen") { path = "/api/sendSeen"; body = { ...body, chatId: v("chatId"), messageId: v("messageId"), fromMe: v("fromMe"), senderId: v("senderId") || undefined }; }
    else if (k === "presence") { path = "/api/presence"; body = { ...body, chatId: v("chatId"), state: v("state") }; }

    ev.target.disabled = true;
    clear(out);
    try { const r = await api("POST", path, body); out.append(jsonOut(r)); ok(enq.checked ? "enfileirado" : "enviado"); }
    catch (e) { out.append(jsonOut(e.data || e.message)); fail(e); }
    finally { ev.target.disabled = false; }
  }
}};
const tf = (id, label, ph) => h("div", { class: "field" }, h("label", {}, label || id), h("input", { id: "f-" + id, placeholder: ph || "" }));
const ta = (id, label) => h("div", { class: "field" }, h("label", {}, label || id), h("textarea", { id: "f-" + id }));
const nf = (id, label) => h("div", { class: "field" }, h("label", {}, label || id), h("input", { id: "f-" + id, type: "number", step: "any" }));
const cf = (id, label) => h("div", { class: "field tight" }, h("label", { class: "check" }, h("input", { type: "checkbox", id: "f-" + id }), label || id));
const sf = (id, opts) => h("div", { class: "field" }, h("label", {}, id), h("select", { id: "f-" + id }, opts.map((o) => h("option", { value: o }, o))));
const rowF = (fs) => h("div", { class: "row" }, fs);
function mediaF(kind, isAudio) {
  return [
    tf("chatId", "chatId", "5517999999999@s.whatsapp.net"),
    ta("data", "data (base64 ou data URI)"),
    rowF([tf("mimetype", "mimetype (opc)"), kind === "file" ? tf("filename", "filename (opc)") : h("div", { class: "field" })]),
    ["file", "image", "video"].includes(kind) ? tf("caption", "caption (opc)") : h("div"),
    isAudio ? cf("voice", "nota de voz (PTT)") : h("div"),
  ];
}

/* ---- groups ---- */
views.groups = { title: "Grupos", async render(root) {
  const names = await sessionNames();
  const sel = sessionSelect("g-sess", names);
  const box = h("div", {});
  const load = async () => {
    const s = sel.value; if (!s) return;
    clear(box); await withLoading(box, async () => {
      try {
        const gs = await api("GET", "/api/groups?session=" + encodeURIComponent(s));
        box.append(table([
          { h: "nome", get: (g) => h("b", {}, g.name || "—") },
          { h: "jid", get: (g) => h("span", { class: "mono muted" }, g.jid) },
          { h: "membros", get: (g) => (g.participants || []).length || "—" },
          { h: "flags", get: (g) => [g.announce && "announce", g.locked && "locked"].filter(Boolean).join(" ") || "—" },
          { h: "", get: (g) => h("button", { class: "btn ghost sm", onclick: () => groupModal(s, g) }, "abrir") },
        ], gs, "nenhum grupo"));
      } catch (e) { box.append(emptyState("⚠", e.message)); }
    });
  };
  sel.addEventListener("change", load);

  root.append(
    h("div", { class: "panel" }, h("div", { class: "row" },
      h("div", { class: "field" }, h("label", {}, "sessão"), sel),
      h("div", { class: "field tight" }, h("label", {}, " "), h("button", { class: "btn ghost", onclick: load }, "Listar")))),
    h("h2", { class: "section" }, "Criar grupo"),
    h("div", { class: "panel" },
      h("div", { class: "field" }, h("label", {}, "nome"), h("input", { id: "g-name" })),
      h("div", { class: "field" }, h("label", {}, "participantes (um por linha)"), h("textarea", { id: "g-parts", placeholder: "5517999999999@s.whatsapp.net" })),
      h("div", { class: "btn-row" }, h("button", { class: "btn", onclick: async (e) => {
        const s = sel.value; if (!s) return fail("sessão");
        const participants = $("#g-parts").value.split("\n").map((x) => x.trim()).filter(Boolean);
        e.target.disabled = true;
        try { const r = await api("POST", "/api/groups", { session: s, name: $("#g-name").value, participants }); ok("grupo criado: " + r.jid); load(); }
        catch (err) { fail(err); } finally { e.target.disabled = false; }
      } }, "Criar"))),
    h("h2", { class: "section" }, "Entrar por link"),
    h("div", { class: "panel" }, h("div", { class: "row" },
      h("div", { class: "field" }, h("label", {}, "código do convite"), h("input", { id: "g-code", placeholder: "ABCdef123…" })),
      h("div", { class: "field tight" }, h("label", {}, " "), h("button", { class: "btn", onclick: async (e) => {
        const s = sel.value; if (!s) return fail("sessão");
        e.target.disabled = true;
        try { const r = await api("POST", "/api/groups/join", { session: s, code: $("#g-code").value.trim() }); ok("entrou: " + r.jid); load(); }
        catch (err) { fail(err); } finally { e.target.disabled = false; }
      } }, "Entrar")))),
    h("h2", { class: "section" }, "Grupos"), box,
  );
  if (sel.value) load();
}};
function groupModal(session, g) {
  const partInput = h("textarea", { placeholder: "um jid por linha" });
  const doParts = (action) => async (e) => {
    const participants = partInput.value.split("\n").map((x) => x.trim()).filter(Boolean);
    if (!participants.length) return fail("informe participantes");
    e.target.disabled = true;
    try { await api("POST", `/api/groups/${encodeURIComponent(g.jid)}/participants`, { session, action, participants }); ok(action + " ok"); }
    catch (err) { fail(err); } finally { e.target.disabled = false; }
  };
  const meta = (label, id, val, ep) => h("div", { class: "row" },
    h("div", { class: "field" }, h("label", {}, label), h("input", { id, value: val || "" })),
    h("div", { class: "field tight" }, h("label", {}, " "), h("button", { class: "btn sm", onclick: async () => {
      try { await api("PUT", `/api/groups/${encodeURIComponent(g.jid)}/${ep}`, { session, [ep]: $("#" + id).value }); ok(ep + " ok"); } catch (e) { fail(e); }
    } }, "salvar")));

  modal(g.name || "Grupo", g.jid, h("div", {},
    kv("tópico", g.topic || "—"),
    h("h2", { class: "section" }, "Participantes"),
    table([{ h: "jid", get: (p) => h("span", { class: "mono" }, p.jid) }, { h: "papel", get: (p) => p.isSuperAdmin ? "owner" : (p.isAdmin ? "admin" : "") }], g.participants || [], "sem participantes na resposta"),
    h("h2", { class: "section" }, "Gerenciar"),
    partInput,
    h("div", { class: "btn-row" },
      h("button", { class: "btn sm", onclick: doParts("add") }, "add"),
      h("button", { class: "btn ghost sm", onclick: doParts("remove") }, "remove"),
      h("button", { class: "btn ghost sm", onclick: doParts("promote") }, "promote"),
      h("button", { class: "btn ghost sm", onclick: doParts("demote") }, "demote")),
    h("h2", { class: "section" }, "Metadados"),
    meta("novo nome", "gm-name", g.name, "name"),
    meta("novo tópico", "gm-topic", g.topic, "topic"),
    h("div", { class: "btn-row" },
      h("button", { class: "btn ghost sm", onclick: async () => {
        try { const r = await api("GET", `/api/groups/${encodeURIComponent(g.jid)}/invite-link?session=${encodeURIComponent(session)}`); prompt("link do convite:", r.link); } catch (e) { fail(e); }
      } }, "invite link"),
      h("button", { class: "btn danger sm", onclick: async () => {
        if (!confirm("sair do grupo?")) return;
        try { await api("POST", `/api/groups/${encodeURIComponent(g.jid)}/leave`, { session }); ok("saiu"); clear($("#modal-root")); route(); } catch (e) { fail(e); }
      } }, "sair")),
  ));
}

/* ---- contacts ---- */
views.contacts = { title: "Contatos", async render(root) {
  const names = await sessionNames();
  const sel = sessionSelect("c-sess", names);
  const out = h("div", {});
  const run = (fn) => async (e) => { e.target.disabled = true; clear(out); out.append(spinner());
    try { const r = await fn(); clear(out); out.append(jsonOut(r)); } catch (err) { clear(out); out.append(jsonOut(err.data || err.message)); fail(err); } finally { e.target.disabled = false; } };
  const S = () => encodeURIComponent(sel.value);

  root.append(h("div", { class: "panel" },
    h("div", { class: "field" }, h("label", {}, "sessão"), sel),
    h("h2", { class: "section" }, "Checar número"),
    h("div", { class: "row" },
      h("div", { class: "field" }, h("label", {}, "telefones (vírgula)"), h("input", { id: "c-phone", placeholder: "+5517999999999,+55…" })),
      h("div", { class: "field tight" }, h("label", {}, " "), h("button", { class: "btn", onclick: run(() => api("GET", `/api/contacts/check?session=${S()}&phone=${encodeURIComponent($("#c-phone").value)}`)) }, "Checar"))),
    h("h2", { class: "section" }, "Info de perfil"),
    h("div", { class: "row" },
      h("div", { class: "field" }, h("label", {}, "jids (vírgula)"), h("input", { id: "c-jid", placeholder: "5517999999999@s.whatsapp.net" })),
      h("div", { class: "field tight" }, h("label", {}, " "), h("button", { class: "btn ghost", onclick: run(() => api("GET", `/api/contacts/info?session=${S()}&jid=${encodeURIComponent($("#c-jid").value)}`)) }, "Buscar"))),
    h("h2", { class: "section" }, "Foto de perfil"),
    h("div", { class: "row" },
      h("div", { class: "field" }, h("label", {}, "jid"), h("input", { id: "c-pic" })),
      h("div", { class: "field tight" }, h("label", {}, " "), h("button", { class: "btn ghost", onclick: run(() => api("GET", `/api/contacts/profile-picture?session=${S()}&jid=${encodeURIComponent($("#c-pic").value)}`)) }, "URL"))),
    out,
  ));
}};

/* ---- outbox / deliveries ---- */
views.outbox = { title: "Fila de saída", async render(root) {
  await listView(root, {
    icon: "⧗",
    path: (s, l) => `/api/outbox?session=${encodeURIComponent(s)}&limit=${l}`,
    cols: [
      { h: "criado", get: (j) => h("span", { class: "muted", title: fmtTime(j.createdAt) }, fmtRel(j.createdAt)) },
      { h: "kind", get: (j) => j.kind },
      { h: "status", get: (j) => badge(j.status) },
      { h: "runAt", get: (j) => h("span", { class: "muted" }, fmtTime(j.runAt)) },
      { h: "tent.", get: (j) => j.attempts },
      { h: "messageId", get: (j) => h("span", { class: "mono" }, trunc(j.messageId, 22)) },
      { h: "erro", get: (j) => h("span", { class: "muted wrap" }, j.lastError || "") },
    ],
  });
}};
views.deliveries = { title: "Webhooks", async render(root) {
  await listView(root, {
    icon: "✉",
    path: (s, l) => `/api/deliveries?session=${encodeURIComponent(s)}&limit=${l}`,
    cols: [
      { h: "criado", get: (d) => h("span", { class: "muted", title: fmtTime(d.createdAt) }, fmtRel(d.createdAt)) },
      { h: "evento", get: (d) => d.event },
      { h: "status", get: (d) => badge(d.status) },
      { h: "cód.", get: (d) => d.responseCode || "" },
      { h: "tent.", get: (d) => d.attempts },
      { h: "url", get: (d) => h("span", { class: "mono muted wrap" }, trunc(d.url, 44)) },
      { h: "erro", get: (d) => h("span", { class: "muted wrap" }, d.lastError || "") },
    ],
  });
}};
async function listView(root, opts) {
  const names = await sessionNames();
  const sel = sessionSelect("lv-sess", names);
  const box = h("div", {});
  const auto = h("input", { type: "checkbox" });
  let timer = null;
  const load = async () => {
    const s = sel.value; if (!s) { clear(box); box.append(emptyState(opts.icon, "selecione uma sessão")); return; }
    try { const rows = await api("GET", opts.path(s, 100)); clear(box); box.append(table(opts.cols, rows, "nada ainda")); }
    catch (e) { clear(box); box.append(emptyState("⚠", e.message)); }
  };
  sel.addEventListener("change", load);
  auto.addEventListener("change", () => { clearInterval(timer); if (auto.checked) timer = setInterval(load, 4000); });
  window.addEventListener("hashchange", () => clearInterval(timer), { once: true });
  root.append(h("div", { class: "panel" }, h("div", { class: "row" },
    h("div", { class: "field" }, h("label", {}, "sessão"), sel),
    h("div", { class: "field tight" }, h("label", {}, " "), h("button", { class: "btn ghost", onclick: load }, "Carregar")),
    h("div", { class: "field tight" }, h("label", {}, " "), h("label", { class: "check" }, auto, "auto 4s")))), box);
  load();
}

/* ---- events ---- */
views.events = { title: "Eventos ao vivo", async render(root) {
  const names = await sessionNames();
  const sel = sessionSelect("e-sess", names);
  const filter = h("input", { value: "*", style: "max-width:240px" });
  const feed = h("div", { class: "feed" });
  const pill = h("span", { class: "pill" }, h("span", { class: "dot" }), "desligado");
  let sock = null, paused = false;

  const stop = () => { if (sock) { sock.close(); sock = null; } setPill("desligado", ""); };
  const setPill = (t, c) => { pill.className = "pill " + c; pill.lastChild.textContent = t; };
  const start = () => {
    stop();
    const s = sel.value; if (!s) return fail("selecione uma sessão");
    const url = `${LS.base.replace(/^http/, "ws")}/ws?session=${encodeURIComponent(s)}&events=${encodeURIComponent(filter.value || "*")}&api_key=${encodeURIComponent(LS.key)}`;
    setPill("conectando…", "warn");
    sock = new WebSocket(url);
    sock.onopen = () => setPill("ao vivo", "ok");
    sock.onclose = () => setPill("fechado", "bad");
    sock.onerror = () => setPill("erro", "bad");
    sock.onmessage = (m) => {
      if (paused) return;
      let o; try { o = JSON.parse(m.data); } catch { o = { event: "?", payload: m.data }; }
      const ev = o.event || o.name || "?";
      const dom = ev.split(".")[0];
      const line = h("div", { class: "row" },
        h("span", { class: "t" }, new Date().toLocaleTimeString()),
        h("span", { class: "e d-" + dom }, ev),
        h("span", { class: "p", html: hl(o.payload !== undefined ? o.payload : o) }));
      feed.prepend(line);
      while (feed.childElementCount > 300) feed.lastChild.remove();
    };
  };
  window.addEventListener("hashchange", stop, { once: true });

  root.append(h("div", { class: "panel" }, h("div", { class: "row" },
    h("div", { class: "field" }, h("label", {}, "sessão"), sel),
    h("div", { class: "field" }, h("label", {}, "eventos (wildcard: message.*, *)"), filter),
    h("div", { class: "field tight" }, h("label", {}, " "), h("div", { class: "actions" },
      h("button", { class: "btn", onclick: start }, "Conectar"),
      h("button", { class: "btn ghost", onclick: stop }, "Parar"),
      h("button", { class: "btn ghost", onclick: (e) => { paused = !paused; e.target.textContent = paused ? "Retomar" : "Pausar"; } }, "Pausar"),
      h("button", { class: "btn ghost", onclick: () => clear(feed) }, "Limpar"),
      pill)))), feed);
}};

/* ---- keys ---- */
views.keys = { title: "API keys", async render(root) {
  await withLoading(root, async () => {
    let list;
    try { list = await api("GET", "/api/keys"); }
    catch (e) { root.append(emptyState("⚿", e.status === 403 ? "precisa de uma chave com escopo * (a master serve)" : e.message)); return; }
    root.append(
      table([
        { h: "id", get: (k) => h("span", { class: "mono" }, k.id) },
        { h: "label", get: (k) => k.label || "—" },
        { h: "escopos", get: (k) => (k.scopes || []).join(", ") },
        { h: "criada", get: (k) => h("span", { class: "muted" }, fmtRel(k.createdAt)) },
        { h: "estado", get: (k) => k.revokedAt ? badge("failed") : badge("ok") },
        { h: "", get: (k) => k.revokedAt ? "" : h("button", { class: "btn ghost sm danger", onclick: async () => {
          if (!confirm("revogar " + k.id + "?")) return;
          try { await api("DELETE", "/api/keys/" + encodeURIComponent(k.id)); ok("revogada"); route(); } catch (e) { fail(e); }
        } }, "revogar") },
      ], list, "nenhuma chave na tabela (a master de env não aparece aqui)"),
      h("h2", { class: "section" }, "Nova chave"),
      h("div", { class: "panel" },
        h("div", { class: "row" },
          h("div", { class: "field" }, h("label", {}, "label"), h("input", { id: "nk-label", placeholder: "app-x" })),
          h("div", { class: "field" }, h("label", {}, "escopos (vírgula, vazio = *)"), h("input", { id: "nk-scopes", placeholder: "*" }))),
        h("div", { class: "btn-row" }, h("button", { class: "btn", onclick: async (e) => {
          const scopes = $("#nk-scopes").value.split(",").map((x) => x.trim()).filter(Boolean);
          e.target.disabled = true;
          try {
            const r = await api("POST", "/api/keys", { label: $("#nk-label").value, scopes });
            modal("Chave criada", "copie agora — o token não é exibido de novo", h("pre", { class: "out" }, r.token));
            route();
          } catch (err) { fail(err); } finally { e.target.disabled = false; }
        } }, "Gerar chave"))),
    );
  });
}};

/* ---- settings ---- */
views.settings = { title: "Conexão", async render(root) {
  const base = h("input", { value: LS.base, placeholder: "https://…" });
  const key = h("input", { value: LS.key, type: "password", placeholder: "X-Api-Key" });
  const out = h("div", {});
  root.append(h("div", { class: "panel" },
    h("div", { class: "field" }, h("label", {}, "base URL do gateway"), base),
    h("div", { class: "field" }, h("label", {}, "API key"), key, h("div", { class: "hint" }, "a master key do .env também funciona aqui")),
    h("div", { class: "btn-row" },
      h("button", { class: "btn", onclick: () => { LS.base = base.value; LS.key = key.value; ok("salvo"); renderConn(); healthPoll(); } }, "Salvar"),
      h("button", { class: "btn ghost", onclick: async (e) => {
        clear(out); e.target.disabled = true;
        try { out.append(jsonOut(await api("GET", "/api/version"))); ok("conectou ✓"); } catch (err) { out.append(jsonOut(err.message)); fail(err); }
        finally { e.target.disabled = false; }
      } }, "Testar")),
    out,
    h("p", { class: "hint", style: "margin-top:16px" }, "As credenciais ficam só no localStorage deste navegador."),
  ));
}};

/* ============================================================ shell */
const NAV = [
  ["dashboard", "▚", "Dashboard"], ["sessions", "◉", "Sessões"], ["console", "➤", "Console"],
  ["groups", "◈", "Grupos"], ["contacts", "☺", "Contatos"], ["outbox", "⧗", "Fila de saída"],
  ["deliveries", "✉", "Webhooks"], ["events", "⚡", "Eventos"], ["keys", "⚿", "API keys"], ["settings", "⚙", "Conexão"],
];

function renderNav(active) {
  const nav = $("#nav"); clear(nav);
  NAV.forEach(([id, ico, label]) => nav.append(
    h("a", { href: "#/" + id, class: id === active ? "active" : "", onclick: () => $("#sidebar").classList.remove("open") },
      h("span", { class: "ico" }, ico), label)));
}
function renderConn() {
  const c = $("#conn"); clear(c);
  c.append(
    h("div", { class: "r" }, h("b", {}, "base"), h("span", {}, trunc(LS.base.replace(/^https?:\/\//, ""), 20))),
    h("div", { class: "r" }, h("b", {}, "key"), LS.key ? h("span", {}, trunc(LS.key, 8) + "…") : h("span", { class: "muted" }, "não definida")),
    h("button", { class: "theme-toggle", onclick: toggleTheme }, LS.theme === "dark" ? "☾ tema escuro" : "☀ tema claro"),
  );
}
function applyTheme() { document.documentElement.setAttribute("data-theme", LS.theme); }
function toggleTheme() { LS.theme = LS.theme === "dark" ? "light" : "dark"; applyTheme(); renderConn(); }

async function healthPoll() {
  const p = $("#health");
  try {
    const hh = await api("GET", "/health");
    p.className = "pill " + (hh.status === "ok" ? "ok" : "bad");
    p.lastChild.textContent = hh.status === "ok" ? "online" : hh.status;
  } catch { p.className = "pill bad"; p.lastChild.textContent = "offline"; }
}

async function route() {
  const hash = (location.hash.replace(/^#\/?/, "") || "dashboard").split("?")[0];
  if (!LS.key && hash !== "settings") { renderOnboard(); return; }
  const name = views[hash] ? hash : "dashboard";
  renderNav(name);
  $("#crumb").textContent = views[name].title;
  const view = $("#view"); clear(view);
  const wrap = h("div", {}); view.append(wrap);
  try { await views[name].render(wrap); }
  catch (e) { clear(wrap); wrap.append(emptyState("⚠", "erro na view: " + e.message)); }
}

function renderOnboard() {
  renderNav("settings");
  $("#crumb").textContent = "Bem-vindo";
  const view = $("#view"); clear(view);
  const base = h("input", { value: LS.base, placeholder: "https://…" });
  const key = h("input", { type: "password", placeholder: "X-Api-Key (a master do .env serve)" });
  view.append(h("div", { class: "onboard" },
    h("div", { class: "logo-lg" }, "w"),
    h("h1", {}, "Conectar ao gateway"),
    h("p", {}, "Cole a API key para começar. Fica só neste navegador."),
    h("div", { class: "panel", style: "text-align:left" },
      h("div", { class: "field" }, h("label", {}, "base URL"), base),
      h("div", { class: "field" }, h("label", {}, "API key"), key),
      h("div", { class: "btn-row" }, h("button", { class: "btn", style: "width:100%", onclick: async (e) => {
        if (!key.value.trim()) return fail("cole a API key");
        e.target.disabled = true;
        LS.base = base.value; LS.key = key.value;
        try { await api("GET", "/api/version"); ok("conectado ✓"); renderConn(); location.hash = "#/dashboard"; route(); }
        catch (err) { fail(err); e.target.disabled = false; }
      } }, "Entrar")))));
}

$("#refresh").addEventListener("click", route);
$("#menu-btn").addEventListener("click", () => $("#sidebar").classList.toggle("open"));
window.addEventListener("hashchange", route);

applyTheme();
renderConn();
route();
healthPoll();
setInterval(healthPoll, 15000);
