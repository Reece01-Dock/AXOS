(function () {
  const statusEl = document.getElementById("status");
  const fmt = (v) => JSON.stringify(v, null, 2);

  async function get(path) {
    const r = await fetch(path, { headers: { Accept: "application/json" } });
    if (!r.ok) throw new Error(path + " " + r.status);
    return r.json();
  }

  async function post(path, body) {
    const r = await fetch(path, {
      method: "POST",
      headers: {
        Accept: "application/json",
        "Content-Type": "application/json",
        "X-Axos-Actor": "webui",
      },
      body: JSON.stringify(body || {}),
    });
    const text = await r.text();
    let data;
    try { data = JSON.parse(text); } catch { data = { raw: text }; }
    if (!r.ok) throw new Error((data && data.error) || path + " " + r.status);
    return data;
  }

  async function refresh() {
    statusEl.textContent = "loading";
    statusEl.classList.remove("ok");
    try {
      const [info, res, ifaces, clients, wifi, vpn, dns, qos] = await Promise.all([
        get("/v1/info"),
        get("/v1/resources"),
        get("/v1/interfaces"),
        get("/v1/clients"),
        get("/v1/wifi"),
        get("/v1/vpn"),
        get("/v1/dns").catch(() => null),
        get("/v1/qos").catch(() => null),
      ]);
      document.getElementById("sys-body").textContent = fmt(info);
      document.getElementById("res-body").textContent = fmt(res);
      document.getElementById("net-body").textContent = fmt({
        interfaces: ifaces,
        clients: clients,
      });
      document.getElementById("wifi-body").textContent = fmt(wifi);
      document.getElementById("vpn-body").textContent = fmt({
        tunnels: vpn,
        dns: dns,
        qos: qos,
      });
      statusEl.textContent = "live · " + (info.model || "?");
      statusEl.classList.add("ok");
    } catch (err) {
      statusEl.textContent = "error";
      statusEl.classList.remove("ok");
      document.getElementById("sys-body").textContent = String(err);
    }
  }

  document.getElementById("btn-refresh").addEventListener("click", refresh);
  document.getElementById("diag-form").addEventListener("submit", async (e) => {
    e.preventDefault();
    const host = new FormData(e.target).get("host") || "1.1.1.1";
    const out = document.getElementById("diag-body");
    out.textContent = "pinging " + host + "…";
    try {
      const r = await post("/v1/diag/ping", { host: String(host), count: 3 });
      out.textContent = fmt(r);
    } catch (err) {
      out.textContent = String(err);
    }
  });

  refresh();
  setInterval(refresh, 15000);
})();
