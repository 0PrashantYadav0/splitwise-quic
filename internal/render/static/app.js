// app.js — client glue for Splitwise-QUIC.
// Handles: starfield, mobile tabs, auth tabs, protocol badge,
//          split-type UX, toasts, and the live WebTransport channel.

(function () {
  "use strict";

  // --- Starfield ----------------------------------------------------------
  function buildStarfield() {
    const field = document.getElementById("starfield");
    if (!field) return;
    const count = Math.min(180, Math.floor((window.innerWidth * window.innerHeight) / 8000));
    const frag = document.createDocumentFragment();
    for (let i = 0; i < count; i++) {
      const s = document.createElement("span");
      s.className = "star";
      const size = Math.random() * 2 + 0.8;
      s.style.width  = size + "px";
      s.style.height = size + "px";
      s.style.left   = Math.random() * 100 + "%";
      s.style.top    = Math.random() * 100 + "%";
      s.style.setProperty("--dur",   (Math.random() * 3 + 2).toFixed(2) + "s");
      s.style.setProperty("--delay", (Math.random() * 5).toFixed(2) + "s");
      s.style.setProperty("--max",   (Math.random() * 0.55 + 0.35).toFixed(2));
      frag.appendChild(s);
    }
    for (let i = 0; i < 3; i++) {
      const sh = document.createElement("span");
      sh.className = "shooting-star";
      sh.style.left = Math.random() * 65 + "%";
      sh.style.top  = Math.random() * 45 + "%";
      sh.style.setProperty("--sdur",   (Math.random() * 4 + 6).toFixed(2) + "s");
      sh.style.setProperty("--sdelay", (Math.random() * 10 + i * 5).toFixed(2) + "s");
      frag.appendChild(sh);
    }
    field.appendChild(frag);
  }

  // --- Mobile section tabs (group page) ------------------------------------
  // Tabs control #grp-main-col / #grp-side-col visibility.
  // On sm+ (≥640px) both columns are visible — we clear any inline styles.
  var SM_BREAKPOINT = 640;

  function wireMobileTabs() {
    var tabs = document.querySelectorAll(".mobile-tab");
    if (!tabs.length) return;

    function getColIds() {
      var ids = [];
      tabs.forEach(function (t) { ids.push(t.dataset.col); });
      return ids;
    }

    function setActiveTab(activeTab) {
      tabs.forEach(function (t) {
        var on = t === activeTab;
        t.classList.toggle("bg-white/10", on);
        t.classList.toggle("text-white",  on);
        t.classList.toggle("text-slate-500", !on);
        t.setAttribute("aria-selected", on ? "true" : "false");
      });
    }

    function applyMobileVisibility(activeColId) {
      getColIds().forEach(function (id) {
        var el = document.getElementById(id);
        if (el) el.style.display = (id === activeColId) ? "" : "none";
      });
    }

    function clearMobileVisibility() {
      getColIds().forEach(function (id) {
        var el = document.getElementById(id);
        if (el) el.style.display = "";
      });
    }

    function isMobile() { return window.innerWidth < SM_BREAKPOINT; }

    // Initial state
    if (isMobile()) {
      var firstColId = tabs[0] && tabs[0].dataset.col;
      if (firstColId) applyMobileVisibility(firstColId);
      setActiveTab(tabs[0]);
    }

    tabs.forEach(function (tab) {
      tab.addEventListener("click", function () {
        if (!isMobile()) return;
        setActiveTab(tab);
        applyMobileVisibility(tab.dataset.col);
        // Scroll to the top of content area on tab switch
        var col = document.getElementById(tab.dataset.col);
        if (col) col.scrollIntoView({ behavior: "smooth", block: "nearest" });
      });
    });

    // On resize: clear inline styles when crossing to desktop, re-apply on back to mobile
    var lastMobile = isMobile();
    window.addEventListener("resize", function () {
      var nowMobile = isMobile();
      if (lastMobile && !nowMobile) {
        clearMobileVisibility();
      } else if (!lastMobile && nowMobile) {
        var activeTab = document.querySelector(".mobile-tab.text-white") || tabs[0];
        if (activeTab) {
          setActiveTab(activeTab);
          applyMobileVisibility(activeTab.dataset.col);
        }
      }
      lastMobile = nowMobile;
    });
  }

  // --- Auth tabs (login / signup) -----------------------------------------
  function wireAuthTabs() {
    var tabs = document.querySelectorAll(".auth-tab");
    if (!tabs.length) return;
    var panes = document.querySelectorAll("[data-pane]");
    tabs.forEach(function (tab) {
      tab.addEventListener("click", function () {
        var target = tab.dataset.tab;
        tabs.forEach(function (t) {
          var active = t.dataset.tab === target;
          t.classList.toggle("border-emerald-500",   active);
          t.classList.toggle("text-emerald-400",      active);
          t.classList.toggle("border-transparent",   !active);
          t.classList.toggle("text-slate-500",        !active);
        });
        panes.forEach(function (p) {
          p.classList.toggle("hidden", p.dataset.pane !== target);
        });
      });
    });
  }

  // --- Protocol badge -----------------------------------------------------
  function showProtocol() {
    var badge = document.getElementById("proto-badge");
    if (!badge) return;
    try {
      var nav   = performance.getEntriesByType("navigation")[0];
      var proto = (nav && nav.nextHopProtocol) || "?";
      badge.textContent = "proto: " + proto;
      if (proto === "h3") {
        badge.classList.add("!border-emerald-500/30", "!bg-emerald-500/10", "!text-emerald-400");
      }
    } catch (e) {
      badge.textContent = "proto: ?";
    }
  }

  // --- Split-type form UX -------------------------------------------------
  var HINTS = {
    equal:      "Everyone splits equally — just tick who's in.",
    exact:      "Enter the exact amount each person owes (must sum to the total).",
    percentage: "Enter each person's percentage (must sum to 100).",
    shares:     "Enter weighted shares (e.g. 2 vs 1 = two-thirds / one-third).",
  };

  function wireSplitForm() {
    var sel = document.getElementById("split-type");
    if (!sel) return;
    var hint   = document.getElementById("split-hint");
    var values = document.querySelectorAll(".split-value");
    function update() {
      var t = sel.value;
      if (hint) hint.textContent = HINTS[t] || "";
      values.forEach(function (v) {
        v.style.display = (t === "equal") ? "none" : "";
      });
    }
    sel.addEventListener("change", update);
    update();
  }

  // --- Toast --------------------------------------------------------------
  var toastTimer = null;
  function toast(msg) {
    var el = document.getElementById("toast");
    if (!el) return;
    el.textContent = msg;
    el.classList.remove("opacity-0", "translate-y-2");
    el.classList.add("opacity-100", "translate-y-0");
    clearTimeout(toastTimer);
    toastTimer = setTimeout(function () {
      el.classList.add("opacity-0", "translate-y-2");
      el.classList.remove("opacity-100", "translate-y-0");
    }, 3500);
  }

  // --- Live channel via WebTransport (QUIC datagrams) ---------------------
  function b64ToBytes(b64) {
    var bin = atob(b64);
    var out = new Uint8Array(bin.length);
    for (var i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
    return out;
  }

  function groupIdFromPath() {
    var m = location.pathname.match(/^\/g\/([^/]+)/);
    return m ? m[1] : null;
  }

  function setLive(ok, text) {
    var dot   = document.getElementById("live-dot");
    var label = document.getElementById("live-text");
    if (dot) {
      dot.className =
        "inline-block h-2 w-2 rounded-full " +
        (ok ? "bg-emerald-500 animate-pulse" : "bg-slate-600");
    }
    if (label) {
      label.textContent = text;
      label.className   = ok
        ? "text-xs font-medium text-emerald-400"
        : "text-xs text-slate-500";
    }
  }

  function refreshLiveRegions() {
    document.querySelectorAll("[hx-trigger~='sse:update']").forEach(function (el) {
      if (window.htmx) window.htmx.trigger(el, "sse:update");
    });
  }

  var GREETINGS = new Set([
    "live channel connected",
    "notifications connected",
  ]);

  function certHashOpts() {
    var meta = document.querySelector('meta[name="cert-hash"]');
    var b64  = meta && meta.content;
    if (!b64) return {};
    return { serverCertificateHashes: [{ algorithm: "sha-256", value: b64ToBytes(b64) }] };
  }

  async function connectUserNotifications() {
    if (typeof WebTransport === "undefined") return;
    try {
      var wt = new WebTransport(location.origin + "/wt", certHashOpts());
      await wt.ready;
      var reader = wt.datagrams.readable.getReader();
      var dec    = new TextDecoder();
      (async function () {
        for (;;) {
          var { value, done } = await reader.read();
          if (done) break;
          var msg = dec.decode(value);
          if (!GREETINGS.has(msg)) toast(msg);
          refreshLiveRegions();
        }
      })();
      wt.closed.catch(function () {}).finally(function () {
        setTimeout(connectUserNotifications, 3000);
      });
    } catch (e) {
      // No WebTransport / not logged in: silent.
    }
  }

  async function connectWebTransport() {
    var gid = groupIdFromPath();
    if (!gid) return;
    if (typeof WebTransport === "undefined") {
      setLive(false, "live via SSE");
      return;
    }
    var url = location.origin + "/g/" + gid + "/wt";
    try {
      var wt = new WebTransport(url, certHashOpts());
      await wt.ready;
      setLive(true, "live over QUIC");

      var reader = wt.datagrams.readable.getReader();
      var dec    = new TextDecoder();
      (async function () {
        for (;;) {
          var { value, done } = await reader.read();
          if (done) break;
          var msg = dec.decode(value);
          if (!GREETINGS.has(msg)) {
            toast(msg || "Group updated");
            refreshLiveRegions();
          }
        }
      })();

      wt.closed.catch(function () {}).finally(function () {
        setLive(false, "reconnecting…");
        setTimeout(connectWebTransport, 2000);
      });
    } catch (e) {
      setLive(false, "live via SSE");
    }
  }

  document.addEventListener("DOMContentLoaded", function () {
    buildStarfield();
    wireMobileTabs();
    wireAuthTabs();
    showProtocol();
    wireSplitForm();
    connectWebTransport();
    connectUserNotifications();

    // Re-wire split-type UX after htmx swaps in the edit form.
    document.body.addEventListener("htmx:afterSwap", function () {
      wireSplitForm();
    });
  });
})();
