/* AXOS Merlin embed — talks to Core API on :9090 with UI token (LANAuth). */
(function (global) {
  function apiBase() {
    var host = window.location.hostname || "192.168.50.1";
    return "http://" + host + ":9090";
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
    if (extra) {
      for (var k in extra) h[k] = extra[k];
    }
    return h;
  }

  function get(path) {
    return fetch(apiBase() + path, { headers: headers() }).then(function (r) {
      if (!r.ok) throw new Error(path + " " + r.status);
      return r.json();
    });
  }

  function post(path, body) {
    return fetch(apiBase() + path, {
      method: "POST",
      headers: headers({ "Content-Type": "application/json" }),
      body: JSON.stringify(body || {}),
    }).then(function (r) {
      return r.text().then(function (t) {
        var data;
        try {
          data = JSON.parse(t);
        } catch (e) {
          data = { raw: t };
        }
        if (!r.ok) throw new Error((data && data.error) || path + " " + r.status);
        return data;
      });
    });
  }

  function fmt(v) {
    return JSON.stringify(v, null, 2);
  }

  function setStatus(text, ok) {
    var el = document.getElementById("axos-status");
    if (!el) return;
    el.textContent = text;
    if (ok) el.classList.add("ok");
    else el.classList.remove("ok");
  }

  function refresh() {
    setStatus("loading…", false);
    Promise.all([
      get("/v1/info"),
      get("/v1/resources"),
      get("/v1/interfaces"),
      get("/v1/clients"),
      get("/v1/wifi"),
      get("/v1/vpn"),
      get("/v1/dns").catch(function () {
        return null;
      }),
      get("/v1/qos").catch(function () {
        return null;
      }),
    ])
      .then(function (all) {
        document.getElementById("axos-sys").textContent = fmt(all[0]);
        document.getElementById("axos-res").textContent = fmt(all[1]);
        document.getElementById("axos-net").textContent = fmt({
          interfaces: all[2],
          clients: all[3],
        });
        document.getElementById("axos-wifi").textContent = fmt(all[4]);
        document.getElementById("axos-vpn").textContent = fmt({
          tunnels: all[5],
          dns: all[6],
          qos: all[7],
        });
        setStatus("live · " + ((all[0] && all[0].model) || "?"), true);
      })
      .catch(function (err) {
        setStatus("error", false);
        document.getElementById("axos-sys").textContent = String(err);
      });
  }

  global.axosEmbedInit = function () {
    if (!token()) {
      setStatus("missing UI token — run axos-merlin-ui.sh", false);
    }
    var btn = document.getElementById("axos-refresh");
    if (btn) btn.onclick = refresh;
    var ping = document.getElementById("axos-ping");
    if (ping) {
      ping.onclick = function () {
        var host = (document.getElementById("axos-host") || {}).value || "1.1.1.1";
        var out = document.getElementById("axos-diag");
        out.textContent = "pinging " + host + "…";
        post("/v1/diag/ping", { host: host, count: 3 })
          .then(function (r) {
            out.textContent = fmt(r);
          })
          .catch(function (e) {
            out.textContent = String(e);
          });
      };
    }
    refresh();
    setInterval(refresh, 20000);
  };
})(window);
