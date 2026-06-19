// app.js - client glue for Splitwise-QUIC.
// Handles: starfield, theme toggle, auth tabs, protocol badge, split-type UX,
// toasts, and the live WebTransport (QUIC DATAGRAM) channel.

(function () {
  "use strict";

  // --- Starfield ----------------------------------------------------------
  function buildStarfield() {
    const field = document.getElementById("starfield");
    if (!field) return;
    const count = Math.min(160, Math.floor((window.innerWidth * window.innerHeight) / 9000));
    const frag = document.createDocumentFragment();
    for (let i = 0; i < count; i++) {
      const s = document.createElement("span");
      s.className = "star";
      const size = Math.random() * 2 + 1;
      s.style.width = size + "px";
      s.style.height = size + "px";
      s.style.left = Math.random() * 100 + "%";
      s.style.top = Math.random() * 100 + "%";
      s.style.setProperty("--dur", (Math.random() * 3 + 2).toFixed(2) + "s");
      s.style.setProperty("--delay", (Math.random() * 4).toFixed(2) + "s");
      s.style.setProperty("--max", (Math.random() * 0.6 + 0.4).toFixed(2));
      frag.appendChild(s);
    }
    // A couple of shooting stars (only visible in dark mode via CSS).
    for (let i = 0; i < 2; i++) {
      const sh = document.createElement("span");
      sh.className = "shooting-star";
      sh.style.left = Math.random() * 60 + "%";
      sh.style.top = Math.random() * 40 + "%";
      sh.style.setProperty("--sdur", (Math.random() * 4 + 6).toFixed(2) + "s");
      sh.style.setProperty("--sdelay", (Math.random() * 8 + i * 4).toFixed(2) + "s");
      frag.appendChild(sh);
    }
    field.appendChild(frag);
  }

  // --- Theme toggle -------------------------------------------------------
  function wireThemeToggle() {
    const btn = document.getElementById("theme-toggle");
    if (!btn) return;
    btn.addEventListener("click", function () {
      const root = document.documentElement;
      const isDark = root.classList.toggle("dark");
      try {
        localStorage.setItem("theme", isDark ? "dark" : "light");
      } catch (e) {}
    });
  }

  // --- Auth tabs (login / signup) ----------------------------------------
  function wireAuthTabs() {
    const tabs = document.querySelectorAll(".auth-tab");
    if (!tabs.length) return;
    const panes = document.querySelectorAll("[data-pane]");
    tabs.forEach((tab) => {
      tab.addEventListener("click", function () {
        const target = tab.dataset.tab;
        tabs.forEach((t) => {
          const active = t.dataset.tab === target;
          t.classList.toggle("border-emerald-500", active);
          t.classList.toggle("text-emerald-600", active);
          t.classList.toggle("dark:text-emerald-400", active);
          t.classList.toggle("border-transparent", !active);
          t.classList.toggle("text-slate-400", !active);
        });
        panes.forEach((p) => p.classList.toggle("hidden", p.dataset.pane !== target));
      });
    });
  }

  // --- Protocol badge -----------------------------------------------------
  function showProtocol() {
    const badge = document.getElementById("proto-badge");
    if (!badge) return;
    try {
      const nav = performance.getEntriesByType("navigation")[0];
      const proto = (nav && nav.nextHopProtocol) || "?";
      badge.textContent = "proto: " + proto;
      if (proto === "h3") {
        badge.classList.add("!bg-emerald-500/20", "!text-emerald-700", "dark:!text-emerald-300");
      }
    } catch (e) {
      badge.textContent = "proto: ?";
    }
  }

  // --- Split-type form UX -------------------------------------------------
  const HINTS = {
    equal: "Everyone splits equally - just tick who's in.",
    exact: "Enter the exact amount each person owes (must sum to the total).",
    percentage: "Enter each person's percentage (must sum to 100).",
    shares: "Enter weighted shares (e.g. 2 vs 1 = two-thirds / one-third).",
  };

  function wireSplitForm() {
    const sel = document.getElementById("split-type");
    if (!sel) return;
    const hint = document.getElementById("split-hint");
    const values = document.querySelectorAll(".split-value");
    function update() {
      const t = sel.value;
      if (hint) hint.textContent = HINTS[t] || "";
      values.forEach((v) => {
        v.style.display = t === "equal" ? "none" : "";
      });
    }
    sel.addEventListener("change", update);
    update();
  }

  // --- Toast --------------------------------------------------------------
  let toastTimer = null;
  function toast(msg) {
    const el = document.getElementById("toast");
    if (!el) return;
    el.textContent = msg;
    el.classList.remove("opacity-0", "translate-y-2");
    el.classList.add("opacity-100", "translate-y-0");
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => {
      el.classList.add("opacity-0", "translate-y-2");
      el.classList.remove("opacity-100", "translate-y-0");
    }, 3000);
  }

  // --- Live channel via WebTransport (QUIC datagrams) ---------------------
  function b64ToBytes(b64) {
    const bin = atob(b64);
    const out = new Uint8Array(bin.length);
    for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
    return out;
  }

  function groupIdFromPath() {
    const m = location.pathname.match(/^\/g\/([^/]+)/);
    return m ? m[1] : null;
  }

  function setLive(ok, text) {
    const dot = document.getElementById("live-dot");
    const label = document.getElementById("live-text");
    if (dot) dot.className =
      "inline-block h-2 w-2 rounded-full " +
      (ok ? "bg-emerald-500 animate-pulse" : "bg-slate-300 dark:bg-slate-600");
    if (label) {
      label.textContent = text;
      label.className = ok ? "font-medium text-emerald-600 dark:text-emerald-400" : "muted";
    }
  }

  function refreshLiveRegions() {
    document.querySelectorAll("[hx-trigger~='sse:update']").forEach((el) => {
      if (window.htmx) window.htmx.trigger(el, "sse:update");
    });
  }

  async function connectWebTransport() {
    const gid = groupIdFromPath();
    if (!gid) return;
    if (typeof WebTransport === "undefined") {
      setLive(false, "live via SSE");
      return;
    }
    const hashMeta = document.querySelector('meta[name="cert-hash"]');
    const hashB64 = hashMeta && hashMeta.content;
    const url = location.origin + "/g/" + gid + "/wt";
    try {
      const opts = {};
      if (hashB64) {
        opts.serverCertificateHashes = [
          { algorithm: "sha-256", value: b64ToBytes(hashB64) },
        ];
      }
      const wt = new WebTransport(url, opts);
      await wt.ready;
      setLive(true, "live over QUIC");

      const reader = wt.datagrams.readable.getReader();
      const dec = new TextDecoder();
      (async () => {
        for (;;) {
          const { value, done } = await reader.read();
          if (done) break;
          const msg = dec.decode(value);
          toast(msg || "Group updated");
          refreshLiveRegions();
        }
      })();

      wt.closed.catch(() => {}).finally(() => {
        setLive(false, "reconnecting…");
        setTimeout(connectWebTransport, 2000);
      });
    } catch (e) {
      setLive(false, "live via SSE");
    }
  }

  document.addEventListener("DOMContentLoaded", function () {
    buildStarfield();
    wireThemeToggle();
    wireAuthTabs();
    showProtocol();
    wireSplitForm();
    connectWebTransport();
  });
})();
