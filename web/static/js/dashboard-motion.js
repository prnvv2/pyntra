/* ============================================================================
   Pyntra Command Center — Motion layer (motion.dev / Motion One, vendored)
   Subtle spring entrance + stagger for KPI cards and panels. Degrades
   gracefully without the library or under reduced-motion.
   ============================================================================ */
(function () {
  "use strict";
  if (window.__pyntraDashMotion) return;
  window.__pyntraDashMotion = true;

  var M = window.Motion || null;
  var REDUCED = window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  var spring = (M && M.spring) ? M.spring({ stiffness: 300, damping: 30 }) : undefined;

  function animate(el, keyframes, opts) {
    if (!el) return;
    if (!M || !M.animate || REDUCED) { el.style.opacity = "1"; el.style.transform = "none"; return; }
    try { M.animate(el, keyframes, opts); } catch (e) { el.style.opacity = "1"; }
  }

  function runEntrance() {
    var page = document.getElementById("page-dashboard");
    if (!page) return;
    var groups = [page.querySelectorAll(".cc-kpi"), page.querySelectorAll(".cc-card")];
    var base = 0;
    groups.forEach(function (list) {
      list.forEach(function (el, i) {
        el.style.opacity = "0";
        animate(el,
          { opacity: [0, 1], transform: ["translateY(14px)", "translateY(0)"] },
          { duration: 0.5, delay: base + i * 0.06, easing: spring || [0.2, 0, 0, 1] });
      });
      base += 0.1;
    });
  }

  function onActive() {
    var page = document.getElementById("page-dashboard");
    if (page && page.classList.contains("active")) runEntrance();
  }

  function boot() {
    var page = document.getElementById("page-dashboard");
    if (!page) return;
    new MutationObserver(function () { if (page.classList.contains("active")) onActive(); })
      .observe(page, { attributes: true, attributeFilter: ["class"] });
    if (page.classList.contains("active")) onActive();
  }

  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", boot);
  else boot();
})();
