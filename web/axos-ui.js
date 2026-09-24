/*
 * AXOS UI — one renderer for both frontends:
 *   - the AXOS tabs inside the stock Merlin web UI (web/merlin/Axos_Content.asp,
 *     styled by Merlin's own form_style.css), and
 *   - the standalone page axosd serves on :9090 (web/index.html + styles.css,
 *     a faithful copy of the same look).
 * Both use Merlin's markup vocabulary (FormTable, FormTable_table,
 * list_table, button_gen, add_btn/remove_btn/edit_btn, apply_gen) so the
 * pages look like the router's own.
 *
 * Page contract: #axos-title, #axos-desc, #axos-root elements, and globals
 *   AXOS_SECTION   section id (all|vpn|lan|dns|firewall|qos|wifi|diag)
 *   AXOS_API_BASE  Core API origin ("" = same origin; default host:9090)
 *   AXOS_UI_TOKEN  LAN token (Merlin: from /userRpm/token.js)
 *   AXOS_EMBED     "merlin" or "standalone"
 */
(function (global) {
  "use strict";

  // ------------------------------------------------------------ config

  var EMBED = global.AXOS_EMBED || "merlin";
  var TOKEN_KEY = "axos-ui-token";

  function apiBase() {
    if (typeof global.AXOS_API_BASE === "string") return global.AXOS_API_BASE;
    return "http://" + (global.location.hostname || "192.168.50.1") + ":9090";
  }

  function token() {
    if (global.AXOS_UI_TOKEN) return global.AXOS_UI_TOKEN;
    try {
      return global.localStorage.getItem(TOKEN_KEY) || "";
    } catch (e) {
      return "";
    }
  }

  // Merlin page (basename) for each section; must match axos-merlin-ui.sh.
  var MERLIN_PAGES = {
    all: "Main_GameServer_Content.asp",
    vpn: "Advanced_VPN_PPTP.asp",
    lan: "Advanced_APPList_Content.asp",
    dns: "WAN_info.asp",
    firewall: "Advanced_VPN_IPSec.asp",
    qos: "Advanced_AiDisk_webdav.asp",
    wifi: "WiFi_Insight.asp",
    diag: "Guest_network.asp",
  };

  // ------------------------------------------------------------ api

  function req(method, path, body) {
    var opts = {
      method: method,
      headers: {
        Accept: "application/json",
        "X-Axos-Actor": EMBED === "merlin" ? "merlin-ui" : "webui",
      },
    };
    var tok = token();
    if (tok) opts.headers["X-Axos-UI-Token"] = tok;
    if (body !== undefined) {
      opts.headers["Content-Type"] = "application/json";
      opts.body = JSON.stringify(body);
    }
    return fetch(apiBase() + path, opts).then(function (r) {
      return r.text().then(function (t) {
        var data = null;
        try {
          data = t ? JSON.parse(t) : null;
        } catch (e) {
          data = { raw: t };
        }
        if (!r.ok) {
          var err = new Error((data && data.error) || method + " " + path + ": HTTP " + r.status);
          err.status = r.status;
          err.data = data;
          throw err;
        }
        return data;
      });
    });
  }
  var api = {
    get: function (p) { return req("GET", p); },
    post: function (p, b) { return req("POST", p, b || {}); },
    put: function (p, b) { return req("PUT", p, b); },
    del: function (p) { return req("DELETE", p); },
  };

  function optional(promise, fallback) {
    return promise.catch(function () { return fallback; });
  }

  // ------------------------------------------------------------ helpers

  function $(id) { return document.getElementById(id); }

  function esc(s) {
    return String(s == null ? "" : s)
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;")
      .replace(/'/g, "&#39;");
  }

  function list(v) { return Object.prototype.toString.call(v) === "[object Array]" ? v : []; }

  function errText(e) { return String((e && e.message) || e); }

  function fmtBytes(n) {
    n = Number(n) || 0;
    var u = ["B", "KB", "MB", "GB", "TB"];
    var i = 0;
    while (n >= 1024 && i < u.length - 1) { n /= 1024; i++; }
    return (i === 0 ? n : n.toFixed(n >= 100 ? 0 : 1)) + " " + u[i];
  }

  function fmtRate(bytesPerSec) {
    var bits = (Number(bytesPerSec) || 0) * 8;
    if (bits >= 1e9) return (bits / 1e9).toFixed(2) + " Gbps";
    if (bits >= 1e6) return (bits / 1e6).toFixed(1) + " Mbps";
    if (bits >= 1e3) return (bits / 1e3).toFixed(0) + " Kbps";
    return bits.toFixed(0) + " bps";
  }

  function fmtUptime(ns) {
    var sec = Math.floor(Number(ns || 0) / 1e9);
    if (!sec) return "-";
    var d = Math.floor(sec / 86400), h = Math.floor((sec % 86400) / 3600), m = Math.floor((sec % 3600) / 60);
    return d + " days " + h + " hours " + m + " minute(s)";
  }

  function fmtTime(t) {
    if (!t || String(t).indexOf("0001-") === 0) return "-";
    var d = new Date(t);
    return isNaN(d.getTime()) ? String(t) : d.toLocaleString();
  }

  function macKey(m) { return String(m || "").toUpperCase().replace(/-/g, ":"); }

  function isIPv4(s) {
    var m = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/.exec(String(s || "").trim());
    if (!m) return false;
    for (var i = 1; i <= 4; i++) if (Number(m[i]) > 255) return false;
    return true;
  }
  function isIPOrCIDR(s) {
    s = String(s || "").trim();
    var parts = s.split("/");
    if (parts.length > 2) return false;
    if (parts.length === 2 && !(/^\d{1,2}$/.test(parts[1]) && Number(parts[1]) <= 32)) return false;
    return isIPv4(parts[0]) || isIPv6(parts[0]);
  }
  // Full 8-group or "::"-compressed form; rejects MACs (6 groups).
  function isIPv6(s) {
    if (!/^[0-9a-fA-F:]+$/.test(s)) return false;
    var groups = s.split(":");
    for (var i = 0; i < groups.length; i++) if (groups[i].length > 4) return false;
    var dbl = s.split("::").length - 1;
    return dbl === 1 ? groups.length <= 8 : dbl === 0 && groups.length === 8;
  }
  function isMAC(s) { return /^([0-9A-Fa-f]{2}[:-]){5}[0-9A-Fa-f]{2}$/.test(String(s || "").trim()); }

  // Merlin's VPN Director interface token for a profile name.
  function directorIface(name) {
    var n = String(name || "").toUpperCase();
    if (n.indexOf("OVPNC") === 0) return "OVPN" + n.slice(5);
    return n || "WAN";
  }

  function clientName(c) { return c.hostname || c.mac || c.ip || "?"; }

  // --- Merlin markup builders

  function formTable(title, rowsHtml, extraAttr) {
    return '<table width="100%" border="1" align="center" cellpadding="4" cellspacing="0" bordercolor="#6b8fa3" class="FormTable axos-block"' +
      (extraAttr || "") + ">" +
      (title ? '<thead><tr><td colspan="2">' + title + "</td></tr></thead>" : "") +
      "<tbody>" + rowsHtml + "</tbody></table>";
  }

  function row(label, cellHtml, attrs) {
    return "<tr" + (attrs || "") + "><th>" + label + "</th><td>" + cellHtml + "</td></tr>";
  }

  // A Merlin list: FormTable_table header + list_table body in #bodyId.
  function listTable(title, cols, bodyId, extraAttr) {
    var head = "";
    for (var i = 0; i < cols.length; i++) {
      head += '<th width="' + cols[i][1] + '">' + cols[i][0] + "</th>";
    }
    return '<table width="100%" border="1" align="center" cellpadding="4" cellspacing="0" class="FormTable_table axos-block"' +
      (extraAttr || "") + ">" +
      '<thead><tr><td colspan="' + cols.length + '"' + (title.id ? ' id="' + title.id + '"' : "") + ">" + (title.text || title) + "</td></tr></thead>" +
      "<tr>" + head + "</tr></table>" +
      '<div id="' + bodyId + '"></div>';
  }

  // Rows for a list_table; widths repeat the header's so columns line up.
  function listRows(cols, rows, emptyText) {
    var html = '<table width="100%" cellspacing="0" cellpadding="4" align="center" class="list_table">';
    if (!rows.length) {
      html += '<tr><td class="hint-color" colspan="' + cols.length + '">' + esc(emptyText || "No data in table.") + "</td></tr>";
    }
    for (var r = 0; r < rows.length; r++) {
      var cells = rows[r].cells || rows[r];
      html += "<tr" + (rows[r].attrs || "") + ">";
      for (var c = 0; c < cells.length; c++) {
        html += '<td width="' + cols[c][1] + '"' + (cols[c][2] ? ' style="' + cols[c][2] + '"' : "") + ">" + cells[c] + "</td>";
      }
      html += "</tr>";
    }
    return html + "</table>";
  }

  function yesNo(name, value) {
    return '<input type="radio" name="' + name + '" class="input" value="1"' + (value ? " checked" : "") + ">Yes " +
      '<input type="radio" name="' + name + '" class="input" value="0"' + (value ? "" : " checked") + ">No";
  }
  function radioValue(name) {
    var els = document.getElementsByName(name);
    for (var i = 0; i < els.length; i++) if (els[i].checked) return els[i].value === "1";
    return false;
  }
  function setRadio(name, value) {
    var els = document.getElementsByName(name);
    for (var i = 0; i < els.length; i++) els[i].checked = (els[i].value === "1") === !!value;
  }

  function button(id, label, extra) {
    return '<input type="button" class="button_gen" id="' + id + '" value="' + esc(label) + '"' + (extra || "") + ">";
  }
  function applyGen(inner) { return '<div class="apply_gen">' + inner + "</div>"; }
  function stateIcon(on, cls, data) {
    return '<span class="axos-state ' + (on ? "axos-on" : "axos-off") + " " + (cls || "") + '" title="' +
      (on ? "Enabled" : "Disabled") + '"' + (data || "") + "></span>";
  }
  function textarea(id, rows) {
    return '<textarea id="' + id + '" class="axos-log" cols="63" rows="' + (rows || 12) + '" wrap="off" readonly="readonly"></textarea>';
  }
  function select(id, options, value, extra) {
    var html = '<select id="' + id + '" class="input_option"' + (extra || "") + ">";
    for (var i = 0; i < options.length; i++) {
      var v = options[i][0];
      html += '<option value="' + esc(v) + '"' + (String(v) === String(value) ? " selected" : "") + ">" + esc(options[i][1]) + "</option>";
    }
    return html + "</select>";
  }

  function on(id, ev, fn) {
    var el = $(id);
    if (el) el["on" + ev] = fn;
  }
  // Delegated click on a container: fn(target, dataset-ish getter).
  function onClick(id, cls, fn) {
    var el = $(id);
    if (!el) return;
    el.onclick = function (ev) {
      var t = ev.target;
      while (t && t !== el) {
        if (t.className && (" " + t.className + " ").indexOf(" " + cls + " ") >= 0) {
          fn(t);
          return;
        }
        t = t.parentNode;
      }
    };
  }

  // --- busy overlay: Merlin's own when present

  function busy(onState) {
    try {
      if (EMBED === "merlin" && typeof global.showLoading === "function") {
        if (onState) global.showLoading();
        else global.hideLoading();
        return;
      }
    } catch (e) { /* fall through to our overlay */ }
    var el = $("axos-busy");
    if (!el) {
      el = document.createElement("div");
      el.id = "axos-busy";
      el.innerHTML = '<div class="axos-busy-box">Applying settings, please wait...</div>';
      document.body.appendChild(el);
    }
    el.style.display = onState ? "block" : "none";
  }

  function setConn(msg) {
    var el = $("axos-conn");
    if (!el) return;
    el.style.display = msg ? "" : "none";
    el.innerHTML = msg ? esc(msg) : "";
  }

  // Safety net for every change: arm an auto-restore, make the change,
  // then confirm. If the change fails the arm is left in place, so the
  // router restores the previous settings by itself. An error flagged
  // noChange (e.g. the user cancelled) confirms instead: nothing changed.
  var ROLLBACK_SECONDS = 120;
  function withRollback(reason, fn) {
    busy(true);
    return api.post("/v1/rollback/arm", { timeout_seconds: ROLLBACK_SECONDS, reason: reason })
      .catch(function (e) {
        if (e.status === 409) {
          e.message = "Another change is still waiting for confirmation (auto-restore pending). Try again in a moment, or confirm it on Administration → AXOS.";
        }
        throw e;
      })
      .then(function (arm) {
        return fn().then(
          function (result) {
            return api.post("/v1/rollback/confirm", { id: arm.id }).then(function () { return result; });
          },
          function (err) {
            if (err && err.noChange) {
              return api.post("/v1/rollback/confirm", { id: arm.id }).then(function () { throw err; });
            }
            err.message = errText(err) + "\n\nAXOS will restore the previous settings automatically within " + ROLLBACK_SECONDS / 60 + " minutes.";
            throw err;
          }
        );
      })
      .then(
        function (r) { busy(false); return r; },
        function (e) { busy(false); throw e; }
      );
  }

  function fail(e) {
    if (e && e.noChange) return;
    alert(errText(e));
  }

  function cancelled() {
    var e = new Error("Cancelled");
    e.noChange = true;
    return e;
  }

  // Bulk policy writes answer 409 + conflicts when they would overwrite an
  // existing VPN Director rule. Ask, then resend with replace=true inside
  // the same armed transaction.
  function withReplaceConfirm(send) {
    return send(false).catch(function (e) {
      if (e.status !== 409 || !e.data || !e.data.conflicts) throw e;
      var lines = e.data.conflicts.map(function (c) {
        return "  " + c.existing.source + ": " + c.existing.interface + " (" + (c.existing.description || "-") + ")  →  " + c.proposed.interface;
      });
      if (!confirm("These devices already have VPN Director rules:\n\n" + lines.join("\n") + "\n\nReplace them?")) {
        throw cancelled();
      }
      return send(true);
    });
  }

  // ------------------------------------------------------------ state

  var state = {
    section: "all",
    timer: null,
    lastIfaces: null, // for throughput deltas
  };

  // ============================================================ sections

  var SECTIONS = {};

  // ------------------------------------------------------------ overview

  SECTIONS.all = {
    title: "Administration - AXOS",
    desc: "AXOS runs alongside Asuswrt-Merlin and applies every change behind a " +
      ROLLBACK_SECONDS / 60 + "-minute safety rollback. This page shows AXOS itself and the router at a glance.",
    refresh: 15000,
    render: function () {
      var pages = [
        ["vpn", "VPN", "VPN clients, routing LAN clients through tunnels, WireGuard import, endpoint latency"],
        ["lan", "LAN", "DHCP reservations and the LAN client list"],
        ["dns", "WAN", "WAN and LAN DNS servers, DNS-over-TLS"],
        ["firewall", "Firewall", "Live iptables rules"],
        ["qos", "Adaptive QoS", "QoS on/off and live throughput"],
        ["wifi", "Wireless", "Radios and wireless client signal"],
        ["diag", "Network Tools", "Ping, traceroute, nslookup, port check, iperf3"],
      ];
      var links = pages.map(function (p) {
        return { cells: ['<a class="axos-link" href="' + sectionHref(p[0]) + '">' + esc(p[1]) + " - AXOS</a>", esc(p[2])] };
      });
      var linkCols = [["Page", "30%"], ["What it covers", "70%", "text-align:left;padding-left:10px"]];
      return (
        formTable("AXOS Status",
          row("Core API", '<div class="axos-val" id="ov-core">-</div>') +
          row("Safety rollback", '<div class="axos-val" id="ov-rb">-</div> <div class="axos-val" id="ov-rb-btn"></div>') +
          row("AXOS memory use", '<div class="axos-val" id="ov-fp">-</div>')) +
        formTable("System",
          row("Model", '<div class="axos-val" id="ov-model">-</div>') +
          row("Firmware", '<div class="axos-val" id="ov-fw">-</div>') +
          row("Uptime", '<div class="axos-val" id="ov-up">-</div>') +
          row("CPU load (1 / 5 / 15 min)", '<div class="axos-val" id="ov-load">-</div>') +
          row("Memory", '<div class="axos-bar"><div id="ov-mem-bar"></div></div> <div class="axos-val" id="ov-mem">-</div>') +
          row("Temperatures", '<div class="axos-val" id="ov-temp">-</div>'),
          ' style="margin-top:8px"') +
        listTable("Network interfaces", IFACE_COLS, "ov-if-body") +
        '<div id="ov-svc-wrap" style="display:none">' + listTable("AXOS services", SVC_COLS, "ov-svc-body") + "</div>" +
        listTable({ id: "ov-bk-title", text: "Configuration backups" }, BACKUP_COLS, "ov-bk-body") +
        applyGen(button("ov-backup", "Create backup")) +
        listTable("AXOS pages", linkCols, "ov-links")
      ).replace('<div id="ov-links"></div>', '<div id="ov-links">' + listRows(linkCols, links) + "</div>");
    },
    bind: function () {
      on("ov-backup", "click", function () {
        busy(true);
        api.post("/v1/backup", { reason: "manual (web UI)" })
          .then(function () { busy(false); SECTIONS.all.load(); }, function (e) { busy(false); fail(e); });
      });
      onClick("ov-bk-body", "axos-restore", function (t) {
        var id = t.getAttribute("data-id");
        if (!confirm("Restore configuration backup " + id + "?\n\nThe router's settings are replaced by this snapshot. AXOS arms an automatic undo first.")) return;
        withRollback("web-ui restore " + id, function () { return api.post("/v1/restore", { backup_id: id }); })
          .then(function () { alert("Backup restored. Some services may restart."); SECTIONS.all.load(); }, fail);
      });
      onClick("ov-svc-body", "axos-svc-restart", function (t) {
        var name = t.getAttribute("data-name");
        if (!confirm("Restart AXOS service " + name + "?")) return;
        busy(true);
        api.post("/v1/supervisor/services/" + encodeURIComponent(name) + "/restart", {})
          .then(function () { busy(false); SECTIONS.all.load(); }, function (e) { busy(false); fail(e); });
      });
      onClick("ov-rb-btn", "axos-rb-confirm", function (t) {
        api.post("/v1/rollback/confirm", { id: t.getAttribute("data-id") }).then(SECTIONS.all.load, fail);
      });
    },
    load: function () {
      return Promise.all([
        api.get("/v1/info"),
        api.get("/v1/resources"),
        optional(api.get("/v1/interfaces"), []),
        optional(api.get("/v1/rollback/status"), null),
        optional(api.get("/v1/footprint"), null),
        optional(api.get("/v1/supervisor/services"), null),
        optional(api.get("/v1/backups"), []),
      ]).then(function (d) {
        var info = d[0] || {}, res = d[1] || {};
        $("ov-core").innerHTML = "Connected - " + esc(apiBase() || global.location.host);
        $("ov-model").textContent = info.model || "-";
        $("ov-fw").textContent = ((info.firmware_version || "") + " " + (info.firmware_revision || "")).trim() || "-";
        $("ov-up").textContent = fmtUptime(info.uptime);
        $("ov-load").textContent = [res.cpu_load_1m, res.cpu_load_5m, res.cpu_load_15m].map(function (v) {
          return v == null ? "-" : Number(v).toFixed(2);
        }).join(" / ");
        var total = Number(res.mem_total_kb) || 0, used = Number(res.mem_used_kb) || 0;
        var pct = total ? Math.round((used / total) * 100) : 0;
        $("ov-mem").textContent = Math.round(used / 1024) + " MB / " + Math.round(total / 1024) + " MB (" + pct + "%)";
        $("ov-mem-bar").style.width = pct + "%";
        var temps = [];
        for (var k in res.temperatures_c || {}) temps.push(k + ": " + Number(res.temperatures_c[k]).toFixed(1) + " °C");
        $("ov-temp").textContent = temps.join(", ") || "-";

        renderIfaces("ov-if-body", list(d[2]));

        var rb = d[3];
        if (rb && rb.Pending) {
          $("ov-rb").innerHTML = '<span class="hint-color">Armed</span> - ' + esc(rb.Reason || rb.ID) +
            ", restores automatically at " + esc(new Date(rb.Deadline).toLocaleTimeString());
          $("ov-rb-btn").innerHTML = '<input type="button" class="button_gen axos-rb-confirm" data-id="' + esc(rb.ID) + '" value="Keep changes">';
        } else {
          $("ov-rb").textContent = rb ? "Idle - no change waiting for confirmation" : "-";
          $("ov-rb-btn").innerHTML = "";
        }

        var fp = d[4];
        $("ov-fp").textContent = fp && fp.current && fp.current.process_rss_kb
          ? (fp.current.process_rss_kb / 1024).toFixed(1) + " MB resident (axosd)" +
            (fp.baseline && fp.baseline.process_rss_kb ? ", baseline " + (fp.baseline.process_rss_kb / 1024).toFixed(1) + " MB" : "")
          : "-";

        var svcs = d[5];
        $("ov-svc-wrap").style.display = svcs ? "" : "none";
        if (svcs) {
          $("ov-svc-body").innerHTML = listRows(SVC_COLS, list(svcs).map(function (s) {
            return [esc(s.name), s.running ? "Running" : '<span class="hint-color">Stopped</span>', esc(s.pid || "-"),
              esc(s.restarts || 0),
              '<input type="button" class="button_gen axos-svc-restart" data-name="' + esc(s.name) + '" value="Restart">'];
          }), "No supervised services.");
        }

        var backups = list(d[6]);
        $("ov-bk-title").textContent = "Configuration backups (" + backups.length + ")";
        $("ov-bk-body").innerHTML = listRows(BACKUP_COLS, backups.slice(0, 10).map(function (b) {
          return [esc(fmtTime(b.created_at)), esc(b.reason || "-"), esc(fmtBytes(b.size_bytes)), '<code>' + esc(b.id) + "</code>",
            '<input type="button" class="button_gen axos-restore" data-id="' + esc(b.id) + '" value="Restore">'];
        }), "No backups yet.");
      });
    },
  };

  var IFACE_COLS = [["Interface", "13%"], ["Type", "11%"], ["Role", "9%"], ["State", "9%"], ["Address", "24%"], ["Link", "10%"], ["Received", "12%"], ["Sent", "12%"]];
  var SVC_COLS = [["Service", "30%"], ["State", "20%"], ["PID", "15%"], ["Restarts", "15%"], ["Action", "20%"]];
  var BACKUP_COLS = [["Created", "25%"], ["Reason", "22%"], ["Size", "11%"], ["ID", "27%"], ["Action", "15%"]];

  function renderIfaces(bodyId, ifaces) {
    ifaces = ifaces.slice().sort(function (a, b) {
      var order = { wan: 0, lan: 1, guest: 2 };
      var ra = order[a.role] != null ? order[a.role] : 3, rb = order[b.role] != null ? order[b.role] : 3;
      return ra - rb || String(a.name).localeCompare(String(b.name));
    });
    $(bodyId).innerHTML = listRows(IFACE_COLS, ifaces.map(function (i) {
      return [esc(i.name), esc(i.type || "-"), esc((i.role || "-").toUpperCase()),
        i.state === "up" ? "Up" : '<span class="hint-color">' + esc(i.state || "?") + "</span>",
        esc(list(i.addresses).join(", ") || "-"), esc(i.link_speed || "-"), esc(fmtBytes(i.rx_bytes)), esc(fmtBytes(i.tx_bytes))];
    }));
  }

  // ------------------------------------------------------------ VPN

  var PROFILE_COLS = [["Enable", "9%"], ["Instance", "33%", "text-align:left;padding-left:10px"], ["Type", "13%"], ["Endpoint", "27%"], ["Routed clients", "18%"]];
  var STEER_COLS = [["<input type=\"checkbox\" id=\"vpn-steer-all\" title=\"Select all\">", "6%"], ["Client Name", "26%", "text-align:left;padding-left:10px"], ["IP", "16%"], ["MAC", "20%"], ["Current route", "16%"], ["Reserved IP", "16%"]];
  var RULE_COLS = [["Enable", "9%"], ["Description", "25%"], ["Local IP", "22%"], ["Remote IP", "22%"], ["Iface", "10%"], ["Edit", "12%"]];
  var BENCH_COLS = [["Rank", "10%"], ["Host", "45%"], ["Average RTT", "25%"], ["Result", "20%"]];

  var vpn = {
    profiles: [], clients: [], reservations: [], groups: [],
    rules: [],        // staged VPN Director list (Merlin-style: edit, then Apply)
    rulesDirty: false,
    checked: {},      // MAC -> true, survives re-render
  };

  SECTIONS.vpn = {
    title: "VPN - AXOS",
    desc: "Send LAN clients through a VPN tunnel or keep them on WAN. AXOS writes the same VPN Director rules as " +
      "VPN → VPN Director, so both pages always agree. Tick clients, pick an interface, then Apply.",
    refresh: 30000,
    render: function () {
      var units = [];
      for (var u = 1; u <= 5; u++) units.push([u, "WGC" + u]);
      return (
        listTable("VPN clients status", PROFILE_COLS, "vpn-prof-body") +
        '<p class="axos-note">Click the icon in the Enable column to connect or disconnect a client.</p>' +

        formTable("Route LAN clients",
          row("Interface", select("vpn-iface", [["WAN", "WAN"]], "WAN")) +
          row("Description", '<input type="text" id="vpn-desc" maxlength="32" class="input_25_table" value="axos" autocomplete="off">') +
          row("Client group",
            select("vpn-group", [["", "None"]], "") +
            ' <input type="text" id="vpn-group-name" maxlength="32" class="input_15_table" placeholder="new group name" autocomplete="off"> ' +
            button("vpn-group-save", "Save group") + " " + button("vpn-group-del", "Delete group")) +
          row("Show",
            select("vpn-show", [["all", "All clients"], ["wan", "Using WAN"], ["vpn", "Using a VPN"], ["none", "No rule"]], "all") +
            ' <input type="text" id="vpn-filter" class="input_15_table" placeholder="filter" autocomplete="off">'),
          ' style="margin-top:8px"') +
        listTable({ id: "vpn-steer-title", text: "LAN clients" }, STEER_COLS, "vpn-steer-body") +
        '<p class="axos-note">VPN Director matches by IP address. Clients without a reserved IP can get a new address from DHCP and drop out of their rule — reserve them on LAN → AXOS.</p>' +
        applyGen(button("vpn-steer-apply", "Apply") + " " + button("vpn-steer-remove", "Remove rules for selected")) +

        '<div class="addRuleFrame axos-addrule"><div class="addRuleText">VPN Director rules ( Max Limit : 199 )</div>' +
        '<div class="add_btn" id="vpn-rule-add" title="Add rule"></div></div>' +
        listTable({ id: "vpn-rules-title", text: "VPN Director rules" }, RULE_COLS, "vpn-rules-body") +
        '<p class="axos-note" id="vpn-rules-dirty" style="display:none"><span class="hint-color">Rules changed — click Apply to save them.</span></p>' +
        applyGen(button("vpn-rules-apply", "Apply") + " " + button("vpn-rules-revert", "Discard changes")) +

        formTable("WireGuard client import",
          row("Client slot", select("wg-unit", units, 5) + ' <span id="wg-unit-state"></span>') +
          row("Import .conf file", '<input type="file" id="wg-file" accept=".conf,.txt">') +
          row("…or paste config", '<textarea id="wg-paste" class="axos-paste" rows="4" placeholder="[Interface]&#10;PrivateKey = ...&#10;[Peer]&#10;..."></textarea><br>' +
            button("wg-parse", "Fill fields from config")) +
          row("Description", '<input type="text" id="wg-desc" maxlength="40" class="input_25_table" autocomplete="off">') +
          row("Private Key", '<input type="password" id="wg-priv" class="input_32_table" autocomplete="off">') +
          row("Address", '<input type="text" id="wg-addr" class="input_32_table" placeholder="10.2.0.2/32" autocomplete="off">') +
          row("DNS Server (Optional)", '<input type="text" id="wg-dns" class="input_32_table" autocomplete="off">') +
          row("MTU (Optional)", '<input type="text" id="wg-mtu" maxlength="4" class="input_6_table" autocomplete="off">') +
          row("Server Public Key", '<input type="text" id="wg-ppub" class="input_32_table" autocomplete="off">') +
          row("Preshared Key (Optional)", '<input type="password" id="wg-psk" class="input_32_table" autocomplete="off">') +
          row("Endpoint Address:Port", '<input type="text" id="wg-ep" class="input_32_table" placeholder="vpn.example.com" autocomplete="off"> : ' +
            '<input type="text" id="wg-port" maxlength="5" class="input_6_table" value="51820" autocomplete="off">') +
          row("Allowed IPs", '<input type="text" id="wg-allowed" class="input_32_table" value="0.0.0.0/0" autocomplete="off">') +
          row("Persistent Keepalive", '<input type="text" id="wg-ka" maxlength="3" class="input_6_table" value="25" autocomplete="off">') +
          row("Enable NAT", yesNo("wg-nat", true)) +
          row("Killswitch", yesNo("wg-ks", false)),
          ' style="margin-top:18px"') +
        applyGen(button("wg-import", "Import")) +

        formTable("Endpoint latency",
          row("Hosts", '<input type="text" id="vpn-bench-hosts" class="input_32_table" value="1.1.1.1 8.8.8.8 9.9.9.9" autocomplete="off"> ' +
            button("vpn-bench-fill", "Use tunnel endpoints")) +
          row("Pings per host", '<input type="text" id="vpn-bench-count" maxlength="2" class="input_3_table" value="3">'),
          ' style="margin-top:18px"') +
        applyGen(button("vpn-bench-run", "Test")) +
        '<div id="vpn-bench-wrap" style="display:none">' + listTable({ id: "vpn-bench-title", text: "Results" }, BENCH_COLS, "vpn-bench-body") + "</div>" +
        ruleEditorHtml()
      );
    },
    bind: bindVPN,
    load: function () {
      return Promise.all([
        optional(api.get("/v1/vpn/profiles"), []),
        api.get("/v1/clients"),
        optional(api.get("/v1/dhcp/reservations"), []),
        optional(api.get("/v1/policy"), []),
        optional(api.get("/v1/vpn/client-groups"), { groups: [] }),
      ]).then(function (d) {
        vpn.profiles = list(d[0]).sort(function (a, b) {
          return String(a.name).localeCompare(String(b.name), undefined, { numeric: true });
        });
        vpn.clients = list(d[1]);
        vpn.reservations = list(d[2]);
        vpn.groups = list((d[4] || {}).groups);
        if (!vpn.rulesDirty) vpn.rules = list(d[3]).map(function (r) { return copyRule(r); });
        renderProfiles();
        fillIfaceSelect("vpn-iface");
        fillIfaceSelect("rule-iface");
        fillGroupSelect();
        renderSteer();
        renderRules();
        renderWGSlot();
      });
    },
  };

  function copyRule(r) {
    return { id: r.id, enabled: r.enabled !== false, description: r.description || "", source: r.source || "", remote: r.remote || "", interface: directorIface(r.interface) };
  }

  function ifaceOptions() {
    var opts = [["WAN", "WAN"]];
    vpn.profiles.forEach(function (p) {
      var v = directorIface(p.name);
      opts.push([v, v + (p.description ? ": " + p.description : "") + (p.enabled ? "" : " (off)")]);
    });
    return opts;
  }

  function fillIfaceSelect(id) {
    var sel = $(id);
    if (!sel) return;
    var cur = sel.getAttribute("data-filled") ? sel.value : "";
    sel.setAttribute("data-filled", "1");
    sel.innerHTML = ifaceOptions().map(function (o) {
      return '<option value="' + esc(o[0]) + '">' + esc(o[1]) + "</option>";
    }).join("");
    if (cur && sel.querySelector('option[value="' + cur + '"]')) sel.value = cur;
    else if (id === "vpn-iface") {
      // Default to the first connected tunnel.
      for (var i = 0; i < vpn.profiles.length; i++) {
        if (vpn.profiles[i].enabled) { sel.value = directorIface(vpn.profiles[i].name); break; }
      }
    }
  }

  function rulesFor(iface) {
    return vpn.rules.filter(function (r) { return r.enabled && directorIface(r.interface) === iface; }).length;
  }

  function renderProfiles() {
    $("vpn-prof-body").innerHTML = listRows(PROFILE_COLS, vpn.profiles.map(function (p) {
      var iface = directorIface(p.name);
      var n = rulesFor(iface);
      return [
        stateIcon(p.enabled, "axos-vpn-toggle", ' data-name="' + esc(p.name) + '" data-on="' + (p.enabled ? 1 : 0) + '"'),
        esc(iface + ": " + (p.description || "(no description)")),
        p.type === "wireguard" ? "WireGuard" : p.type === "openvpn" ? "OpenVPN" : esc(p.type || "-"),
        esc(p.endpoint || "-"),
        n ? '<span class="hint-color">' + n + " via VPN Director</span>" : "-",
      ];
    }), "No VPN client slots configured.");
  }

  function reservationFor(mac) {
    var k = macKey(mac);
    for (var i = 0; i < vpn.reservations.length; i++) if (macKey(vpn.reservations[i].mac) === k) return vpn.reservations[i];
    return null;
  }

  function ruleForIP(ip) {
    for (var i = 0; i < vpn.rules.length; i++) {
      if (!vpn.rules[i].remote && String(vpn.rules[i].source).trim() === ip) return vpn.rules[i];
    }
    return null;
  }

  function renderSteer() {
    var show = ($("vpn-show") || {}).value || "all";
    var filter = String(($("vpn-filter") || {}).value || "").toLowerCase();
    var rows = [];
    var clients = vpn.clients.slice().sort(function (a, b) { return clientName(a).localeCompare(clientName(b)); });
    var shown = 0;
    clients.forEach(function (c) {
      var rule = c.ip ? ruleForIP(c.ip) : null;
      var route = rule && rule.enabled ? directorIface(rule.interface) : "";
      if (show === "wan" && route !== "WAN" && route !== "") return;
      if (show === "vpn" && (!route || route === "WAN")) return;
      if (show === "none" && rule) return;
      var hay = (clientName(c) + " " + c.ip + " " + c.mac).toLowerCase();
      if (filter && hay.indexOf(filter) < 0) return;
      shown++;
      var res = reservationFor(c.mac);
      var key = macKey(c.mac);
      rows.push([
        '<input type="checkbox" class="vpn-steer-cb" data-mac="' + esc(key) + '"' + (vpn.checked[key] ? " checked" : "") + (c.ip ? "" : " disabled") + ">",
        esc(clientName(c)),
        esc(c.ip || "-"),
        esc(key),
        route ? (route === "WAN" ? "WAN (forced)" : '<span class="hint-color">' + esc(route) + "</span>") : "WAN",
        res ? (res.ip === c.ip ? "Yes" : '<span class="hint-color">' + esc(res.ip) + "</span>") : "No",
      ]);
    });
    $("vpn-steer-title").textContent = "LAN clients (" + shown + " / " + vpn.clients.length + ")";
    $("vpn-steer-body").innerHTML = listRows(STEER_COLS, rows, "No clients match.");
  }

  function renderRules() {
    var rows = vpn.rules.map(function (r, i) {
      return [
        stateIcon(r.enabled, "axos-rule-toggle", ' data-idx="' + i + '"'),
        esc(r.description || "-"), esc(r.source), esc(r.remote || ""), esc(r.interface),
        '<input class="edit_btn axos-rule-edit" data-idx="' + i + '" value="" type="button" title="Edit">' +
        '<input class="remove_btn axos-rule-del" data-idx="' + i + '" value="" type="button" title="Delete">',
      ];
    });
    $("vpn-rules-title").textContent = "VPN Director rules (" + vpn.rules.length + " / 199)";
    $("vpn-rules-body").innerHTML = listRows(RULE_COLS, rows, "No rules");
    $("vpn-rules-dirty").style.display = vpn.rulesDirty ? "" : "none";
    renderProfiles();
  }

  function fillGroupSelect() {
    var sel = $("vpn-group");
    var cur = sel.value;
    sel.innerHTML = '<option value="">None</option>' + vpn.groups.map(function (g) {
      return '<option value="' + esc(g.id) + '">' + esc(g.name) + " (" + list(g.members).length + ")</option>";
    }).join("");
    if (cur) sel.value = cur;
  }

  function selectedMACs() {
    var out = [];
    for (var k in vpn.checked) if (vpn.checked[k]) out.push(k);
    return out;
  }

  function selectedIPs() {
    var macs = {};
    selectedMACs().forEach(function (m) { macs[m] = true; });
    return vpn.clients.filter(function (c) { return macs[macKey(c.mac)] && c.ip; }).map(function (c) { return c.ip; });
  }

  function ruleEditorHtml() {
    return '<div id="rule-editor" class="pop_div_bg axos-pop"><div class="axos-pop-inner">' +
      formTable("Enter rule parameters",
        row("Interface", select("rule-iface", [["WAN", "WAN"]], "WAN")) +
        row("Enable", '<input type="checkbox" id="rule-enable" checked>') +
        row("Description", '<input type="text" id="rule-desc" maxlength="32" class="input_25_table" autocomplete="off"> <span>Optional</span>') +
        row("Local IP", '<input type="text" id="rule-local" maxlength="18" class="input_18_table" autocomplete="off"> ' +
          select("rule-pick", [["", "Pick a client"]], "")) +
        row("Remote IP", '<input type="text" id="rule-remote" maxlength="18" class="input_18_table" autocomplete="off"> <span>Optional</span>')) +
      '<div class="hint-color" style="margin:10px 0;">* IP addresses can be entered in CIDR format (for example, 192.168.1.0/24).</div>' +
      '<div class="axos-pop-btns">' + button("rule-cancel", "Cancel") + " " + button("rule-ok", "OK") + "</div></div></div>";
  }

  var editIdx = -1;
  function openRuleEditor(idx) {
    editIdx = idx;
    var r = idx >= 0 ? vpn.rules[idx] : { enabled: true, description: "", source: "", remote: "", interface: $("vpn-iface").value || "WAN" };
    fillIfaceSelect("rule-iface");
    $("rule-iface").value = r.interface;
    $("rule-enable").checked = r.enabled;
    $("rule-desc").value = r.description;
    $("rule-local").value = r.source;
    $("rule-remote").value = r.remote;
    $("rule-pick").innerHTML = '<option value="">Pick a client</option>' + vpn.clients.filter(function (c) { return c.ip; }).map(function (c) {
      return '<option value="' + esc(c.ip) + '">' + esc(clientName(c) + " (" + c.ip + ")") + "</option>";
    }).join("");
    $("rule-editor").style.display = "block";
  }

  function saveRuleEditor() {
    var r = {
      id: editIdx >= 0 ? vpn.rules[editIdx].id : "",
      enabled: $("rule-enable").checked,
      description: $("rule-desc").value.trim(),
      source: $("rule-local").value.trim(),
      remote: $("rule-remote").value.trim(),
      interface: $("rule-iface").value,
    };
    if (!isIPOrCIDR(r.source)) { alert("Local IP must be an IP address or CIDR."); return; }
    if (r.remote && !isIPOrCIDR(r.remote)) { alert("Remote IP must be empty or an IP address / CIDR."); return; }
    if (/[<>]/.test(r.description)) { alert("Description cannot contain < or >."); return; }
    for (var i = 0; i < vpn.rules.length; i++) {
      if (i !== editIdx && vpn.rules[i].source === r.source && vpn.rules[i].remote === r.remote) {
        alert("A rule for " + r.source + (r.remote ? " → " + r.remote : "") + " already exists."); return;
      }
    }
    if (editIdx < 0 && vpn.rules.length >= 199) { alert("VPN Director holds at most 199 rules."); return; }
    if (editIdx >= 0) vpn.rules[editIdx] = r; else vpn.rules.push(r);
    vpn.rulesDirty = true;
    $("rule-editor").style.display = "none";
    renderRules();
  }

  function renderWGSlot() {
    var unit = Number($("wg-unit").value);
    var p = null;
    vpn.profiles.forEach(function (x) { if (x.name === "wgc" + unit) p = x; });
    $("wg-unit-state").innerHTML = p
      ? '<span class="hint-color">In use: ' + esc(p.description || "no description") + (p.enabled ? ", connected" : "") + " — import overwrites it.</span>"
      : "Empty";
  }

  // Parse a wg-quick style config into the import fields.
  function parseWGConf(text) {
    var sec = "", out = {};
    String(text || "").split(/\r?\n/).forEach(function (line) {
      line = line.replace(/#.*/, "").trim();
      if (!line) return;
      var m = /^\[(\w+)\]$/.exec(line);
      if (m) { sec = m[1].toLowerCase(); return; }
      var kv = /^(\w+)\s*=\s*(.+)$/.exec(line);
      if (!kv) return;
      out[sec + "." + kv[1].toLowerCase()] = kv[2].trim();
    });
    var ep = out["peer.endpoint"] || "";
    var host = ep, port = "";
    var em = /^\[?([^\]]+?)\]?:(\d+)$/.exec(ep);
    if (em) { host = em[1]; port = em[2]; }
    return {
      priv: out["interface.privatekey"] || "",
      addr: (out["interface.address"] || "").split(",")[0].trim(),
      dns: (out["interface.dns"] || "").split(",")[0].trim(),
      mtu: out["interface.mtu"] || "",
      ppub: out["peer.publickey"] || "",
      psk: out["peer.presharedkey"] || "",
      ep: host, port: port,
      allowed: out["peer.allowedips"] || "",
      ka: out["peer.persistentkeepalive"] || "",
    };
  }

  function fillWG(f) {
    var map = { priv: "wg-priv", addr: "wg-addr", dns: "wg-dns", mtu: "wg-mtu", ppub: "wg-ppub", psk: "wg-psk", ep: "wg-ep", port: "wg-port", allowed: "wg-allowed", ka: "wg-ka" };
    for (var k in map) if (f[k]) $(map[k]).value = f[k];
  }

  function bindVPN() {
    onClick("vpn-prof-body", "axos-vpn-toggle", function (t) {
      var name = t.getAttribute("data-name");
      var up = t.getAttribute("data-on") !== "1";
      if (!confirm((up ? "Connect " : "Disconnect ") + directorIface(name) + "?")) return;
      withRollback("web-ui vpn " + name + (up ? " up" : " down"), function () {
        return api.post("/v1/vpn/" + encodeURIComponent(name) + "/" + (up ? "up" : "down"), {});
      }).then(SECTIONS.vpn.load, fail);
    });

    on("vpn-show", "change", renderSteer);
    on("vpn-filter", "input", renderSteer);
    var body = $("vpn-steer-body");
    body.onchange = function (ev) {
      var t = ev.target;
      if (t.className === "vpn-steer-cb") vpn.checked[t.getAttribute("data-mac")] = t.checked;
    };
    // The select-all box lives in the header table.
    document.addEventListener("change", function (ev) {
      if (ev.target && ev.target.id === "vpn-steer-all") {
        var boxes = document.querySelectorAll(".vpn-steer-cb");
        for (var i = 0; i < boxes.length; i++) {
          if (boxes[i].disabled) continue;
          boxes[i].checked = ev.target.checked;
          vpn.checked[boxes[i].getAttribute("data-mac")] = ev.target.checked;
        }
      }
    });

    on("vpn-group", "change", function () {
      var id = $("vpn-group").value;
      vpn.checked = {};
      vpn.groups.forEach(function (g) {
        if (g.id !== id) return;
        list(g.members).forEach(function (m) { vpn.checked[macKey(m)] = true; });
        if (g.interface && $("vpn-iface").querySelector('option[value="' + g.interface + '"]')) $("vpn-iface").value = g.interface;
        $("vpn-desc").value = g.name;
      });
      renderSteer();
    });
    on("vpn-group-save", "click", function () {
      var name = $("vpn-group-name").value.trim();
      if (!name) {
        vpn.groups.forEach(function (g) { if (g.id === $("vpn-group").value) name = g.name; });
      }
      var macs = selectedMACs();
      if (!name) { alert("Enter a group name (or select a group to update)."); return; }
      if (!macs.length) { alert("Tick the clients that belong in this group, then Save group."); return; }
      var id = String(name).toLowerCase().replace(/[^a-z0-9_-]+/g, "-").replace(/^-+|-+$/g, "").slice(0, 32) || "group";
      var next = vpn.groups.filter(function (g) { return g.id !== id && g.name !== name; });
      next.push({ id: id, name: name, members: macs, interface: $("vpn-iface").value });
      api.put("/v1/vpn/client-groups", { groups: next }).then(function (r) {
        vpn.groups = list(r && r.groups);
        fillGroupSelect();
        $("vpn-group").value = id;
        $("vpn-group-name").value = "";
      }, fail);
    });
    on("vpn-group-del", "click", function () {
      var id = $("vpn-group").value;
      if (!id) { alert("Select a group first."); return; }
      if (!confirm("Delete this group? Existing VPN Director rules are not changed.")) return;
      api.put("/v1/vpn/client-groups", { groups: vpn.groups.filter(function (g) { return g.id !== id; }) }).then(function (r) {
        vpn.groups = list(r && r.groups);
        fillGroupSelect();
      }, fail);
    });

    on("vpn-steer-apply", "click", function () {
      var ips = selectedIPs();
      var iface = $("vpn-iface").value;
      if (!ips.length) { alert("Select at least one client with an IP address."); return; }
      if (vpn.rulesDirty && !confirm("You have unsaved rule edits below. Apply discards them. Continue?")) return;
      var desc = $("vpn-desc").value.trim() || "axos";
      withRollback("web-ui route " + ips.length + " client(s) to " + iface, function () {
        return withReplaceConfirm(function (replace) {
          return api.post("/v1/policy/bulk", { interface: iface, description: desc, sources: ips, replace: replace });
        });
      }).then(function () {
        vpn.rulesDirty = false;
        vpn.checked = {};
        SECTIONS.vpn.load();
      }, fail);
    });
    on("vpn-steer-remove", "click", function () {
      var ips = selectedIPs();
      if (!ips.length) { alert("Select at least one client."); return; }
      if (!confirm("Remove every VPN Director rule for " + ips.length + " client(s)? They go back to the default route.")) return;
      withRollback("web-ui remove rules for " + ips.length + " client(s)", function () {
        return api.post("/v1/policy/bulk/remove", { sources: ips });
      }).then(function () {
        vpn.rulesDirty = false;
        vpn.checked = {};
        SECTIONS.vpn.load();
      }, fail);
    });

    on("vpn-rule-add", "click", function () { openRuleEditor(-1); });
    // Rules are staged like Merlin's VPN Director page: edit, then Apply.
    $("vpn-rules-body").onclick = function (ev) {
      var t = ev.target, cls = " " + (t && t.className) + " ";
      var idx = Number(t && t.getAttribute && t.getAttribute("data-idx"));
      if (cls.indexOf(" axos-rule-toggle ") >= 0) {
        vpn.rules[idx].enabled = !vpn.rules[idx].enabled;
      } else if (cls.indexOf(" axos-rule-del ") >= 0) {
        vpn.rules.splice(idx, 1);
      } else if (cls.indexOf(" axos-rule-edit ") >= 0) {
        openRuleEditor(idx);
        return;
      } else {
        return;
      }
      vpn.rulesDirty = true;
      renderRules();
    };
    on("rule-cancel", "click", function () { $("rule-editor").style.display = "none"; });
    on("rule-ok", "click", saveRuleEditor);
    on("rule-pick", "change", function () { if ($("rule-pick").value) $("rule-local").value = $("rule-pick").value; });
    on("vpn-rules-revert", "click", function () { vpn.rulesDirty = false; SECTIONS.vpn.load(); });
    on("vpn-rules-apply", "click", function () {
      if (!vpn.rulesDirty) { alert("No changes to apply."); return; }
      var body = vpn.rules.map(function (r) {
        return { description: r.description, source: r.source, remote: r.remote, interface: r.interface, enabled: r.enabled };
      });
      withRollback("web-ui VPN Director rules (" + body.length + ")", function () {
        return api.put("/v1/policy", body);
      }).then(function () { vpn.rulesDirty = false; SECTIONS.vpn.load(); }, fail);
    });

    on("wg-unit", "change", renderWGSlot);
    on("wg-parse", "click", function () {
      var f = parseWGConf($("wg-paste").value);
      if (!f.priv && !f.ppub) { alert("That doesn't look like a WireGuard config ([Interface] / [Peer])."); return; }
      fillWG(f);
      $("wg-paste").value = "";
    });
    on("wg-file", "change", function () {
      var file = $("wg-file").files[0];
      if (!file) return;
      var rd = new FileReader();
      rd.onload = function () {
        fillWG(parseWGConf(rd.result));
        if (!$("wg-desc").value) $("wg-desc").value = file.name.replace(/\.(conf|txt)$/i, "").slice(0, 40);
        $("wg-file").value = "";
      };
      rd.readAsText(file);
    });
    on("wg-import", "click", function () {
      var unit = Number($("wg-unit").value);
      var body = {
        unit: unit,
        description: $("wg-desc").value.trim(),
        private_key: $("wg-priv").value.trim(),
        address: $("wg-addr").value.trim(),
        dns: $("wg-dns").value.trim(),
        mtu: Number($("wg-mtu").value) || 0,
        peer_public_key: $("wg-ppub").value.trim(),
        preshared_key: $("wg-psk").value.trim(),
        endpoint: $("wg-ep").value.trim(),
        endpoint_port: Number($("wg-port").value) || 51820,
        allowed_ips: $("wg-allowed").value.split(/[\s,]+/).filter(Boolean),
        keepalive: Number($("wg-ka").value) || 0,
        nat: radioValue("wg-nat"),
        kill_switch: radioValue("wg-ks"),
      };
      var missing = [];
      if (!body.private_key) missing.push("Private Key");
      if (!body.address) missing.push("Address");
      if (!body.peer_public_key) missing.push("Server Public Key");
      if (!body.endpoint) missing.push("Endpoint Address");
      if (missing.length) { alert("Required: " + missing.join(", ")); return; }
      var inUse = vpn.profiles.some(function (p) { return p.name === "wgc" + unit; });
      if (inUse && !confirm("WGC" + unit + " is already configured. Overwrite it?")) return;
      withRollback("web-ui import WGC" + unit, function () { return api.post("/v1/vpn/wireguard/import", body); })
        .then(function () {
          $("wg-priv").value = "";
          $("wg-psk").value = "";
          alert("Imported into WGC" + unit + ". Connect it from VPN clients status above.");
          SECTIONS.vpn.load();
        }, fail);
    });

    on("vpn-bench-fill", "click", function () {
      var hosts = vpn.profiles.map(function (p) { return String(p.endpoint || "").replace(/:\d+$/, ""); }).filter(Boolean);
      if (!hosts.length) { alert("No tunnel endpoints configured."); return; }
      $("vpn-bench-hosts").value = hosts.join(" ");
    });
    on("vpn-bench-run", "click", function () {
      var hosts = $("vpn-bench-hosts").value.trim().split(/[\s,]+/).filter(Boolean);
      if (!hosts.length) { alert("Enter at least one host."); return; }
      var btn = $("vpn-bench-run");
      btn.disabled = true;
      $("vpn-bench-wrap").style.display = "";
      $("vpn-bench-title").textContent = "Results - testing " + hosts.length + " host(s)...";
      $("vpn-bench-body").innerHTML = "";
      api.post("/v1/vpn/benchmark", { hosts: hosts, count: Number($("vpn-bench-count").value) || 3 })
        .then(function (r) {
          var rows = list(r && r.results).map(function (s, i) {
            return [String(i + 1), esc(s.host), s.avg_ms != null ? Number(s.avg_ms).toFixed(1) + " ms" : "-",
              s.ok ? (s.host === r.best ? '<span class="hint-color">Fastest</span>' : "OK") : '<span class="hint-color">Unreachable</span>'];
          });
          $("vpn-bench-title").textContent = "Results" + (r && r.best ? " - fastest: " + r.best : "");
          $("vpn-bench-body").innerHTML = listRows(BENCH_COLS, rows);
        }, function (e) {
          $("vpn-bench-title").textContent = "Results - " + errText(e);
        })
        .then(function () { btn.disabled = false; });
    });
  }

  // ------------------------------------------------------------ LAN

  var RES_COLS = [["Client Name (MAC Address)", "36%"], ["IP Address", "22%"], ["Host Name", "30%"], ["Add / Delete", "12%"]];
  var CLIENT_COLS = [["Client Name", "22%", "text-align:left;padding-left:10px"], ["IP", "14%"], ["MAC", "17%"], ["Connection", "13%"], ["Signal", "10%"], ["Rate", "10%"], ["Reserved IP", "14%"]];

  var lan = { clients: [], reservations: [], radios: [] };

  SECTIONS.lan = {
    title: "LAN - AXOS",
    desc: "Reserve IP addresses for your devices and see everything on the LAN. Reserved addresses keep VPN routing rules and port forwards pointing at the right device.",
    refresh: 30000,
    render: function () {
      return (
        '<table width="100%" border="1" align="center" cellpadding="4" cellspacing="0" class="FormTable_table axos-block">' +
        '<thead><tr><td colspan="4" id="lan-res-title">Manually Assigned IP around the DHCP list (Max Limit : 128)</td></tr></thead>' +
        "<tr>" + RES_COLS.map(function (c) { return '<th width="' + c[1] + '">' + c[0] + "</th>"; }).join("") + "</tr>" +
        "<tr>" +
        '<td width="36%">' + select("lan-res-pick", [["", "Select a client"]], "") + '<br><input type="text" id="lan-res-mac" maxlength="17" class="input_20_table" placeholder="AA:BB:CC:DD:EE:FF" autocomplete="off"></td>' +
        '<td width="22%"><input type="text" id="lan-res-ip" maxlength="15" class="input_15_table" autocomplete="off"></td>' +
        '<td width="30%"><input type="text" id="lan-res-name" maxlength="32" class="input_15_table" autocomplete="off"></td>' +
        '<td width="12%"><input type="button" class="add_btn" id="lan-res-add" value=""></td>' +
        "</tr></table>" +
        '<div id="lan-res-body"></div>' +
        formTable("Filter", row("Search", '<input type="text" id="lan-filter" class="input_25_table" placeholder="name, IP or MAC" autocomplete="off">'), ' style="margin-top:18px"') +
        listTable({ id: "lan-clients-title", text: "Clients" }, CLIENT_COLS, "lan-clients-body")
      );
    },
    bind: function () {
      on("lan-res-pick", "change", function () {
        var mac = $("lan-res-pick").value;
        lan.clients.forEach(function (c) {
          if (macKey(c.mac) !== mac) return;
          $("lan-res-mac").value = macKey(c.mac);
          $("lan-res-ip").value = c.ip || "";
          $("lan-res-name").value = (c.hostname || "").replace(/[^A-Za-z0-9-]/g, "-").slice(0, 32);
        });
      });
      on("lan-res-add", "click", function () {
        addReservation($("lan-res-mac").value.trim(), $("lan-res-ip").value.trim(), $("lan-res-name").value.trim(), function () {
          $("lan-res-mac").value = $("lan-res-ip").value = $("lan-res-name").value = "";
          $("lan-res-pick").value = "";
        });
      });
      onClick("lan-res-body", "axos-res-del", function (t) {
        var mac = t.getAttribute("data-mac");
        if (!confirm("Delete the reservation for " + mac + "?")) return;
        withRollback("web-ui dhcp delete " + mac, function () {
          return api.del("/v1/dhcp/reservations/" + encodeURIComponent(mac));
        }).then(SECTIONS.lan.load, fail);
      });
      onClick("lan-clients-body", "axos-reserve", function (t) {
        addReservation(t.getAttribute("data-mac"), t.getAttribute("data-ip"), (t.getAttribute("data-name") || "").replace(/[^A-Za-z0-9-]/g, "-").slice(0, 32));
      });
      on("lan-filter", "input", renderLanClients);
    },
    load: function () {
      return Promise.all([
        api.get("/v1/clients"),
        optional(api.get("/v1/dhcp/reservations"), []),
        optional(api.get("/v1/wifi"), []),
      ]).then(function (d) {
        lan.clients = list(d[0]);
        lan.reservations = list(d[1]);
        lan.radios = list(d[2]);
        var pick = $("lan-res-pick");
        var cur = pick.value;
        pick.innerHTML = '<option value="">Select a client</option>' + lan.clients.map(function (c) {
          return '<option value="' + esc(macKey(c.mac)) + '">' + esc(clientName(c) + " (" + macKey(c.mac) + ")") + "</option>";
        }).join("");
        pick.value = cur;
        $("lan-res-title").textContent = "Manually Assigned IP around the DHCP list (" + lan.reservations.length + " / 128)";
        var names = {};
        lan.clients.forEach(function (c) { names[macKey(c.mac)] = c.hostname; });
        $("lan-res-body").innerHTML = listRows(RES_COLS, lan.reservations.map(function (r) {
          var name = names[macKey(r.mac)];
          return [(name ? esc(name) + "<br>" : "") + esc(macKey(r.mac)), esc(r.ip), esc(r.hostname || ""),
            '<input class="remove_btn axos-res-del" type="button" value="" data-mac="' + esc(r.mac) + '">'];
        }));
        renderLanClients();
      });
    },
  };

  function addReservation(mac, ip, name, done) {
    if (!isMAC(mac)) { alert("Enter a valid MAC address."); return; }
    if (!isIPv4(ip)) { alert("Enter a valid IP address."); return; }
    for (var i = 0; i < lan.reservations.length; i++) {
      var r = lan.reservations[i];
      if (r.ip === ip && macKey(r.mac) !== macKey(mac)) { alert(ip + " is already reserved for " + macKey(r.mac) + "."); return; }
    }
    if (lan.reservations.length >= 128) { alert("The reservation list is full (128)."); return; }
    withRollback("web-ui dhcp reserve " + macKey(mac), function () {
      return api.post("/v1/dhcp/reservations", { mac: macKey(mac), ip: ip, hostname: name || "" });
    }).then(function () { if (done) done(); SECTIONS.lan.load(); }, fail);
  }

  function bandOf(iface) {
    for (var i = 0; i < lan.radios.length; i++) if (lan.radios[i].interface === iface) return lan.radios[i].band;
    return "";
  }

  function signalText(rssi) {
    if (rssi == null) return "-";
    var q = rssi >= -55 ? "Excellent" : rssi >= -67 ? "Good" : rssi >= -75 ? "Fair" : "Weak";
    return esc(rssi + " dBm") + (q === "Weak" || q === "Fair" ? ' <span class="hint-color">' + q + "</span>" : " " + q);
  }

  function renderLanClients() {
    var filter = String(($("lan-filter") || {}).value || "").toLowerCase();
    var res = {};
    lan.reservations.forEach(function (r) { res[macKey(r.mac)] = r; });
    var rows = [];
    lan.clients.slice().sort(function (a, b) { return clientName(a).localeCompare(clientName(b)); }).forEach(function (c) {
      var hay = (clientName(c) + " " + c.ip + " " + c.mac).toLowerCase();
      if (filter && hay.indexOf(filter) < 0) return;
      var r = res[macKey(c.mac)];
      var band = bandOf(c.interface);
      rows.push([
        esc(clientName(c)), esc(c.ip || "-"), esc(macKey(c.mac)),
        c.wireless ? esc(band || c.interface || "Wireless") : "Wired" + (c.interface ? " (" + esc(c.interface) + ")" : ""),
        c.wireless ? signalText(c.rssi_dbm) : "-",
        c.phy_rate_mbps != null ? esc(Math.round(c.phy_rate_mbps) + " Mbps") : "-",
        r ? (r.ip === c.ip ? "Yes" : '<span class="hint-color">' + esc(r.ip) + "</span>")
          : c.ip ? '<input type="button" class="button_gen axos-reserve axos-small" value="Reserve" data-mac="' + esc(macKey(c.mac)) +
            '" data-ip="' + esc(c.ip) + '" data-name="' + esc(c.hostname || "") + '">' : "-",
      ]);
    });
    $("lan-clients-title").textContent = "Clients (" + rows.length + " / " + lan.clients.length + ")";
    $("lan-clients-body").innerHTML = listRows(CLIENT_COLS, rows, "No clients match.");
  }

  // ------------------------------------------------------------ WAN / DNS

  var DOT_COLS = [["Address", "30%"], ["TLS Port", "15%"], ["TLS Hostname", "43%"], ["Add / Delete", "12%"]];
  var DOT_PRESETS = [
    ["", "Presets"],
    ["1.1.1.1|853|cloudflare-dns.com", "Cloudflare (1.1.1.1)"],
    ["1.0.0.1|853|cloudflare-dns.com", "Cloudflare (1.0.0.1)"],
    ["9.9.9.9|853|dns.quad9.net", "Quad9 (9.9.9.9)"],
    ["149.112.112.112|853|dns.quad9.net", "Quad9 (149.112.112.112)"],
    ["8.8.8.8|853|dns.google", "Google (8.8.8.8)"],
    ["8.8.4.4|853|dns.google", "Google (8.8.4.4)"],
  ];
  var WAN_COLS = [["Interface", "14%"], ["State", "10%"], ["Address", "30%"], ["Link", "14%"], ["Received", "16%"], ["Sent", "16%"]];

  var dns = { cfg: {}, dot: [], dirty: false };

  // dnspriv_rulelist: <addr>port>hostname>spkipin
  function parseDoT(raw) {
    return String(raw || "").split("<").filter(Boolean).map(function (e) {
      var f = e.split(">");
      return { addr: f[0] || "", port: f[1] || "", host: f[2] || "", spki: f[3] || "" };
    }).filter(function (d) { return d.addr; });
  }
  function formatDoT(list_) {
    return list_.map(function (d) { return "<" + d.addr + ">" + (d.port || "") + ">" + (d.host || "") + ">" + (d.spki || ""); }).join("");
  }

  SECTIONS.dns = {
    title: "WAN - AXOS DNS",
    desc: "WAN DNS servers, DNS-over-TLS, and the DNS servers handed to LAN clients. Applying a WAN DNS change reconnects the WAN for a few seconds, exactly like WAN → Internet Connection.",
    refresh: 30000,
    render: function () {
      return (
        formTable("WAN DNS Setting",
          row("Connect to DNS Server automatically", yesNo("dns-auto", true)) +
          row("DNS Server1", '<input type="text" id="dns-wan1" maxlength="15" class="input_15_table" autocomplete="off">') +
          row("DNS Server2", '<input type="text" id="dns-wan2" maxlength="15" class="input_15_table" autocomplete="off"> <span id="dns-isp"></span>') +
          row("DNS Privacy Protocol", select("dns-dot", [["0", "None"], ["1", "DNS-over-TLS (DoT)"]], "0")) +
          row("DNS-over-TLS Profile", select("dns-dot-profile", [["1", "Strict"], ["0", "Opportunistic"]], "1"), ' id="dns-dot-profile-row"')) +
        '<div id="dns-dot-wrap">' +
        '<table width="100%" border="1" align="center" cellpadding="4" cellspacing="0" class="FormTable_table axos-block">' +
        '<thead><tr><td colspan="4">DNS-over-TLS Server List ( Max Limit : 8 )</td></tr></thead>' +
        "<tr>" + DOT_COLS.map(function (c) { return '<th width="' + c[1] + '">' + c[0] + "</th>"; }).join("") + "</tr>" +
        "<tr>" +
        '<td width="30%"><input type="text" id="dot-addr" class="input_15_table" autocomplete="off"><br>' + select("dot-preset", DOT_PRESETS, "") + "</td>" +
        '<td width="15%"><input type="text" id="dot-port" maxlength="5" class="input_6_table" placeholder="853" autocomplete="off"></td>' +
        '<td width="43%"><input type="text" id="dot-host" class="input_25_table" autocomplete="off"></td>' +
        '<td width="12%"><input type="button" class="add_btn" id="dot-add" value=""></td></tr></table>' +
        '<div id="dot-body"></div></div>' +
        formTable("LAN DNS (given to DHCP clients)",
          row("DNS Server1", '<input type="text" id="dns-lan1" maxlength="15" class="input_15_table" autocomplete="off">') +
          row("DNS Server2", '<input type="text" id="dns-lan2" maxlength="15" class="input_15_table" autocomplete="off"> <span>Empty = the router itself</span>'),
          ' style="margin-top:18px"') +
        applyGen(button("dns-apply", "Apply")) +
        listTable({ id: "dns-wan-title", text: "WAN status" }, WAN_COLS, "dns-wan-body") +
        formTable("DNS lookup test",
          row("Host name", '<input type="text" id="dns-test-name" class="input_25_table" value="www.asus.com" autocomplete="off"> ' + button("dns-test", "Lookup")),
          ' style="margin-top:18px"') +
        textarea("dns-test-out", 8)
      );
    },
    bind: function () {
      function syncAuto() {
        var auto = radioValue("dns-auto");
        $("dns-wan1").disabled = auto;
        $("dns-wan2").disabled = auto;
      }
      function syncDot() {
        var on_ = $("dns-dot").value === "1";
        $("dns-dot-wrap").style.display = on_ ? "" : "none";
        $("dns-dot-profile-row").style.display = on_ ? "" : "none";
      }
      var autos = document.getElementsByName("dns-auto");
      for (var i = 0; i < autos.length; i++) autos[i].onclick = function () { dns.dirty = true; syncAuto(); };
      on("dns-dot", "change", function () { dns.dirty = true; syncDot(); });
      ["dns-wan1", "dns-wan2", "dns-lan1", "dns-lan2", "dns-dot-profile"].forEach(function (id) {
        on(id, "change", function () { dns.dirty = true; });
      });
      dns.syncAuto = syncAuto;
      dns.syncDot = syncDot;
      on("dot-preset", "change", function () {
        var v = $("dot-preset").value.split("|");
        if (v.length === 3) { $("dot-addr").value = v[0]; $("dot-port").value = v[1]; $("dot-host").value = v[2]; }
        $("dot-preset").value = "";
      });
      on("dot-add", "click", function () {
        var d = { addr: $("dot-addr").value.trim(), port: $("dot-port").value.trim(), host: $("dot-host").value.trim(), spki: "" };
        if (!isIPv4(d.addr) && !isIPv6(d.addr)) { alert("Address must be an IP address."); return; }
        if (d.port && !(/^\d+$/.test(d.port) && Number(d.port) > 0 && Number(d.port) < 65536)) { alert("Invalid port."); return; }
        if (/[<>]/.test(d.host)) { alert("Hostname cannot contain < or >."); return; }
        if (dns.dot.length >= 8) { alert("At most 8 servers."); return; }
        dns.dot.push(d);
        dns.dirty = true;
        $("dot-addr").value = $("dot-port").value = $("dot-host").value = "";
        renderDoT();
      });
      onClick("dot-body", "axos-dot-del", function (t) {
        dns.dot.splice(Number(t.getAttribute("data-idx")), 1);
        dns.dirty = true;
        renderDoT();
      });
      on("dns-apply", "click", function () {
        var auto = radioValue("dns-auto");
        var wan = auto ? [] : [$("dns-wan1").value.trim(), $("dns-wan2").value.trim()].filter(Boolean);
        var lanDNS = [$("dns-lan1").value.trim(), $("dns-lan2").value.trim()].filter(Boolean);
        var bad = wan.concat(lanDNS).filter(function (ip) { return !isIPv4(ip); });
        if (bad.length) { alert("Not an IP address: " + bad.join(", ")); return; }
        if (!auto && !wan.length) { alert("Enter at least one DNS server, or choose automatic."); return; }
        var dotOn = $("dns-dot").value === "1";
        if (dotOn && !dns.dot.length) { alert("Add at least one DNS-over-TLS server."); return; }
        withRollback("web-ui dns", function () {
          return api.post("/v1/dns", {
            wan_upstreams: wan,
            lan_upstreams: lanDNS,
            dot_enabled: dotOn,
            dot_profile: $("dns-dot-profile").value,
            dot_rules: formatDoT(dns.dot),
          });
        }).then(function () { dns.dirty = false; SECTIONS.dns.load(); }, fail);
      });
      on("dns-test", "click", function () {
        var out = $("dns-test-out");
        out.value = "Looking up...";
        api.post("/v1/diag/dns", { name: $("dns-test-name").value.trim() || "www.asus.com" })
          .then(function (r) { out.value = (r.ok ? "" : "FAILED\n") + (r.output || JSON.stringify(r, null, 2)); },
            function (e) { out.value = errText(e); });
      });
    },
    load: function () {
      return Promise.all([
        api.get("/v1/dns"),
        optional(api.get("/v1/interfaces"), []),
        optional(api.get("/v1/routes"), []),
      ]).then(function (d) {
        var cfg = d[0] || {};
        dns.cfg = cfg;
        if (!dns.dirty) {
          setRadio("dns-auto", cfg.wan_dns_auto !== false);
          var w = list(cfg.wan_upstreams), l = list(cfg.lan_upstreams);
          $("dns-wan1").value = cfg.wan_dns_auto === false ? w[0] || "" : "";
          $("dns-wan2").value = cfg.wan_dns_auto === false ? w[1] || "" : "";
          $("dns-isp").innerHTML = cfg.wan_dns_auto !== false && w.length ? "From ISP: " + esc(w.join(", ")) : "";
          $("dns-lan1").value = l[0] || "";
          $("dns-lan2").value = l[1] || "";
          $("dns-dot").value = cfg.dot_enabled ? "1" : "0";
          $("dns-dot-profile").value = cfg.dot_profile === "0" ? "0" : "1";
          dns.dot = parseDoT(cfg.dot_rules);
          dns.syncAuto();
          dns.syncDot();
          renderDoT();
        }
        var wan = list(d[1]).filter(function (i) { return i.role === "wan"; });
        var gw = list(d[2]).filter(function (r) { return r.destination === "default"; })[0];
        $("dns-wan-title").textContent = "WAN status" + (gw ? " - default route via " + (gw.gateway || gw.interface) + " (" + gw.interface + ")" : "");
        $("dns-wan-body").innerHTML = listRows(WAN_COLS, wan.map(function (i) {
          return [esc(i.name), i.state === "up" ? "Up" : '<span class="hint-color">' + esc(i.state) + "</span>",
            esc(list(i.addresses).join(", ") || "-"), esc(i.link_speed || "-"), esc(fmtBytes(i.rx_bytes)), esc(fmtBytes(i.tx_bytes))];
        }), "No WAN interface found.");
      });
    },
  };

  function renderDoT() {
    $("dot-body").innerHTML = listRows(DOT_COLS, dns.dot.map(function (d, i) {
      return [esc(d.addr), esc(d.port || "853"), esc(d.host || "-"), '<input class="remove_btn axos-dot-del" type="button" value="" data-idx="' + i + '">'];
    }));
  }

  // ------------------------------------------------------------ Firewall

  var FW_COLS = [["Table", "10%"], ["Chain", "16%"], ["Rule", "62%", "text-align:left;padding-left:10px;font-family:Lucida Console,monospace;word-break:break-all"], ["Delete", "12%"]];
  var FW_LIMIT = 100;
  var fw = { rules: [], showAll: false };

  SECTIONS.firewall = {
    title: "Firewall - AXOS",
    desc: "Live iptables rules on the router. Rules added here take effect immediately but are runtime-only: a firewall restart (or reboot) removes them. Each add or delete is protected by the " +
      ROLLBACK_SECONDS / 60 + "-minute safety rollback.",
    refresh: 30000,
    render: function () {
      return (
        formTable("Filter",
          row("Table", select("fw-table", [["", "All"], ["filter", "filter"], ["nat", "nat"], ["mangle", "mangle"], ["raw", "raw"]], "filter")) +
          row("Chain", select("fw-chain", [["", "All"]], "")) +
          row("Search", '<input type="text" id="fw-search" class="input_25_table" placeholder="e.g. 443 or ACCEPT" autocomplete="off">')) +
        '<table width="100%" border="1" align="center" cellpadding="4" cellspacing="0" class="FormTable_table axos-block">' +
        '<thead><tr><td colspan="4">Add rule</td></tr></thead>' +
        '<tr><th width="14%">Table</th><th width="20%">Chain</th><th width="54%">Rule (iptables arguments)</th><th width="12%">Add</th></tr>' +
        "<tr>" +
        '<td width="14%">' + select("fw-add-table", [["filter", "filter"], ["nat", "nat"], ["mangle", "mangle"], ["raw", "raw"]], "filter") + "</td>" +
        '<td width="20%"><input type="text" id="fw-add-chain" class="input_12_table" value="INPUT" autocomplete="off"></td>' +
        '<td width="54%"><input type="text" id="fw-add-rule" class="input_32_table" style="width:95%" placeholder="-p tcp --dport 8443 -j ACCEPT" autocomplete="off"></td>' +
        '<td width="12%"><input type="button" class="add_btn" id="fw-add" value=""></td></tr></table>' +
        listTable({ id: "fw-title", text: "Rules" }, FW_COLS, "fw-body") +
        applyGen(button("fw-more", "Show all"))
      );
    },
    bind: function () {
      on("fw-table", "change", function () { fillChains(); renderFw(); });
      on("fw-chain", "change", renderFw);
      on("fw-search", "input", renderFw);
      on("fw-more", "click", function () { fw.showAll = !fw.showAll; renderFw(); });
      on("fw-add", "click", function () {
        var rule = { table: $("fw-add-table").value, chain: $("fw-add-chain").value.trim(), rule: $("fw-add-rule").value.trim() };
        if (!/^[A-Za-z0-9_-]+$/.test(rule.chain)) { alert("Enter a chain name, e.g. INPUT."); return; }
        if (!rule.rule) { alert("Enter the rule, e.g. -p tcp --dport 8443 -j ACCEPT"); return; }
        if (!confirm("Append to " + rule.table + "/" + rule.chain + ":\n\n  " + rule.rule + "\n\nContinue?")) return;
        withRollback("web-ui firewall add", function () { return api.post("/v1/firewall/apply", rule); })
          .then(function () { $("fw-add-rule").value = ""; SECTIONS.firewall.load(); }, fail);
      });
      onClick("fw-body", "axos-fw-del", function (t) {
        var r = fw.view[Number(t.getAttribute("data-idx"))];
        if (!r) return;
        if (!confirm("Delete from " + (r.table || "filter") + "/" + r.chain + ":\n\n  " + r.rule + "\n\nContinue?")) return;
        withRollback("web-ui firewall delete", function () {
          return api.post("/v1/firewall/delete", { table: r.table || "filter", chain: r.chain, rule: r.rule });
        }).then(SECTIONS.firewall.load, fail);
      });
    },
    load: function () {
      return api.get("/v1/firewall").then(function (rules) {
        fw.rules = list(rules);
        fillChains();
        renderFw();
      });
    },
  };

  function fillChains() {
    var table = $("fw-table").value;
    var sel = $("fw-chain");
    var cur = sel.value;
    var seen = {}, chains = [];
    fw.rules.forEach(function (r) {
      if (table && (r.table || "filter") !== table) return;
      if (!seen[r.chain]) { seen[r.chain] = true; chains.push(r.chain); }
    });
    chains.sort();
    sel.innerHTML = '<option value="">All</option>' + chains.map(function (c) {
      return '<option value="' + esc(c) + '">' + esc(c) + "</option>";
    }).join("");
    if (seen[cur]) sel.value = cur;
  }

  function renderFw() {
    var table = $("fw-table").value, chain = $("fw-chain").value, q = $("fw-search").value.toLowerCase();
    var view = fw.rules.filter(function (r) {
      if (table && (r.table || "filter") !== table) return false;
      if (chain && r.chain !== chain) return false;
      return !q || (r.chain + " " + r.rule).toLowerCase().indexOf(q) >= 0;
    });
    var total = view.length;
    if (!fw.showAll) view = view.slice(0, FW_LIMIT);
    fw.view = view;
    $("fw-title").textContent = "Rules (" + (view.length < total ? view.length + " of " : "") + total + ")";
    $("fw-more").value = fw.showAll ? "Show first " + FW_LIMIT : "Show all";
    $("fw-more").style.display = total > FW_LIMIT ? "" : "none";
    $("fw-body").innerHTML = listRows(FW_COLS, view.map(function (r, i) {
      return [esc(r.table || "filter"), esc(r.chain), esc(r.rule), '<input class="remove_btn axos-fw-del" type="button" value="" data-idx="' + i + '">'];
    }), "No rules match.");
  }

  // ------------------------------------------------------------ QoS

  var RATE_COLS = [["Interface", "20%"], ["Role", "14%"], ["Download", "22%"], ["Upload", "22%"], ["Total received", "22%"]];

  SECTIONS.qos = {
    title: "Adaptive QoS - AXOS",
    desc: "Turn QoS on or off and watch live throughput. Applying restarts QoS and the firewall (a few seconds), the same as Adaptive QoS → QoS.",
    refresh: 5000,
    render: function () {
      return (
        formTable("QoS",
          row("Enable QoS", yesNo("qos-enable", false)) +
          row("QoS type", '<span id="qos-mode">-</span>') +
          row("Upload Bandwidth", '<span id="qos-up">-</span>') +
          row("Download Bandwidth", '<span id="qos-down">-</span>')) +
        applyGen(button("qos-apply", "Apply")) +
        listTable({ id: "qos-rate-title", text: "Live throughput" }, RATE_COLS, "qos-rate-body")
      );
    },
    bind: function () {
      var r = document.getElementsByName("qos-enable");
      for (var i = 0; i < r.length; i++) r[i].onclick = function () { state.qosDirty = true; };
      on("qos-apply", "click", function () {
        var enabled = radioValue("qos-enable");
        withRollback("web-ui qos " + (enabled ? "on" : "off"), function () { return api.post("/v1/qos", { enabled: enabled }); })
          .then(function () { state.qosDirty = false; SECTIONS.qos.load(); }, fail);
      });
    },
    load: function () {
      return Promise.all([optional(api.get("/v1/qos"), {}), optional(api.get("/v1/interfaces"), [])]).then(function (d) {
        var q = d[0] || {};
        if (!state.qosDirty) setRadio("qos-enable", !!q.enabled);
        var types = { 0: "Traditional QoS", 1: "Adaptive QoS", 2: "Bandwidth Limiter", 3: "Cake / FQ" };
        $("qos-mode").textContent = q.mode || types[q.method] || (q.method != null ? "Type " + q.method : "-");
        function bw(kbps) { var n = Number(kbps); return n ? (n / 1024).toFixed(1) + " Mb/s" : "Automatic"; }
        $("qos-up").textContent = bw(q.obw_kbps);
        $("qos-down").textContent = bw(q.ibw_kbps);
        renderRates(list(d[1]));
      });
    },
  };

  function renderRates(ifaces) {
    var now = Date.now();
    var prev = state.lastIfaces;
    var byName = {};
    if (prev) prev.list.forEach(function (i) { byName[i.name] = i; });
    var dt = prev ? (now - prev.at) / 1000 : 0;
    var rows = ifaces.filter(function (i) { return i.role === "wan" || i.role === "lan" && i.type === "bridge"; }).map(function (i) {
      var p = byName[i.name];
      var down = p && dt > 0 ? Math.max(0, (i.rx_bytes - p.rx_bytes) / dt) : null;
      var up = p && dt > 0 ? Math.max(0, (i.tx_bytes - p.tx_bytes) / dt) : null;
      // For the WAN, rx is download; for the LAN bridge the direction flips.
      if (i.role !== "wan") { var t = down; down = up; up = t; }
      return [esc(i.name), esc((i.role || "").toUpperCase()), down == null ? "measuring..." : fmtRate(down), up == null ? "measuring..." : fmtRate(up), esc(fmtBytes(i.rx_bytes))];
    });
    state.lastIfaces = { at: now, list: ifaces };
    $("qos-rate-body").innerHTML = listRows(RATE_COLS, rows, "No WAN/LAN interfaces reported.");
  }

  // ------------------------------------------------------------ Wireless

  var RADIO_COLS = [["Band", "10%"], ["Interface", "11%"], ["SSID", "25%"], ["Channel", "10%"], ["Bandwidth", "12%"], ["TX Power", "10%"], ["Clients", "10%"], ["Status", "12%"]];
  var WCLIENT_COLS = [["Client Name", "24%", "text-align:left;padding-left:10px"], ["IP", "15%"], ["MAC", "19%"], ["Band", "12%"], ["Signal", "18%"], ["PHY Rate", "12%"]];

  SECTIONS.wifi = {
    title: "Wireless - AXOS",
    desc: "Radio settings in use and the signal each wireless client gets. Weak clients are listed first so problems stand out.",
    refresh: 15000,
    render: function () {
      return listTable("Radios", RADIO_COLS, "wifi-radio-body") +
        listTable({ id: "wifi-clients-title", text: "Wireless clients" }, WCLIENT_COLS, "wifi-clients-body");
    },
    bind: function () {},
    load: function () {
      return Promise.all([api.get("/v1/wifi"), api.get("/v1/clients")]).then(function (d) {
        lan.radios = list(d[0]);
        $("wifi-radio-body").innerHTML = listRows(RADIO_COLS, lan.radios.map(function (r) {
          return [esc(r.band || "-"), esc(r.interface), esc(r.ssid || "-"), esc(r.channel || "Auto"),
            r.channel_width_mhz ? esc(r.channel_width_mhz + " MHz") : "-", r.tx_power_pct ? esc(r.tx_power_pct + "%") : "-",
            esc(r.client_count || 0), r.enabled === false ? '<span class="hint-color">Disabled</span>' : "Enabled"];
        }), "No radios reported.");
        var wc = list(d[1]).filter(function (c) { return c.wireless; }).sort(function (a, b) {
          return (a.rssi_dbm == null ? 0 : a.rssi_dbm) - (b.rssi_dbm == null ? 0 : b.rssi_dbm);
        });
        $("wifi-clients-title").textContent = "Wireless clients (" + wc.length + ")";
        $("wifi-clients-body").innerHTML = listRows(WCLIENT_COLS, wc.map(function (c) {
          return [esc(clientName(c)), esc(c.ip || "-"), esc(macKey(c.mac)), esc(bandOf(c.interface) || c.interface || "-"),
            signalText(c.rssi_dbm), c.phy_rate_mbps != null ? esc(Math.round(c.phy_rate_mbps) + " Mbps") : "-"];
        }), "No wireless clients.");
      });
    },
  };

  // ------------------------------------------------------------ Tools

  SECTIONS.diag = {
    title: "Network Tools - AXOS",
    desc: "Run diagnostics from the router itself: ping, traceroute, nslookup, TCP port check, and iperf3 throughput against a server you run.",
    refresh: 0,
    render: function () {
      return (
        formTable("",
          row("Method", select("diag-method", [["ping", "Ping"], ["traceroute", "Traceroute"], ["nslookup", "Nslookup"], ["port", "Port check"], ["iperf3", "iperf3 (client)"]], "ping")) +
          row("Target", '<input type="text" id="diag-target" class="input_32_table" maxlength="100" placeholder="ex: www.google.com" autocomplete="off"> ' +
            select("diag-pick", [["", "LAN clients"]], "")) +
          row("Count", '<input type="text" id="diag-count" class="input_3_table" maxlength="2" placeholder="5" autocomplete="off">', ' id="diag-row-count"') +
          row("Max hops", '<input type="text" id="diag-hops" class="input_3_table" maxlength="2" placeholder="20" autocomplete="off">', ' id="diag-row-hops"') +
          row("Port", '<input type="text" id="diag-port" class="input_6_table" maxlength="5" placeholder="443" autocomplete="off">', ' id="diag-row-port"') +
          row("Duration (seconds)", '<input type="text" id="diag-secs" class="input_3_table" maxlength="2" placeholder="10" autocomplete="off">', ' id="diag-row-secs"') +
          row("Direction", '<input type="radio" name="diag-rev" value="0" class="input" checked>Upload (router → server) ' +
            '<input type="radio" name="diag-rev" value="1" class="input">Download', ' id="diag-row-rev"')) +
        applyGen(button("diag-run", "Diagnose") + ' <span id="diag-spin" class="axos-spin" style="display:none"></span>') +
        '<div style="margin-top:8px">' + textarea("diag-out", 24) + "</div>"
      );
    },
    bind: function () {
      function sync() {
        var m = $("diag-method").value;
        $("diag-row-count").style.display = m === "ping" ? "" : "none";
        $("diag-row-hops").style.display = m === "traceroute" ? "" : "none";
        $("diag-row-port").style.display = m === "port" || m === "iperf3" ? "" : "none";
        $("diag-row-secs").style.display = m === "iperf3" ? "" : "none";
        $("diag-row-rev").style.display = m === "iperf3" ? "" : "none";
        $("diag-port").placeholder = m === "iperf3" ? "5201" : "443";
      }
      on("diag-method", "change", sync);
      sync();
      on("diag-pick", "change", function () { if ($("diag-pick").value) $("diag-target").value = $("diag-pick").value; $("diag-pick").value = ""; });
      api.get("/v1/clients").then(function (cs) {
        $("diag-pick").innerHTML = '<option value="">LAN clients</option>' + list(cs).filter(function (c) { return c.ip; }).map(function (c) {
          return '<option value="' + esc(c.ip) + '">' + esc(clientName(c) + " (" + c.ip + ")") + "</option>";
        }).join("");
      }, function () {});
      on("diag-target", "keydown", function (ev) { if (ev.keyCode === 13) { ev.preventDefault(); $("diag-run").click(); } });
      on("diag-run", "click", function () {
        var m = $("diag-method").value;
        var target = $("diag-target").value.trim();
        if (!target) { alert("Enter a target."); return; }
        if (!/^[A-Za-z0-9.:_-]+$/.test(target)) { alert("The target must be a host name or IP address."); return; }
        var path, body;
        if (m === "ping") { path = "/v1/diag/ping"; body = { host: target, count: Number($("diag-count").value) || 5 }; }
        else if (m === "traceroute") { path = "/v1/diag/traceroute"; body = { host: target, max_hops: Number($("diag-hops").value) || 20 }; }
        else if (m === "nslookup") { path = "/v1/diag/dns"; body = { name: target }; }
        else if (m === "port") { path = "/v1/diag/port"; body = { host: target, port: Number($("diag-port").value) || 443 }; }
        else {
          path = "/v1/perf/iperf3";
          body = { mode: "client", target: target, port: Number($("diag-port").value) || 5201, seconds: Number($("diag-secs").value) || 10, reverse: radioValue("diag-rev") };
        }
        var out = $("diag-out"), btn = $("diag-run");
        btn.disabled = true;
        $("diag-spin").style.display = "";
        out.value = "Running " + m + " " + target + " ...\n";
        api.post(path, body).then(function (r) {
          var text = r.output || r.summary || JSON.stringify(r, null, 2);
          if (r.summary && r.output) text = r.summary + "\n\n" + r.output;
          out.value = (r.ok === false ? "FAILED\n\n" : "") + text;
        }, function (e) {
          out.value = errText(e);
        }).then(function () {
          btn.disabled = false;
          $("diag-spin").style.display = "none";
        });
      });
    },
    load: function () { return Promise.resolve(); },
  };

  // ============================================================ init

  function sectionHref(id) {
    if (EMBED === "merlin") return "/" + MERLIN_PAGES[id];
    return "#" + id;
  }

  function currentSection() {
    var s = String(global.AXOS_SECTION || "").toLowerCase();
    if (EMBED !== "merlin" && global.location.hash) s = global.location.hash.slice(1).toLowerCase();
    return SECTIONS[s] ? s : "all";
  }

  function load() {
    var sec = SECTIONS[state.section];
    return sec.load().then(function () { setConn(""); }, function (e) {
      if (e && e.status === 401 && EMBED !== "merlin") {
        askToken();
        return;
      }
      var hint = e && e.status === 401
        ? "The AXOS UI token was rejected. Run /jffs/axos/bin/axos-merlin-ui.sh, then log out and back in."
        : "Cannot reach the AXOS Core API (" + errText(e) + "). Check that axosd is running on the router.";
      setConn(hint);
    });
  }

  function askToken() {
    var t = global.prompt("Enter the AXOS UI token (on the router: cat /jffs/axos/run/ui.token):", "");
    if (!t) { setConn("An AXOS UI token is required to use this page from the LAN."); return; }
    try { global.localStorage.setItem(TOKEN_KEY, t.trim()); } catch (e) { global.AXOS_UI_TOKEN = t.trim(); }
    load();
  }

  function show(id) {
    state.section = id;
    var sec = SECTIONS[id];
    if (state.timer) { clearInterval(state.timer); state.timer = null; }
    state.lastIfaces = null;
    var title = $("axos-title"), desc = $("axos-desc");
    if (title) title.textContent = sec.title;
    if (desc) desc.textContent = sec.desc;
    if (EMBED !== "merlin") document.title = "AXOS - " + sec.title;
    $("axos-root").innerHTML = sec.render();
    sec.bind();
    load();
    if (sec.refresh) {
      state.timer = setInterval(function () {
        if (document.hidden) return;
        var pop = $("rule-editor");
        if (pop && pop.style.display === "block") return; // don't refresh under an open editor
        load();
      }, sec.refresh);
    }
    if (typeof global.axosOnSection === "function") global.axosOnSection(id);
  }

  global.AXOS = {
    init: function () {
      try {
        if (EMBED === "merlin" && !token()) setConn("Missing AXOS UI token. Run /jffs/axos/bin/axos-merlin-ui.sh, then log out and back in.");
        show(currentSection());
        if (EMBED !== "merlin") {
          global.addEventListener("hashchange", function () { show(currentSection()); });
        }
      } catch (e) {
        setConn("AXOS UI failed to start: " + errText(e));
        if (global.console) global.console.error(e);
      }
    },
    sections: SECTIONS,
    // exported for tests
    _parseWGConf: parseWGConf,
    _parseDoT: parseDoT,
    _formatDoT: formatDoT,
    _isIPOrCIDR: isIPOrCIDR,
  };
  // Back-compat with pages that still call the old entry point.
  global.axosEmbedInit = global.AXOS.init;
})(window);
