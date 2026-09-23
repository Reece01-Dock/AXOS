/* AXOS Merlin embed - Core API on :9090 with UI token (LANAuth). */
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
  function put(path, body) {
    return req("PUT", path, body || {});
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

  function asList(v) {
    return Object.prototype.toString.call(v) === "[object Array]" ? v : [];
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
    if (!ns) return "-";
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

  // Section -> which data-axos panels to show (comma list). "all" = everything.
  var SECTION_PANELS = {
    all: null,
    vpn: "vpn",
    dns: "dns",
    lan: "dhcp,clients",
    firewall: "firewall",
    qos: "qos",
    wifi: "wifi,clients",
    diag: "diag",
    system: "system,resources,backup",
  };

  function applySection() {
    var section = (global.AXOS_SECTION || "all").toLowerCase();
    if (section === "__axos_section__") section = "all";
    var want = SECTION_PANELS[section];
    var nodes = document.querySelectorAll("#axos-root [data-axos]");
    for (var i = 0; i < nodes.length; i++) {
      var key = nodes[i].getAttribute("data-axos");
      var show = !want || ("," + want + ",").indexOf("," + key + ",") >= 0;
      if (show) nodes[i].classList.remove("axos-hidden");
      else nodes[i].classList.add("axos-hidden");
    }
    var full = document.getElementById("axos-full-link");
    if (full) {
      if (section === "all") full.style.display = "none";
      else full.style.display = "";
    }
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
    for (var k in temps) tparts.push(k + "=" + temps[k] + "C");
    document.getElementById("axos-res-body").innerHTML = kvRows([
      ["Load (1/5/15)", [res.cpu_load_1m, res.cpu_load_5m, res.cpu_load_15m].join(" / ")],
      [
        "Memory",
        Math.round((res.mem_used_kb || 0) / 1024) +
          " / " +
          Math.round((res.mem_total_kb || 0) / 1024) +
          " MB",
      ],
      ["Temperatures", tparts.join(", ") || "-"],
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
        esc(r.ssid || "-") +
        "</td><td>" +
        esc(r.channel != null ? r.channel : "-") +
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
        esc(c.hostname || "-") +
        "</td><td>" +
        esc(c.ip || "-") +
        "</td><td>" +
        esc(c.mac || "-") +
        "</td><td>" +
        esc(c.interface || "-") +
        "</td></tr>";
    });
    document.getElementById("axos-clients-body").innerHTML =
      html || "<tr><td colspan='4'>No clients</td></tr>";
  }

  function renderVpn(profiles) {
    var html = "";
    (profiles || []).forEach(function (p) {
      var state = p.enabled
        ? '<span style="color:#7dffb3">Connected</span>'
        : '<span style="color:#ffcc66">Down</span>';
      html +=
        "<tr><th>" +
        esc(directorIface(p.name)) +
        (p.description ? " <span class='hint'>" + esc(p.description) + "</span>" : "") +
        "</th><td>" +
        state +
        ' &nbsp; <input type="button" class="button_gen axos-vpn-up" data-name="' +
        esc(p.name) +
        '" value="Connect"> ' +
        '<input type="button" class="button_gen axos-vpn-down" data-name="' +
        esc(p.name) +
        '" value="Disconnect"></td></tr>';
    });
    var body = document.getElementById("axos-vpn-body");
    if (body) body.innerHTML = html || "<tr><td colspan='2'>No VPN client slots</td></tr>";

    var st = document.getElementById("axos-vpn-status");
    if (st) {
      var up = (profiles || []).filter(function (p) { return p.enabled; });
      st.innerHTML =
        up.length === 0
          ? "<p>No VPN tunnel is up. Connect a client below first (Cloudflare WARP is usually a WireGuard slot such as WGC5).</p>"
          : "<p>Active tunnel(s): <b>" +
            up.map(function (p) { return esc(directorIface(p.name)); }).join(", ") +
            "</b></p>";
    }
    fillSteerIface(profiles);
  }

  var steerGroups = [];
  var steerClients = [];
  var steerPolicy = [];

  function directorIface(name) {
    if (!name) return "WAN";
    var n = String(name).toLowerCase();
    if (n === "wan") return "WAN";
    if (n.indexOf("wgc") === 0) return "WGC" + n.replace("wgc", "");
    if (n.indexOf("ovpnc") === 0) return "OVPN" + n.replace("ovpnc", "");
    return String(name).toUpperCase();
  }

  function fillSteerIface(profiles) {
    var sel = document.getElementById("axos-steer-iface");
    if (!sel) return;
    var cur = sel.value;
    var opts = '<option value="WAN">WAN</option>';
    (profiles || []).forEach(function (p) {
      var v = directorIface(p.name);
      var label = v;
      if (p.description) label += " - " + p.description;
      if (p.type === "wireguard") label += " (WireGuard)";
      if (p.type === "openvpn") label += " (OpenVPN)";
      opts += '<option value="' + esc(v) + '">' + esc(label) + "</option>";
    });
    sel.innerHTML = opts;
    if (cur && sel.querySelector('option[value="' + cur + '"]')) sel.value = cur;
    else {
      for (var i = 0; i < (profiles || []).length; i++) {
        if (profiles[i].type === "wireguard") {
          sel.value = directorIface(profiles[i].name);
          break;
        }
      }
    }
  }

  function policyForClient(c) {
    var mac = (c.mac || "").toLowerCase();
    var ip = (c.ip || "").toLowerCase();
    for (var i = 0; i < steerPolicy.length; i++) {
      var s = String(steerPolicy[i].source || "").toLowerCase();
      if (s && (s === mac || s === ip)) return steerPolicy[i];
    }
    return null;
  }

  function renderSteer(clients, policy) {
    steerClients = clients || [];
    steerPolicy = policy || [];
    var body = document.getElementById("axos-steer-body");
    if (!body) return;
    var html = "";
    steerClients.forEach(function (c, idx) {
      var pol = policyForClient(c);
      var iface = pol && pol.enabled !== false ? pol.interface : "WAN";
      var onVpn = iface && String(iface).toUpperCase() !== "WAN";
      html +=
        "<tr>" +
        "<td><input type='checkbox' class='axos-steer-cb' data-idx='" + idx + "' data-mac='" + esc(c.mac) + "'></td>" +
        "<td>" + esc(c.hostname || c.mac || "?") + "</td>" +
        "<td>" + esc(c.ip || "-") + "</td>" +
        "<td>" + esc(c.mac || "-") + "</td>" +
        "<td>" + esc(c.interface || "-") + "</td>" +
        "<td>" + (onVpn ? "<span style='color:#FC0'>" + esc(iface) + "</span>" : "WAN") + "</td>" +
        "</tr>";
    });
    body.innerHTML = html || "<tr><td colspan='6'>No clients found</td></tr>";
  }

  function fillGroupSel() {
    var sel = document.getElementById("axos-group-sel");
    if (!sel) return;
    var cur = sel.value;
    var opts = '<option value="">All clients</option>';
    steerGroups.forEach(function (g) {
      opts += '<option value="' + esc(g.id) + '">' + esc(g.name) + " (" + (g.members || []).length + ")</option>";
    });
    sel.innerHTML = opts;
    if (cur) sel.value = cur;
  }

  function selectedMACs() {
    var macs = [];
    var boxes = document.querySelectorAll(".axos-steer-cb:checked");
    for (var i = 0; i < boxes.length; i++) {
      var m = boxes[i].getAttribute("data-mac");
      if (m) macs.push(m);
    }
    return macs;
  }

  function setSteerChecks(macs) {
    var want = {};
    (macs || []).forEach(function (m) { want[String(m).toLowerCase()] = true; });
    var boxes = document.querySelectorAll(".axos-steer-cb");
    for (var i = 0; i < boxes.length; i++) {
      var m = (boxes[i].getAttribute("data-mac") || "").toLowerCase();
      boxes[i].checked = macs && macs.length ? !!want[m] : false;
    }
    var all = document.getElementById("axos-steer-allcb");
    if (all) all.checked = false;
  }

  function applySteer(iface, desc) {
    var macs = selectedMACs();
    var st = document.getElementById("axos-steer-status");
    if (!macs.length) {
      alert("Select at least one client in the list.");
      return;
    }
    if (st) st.textContent = "Applying...";
    withRollback("merlin-ui-steer", function () {
      return post("/v1/policy/bulk", {
        interface: iface,
        description: desc || "axos",
        sources: macs,
        enabled: true,
      });
    })
      .then(function (r) {
        if (st) st.textContent = "Applied " + ((r && r.applied) || macs.length) + " client(s) to " + iface + ".";
        refresh();
      })
      .catch(function (e) {
        if (st) st.textContent = String(e.message || e);
        alert(String(e.message || e));
      });
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
        esc(r.hostname || "-") +
        '</td><td><input type="button" class="button_gen axos-dhcp-del" data-mac="' +
        esc(r.mac) +
        '" value="Delete"></td></tr>';
    });
    document.getElementById("axos-dhcp-body").innerHTML =
      html || "<tr><td colspan='4'>None</td></tr>";
  }

  function renderPolicy(list) {
    var html = "";
    (list || []).forEach(function (r) {
      var en = r.enabled !== false;
      html +=
        "<tr>" +
        "<td>" + (en ? "<span style='color:#7dffb3'>Yes</span>" : "<span style='color:#888'>No</span>") + "</td>" +
        "<td>" + esc(r.description || "-") + "</td>" +
        "<td>" + esc(r.source) + "</td>" +
        "<td><span style='color:#FC0'>" + esc(r.interface) + "</span></td>" +
        '<td><input type="button" class="button_gen axos-pol-del" data-id="' + esc(r.id) + '" value="Remove"></td>' +
        "</tr>";
    });
    var body = document.getElementById("axos-policy-body");
    if (body) body.innerHTML = html || "<tr><td colspan='5'>No rules yet — tick clients and click Apply.</td></tr>";
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
    setStatus("loading...", false);
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
      get("/v1/vpn/client-groups").catch(function () {
        return { groups: [] };
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
          firewall = all[10] || [],
          groupsWrap = all[11] || {};

        steerGroups = groupsWrap.groups || [];
        fillGroupSel();

        renderSys(info);
        renderRes(res);
        renderWifi(wifi);
        renderClients(asList(clients));
        renderVpn(asList(profiles));
        renderSteer(asList(clients), asList(policy));
        renderDhcp(asList(dhcp));
        renderPolicy(asList(policy));
        renderFw(asList(firewall));

        document.getElementById("axos-dns-wan").value = (dns.wan_upstreams || []).join(" ");
        document.getElementById("axos-dns-dot").textContent = dns.dot_enabled
          ? "enabled (profile " + (dns.dot_profile || "?") + ")"
          : "disabled";
        document.getElementById("axos-qos-enable").checked = !!qos.enabled;
        document.getElementById("axos-qos-mode").textContent =
          "mode=" + (qos.mode != null ? qos.mode : "-");

        document.getElementById("axos-backups").textContent =
          backups.length === 0
            ? "none"
            : backups
                .slice(0, 8)
                .map(function (b) {
                  return b.id + "  " + (b.reason || "") + "  " + (b.created_at || "");
                })
                .join("\n");

        setStatus("live - " + (info.model || "?"), true);
      })
      .catch(function (err) {
        setStatus("error: " + err.message, false);
      });
  }

  function bindVpnButtons() {
    var body = document.getElementById("axos-vpn-body");
    if (!body) return;
    body.onclick = function (ev) {
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
    var body = document.getElementById("axos-dhcp-body");
    if (!body) return;
    body.onclick = function (ev) {
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
    var body = document.getElementById("axos-policy-body");
    if (!body) return;
    body.onclick = function (ev) {
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

  global.axosEmbedInit = function () {
    try {
      if (!token()) setStatus("missing UI token - run axos-merlin-ui.sh", false);

      applySection();

      function on(id, fn) {
        var el = document.getElementById(id);
        if (el) el.onclick = fn;
      }

      on("axos-refresh", refresh);
      bindVpnButtons();
      bindDhcpDelete();
      bindPolicyDelete();

      on("axos-steer-apply", function () {
        var iface = document.getElementById("axos-steer-iface").value || "WAN";
        var descEl = document.getElementById("axos-steer-desc");
        var desc = descEl ? descEl.value.trim() : "axos";
        applySteer(iface, desc || "axos");
      });
      on("axos-steer-allcb", function () {
        var all = document.getElementById("axos-steer-allcb");
        var boxes = document.querySelectorAll(".axos-steer-cb");
        for (var i = 0; i < boxes.length; i++) boxes[i].checked = !!(all && all.checked);
      });
      // Selecting a group ticks its members (Merlin-style preset).
      var gsel = document.getElementById("axos-group-sel");
      if (gsel) {
        gsel.onchange = function () {
          var id = gsel.value;
          if (!id) {
            setSteerChecks([]);
            return;
          }
          for (var i = 0; i < steerGroups.length; i++) {
            if (steerGroups[i].id === id) {
              setSteerChecks(steerGroups[i].members || []);
              if (steerGroups[i].interface) {
                var sel = document.getElementById("axos-steer-iface");
                if (sel) sel.value = directorIface(steerGroups[i].interface);
              }
              break;
            }
          }
        };
      }
      on("axos-group-save", function () {
        var name = (document.getElementById("axos-group-name").value || "").trim();
        var macs = selectedMACs();
        if (!name) {
          alert("Enter a group name.");
          return;
        }
        if (!macs.length) {
          alert("Tick the clients that belong in this group, then Save.");
          return;
        }
        var id = name.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "") || "group";
        var iface = document.getElementById("axos-steer-iface").value || "";
        var next = steerGroups.slice();
        var found = false;
        for (var i = 0; i < next.length; i++) {
          if (next[i].id === id || next[i].name === name) {
            next[i] = { id: next[i].id || id, name: name, members: macs, interface: iface };
            found = true;
            break;
          }
        }
        if (!found) next.push({ id: id, name: name, members: macs, interface: iface });
        put("/v1/vpn/client-groups", { groups: next })
          .then(function (r) {
            steerGroups = (r && r.groups) || next;
            fillGroupSel();
            document.getElementById("axos-group-sel").value = id;
            document.getElementById("axos-group-name").value = "";
          })
          .catch(function (e) {
            alert(String(e.message || e));
          });
      });
      on("axos-group-del", function () {
        var id = document.getElementById("axos-group-sel").value;
        if (!id || !confirm("Delete this group?")) return;
        var next = steerGroups.filter(function (g) { return g.id !== id; });
        put("/v1/vpn/client-groups", { groups: next })
          .then(function (r) {
            steerGroups = (r && r.groups) || next;
            fillGroupSel();
            setSteerChecks([]);
          })
          .catch(function (e) {
            alert(String(e.message || e));
          });
      });

      on("axos-dns-apply", function () {
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
    });

      on("axos-qos-apply", function () {
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
    });

    function diag(path, body) {
      var out = document.getElementById("axos-diag");
      if (!out) return;
      out.textContent = "running...";
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

      on("axos-ping", function () {
      diag("/v1/diag/ping", {
        host: document.getElementById("axos-host").value || "1.1.1.1",
        count: 3,
      });
    });
      on("axos-dnslookup", function () {
      diag("/v1/diag/dns", {
        name: document.getElementById("axos-host").value || "example.com",
      });
    });
      on("axos-port", function () {
      diag("/v1/diag/port", {
        host: document.getElementById("axos-host").value || "1.1.1.1",
        port: 443,
      });
    });

      on("axos-dhcp-add", function () {
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
    });

      on("axos-backup", function () {
      var st = document.getElementById("axos-backup-status");
      if (st) st.textContent = "creating...";
      post("/v1/backup", { reason: "merlin-ui" })
        .then(function (b) {
          if (st) st.textContent = "ok - " + (b.id || "");
          refresh();
        })
        .catch(function (e) {
          if (st) st.textContent = String(e.message || e);
        });
    });

      on("axos-ep-ping", function () {
      var hosts = document
        .getElementById("axos-ep-hosts")
        .value.trim()
        .split(/\s+/)
        .filter(Boolean);
      var out = document.getElementById("axos-ep-out");
      if (!out) return;
      if (!hosts.length) {
        out.textContent = "no hosts";
        return;
      }
      out.textContent = "benchmarking " + hosts.length + "...";
      post("/v1/vpn/benchmark", { hosts: hosts, count: 3 })
        .then(function (r) {
          var rows = (r && r.results) || [];
          var lines = rows.map(function (s, i) {
            var line =
              s.avg_ms != null
                ? Number(s.avg_ms).toFixed(1) + " ms avg"
                : s.ok
                  ? "ok (no rtt)"
                  : "fail";
            return i + 1 + ". " + s.host + "  " + line;
          });
          if (r && r.best) lines.push("", "best: " + r.best);
          out.textContent = lines.join("\n") || "no results";
        })
        .catch(function (e) {
          out.textContent = String(e.message || e);
        });
    });

      refresh();
      setInterval(refresh, 30000);
    } catch (e) {
      setStatus("init error: " + (e && e.message ? e.message : e), false);
      if (window.console && console.error) console.error("axosEmbedInit", e);
    }
  };
})(window);
