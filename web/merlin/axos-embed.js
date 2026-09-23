/* AXOS Merlin embed — Core API on :9090 with UI token (LANAuth). */
(function (global) {
  function apiBase() {
    return "http://" + (window.location.hostname || "192.168.50.1") + ":9090";
  }
  function token() {
    return global.AXOS_UI_TOKEN || "";
  }
  function headers(extra) {
    var h = {
      Accept: "application/json",
      "X-Axos-Actor": "merlin-ui",
      "X-Axos-UI-Token": token(),
    };
    if (extra) for (var k in extra) h[k] = extra[k];
    return h;
  }
  function req(method, path, body) {
    var opts = { method: method, headers: headers() };
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
        if (!r.ok) throw new Error((data && data.error) || path + " " + r.status);
        return data;
      });
    });
  }
  function get(path) {
    return req("GET", path);
  }
  function post(path, body) {
    return req("POST", path, body || {});
  }
  function del(path) {
    return req("DELETE", path);
  }

  function setStatus(text, ok) {
    var el = document.getElementById("axos-status");
    if (!el) return;
    el.textContent = text;
    if (ok) el.classList.add("ok");
    else el.classList.remove("ok");
  }

  function esc(s) {
    return String(s == null ? "" : s)
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;");
  }

  function kvRows(pairs) {
    var html = "";
    for (var i = 0; i < pairs.length; i++) {
      html +=
        "<tr><th>" +
        esc(pairs[i][0]) +
        "</th><td>" +
        esc(pairs[i][1]) +
        "</td></tr>";
    }
    return html;
  }

  function fmtUptime(ns) {
    if (!ns) return "—";
    var sec = Math.floor(Number(ns) / 1e9);
    var d = Math.floor(sec / 86400);
    var h = Math.floor((sec % 86400) / 3600);
    var m = Math.floor((sec % 3600) / 60);
    return d + "d " + h + "h " + m + "m";
  }

  function withRollback(reason, fn) {
    return post("/v1/rollback/arm", { timeout_seconds: 120, reason: reason }).then(
      function (arm) {
        return fn().then(
          function (result) {
            return post("/v1/rollback/confirm", { id: arm.id }).then(function () {
              return result;
            });
          },
          function (err) {
            // Leave armed so auto-restore can undo a partial change.
            throw err;
          }
        );
      }
    );
  }

  function renderSys(info) {
    document.getElementById("axos-sys-body").innerHTML = kvRows([
      ["Model", info.model],
      ["Firmware", (info.firmware_version || "") + " " + (info.firmware_revision || "")],
      ["Serial", info.serial],
      ["Uptime", fmtUptime(info.uptime)],
      ["Boot", info.boot_time],
    ]);
  }

  function renderRes(res) {
    var temps = res.temperatures_c || {};
    var tparts = [];
    for (var k in temps) tparts.push(k + "=" + temps[k] + "°C");
    document.getElementById("axos-res-body").innerHTML = kvRows([
      ["Load (1/5/15)", [res.cpu_load_1m, res.cpu_load_5m, res.cpu_load_15m].join(" / ")],
      [
        "Memory",
        Math.round((res.mem_used_kb || 0) / 1024) +
          " / " +
          Math.round((res.mem_total_kb || 0) / 1024) +
          " MB",
      ],
      ["Temperatures", tparts.join(", ") || "—"],
    ]);
  }

  function renderWifi(radios) {
    var html = "";
    (radios || []).forEach(function (r) {
      html +=
        "<tr><td>" +
        esc(r.interface || r.name || "?") +
        " (" +
        esc(r.band || "") +
        ")</td><td>" +
        esc(r.ssid || "—") +
        "</td><td>" +
        esc(r.channel != null ? r.channel : "—") +
        "</td><td>" +
        esc(r.client_count != null ? r.client_count : 0) +
        "</td></tr>";
    });
    document.getElementById("axos-wifi-body").innerHTML =
      html || "<tr><td colspan='4'>No radios</td></tr>";
  }

  function renderClients(clients) {
    var html = "";
    (clients || []).slice(0, 40).forEach(function (c) {
      html +=
        "<tr><td>" +
        esc(c.hostname || "—") +
        "</td><td>" +
        esc(c.ip || "—") +
        "</td><td>" +
        esc(c.mac || "—") +
        "</td><td>" +
        esc(c.interface || "—") +
        "</td></tr>";
    });
    document.getElementById("axos-clients-body").innerHTML =
      html || "<tr><td colspan='4'>No clients</td></tr>";
  }

  function renderVpn(profiles) {
    var html = "";
    (profiles || []).forEach(function (p) {
      html +=
        "<tr><td>" +
        esc(p.name) +
        "</td><td>" +
        esc(p.type) +
        "</td><td>" +
        esc(p.endpoint || p.description || "—") +
        "</td><td>" +
        (p.enabled ? "yes" : "no") +
        '</td><td class="axos-actions">' +
        '<input type="button" class="button_gen axos-vpn-up" data-name="' +
        esc(p.name) +
        '" value="Up"> ' +
        '<input type="button" class="button_gen axos-vpn-down" data-name="' +
        esc(p.name) +
        '" value="Down">' +
        "</td></tr>";
    });
    document.getElementById("axos-vpn-body").innerHTML =
      html || "<tr><td colspan='5'>No profiles</td></tr>";
  }

  function renderDhcp(list) {
    var html = "";
    (list || []).forEach(function (r) {
      html +=
        "<tr><td>" +
        esc(r.mac) +
        "</td><td>" +
        esc(r.ip) +
        "</td><td>" +
        esc(r.hostname || "—") +
        '</td><td><input type="button" class="button_gen axos-dhcp-del" data-mac="' +
        esc(r.mac) +
        '" value="Delete"></td></tr>";
    });
    document.getElementById("axos-dhcp-body").innerHTML =
      html || "<tr><td colspan='4'>None</td></tr>";
  }

  function renderPolicy(list) {
    var html = "";
    (list || []).forEach(function (r) {
      html +=
        "<tr><td>" +
        esc(r.id) +
        "</td><td>" +
        esc(r.source) +
        "</td><td>" +
        esc(r.interface) +
        "</td><td>" +
        (r.enabled ? "yes" : "no") +
        "</td><td>" +
        esc(r.description || "—") +
        '</td><td><input type="button" class="button_gen axos-pol-del" data-id="' +
        esc(r.id) +
        '" value="Delete"></td></tr>';
    });
    document.getElementById("axos-policy-body").innerHTML =
      html || "<tr><td colspan='6'>None</td></tr>";
  }

  function renderFw(rules) {
    var html = "";
    var n = 0;
    (rules || []).forEach(function (r) {
      if (r.table && r.table !== "filter") return;
      if (n >= 25) return;
      n++;
      html +=
        "<tr><td>" +
        esc(r.chain) +
        "</td><td><code>" +
        esc(r.rule) +
        '</code></td><td></td></tr>';
    });
    document.getElementById("axos-fw-body").innerHTML =
      html || "<tr><td colspan='3'>None</td></tr>";
  }

  function refresh() {
    setStatus("loading…", false);
    Promise.all([
      get("/v1/info"),
      get("/v1/resources"),
      get("/v1/wifi"),
      get("/v1/clients"),
      get("/v1/dns").catch(function () {
        return {};
      }),
      get("/v1/qos").catch(function () {
        return {};
      }),
      get("/v1/vpn/profiles").catch(function () {
        return [];
      }),
      get("/v1/dhcp/reservations").catch(function () {
        return [];
      }),
      get("/v1/backups").catch(function () {
        return [];
      }),
      get("/v1/policy").catch(function () {
        return [];
      }),
      get("/v1/firewall").catch(function () {
        return [];
      }),
    ])
      .then(function (all) {
        var info = all[0],
          res = all[1],
          wifi = all[2],
          clients = all[3],
          dns = all[4] || {},
          qos = all[5] || {},
          profiles = all[6] || [],
          dhcp = all[7] || [],
          backups = all[8] || [],
          policy = all[9] || [],
          firewall = all[10] || [];

        renderSys(info);
        renderRes(res);
        renderWifi(wifi);
        renderClients(clients);
        renderVpn(profiles);
        renderDhcp(dhcp);
        renderPolicy(policy);
        renderFw(firewall);

        document.getElementById("axos-dns-wan").value = (dns.wan_upstreams || []).join(" ");
        document.getElementById("axos-dns-dot").textContent = dns.dot_enabled
          ? "enabled (profile " + (dns.dot_profile || "?") + ")"
          : "disabled";
        document.getElementById("axos-qos-enable").checked = !!qos.enabled;
        document.getElementById("axos-qos-mode").textContent =
          "mode=" + (qos.mode != null ? qos.mode : "—");

        document.getElementById("axos-backups").textContent =
          backups.length === 0
            ? "none"
            : backups
                .slice(0, 8)
                .map(function (b) {
                  return b.id + "  " + (b.reason || "") + "  " + (b.created_at || "");
                })
                .join("\n");

        setStatus("live · " + (info.model || "?"), true);
      })
      .catch(function (err) {
        setStatus("error: " + err.message, false);
      });
  }

  function bindVpnButtons() {
    document.getElementById("axos-vpn-body").onclick = function (ev) {
      var t = ev.target;
      if (!t || !t.className) return;
      var name = t.getAttribute("data-name");
      if (!name) return;
      var up = t.className.indexOf("axos-vpn-up") >= 0;
      var down = t.className.indexOf("axos-vpn-down") >= 0;
      if (!up && !down) return;
      t.disabled = true;
      withRollback("merlin-ui-vpn", function () {
        return post("/v1/vpn/" + encodeURIComponent(name) + "/" + (up ? "up" : "down"), {});
      })
        .then(function () {
          refresh();
        })
        .catch(function (e) {
          alert(String(e.message || e));
        })
        .then(function () {
          t.disabled = false;
        });
    };
  }

  function bindDhcpDelete() {
    document.getElementById("axos-dhcp-body").onclick = function (ev) {
      var t = ev.target;
      if (!t || t.className.indexOf("axos-dhcp-del") < 0) return;
      var mac = t.getAttribute("data-mac");
      if (!mac || !confirm("Delete reservation " + mac + "?")) return;
      withRollback("merlin-ui-dhcp-del", function () {
        return del("/v1/dhcp/reservations/" + encodeURIComponent(mac));
      })
        .then(refresh)
        .catch(function (e) {
          alert(String(e.message || e));
        });
    };
  }

  function bindPolicyDelete() {
    document.getElementById("axos-policy-body").onclick = function (ev) {
      var t = ev.target;
      if (!t || t.className.indexOf("axos-pol-del") < 0) return;
      var id = t.getAttribute("data-id");
      if (!id || !confirm("Delete policy " + id + "?")) return;
      withRollback("merlin-ui-policy-del", function () {
        return del("/v1/policy/" + encodeURIComponent(id));
      })
        .then(refresh)
        .catch(function (e) {
          alert(String(e.message || e));
        });
    };
  }

  function parseAvgMs(output) {
    // busybox/iputils: "round-trip min/avg/max = 1.2/3.4/5.6 ms" or "rtt min/avg/max/mdev = ..."
    var m = String(output || "").match(/(?:round-trip|rtt)[^=]*=\s*([\d.]+)\/([\d.]+)\/([\d.]+)/i);
    if (m) return parseFloat(m[2]);
    return null;
  }

  global.axosEmbedInit = function () {
    if (!token()) setStatus("missing UI token — run axos-merlin-ui.sh", false);

    document.getElementById("axos-refresh").onclick = refresh;
    bindVpnButtons();
    bindDhcpDelete();
    bindPolicyDelete();

    document.getElementById("axos-dns-apply").onclick = function () {
      var wan = document.getElementById("axos-dns-wan").value.trim().split(/\s+/).filter(Boolean);
      withRollback("merlin-ui-dns", function () {
        return get("/v1/dns").then(function (cur) {
          cur = cur || {};
          return post("/v1/dns", {
            wan_upstreams: wan,
            lan_upstreams: cur.lan_upstreams || [],
            dot_enabled: !!cur.dot_enabled,
            dot_profile: cur.dot_profile || "",
            dot_rules: cur.dot_rules || "",
          });
        });
      })
        .then(function () {
          alert("DNS applied");
          refresh();
        })
        .catch(function (e) {
          alert(String(e.message || e));
        });
    };

    document.getElementById("axos-qos-apply").onclick = function () {
      var enabled = document.getElementById("axos-qos-enable").checked;
      withRollback("merlin-ui-qos", function () {
        return post("/v1/qos", { enabled: enabled });
      })
        .then(function () {
          alert("QoS " + (enabled ? "enabled" : "disabled"));
          refresh();
        })
        .catch(function (e) {
          alert(String(e.message || e));
        });
    };

    function diag(path, body) {
      var out = document.getElementById("axos-diag");
      out.textContent = "running…";
      post(path, body)
        .then(function (r) {
          out.textContent = r.ok
            ? r.output || JSON.stringify(r, null, 2)
            : "FAILED\n" + (r.output || JSON.stringify(r, null, 2));
        })
        .catch(function (e) {
          out.textContent = String(e.message || e);
        });
    }

    document.getElementById("axos-ping").onclick = function () {
      diag("/v1/diag/ping", {
        host: document.getElementById("axos-host").value || "1.1.1.1",
        count: 3,
      });
    };
    document.getElementById("axos-dnslookup").onclick = function () {
      diag("/v1/diag/dns", {
        name: document.getElementById("axos-host").value || "example.com",
      });
    };
    document.getElementById("axos-port").onclick = function () {
      diag("/v1/diag/port", {
        host: document.getElementById("axos-host").value || "1.1.1.1",
        port: 443,
      });
    };

    document.getElementById("axos-dhcp-add").onclick = function () {
      var mac = document.getElementById("axos-dhcp-mac").value.trim();
      var ip = document.getElementById("axos-dhcp-ip").value.trim();
      var hostname = document.getElementById("axos-dhcp-name").value.trim();
      if (!mac || !ip) {
        alert("MAC and IP required");
        return;
      }
      withRollback("merlin-ui-dhcp-add", function () {
        return post("/v1/dhcp/reservations", { mac: mac, ip: ip, hostname: hostname });
      })
        .then(function () {
          document.getElementById("axos-dhcp-mac").value = "";
          document.getElementById("axos-dhcp-ip").value = "";
          document.getElementById("axos-dhcp-name").value = "";
          refresh();
        })
        .catch(function (e) {
          alert(String(e.message || e));
        });
    };

    document.getElementById("axos-backup").onclick = function () {
      var st = document.getElementById("axos-backup-status");
      st.textContent = "creating…";
      post("/v1/backup", { reason: "merlin-ui" })
        .then(function (b) {
          st.textContent = "ok · " + (b.id || "");
          refresh();
        })
        .catch(function (e) {
          st.textContent = String(e.message || e);
        });
    };

    document.getElementById("axos-pol-add").onclick = function () {
      var source = document.getElementById("axos-pol-src").value.trim();
      var iface = document.getElementById("axos-pol-if").value.trim();
      var desc = document.getElementById("axos-pol-desc").value.trim();
      var enabled = document.getElementById("axos-pol-en").checked;
      if (!source || !iface) {
        alert("Source and interface required");
        return;
      }
      withRollback("merlin-ui-policy-add", function () {
        return post("/v1/policy", {
          source: source,
          interface: iface,
          description: desc,
          enabled: enabled,
        });
      })
        .then(function () {
          document.getElementById("axos-pol-src").value = "";
          document.getElementById("axos-pol-if").value = "";
          document.getElementById("axos-pol-desc").value = "";
          refresh();
        })
        .catch(function (e) {
          alert(String(e.message || e));
        });
    };

    document.getElementById("axos-ep-ping").onclick = function () {
      var hosts = document
        .getElementById("axos-ep-hosts")
        .value.trim()
        .split(/\s+/)
        .filter(Boolean);
      var out = document.getElementById("axos-ep-out");
      if (!hosts.length) {
        out.textContent = "no hosts";
        return;
      }
      out.textContent = "pinging " + hosts.length + "…";
      var chain = Promise.resolve([]);
      hosts.forEach(function (h) {
        chain = chain.then(function (rows) {
          return post("/v1/diag/ping", { host: h, count: 3 })
            .then(function (r) {
              var avg = parseAvgMs(r.output);
              rows.push({
                host: h,
                ok: !!r.ok,
                avg_ms: avg,
                line: avg != null ? avg.toFixed(1) + " ms avg" : r.ok ? "ok (no rtt)" : "fail",
              });
              return rows;
            })
            .catch(function (e) {
              rows.push({ host: h, ok: false, avg_ms: null, line: String(e.message || e) });
              return rows;
            });
        });
      });
      chain.then(function (rows) {
        rows.sort(function (a, b) {
          if (a.avg_ms == null && b.avg_ms == null) return 0;
          if (a.avg_ms == null) return 1;
          if (b.avg_ms == null) return -1;
          return a.avg_ms - b.avg_ms;
        });
        out.textContent = rows
          .map(function (r, i) {
            return i + 1 + ". " + r.host + "  " + r.line;
          })
          .join("\n");
      });
    };

    refresh();
    setInterval(refresh, 30000);
  };
})(window);
