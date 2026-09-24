
async function refreshDashboard() {
 const runningEl = document.getElementById('dashboard-running-tasks');
 const vulnTotalEl = document.getElementById('dashboard-vuln-total');
 const severityIds = ['critical', 'high', 'medium', 'low', 'info'];

 if (runningEl) runningEl.textContent = '…';
 if (vulnTotalEl) vulnTotalEl.textContent = '…';
 severityIds.forEach(s => {
 const el = document.getElementById('dashboard-severity-' + s);
 if (el) el.textContent = '0';
 const barEl = document.getElementById('dashboard-bar-' + s);
 if (barEl) barEl.style.width = '0%';
 });
 setDashboardOverviewPlaceholder('…');
 setEl('dashboard-kpi-tools-calls', '…');
 setEl('dashboard-kpi-success-rate', '…');
 var chartPlaceholder = document.getElementById('dashboard-tools-pie-placeholder');
 if (chartPlaceholder) { chartPlaceholder.style.removeProperty('display'); chartPlaceholder.textContent = (typeof window.t === 'function' ? window.t('common.loading') : 'Loading…'); }
 var barChartEl = document.getElementById('dashboard-tools-bar-chart');
 if (barChartEl) { barChartEl.style.display = 'none'; barChartEl.innerHTML = ''; }

 if (typeof apiFetch === 'undefined') {
 if (runningEl) runningEl.textContent = '-';
 if (vulnTotalEl) vulnTotalEl.textContent = '-';
 setDashboardOverviewPlaceholder('-');
 return;
 }

 try {
 const [tasksRes, vulnRes, batchRes, monitorRes, knowledgeRes, skillsRes] = await Promise.all([
 apiFetch('/api/agent-loop/tasks').then(r => r.ok ? r.json() : null).catch(() => null),
 apiFetch('/api/vulnerabilities/stats').then(r => r.ok ? r.json() : null).catch(() => null),
 apiFetch('/api/batch-tasks?limit=500&page=1').then(r => r.ok ? r.json() : null).catch(() => null),
 apiFetch('/api/monitor/stats').then(r => r.ok ? r.json() : null).catch(() => null),
 apiFetch('/api/knowledge/stats').then(r => r.ok ? r.json() : null).catch(() => null),
 apiFetch('/api/skills/stats').then(r => r.ok ? r.json() : null).catch(() => null)
 ]);
 let agentRunningCount = null;
 if (tasksRes && Array.isArray(tasksRes.tasks)) {
 agentRunningCount = tasksRes.tasks.length;
 }
 let batchRunningCount = 0;
 if (batchRes && Array.isArray(batchRes.queues)) {
 batchRes.queues.forEach(q => {
 if ((q.status || '').toLowerCase() === 'running') batchRunningCount++;
 });
 }
 if (runningEl) {
 if (agentRunningCount !== null) {
 runningEl.textContent = String(agentRunningCount + batchRunningCount);
 } else if (batchRes && Array.isArray(batchRes.queues)) {
 runningEl.textContent = String(batchRunningCount);
 } else {
 runningEl.textContent = '-';
 }
 }

 if (vulnRes && typeof vulnRes.total === 'number') {
 if (vulnTotalEl) vulnTotalEl.textContent = String(vulnRes.total);
 const bySeverity = vulnRes.by_severity || {};
 const total = vulnRes.total || 0;
 severityIds.forEach(sev => {
 const count = bySeverity[sev] || 0;
 const el = document.getElementById('dashboard-severity-' + sev);
 if (el) el.textContent = String(count);
 const barEl = document.getElementById('dashboard-bar-' + sev);
 if (barEl) barEl.style.width = total > 0 ? (count / total * 100) + '%' : '0%';
 });
 } else {
 if (vulnTotalEl) vulnTotalEl.textContent = '-';
 severityIds.forEach(sev => {
 const barEl = document.getElementById('dashboard-bar-' + sev);
 if (barEl) barEl.style.width = '0%';
 });
 }
 if (batchRes && Array.isArray(batchRes.queues)) {
 const queues = batchRes.queues;
 let pending = 0, running = batchRunningCount, done = 0;
 queues.forEach(q => {
 const s = (q.status || '').toLowerCase();
 if (s === 'pending' || s === 'paused') pending++;
 else if (s === 'running') { /* already counted into batchRunningCount */ }
 else if (s === 'completed' || s === 'cancelled') done++;
 });
 const total = pending + running + done;
 setEl('dashboard-batch-pending', String(pending));
 setEl('dashboard-batch-running', String(running));
 setEl('dashboard-batch-done', String(done));
 setEl('dashboard-batch-total', total > 0 ? (typeof window.t === 'function' ? window.t('dashboard.totalCount', { count: total }) : ` ${total} `) : (typeof window.t === 'function' ? window.t('dashboard.noTasks') : 'No tasks'));
 if (total > 0) {
 const pendingPct = (pending / total * 100).toFixed(1);
 const runningPct = (running / total * 100).toFixed(1);
 const donePct = (done / total * 100).toFixed(1);
 updateProgressBar('dashboard-batch-progress-pending', pendingPct);
 updateProgressBar('dashboard-batch-progress-running', runningPct);
 updateProgressBar('dashboard-batch-progress-done', donePct);
 } else {
 updateProgressBar('dashboard-batch-progress-pending', '0');
 updateProgressBar('dashboard-batch-progress-running', '0');
 updateProgressBar('dashboard-batch-progress-done', '0');
 }
 } else {
 setEl('dashboard-batch-pending', '-');
 setEl('dashboard-batch-running', '-');
 setEl('dashboard-batch-done', '-');
 setEl('dashboard-batch-total', '-');
 updateProgressBar('dashboard-batch-progress-pending', '0');
 updateProgressBar('dashboard-batch-progress-running', '0');
 updateProgressBar('dashboard-batch-progress-done', '0');
 }
 if (monitorRes && typeof monitorRes === 'object') {
 const names = Object.keys(monitorRes);
 let totalCalls = 0, totalSuccess = 0, totalFailed = 0;
 names.forEach(k => {
 const v = monitorRes[k];
 const n = v && (v.totalCalls ?? v.TotalCalls);
 if (typeof n === 'number') totalCalls += n;
 const s = v && (v.successCalls ?? v.SuccessCalls);
 if (typeof s === 'number') totalSuccess += s;
 const f = v && (v.failedCalls ?? v.FailedCalls);
 if (typeof f === 'number') totalFailed += f;
 });
 setEl('dashboard-tools-count', String(names.length));
 setEl('dashboard-tools-calls', formatNumber(totalCalls));
 setEl('dashboard-kpi-tools-calls', String(totalCalls));
 var rateStr = totalCalls > 0 ? ((totalSuccess / totalCalls) * 100).toFixed(1) + '%' : '-';
 setEl('dashboard-kpi-success-rate', rateStr);
 setEl('dashboard-tools-success-rate', rateStr !== '-' ? `Success rate ${rateStr}` : '-');
 renderDashboardToolsBar(monitorRes);
 } else {
 setEl('dashboard-tools-count', '-');
 setEl('dashboard-tools-calls', '-');
 setEl('dashboard-kpi-tools-calls', '-');
 setEl('dashboard-kpi-success-rate', '-');
 setEl('dashboard-tools-success-rate', '-');
 renderDashboardToolsBar(null);
 }
 const knowledgeItemsEl = document.getElementById('dashboard-knowledge-items');
 const knowledgeCategoriesEl = document.getElementById('dashboard-knowledge-categories');
 const knowledgeStatusEl = document.getElementById('dashboard-knowledge-status');
 if (knowledgeRes && typeof knowledgeRes === 'object') {
 if (knowledgeRes.enabled === false) {
 if (knowledgeStatusEl) knowledgeStatusEl.textContent = (typeof window.t === 'function' ? window.t('dashboard.notEnabled') : 'Disabled');
 if (knowledgeItemsEl) knowledgeItemsEl.textContent = '-';
 if (knowledgeCategoriesEl) knowledgeCategoriesEl.textContent = '-';
 } else {
 const categories = knowledgeRes.total_categories ?? 0;
 const items = knowledgeRes.total_items ?? 0;
 if (knowledgeItemsEl) knowledgeItemsEl.textContent = formatNumber(items);
 if (knowledgeCategoriesEl) knowledgeCategoriesEl.textContent = formatNumber(categories);
 if (knowledgeStatusEl) {
 if (items > 0 || categories > 0) {
 knowledgeStatusEl.textContent = (typeof window.t === 'function' ? window.t('dashboard.enabled') : 'Enabled');
 } else {
 knowledgeStatusEl.textContent = (typeof window.t === 'function' ? window.t('dashboard.toConfigure') : 'To configure');
 }
 }
 }
 } else {
 if (knowledgeItemsEl) knowledgeItemsEl.textContent = '-';
 if (knowledgeCategoriesEl) knowledgeCategoriesEl.textContent = '-';
 if (knowledgeStatusEl) knowledgeStatusEl.textContent = '-';
 }
 if (skillsRes && typeof skillsRes === 'object') {
 const totalSkills = skillsRes.total_skills ?? 0;
 const totalCalls = skillsRes.total_calls ?? 0;
 setEl('dashboard-skills-count', formatNumber(totalSkills));
 setEl('dashboard-skills-calls', formatNumber(totalCalls));
 const statusEl = document.getElementById('dashboard-skills-status');
 if (statusEl) {
 if (totalCalls === 0) {
 statusEl.textContent = (typeof window.t === 'function' ? window.t('dashboard.toUse') : 'To use');
 statusEl.style.background = 'rgba(0, 0, 0, 0.05)';
 statusEl.style.color = 'var(--text-secondary)';
 } else if (totalCalls < 10) {
 statusEl.textContent = (typeof window.t === 'function' ? window.t('dashboard.active') : 'Active');
 statusEl.style.background = 'rgba(16, 185, 129, 0.1)';
 statusEl.style.color = '#10b981';
 } else {
 statusEl.textContent = (typeof window.t === 'function' ? window.t('dashboard.highFreq') : 'High frequency');
 statusEl.style.background = 'rgba(59, 130, 246, 0.1)';
 statusEl.style.color = '#3b82f6';
 }
 }
 } else {
 setEl('dashboard-skills-count', '-');
 setEl('dashboard-skills-calls', '-');
 const statusEl = document.getElementById('dashboard-skills-status');
 if (statusEl) statusEl.textContent = '-';
 }
 } catch (e) {
 console.warn('Dashboardfailed', e);
 if (runningEl) runningEl.textContent = '-';
 if (vulnTotalEl) vulnTotalEl.textContent = '-';
 setDashboardOverviewPlaceholder('-');
 setEl('dashboard-kpi-success-rate', '-');
 setEl('dashboard-kpi-tools-calls', '-');
 renderDashboardToolsBar(null);
 var ph = document.getElementById('dashboard-tools-pie-placeholder');
 if (ph) { ph.style.removeProperty('display'); ph.textContent = (typeof window.t === 'function' ? window.t('dashboard.noCallData') : 'No call data'); }
 }
}

function setEl(id, text) {
 const el = document.getElementById(id);
 if (el) el.textContent = text;
}

function setDashboardOverviewPlaceholder(t) {
 ['dashboard-batch-pending', 'dashboard-batch-running', 'dashboard-batch-done', 'dashboard-batch-total',
 'dashboard-tools-count', 'dashboard-tools-calls', 'dashboard-tools-success-rate',
 'dashboard-skills-count', 'dashboard-skills-calls', 'dashboard-skills-status',
 'dashboard-knowledge-items', 'dashboard-knowledge-categories', 'dashboard-knowledge-status'].forEach(id => setEl(id, t));
 updateProgressBar('dashboard-batch-progress-pending', '0');
 updateProgressBar('dashboard-batch-progress-running', '0');
 updateProgressBar('dashboard-batch-progress-done', '0');
}
function formatNumber(num) {
 if (typeof num !== 'number' || isNaN(num)) return '-';
 if (num === 0) return '0';
 return num.toLocaleString('zh-CN');
}
function updateProgressBar(id, percentage) {
 const el = document.getElementById(id);
 if (el) {
 const pct = parseFloat(percentage) || 0;
 el.style.width = Math.max(0, Math.min(100, pct)) + '%';
 }
}
var DASHBOARD_BAR_COLORS = [
 '#93c5fd', '#a78bfa', '#6ee7b7', '#fde047', '#fda4af',
 '#7dd3fc', '#a5b4fc', '#5eead4', '#fdba74', '#e9d5ff',
 '#67e8f9', '#c4b5fd', '#86efac', '#fcd34d', '#f9a8d4',
 '#bae6fd', '#c7d2fe', '#99f6e4', '#fed7aa', '#ddd6fe',
 '#22d3ee', '#8b5cf6', '#4ade80', '#fbbf24', '#fb7185',
 '#38bdf8', '#818cf8', '#2dd4bf', '#fb923c', '#e0e7ff'
];

function esc(s) {
 if (typeof s !== 'string') return '';
 return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/"/g, '&quot;');
}

function renderDashboardToolsBar(monitorRes) {
 const placeholder = document.getElementById('dashboard-tools-pie-placeholder');
 const barChartEl = document.getElementById('dashboard-tools-bar-chart');
 if (!placeholder || !barChartEl) return;

 if (!monitorRes || typeof monitorRes !== 'object') {
 placeholder.style.removeProperty('display');
 placeholder.textContent = (typeof window.t === 'function' ? window.t('dashboard.noCallData') : 'No call data');
 barChartEl.style.display = 'none';
 barChartEl.innerHTML = '';
 return;
 }

 const entries = Object.keys(monitorRes).map(function (k) {
 const v = monitorRes[k];
 const totalCalls = v && (v.totalCalls ?? v.TotalCalls);
 return { name: k, totalCalls: typeof totalCalls === 'number' ? totalCalls : 0 };
 }).filter(function (e) { return e.totalCalls > 0; })
 .sort(function (a, b) { return b.totalCalls - a.totalCalls; })
 .slice(0, 30);

 if (entries.length === 0) {
 placeholder.style.removeProperty('display');
 placeholder.textContent = (typeof window.t === 'function' ? window.t('dashboard.noCallData') : 'No call data');
 barChartEl.style.display = 'none';
 barChartEl.innerHTML = '';
 return;
 }

 placeholder.style.display = 'none';
 barChartEl.style.display = 'block';

 const maxCalls = Math.max.apply(null, entries.map(function (e) { return e.totalCalls; }));
 var html = '';
 entries.forEach(function (e, i) {
 var pct = maxCalls > 0 ? (e.totalCalls / maxCalls) * 100 : 0;
 var label = e.name.length > 12 ? e.name.slice(0, 10) + '…' : e.name;
 var fullName = esc(e.name);
 html += '<div class="dashboard-tools-bar-item" data-tooltip="' + fullName + '">';
 html += '<span class="dashboard-tools-bar-label">' + esc(label) + '</span>';
 html += '<div class="dashboard-tools-bar-track"><div class="dashboard-tools-bar-fill" style="width:' + pct + '%"></div></div>';
 html += '<span class="dashboard-tools-bar-value">' + e.totalCalls + '</span>';
 html += '</div>';
 });
 barChartEl.innerHTML = html;
 attachDashboardBarTooltips(barChartEl);
}

var dashboardBarTooltipEl = null;
var dashboardBarTooltipTimer = null;

function attachDashboardBarTooltips(barChartEl) {
 if (!barChartEl) return;
 if (!dashboardBarTooltipEl) {
 dashboardBarTooltipEl = document.createElement('div');
 dashboardBarTooltipEl.className = 'dashboard-tools-bar-tooltip';
 dashboardBarTooltipEl.setAttribute('role', 'tooltip');
 document.body.appendChild(dashboardBarTooltipEl);
 }
 barChartEl.removeEventListener('mouseover', dashboardBarTooltipOnOver);
 barChartEl.removeEventListener('mouseout', dashboardBarTooltipOnOut);
 barChartEl.addEventListener('mouseover', dashboardBarTooltipOnOver);
 barChartEl.addEventListener('mouseout', dashboardBarTooltipOnOut);
}

function dashboardBarTooltipOnOver(ev) {
 var item = ev.target && ev.target.closest && ev.target.closest('.dashboard-tools-bar-item');
 if (!item || !dashboardBarTooltipEl) return;
 var text = item.getAttribute('data-tooltip');
 if (!text) return;
 clearTimeout(dashboardBarTooltipTimer);
 dashboardBarTooltipTimer = setTimeout(function () {
 dashboardBarTooltipEl.textContent = text;
 dashboardBarTooltipEl.style.display = 'block';
 requestAnimationFrame(function () {
 var rect = item.getBoundingClientRect();
 var ttRect = dashboardBarTooltipEl.getBoundingClientRect();
 var x = rect.left + (rect.width / 2) - (ttRect.width / 2);
 var y = rect.top - ttRect.height - 6;
 if (y < 8) y = rect.bottom + 6;
 var pad = 8;
 if (x < pad) x = pad;
 if (x + ttRect.width > window.innerWidth - pad) x = window.innerWidth - ttRect.width - pad;
 dashboardBarTooltipEl.style.left = x + 'px';
 dashboardBarTooltipEl.style.top = y + 'px';
 });
 }, 180);
}

function dashboardBarTooltipOnOut(ev) {
 var item = ev.target && ev.target.closest && ev.target.closest('.dashboard-tools-bar-item');
 var related = ev.relatedTarget && ev.relatedTarget.closest && ev.relatedTarget.closest('.dashboard-tools-bar-item');
 if (item && item === related) return;
 clearTimeout(dashboardBarTooltipTimer);
 dashboardBarTooltipTimer = null;
 if (dashboardBarTooltipEl) dashboardBarTooltipEl.style.display = 'none';
}

/* ============================================================
   Enterprise motion layer (Pyntra dashboard)
   Non-invasive: enhances the existing dashboard with count-up
   animations, live per-KPI sparklines, an animated severity
   donut, auto-refresh + live indicator, and card entrance.
   All driven by the SAME real data refreshDashboard() loads.
   ============================================================ */
(function () {
  "use strict";
  if (window.__pyntraDashPro) return;
  window.__pyntraDashPro = true;

  var REDUCED = window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  var KPI_IDS = ["dashboard-running-tasks", "dashboard-vuln-total", "dashboard-kpi-tools-calls", "dashboard-kpi-success-rate"];
  var SEV_IDS = ["critical", "high", "medium", "low", "info"];
  var history = {};
  var refreshTimer = null;

  function cssv(n, fb) {
    try { var v = getComputedStyle(document.documentElement).getPropertyValue(n).trim(); return v || fb; }
    catch (e) { return fb; }
  }
  function sevColor(s) {
    var fb = { critical: "#dc2626", high: "#ea580c", medium: "#d97706", low: "#2563eb", info: "#64748b" };
    return cssv("--sev-" + s, fb[s]);
  }
  function parseVal(txt) {
    if (txt == null) return null;
    var m = String(txt).replace(/,/g, "").match(/-?\d+(\.\d+)?/);
    return m ? parseFloat(m[0]) : null;
  }
  function fmt(v, pct) {
    if (pct) return (Math.round(v * 10) / 10).toFixed(1) + "%";
    return Math.round(v).toLocaleString();
  }

  /* count-up on the real KPI numbers */
  function animateTo(el, target, pct) {
    // _writing stays true for the WHOLE write (incl. across rAF frames) so the
    // MutationObserver — which fires asynchronously — never re-triggers us.
    if (REDUCED || (typeof el._shown === "number" && el._shown === target)) {
      el._writing = true; el.textContent = fmt(target, pct); el._shown = target;
      Promise.resolve().then(function () { el._writing = false; });
      return;
    }
    var from = (typeof el._shown === "number") ? el._shown : 0;
    var dur = 700, start = performance.now();
    cancelAnimationFrame(el._raf);
    el._writing = true;
    function step(now) {
      var p = Math.min(1, (now - start) / dur), e = 1 - Math.pow(1 - p, 3);
      var v = from + (target - from) * e;
      el.textContent = fmt(v, pct);
      if (p < 1) { el._raf = requestAnimationFrame(step); }
      else { el._shown = target; Promise.resolve().then(function () { el._writing = false; }); }
    }
    el._raf = requestAnimationFrame(step);
  }
  function watchKPI(id) {
    var el = document.getElementById(id); if (!el) return;
    var pct = id.indexOf("success-rate") > -1;
    function handle() {
      if (el._writing) return;
      var val = parseVal(el.textContent.trim());
      if (val == null) return;
      if (val === el._shown) return;
      pushHistory(id, val);
      animateTo(el, val, pct);
    }
    new MutationObserver(handle).observe(el, { childList: true, characterData: true, subtree: true });
    handle();
  }

  /* rolling sparkline history + canvas */
  function pushHistory(id, v) {
    (history[id] = history[id] || []).push(v);
    if (history[id].length > 40) history[id].shift();
    var cv = document.getElementById("spark-" + id);
    if (cv) drawSpark(cv, history[id], id);
  }
  function ensureSparks() {
    KPI_IDS.forEach(function (id) {
      var el = document.getElementById(id); if (!el) return;
      var card = el.closest(".dashboard-kpi-card"); if (!card || card.querySelector(".kpi-spark")) return;
      var cv = document.createElement("canvas");
      cv.className = "kpi-spark"; cv.id = "spark-" + id;
      cv.setAttribute("aria-hidden", "true");
      card.appendChild(cv);
    });
  }
  function drawSpark(cv, data, id) {
    var c = cv.getContext("2d"); if (!c) return;
    if (!data || data.length < 2) { c.clearRect(0, 0, cv.width, cv.height); return; }
    var dpr = window.devicePixelRatio || 1;
    var w = cv.clientWidth || 92, h = cv.clientHeight || 30;
    cv.width = w * dpr; cv.height = h * dpr;
    c.setTransform(dpr, 0, 0, dpr, 0, 0); c.clearRect(0, 0, w, h);
    var color = id.indexOf("vuln") > -1 ? sevColor("critical")
      : id.indexOf("success") > -1 ? cssv("--success-color", "#16a34a")
      : id.indexOf("tools") > -1 ? cssv("--info-color", "#64748b")
      : cssv("--accent-color", "#2563eb");
    var mn = Math.min.apply(null, data), mx = Math.max.apply(null, data), rg = (mx - mn) || 1, p = 3;
    var X = function (i) { return p + i * (w - 2 * p) / (data.length - 1); };
    var Y = function (v) { return h - p - (v - mn) / rg * (h - 2 * p); };
    // Flat, low-opacity area (no gradient) under a crisp line.
    c.beginPath(); c.moveTo(X(0), Y(data[0]));
    for (var i = 1; i < data.length; i++) c.lineTo(X(i), Y(data[i]));
    c.lineTo(X(data.length - 1), h - p); c.lineTo(X(0), h - p); c.closePath(); c.fillStyle = hexA(color, 0.08); c.fill();
    c.beginPath(); c.moveTo(X(0), Y(data[0]));
    for (i = 1; i < data.length; i++) c.lineTo(X(i), Y(data[i]));
    c.lineWidth = 1.5; c.strokeStyle = color; c.lineJoin = "round"; c.stroke();
    c.beginPath(); c.arc(X(data.length - 1), Y(data[data.length - 1]), 2.2, 0, 7); c.fillStyle = color; c.fill();
  }
  function hexA(hex, a) {
    hex = String(hex).trim();
    if (hex.charAt(0) === "#") {
      hex = hex.slice(1); if (hex.length === 3) hex = hex.split("").map(function (ch) { return ch + ch; }).join("");
      var n = parseInt(hex, 16); return "rgba(" + ((n >> 16) & 255) + "," + ((n >> 8) & 255) + "," + (n & 255) + "," + a + ")";
    }
    return hex;
  }

  /* animated severity donut */
  function ensureDonut() {
    var bar = document.getElementById("dashboard-stacked-bar");
    var wrap = bar && bar.closest(".dashboard-chart-wrap"); if (!wrap) return null;
    if (!wrap.classList.contains("has-donut")) {
      wrap.classList.add("has-donut");
      var box = document.createElement("div"); box.className = "dashboard-donut-box";
      var cv = document.createElement("canvas"); cv.id = "dashboard-donut"; cv.className = "dashboard-donut";
      var ctr = document.createElement("div"); ctr.className = "dashboard-donut-center";
      ctr.innerHTML = '<span class="dashboard-donut-total" id="dashboard-donut-total">0</span><span class="dashboard-donut-cap">Findings</span>';
      box.appendChild(cv); box.appendChild(ctr);
      wrap.insertBefore(box, wrap.firstChild);
    }
    return document.getElementById("dashboard-donut");
  }
  var donutTimer = null;
  function drawDonut() {
    var cv = ensureDonut(); if (!cv) return;
    var counts = SEV_IDS.map(function (s) { var el = document.getElementById("dashboard-severity-" + s); return Math.max(0, parseVal(el && el.textContent) || 0); });
    var total = counts.reduce(function (a, b) { return a + b; }, 0);
    var totalEl = document.getElementById("dashboard-donut-total"); if (totalEl) totalEl.textContent = total.toLocaleString();
    var dpr = window.devicePixelRatio || 1, S = 132;
    cv.width = S * dpr; cv.height = S * dpr; cv.style.width = S + "px"; cv.style.height = S + "px";
    var x = cv.getContext("2d"); if (!x) return; x.setTransform(dpr, 0, 0, dpr, 0, 0);
    var cx = S / 2, cy = S / 2, r = S / 2 - 8, lw = 14;
    function render(prog) {
      x.clearRect(0, 0, S, S);
      x.lineWidth = lw; x.lineCap = "butt";
      x.beginPath(); x.arc(cx, cy, r, 0, Math.PI * 2); x.strokeStyle = cssv("--bg-tertiary", "#e8edf4"); x.stroke();
      if (total === 0) return;
      var a0 = -Math.PI / 2;
      for (var i = 0; i < SEV_IDS.length; i++) {
        if (!counts[i]) continue;
        var frac = counts[i] / total, a1 = a0 + frac * Math.PI * 2 * prog;
        x.beginPath(); x.arc(cx, cy, r, a0, a1); x.strokeStyle = sevColor(SEV_IDS[i]); x.stroke();
        a0 = a1;
      }
    }
    if (REDUCED) { render(1); return; }
    var start = performance.now(), dur = 800;
    (function step(now) {
      var pr = Math.min(1, (now - start) / dur), e = 1 - Math.pow(1 - pr, 3);
      render(e); if (pr < 1) requestAnimationFrame(step);
    })(start);
  }

  /* live indicator + auto refresh */
  function ensureLive() {
    var actions = document.querySelector("#page-dashboard .page-header-actions");
    if (!actions || document.getElementById("dashboard-live")) return;
    var s = document.createElement("span");
    s.id = "dashboard-live"; s.className = "dashboard-live";
    s.innerHTML = '<span class="dashboard-live-dot"></span><span>Live</span><span class="dashboard-live-sep">/</span><span id="dashboard-live-time">just now</span>';
    actions.insertBefore(s, actions.firstChild);
  }
  function markUpdated() {
    var t = document.getElementById("dashboard-live-time");
    if (t) t.textContent = new Date().toLocaleTimeString();
    drawDonut();
    KPI_IDS.forEach(function (id) { var cv = document.getElementById("spark-" + id); if (cv && history[id]) drawSpark(cv, history[id], id); });
  }
  function dashActive() {
    var p = document.getElementById("page-dashboard");
    return p && p.classList.contains("active");
  }
  function startAuto() {
    if (refreshTimer) return;
    refreshTimer = setInterval(function () {
      if (dashActive() && !document.hidden && typeof refreshDashboard === "function") {
        Promise.resolve(refreshDashboard()).then(markUpdated);
      }
    }, 15000);
  }

  function boot() {
    ensureSparks(); ensureDonut(); ensureLive();
    KPI_IDS.forEach(watchKPI);
    SEV_IDS.forEach(function (s) {
      var el = document.getElementById("dashboard-severity-" + s); if (!el) return;
      new MutationObserver(function () { clearTimeout(donutTimer); donutTimer = setTimeout(function () { drawDonut(); }, 60); })
        .observe(el, { childList: true, characterData: true, subtree: true });
    });
    new MutationObserver(function () { setTimeout(markUpdated, 30); })
      .observe(document.documentElement, { attributes: true, attributeFilter: ["data-theme"] });
    startAuto();
    setTimeout(markUpdated, 400);
  }

  if (typeof window.refreshDashboard === "function") {
    var orig = window.refreshDashboard;
    window.refreshDashboard = function () {
      var r = orig.apply(this, arguments);
      Promise.resolve(r).then(function () { setTimeout(markUpdated, 50); });
      return r;
    };
  }

  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", boot);
  else boot();
})();
