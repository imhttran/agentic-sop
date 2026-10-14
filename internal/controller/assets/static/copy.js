// Copy-to-clipboard for diagnostic actions (Copy error / Copy details).
//
// The server puts the exact text to copy in a `data-copy` attribute. That text is
// assembled from SOP-reported, non-sensitive fields only — never from secrets, tokens,
// cookies, or raw logs — so the browser copies exactly what the server authored and
// nothing more. This handler uses the async Clipboard API when it is available, falls
// back to a hidden textarea with document.execCommand, and reports success or failure
// as accessible status text in the enclosing .copy-control. It adds no framework and no
// dependency.
(function () {
  "use strict";

  function statusEl(btn) {
    var control = btn.closest(".copy-control");
    return control ? control.querySelector("[data-copy-status]") : null;
  }

  function feedback(btn, message, ok) {
    var status = statusEl(btn);
    if (!status) return;
    status.textContent = message;
    status.classList.toggle("copy-failed", !ok);
    if (status._copyTimer) clearTimeout(status._copyTimer);
    status._copyTimer = setTimeout(function () {
      status.textContent = "";
      status.classList.remove("copy-failed");
    }, 2000);
  }

  // fallbackCopy is the pre-Clipboard-API path: a temporary, off-screen textarea plus
  // execCommand("copy"). It works without permissions and is removed immediately.
  function fallbackCopy(text) {
    var ta = document.createElement("textarea");
    ta.value = text;
    ta.setAttribute("readonly", "");
    ta.style.position = "fixed";
    ta.style.top = "-1000px";
    ta.setAttribute("aria-hidden", "true");
    document.body.appendChild(ta);
    ta.select();
    var ok = false;
    try {
      ok = document.execCommand("copy");
    } catch (e) {
      ok = false;
    }
    document.body.removeChild(ta);
    return ok;
  }

  function writeClipboard(text) {
    if (navigator.clipboard && navigator.clipboard.writeText) {
      return navigator.clipboard.writeText(text).then(
        function () {
          return true;
        },
        function () {
          return fallbackCopy(text);
        },
      );
    }
    return Promise.resolve(fallbackCopy(text));
  }

  document.addEventListener("click", function (ev) {
    var el = ev.target;
    if (!el || !el.closest) return;
    var btn = el.closest("[data-copy]");
    if (!btn) return;
    ev.preventDefault();
    var text = btn.getAttribute("data-copy") || "";
    writeClipboard(text).then(function (ok) {
      feedback(btn, ok ? "Copied" : "Copy failed", ok);
    });
  });
})();
