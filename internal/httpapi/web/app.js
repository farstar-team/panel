const $ = (s, root = document) => root.querySelector(s);
const $$ = (s, root = document) => [...root.querySelectorAll(s)];
const state = {
  csrf: "", tunnels: [], system: {}, events: [], eventSource: null,
  chartIn: Array(60).fill(0), chartOut: Array(60).fill(0), lastIn: 0, lastOut: 0,
};

document.addEventListener("DOMContentLoaded", init);

async function init() {
  bindUI();
  applyTheme(localStorage.getItem("farstar-theme") || "dark");
  try {
    const session = await api("/api/session");
    state.csrf = session.csrf;
    showApp();
  } catch {
    showLogin();
  }
}

function bindUI() {
  $("#login-form").addEventListener("submit", login);
  $("#logout-btn").addEventListener("click", logout);
  $$(".nav-item").forEach(btn => btn.addEventListener("click", () => switchView(btn.dataset.view)));
  $$("[data-goto]").forEach(btn => btn.addEventListener("click", () => switchView(btn.dataset.goto)));
  ["#new-tunnel-top","#new-tunnel-btn","#quick-new","#empty-state button"].forEach(id => $(id)?.addEventListener("click", () => openTunnelModal()));
  $("#quick-logs").addEventListener("click", () => switchView("activity"));
  ["#quick-backup","#setting-backup"].forEach(id => $(id).addEventListener("click", downloadBackup));
  ["#theme-btn","#setting-theme"].forEach(id => $(id).addEventListener("click", toggleTheme));
  $("#menu-btn").addEventListener("click", () => $(".sidebar").classList.toggle("open"));
  $("#search-input").addEventListener("input", renderTunnels);
  $("#tunnel-list").addEventListener("click", handleTunnelAction);
  $("#recent-tunnels").addEventListener("click", handleTunnelAction);
  $("#tunnel-form").addEventListener("submit", saveTunnel);
  $("#t-role").addEventListener("change", updateFormVisibility);
  $("#t-protocol").addEventListener("change", updateFormVisibility);
  $$("[data-close-modal]").forEach(el => el.addEventListener("click", closeModal));
  $$("[data-close-logs]").forEach(el => el.addEventListener("click", closeLogs));
  $("#clear-activity").addEventListener("click", () => { state.events = []; renderActivity(); });
  window.addEventListener("resize", drawChart);
}

async function login(event) {
  event.preventDefault();
  const button = event.submitter;
  button.disabled = true;
  $("#login-error").textContent = "";
  try {
    const result = await api("/api/auth/login", {
      method: "POST",
      body: { username: $("#login-user").value, password: $("#login-pass").value },
      noCSRF: true,
    });
    state.csrf = result.csrf;
    $("#login-pass").value = "";
    showApp();
  } catch (error) {
    $("#login-error").textContent = error.message;
  } finally {
    button.disabled = false;
  }
}

async function logout() {
  try { await api("/api/auth/logout", { method: "POST" }); } catch {}
  if (state.eventSource) state.eventSource.close();
  state.csrf = "";
  showLogin();
}

function showLogin() {
  $("#app").classList.add("hidden");
  $("#login-view").classList.remove("hidden");
  setTimeout(() => $("#login-user").focus(), 50);
}

async function showApp() {
  $("#login-view").classList.add("hidden");
  $("#app").classList.remove("hidden");
  await refreshAll();
  connectEvents();
}

async function refreshAll() {
  try {
    [state.tunnels, state.system] = await Promise.all([api("/api/tunnels"), api("/api/system")]);
    renderAll();
  } catch (error) {
    if (error.status === 401) return showLogin();
    toast(error.message, "error");
  }
}

function connectEvents() {
  state.eventSource?.close();
  const source = new EventSource("/api/events");
  state.eventSource = source;
  source.onmessage = event => {
    const data = JSON.parse(event.data);
    captureChanges(data.tunnels);
    updateTraffic(data.tunnels);
    state.tunnels = data.tunnels;
    state.system = data.system;
    renderAll();
  };
  source.onerror = () => {
    if (source.readyState === EventSource.CLOSED) setTimeout(connectEvents, 3000);
  };
}

function renderAll() {
  renderSummary();
  renderTunnels();
  renderRecent();
  renderActivity();
  drawChart();
}

function renderSummary() {
  const active = state.tunnels.filter(t => t.status === "running").length;
  const connections = sum(state.tunnels, "active_connections");
  const bytes = sum(state.tunnels, "bytes_in") + sum(state.tunnels, "bytes_out");
  const health = state.tunnels.length ? Math.round(active / state.tunnels.length * 100) : 100;
  $("#active-tunnels").textContent = fa(active);
  $("#total-tunnels").textContent = `از ${fa(state.tunnels.length)} تانل`;
  $("#total-traffic").textContent = formatBytes(bytes);
  $("#active-connections").textContent = fa(connections);
  $("#memory-use").textContent = `${Number(state.system.memory_mb || 0).toFixed(1)} MB`;
  $("#panel-uptime").textContent = `آپ‌تایم ${formatDuration(state.system.uptime_seconds || 0)}`;
  $("#health-percent").textContent = `${health}%`;
  $("#nav-count").textContent = fa(state.tunnels.length);
}

function renderTunnels() {
  const query = ($("#search-input")?.value || "").trim().toLowerCase();
  const filtered = state.tunnels.filter(t => [t.name,t.protocol,t.listen_addr,t.remote_addr,t.role].join(" ").toLowerCase().includes(query));
  $("#tunnel-list").innerHTML = filtered.map(tunnelRow).join("");
  $("#empty-state").classList.toggle("hidden", state.tunnels.length !== 0);
}

function renderRecent() {
  $("#recent-tunnels").innerHTML = state.tunnels.slice(0,4).map(t => tunnelRow(t, true)).join("") ||
    `<div class="muted" style="padding:20px;text-align:center;font-size:11px">هنوز تانلی ساخته نشده است.</div>`;
}

function tunnelRow(t, compact = false) {
  const running = t.status === "running" || t.status === "starting";
  const address = t.role === "server" ? t.listen_addr : t.remote_addr;
  const statusText = ({running:"فعال",starting:"در حال شروع",stopping:"در حال توقف",stopped:"متوقف"})[t.status] || t.status;
  return `<article class="tunnel-row" data-id="${esc(t.id)}">
    <div class="tunnel-name"><div class="protocol-icon">${t.protocol === "wss" ? "W" : "T"}</div><div><b>${esc(t.name)}</b><small>${t.role === "server" ? "سرور" : "کلاینت"} · ${t.protocol.toUpperCase()}</small></div></div>
    <div><span class="status ${esc(t.status)}">${statusText}</span></div>
    <div class="metric address-metric"><b>${esc(address || "—")}</b><small>آدرس مسیر</small></div>
    ${compact ? "" : `<div class="metric hide-compact"><b>${formatBytes((t.bytes_in||0)+(t.bytes_out||0))}</b><small>${fa(t.active_connections)} اتصال فعال</small></div>`}
    <div class="row-actions">
      <button data-action="${running ? "stop" : "start"}" title="${running ? "توقف" : "شروع"}">${running ? "■" : "▶"}</button>
      <button data-action="logs" title="لاگ">⌁</button>
      <button data-action="edit" title="ویرایش">✎</button>
      <button data-action="delete" class="danger" title="حذف">×</button>
    </div>
  </article>`;
}

async function handleTunnelAction(event) {
  const button = event.target.closest("[data-action]");
  const row = event.target.closest("[data-id]");
  if (!button || !row) return;
  const tunnel = state.tunnels.find(t => t.id === row.dataset.id);
  if (!tunnel) return;
  const action = button.dataset.action;
  if (action === "edit") return openTunnelModal(tunnel);
  if (action === "logs") return openLogs(tunnel);
  if (action === "delete") {
    if (!confirm(`تانل «${tunnel.name}» حذف شود؟`)) return;
    try {
      await api(`/api/tunnels/${encodeURIComponent(tunnel.id)}`, { method:"DELETE" });
      state.tunnels = state.tunnels.filter(t => t.id !== tunnel.id);
      addActivity(`تانل ${tunnel.name} حذف شد`, "تنظیمات مسیر برای همیشه پاک شد.");
      renderAll(); toast("تانل حذف شد.", "success");
    } catch (error) { toast(error.message, "error"); }
    return;
  }
  button.disabled = true;
  try {
    await api(`/api/tunnels/${encodeURIComponent(tunnel.id)}/${action}`, { method:"POST" });
    tunnel.status = action === "start" ? "starting" : "stopping";
    addActivity(`${action === "start" ? "راه‌اندازی" : "توقف"} ${tunnel.name}`, "دستور برای موتور تانل ارسال شد.");
    renderAll();
  } catch (error) { toast(error.message, "error"); }
  finally { button.disabled = false; }
}

function openTunnelModal(tunnel = null) {
  $("#modal-title").textContent = tunnel ? "ویرایش مسیر" : "ساخت تانل جدید";
  $("#tunnel-id").value = tunnel?.id || "";
  $("#t-name").value = tunnel?.name || "";
  $("#t-role").value = tunnel?.role || "server";
  $("#t-protocol").value = tunnel?.protocol || "tcp";
  $("#t-listen").value = tunnel?.listen_addr || "0.0.0.0:443";
  $("#t-remote").value = tunnel?.remote_addr || "";
  $("#t-public").value = (tunnel?.public_ports || ["0.0.0.0:8000"]).join("\n");
  $("#t-local").value = (tunnel?.local_services || ["127.0.0.1:3000"]).join("\n");
  $("#t-secret").value = "";
  $("#t-cert").value = tunnel?.tls_cert || "";
  $("#t-key").value = tunnel?.tls_key || "";
  $("#t-tls-name").value = tunnel?.tls_server_name || "";
  $("#t-ca").value = tunnel?.tls_ca_cert || "";
  $("#t-skip-verify").checked = !!tunnel?.skip_tls_verify;
  $("#t-autostart").checked = tunnel ? tunnel.autostart : true;
  $("#form-error").textContent = "";
  updateFormVisibility();
  $("#modal").classList.remove("hidden");
  setTimeout(() => $("#t-name").focus(), 50);
}

function closeModal() { $("#modal").classList.add("hidden"); }

function updateFormVisibility() {
  const client = $("#t-role").value === "client";
  const wss = $("#t-protocol").value === "wss";
  $("#listen-field").classList.toggle("hidden", client);
  $("#remote-field").classList.toggle("hidden", !client);
  $("#public-field").classList.toggle("hidden", client);
  $("#local-field").classList.toggle("hidden", !client);
  $("#tls-fields").classList.toggle("hidden", !wss);
  $("#t-cert").closest("label").classList.toggle("hidden", !wss || client);
  $("#t-key").closest("label").classList.toggle("hidden", !wss || client);
  ["#tls-name-field","#tls-ca-field","#skip-verify-field"].forEach(id => $(id).classList.toggle("hidden", !wss || !client));
  if (client && wss && !$("#t-remote").value) $("#t-remote").placeholder = "wss://tunnel.example.com/tunnel";
  if (client && !wss && !$("#t-remote").value) $("#t-remote").placeholder = "1.2.3.4:443";
}

async function saveTunnel(event) {
  event.preventDefault();
  const id = $("#tunnel-id").value;
  const role = $("#t-role").value;
  const payload = {
    name: $("#t-name").value.trim(), role, protocol: $("#t-protocol").value,
    listen_addr: role === "server" ? $("#t-listen").value.trim() : "",
    remote_addr: role === "client" ? $("#t-remote").value.trim() : "",
    public_ports: role === "server" ? lines($("#t-public").value) : [],
    local_services: role === "client" ? lines($("#t-local").value) : [],
    secret: $("#t-secret").value,
    tls_cert: $("#t-cert").value.trim(), tls_key: $("#t-key").value.trim(),
    tls_server_name: $("#t-tls-name").value.trim(), tls_ca_cert: $("#t-ca").value.trim(),
    skip_tls_verify: $("#t-skip-verify").checked, autostart: $("#t-autostart").checked,
  };
  const button = $("#save-tunnel");
  button.disabled = true; $("#form-error").textContent = "";
  try {
    await api(id ? `/api/tunnels/${encodeURIComponent(id)}` : "/api/tunnels", {
      method: id ? "PUT" : "POST", body: payload,
    });
    closeModal();
    addActivity(`${id ? "ویرایش" : "ساخت"} تانل ${payload.name}`, `${payload.protocol.toUpperCase()} · ${role === "server" ? "سرور" : "کلاینت"}`);
    await refreshAll();
    toast(id ? "تغییرات ذخیره شد." : "تانل جدید ساخته شد.", "success");
  } catch (error) {
    $("#form-error").textContent = error.message;
  } finally { button.disabled = false; }
}

async function openLogs(tunnel) {
  $("#log-title").textContent = `لاگ ${tunnel.name}`;
  $("#log-content").textContent = "در حال دریافت...";
  $("#log-drawer").classList.remove("hidden");
  try {
    const result = await api(`/api/tunnels/${encodeURIComponent(tunnel.id)}/logs`);
    $("#log-content").textContent = result.logs || "هنوز لاگی ثبت نشده است.";
    $("#log-content").scrollTop = $("#log-content").scrollHeight;
  } catch (error) { $("#log-content").textContent = error.message; }
}
function closeLogs() { $("#log-drawer").classList.add("hidden"); }

function switchView(view) {
  $$(".view").forEach(el => el.classList.add("hidden"));
  $(`#${view}-view`).classList.remove("hidden");
  $$(".nav-item").forEach(el => el.classList.toggle("active", el.dataset.view === view));
  const titles = {dashboard:"نمای کلی شبکه",tunnels:"مدیریت تانل‌ها",activity:"رویدادهای زنده",settings:"تنظیمات پنل"};
  $("#page-title").textContent = titles[view];
  $(".sidebar").classList.remove("open");
  if (view === "dashboard") setTimeout(drawChart, 30);
}

function captureChanges(next) {
  const old = new Map(state.tunnels.map(t => [t.id,t]));
  next.forEach(t => {
    const previous = old.get(t.id);
    if (previous && previous.status !== t.status) {
      const labels = {running:"فعال شد",stopped:"متوقف شد",starting:"در حال راه‌اندازی است",stopping:"در حال توقف است"};
      addActivity(`تانل ${t.name} ${labels[t.status] || t.status}`, t.last_error || `${t.protocol.toUpperCase()} · ${t.role}`);
    }
  });
}

function addActivity(title, detail) {
  state.events.unshift({title,detail,time:new Date()});
  state.events = state.events.slice(0,80);
  renderActivity();
}

function renderActivity() {
  $("#activity-list").innerHTML = state.events.map(e => `<div class="activity-item"><span></span><div><b>${esc(e.title)}</b><small>${esc(e.detail)}</small></div><time>${e.time.toLocaleTimeString("fa-IR",{hour:"2-digit",minute:"2-digit"})}</time></div>`).join("") ||
    `<div class="muted" style="padding:35px;text-align:center;font-size:11px">رویداد تازه‌ای ثبت نشده است.</div>`;
}

function updateTraffic(tunnels) {
  const nowIn = sum(tunnels,"bytes_in"), nowOut = sum(tunnels,"bytes_out");
  state.chartIn.push(Math.max(0, nowIn - state.lastIn) / 2);
  state.chartOut.push(Math.max(0, nowOut - state.lastOut) / 2);
  state.chartIn.shift(); state.chartOut.shift();
  state.lastIn = nowIn; state.lastOut = nowOut;
}

function drawChart() {
  const canvas = $("#traffic-chart");
  if (!canvas || canvas.offsetParent === null) return;
  const dpr = window.devicePixelRatio || 1, rect = canvas.getBoundingClientRect();
  canvas.width = rect.width * dpr; canvas.height = rect.height * dpr;
  const ctx = canvas.getContext("2d"); ctx.scale(dpr,dpr);
  const w=rect.width,h=rect.height,p=12;
  const css=getComputedStyle(document.documentElement), line=css.getPropertyValue("--line"), primary=css.getPropertyValue("--primary"), cyan=css.getPropertyValue("--cyan");
  ctx.clearRect(0,0,w,h); ctx.strokeStyle=line; ctx.lineWidth=1;
  for(let i=1;i<5;i++){const y=(h/5)*i;ctx.beginPath();ctx.moveTo(0,y);ctx.lineTo(w,y);ctx.stroke()}
  const max=Math.max(1024,...state.chartIn,...state.chartOut);
  drawSeries(ctx,state.chartIn,primary,w,h,p,max,true); drawSeries(ctx,state.chartOut,cyan,w,h,p,max,false);
}

function drawSeries(ctx,data,color,w,h,p,max,fill) {
  ctx.beginPath();
  data.forEach((v,i)=>{const x=p+i*(w-2*p)/(data.length-1),y=h-p-(v/max)*(h-2*p);i?ctx.lineTo(x,y):ctx.moveTo(x,y)});
  ctx.strokeStyle=color;ctx.lineWidth=2;ctx.stroke();
  if(fill){ctx.lineTo(w-p,h-p);ctx.lineTo(p,h-p);ctx.closePath();const g=ctx.createLinearGradient(0,0,0,h);g.addColorStop(0,color+"33");g.addColorStop(1,color+"00");ctx.fillStyle=g;ctx.fill()}
}

function downloadBackup() {
  if (confirm("فایل بکاپ شامل رازهای تانل است. دانلود در اتصال HTTPS و نگهداری در محل امن توصیه می‌شود. ادامه می‌دهید؟")) {
    window.location.href = "/api/backup";
  }
}

function toggleTheme() { applyTheme(document.documentElement.dataset.theme === "light" ? "dark" : "light"); }
function applyTheme(theme) {
  document.documentElement.dataset.theme = theme;
  localStorage.setItem("farstar-theme",theme);
  setTimeout(drawChart,20);
}

async function api(url, options = {}) {
  const headers = {"Accept":"application/json", ...(options.headers || {})};
  if (options.body !== undefined) headers["Content-Type"] = "application/json";
  if (!options.noCSRF && options.method && options.method !== "GET") headers["X-CSRF-Token"] = state.csrf;
  const response = await fetch(url, {...options, headers, body: options.body !== undefined ? JSON.stringify(options.body) : undefined});
  let data = null; const text = await response.text();
  if (text) { try { data = JSON.parse(text); } catch { data = {error:text}; } }
  if (!response.ok) {
    const error = new Error(data?.error || `خطای ${response.status}`); error.status=response.status; throw error;
  }
  return data;
}

function toast(message,type="") {
  const el=document.createElement("div");el.className=`toast ${type}`;el.textContent=message;$("#toast-root").append(el);
  setTimeout(()=>el.remove(),3500);
}
function lines(value){return value.split(/\r?\n/).map(v=>v.trim()).filter(Boolean)}
function sum(items,key){return items.reduce((n,item)=>n+Number(item[key]||0),0)}
function fa(value){return Number(value||0).toLocaleString("fa-IR")}
function formatBytes(value){let n=Number(value||0),units=["B","KB","MB","GB","TB"],i=0;while(n>=1024&&i<units.length-1){n/=1024;i++}return `${n<10&&i? n.toFixed(1):Math.round(n).toLocaleString("en-US")} ${units[i]}`}
function formatDuration(seconds){seconds=Math.max(0,Number(seconds));const d=Math.floor(seconds/86400),h=Math.floor(seconds%86400/3600),m=Math.floor(seconds%3600/60);return d?`${fa(d)} روز`:h?`${fa(h)} ساعت`:m?`${fa(m)} دقیقه`:`${fa(seconds)} ثانیه`}
function esc(value){return String(value??"").replace(/[&<>"']/g,c=>({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;"}[c]))}
