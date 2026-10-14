// CTRL007 — live activity delivery (browser consumer).
//
// Local-first: subscribes to the read-only SSE endpoint
// (/projects/<id>/activity/stream) using the last-seen cursor as ?after=, and
// falls back to bounded polling of /projects/<id>/activity/window when SSE is
// unavailable. Both transports deliver the identical payload shape and both are
// read-only: this script never issues a POST to any /commands/ route, so a
// reconnect/refresh/transport failure cannot re-trigger a lifecycle action.
//
// Identity/dedup: every rendered event is keyed by its stable cursor. Rows are
// inserted only when their cursor has not been rendered before, and events are
// placed in seq order, so a reconnect (Last-Event-ID / ?after=cursor) or a poll
// refresh never re-renders an already-seen event and the timeline stays
// oldest-first. The cursor is persisted per project in localStorage so a full
// page refresh recovers the recent timeline without duplicating rows.
//
// Reset: when the server cannot find the cursor we sent (rotation/truncation or
// a stale cursor) it answers with reset=true plus the bounded recent window
// instead of the (empty) set of later events. The consumer rebases its rendered
// timeline from that window and adopts the returned cursor, so an unknown cursor
// never leaves a silently gapped timeline.

(function () {
    "use strict";

    var POLL_MS = 3000;

    function init(root) {
        var project = root.getAttribute("data-project");
        var streamURL = root.getAttribute("data-stream-url");
        var windowURL = root.getAttribute("data-window-url");
        if (!project || !windowURL) return;

        var pollMs = parseInt(root.getAttribute("data-poll-ms"), 10);
        if (!(pollMs > 0)) pollMs = POLL_MS;

        var cursorKey = "sopctrlcursor:" + project;
        var seen = Object.create(null);
        var order = [];

        var list = root.querySelector("ol.timeline");
        var empty = root.querySelector("p.muted");
        if (!list) {
            list = document.createElement("ol");
            list.className = "timeline";
        }
        var seeded = root.querySelectorAll("li.tl-item[data-cursor]");
        for (var i = 0; i < seeded.length; i++) {
            var c = seeded[i].getAttribute("data-cursor");
            if (c && !seen[c]) {
                seen[c] = true;
                order.push({ cursor: c, seq: Number(seeded[i].getAttribute("data-seq") || 0) });
            }
        }

        function lastCursor() {
            var stored = null;
            try {
                stored = localStorage.getItem(cursorKey);
            } catch (e) {
                stored = null;
            }
            if (stored) return stored;
            if (order.length) return order[order.length - 1].cursor;
            return "";
        }

        function remember(cursor) {
            try {
                localStorage.setItem(cursorKey, cursor);
            } catch (e) {
                /* storage unavailable: in-memory order still dedups this session */
            }
        }

        function rowFor(ev) {
            var li = document.createElement("li");
            li.className = "tl-item";
            li.setAttribute("data-cursor", ev.cursor);
            li.setAttribute("data-seq", String(ev.seq));

            var dot = document.createElement("span");
            dot.className = "tl-dot " + (ev.stageClass || "s-planned");
            dot.setAttribute("aria-hidden", "true");

            var body = document.createElement("div");
            body.className = "tl-body";

            var head = document.createElement("div");
            head.className = "tl-head";
            var badge = document.createElement("span");
            badge.className = "badge " + (ev.stageClass || "s-planned");
            badge.textContent = ev.stage || "";
            var time = document.createElement("time");
            time.className = "muted small";
            time.textContent = ev.since || "";
            head.appendChild(badge);
            head.appendChild(time);

            var text = document.createElement("div");
            text.className = "tl-text";
            if (ev.action) text.appendChild(document.createTextNode(ev.action));
            if (ev.detail) {
                var detail = document.createElement("span");
                detail.className = "muted";
                if (ev.action) {
                    text.appendChild(document.createElement("br"));
                    detail.textContent = ev.detail;
                } else {
                    detail.textContent = ev.detail;
                }
                text.appendChild(detail);
            }

            body.appendChild(head);
            body.appendChild(text);
            li.appendChild(dot);
            li.appendChild(body);
            return li;
        }

        function append(events) {
            if (!events || !events.length) return 0;
            var fresh = [];
            for (var i = 0; i < events.length; i++) {
                var ev = events[i];
                if (!ev || !ev.cursor || seen[ev.cursor]) continue;
                seen[ev.cursor] = true;
                order.push({ cursor: ev.cursor, seq: Number(ev.seq || 0) });
                fresh.push(ev);
            }
            if (!fresh.length) return 0;
            fresh.sort(function (a, b) {
                return Number(a.seq || 0) - Number(b.seq || 0);
            });
            for (var j = 0; j < fresh.length; j++) {
                var li = rowFor(fresh[j]);
                var next = null;
                var rows = list.querySelectorAll("li.tl-item[data-seq]");
                for (var k = 0; k < rows.length; k++) {
                    if (Number(rows[k].getAttribute("data-seq") || 0) > Number(fresh[j].seq || 0)) {
                        next = rows[k];
                        break;
                    }
                }
                list.insertBefore(li, next);
            }
            if (empty && empty.parentNode) empty.parentNode.removeChild(empty);
            if (!list.parentNode) root.appendChild(list);
            return fresh.length;
        }

        // rebase replaces the entire rendered timeline with the server's
        // bounded recent window. It is used on reset (unknown cursor) so a
        // stale/gapped timeline is corrected instead of silently kept.
        function rebase(events) {
            seen = Object.create(null);
            order = [];
            var rows = list.querySelectorAll("li.tl-item[data-cursor]");
            for (var i = 0; i < rows.length; i++) {
                rows[i].parentNode.removeChild(rows[i]);
            }
            if (events && events.length) append(events);
        }

        function applyWindow(payload) {
            if (payload.reset) {
                // Unknown/ stale cursor: the server returned the bounded recent
                // window instead of later events. Rebase so the timeline is
                // whole again, then adopt the returned cursor.
                rebase(payload.events);
            } else {
                append(payload.events);
            }
            if (payload.cursor) remember(payload.cursor);
            // More=truncated backlog: immediately issue the next cursor request
            // instead of waiting for the next tick, so a drain has no gaps.
            if (payload.more) poll();
        }

        function poll() {
            var url = windowURL + "?limit=100";
            var cur = lastCursor();
            if (cur) url += "&after=" + encodeURIComponent(cur);
            fetch(url, { headers: { Accept: "application/json" } })
                .then(function (r) {
                    if (!r.ok) throw new Error("activity window: " + r.status);
                    return r.json();
                })
                .then(applyWindow)
                .catch(function () {
                    // Transport failure is non-fatal: SOP execution state is
                    // owned by SOP, not the browser. Retry on the next tick.
                });
        }

        function startSSE() {
            if (!streamURL || typeof EventSource === "undefined") {
                setInterval(poll, pollMs);
                return;
            }
            var url = streamURL;
            var cur = lastCursor();
            if (cur) url += "?after=" + encodeURIComponent(cur);
            var es = new EventSource(url);
            es.addEventListener("activity", function (msg) {
                try {
                    var ev = JSON.parse(msg.data);
                    if (msg.lastEventId) ev.cursor = msg.lastEventId;
                    applyWindow({ events: [ev], cursor: ev.cursor, more: false });
                } catch (e) {
                    /* ignore malformed frame */
                }
            });
            es.onerror = function () {
                es.close();
                // Fall back to bounded polling; the same cursor keeps dedup,
                // so the refresh does not duplicate lifecycle rows.
                poll();
                setInterval(poll, pollMs);
            };
        }

        poll();
        startSSE();
    }

    function boot() {
        var nodes = document.querySelectorAll("[data-activity-live]");
        for (var i = 0; i < nodes.length; i++) init(nodes[i]);
    }

    if (document.readyState === "loading") {
        document.addEventListener("DOMContentLoaded", boot);
    } else {
        boot();
    }
})();
