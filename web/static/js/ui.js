/* ============================================================================
   Pyntra UI helpers (T1-P4)
   Self-contained, token-styled replacements for native alert()/confirm(), plus
   a ⌘K / Ctrl-K command palette for fast page navigation. No dependencies; all
   DOM is created lazily and cleaned up. Exposes window.pyToast, window.pyConfirm,
   window.pyCommandPalette.
   ============================================================================ */
(function () {
  "use strict";
  if (window.__pyntraUI) return;
  window.__pyntraUI = true;

  function t(key, fallback) {
    try { return (typeof window.t === 'function') ? window.t(key) : fallback; }
    catch (e) { return fallback; }
  }
  function esc(s) {
    return String(s == null ? '' : s)
      .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
  }

  /* ---- Toasts ------------------------------------------------------------ */
  var toastHost = null;
  function ensureToastHost() {
    if (toastHost) return toastHost;
    toastHost = document.createElement('div');
    toastHost.className = 'py-toast-host';
    toastHost.setAttribute('role', 'region');
    toastHost.setAttribute('aria-live', 'polite');
    document.body.appendChild(toastHost);
    return toastHost;
  }
  function pyToast(message, opts) {
    opts = opts || {};
    var type = opts.type || 'info';           // info | success | warning | danger
    var duration = opts.duration != null ? opts.duration : 3800;
    var host = ensureToastHost();
    var el = document.createElement('div');
    el.className = 'py-toast py-toast-' + type;
    el.setAttribute('role', type === 'danger' ? 'alert' : 'status');
    el.innerHTML = '<span class="py-toast-msg">' + esc(message) + '</span>'
      + '<button type="button" class="py-toast-close" aria-label="Dismiss">×</button>';
    host.appendChild(el);
    requestAnimationFrame(function () { el.classList.add('is-in'); });
    var timer = null;
    function dismiss() {
      if (timer) clearTimeout(timer);
      el.classList.remove('is-in');
      setTimeout(function () { if (el.parentNode) el.parentNode.removeChild(el); }, 220);
    }
    el.querySelector('.py-toast-close').addEventListener('click', dismiss);
    if (duration > 0) timer = setTimeout(dismiss, duration);
    return dismiss;
  }

  /* ---- Confirm / alert modal -------------------------------------------- */
  function pyModal(opts) {
    opts = opts || {};
    return new Promise(function (resolve) {
      var scrim = document.createElement('div');
      scrim.className = 'py-modal-scrim';
      var danger = !!opts.danger;
      var confirmLabel = opts.confirmLabel || (opts.alert ? t('common.ok', 'OK') : t('common.confirm', 'Confirm'));
      var cancelLabel = opts.cancelLabel || t('common.cancel', 'Cancel');
      scrim.innerHTML =
        '<div class="py-modal" role="dialog" aria-modal="true">' +
          (opts.title ? '<div class="py-modal-title">' + esc(opts.title) + '</div>' : '') +
          '<div class="py-modal-body">' + esc(opts.message || '') + '</div>' +
          '<div class="py-modal-actions">' +
            (opts.alert ? '' : '<button type="button" class="btn-secondary btn-small py-modal-cancel">' + esc(cancelLabel) + '</button>') +
            '<button type="button" class="btn-small ' + (danger ? 'btn-danger' : 'btn-primary') + ' py-modal-ok">' + esc(confirmLabel) + '</button>' +
          '</div>' +
        '</div>';
      document.body.appendChild(scrim);
      var okBtn = scrim.querySelector('.py-modal-ok');
      var cancelBtn = scrim.querySelector('.py-modal-cancel');
      function close(val) {
        document.removeEventListener('keydown', onKey);
        if (scrim.parentNode) scrim.parentNode.removeChild(scrim);
        resolve(val);
      }
      function onKey(e) {
        if (e.key === 'Escape') { close(false); }
        else if (e.key === 'Enter') { close(true); }
      }
      okBtn.addEventListener('click', function () { close(true); });
      if (cancelBtn) cancelBtn.addEventListener('click', function () { close(false); });
      scrim.addEventListener('click', function (e) { if (e.target === scrim && !opts.alert) close(false); });
      document.addEventListener('keydown', onKey);
      requestAnimationFrame(function () { scrim.classList.add('is-in'); okBtn.focus(); });
    });
  }
  function pyConfirm(message, opts) { opts = opts || {}; opts.message = message; return pyModal(opts); }
  function pyAlert(message, opts) { opts = opts || {}; opts.message = message; opts.alert = true; return pyModal(opts); }

  /* ---- Command palette (⌘K) --------------------------------------------- */
  var paletteEl = null, paletteInput = null, paletteResults = null, paletteItems = [], paletteActive = 0;
  function collectCommands() {
    var cmds = [];
    document.querySelectorAll('.nav-item-content[onclick], .nav-submenu-item[onclick]').forEach(function (n) {
      var label = (n.getAttribute('data-title') || n.textContent || '').trim();
      var oc = n.getAttribute('onclick') || '';
      var m = oc.match(/switchPage\(['"]([^'"]+)['"]\)/);
      if (label && m) cmds.push({ label: label, page: m[1] });
    });
    // de-dup by page
    var seen = {}, out = [];
    cmds.forEach(function (c) { if (!seen[c.page]) { seen[c.page] = 1; out.push(c); } });
    return out;
  }
  function ensurePalette() {
    if (paletteEl) return;
    paletteEl = document.createElement('div');
    paletteEl.className = 'py-palette-scrim';
    paletteEl.innerHTML =
      '<div class="py-palette" role="dialog" aria-modal="true" aria-label="Command palette">' +
        '<input type="text" class="py-palette-input" placeholder="' + esc(t('palette.placeholder', 'Jump to…')) + '" aria-label="Search pages" />' +
        '<div class="py-palette-results" role="listbox"></div>' +
      '</div>';
    document.body.appendChild(paletteEl);
    paletteInput = paletteEl.querySelector('.py-palette-input');
    paletteResults = paletteEl.querySelector('.py-palette-results');
    paletteInput.addEventListener('input', renderPalette);
    paletteInput.addEventListener('keydown', paletteKey);
    paletteEl.addEventListener('click', function (e) { if (e.target === paletteEl) closePalette(); });
  }
  function renderPalette() {
    var q = (paletteInput.value || '').toLowerCase().trim();
    var all = collectCommands();
    paletteItems = q ? all.filter(function (c) { return c.label.toLowerCase().indexOf(q) > -1; }) : all;
    paletteActive = 0;
    paletteResults.innerHTML = paletteItems.map(function (c, i) {
      return '<div class="py-palette-item' + (i === 0 ? ' is-active' : '') + '" role="option" data-i="' + i + '">' + esc(c.label) + '</div>';
    }).join('') || '<div class="py-palette-empty">' + esc(t('palette.noResults', 'No matches')) + '</div>';
    Array.prototype.forEach.call(paletteResults.querySelectorAll('.py-palette-item'), function (el) {
      el.addEventListener('click', function () { activatePalette(parseInt(el.getAttribute('data-i'), 10)); });
    });
  }
  function moveActive(d) {
    if (!paletteItems.length) return;
    paletteActive = (paletteActive + d + paletteItems.length) % paletteItems.length;
    Array.prototype.forEach.call(paletteResults.querySelectorAll('.py-palette-item'), function (el, i) {
      el.classList.toggle('is-active', i === paletteActive);
      if (i === paletteActive) el.scrollIntoView({ block: 'nearest' });
    });
  }
  function paletteKey(e) {
    if (e.key === 'ArrowDown') { e.preventDefault(); moveActive(1); }
    else if (e.key === 'ArrowUp') { e.preventDefault(); moveActive(-1); }
    else if (e.key === 'Enter') { e.preventDefault(); activatePalette(paletteActive); }
    else if (e.key === 'Escape') { e.preventDefault(); closePalette(); }
  }
  function activatePalette(i) {
    var cmd = paletteItems[i];
    closePalette();
    if (cmd && typeof window.switchPage === 'function') window.switchPage(cmd.page);
  }
  function openPalette() {
    ensurePalette();
    paletteInput.value = '';
    renderPalette();
    paletteEl.classList.add('is-in');
    requestAnimationFrame(function () { paletteInput.focus(); });
  }
  function closePalette() { if (paletteEl) paletteEl.classList.remove('is-in'); }

  document.addEventListener('keydown', function (e) {
    if ((e.metaKey || e.ctrlKey) && (e.key === 'k' || e.key === 'K')) {
      e.preventDefault();
      if (paletteEl && paletteEl.classList.contains('is-in')) closePalette(); else openPalette();
    }
  });

  window.pyToast = pyToast;
  window.pyConfirm = pyConfirm;
  window.pyAlert = pyAlert;
  window.pyCommandPalette = openPalette;
})();

/* ---- Kill-switch (T3-D): halt/resume all agent tool execution ----------- */
async function toggleKillSwitch() {
  const btn = document.getElementById('killswitch-btn');
  if (!btn || typeof apiFetch === 'undefined') return;
  const pressed = btn.getAttribute('aria-pressed') === 'true';
  const action = pressed ? 'resume' : 'halt';
  if (action === 'halt' && typeof window.pyConfirm === 'function') {
    const ok = await window.pyConfirm('Halt ALL agent tool execution now? Running and future tool calls will be blocked until you resume.', { title: 'Engage kill-switch', danger: true, confirmLabel: 'Halt' });
    if (!ok) return;
  }
  try {
    const j = await apiFetch('/api/agent/' + action, { method: 'POST' }).then(r => r.json());
    setKillSwitchState(!!j.halted);
    if (window.pyToast) window.pyToast(j.halted ? 'Agent execution halted' : 'Agent execution resumed', { type: j.halted ? 'warning' : 'success' });
  } catch (e) {
    if (window.pyToast) window.pyToast('Kill-switch failed: ' + e.message, { type: 'danger' });
  }
}
function setKillSwitchState(halted) {
  const btn = document.getElementById('killswitch-btn');
  if (!btn) return;
  btn.setAttribute('aria-pressed', halted ? 'true' : 'false');
  btn.classList.toggle('is-active', halted);
  const label = document.getElementById('killswitch-label');
  if (label) label.textContent = halted ? 'Resume' : 'Halt';
}
async function refreshKillSwitch() {
  if (typeof apiFetch === 'undefined') return;
  try { const j = await apiFetch('/api/agent/halt-status').then(r => r.json()); setKillSwitchState(!!j.halted); } catch (e) { /* ignore */ }
}
window.toggleKillSwitch = toggleKillSwitch;
document.addEventListener('DOMContentLoaded', function () { setTimeout(refreshKillSwitch, 800); });

/* ---- Login SSO placeholder --------------------------------------------- */
window.pyntraSSO = function () {
  var msg = "Single sign-on isn't configured on this instance. Sign in with your access password.";
  if (window.pyToast) window.pyToast(msg, { type: "info", duration: 5000 });
  else alert(msg);
  var p = document.getElementById("login-password"); if (p) p.focus();
};
