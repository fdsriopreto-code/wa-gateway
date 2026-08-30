"use strict";

/* ═══════════════════════════════════════════════ state */
const LS = {
  get base(){return localStorage.getItem("wa.base")||location.origin;},
  set base(v){localStorage.setItem("wa.base",v.replace(/\/+$/,""));},
  get key(){return localStorage.getItem("wa.key")||"";},
  set key(v){localStorage.setItem("wa.key",v);},
  get theme(){return localStorage.getItem("wa.theme")||"dark";},
  set theme(v){localStorage.setItem("wa.theme",v);},
  get sess(){return localStorage.getItem("wa.sess")||"";},
  set sess(v){localStorage.setItem("wa.sess",v);},
  get chat(){try{return JSON.parse(localStorage.getItem("wa.chat")||"{}");}catch{return{};}},
  set chat(v){localStorage.setItem("wa.chat",JSON.stringify(v));},
};

/* ═══════════════════════════════════════════════ icons (feather-ish) */
const ICONS = {
  overview:'<rect x="3" y="3" width="7" height="9" rx="1"/><rect x="14" y="3" width="7" height="5" rx="1"/><rect x="14" y="12" width="7" height="9" rx="1"/><rect x="3" y="16" width="7" height="5" rx="1"/>',
  sessions:'<rect x="5" y="2" width="14" height="20" rx="2"/><path d="M12 18h.01"/>',
  playground:'<path d="m4 17 6-6-6-6M12 19h8"/>',
  chat:'<path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/>',
  events:'<path d="M13 2 3 14h9l-1 8 10-12h-9l1-8z"/>',
  monitor:'<path d="M18 20V10M12 20V4M6 20v-6"/>',
  keys:'<circle cx="7.5" cy="15.5" r="5.5"/><path d="m21 2-9.6 9.6M15.5 7.5l3 3L22 7l-3-3"/>',
  settings:'<path d="M4 21v-7M4 10V3M12 21v-9M12 8V3M20 21v-5M20 12V3M1 14h6M9 8h6M17 16h6"/>',
  search:'<circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/>',
  copy:'<rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/>',
  check:'<path d="M20 6 9 17l-5-5"/>',
  refresh:'<path d="M21 12a9 9 0 1 1-3-6.7L21 8"/><path d="M21 3v5h-5"/>',
  redo:'<path d="M3 12a9 9 0 1 0 3-6.7L3 8"/><path d="M3 3v5h5"/>',
  x:'<path d="M18 6 6 18M6 6l12 12"/>',
  send:'<path d="M22 2 11 13"/><path d="M22 2 15 22l-4-9-9-4 20-7z"/>',
  play:'<path d="m5 3 14 9-14 9V3z"/>',
  stop:'<rect x="5" y="5" width="14" height="14" rx="2"/>',
  trash:'<path d="M3 6h18M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2m3 0v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6"/>',
  plus:'<path d="M12 5v14M5 12h14"/>',
  chev:'<path d="m9 18 6-6-6-6"/>',
  menu:'<path d="M3 12h18M3 6h18M3 18h18"/>',
  sun:'<circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/>',
  moon:'<path d="M12 3a6 6 0 0 0 9 9 9 9 0 1 1-9-9z"/>',
  alert:'<path d="M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0zM12 9v4M12 17h.01"/>',
  link:'<path d="M9 17H7A5 5 0 0 1 7 7h2M15 7h2a5 5 0 0 1 0 10h-2M8 12h8"/>',
  inbox:'<path d="M22 12h-6l-2 3h-4l-2-3H2M5.5 5.1 2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.5-6.9A2 2 0 0 0 16.8 4H7.2a2 2 0 0 0-1.8 1.1z"/>',
  clock:'<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>',
  circle:'<circle cx="12" cy="12" r="9"/>',
  qr:'<rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><path d="M14 14h3v3M20 14v.01M14 20v.01M20 20v.01M17 17v.01"/>',
  cmd:'<path d="M15 6a3 3 0 1 0 3 3h-3zM9 6a3 3 0 1 1-3 3h3zM15 18a3 3 0 1 1 3-3h-3zM9 18a3 3 0 1 0-3-3h3z"/>',
  wifi:'<path d="M5 12.5a11 11 0 0 1 14 0M8.5 16a6 6 0 0 1 7 0M12 20h.01"/>',
};
function ic(name, cls){
  const w=document.createElement("span"); w.style.display="contents";
  w.innerHTML=`<svg class="ico ${cls||""}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">${ICONS[name]||""}</svg>`;
  return w.firstChild;
}

/* ═══════════════════════════════════════════════ dom */
function h(tag, attrs, ...kids){
  const e=document.createElement(tag);
  for(const [k,v] of Object.entries(attrs||{})){
    if(v==null||v===false) continue;
    if(k==="class") e.className=v;
    else if(k==="html") e.innerHTML=v;
    else if(k==="style") e.style.cssText=v;
    else if(k.startsWith("on")&&typeof v==="function") e.addEventListener(k.slice(2),v);
    else if(v===true) e.setAttribute(k,"");
    else e.setAttribute(k,v);
  }
  for(const kid of kids.flat()){
    if(kid==null||kid===false) continue;
    e.append(kid.nodeType?kid:document.createTextNode(String(kid)));
  }
  return e;
}
const $=(s,r=document)=>r.querySelector(s);
const clear=(n)=>{while(n&&n.firstChild)n.removeChild(n.firstChild);};
const esc=(s)=>String(s).replace(/[&<>"]/g,c=>({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;"}[c]));
const fmtTime=(s)=>{if(!s)return"—";const d=new Date(typeof s==="number"?s*1000:s);return isNaN(d)?"—":d.toLocaleString();};
const fmtClock=(s)=>{const d=new Date(typeof s==="number"?s*1000:s);return isNaN(d)?"":d.toLocaleTimeString([], {hour:"2-digit",minute:"2-digit"});};
const fmtRel=(s)=>{if(!s)return"";const d=new Date(typeof s==="number"?s*1000:s),df=(Date.now()-d)/1000;
  if(Math.abs(df)<60)return"agora";if(df<3600)return Math.floor(df/60)+"min";if(df<86400)return Math.floor(df/3600)+"h";return Math.floor(df/86400)+"d";};
const trunc=(s,n=18)=>!s?"":(s.length>n?s.slice(0,n)+"…":s);
const nfmt=(n)=>typeof n==="number"?n.toLocaleString("pt-BR"):n;

/* ═══════════════════════════════════════════════ api */
async function api(method, path, body){
  const t0=performance.now();
  const res=await fetch(LS.base+path,{
    method,
    headers:{"X-Api-Key":LS.key,...(body!==undefined?{"Content-Type":"application/json"}:{})},
    body:body!==undefined?JSON.stringify(body):undefined,
  });
  const ms=Math.round(performance.now()-t0);
  const text=await res.text();
  let data=null; try{data=text?JSON.parse(text):null;}catch{data=text;}
  if(!res.ok){
    const e=new Error((data&&(data.message||data.error))||res.statusText||("HTTP "+res.status));
    e.status=res.status; e.data=data; e.ms=ms; throw e;
  }
  return {data,ms,status:res.status};
}
const apiData=async(m,p,b)=>(await api(m,p,b)).data;

/* ═══════════════════════════════════════════════ ui bits */
function toast(msg, kind="", title){
  const root=$("#toast-root");
  while(root.childElementCount>=4) root.firstChild.remove();
  const t=h("div",{class:"toast "+kind,onclick:()=>dismiss()},
    h("span",{class:"ic"},ic(kind==="ok"?"check":kind==="err"?"alert":"circle","sm")),
    h("div",{}, title?h("b",{},title):null, msg),
    h("span",{class:"bar"}));
  root.append(t);
  let done=false;
  const dismiss=()=>{if(done)return;done=true;t.classList.add("out");setTimeout(()=>t.remove(),200);};
  setTimeout(dismiss,4200);
}
const ok=(m)=>toast(m,"ok");
const fail=(e)=>toast(typeof e==="string"?e:(e.message||"erro"),"err",typeof e==="object"&&e.status?"HTTP "+e.status:null);

let modalEscHandler=null;
function modal(title, sub, node, opts={}){
  if(modalEscHandler){removeEventListener("keydown",modalEscHandler);modalEscHandler=null;}
  const root=$("#modal-root"), close=()=>{clear(root);if(modalEscHandler){removeEventListener("keydown",modalEscHandler);modalEscHandler=null;}};
  const box=h("div",{class:"modal",style:opts.wide?"width:min(620px,95vw)":""},
    h("div",{class:"between",style:"margin-bottom:5px"},
      h("h3",{},title),
      h("button",{class:"btn icon ghost sm",onclick:close},ic("x","sm"))),
    sub?h("div",{class:"msub"},sub):null,
    node);
  const bg=h("div",{class:"modal-bg",onclick:e=>{if(e.target===bg)close();}},box);
  clear(root); root.append(bg);
  modalEscHandler=(e)=>{if(e.key==="Escape")close();};
  addEventListener("keydown",modalEscHandler);
  const f=box.querySelector("input,textarea,select"); if(f&&!opts.noFocus)setTimeout(()=>f.focus(),40);
  return {close,box};
}

function hl(v,ind=0){
  const p="  ".repeat(ind),p1="  ".repeat(ind+1);
  if(v===null)return'<span class="j-null">null</span>';
  if(typeof v==="string")return'<span class="j-str">"'+esc(v)+'"</span>';
  if(typeof v==="number"||typeof v==="boolean")return`<span class="j-${typeof v==="number"?"num":"bool"}">${v}</span>`;
  if(Array.isArray(v))return v.length?"[\n"+v.map(x=>p1+hl(x,ind+1)).join(",\n")+"\n"+p+"]":"[]";
  const ks=Object.keys(v);
  return ks.length?"{\n"+ks.map(k=>p1+`<span class="j-key">"${esc(k)}"</span>: `+hl(v[k],ind+1)).join(",\n")+"\n"+p+"}":"{}";
}
function copyBtn(label, text){
  const s=h("span",{onclick:async()=>{await navigator.clipboard.writeText(text);clear(s);s.append(ic("check","sm"),"copiado");setTimeout(()=>{clear(s);s.append(ic("copy","sm"),label);},1200);}},ic("copy","sm"),label);
  return s;
}
function copyChip(label, value, shown){
  const c=h("button",{class:"cchip",title:"copiar: "+value,onclick:async()=>{
    try{await navigator.clipboard.writeText(value);}catch{}
    c.classList.add("done");const v=c.querySelector(".vl");const old=v.textContent;v.textContent="copiado ✓";
    setTimeout(()=>{c.classList.remove("done");v.textContent=old;},1100);
  }},label?h("span",{class:"lb"},label):null,h("span",{class:"vl"},shown||value));
  return c;
}
function jsonOut(data, extra){
  const raw=typeof data==="string"?data:JSON.stringify(data,null,2);
  const pre=h("pre",{class:"out"});
  pre.innerHTML=typeof data==="string"?esc(data):hl(data);
  return h("div",{}, h("div",{class:"out-bar"}, extra||null, copyBtn("copiar",raw)), pre);
}
const spinner=()=>h("span",{class:"spinner"});
const loadingBox=()=>h("div",{class:"loading"},spinner());
function skeleton(kind,n=4){
  const cls=kind==="card"?"skel-card":kind==="kpi"?"skel-kpi":"skel-row";
  if(kind==="kpi"){const g=h("div",{class:"kgrid"});for(let i=0;i<n;i++)g.append(h("div",{class:"skel "+cls}));return g;}
  const w=h("div",{});
  for(let i=0;i<n;i++) w.append(h("div",{class:"skel "+cls,style:kind==="card"?"margin-bottom:13px":""}));
  return w;
}
const empty=(iconName,txt,action,head)=>h("div",{class:"empty"},
  h("div",{class:"eic"},ic(iconName,"lg")),
  h("h4",{},head||txt),
  head?h("p",{},txt):null,
  action||null);
const badge=(t)=>h("span",{class:"badge s-"+String(t||"?").toLowerCase()},h("span",{class:"dot"}),String(t||"?"));

function table(cols,rows,emptyMsg){
  if(!rows||!rows.length)return empty("inbox",emptyMsg||"nada aqui");
  return h("div",{class:"tbl-wrap"},h("div",{class:"tbl-scroll"},h("table",{},
    h("thead",{},h("tr",{},cols.map(c=>h("th",{},c.h)))),
    h("tbody",{},rows.map(r=>h("tr",{},cols.map(c=>{
      const val=c.get(r);
      return h("td",{class:c.cls||""},val&&val.nodeType?val:(val==null?"":String(val)));
    })))))));
}
function kv(k,v){
  return h("div",{class:"kv"},h("span",{class:"k"},k),h("span",{},v&&v.nodeType?v:String(v)));
}

/* session cache */
let SESSIONS=[];
async function loadSessions(){try{SESSIONS=(await apiData("GET","/api/sessions"))||[];}catch{SESSIONS=[];}return SESSIONS;}
function sessionSelect(id,cur){
  cur=cur||LS.sess;
  return h("select",{id,onchange:e=>{LS.sess=e.target.value;}},SESSIONS.length
    ?SESSIONS.map(s=>h("option",{value:s.name,selected:s.name===cur||undefined},`${s.name} · ${s.status}`))
    :h("option",{value:""},"— sem sessões —"));
}

/* ═══════════════════════════════════════════════ event catalog (webhook editor) */
const EVENT_CATALOG=[
  {g:"Sessão",items:[
    ["session.status","conexão muda (WORKING / QR / …)"],
    ["session.qr","novo QR code"],
    ["session.pair","pareado com sucesso"],
    ["session.logged_out","deslogado"],
  ]},
  {g:"Mensagens",items:[
    ["message","recebidas (não enviadas por mim)"],
    ["message.any","todas, inclusive as que eu enviei"],
    ["message.ack","recibos (entregue / lida / ouvida)"],
    ["message.reaction","reações"],
    ["message.revoked","apagadas"],
    ["message.edited","editadas"],
  ]},
  {g:"Grupos",items:[
    ["group.update","metadados do grupo mudaram"],
    ["group.join","entrei num grupo"],
  ]},
  {g:"Contatos & presença",items:[
    ["presence.update","online / offline / visto por último"],
    ["chat.presence","digitando / gravando"],
    ["contact.update","contato alterado"],
    ["contact.picture","foto de perfil"],
  ]},
  {g:"Chamadas",items:[
    ["call.received","chamada recebida"],
    ["call.terminated","chamada encerrada"],
  ]},
];
const WILDCARDS=[["*","tudo"],["message.*","mensagens"],["session.*","sessão"],["group.*","grupos"],["call.*","chamadas"]];

/* ═══════════════════════════════════════════════ ENDPOINT REGISTRY */
const GROUPS=["Sessões","Mensagens","Grupos","Contatos","Fila & monitor"];
const ENDPOINTS=[
  {g:"Sessões",m:"GET",path:"/api/sessions",title:"Listar sessões",fields:[]},
  {g:"Sessões",m:"POST",path:"/api/sessions",title:"Criar sessão",fields:[
    ["name","text",{req:1,ph:"default"}],["start","bool",{def:true}],
    ["config","json",{ph:'{"webhooks":[{"url":"https://…","events":["message"]}]}'}]]},
  {g:"Sessões",m:"GET",path:"/api/sessions/{session}",title:"Ver sessão",fields:[["session","session",{req:1}]]},
  {g:"Sessões",m:"PUT",path:"/api/sessions/{session}",title:"Atualizar config",fields:[
    ["session","session",{req:1}],["config","json",{req:1,ph:'{"outbox":{"minIntervalMs":8000}}'}]]},
  {g:"Sessões",m:"DELETE",path:"/api/sessions/{session}",title:"Apagar sessão",fields:[["session","session",{req:1}]]},
  ...["start","stop","restart","logout"].map(a=>({
    g:"Sessões",m:"POST",path:"/api/sessions/{session}/"+a,title:a[0].toUpperCase()+a.slice(1),
    fields:[["session","session",{req:1}]]})),
  {g:"Sessões",m:"GET",path:"/api/{session}/auth/qr",title:"QR (texto)",fields:[["session","session",{req:1}]]},
  {g:"Sessões",m:"POST",path:"/api/sessions/{session}/auth/pair-code",title:"Parear por código (sem QR)",fields:[
    ["session","session",{req:1}],["phone","text",{req:1,ph:"5517999999999 (só dígitos, internacional)"}]]},
  {g:"Sessões",m:"GET",path:"/api/{session}/me",title:"Meu perfil (JID, pushName…)",fields:[["session","session",{req:1}]]},
  {g:"Sessões",m:"PUT",path:"/api/{session}/profile/status",title:"Mudar meu recado",fields:[["session","session",{req:1}],["status","text",{req:1}]]},
  {g:"Sessões",m:"POST",path:"/api/{session}/presence",title:"Presença global (online/offline)",fields:[["session","session",{req:1}],["available","bool",{def:true}]]},
  {g:"Sessões",m:"GET",path:"/api/{session}/blocklist",title:"Lista de bloqueados",fields:[["session","session",{req:1}]]},
  {g:"Sessões",m:"POST",path:"/api/{session}/block",title:"Bloquear / desbloquear",fields:[
    ["session","session",{req:1}],["jid","text",{req:1,ph:"5517…@s.whatsapp.net"}],["block","bool",{def:true,hint:"desmarcado = desbloqueia"}]]},

  {g:"Mensagens",m:"POST",path:"/api/sendText",title:"Enviar texto",fields:[
    ["session","session",{req:1}],["chatId","text",{req:1,ph:"5517999999999@s.whatsapp.net"}],
    ["text","area",{req:1}],
    ["quotedId","text",{ph:"citar: id da mensagem"}],["quotedParticipant","text",{ph:"citar em grupo: jid do autor"}],
    ["mentions","lines",{ph:"mencionar: um número por linha"}],["linkPreview","bool",{hint:"anexa preview do 1º link"}],
    ["enqueue","bool",{hint:"fila com pacing anti-ban"}],["delay","text",{ph:"30s"}]]},
  {g:"Mensagens",m:"POST",path:"/api/sendSticker",title:"Enviar sticker (webp)",fields:[
    ["session","session",{req:1}],["chatId","text",{req:1}],["data","area",{req:1,ph:"base64 do .webp"}],
    ["enqueue","bool",{}],["delay","text",{ph:"30s"}]]},
  {g:"Mensagens",m:"POST",path:"/api/sendPoll",title:"Enviar enquete",fields:[
    ["session","session",{req:1}],["chatId","text",{req:1}],["name","text",{req:1,ph:"pergunta"}],
    ["options","lines",{req:1,ph:"uma opção por linha (mín. 2)"}],["selectable","num",{ph:"1"}],
    ["enqueue","bool",{}],["delay","text",{ph:"30s"}]]},
  ...[["sendImage","imagem"],["sendFile","documento"],["sendVideo","vídeo"],["sendAudio","áudio"]].map(([p,l])=>({
    g:"Mensagens",m:"POST",path:"/api/"+p,title:"Enviar "+l,fields:[
      ["session","session",{req:1}],["chatId","text",{req:1,ph:"…@s.whatsapp.net"}],
      ["data","area",{req:1,ph:"base64 ou data:URI"}],["mimetype","text",{}],
      p==="sendFile"?["filename","text",{}]:["caption","text",{}],
      p==="sendAudio"?["voice","bool",{hint:"nota de voz (PTT)"}]:null,
      ["enqueue","bool",{}],["delay","text",{ph:"30s"}]].filter(Boolean)})),
  {g:"Mensagens",m:"POST",path:"/api/sendLocation",title:"Enviar localização",fields:[
    ["session","session",{req:1}],["chatId","text",{req:1}],
    ["latitude","num",{req:1}],["longitude","num",{req:1}],["name","text",{}],["address","text",{}]]},
  {g:"Mensagens",m:"POST",path:"/api/sendContact",title:"Enviar contato",custom:"contact",fields:[
    ["session","session",{req:1}],["chatId","text",{req:1}],
    ["name","text",{req:1,ph:"nome do contato"}],["phone","text",{ph:"+55…"}]]},
  {g:"Mensagens",m:"POST",path:"/api/forwardMessage",title:"Encaminhar mensagem guardada",fields:[
    ["session","session",{req:1}],["toChatId","text",{req:1,ph:"destino …@s.whatsapp.net"}],["messageId","text",{req:1}],
    ["enqueue","bool",{}],["delay","text",{ph:"30s"}]]},
  {g:"Mensagens",m:"POST",path:"/api/reaction",title:"Reagir",fields:[
    ["session","session",{req:1}],["chatId","text",{req:1}],["messageId","text",{req:1}],
    ["emoji","text",{ph:'👍  ("" remove)'}],["fromMe","bool",{}],["senderId","text",{ph:"grupo: jid do autor"}]]},
  {g:"Mensagens",m:"POST",path:"/api/editMessage",title:"Editar mensagem",fields:[
    ["session","session",{req:1}],["chatId","text",{req:1}],["messageId","text",{req:1}],["text","area",{req:1}],["fromMe","bool",{def:true}]]},
  {g:"Mensagens",m:"POST",path:"/api/deleteMessage",title:"Apagar (revoke)",fields:[
    ["session","session",{req:1}],["chatId","text",{req:1}],["messageId","text",{req:1}],["fromMe","bool",{def:true}],["senderId","text",{}]]},
  {g:"Mensagens",m:"POST",path:"/api/sendSeen",title:"Marcar como lida",fields:[
    ["session","session",{req:1}],["chatId","text",{req:1}],["messageId","text",{req:1}],["fromMe","bool",{}],["senderId","text",{}]]},
  {g:"Mensagens",m:"POST",path:"/api/presence",title:"Presença (digitando…)",fields:[
    ["session","session",{req:1}],["chatId","text",{req:1}],["state","select",{opts:["typing","recording","paused"],req:1}]]},

  {g:"Grupos",m:"GET",path:"/api/groups",title:"Listar grupos",fields:[["session","session",{req:1}]]},
  {g:"Grupos",m:"POST",path:"/api/groups",title:"Criar grupo",fields:[
    ["session","session",{req:1}],["name","text",{req:1}],["participants","lines",{ph:"um jid por linha"}]]},
  {g:"Grupos",m:"POST",path:"/api/groups/join",title:"Entrar por link",fields:[
    ["session","session",{req:1}],["code","text",{req:1,ph:"código do convite"}]]},
  {g:"Grupos",m:"GET",path:"/api/groups/{jid}",title:"Info do grupo",fields:[["session","session",{req:1}],["jid","text",{req:1,ph:"…@g.us"}]]},
  {g:"Grupos",m:"POST",path:"/api/groups/{jid}/leave",title:"Sair do grupo",fields:[["session","session",{req:1}],["jid","text",{req:1}]]},
  {g:"Grupos",m:"POST",path:"/api/groups/{jid}/participants",title:"Participantes",fields:[
    ["session","session",{req:1}],["jid","text",{req:1}],
    ["action","select",{opts:["add","remove","promote","demote"],req:1}],["participants","lines",{req:1}]]},
  {g:"Grupos",m:"PUT",path:"/api/groups/{jid}/name",title:"Renomear grupo",fields:[["session","session",{req:1}],["jid","text",{req:1}],["name","text",{req:1}]]},
  {g:"Grupos",m:"PUT",path:"/api/groups/{jid}/topic",title:"Tópico do grupo",fields:[["session","session",{req:1}],["jid","text",{req:1}],["topic","text",{}]]},
  {g:"Grupos",m:"PUT",path:"/api/groups/{jid}/photo",title:"Foto do grupo",fields:[["session","session",{req:1}],["jid","text",{req:1}],["data","area",{req:1,ph:"base64 jpeg"}]]},
  {g:"Grupos",m:"PUT",path:"/api/groups/{jid}/announce",title:"Só admins enviam",fields:[["session","session",{req:1}],["jid","text",{req:1}],["enabled","bool",{def:true}]]},
  {g:"Grupos",m:"PUT",path:"/api/groups/{jid}/locked",title:"Só admins editam infos",fields:[["session","session",{req:1}],["jid","text",{req:1}],["enabled","bool",{def:true}]]},
  {g:"Grupos",m:"GET",path:"/api/groups/{jid}/invite-link",title:"Link de convite",fields:[["session","session",{req:1}],["jid","text",{req:1}],["reset","bool",{}]]},

  {g:"Contatos",m:"GET",path:"/api/contacts/check",title:"Número está no WhatsApp?",fields:[
    ["session","session",{req:1}],["phone","text",{req:1,ph:"+5517999999999,+55…"}]]},
  {g:"Contatos",m:"GET",path:"/api/contacts/info",title:"Info de perfil",fields:[
    ["session","session",{req:1}],["jid","text",{req:1,ph:"…@s.whatsapp.net"}]]},
  {g:"Contatos",m:"GET",path:"/api/contacts/profile-picture",title:"Foto de perfil",fields:[
    ["session","session",{req:1}],["jid","text",{req:1}],["preview","bool",{}]]},

  {g:"Fila & monitor",m:"GET",path:"/api/chats",title:"Listar conversas (histórico)",fields:[["session","session",{req:1}],["limit","num",{ph:"100"}]]},
  {g:"Fila & monitor",m:"GET",path:"/api/chats/{chatId}/messages",title:"Histórico de uma conversa",fields:[
    ["session","session",{req:1}],["chatId","text",{req:1,ph:"…@s.whatsapp.net"}],["limit","num",{ph:"50"}],["before","text",{ph:"RFC3339 (paginação)"}]]},
  {g:"Fila & monitor",m:"GET",path:"/api/messages/{id}/download",title:"Baixar mídia de mensagem guardada",fields:[["session","session",{req:1}],["id","text",{req:1,ph:"messageId"}]]},
  {g:"Fila & monitor",m:"GET",path:"/api/outbox",title:"Jobs da fila de saída",fields:[["session","session",{req:1}],["limit","num",{ph:"50"}]]},
  {g:"Fila & monitor",m:"GET",path:"/api/deliveries",title:"Entregas de webhook",fields:[["session","session",{req:1}],["limit","num",{ph:"100"}]]},
  {g:"Fila & monitor",m:"GET",path:"/api/stats",title:"Estatísticas",fields:[]},
  {g:"Fila & monitor",m:"GET",path:"/api/keys",title:"Listar API keys",fields:[]},
  {g:"Fila & monitor",m:"POST",path:"/api/keys",title:"Criar API key",fields:[["label","text",{}],["scopes","lines",{ph:"* (uma por linha)"}]]},
];

function fieldEl(f){
  const [k,t,o={}]=f;
  let input;
  if(t==="session"){
    input=h("select",{},SESSIONS.length
      ?SESSIONS.map(s=>h("option",{value:s.name,selected:s.name===LS.sess||undefined},`${s.name} · ${s.status}`))
      :h("option",{value:""},"— sem sessões —"));
  }else if(t==="bool"){
    input=h("input",{type:"checkbox",checked:o.def||undefined});
    return {k,t,get:()=>input.checked,node:h("div",{class:"field"},h("label",{class:"check"},input,k,o.hint?h("span",{class:"hint"}," — "+o.hint):null))};
  }else if(t==="select"){
    input=h("select",{},o.opts.map(x=>h("option",{value:x},x)));
  }else if(t==="area"||t==="json"||t==="lines"){
    input=h("textarea",{placeholder:o.ph||""});
  }else{
    input=h("input",{type:t==="num"?"number":"text",step:t==="num"?"any":undefined,placeholder:o.ph||""});
  }
  const get=()=>{
    const v=input.value;
    if(t==="num")return v===""?undefined:Number(v);
    if(t==="json"){if(!v.trim())return undefined;try{return JSON.parse(v);}catch{throw new Error(`campo "${k}": JSON inválido`);}}
    if(t==="lines"){const a=v.split("\n").map(x=>x.trim()).filter(Boolean);return a.length?a:undefined;}
    return v===""?undefined:v;
  };
  return {k,t,get,node:h("div",{class:"field"},
    h("label",{},k,o.req?h("span",{class:"req"},"*"):null),input,
    o.hint?h("span",{class:"hint"},o.hint):null)};
}

function endpointCard(ep, openFirst){
  const card=h("div",{class:"ep"+(openFirst?" open":"")});
  const body=h("div",{class:"ep-body"});
  card.append(
    h("div",{class:"ep-head",onclick:()=>card.classList.toggle("open")},
      h("span",{class:"method "+ep.m.toLowerCase()},ep.m),
      h("span",{class:"path"},ep.path),
      h("span",{class:"ttl"},ep.title),
      ic("chev","sm chev")),
    body);
  const fields=ep.fields.map(fieldEl);
  fields.forEach(f=>body.append(f.node));
  const respBox=h("div",{});
  const runBtn=h("button",{class:"btn",onclick:run},ic("send","sm"),"Enviar");
  body.append(h("div",{class:"btn-row",style:"margin-top:10px"},runBtn),respBox);

  async function run(){
    let vals;
    try{vals={};for(const f of fields){const v=f.get();if(v!==undefined)vals[f.k]=v;}}catch(e){return fail(e.message);}
    for(const [k,,o={}] of ep.fields) if(o.req&&(vals[k]===undefined||vals[k]==="")) return fail(`campo obrigatório: ${k}`);
    if(vals.session) LS.sess=vals.session;
    let path=ep.path;
    const pk=(ep.path.match(/\{(\w+)\}/g)||[]).map(s=>s.slice(1,-1));
    pk.forEach(p=>{path=path.replace(`{${p}}`,encodeURIComponent(vals[p]??""));delete vals[p];});
    let bodyObj;
    if(ep.custom==="contact") bodyObj={session:vals.session,chatId:vals.chatId,contacts:[{name:vals.name,phone:vals.phone}]};
    else if(ep.m==="GET"||ep.m==="DELETE"){const q=new URLSearchParams();Object.entries(vals).forEach(([k,v])=>q.set(k,v));const s=q.toString();if(s)path+="?"+s;}
    else bodyObj=vals;
    runBtn.disabled=true; clear(respBox); respBox.append(h("div",{class:"resp"},loadingBox()));
    try{
      const r=await api(ep.m,path,bodyObj);
      renderResp(r.status,r.ms,r.data,ep.m,path,bodyObj); ok(`${ep.m} ${ep.path} → ${r.status}`); pushRecent(ep);
    }catch(e){ renderResp(e.status||0,e.ms||0,e.data??e.message,ep.m,path,bodyObj); fail(e); }
    finally{ runBtn.disabled=false; }
  }
  function renderResp(code,ms,data,m,path,bodyObj){
    clear(respBox);
    const good=code>=200&&code<300;
    const raw=typeof data==="string"?data:JSON.stringify(data,null,2);
    const curl=`curl -X ${m} '${LS.base}${path}' \\\n  -H 'X-Api-Key: ***'`+(bodyObj?` \\\n  -H 'Content-Type: application/json' \\\n  -d '${JSON.stringify(bodyObj)}'`:"");
    const pane=h("div",{});
    const tab=(name,make)=>{const b=h("button",{onclick:()=>{[...bar.children].forEach(x=>x.classList.remove("active"));b.classList.add("active");clear(pane);pane.append(make());}},name);return b;};
    const bar=h("div",{class:"tabs-mini"});
    const tPretty=tab("json",()=>jsonOut(data));
    const tRaw=tab("raw",()=>h("pre",{class:"out"},raw));
    const tCurl=tab("cURL",()=>h("div",{},h("div",{class:"out-bar"},copyBtn("copiar",curl)),h("pre",{class:"out"},curl)));
    bar.append(tPretty,tRaw,tCurl); tPretty.classList.add("active"); pane.append(jsonOut(data));
    respBox.append(h("div",{class:"resp"},
      h("div",{class:"meta"},
        h("span",{class:"code "+(good?"ok":"bad")},code||"ERR"),
        h("span",{class:"ms"},ms+" ms"), bar),
      pane));
  }
  return card;
}

/* ═══════════════════════════════════════════════ views */
const views={};

views.quickstart={title:"Início rápido",async render(root){
  const page=h("div",{class:"page"}); root.append(page);
  page.append(h("div",{class:"page-head"},h("h2",{},"Início rápido"),
    h("p",{},"Do zero a mandar mensagem em 4 passos. Cada bloco já vem com o que copiar.")));
  let sessions=[];try{sessions=await loadSessions();}catch{}
  const connected=sessions.find(s=>s.status==="WORKING");
  const s0=connected||sessions[0];
  const num=(s0&&s0.jid?s0.jid:"5599999999999").split("@")[0].split(":")[0];
  const key=LS.key||"SUA_API_KEY";
  const step=(n,done,title,desc,...extra)=>h("div",{class:"qs-step"+(done?" done":"")},
    h("div",{class:"n"},done?"✓":n),h("div",{class:"body"},h("h4",{},title),desc?h("p",{},desc):null,...extra));

  page.append(h("div",{class:"card pad"},
    step(1,!!LS.key,"Conecte-se ao gateway",
      "Vá em Conexão, cole sua API key. Já feito se você está vendo isto.",
      h("a",{class:"btn ghost sm",href:"#/settings"},"Conexão")),
    step(2,sessions.length>0,"Crie uma sessão",
      "Uma sessão = um número de WhatsApp. Depois de criar, escaneie o QR (ou use 'Parear por código').",
      h("a",{class:"btn sm",href:"#/sessions"},ic("plus","sm"),"Sessões")),
    step(3,!!connected,"Pareie o número",
      connected?`Conectado: ${s0.name} · ${num}`:"Abra a sessão, escaneie o QR no WhatsApp → Aparelhos conectados.",
      !connected&&sessions.length?h("a",{class:"btn ghost sm",href:"#/sessions"},"Ver QR"):null),
    step(4,false,"Envie a primeira mensagem",
      "Troque o número pelo destino real. O chatId é o número + @s.whatsapp.net.",
      h("div",{class:"out-bar"},copyBtn("copiar",`curl -X POST '${LS.base}/api/sendText' -H 'X-Api-Key: ${key}' -H 'Content-Type: application/json' -d '{"session":"${s0?s0.name:"default"}","chatId":"${num}@s.whatsapp.net","text":"funcionou!"}'`)),
      h("pre",{class:"out"},`curl -X POST '${LS.base}/api/sendText' \\\n  -H 'X-Api-Key: ${key}' -H 'Content-Type: application/json' \\\n  -d '{"session":"${s0?s0.name:"default"}","chatId":"${num}@s.whatsapp.net","text":"funcionou!"}'`),
      h("div",{style:"margin-top:8px"},h("a",{class:"btn ghost sm",href:"#/playground"},"abrir no Playground"),
        h("a",{class:"btn ghost sm",href:"#/chat",style:"margin-left:6px"},"testar no Chat"))),
  ));

  page.append(h("div",{class:"sec-title"},ic("link","sm"),"Receber mensagens (webhook / n8n)"),
    h("div",{class:"card pad"},
      h("p",{class:"muted",style:"font-size:12.5px;margin-bottom:10px"},
        "1) No n8n, adicione o node ",h("b",{},"wa-gateway Trigger")," (ou um node Webhook). "),
      h("p",{class:"muted",style:"font-size:12.5px;margin-bottom:10px"},
        "2) ",h("b",{},"Ative o workflow")," (toggle no topo) — sem isso o n8n usa uma URL de teste temporária e a mensagem real não chega."),
      h("p",{class:"muted",style:"font-size:12.5px;margin-bottom:10px"},
        "3) Ou registre manual na config da sessão:"),
      h("div",{class:"out-bar"},copyBtn("copiar",`{"webhooks":[{"url":"https://SEU-N8N/webhook/xxxx","events":["message"],"hmac":{"secret":"um-segredo"}}]}`)),
      h("pre",{class:"out"},`{\n  "webhooks": [\n    { "url": "https://SEU-N8N/webhook/xxxx",\n      "events": ["message"],\n      "hmac": { "secret": "um-segredo" } }\n  ]\n}`),
      s0?h("button",{class:"btn ghost sm",style:"margin-top:10px",onclick:async()=>{
        try{const r=await apiData("POST",`/api/sessions/${encodeURIComponent(s0.name)}/webhook/test`,{});ok("teste disparado");
          modal("Teste de webhook",null,jsonOut(r));}catch(e){fail(e);}
      }},ic("send","sm"),"Disparar webhook.test na sessão "+s0.name):null));

  page.append(h("div",{class:"sec-title"},ic("cmd","sm"),"Integração"),
    h("div",{class:"card pad"},
      kv("Node n8n",h("a",{class:"mono",style:"color:var(--ac)",href:"https://www.npmjs.com/package/n8n-nodes-wa-gateway",target:"_blank",rel:"noopener"},"n8n-nodes-wa-gateway")),
      kv("Swagger UI",h("a",{class:"mono",style:"color:var(--ac)",href:LS.base+"/docs",target:"_blank",rel:"noopener"},LS.base+"/docs")),
      kv("MCP (IA)",h("span",{class:"mono muted"},"POST "+LS.base+"/mcp"))));
}};

views.overview={title:"Visão geral",async render(root){
  const page=h("div",{class:"page"}); root.append(page);
  page.append(h("div",{class:"page-head"},h("h2",{},"Visão geral"),h("p",{},"Estado do gateway, das sessões e atividade em tempo real.")));
  const body=h("div",{}); page.append(body);
  body.append(skeleton("kpi",5),h("div",{style:"height:20px"}),skeleton("card",1));
  let stats,sessions;
  try{[stats,sessions]=await Promise.all([apiData("GET","/api/stats"),loadSessions()]);}
  catch(e){clear(body);body.append(empty("alert",e.message,null,"não deu pra carregar"));return;}
  clear(body);
  const by=(stats.sessions&&stats.sessions.byStatus)||{};
  const cards=[
    ["sessions","Sessões",stats.sessions?stats.sessions.total:0,true],
    ["wifi","Conectadas",by.WORKING||0,!!by.WORKING],
    ["qr","Aguardando QR",by.SCAN_QR_CODE||0,!!by.SCAN_QR_CODE],
    ["circle","Paradas",(by.STOPPED||0)+(by.LOGGED_OUT||0),false],
    ["alert","Falhas",by.FAILED||0,false],
  ];
  body.append(h("div",{class:"kgrid"},cards.map(([i,k,v,on])=>
    h("div",{class:"kpi"+(on&&v?" on":"")},
      h("div",{class:"ir"},ic(i,"sm")),
      h("div",{class:"k"},k),
      h("div",{class:"v"+(v?"":" dim")},nfmt(v))))));

  body.append(h("div",{class:"sec-title"},ic("sessions","sm"),"Sessões"));
  if(sessions&&sessions.length){
    const strip=h("div",{class:"sess-strip"});
    sessions.forEach(s=>strip.append(h("a",{class:"schip",href:"#/sessions"},
      h("span",{class:"ring "+String(s.status).toLowerCase()}),s.name,
      h("span",{class:"faint",style:"font-weight:500"},s.jid?"· "+s.jid.split("@")[0].split(":")[0]:"· sem JID"))));
    body.append(strip);
  }else{
    body.append(empty("sessions","Crie uma sessão pra parear um número de WhatsApp.",
      h("a",{class:"btn sm",href:"#/sessions"},ic("plus","sm"),"Criar sessão"),"Nenhuma sessão ainda"));
  }

  // atividade ao vivo
  body.append(h("div",{class:"sec-title"},ic("events","sm"),"Atividade ao vivo"));
  const feed=h("div",{class:"mini-feed"},h("div",{class:"row"},h("span",{class:"faint"},"conectando ao stream…")));
  body.append(h("div",{class:"card pad"},feed));
  const anySess=(sessions||[])[0];
  if(anySess){
    let sock=null;
    try{
      sock=new WebSocket(`${LS.base.replace(/^http/,"ws")}/ws?session=*&events=*&api_key=${encodeURIComponent(LS.key)}`);
      sock.onopen=()=>{clear(feed);feed.append(h("div",{class:"row"},h("span",{class:"faint"},"aguardando eventos…")));};
      sock.onmessage=m=>{
        let o;try{o=JSON.parse(m.data);}catch{return;}
        if(feed.querySelector(".faint"))clear(feed);
        const ev=o.event||"?",dom=ev.split(".")[0],p=o.payload||{};
        const txt=p.body||(p.status?"status: "+p.status:"")||(p.code?"QR gerado":"")||JSON.stringify(p).slice(0,80);
        feed.prepend(h("div",{class:"row"},
          h("span",{class:"ev d-"+dom},ev),
          h("span",{class:"txt"},(o.session?o.session+" · ":"")+txt),
          h("span",{class:"ago"},new Date().toLocaleTimeString())));
        while(feed.childElementCount>9)feed.lastChild.remove();
      };
    }catch{}
    addEventListener("hashchange",()=>{try{sock&&sock.close();}catch{}},{once:true});
  }else{clear(feed);feed.append(h("div",{class:"row"},h("span",{class:"faint"},"sem sessão pra escutar")));}

  body.append(h("div",{class:"sec-title"},ic("settings","sm"),"Ambiente"),
    h("div",{class:"card pad"},
      kv("versão",stats.version||"?"),
      kv("commit",h("span",{class:"mono muted"},stats.commit||"dev")),
      kv("iniciado",fmtTime(stats.started)),
      kv("database",stats.database?badge("ok"):badge("failed")),
      kv("base",h("span",{class:"mono muted"},LS.base))));
}};

views.sessions={title:"Sessões",async render(root){
  const page=h("div",{class:"page"}); root.append(page);
  page.append(h("div",{class:"page-head"},h("div",{},h("h2",{},"Sessões"),
    h("p",{},"Status atualiza sozinho. Sessões conectadas voltam automaticamente se o servidor reiniciar."))));
  const grid=h("div",{class:"sess-grid"}); page.append(grid);
  grid.append(h("div",{class:"skel skel-card"}),h("div",{class:"skel skel-card"}));
  const refs=new Map(); const pendingQR=new Set(); let poll=null;
  const label={SCAN_QR_CODE:"escaneie o QR",STARTING:"conectando…",WORKING:"conectado",FAILED:"falhou",STOPPED:"parada",LOGGED_OUT:"deslogada"};

  const qrURL=(name)=>`${LS.base}/api/${encodeURIComponent(name)}/auth/qr.png?api_key=${encodeURIComponent(LS.key)}&c=`;
  const buildCard=(s)=>{
    const badgeSlot=h("span",{},badge(s.status));
    const qrSlot=h("div",{});
    const act=(verb,fn,dg)=>h("button",{class:"btn ghost sm"+(dg?" danger":""),onclick:async e=>{
      const b=e.currentTarget; b.disabled=true;
      try{await apiData("POST",`/api/sessions/${s.name}/${fn}`);if(fn==="start"||fn==="restart")pendingQR.add(s.name);ok(`${s.name}: ${verb}`);await refresh();}
      catch(err){fail(err);}finally{b.disabled=false;}
    }},verb);
    const num=(s.jid||"").split("@")[0].split(":")[0];
    const card=h("div",{class:"card sess hover","data-name":s.name},
      h("div",{class:"top"},
        h("div",{},h("div",{class:"nm"},ic("sessions","sm"),s.name),
          h("div",{class:"sub"},"engine "+s.engine+"  ·  ",h("span",{class:"j jid"},s.jid||"sem JID"))),
        badgeSlot),
      qrSlot,
      h("div",{class:"copy-row"},
        copyChip("sessão",s.name),
        num?copyChip("número",num):null,
        s.jid?copyChip("chatId",num+"@s.whatsapp.net",num+"@s.whatsapp.net"):null,
        h("button",{class:"cchip",onclick:()=>snippetModal(s)},ic("playground","sm"),"snippet")),
      h("div",{class:"acts"},
        act("start","start"),act("stop","stop"),act("restart","restart"),act("logout","logout",1),
        h("button",{class:"btn subtle sm",onclick:()=>cfgModal(s)},ic("settings","sm"),"Configurar"),
        h("button",{class:"btn danger ghost sm",title:"apagar sessão",onclick:async()=>{
          if(!confirm(`apagar "${s.name}"?`))return;
          try{await apiData("DELETE",`/api/sessions/${s.name}`);ok("apagada");route();}catch(e){fail(e);}
        }},ic("trash","sm"))));
    setQR(qrSlot,s);
    refs.set(s.name,{card,badgeSlot,qrSlot,status:s.status});
    return card;
  };
  const setQR=(slot,s)=>{
    clear(slot);
    if(s.status==="SCAN_QR_CODE"){
      const img=h("img",{alt:"QR",src:qrURL(s.name)+Date.now()});
      slot.append(h("div",{class:"qrbox"},img),
        h("div",{class:"muted",style:"text-align:center;font-size:11px;margin-top:6px"},"WhatsApp → Aparelhos conectados → Conectar"),
        h("div",{style:"text-align:center;margin-top:6px"},
          h("button",{class:"btn subtle sm",onclick:async()=>{
            const phone=prompt("Parear por código — número (só dígitos, internacional):",""); if(!phone)return;
            try{const r=await apiData("POST",`/api/sessions/${encodeURIComponent(s.name)}/auth/pair-code`,{phone:phone.replace(/\D/g,"")});
              modal("Código de pareamento — "+s.name,"No celular: Aparelhos conectados → Conectar → Conectar com número",
                h("div",{style:"text-align:center"},h("div",{style:"font:800 30px/1.2 var(--mono);letter-spacing:.18em;padding:12px"},r.code),
                  h("div",{class:"out-bar"},copyBtn("copiar código",r.code))));
            }catch(e){fail(e);}
          }},ic("cmd","sm"),"Parear por código")));
      slot.dataset.code="";
    }
  };
  const apply=(s)=>{
    const r=refs.get(s.name); if(!r) return;
    r.card.querySelector(".jid").textContent=s.jid||"sem JID";
    if(s.status!==r.status){
      clear(r.badgeSlot); r.badgeSlot.append(badge(s.status));
      if(s.status!=="SCAN_QR_CODE") setQR(r.qrSlot,s);
      r.status=s.status;
      if(s.status==="WORKING"&&pendingQR.has(s.name)){pendingQR.delete(s.name);ok(s.name+" conectada ✓");}
    }
    if(s.status==="SCAN_QR_CODE"&&!r.qrSlot.querySelector("img")) setQR(r.qrSlot,s);
  };
  const refreshQR=async()=>{ // troca a imagem só quando o code do whatsmeow gira
    for(const [name,r] of refs){
      if(r.status!=="SCAN_QR_CODE")continue;
      try{const q=await apiData("GET",`/api/${encodeURIComponent(name)}/auth/qr`);
        const img=r.qrSlot.querySelector("img");
        if(q.code&&img&&r.qrSlot.dataset.code!==q.code){r.qrSlot.dataset.code=q.code;img.src=qrURL(name)+encodeURIComponent(q.code.slice(0,10));}
      }catch{}
    }
  };
  const refresh=async()=>{
    let list;try{list=await apiData("GET","/api/sessions")||[];}catch{return;}
    SESSIONS=list;
    const now=list.map(s=>s.name).sort().join(","), had=[...refs.keys()].sort().join(",");
    if(now!==had){rebuild(list);return;}
    list.forEach(apply); refreshQR();
  };
  const rebuild=(list)=>{
    clear(grid); refs.clear();
    if(!list.length){grid.append(empty("sessions","nenhuma sessão ainda"));return;}
    list.forEach(s=>grid.append(buildCard(s)));
  };

  const list=await loadSessions();
  rebuild(list);
  poll=setInterval(refresh,2500);
  addEventListener("hashchange",()=>clearInterval(poll),{once:true});

  page.append(
    h("div",{class:"sec-title"},ic("plus","sm"),"Nova sessão"),
    h("div",{class:"card pad"},
      h("div",{class:"frow"},
        h("div",{class:"field"},h("label",{},"nome"),h("input",{id:"s-name",placeholder:"default"})),
        h("div",{class:"field narrow"},h("label",{}," "),h("label",{class:"check"},h("input",{type:"checkbox",id:"s-start",checked:true}),"iniciar já"))),
      h("div",{class:"btn-row",style:"margin-top:10px"},h("button",{class:"btn",onclick:async e=>{
        const name=$("#s-name").value.trim(); if(!name)return fail("nome obrigatório");
        e.target.disabled=true;
        try{const rec=await apiData("POST","/api/sessions",{name,start:$("#s-start").checked}); ok("criada"); LS.sess=name; route(); cfgModal(rec);}
        catch(err){fail(err);e.target.disabled=false;}
      }},ic("plus","sm"),"Criar sessão")),
      h("p",{class:"hint",style:"margin-top:8px"},"Depois de criar, abra Configurar para adicionar webhook e escolher os eventos.")),
  );
}};

/* ── editor de configuração da sessão (webhooks + eventos + fila) ── */
function cfgModal(s){
  let cfg={}; try{cfg=typeof s.config==="string"?JSON.parse(s.config||"{}"):(s.config||{});}catch{}
  const webhooks=Array.isArray(cfg.webhooks)?structuredCloneSafe(cfg.webhooks):[];
  const whList=h("div",{style:"display:flex;flex-direction:column;gap:12px"});
  const editors=[];

  const addEditor=(wh)=>{
    const ed=webhookEditor(wh,()=>{ed.node.remove();const i=editors.indexOf(ed);if(i>=0)editors.splice(i,1);});
    editors.push(ed); whList.append(ed.node);
  };
  webhooks.forEach(addEditor);

  const ob=cfg.outbox||{};
  const obMin=h("input",{type:"number",value:ob.minIntervalMs||"",placeholder:"3000"});
  const obJit=h("input",{type:"number",value:ob.jitterMs||"",placeholder:"2000"});
  const obDay=h("input",{type:"number",value:ob.dailyLimit||"",placeholder:"0 = ilimitado"});
  const rawChk=h("input",{type:"checkbox",checked:cfg.rawEvents||undefined});
  const autoReadChk=h("input",{type:"checkbox",checked:cfg.autoRead||undefined});
  const autoOnlineChk=h("input",{type:"checkbox",checked:cfg.autoOnline||undefined});

  const bodyNode=h("div",{},
    h("div",{class:"tabs",style:"margin-bottom:14px"},
      tabBtn("Webhooks",true),tabBtn("Fila de saída"),tabBtn("Avançado")),
    // pane webhooks
    h("div",{class:"cfg-pane","data-pane":"0"},
      whList,
      h("div",{class:"btn-row",style:"margin-top:12px"},
        h("button",{class:"btn ghost sm",onclick:()=>addEditor({url:"",events:["message","session.status"]})},ic("plus","sm"),"Adicionar webhook"),
        h("button",{class:"btn subtle sm",onclick:testWebhooks},ic("send","sm"),"Testar webhooks (salvos)")),
      h("div",{id:"wh-test-out"}),
      webhooks.length?null:h("p",{class:"hint",style:"margin-top:8px"},"Nenhum webhook. Adicione um e marque os eventos que quer receber.")),
    // pane outbox
    h("div",{class:"cfg-pane","data-pane":"1",hidden:true},
      h("p",{class:"hint",style:"margin-bottom:12px"},"Espaçamento entre envios enfileirados (evita ban). Vazio usa o padrão global."),
      h("div",{class:"frow"},
        h("div",{class:"field"},h("label",{},"intervalo mínimo (ms)"),obMin),
        h("div",{class:"field"},h("label",{},"jitter aleatório (ms)"),obJit),
        h("div",{class:"field"},h("label",{},"limite diário"),obDay))),
    // pane advanced
    h("div",{class:"cfg-pane","data-pane":"2",hidden:true},
      h("label",{class:"check",style:"margin-bottom:10px"},autoReadChk,h("span",{},"marcar mensagens recebidas como lidas automaticamente (recibo azul)")),
      h("label",{class:"check",style:"margin-bottom:10px"},autoOnlineChk,h("span",{},"manter a sessão ",h("code",{},"online")," (presença disponível) após conectar")),
      h("label",{class:"check",style:"margin-bottom:12px"},rawChk,h("span",{},"incluir ",h("code",{},"raw")," (struct cru do whatsmeow) nos eventos de mensagem")),
      h("details",{},h("summary",{class:"hint",style:"cursor:pointer"},"ver JSON final"),
        h("pre",{class:"out",id:"cfg-preview",style:"margin-top:8px"}))),
    h("div",{class:"btn-row",style:"margin-top:18px"},
      h("button",{class:"btn",onclick:save},ic("check","sm"),"Salvar configuração")),
  );

  const m=modal("Configurar — "+s.name,"Webhook, eventos e pacing da fila — igual ao painel da Evolution.",bodyNode,{wide:true});
  syncPreview();

  function tabBtn(label,active){
    return h("button",{class:active?"active":"",onclick:e=>{
      [...e.target.parentElement.children].forEach(x=>x.classList.remove("active"));
      e.target.classList.add("active");
      const idx=[...e.target.parentElement.children].indexOf(e.target);
      bodyNode.querySelectorAll(".cfg-pane").forEach(p=>p.hidden=(+p.dataset.pane!==idx));
      if(idx===2)syncPreview();
    }},label);
  }
  function collect(){
    const out={};
    const whs=editors.map(e=>e.read()).filter(w=>w.url);
    if(whs.length)out.webhooks=whs;
    const o={};
    if(obMin.value)o.minIntervalMs=+obMin.value;
    if(obJit.value)o.jitterMs=+obJit.value;
    if(obDay.value)o.dailyLimit=+obDay.value;
    if(Object.keys(o).length)out.outbox=o;
    if(rawChk.checked)out.rawEvents=true;
    if(autoReadChk.checked)out.autoRead=true;
    if(autoOnlineChk.checked)out.autoOnline=true;
    if(cfg.metadata)out.metadata=cfg.metadata;
    return out;
  }
  function syncPreview(){const p=$("#cfg-preview");if(p)p.textContent=JSON.stringify(collect(),null,2);}
  async function testWebhooks(e){
    const o=$("#wh-test-out"); clear(o); o.append(spinner());
    try{
      const r=await apiData("POST",`/api/sessions/${encodeURIComponent(s.name)}/webhook/test`,{});
      clear(o);
      o.append(table([
        {h:"url",get:x=>h("span",{class:"mono muted wrap"},trunc(x.url,42))},
        {h:"resultado",get:x=>x.ok?badge("ok"):badge("failed")},
        {h:"http",get:x=>x.status||x.error||""},
        {h:"ms",get:x=>x.ms},
      ],r,"sem webhooks"));
    }catch(err){clear(o);o.append(h("p",{class:"hint"},err.message));fail(err);}
  }
  async function save(e){
    const config=collect();
    e.target.disabled=true;
    try{await apiData("PUT",`/api/sessions/${s.name}`,{config});ok("configuração salva");m.close();route();}
    catch(err){fail(err);e.target.disabled=false;}
  }
}
function structuredCloneSafe(v){try{return structuredClone(v);}catch{return JSON.parse(JSON.stringify(v));}}

function webhookEditor(wh, onRemove){
  const sel=new Set(wh.events||[]);
  const url=h("input",{value:wh.url||"",placeholder:"https://seu-endpoint.com/webhook"});
  const secret=h("input",{value:(wh.hmac&&wh.hmac.secret)||"",placeholder:"opcional — assina com HMAC-SHA256"});
  const chips=h("div",{style:"display:flex;flex-wrap:wrap;gap:6px;margin:6px 0 10px"});
  const grid=h("div",{style:"display:grid;grid-template-columns:1fr 1fr;gap:4px 14px"});

  const renderChips=()=>{
    clear(chips);
    WILDCARDS.forEach(([w,lbl])=>{
      const on=sel.has(w);
      chips.append(h("button",{class:"badge",style:`cursor:pointer;border:1px solid ${on?"var(--ac)":"var(--line)"};background:${on?"var(--ac-bg)":"transparent"};color:${on?"var(--ac)":"var(--fg-2)"}`,
        onclick:()=>{on?sel.delete(w):sel.add(w);renderChips();renderGrid();}},on?ic("check","sm"):null,w));
    });
  };
  const renderGrid=()=>{
    clear(grid);
    const wildAll=sel.has("*");
    EVENT_CATALOG.forEach(cat=>{
      grid.append(h("div",{style:"grid-column:1/-1;font-size:10px;font-weight:700;letter-spacing:.08em;text-transform:uppercase;color:var(--fg-4);margin-top:8px"},cat.g));
      cat.items.forEach(([ev,desc])=>{
        const covered=wildAll||sel.has(ev)||(sel.has(ev.split(".")[0]+".*"));
        const cb=h("input",{type:"checkbox",checked:covered||undefined,disabled:(wildAll||sel.has(ev.split(".")[0]+".*"))||undefined,
          onchange:()=>{cb.checked?sel.add(ev):sel.delete(ev);}});
        grid.append(h("label",{class:"check",style:"align-items:flex-start;padding:3px 0",title:desc},
          cb,h("span",{},ev,h("span",{class:"hint",style:"display:block"},desc))));
      });
    });
  };
  renderChips(); renderGrid();

  const node=h("div",{class:"card pad",style:"padding:14px"},
    h("div",{class:"between",style:"margin-bottom:8px"},
      h("b",{style:"font-size:12px"},ic("link","sm"),"  Webhook"),
      h("button",{class:"btn danger ghost sm",onclick:onRemove},ic("trash","sm"))),
    h("div",{class:"field"},h("label",{},"URL"),url),
    h("div",{class:"field"},h("label",{},"segredo HMAC"),secret),
    h("label",{style:"font-size:11.5px;color:var(--dim);font-weight:600"},"Atalhos"),chips,
    h("label",{style:"font-size:11.5px;color:var(--dim);font-weight:600"},"Eventos"),grid);

  return {node, read(){
    const w={url:url.value.trim(),events:[...sel]};
    if(!w.events.length)w.events=["*"];
    if(secret.value.trim())w.hmac={secret:secret.value.trim()};
    return w;
  }};
}

function snippetModal(s){
  const num=(s.jid||"5599999999999").split("@")[0].split(":")[0];
  const chat=num+"@s.whatsapp.net";
  const key=LS.key||"SUA_API_KEY";
  const curl=`curl -X POST '${LS.base}/api/sendText' \\\n`+
    `  -H 'X-Api-Key: ${key}' -H 'Content-Type: application/json' \\\n`+
    `  -d '{"session":"${s.name}","chatId":"${chat}","text":"olá do wa-gateway"}'`;
  const node=`Node "wa-gateway"\n  Recurso:  Mensagem\n  Operação: Enviar texto\n  Sessão:   ${s.name}\n  Chat ID:  {{ $json.payload.chatId }}\n  Texto:    {{ $json.output }}`;
  const wh=JSON.stringify({webhooks:[{url:"https://SEU-N8N/webhook/xxxx",events:["message"],hmac:{secret:"um-segredo"}}]},null,2);
  const pane=h("div",{});
  const bar=h("div",{class:"tabs-mini",style:"margin-bottom:8px"});
  const tab=(nm,txt)=>{const b=h("button",{onclick:()=>{[...bar.children].forEach(x=>x.classList.remove("active"));b.classList.add("active");
    clear(pane);pane.append(h("div",{class:"out-bar"},copyBtn("copiar",txt)),h("pre",{class:"out"},txt));}},nm);return b;};
  const a=tab("cURL",curl),bb=tab("n8n",node),cc=tab("webhook config",wh);
  bar.append(a,bb,cc); a.classList.add("active");
  pane.append(h("div",{class:"out-bar"},copyBtn("copiar",curl)),h("pre",{class:"out"},curl));
  modal("Snippets — "+s.name,"pronto pra colar. chatId já montado com o número da sessão.",h("div",{},bar,pane));
}

function qrModal(name){
  const box=h("div",{class:"qr wait"},"gerando QR…");
  const sub=h("div",{class:"msub",style:"text-align:center;margin:12px 0 0"},"aguardando…");
  const img=h("img",{alt:"QR code"});
  modal("Conectar "+name,"WhatsApp → Aparelhos conectados → Conectar aparelho",h("div",{},box,sub));
  const label={SCAN_QR_CODE:"escaneie o código",STARTING:"conectando…",WORKING:"conectado ✓",FAILED:"falhou"};
  let stop=false,last=null;
  const tick=async()=>{
    if(stop||!$("#modal-root .modal"))return;
    try{
      const s=await apiData("GET","/api/sessions/"+name);
      sub.textContent=label[s.status]||s.status;
      if(s.status==="WORKING"){stop=true;ok(name+" conectada ✓");clear($("#modal-root"));route();return;}
      if(s.status==="SCAN_QR_CODE"){
        let code=null;try{code=(await apiData("GET",`/api/${encodeURIComponent(name)}/auth/qr`)).code;}catch{}
        if(code&&code!==last){last=code;if(box.classList.contains("wait")){box.className="qr";clear(box);box.append(img);}
          img.src=`${LS.base}/api/${encodeURIComponent(name)}/auth/qr.png?api_key=${encodeURIComponent(LS.key)}&c=${encodeURIComponent(code.slice(0,10))}`;}
      }
    }catch{}
    setTimeout(tick,3000);
  };
  tick();
  const bg=$("#modal-root .modal-bg"); if(bg)bg.addEventListener("click",()=>{stop=true;});
}

/* ── chat: testa envio + recebimento numa conversa ao vivo ── */
views.chat={title:"Chat",async render(root){
  await loadSessions();
  const saved=LS.chat;
  const page=h("div",{class:"page"}); root.append(page);
  page.append(h("div",{class:"page-head"},h("div",{},h("h2",{},"Chat"),
    h("p",{},"Conversa ao vivo com um número: você envia e vê as mensagens recebidas + recibos chegando em tempo real."))));

  const sess=sessionSelect("chat-sess",saved.session);
  const chatId=h("input",{value:saved.chatId||"",placeholder:"5517999999999@s.whatsapp.net"});
  const openBtn=h("button",{class:"btn"},ic("play","sm"),"Abrir conversa");
  page.append(h("div",{class:"card pad",style:"margin-bottom:14px"},
    h("div",{class:"frow",style:"align-items:flex-end"},
      h("div",{class:"field"},h("label",{},"sessão"),sess),
      h("div",{class:"field"},h("label",{},"chatId (número ou grupo)"),chatId),
      h("div",{class:"field narrow"},h("label",{}," "),openBtn))));

  const stage=h("div",{}); page.append(stage);
  let sock=null;
  addEventListener("hashchange",()=>{if(sock)sock.close();},{once:true});

  openBtn.onclick=()=>{
    const session=sess.value, cid=chatId.value.trim();
    if(!session||!cid)return fail("escolha a sessão e o chatId");
    LS.sess=session; LS.chat={session,chatId:cid};
    if(sock)sock.close();
    clear(stage);

    const bubbles=h("div",{class:"chat-body"});
    const status=h("span",{class:"pill"},h("span",{class:"dot"}),"conectando…");
    const composer=h("input",{placeholder:"mensagem…  (Enter envia)",onkeydown:e=>{if(e.key==="Enter")sendMsg();}});
    const sendBtn=h("button",{class:"btn icon",onclick:sendMsg},ic("send","sm"));
    const who=cid.split("@")[0].split(":")[0];
    stage.append(h("div",{class:"chat-wrap"},
      h("div",{class:"chat-top"},
        h("div",{class:"av"},who.slice(-2)),
        h("div",{class:"who"},who,h("small",{},cid)),
        h("div",{style:"margin-left:auto"},status)),
      bubbles,
      h("div",{class:"chat-input"},composer,sendBtn)));
    bubbles.append(h("div",{class:"chat-empty"},"carregando conversa…"));

    const pending=new Map();
    const mediaSrc=(id)=>`${LS.base}/api/messages/${encodeURIComponent(id)}/download?session=${encodeURIComponent(session)}&api_key=${encodeURIComponent(LS.key)}`;
    const addBubble=(cls,{text,type,id,mediaUrl},metaText)=>{
      if(bubbles.querySelector(".chat-empty"))clear(bubbles);
      const b=h("div",{class:"bubble "+cls});
      const img=(type==="image"||type==="sticker");
      if(img){
        const src=mediaUrl||(id?mediaSrc(id):null);
        if(src)b.append(h("img",{class:"media",src,alt:type,onclick:()=>window.open(src)}));
      }
      if(text)b.append(h("div",{},text));
      else if(!img)b.append(h("div",{class:"faint"},"["+(type||"?")+"]"));
      b.append(h("div",{class:"meta"},metaText||""));
      bubbles.append(b); bubbles.scrollTop=bubbles.scrollHeight; return b;
    };
    const setTick=(b,t)=>{
      const meta=b.querySelector(".meta"); clear(meta);
      const cls=t==="read"?"tick read":"tick";
      meta.append(fmtClock(Date.now())+" ",h("span",{class:cls},t==="delivered"||t==="read"?"✓✓":"✓"));
    };

    (async()=>{
      try{
        const hist=await apiData("GET",`/api/chats/${encodeURIComponent(cid)}/messages?session=${encodeURIComponent(session)}&limit=40`);
        clear(bubbles);
        if(Array.isArray(hist)&&hist.length){
          hist.slice().reverse().forEach(m=>addBubble(m.fromMe?"out":"in",
            {text:m.body,type:m.type,id:m.id},
            fmtClock(m.timestamp)+(m.pushName&&!m.fromMe?" · "+m.pushName:"")));
          bubbles.append(h("div",{class:"bubble sys"},"ao vivo abaixo"));
        }else bubbles.append(h("div",{class:"chat-empty"},"sem histórico — envie ou receba uma mensagem"));
      }catch{clear(bubbles);bubbles.append(h("div",{class:"chat-empty"},"conectado — envie uma mensagem"));}
    })();

    async function sendMsg(){
      const text=composer.value.trim(); if(!text)return;
      composer.value="";
      const b=addBubble("out",{text},"enviando…");
      try{
        const r=await apiData("POST","/api/sendText",{session,chatId:cid,text});
        pending.set(r.messageId,b); setTick(b,"sent");
      }catch(e){ b.querySelector(".meta").textContent="falhou: "+e.message; b.style.opacity=".6"; }
    }

    const wsBase=LS.base.replace(/^http/,"ws");
    sock=new WebSocket(`${wsBase}/ws?session=${encodeURIComponent(session)}&events=${encodeURIComponent("message,message.any,message.ack")}&api_key=${encodeURIComponent(LS.key)}`);
    sock.onopen=()=>{status.className="pill ok";status.lastChild.textContent="ao vivo";};
    sock.onclose=()=>{status.className="pill bad";status.lastChild.textContent="desconectado";};
    sock.onerror=()=>{status.className="pill bad";status.lastChild.textContent="erro";};
    sock.onmessage=(m)=>{
      let o;try{o=JSON.parse(m.data);}catch{return;}
      const ev=o.event||o.name, p=o.payload||{};
      const sameChat=(p.chatId===cid)||(p.from===cid)||(p.chatLid===cid);
      if(!sameChat)return;
      if(ev==="message"){
        addBubble(p.fromMe?"out":"in",
          {text:p.body,type:p.type,id:p.id,mediaUrl:p.media&&p.media.url?LS.base+p.media.url+"?api_key="+encodeURIComponent(LS.key):null},
          fmtClock(p.timestamp||Date.now())+(p.pushName&&!p.fromMe?" · "+p.pushName:""));
      }else if(ev==="message.ack"&&Array.isArray(p.ids)){
        p.ids.forEach(id=>{const b=pending.get(id); if(b)setTick(b,p.type);});
      }
    };
  };

  if(saved.session&&saved.chatId){sess.value=saved.session;openBtn.click();}
}};

function recents(){try{return JSON.parse(sessionStorage.getItem("wa.recent")||"[]");}catch{return[];}}
function pushRecent(ep){
  const key=ep.m+" "+ep.path;
  const l=recents().filter(x=>x.k!==key);
  l.unshift({k:key,m:ep.m,path:ep.path,g:ep.g});
  sessionStorage.setItem("wa.recent",JSON.stringify(l.slice(0,8)));
}
views.playground={title:"Playground",async render(root){
  await loadSessions();
  const page=h("div",{class:"page"}); root.append(page);
  page.append(h("div",{class:"page-head"},h("h2",{},"Playground"),
    h("p",{},"Todos os endpoints, prontos pra testar. A sessão escolhida e as chamadas recentes ficam lembradas.")));
  const search=h("input",{placeholder:"filtrar endpoint…  ( / foca aqui · ⌘K busca global )",style:"margin-bottom:12px"});
  const rec=h("div",{class:"recent"});
  const nav=h("div",{class:"pg-nav"}); const list=h("div",{class:"pg-list"});
  page.append(search,rec,h("div",{class:"pg"},nav,list));
  let active=sessionStorage.getItem("wa.pg")||GROUPS[0];
  const open=(g,path)=>{active=g;sessionStorage.setItem("wa.pg",g);search.value="";draw();
    requestAnimationFrame(()=>{const c=[...list.querySelectorAll(".ep")].find(x=>x.querySelector(".path").textContent===path);
      if(c){c.classList.add("open");c.scrollIntoView({block:"center",behavior:"smooth"});}});};
  const drawRecent=()=>{
    clear(rec);
    const r=recents(); if(!r.length)return;
    rec.append(h("span",{class:"faint",style:"font-size:10.5px;align-self:center"},"recentes:"));
    r.forEach(x=>rec.append(h("button",{class:"rc",onclick:()=>open(x.g,x.path)},
      h("span",{class:"method "+x.m.toLowerCase(),style:"font-size:8.5px"},x.m),x.path.replace("/api/",""))));
  };
  const draw=()=>{
    clear(nav);clear(list); drawRecent();
    GROUPS.forEach(g=>{
      const cnt=ENDPOINTS.filter(e=>e.g===g).length;
      nav.append(h("button",{class:g===active?"active":"",onclick:()=>{active=g;sessionStorage.setItem("wa.pg",g);search.value="";draw();}},
        g,h("span",{class:"cnt"},cnt)));
    });
    const q=search.value.toLowerCase();
    const eps=ENDPOINTS.filter(e=>q?(e.path+e.title).toLowerCase().includes(q):e.g===active);
    if(!eps.length){list.append(empty("search","tente outro termo",null,"nada encontrado"));return;}
    eps.forEach((e,i)=>list.append(endpointCard(e,q&&i===0)));
  };
  search.addEventListener("input",draw);
  draw();
}};

views.events={title:"Eventos ao vivo",async render(root){
  await loadSessions();
  const page=h("div",{class:"page"}); root.append(page);
  page.append(h("div",{class:"page-head"},h("div",{},h("h2",{},"Eventos ao vivo"),h("p",{},"Stream do WebSocket da sessão."))));
  const sel=sessionSelect("e-sess");
  const filter=h("input",{value:"*",style:"max-width:220px"});
  const feed=h("div",{class:"feed"});
  const pill=h("span",{class:"pill"},h("span",{class:"dot"}),"desligado");
  let sock=null,paused=false;
  const setPill=(t,c)=>{pill.className="pill "+(c||"");pill.lastChild.textContent=t;};
  const stop=()=>{if(sock){sock.close();sock=null;}setPill("desligado","");};
  const start=()=>{
    stop();const s=sel.value;if(!s)return fail("selecione uma sessão");LS.sess=s;
    setPill("conectando…","warn");
    sock=new WebSocket(`${LS.base.replace(/^http/,"ws")}/ws?session=${encodeURIComponent(s)}&events=${encodeURIComponent(filter.value||"*")}&api_key=${encodeURIComponent(LS.key)}`);
    sock.onopen=()=>setPill("ao vivo","ok");
    sock.onclose=()=>setPill("fechado","bad");
    sock.onerror=()=>setPill("erro","bad");
    sock.onmessage=(m)=>{
      if(paused)return;
      let o;try{o=JSON.parse(m.data);}catch{o={event:"?",payload:m.data};}
      const ev=o.event||o.name||"?",dom=ev.split(".")[0];
      feed.prepend(h("div",{class:"ln"},
        h("span",{class:"t"},new Date().toLocaleTimeString()),
        h("span",{class:"e d-"+dom},ev),
        h("span",{class:"p",html:hl(o.payload!==undefined?o.payload:o)})));
      while(feed.childElementCount>300)feed.lastChild.remove();
    };
  };
  addEventListener("hashchange",stop,{once:true});
  page.append(h("div",{class:"card pad",style:"margin-bottom:14px"},h("div",{class:"frow",style:"align-items:flex-end"},
    h("div",{class:"field"},h("label",{},"sessão"),sel),
    h("div",{class:"field"},h("label",{},"eventos (wildcard: message.*, *)"),filter),
    h("div",{class:"field narrow"},h("label",{}," "),h("div",{class:"btn-row"},
      h("button",{class:"btn",onclick:start},ic("play","sm"),"Conectar"),
      h("button",{class:"btn ghost",onclick:stop},ic("stop","sm"),"Parar"),
      h("button",{class:"btn ghost",onclick:e=>{paused=!paused;clear(e.currentTarget);e.currentTarget.append(paused?"Retomar":"Pausar");}},"Pausar"),
      h("button",{class:"btn ghost",onclick:()=>clear(feed)},"Limpar"),pill)))),feed);
}};

views.monitor={title:"Monitoramento",async render(root){
  await loadSessions();
  const page=h("div",{class:"page"}); root.append(page);
  page.append(h("div",{class:"page-head"},h("div",{},h("h2",{},"Monitoramento"),h("p",{},"Fila de saída e entregas de webhook, por sessão."))));
  const sel=sessionSelect("m-sess");
  const auto=h("input",{type:"checkbox"});
  const tabsEl=h("div",{class:"tabs"}); const box=h("div",{});
  let tab="outbox",timer=null;
  const cols={
    outbox:[
      {h:"criado",get:j=>h("span",{class:"muted",title:fmtTime(j.createdAt)},fmtRel(j.createdAt))},
      {h:"kind",get:j=>j.kind},{h:"status",get:j=>badge(j.status)},
      {h:"runAt",get:j=>h("span",{class:"muted"},fmtTime(j.runAt))},{h:"tent.",get:j=>j.attempts},
      {h:"messageId",get:j=>h("span",{class:"mono"},trunc(j.messageId,22))},
      {h:"erro",get:j=>h("span",{class:"muted wrap"},j.lastError||"")}],
    deliveries:[
      {h:"criado",get:d=>h("span",{class:"muted",title:fmtTime(d.createdAt)},fmtRel(d.createdAt))},
      {h:"evento",get:d=>d.event},{h:"status",get:d=>badge(d.status)},
      {h:"cód",get:d=>d.responseCode||""},{h:"tent.",get:d=>d.attempts},
      {h:"url",get:d=>h("span",{class:"mono muted wrap"},trunc(d.url,44))},
      {h:"erro",get:d=>h("span",{class:"muted wrap"},d.lastError||"")}],
  };
  const load=async()=>{
    const s=sel.value;if(!s){clear(box);box.append(empty("inbox","selecione uma sessão"));return;}
    LS.sess=s;
    try{const rows=await apiData("GET",`/api/${tab}?session=${encodeURIComponent(s)}&limit=100`);clear(box);box.append(table(cols[tab],rows,"nada ainda"));}
    catch(e){clear(box);box.append(empty("alert",e.message));}
  };
  const drawTabs=()=>{clear(tabsEl);[["outbox","Fila de saída","clock"],["deliveries","Webhooks","link"]].forEach(([t,l,i])=>
    tabsEl.append(h("button",{class:t===tab?"active":"",onclick:()=>{tab=t;drawTabs();load();}},ic(i,"sm"),l)));};
  sel.addEventListener("change",load);
  auto.addEventListener("change",()=>{clearInterval(timer);if(auto.checked)timer=setInterval(load,4000);});
  addEventListener("hashchange",()=>clearInterval(timer),{once:true});
  page.append(h("div",{class:"card pad",style:"margin-bottom:14px"},h("div",{class:"frow",style:"align-items:flex-end"},
    h("div",{class:"field"},h("label",{},"sessão"),sel),
    h("div",{class:"field narrow"},h("label",{}," "),h("button",{class:"btn ghost",onclick:load},ic("refresh","sm"),"Carregar")),
    h("div",{class:"field narrow"},h("label",{}," "),h("label",{class:"check"},auto,"auto 4s")))),
    tabsEl,box);
  drawTabs();load();
}};

views.keys={title:"API keys",async render(root){
  const page=h("div",{class:"page"}); root.append(page);
  page.append(h("div",{class:"page-head"},h("div",{},h("h2",{},"API keys"),h("p",{},"Chaves da tabela api_keys. A master do .env não aparece aqui."))));
  const body=h("div",{}); page.append(body); body.append(skeleton("row",4));
  let list;
  try{list=await apiData("GET","/api/keys");}
  catch(e){clear(body);body.append(empty("keys",e.status===403?"precisa de uma chave com escopo * (a master serve)":e.message));return;}
  clear(body);
  body.append(
    table([
      {h:"id",get:k=>h("span",{class:"mono"},k.id)},
      {h:"label",get:k=>k.label||"—"},
      {h:"escopos",get:k=>(k.scopes||[]).join(", ")},
      {h:"criada",get:k=>h("span",{class:"muted"},fmtRel(k.createdAt))},
      {h:"estado",get:k=>k.revokedAt?badge("failed"):badge("ok")},
      {h:"",get:k=>k.revokedAt?"":h("button",{class:"btn danger ghost sm",onclick:async()=>{
        if(!confirm("revogar "+k.id+"?"))return;
        try{await apiData("DELETE","/api/keys/"+encodeURIComponent(k.id));ok("revogada");route();}catch(e){fail(e);}
      }},ic("trash","sm"))},
    ],list,"nenhuma chave"),
    h("div",{class:"sec-title"},ic("plus","sm"),"Nova chave"),
    h("div",{class:"card pad"},
      h("div",{class:"frow"},
        h("div",{class:"field"},h("label",{},"label"),h("input",{id:"nk-l",placeholder:"app-x"})),
        h("div",{class:"field"},h("label",{},"escopos (vírgula, vazio = *)"),h("input",{id:"nk-s",placeholder:"*"}))),
      h("div",{class:"btn-row",style:"margin-top:10px"},h("button",{class:"btn",onclick:async e=>{
        const scopes=$("#nk-s").value.split(",").map(x=>x.trim()).filter(Boolean);
        e.target.disabled=true;
        try{const r=await apiData("POST","/api/keys",{label:$("#nk-l").value,scopes});
          modal("Chave criada","copie agora — não é exibida de novo",h("div",{},h("pre",{class:"out"},r.token),
            h("div",{class:"out-bar",style:"margin-top:6px"},copyBtn("copiar token",r.token))));
          route();
        }catch(err){fail(err);}finally{e.target.disabled=false;}
      }},ic("plus","sm"),"Gerar chave"))),
  );
}};

views.settings={title:"Conexão",async render(root){
  const page=h("div",{class:"page"}); root.append(page);
  page.append(h("div",{class:"page-head"},h("div",{},h("h2",{},"Conexão"),h("p",{},"Credenciais ficam só no localStorage deste navegador."))));
  const base=h("input",{value:LS.base,placeholder:"https://…"});
  const key=h("input",{value:LS.key,type:"password",placeholder:"X-Api-Key"});
  const out=h("div",{style:"margin-top:12px"});
  page.append(h("div",{class:"card pad"},
    h("div",{class:"field"},h("label",{},"base URL do gateway"),base),
    h("div",{class:"field"},h("label",{},"API key"),key,h("span",{class:"hint"},"a master key do .env também funciona")),
    h("div",{class:"btn-row",style:"margin-top:10px"},
      h("button",{class:"btn",onclick:()=>{LS.base=base.value;LS.key=key.value;ok("salvo");renderFoot();health();}},ic("check","sm"),"Salvar"),
      h("button",{class:"btn ghost",onclick:async e=>{
        clear(out);e.target.disabled=true;
        try{out.append(jsonOut(await apiData("GET","/api/version")));ok("conectou ✓");}catch(err){out.append(jsonOut(err.message));fail(err);}
        finally{e.target.disabled=false;}
      }},"Testar")),
    out));
  page.append(h("div",{class:"sec-title"},ic("link","sm"),"Endpoints de integração"),
    h("div",{class:"card pad"},
      kv("OpenAPI",h("a",{class:"mono",style:"color:var(--ac)",href:LS.base+"/openapi.json",target:"_blank",rel:"noopener"},"/openapi.json")),
      kv("Swagger UI",h("a",{class:"mono",style:"color:var(--ac)",href:LS.base+"/docs",target:"_blank",rel:"noopener"},"/docs")),
      kv("MCP (agentes IA)",h("span",{class:"mono muted"},"POST "+LS.base+"/mcp")),
      kv("WebSocket",h("span",{class:"mono muted"},"/ws?session=…&events=*&api_key=…"))));
}};

/* ═══════════════════════════════════════════════ command palette (⌘K) */
function openPalette(){
  if($(".palette-bg"))return;
  const items=[
    ...NAV.flatMap(([,its])=>its.map(([id,ico,label])=>({kind:"nav",id,label,sub:"ir para"}))),
    ...ENDPOINTS.map(e=>({kind:"ep",ep:e,label:e.title,sub:e.path,m:e.m})),
  ];
  const input=h("input",{placeholder:"buscar view ou endpoint…",autocomplete:"off"});
  const listEl=h("div",{class:"list"});
  const box=h("div",{class:"palette"},input,listEl);
  const bg=h("div",{class:"palette-bg",onclick:e=>{if(e.target===bg)close();}},box);
  document.body.append(bg);
  let sel=0,shown=[];
  const close=()=>{bg.remove();removeEventListener("keydown",onKey);};
  const render=()=>{
    const q=input.value.toLowerCase().trim();
    shown=items.filter(it=>!q||(it.label+" "+it.sub).toLowerCase().includes(q)).slice(0,40);
    sel=Math.min(sel,Math.max(0,shown.length-1));
    clear(listEl);
    if(!shown.length){listEl.append(h("div",{class:"empty"},"nada encontrado"));return;}
    shown.forEach((it,i)=>{
      listEl.append(h("div",{class:"row"+(i===sel?" sel":""),onmouseenter:()=>{sel=i;paint();},onclick:()=>go(it)},
        it.kind==="ep"?h("span",{class:"method "+it.m.toLowerCase()},it.m):ic(it.id,"sm"),
        h("span",{},it.label),
        h("span",{class:"sub"},it.sub)));
    });
  };
  const paint=()=>{[...listEl.children].forEach((c,i)=>c.classList.toggle("sel",i===sel));};
  const go=(it)=>{
    close();
    if(it.kind==="nav"){location.hash="#/"+it.id;}
    else{location.hash="#/playground";setTimeout(()=>{
      sessionStorage.setItem("wa.pg",it.ep.g);route().then(()=>{
        const cards=[...document.querySelectorAll(".ep")];
        const idx=ENDPOINTS.filter(e=>e.g===it.ep.g).indexOf(it.ep);
        const c=cards[idx]; if(c){c.classList.add("open");c.scrollIntoView({block:"center",behavior:"smooth"});}
      });
    },60);}
  };
  const onKey=(e)=>{
    if(e.key==="Escape")return close();
    if(e.key==="ArrowDown"){e.preventDefault();sel=(sel+1)%shown.length;paint();listEl.children[sel]?.scrollIntoView({block:"nearest"});}
    if(e.key==="ArrowUp"){e.preventDefault();sel=(sel-1+shown.length)%shown.length;paint();listEl.children[sel]?.scrollIntoView({block:"nearest"});}
    if(e.key==="Enter"&&shown[sel]){e.preventDefault();go(shown[sel]);}
  };
  input.addEventListener("input",render);
  addEventListener("keydown",onKey);
  render(); setTimeout(()=>input.focus(),30);
}

/* ═══════════════════════════════════════════════ shell */
const NAV=[
  ["Principal",[["quickstart","play","Início rápido"],["overview","overview","Visão geral"],["sessions","sessions","Sessões"],["chat","chat","Chat"],["playground","playground","Playground"]]],
  ["Observar",[["events","events","Eventos ao vivo"],["monitor","monitor","Monitoramento"]]],
  ["Config",[["keys","keys","API keys"],["settings","settings","Conexão"]]],
];
function renderNav(active){
  const nav=$("#nav");clear(nav);
  nav.append(h("div",{class:"conn-chip",id:"conn-chip"},h("span",{class:"lamp"}),
    h("div",{},h("div",{class:"lbl"},"verificando…"),h("div",{class:"sub"},trunc(LS.base.replace(/^https?:\/\//,""),22)))));
  NAV.forEach(([grp,items])=>{
    const g=h("div",{class:"nav-group"},h("div",{class:"glbl"},grp));
    items.forEach(([id,icon,label])=>g.append(h("a",{href:"#/"+id,class:id===active?"active":"",onclick:()=>closeMenu()},
      ic(icon,"sm"),label)));
    nav.append(g);
  });
  const docs=h("div",{class:"nav-group"},h("div",{class:"glbl"},"Referência"));
  docs.append(h("a",{href:LS.base+"/docs",target:"_blank",rel:"noopener",onclick:()=>closeMenu()},ic("link","sm"),"API docs (Swagger)"));
  nav.append(docs);
  paintConn();
}
function renderFoot(){
  const f=$("#foot");clear(f);
  f.append(
    h("div",{class:"r"},h("b",{},"key"),h("span",{class:"v"},LS.key?trunc(LS.key,8)+"…":"—")),
    h("button",{class:"tbtn",onclick:toggleTheme},ic(LS.theme==="dark"?"sun":"moon","sm"),LS.theme==="dark"?"Tema claro":"Tema escuro"),
  );
}
function applyTheme(){document.documentElement.setAttribute("data-theme",LS.theme);}
function toggleTheme(){LS.theme=LS.theme==="dark"?"light":"dark";applyTheme();renderFoot();route();}
function closeMenu(){$("#sidebar").classList.remove("open");$("#scrim")?.classList.remove("on");}

let _healthy=null;
function paintConn(){
  const c=$("#conn-chip");if(!c)return;
  c.className="conn-chip"+(_healthy===true?" ok":_healthy===false?" bad":"");
  c.querySelector(".lbl").textContent=_healthy===true?"online":_healthy===false?"offline":"verificando…";
}
async function health(){
  const p=$("#health");
  try{const hh=await apiData("GET","/health");_healthy=hh.status==="ok";p.className="pill "+(_healthy?"ok":"bad");p.lastChild.textContent=_healthy?"online":hh.status;}
  catch{_healthy=false;p.className="pill bad";p.lastChild.textContent="offline";}
  paintConn();
}

async function route(){
  const hash=(location.hash.replace(/^#\/?/,"")||"overview").split("?")[0];
  if(!LS.key&&hash!=="settings")return onboard();
  const name=views[hash]?hash:"overview";
  renderNav(name);
  $("#crumb").textContent=views[name].title;
  const view=$("#view");clear(view);
  try{await views[name].render(view);}
  catch(e){view.append(empty("alert","erro: "+e.message));}
}
function onboard(){
  renderNav("settings");$("#crumb").textContent="Bem-vindo";
  const view=$("#view");clear(view);
  const base=h("input",{value:LS.base,placeholder:"https://…"});
  const key=h("input",{type:"password",placeholder:"X-Api-Key (a master do .env serve)"});
  view.append(h("div",{class:"onboard"},
    h("div",{class:"logo"},"w"),h("h2",{},"Conectar ao gateway"),
    h("p",{},"Cole a API key para começar."),
    h("div",{class:"card pad",style:"text-align:left"},
      h("div",{class:"field"},h("label",{},"base URL"),base),
      h("div",{class:"field"},h("label",{},"API key"),key),
      h("div",{class:"btn-row",style:"margin-top:8px"},h("button",{class:"btn block",onclick:async e=>{
        if(!key.value.trim())return fail("cole a API key");
        e.target.disabled=true;LS.base=base.value;LS.key=key.value;
        try{await apiData("GET","/api/version");ok("conectado ✓");renderFoot();location.hash="#/quickstart";route();}
        catch(err){fail(err);e.target.disabled=false;}
      }},"Entrar")))));
}

$("#refresh").addEventListener("click",route);
$("#menu-btn").addEventListener("click",()=>{
  const open=$("#sidebar").classList.toggle("open");
  $("#scrim").classList.toggle("on",open);
});
$("#scrim").addEventListener("click",closeMenu);
addEventListener("hashchange",route);
addEventListener("keydown",e=>{
  if((e.metaKey||e.ctrlKey)&&e.key.toLowerCase()==="k"){e.preventDefault();openPalette();return;}
  const typing=/^(INPUT|TEXTAREA|SELECT)$/.test((e.target||{}).tagName||"");
  if(e.key==="/"&&!typing){const s=$("#view input[placeholder^='filtrar']");if(s){e.preventDefault();s.focus();}}
});
$("#kbd-hint")?.addEventListener("click",openPalette);

applyTheme();renderFoot();route();health();setInterval(health,15000);
