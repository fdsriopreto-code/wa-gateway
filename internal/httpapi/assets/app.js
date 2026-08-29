"use strict";

/* ------------------------------------------------------------------ helpers */

const LS = {
  get base() { return localStorage.getItem("wa.base") || location.origin; },
  set base(v) { localStorage.setItem("wa.base", v); },
  get key() { return localStorage.getItem("wa.key") || ""; },
  set key(v) { localStorage.setItem("wa.key", v); },
};

function h(tag, attrs, ...kids) {
  const e = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs || {})) {
    if (v == null || v === false) continue;
    if (k === "class") e.className = v;
    else if (k === "html") e.innerHTML = v;
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
const $ = (sel, root = document) => root.querySelector(sel);
const clear = (node) => { while (node.firstChild) node.removeChild(node.firstChild); };
const fmtTime = (s) => s ? new Date(s).toLocaleString() : "—";
const short = (s, n = 14) => !s ? "" : (s.length > n ? s.slice(0, n) + "…" : s);

async function api(method, path, body) {
  const res = await fetch(LS.base + path, {
    method,
    headers: {
      "X-Api-Key": LS.key,
      ...(body !== undefined ? { "Content-Type": "application/json" } : {}),
    },
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  const text = await res.text();
  let data = null;
  try { data = text ? JSON.parse(text) : null; } catch { data = text; }
  if (!res.ok) {
    const msg = (data && data.message) || (data && data.error) || res.statusText;
    const err = new Error(msg);
    err.status = res.status; err.data = data;
    throw err;
  }
  return data;
}

function toast(msg, kind = "") {
  const t = h("div", { class: "toast " + kind }, msg);
  $("#toast-root").append(t);
  setTimeout(() => t.remove(), 4500);
}
function ok(msg) { toast(msg, "ok"); }
function fail(e) { toast(typeof e === "string" ? e : (e.message || "erro"), "err"); }

function modal(title, contentNode, opts = {}) {
  const root = $("#modal-root");
  const close = () => clear(root);
  const box = h("div", { class: "modal" },
    h("h2", {}, title),
    contentNode,
    h("div", { class: "btn-row" },
      opts.hideClose ? null : h("button", { class: "btn ghost", onclick: close }, "Fechar"),
    ),
  );
  const bg = h("div", { class: "modal-bg", onclick: (e) => { if (e.target === bg) close(); } }, box);
  clear(root); root.append(bg);
  return { close, box };
}

function jsonOut(data) {
  return h("pre", { class: "out" }, typeof data === "string" ? data : JSON.stringify(data, null, 2));
}

/* table builder: cols = [{h, get}] */
function table(cols, rows, opts = {}) {
  if (!rows || !rows.length) return h("div", { class: "empty" }, opts.empty || "nada por aqui");
  return h("table", {},
    h("thead", {}, h("tr", {}, cols.map((c) => h("th", {}, c.h)))),
    h("tbody", {}, rows.map((r) => h("tr", {}, cols.map((c) => {
      const v = c.get(r);
      return h("td", { class: c.cls || "" }, v && v.nodeType ? v : (v == null ? "" : String(v)));
    })))),
  );
}

function badge(text) { return h("span", { class: "badge " + text }, text); }

/* ------------------------------------------------------------------ data */

async function sessionNames() {
  try {
    const list = await api("GET", "/api/sessions");
    return (list || []).map((s) => s.name);
  } catch { return []; }
}

function sessionSelect(id, names, current) {
  return h("select", { id },
    names.length ? names.map((n) => h("option", { value: n, selected: n === current || undefined }, n))
                 : h("option", { value: "" }, "— sem sessões —"));
}

/* ------------------------------------------------------------------ views */

const views = {};

/* ---- dashboard ---- */
views.dashboard = {
  title: "Dashboard",
  async render(root) {
    let stats;
    try { stats = await api("GET", "/api/stats"); }
    catch (e) { root.append(h("div", { class: "empty" }, "falha ao carregar stats: " + e.message)); return; }
    const by = (stats.sessions && stats.sessions.byStatus) || {};
    const cards = [
      ["Sessões", stats.sessions ? stats.sessions.total : 0],
      ["Conectadas", by.WORKING || 0],
      ["Aguardando QR", by.SCAN_QR_CODE || 0],
      ["Paradas", (by.STOPPED || 0) + (by.LOGGED_OUT || 0)],
      ["Falhas", by.FAILED || 0],
    ];
    root.append(
      h("div", { class: "grid" }, cards.map(([label, n]) =>
        h("div", { class: "card" }, h("h3", {}, label), h("div", { class: "big" }, n)))),
      h("h2", { class: "section" }, "Ambiente"),
      h("div", { class: "panel" },
        h("div", {}, h("b", {}, "versão: "), stats.version || "?"),
        h("div", {}, h("b", {}, "iniciado: "), fmtTime(stats.started)),
        h("div", {}, h("b", {}, "database: "), stats.database ? badge("ok") : badge("failed")),
        h("div", { class: "muted", style: "margin-top:8px" }, "base: " + LS.base),
      ),
    );
  },
};

/* ---- sessions ---- */
views.sessions = {
  title: "Sessões",
  async render(root) {
    const list = await api("GET", "/api/sessions").catch((e) => { fail(e); return []; });

    const create = h("div", { class: "panel" },
      h("h2", { class: "section", style: "margin-top:0" }, "Nova sessão"),
      h("div", { class: "row" },
        h("div", { class: "field" }, h("label", {}, "nome"), h("input", { id: "s-name", placeholder: "default" })),
        h("div", { class: "field", style: "flex:0 0 auto" }, h("label", {}, " "),
          h("label", { class: "check" }, h("input", { type: "checkbox", id: "s-start", checked: true }), "iniciar já")),
      ),
      h("div", { class: "field" },
        h("label", {}, "config (JSON, opcional) — webhooks / outbox"),
        h("textarea", { id: "s-config", placeholder: '{"webhooks":[{"url":"https://...","events":["*"],"hmac":{"secret":"x"}}]}' })),
      h("div", { class: "btn-row" },
        h("button", { class: "btn", onclick: async (ev) => {
          const name = $("#s-name").value.trim();
          if (!name) return fail("nome obrigatório");
          let config;
          const raw = $("#s-config").value.trim();
          if (raw) { try { config = JSON.parse(raw); } catch { return fail("config não é JSON válido"); } }
          ev.target.disabled = true;
          try {
            await api("POST", "/api/sessions", { name, start: $("#s-start").checked, config });
            ok("sessão criada"); route();
          } catch (e) { fail(e); } finally { ev.target.disabled = false; }
        } }, "Criar"),
      ),
    );

    const act = (name, verb, fn) => h("button", { class: "btn ghost sm", onclick: async (e) => {
      e.target.disabled = true;
      try { await fn(); ok(name + ": " + verb); route(); } catch (err) { fail(err); e.target.disabled = false; }
    } }, verb);

    const rows = table([
      { h: "nome", get: (s) => h("b", {}, s.name) },
      { h: "engine", get: (s) => s.engine, cls: "muted" },
      { h: "status", get: (s) => badge(s.status) },
      { h: "jid", get: (s) => h("span", { class: "mono" }, s.jid || "—") },
      { h: "ações", get: (s) => h("div", { class: "actions" },
        s.status === "SCAN_QR_CODE" ? h("button", { class: "btn sm", onclick: () => qrModal(s.name) }, "QR") : null,
        act(s.name, "start", () => api("POST", `/api/sessions/${s.name}/start`)),
        act(s.name, "stop", () => api("POST", `/api/sessions/${s.name}/stop`)),
        act(s.name, "restart", () => api("POST", `/api/sessions/${s.name}/restart`)),
        act(s.name, "logout", () => api("POST", `/api/sessions/${s.name}/logout`)),
        h("button", { class: "btn ghost sm", onclick: () => cfgModal(s) }, "config"),
        h("button", { class: "btn danger sm", onclick: async () => {
          if (!confirm(`apagar a sessão "${s.name}"?`)) return;
          try { await api("DELETE", `/api/sessions/${s.name}`); ok("apagada"); route(); } catch (e) { fail(e); }
        } }, "×"),
      ) },
    ], list, { empty: "nenhuma sessão — crie uma abaixo" });

    root.append(rows, h("div", { style: "height:16px" }), create);
  },
};

function qrModal(name) {
  const img = h("img", { alt: "QR" });
  const status = h("div", { class: "muted", style: "text-align:center;margin-top:8px" }, "carregando…");
  const m = modal("Escaneie o QR — " + name, h("div", {},
    h("div", { class: "qr" }, img),
    status,
    h("div", { class: "muted", style: "text-align:center;font-size:12px;margin-top:6px" },
      "WhatsApp → Aparelhos conectados → Conectar aparelho"),
  ));
  let stop = false;
  const tick = async () => {
    if (stop) return;
    img.src = `${LS.base}/api/${encodeURIComponent(name)}/auth/qr.png?api_key=${encodeURIComponent(LS.key)}&_t=${Date.now()}`;
    try {
      const s = await api("GET", `/api/sessions/${name}`);
      status.textContent = "status: " + s.status;
      if (s.status === "WORKING") { stop = true; ok(name + " conectada!"); m.close(); route(); return; }
    } catch { /* ignora */ }
    setTimeout(tick, 3000);
  };
  tick();
  const bg = $("#modal-root .modal-bg");
  if (bg) bg.addEventListener("click", (e) => { if (e.target === bg) stop = true; });
}

function cfgModal(s) {
  const ta = h("textarea", { style: "min-height:200px" }, JSON.stringify(s.config || {}, null, 2));
  modal("Config — " + s.name, h("div", {},
    ta,
    h("div", { class: "btn-row" },
      h("button", { class: "btn", onclick: async (e) => {
        let config;
        try { config = JSON.parse(ta.value); } catch { return fail("JSON inválido"); }
        e.target.disabled = true;
        try { await api("PUT", `/api/sessions/${s.name}`, { config }); ok("config salva"); clear($("#modal-root")); route(); }
        catch (err) { fail(err); e.target.disabled = false; }
      } }, "Salvar"),
    ),
  ));
}

/* ---- console (envio) ---- */
views.console = {
  title: "Console de envio",
  async render(root) {
    const names = await sessionNames();
    const fields = h("div", {});
    const out = h("div", {});

    const kinds = {
      text: () => [textField("chatId", "chatId", "55...@s.whatsapp.net"), areaField("text", "texto")],
      image: () => mediaFields("image"),
      file: () => mediaFields("file"),
      video: () => mediaFields("video"),
      audio: () => mediaFields("audio", true),
      location: () => [textField("chatId"), rowFields([numField("latitude"), numField("longitude")]),
        rowFields([textField("name", "name (opc)"), textField("address", "address (opc)")])],
      contact: () => [textField("chatId"), textField("cname", "nome do contato"), textField("cphone", "telefone (+55...)")],
      reaction: () => [textField("chatId"), textField("messageId"), textField("emoji", "emoji (vazio = remover)"),
        checkField("fromMe", "fromMe"), textField("senderId", "senderId (grupo, se não fromMe)")],
      deleteMessage: () => [textField("chatId"), textField("messageId"), checkField("fromMe", "fromMe"), textField("senderId", "senderId (opc)")],
      editMessage: () => [textField("chatId"), textField("messageId"), areaField("text", "novo texto"), checkField("fromMe", "fromMe")],
      sendSeen: () => [textField("chatId"), textField("messageId"), checkField("fromMe", "fromMe"), textField("senderId", "senderId (opc)")],
      presence: () => [textField("chatId"), selField("state", ["typing", "recording", "paused"])],
    };

    const kindSel = h("select", { id: "k", onchange: renderFields }, Object.keys(kinds).map((k) => h("option", { value: k }, k)));
    function renderFields() {
      clear(fields);
      kinds[kindSel.value]().forEach((f) => fields.append(f));
    }

    const enqueue = h("input", { type: "checkbox", id: "q-enq" });
    const delay = h("input", { id: "q-delay", placeholder: "ex.: 30s", style: "max-width:120px" });

    root.append(h("div", { class: "panel" },
      h("div", { class: "row" },
        h("div", { class: "field" }, h("label", {}, "sessão"), sessionSelect("k-sess", names)),
        h("div", { class: "field" }, h("label", {}, "tipo"), kindSel),
      ),
      fields,
      h("div", { class: "row", style: "align-items:flex-end" },
        h("div", { class: "field", style: "flex:0 0 auto" }, h("label", {}, " "),
          h("label", { class: "check" }, enqueue, "enfileirar (pacing anti-ban)")),
        h("div", { class: "field", style: "flex:0 0 auto" }, h("label", {}, "delay extra"), delay),
      ),
      h("div", { class: "btn-row" }, h("button", { class: "btn", onclick: submit }, "Enviar")),
      out,
    ));
    renderFields();

    function val(id) { const e = $("#f-" + id); return e ? (e.type === "checkbox" ? e.checked : e.value) : undefined; }
    async function submit(ev) {
      const session = $("#k-sess").value;
      if (!session) return fail("selecione uma sessão");
      const k = kindSel.value;
      let path, body = { session };
      const q = { enqueue: enqueue.checked || undefined, delay: (enqueue.checked && delay.value) || undefined };
      switch (k) {
        case "text": path = "/api/sendText"; body = { ...body, chatId: val("chatId"), text: val("text"), ...q }; break;
        case "image": case "file": case "video": case "audio":
          path = "/api/send" + k[0].toUpperCase() + k.slice(1);
          body = { ...body, ...mediaBody(), ...q }; break;
        case "location": path = "/api/sendLocation"; body = { ...body, chatId: val("chatId"),
          latitude: parseFloat(val("latitude")), longitude: parseFloat(val("longitude")),
          name: val("name") || undefined, address: val("address") || undefined, ...q }; break;
        case "contact": path = "/api/sendContact"; body = { ...body, chatId: val("chatId"),
          contacts: [{ name: val("cname"), phone: val("cphone") || undefined }], ...q }; break;
        case "reaction": path = "/api/reaction"; body = { ...body, chatId: val("chatId"), messageId: val("messageId"),
          emoji: val("emoji") || "", fromMe: val("fromMe"), senderId: val("senderId") || undefined }; break;
        case "deleteMessage": path = "/api/deleteMessage"; body = { ...body, chatId: val("chatId"), messageId: val("messageId"),
          fromMe: val("fromMe"), senderId: val("senderId") || undefined }; break;
        case "editMessage": path = "/api/editMessage"; body = { ...body, chatId: val("chatId"), messageId: val("messageId"),
          text: val("text"), fromMe: val("fromMe") }; break;
        case "sendSeen": path = "/api/sendSeen"; body = { ...body, chatId: val("chatId"), messageId: val("messageId"),
          fromMe: val("fromMe"), senderId: val("senderId") || undefined }; break;
        case "presence": path = "/api/presence"; body = { ...body, chatId: val("chatId"), state: val("state") }; break;
      }
      ev.target.disabled = true;
      clear(out);
      try { const r = await api("POST", path, body); out.append(jsonOut(r)); ok("enviado"); }
      catch (e) { out.append(jsonOut(e.data || e.message)); fail(e); }
      finally { ev.target.disabled = false; }
    }
    function mediaBody() {
      return {
        chatId: val("chatId"),
        data: val("data"),
        mimetype: val("mimetype") || undefined,
        filename: val("filename") || undefined,
        caption: val("caption") || undefined,
        voice: val("voice") || undefined,
      };
    }
  },
};
function textField(id, label, ph) { return h("div", { class: "field" }, h("label", {}, label || id), h("input", { id: "f-" + id, placeholder: ph || "" })); }
function areaField(id, label) { return h("div", { class: "field" }, h("label", {}, label || id), h("textarea", { id: "f-" + id })); }
function numField(id, label) { return h("div", { class: "field" }, h("label", {}, label || id), h("input", { id: "f-" + id, type: "number", step: "any" })); }
function checkField(id, label) { return h("div", { class: "field" }, h("label", { class: "check" }, h("input", { type: "checkbox", id: "f-" + id }), label || id)); }
function selField(id, opts) { return h("div", { class: "field" }, h("label", {}, id), h("select", { id: "f-" + id }, opts.map((o) => h("option", { value: o }, o)))); }
function rowFields(fs) { return h("div", { class: "row" }, fs); }
function mediaFields(kind, isAudio) {
  return [
    textField("chatId", "chatId", "55...@s.whatsapp.net"),
    areaField("data", "data (base64 ou data URI)"),
    rowFields([textField("mimetype", "mimetype (opc)"), kind === "file" ? textField("filename", "filename (opc)") : h("div", { class: "field" })]),
    kind === "file" || kind === "image" || kind === "video" ? textField("caption", "caption (opc)") : h("div"),
    isAudio ? checkField("voice", "nota de voz (PTT)") : h("div"),
  ];
}

/* ---- groups ---- */
views.groups = {
  title: "Grupos",
  async render(root) {
    const names = await sessionNames();
    const sel = sessionSelect("g-sess", names);
    const listBox = h("div", {});
    const load = async () => {
      clear(listBox);
      const s = sel.value; if (!s) return;
      listBox.append(h("div", { class: "muted" }, "carregando…"));
      try {
        const gs = await api("GET", "/api/groups?session=" + encodeURIComponent(s));
        clear(listBox);
        listBox.append(table([
          { h: "nome", get: (g) => h("b", {}, g.name || "—") },
          { h: "jid", get: (g) => h("span", { class: "mono" }, g.jid) },
          { h: "membros", get: (g) => (g.participants || []).length || g.participantCount || "—" },
          { h: "flags", get: (g) => [g.announce ? "announce " : "", g.locked ? "locked" : ""].join("") || "—" },
          { h: "", get: (g) => h("button", { class: "btn ghost sm", onclick: () => groupModal(s, g) }, "abrir") },
        ], gs, { empty: "nenhum grupo" }));
      } catch (e) { clear(listBox); listBox.append(h("div", { class: "empty" }, e.message)); }
    };
    sel.addEventListener("change", load);

    root.append(h("div", { class: "panel" },
      h("div", { class: "row" },
        h("div", { class: "field" }, h("label", {}, "sessão"), sel),
        h("div", { class: "field", style: "flex:0 0 auto" }, h("label", {}, " "),
          h("button", { class: "btn ghost", onclick: load }, "Listar")),
      ),
    ));

    root.append(h("h2", { class: "section" }, "Criar grupo"),
      h("div", { class: "panel" },
        h("div", { class: "field" }, h("label", {}, "nome"), h("input", { id: "g-name" })),
        h("div", { class: "field" }, h("label", {}, "participantes (um por linha, 55...@s.whatsapp.net)"), h("textarea", { id: "g-parts" })),
        h("div", { class: "btn-row" }, h("button", { class: "btn", onclick: async (e) => {
          const s = sel.value; if (!s) return fail("sessão");
          const participants = $("#g-parts").value.split("\n").map((x) => x.trim()).filter(Boolean);
          e.target.disabled = true;
          try { const r = await api("POST", "/api/groups", { session: s, name: $("#g-name").value, participants }); ok("grupo criado: " + r.jid); load(); }
          catch (err) { fail(err); } finally { e.target.disabled = false; }
        } }, "Criar")),
      ),
      h("h2", { class: "section" }, "Entrar por link"),
      h("div", { class: "panel" },
        h("div", { class: "row" },
          h("div", { class: "field" }, h("label", {}, "código do convite"), h("input", { id: "g-code", placeholder: "ABCdef..." })),
          h("div", { class: "field", style: "flex:0 0 auto" }, h("label", {}, " "),
            h("button", { class: "btn", onclick: async (e) => {
              const s = sel.value; if (!s) return fail("sessão");
              e.target.disabled = true;
              try { const r = await api("POST", "/api/groups/join", { session: s, code: $("#g-code").value.trim() }); ok("entrou: " + r.jid); load(); }
              catch (err) { fail(err); } finally { e.target.disabled = false; }
            } }, "Entrar")),
        ),
      ),
      h("div", { style: "height:16px" }), listBox,
    );
    if (sel.value) load();
  },
};
function groupModal(session, g) {
  const parts = table([
    { h: "jid", get: (p) => h("span", { class: "mono" }, p.jid) },
    { h: "admin", get: (p) => p.isSuperAdmin ? "owner" : (p.isAdmin ? "admin" : "") },
  ], g.participants || [], { empty: "sem participantes na resposta" });

  const partInput = h("textarea", { placeholder: "um jid por linha" });
  const doParts = (action) => async (e) => {
    const participants = partInput.value.split("\n").map((x) => x.trim()).filter(Boolean);
    if (!participants.length) return fail("informe participantes");
    e.target.disabled = true;
    try { await api("POST", `/api/groups/${encodeURIComponent(g.jid)}/participants`, { session, action, participants }); ok(action + " ok"); }
    catch (err) { fail(err); } finally { e.target.disabled = false; }
  };

  modal("Grupo — " + (g.name || g.jid), h("div", {},
    h("div", { class: "muted mono", style: "margin-bottom:8px" }, g.jid),
    h("div", {}, h("b", {}, "tópico: "), g.topic || "—"),
    h("h2", { class: "section" }, "Participantes"), parts,
    h("h2", { class: "section" }, "Gerenciar participantes"), partInput,
    h("div", { class: "btn-row" },
      h("button", { class: "btn sm", onclick: doParts("add") }, "add"),
      h("button", { class: "btn ghost sm", onclick: doParts("remove") }, "remove"),
      h("button", { class: "btn ghost sm", onclick: doParts("promote") }, "promote"),
      h("button", { class: "btn ghost sm", onclick: doParts("demote") }, "demote"),
    ),
    h("h2", { class: "section" }, "Metadados"),
    h("div", { class: "row" },
      h("div", { class: "field" }, h("label", {}, "novo nome"), h("input", { id: "gm-name", value: g.name || "" })),
      h("div", { class: "field", style: "flex:0 0 auto" }, h("label", {}, " "),
        h("button", { class: "btn sm", onclick: async (e) => {
          try { await api("PUT", `/api/groups/${encodeURIComponent(g.jid)}/name`, { session, name: $("#gm-name").value }); ok("nome ok"); } catch (err) { fail(err); }
        } }, "salvar")),
    ),
    h("div", { class: "row" },
      h("div", { class: "field" }, h("label", {}, "novo tópico"), h("input", { id: "gm-topic", value: g.topic || "" })),
      h("div", { class: "field", style: "flex:0 0 auto" }, h("label", {}, " "),
        h("button", { class: "btn sm", onclick: async (e) => {
          try { await api("PUT", `/api/groups/${encodeURIComponent(g.jid)}/topic`, { session, topic: $("#gm-topic").value }); ok("tópico ok"); } catch (err) { fail(err); }
        } }, "salvar")),
    ),
    h("div", { class: "btn-row" },
      h("button", { class: "btn ghost sm", onclick: async () => {
        try { const r = await api("GET", `/api/groups/${encodeURIComponent(g.jid)}/invite-link?session=${encodeURIComponent(session)}`); prompt("link do convite:", r.link); } catch (e) { fail(e); }
      } }, "invite link"),
      h("button", { class: "btn danger sm", onclick: async () => {
        if (!confirm("sair do grupo?")) return;
        try { await api("POST", `/api/groups/${encodeURIComponent(g.jid)}/leave`, { session }); ok("saiu"); clear($("#modal-root")); route(); } catch (e) { fail(e); }
      } }, "sair"),
    ),
  ));
}

/* ---- contatos ---- */
views.contacts = {
  title: "Contatos",
  async render(root) {
    const names = await sessionNames();
    const sel = sessionSelect("c-sess", names);
    const out = h("div", {});
    const run = (fn) => async (e) => { e.target.disabled = true; clear(out); try { out.append(jsonOut(await fn())); } catch (err) { out.append(jsonOut(err.data || err.message)); fail(err); } finally { e.target.disabled = false; } };

    root.append(h("div", { class: "panel" },
      h("div", { class: "field" }, h("label", {}, "sessão"), sel),
      h("h2", { class: "section" }, "Checar número (está no WhatsApp?)"),
      h("div", { class: "row" },
        h("div", { class: "field" }, h("label", {}, "telefones (vírgula)"), h("input", { id: "c-phone", placeholder: "+5511999999999,+55..." })),
        h("div", { class: "field", style: "flex:0 0 auto" }, h("label", {}, " "),
          h("button", { class: "btn", onclick: run(() => api("GET", `/api/contacts/check?session=${encodeURIComponent(sel.value)}&phone=${encodeURIComponent($("#c-phone").value)}`)) }, "Checar")),
      ),
      h("h2", { class: "section" }, "Info de perfil"),
      h("div", { class: "row" },
        h("div", { class: "field" }, h("label", {}, "jids (vírgula)"), h("input", { id: "c-jid", placeholder: "55...@s.whatsapp.net" })),
        h("div", { class: "field", style: "flex:0 0 auto" }, h("label", {}, " "),
          h("button", { class: "btn ghost", onclick: run(() => api("GET", `/api/contacts/info?session=${encodeURIComponent(sel.value)}&jid=${encodeURIComponent($("#c-jid").value)}`)) }, "Buscar")),
      ),
      h("h2", { class: "section" }, "Foto de perfil"),
      h("div", { class: "row" },
        h("div", { class: "field" }, h("label", {}, "jid"), h("input", { id: "c-pic" })),
        h("div", { class: "field", style: "flex:0 0 auto" }, h("label", {}, " "),
          h("button", { class: "btn ghost", onclick: run(() => api("GET", `/api/contacts/profile-picture?session=${encodeURIComponent(sel.value)}&jid=${encodeURIComponent($("#c-pic").value)}`)) }, "URL")),
      ),
      out,
    ));
  },
};

/* ---- outbox ---- */
views.outbox = {
  title: "Fila de saída",
  async render(root) {
    await listView(root, {
      path: (s, l) => `/api/outbox?session=${encodeURIComponent(s)}&limit=${l}`,
      cols: [
        { h: "criado", get: (j) => fmtTime(j.createdAt), cls: "muted" },
        { h: "kind", get: (j) => j.kind },
        { h: "status", get: (j) => badge(j.status) },
        { h: "runAt", get: (j) => fmtTime(j.runAt), cls: "muted" },
        { h: "tent.", get: (j) => j.attempts },
        { h: "messageId", get: (j) => h("span", { class: "mono" }, short(j.messageId, 20)) },
        { h: "erro", get: (j) => h("span", { class: "muted wrap-anywhere" }, j.lastError || "") },
      ],
    });
  },
};

/* ---- deliveries ---- */
views.deliveries = {
  title: "Webhooks entregues",
  async render(root) {
    await listView(root, {
      path: (s, l) => `/api/deliveries?session=${encodeURIComponent(s)}&limit=${l}`,
      cols: [
        { h: "criado", get: (d) => fmtTime(d.createdAt), cls: "muted" },
        { h: "evento", get: (d) => d.event },
        { h: "status", get: (d) => badge(d.status) },
        { h: "cód.", get: (d) => d.responseCode || "" },
        { h: "tent.", get: (d) => d.attempts },
        { h: "url", get: (d) => h("span", { class: "mono wrap-anywhere" }, short(d.url, 40)) },
        { h: "erro", get: (d) => h("span", { class: "muted wrap-anywhere" }, d.lastError || "") },
      ],
    });
  },
};

async function listView(root, opts) {
  const names = await sessionNames();
  const sel = sessionSelect("lv-sess", names);
  const box = h("div", {});
  const auto = h("input", { type: "checkbox", id: "lv-auto" });
  let timer = null;
  const load = async () => {
    const s = sel.value; if (!s) { clear(box); return; }
    try {
      const rows = await api("GET", opts.path(s, 100));
      clear(box); box.append(table(opts.cols, rows, { empty: "nada ainda" }));
    } catch (e) { clear(box); box.append(h("div", { class: "empty" }, e.message)); }
  };
  sel.addEventListener("change", load);
  auto.addEventListener("change", () => {
    clearInterval(timer);
    if (auto.checked) timer = setInterval(load, 4000);
  });
  window.addEventListener("hashchange", () => clearInterval(timer), { once: true });

  root.append(h("div", { class: "panel" },
    h("div", { class: "row" },
      h("div", { class: "field" }, h("label", {}, "sessão"), sel),
      h("div", { class: "field", style: "flex:0 0 auto" }, h("label", {}, " "), h("button", { class: "btn ghost", onclick: load }, "Carregar")),
      h("div", { class: "field", style: "flex:0 0 auto" }, h("label", {}, " "), h("label", { class: "check" }, auto, "auto 4s")),
    ),
  ), box);
  if (sel.value) load();
}

/* ---- events (websocket) ---- */
views.events = {
  title: "Eventos ao vivo",
  async render(root) {
    const names = await sessionNames();
    const sel = sessionSelect("e-sess", names);
    const evInput = h("input", { id: "e-filter", value: "*", style: "max-width:220px" });
    const log = h("div", { class: "log" });
    const statusPill = h("span", { class: "pill" }, "desligado");
    let sock = null;

    const stop = () => { if (sock) { sock.close(); sock = null; } statusPill.textContent = "desligado"; statusPill.className = "pill"; };
    const start = () => {
      stop();
      const s = sel.value; if (!s) return fail("sessão");
      const wsBase = LS.base.replace(/^http/, "ws");
      const url = `${wsBase}/ws?session=${encodeURIComponent(s)}&events=${encodeURIComponent(evInput.value || "*")}&api_key=${encodeURIComponent(LS.key)}`;
      sock = new WebSocket(url);
      statusPill.textContent = "conectando…";
      sock.onopen = () => { statusPill.textContent = "ao vivo"; statusPill.className = "pill ok"; };
      sock.onclose = () => { statusPill.textContent = "fechado"; statusPill.className = "pill bad"; };
      sock.onerror = () => { statusPill.textContent = "erro"; statusPill.className = "pill bad"; };
      sock.onmessage = (m) => {
        let obj; try { obj = JSON.parse(m.data); } catch { obj = { raw: m.data }; }
        const line = h("div", { class: "line" },
          h("span", { class: "ts" }, new Date().toLocaleTimeString() + " "),
          h("span", { class: "ev" }, obj.event || obj.name || "?"),
          " ",
          h("span", {}, JSON.stringify(obj.payload ?? obj)),
        );
        log.prepend(line);
        while (log.childElementCount > 400) log.lastChild.remove();
      };
    };
    window.addEventListener("hashchange", stop, { once: true });

    root.append(h("div", { class: "panel" },
      h("div", { class: "row" },
        h("div", { class: "field" }, h("label", {}, "sessão"), sel),
        h("div", { class: "field" }, h("label", {}, "eventos (wildcard)"), evInput),
        h("div", { class: "field", style: "flex:0 0 auto" }, h("label", {}, " "),
          h("div", { class: "actions" },
            h("button", { class: "btn", onclick: start }, "Conectar"),
            h("button", { class: "btn ghost", onclick: stop }, "Parar"),
            h("button", { class: "btn ghost", onclick: () => clear(log) }, "Limpar"),
            statusPill,
          )),
      ),
    ), log);
  },
};

/* ---- keys ---- */
views.keys = {
  title: "API keys",
  async render(root) {
    let list;
    try { list = await api("GET", "/api/keys"); }
    catch (e) { root.append(h("div", { class: "empty" }, e.status === 403 ? "requer uma chave com escopo * (a master serve)" : e.message)); return; }

    root.append(
      table([
        { h: "id", get: (k) => h("span", { class: "mono" }, k.id) },
        { h: "label", get: (k) => k.label || "—" },
        { h: "escopos", get: (k) => (k.scopes || []).join(", ") },
        { h: "criada", get: (k) => fmtTime(k.createdAt), cls: "muted" },
        { h: "último uso", get: (k) => fmtTime(k.lastUsedAt), cls: "muted" },
        { h: "estado", get: (k) => k.revokedAt ? badge("failed") : badge("ok") },
        { h: "", get: (k) => k.revokedAt ? "" : h("button", { class: "btn danger sm", onclick: async () => {
          if (!confirm("revogar " + k.id + "?")) return;
          try { await api("DELETE", "/api/keys/" + encodeURIComponent(k.id)); ok("revogada"); route(); } catch (e) { fail(e); }
        } }, "revogar") },
      ], list, { empty: "nenhuma chave na tabela (a master de env não aparece aqui)" }),
      h("h2", { class: "section" }, "Nova chave"),
      h("div", { class: "panel" },
        h("div", { class: "row" },
          h("div", { class: "field" }, h("label", {}, "label"), h("input", { id: "nk-label", placeholder: "app-x" })),
          h("div", { class: "field" }, h("label", {}, "escopos (vírgula, vazio = *)"), h("input", { id: "nk-scopes", placeholder: "*" })),
        ),
        h("div", { class: "btn-row" }, h("button", { class: "btn", onclick: async (e) => {
          const scopes = $("#nk-scopes").value.split(",").map((x) => x.trim()).filter(Boolean);
          e.target.disabled = true;
          try {
            const r = await api("POST", "/api/keys", { label: $("#nk-label").value, scopes });
            modal("Chave criada", h("div", {},
              h("p", { class: "muted" }, "copie agora — o token não é exibido de novo:"),
              h("pre", { class: "out" }, r.token),
            ));
            route();
          } catch (err) { fail(err); } finally { e.target.disabled = false; }
        } }, "Gerar")),
      ),
    );
  },
};

/* ---- settings ---- */
views.settings = {
  title: "Conexão",
  async render(root) {
    const base = h("input", { id: "st-base", value: LS.base });
    const key = h("input", { id: "st-key", value: LS.key, type: "password", placeholder: "X-Api-Key" });
    const out = h("div", {});
    root.append(h("div", { class: "panel" },
      h("div", { class: "field" }, h("label", {}, "base URL do gateway"), base),
      h("div", { class: "field" }, h("label", {}, "API key"), key),
      h("div", { class: "btn-row" },
        h("button", { class: "btn", onclick: () => { LS.base = base.value.replace(/\/$/, ""); LS.key = key.value; ok("salvo"); renderConn(); healthPoll(); } }, "Salvar"),
        h("button", { class: "btn ghost", onclick: async () => {
          clear(out);
          try { const v = await api("GET", "/api/version"); out.append(jsonOut(v)); ok("conectou"); }
          catch (e) { out.append(jsonOut(e.message)); fail(e); }
        } }, "Testar"),
      ),
      out,
      h("p", { class: "muted", style: "margin-top:14px;font-size:12px" },
        "As credenciais ficam só no localStorage deste navegador. A master key do .env também funciona aqui."),
    ));
  },
};

/* ------------------------------------------------------------------ shell */

const NAV = [
  ["dashboard", "▚", "Dashboard"],
  ["sessions", "◉", "Sessões"],
  ["console", "➤", "Console"],
  ["groups", "◈", "Grupos"],
  ["contacts", "☺", "Contatos"],
  ["outbox", "⧗", "Fila de saída"],
  ["deliveries", "✉", "Webhooks"],
  ["events", "⚡", "Eventos"],
  ["keys", "⚿", "API keys"],
  ["settings", "⚙", "Conexão"],
];

function renderNav(active) {
  const nav = $("#nav");
  clear(nav);
  NAV.forEach(([id, ico, label]) => {
    nav.append(h("a", { href: "#/" + id, class: id === active ? "active" : "" },
      h("span", { class: "ico" }, ico), label));
  });
}
function renderConn() {
  $("#conn").innerHTML = "";
  $("#conn").append(
    h("div", {}, h("b", {}, "base "), short(LS.base.replace(/^https?:\/\//, ""), 22)),
    h("div", {}, h("b", {}, "key "), LS.key ? short(LS.key, 10) : h("span", { class: "muted" }, "— não definida")),
  );
}

async function healthPoll() {
  const pill = $("#health");
  try {
    const hh = await api("GET", "/health");
    pill.textContent = hh.status === "ok" ? "online" : hh.status;
    pill.className = "pill " + (hh.status === "ok" ? "ok" : "bad");
  } catch {
    pill.textContent = "offline";
    pill.className = "pill bad";
  }
}

let currentView = null;
async function route() {
  const hash = location.hash.replace(/^#\/?/, "") || "dashboard";
  const name = views[hash] ? hash : "dashboard";
  currentView = name;
  renderNav(name);
  $("#crumb").textContent = views[name].title;
  const view = $("#view");
  clear(view);
  const wrap = h("div", {});
  view.append(wrap);
  try { await views[name].render(wrap); }
  catch (e) { clear(wrap); wrap.append(h("div", { class: "empty" }, "erro na view: " + e.message)); }
}

$("#refresh").addEventListener("click", route);
window.addEventListener("hashchange", route);

renderConn();
route();
healthPoll();
setInterval(healthPoll, 15000);

if (!LS.key) location.hash = "#/settings";
