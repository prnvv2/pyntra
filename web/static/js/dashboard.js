/* ============================================================================
   Pyntra — Command Center controller (clean SaaS layout)
   KPI cards + deltas · hero area chart (session trend) · severity segments ·
   recent-findings table · top-tools bars · success gauge · Pyntra Assistant.
   Entry point: refreshDashboard().
   ============================================================================ */
(function () {
  "use strict";

  var REDUCED = window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  var SEVS = ["critical", "high", "medium", "low", "info"];
  var SEV_COLORS = { critical: "#dc2626", high: "#ea580c", medium: "#b45309", low: "#1d4ed8", info: "#475569" };
  var SEV_LABEL = { critical: "Critical", high: "High", medium: "Medium", low: "Low", info: "Info" };
  var prev = {};            // previous metric values (for deltas)
  var trend = [];           // session activity history for the area chart
  var autoTimer = null;

  function $(id) { return document.getElementById(id); }
  function esc(s) { return String(s == null ? "" : s).replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;"); }
  function cssv(n, fb) { try { var v = getComputedStyle(document.documentElement).getPropertyValue(n).trim(); return v || fb; } catch (e) { return fb; } }
  function fmt(n) { return (typeof n === "number" && !isNaN(n)) ? n.toLocaleString(window.__locale || undefined) : "-"; }

  function countUp(el, target, suffix) {
    if (!el) return; suffix = suffix || "";
    if (REDUCED || typeof target !== "number") { el.textContent = (typeof target === "number" ? fmt(target) : target) + suffix; el._v = target; return; }
    var from = typeof el._v === "number" ? el._v : 0;
    if (from === target) { el.textContent = fmt(target) + suffix; return; }
    var s = performance.now(); cancelAnimationFrame(el._raf);
    (function step(now) {
      var p = Math.min(1, (now - s) / 600), e = 1 - Math.pow(1 - p, 3);
      el.textContent = fmt(Math.round(from + (target - from) * e)) + suffix;
      if (p < 1) el._raf = requestAnimationFrame(step); else { el._v = target; el.textContent = fmt(target) + suffix; }
    })(s);
  }

  function setDelta(id, cur, key) {
    var el = $(id); if (!el) return;
    var p = prev[key];
    if (typeof p !== "number" || p === cur) { el.className = "cc-delta"; el.textContent = (typeof p !== "number") ? "—" : "0%"; }
    else {
      var pct = p === 0 ? 100 : ((cur - p) / Math.abs(p) * 100);
      el.className = "cc-delta " + (cur >= p ? "up" : "down");
      el.textContent = Math.abs(pct).toFixed(pct % 1 === 0 ? 0 : 1) + "%";
    }
    prev[key] = cur;
  }

  /* ---- Hero area chart --------------------------------------------------- */
  var areaPts = [];
  function drawArea() {
    var cv = $("cc-area"); if (!cv) return;
    var dpr = window.devicePixelRatio || 1, w = cv.clientWidth || 600, h = cv.clientHeight || 230;
    cv.width = w * dpr; cv.height = h * dpr;
    var c = cv.getContext("2d"); if (!c) return; c.setTransform(dpr, 0, 0, dpr, 0, 0); c.clearRect(0, 0, w, h);
    var data = trend.length ? trend : [0, 0];
    if (data.length < 2) data = [data[0] || 0, data[0] || 0];
    var pad = 8, mn = Math.min.apply(null, data), mx = Math.max.apply(null, data), rg = (mx - mn) || 1;
    var X = function (i) { return pad + i * (w - 2 * pad) / (data.length - 1); };
    var Y = function (v) { return h - pad - (v - mn) / rg * (h - 2 * pad - 10); };
    // gridlines
    c.strokeStyle = cssv("--border-1", "#eee"); c.lineWidth = 1;
    for (var g = 0; g <= 3; g++) { var gy = pad + g * (h - 2 * pad) / 3; c.beginPath(); c.moveTo(pad, gy); c.lineTo(w - pad, gy); c.globalAlpha = .5; c.stroke(); c.globalAlpha = 1; }
    var red = cssv("--primary", "#dc2626");
    var grad = c.createLinearGradient(0, 0, 0, h);
    grad.addColorStop(0, "rgba(" + (cssv("--primary-rgb", "220,38,38")) + ",0.22)");
    grad.addColorStop(1, "rgba(" + (cssv("--primary-rgb", "220,38,38")) + ",0.0)");
    areaPts = [];
    c.beginPath(); c.moveTo(X(0), Y(data[0]));
    for (var i = 1; i < data.length; i++) c.lineTo(X(i), Y(data[i]));
    for (i = 0; i < data.length; i++) areaPts.push({ x: X(i), y: Y(data[i]), v: data[i] });
    c.lineTo(X(data.length - 1), h - pad); c.lineTo(X(0), h - pad); c.closePath(); c.fillStyle = grad; c.fill();
    c.beginPath(); c.moveTo(X(0), Y(data[0]));
    for (i = 1; i < data.length; i++) c.lineTo(X(i), Y(data[i]));
    c.lineWidth = 2.5; c.strokeStyle = red; c.lineJoin = "round"; c.stroke();
    var last = data.length - 1;
    c.beginPath(); c.arc(X(last), Y(data[last]), 4, 0, 7); c.fillStyle = red; c.fill();
    c.strokeStyle = cssv("--surface-1", "#fff"); c.lineWidth = 2; c.stroke();
  }
  function wireAreaHover() {
    var cv = $("cc-area"), tip = $("cc-area-tip"); if (!cv || !tip || cv.__wired) return; cv.__wired = true;
    cv.addEventListener("mousemove", function (ev) {
      if (!areaPts.length) return;
      var r = cv.getBoundingClientRect(), x = ev.clientX - r.left, best = areaPts[0], bd = 1e9;
      areaPts.forEach(function (p) { var d = Math.abs(p.x - x); if (d < bd) { bd = d; best = p; } });
      tip.hidden = false; tip.innerHTML = "<b>" + fmt(best.v) + "</b> activity";
      tip.style.left = Math.min(Math.max(best.x - 30, 4), r.width - 90) + "px";
      tip.style.top = Math.max(best.y - 40, 0) + "px";
    });
    cv.addEventListener("mouseleave", function () { tip.hidden = true; });
  }

  /* ---- Severity segments ------------------------------------------------- */
  function renderSegs(counts, total) {
    var box = $("cc-segs"); if (!box) return;
    box.innerHTML = SEVS.map(function (s) {
      var v = counts[s] || 0, pct = total > 0 ? (v / total * 100) : 0;
      return '<div class="cc-seg"><div class="cc-seg-top"><span class="cc-dot" style="background:' + SEV_COLORS[s] + '"></span>' + SEV_LABEL[s] + '</div>' +
        '<div class="cc-seg-val">' + v + '</div>' +
        '<div class="cc-seg-bar"><span style="width:' + pct + '%;background:' + SEV_COLORS[s] + '"></span></div></div>';
    }).join("");
  }

  /* ---- Recent findings table -------------------------------------------- */
  function timeAgo(iso) { if (!iso) return ""; var t = new Date(iso).getTime(); if (isNaN(t)) return ""; var s = Math.max(0, (Date.now() - t) / 1000); if (s < 60) return Math.floor(s) + "s ago"; if (s < 3600) return Math.floor(s / 60) + "m ago"; if (s < 86400) return Math.floor(s / 3600) + "h ago"; return Math.floor(s / 86400) + "d ago"; }
  function renderFindings(items) {
    var body = $("cc-findings-body"); if (!body) return;
    if (!items || !items.length) { body.innerHTML = '<tr><td colspan="5" class="cc-table-empty">No findings recorded yet</td></tr>'; return; }
    body.innerHTML = items.slice(0, 6).map(function (v) {
      var sev = String(v.severity || "info").toLowerCase();
      return '<tr onclick="switchPage(\'vulnerabilities\')">' +
        '<td><span class="cc-sev-badge cc-sev-' + sev + '">' + (SEV_LABEL[sev] || "Info") + '</span></td>' +
        '<td class="cc-f-title">' + esc(v.title || "(finding)") + '</td>' +
        '<td class="cc-f-target">' + esc(v.target || "—") + '</td>' +
        '<td><span class="cc-status">' + esc(v.status || "open") + '</span></td>' +
        '<td class="ta-r cc-f-when">' + timeAgo(v.created_at || v.createdAt) + '</td></tr>';
    }).join("");
  }

  /* ---- Top tools bars ---------------------------------------------------- */
  function renderBars(monitor) {
    var box = $("cc-bars"), sub = $("cc-bars-sub"); if (!box) return;
    var entries = Object.keys(monitor || {}).map(function (k) { var v = monitor[k]; return { name: k, calls: (v && (v.totalCalls != null ? v.totalCalls : v.TotalCalls)) || 0 }; })
      .filter(function (e) { return e.calls > 0; }).sort(function (a, b) { return b.calls - a.calls; }).slice(0, 7);
    if (!entries.length) { box.innerHTML = '<div class="cc-empty">No tool calls yet</div>'; if (sub) sub.textContent = ""; return; }
    if (sub) sub.textContent = "by calls";
    var max = entries[0].calls;
    box.innerHTML = entries.map(function (e, i) {
      var pct = max > 0 ? Math.max(6, e.calls / max * 100) : 6;
      var short = e.name.length > 7 ? e.name.slice(0, 6) + "…" : e.name;
      return '<div class="cc-bar' + (i === 0 ? " is-top" : "") + '" title="' + esc(e.name) + ' · ' + e.calls + '">' +
        '<span class="cc-bar-val">' + e.calls + '</span>' +
        '<div class="cc-bar-track" style="height:100%"><div class="cc-bar-fill" style="height:' + pct + '%"></div></div>' +
        '<span class="cc-bar-label">' + esc(short) + '</span></div>';
    }).join("");
  }

  /* ---- Success gauge (semicircular) ------------------------------------- */
  function drawGauge(pct, calls) {
    var cv = $("cc-gauge"); if (!cv) return;
    var dpr = window.devicePixelRatio || 1, W = 220, H = 130;
    cv.width = W * dpr; cv.height = H * dpr; cv.style.width = W + "px"; cv.style.height = H + "px";
    var x = cv.getContext("2d"); if (!x) return; x.setTransform(dpr, 0, 0, dpr, 0, 0);
    var cx = W / 2, cy = H - 8, r = 92, lw = 14, target = Math.max(0, Math.min(100, pct || 0)) / 100;
    function render(p) {
      x.clearRect(0, 0, W, H); x.lineCap = "round";
      // track
      x.lineWidth = lw; x.beginPath(); x.arc(cx, cy, r, Math.PI, Math.PI * 2); x.strokeStyle = cssv("--surface-3", "#eee"); x.stroke();
      if (calls <= 0) return;
      var g = x.createLinearGradient(cx - r, 0, cx + r, 0);
      g.addColorStop(0, "#16a34a"); g.addColorStop(1, cssv("--primary", "#dc2626"));
      x.beginPath(); x.arc(cx, cy, r, Math.PI, Math.PI + Math.PI * target * p); x.strokeStyle = g; x.stroke();
    }
    if (REDUCED) { render(1); } else { var s = performance.now(); (function step(now) { var pr = Math.min(1, (now - s) / 850), e = 1 - Math.pow(1 - pr, 3); render(e); if (pr < 1) requestAnimationFrame(step); })(s); }
  }

  /* ---- Engine (feeds assistant subtitle only; no strip now) ------------- */

  /* ---- Main refresh ------------------------------------------------------ */
  async function refreshDashboard() {
    if (typeof apiFetch === "undefined") return;
    markLive();
    try {
      var res = await Promise.all([
        apiFetch("/api/agent-loop/tasks").then(function (r) { return r.ok ? r.json() : null; }).catch(function () { return null; }),
        apiFetch("/api/vulnerabilities/stats").then(function (r) { return r.ok ? r.json() : null; }).catch(function () { return null; }),
        apiFetch("/api/batch-tasks?limit=500&page=1").then(function (r) { return r.ok ? r.json() : null; }).catch(function () { return null; }),
        apiFetch("/api/monitor/stats").then(function (r) { return r.ok ? r.json() : null; }).catch(function () { return null; }),
        apiFetch("/api/vulnerabilities?limit=8&page=1").then(function (r) { return r.ok ? r.json() : null; }).catch(function () { return null; })
      ]);
      var tasks = res[0], vuln = res[1], batch = res[2], monitor = res[3], vulnList = res[4];

      var running = (tasks && Array.isArray(tasks.tasks)) ? tasks.tasks.length : 0;
      if (batch && Array.isArray(batch.queues)) batch.queues.forEach(function (q) { if ((q.status || "").toLowerCase() === "running") running++; });
      countUp($("cc-running"), running); setDelta("cc-running-d", running, "running");

      var counts = {}, vtotal = (vuln && typeof vuln.total === "number") ? vuln.total : 0, bySev = (vuln && vuln.by_severity) || {};
      SEVS.forEach(function (s) { counts[s] = bySev[s] || 0; });
      countUp($("cc-vulns"), vtotal); setDelta("cc-vulns-d", vtotal, "vulns");
      renderSegs(counts, vtotal);

      var totalCalls = 0, ok = 0, fail = 0;
      if (monitor && typeof monitor === "object") Object.keys(monitor).forEach(function (k) {
        var v = monitor[k];
        totalCalls += (v && (v.totalCalls != null ? v.totalCalls : v.TotalCalls)) || 0;
        ok += (v && (v.successCalls != null ? v.successCalls : v.SuccessCalls)) || 0;
        fail += (v && (v.failedCalls != null ? v.failedCalls : v.FailedCalls)) || 0;
      });
      countUp($("cc-toolcalls"), totalCalls); setDelta("cc-toolcalls-d", totalCalls, "toolcalls");
      var rate = totalCalls > 0 ? (ok / totalCalls * 100) : 0;
      var sEl = $("cc-success"); if (sEl) { sEl.textContent = totalCalls > 0 ? rate.toFixed(1) + "%" : "–"; }
      setDelta("cc-success-d", Math.round(rate), "success");
      if ($("cc-gauge-val")) $("cc-gauge-val").textContent = totalCalls > 0 ? Math.round(rate) + "%" : "–";
      if ($("cc-gauge-calls")) $("cc-gauge-calls").textContent = fmt(totalCalls);
      if ($("cc-gauge-ok")) $("cc-gauge-ok").textContent = fmt(ok);
      if ($("cc-gauge-fail")) $("cc-gauge-fail").textContent = fmt(fail);
      drawGauge(rate, totalCalls);
      renderBars(monitor);

      // trend = session activity (tool calls + findings)
      trend.push(totalCalls + vtotal); if (trend.length > 40) trend.shift();
      if ($("cc-chart-total")) countUp($("cc-chart-total"), totalCalls + vtotal);
      setDelta("cc-chart-delta", totalCalls + vtotal, "trend");
      drawArea(); wireAreaHover();

      renderFindings(vulnList && (vulnList.vulnerabilities || []) || []);
    } catch (e) { /* keep prior */ }
  }

  function markLive() { var t = $("cc-live-time"); if (t) t.textContent = new Date().toLocaleTimeString(); }
  function dashActive() { var p = $("page-dashboard"); return p && p.classList.contains("active"); }
  function startAuto() { if (autoTimer) return; autoTimer = setInterval(function () { if (dashActive() && !document.hidden) refreshDashboard(); }, 15000); }

  // Assistant → route question into chat
  window.ccAsk = function (ev) {
    if (ev && ev.preventDefault) ev.preventDefault();
    var inp = $("cc-ask"); var q = inp ? inp.value.trim() : "";
    if (typeof switchPage === "function") switchPage("chat");
    setTimeout(function () {
      var box = document.querySelector("#page-chat textarea, #page-chat input[type=text]");
      if (box && q) { box.value = q; box.focus(); box.dispatchEvent(new Event("input", { bubbles: true })); }
      else if (box) { box.focus(); }
    }, 180);
    if (inp) inp.value = "";
  };

  // Export report → authenticated download
  window.ccExportReport = async function () {
    if (typeof apiFetch === "undefined") return;
    try {
      var r = await apiFetch("/api/reports/generate?format=md");
      var text = await r.text();
      var blob = new Blob([text], { type: "text/markdown" });
      var url = URL.createObjectURL(blob), a = document.createElement("a");
      a.href = url; a.download = "pyntra-report.md"; document.body.appendChild(a); a.click();
      setTimeout(function () { URL.revokeObjectURL(url); a.remove(); }, 1000);
      if (window.pyToast) window.pyToast("Report downloaded", { type: "success" });
    } catch (e) { if (window.pyToast) window.pyToast("Export failed: " + e.message, { type: "danger" }); }
  };

  new MutationObserver(function () { if (dashActive()) setTimeout(function () { drawArea(); }, 60); })
    .observe(document.documentElement, { attributes: true, attributeFilter: ["data-theme"] });
  window.addEventListener("resize", function () { if (dashActive()) drawArea(); });

  window.refreshDashboard = refreshDashboard;
  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", startAuto); else startAuto();
})();
