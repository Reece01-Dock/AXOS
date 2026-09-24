// ui-check: drives every AXOS UI feature in Chromium against the mock
// backend and checks the resulting API state. Run via scripts/ui-check/run.sh.
const { chromium } = require(process.env.PW);
const API = "http://127.0.0.1:9090";
const M = "http://127.0.0.1:8080/";
const j = async (p) => (await fetch(API + p)).json();
let fails = 0;
function check(name, cond, extra) { console.log((cond ? "PASS " : "FAIL ") + name + (cond || !extra ? "" : "  " + JSON.stringify(extra))); if (!cond) fails++; }
async function idle() { const s = await j("/v1/rollback/status"); return !s.Pending; }

(async () => {
  const browser = await chromium.launch();
  const ctx = await browser.newContext({ viewport: { width: 1100, height: 900 } });
  const page = await ctx.newPage();
  const errs = [];
  page.on("pageerror", e => errs.push(e.message));
  let dialogs = [];
  let answer = true;
  page.on("dialog", async d => { dialogs.push(d.type() + ": " + d.message()); if (d.type() === "prompt") await d.dismiss(); else if (answer) await d.accept(); else await d.dismiss(); });
  const settle = () => page.waitForTimeout(700);

  // ---------------- VPN
  await page.goto(M + "Advanced_VPN_PPTP.asp", { waitUntil: "networkidle" });
  check("vpn: interface defaults to connected tunnel", await page.$eval("#vpn-iface", e => e.value) === "WGC5");
  await page.check('.vpn-steer-cb[data-mac="11:22:33:44:55:66"]');
  await page.click("#vpn-steer-apply"); await settle();
  let pol = await j("/v1/policy");
  check("vpn: steer gaming-pc to WGC5 by IP", pol.some(r => r.source === "192.168.1.50" && r.interface === "WGC5"), pol);
  check("vpn: rollback confirmed", await idle());

  // Conflict: move TV (has WGC5 rule "TV") to WAN; first decline, then accept.
  await page.selectOption("#vpn-iface", "WAN");
  await page.check('.vpn-steer-cb[data-mac="11:22:33:44:55:77"]');
  answer = false; dialogs = [];
  await page.click("#vpn-steer-apply"); await settle();
  pol = await j("/v1/policy");
  check("vpn: conflict asks before replacing", dialogs.some(d => d.includes("already have VPN Director rules")), dialogs);
  check("vpn: declined conflict leaves rule alone", pol.find(r => r.source === "192.168.1.51").interface === "WGC5", pol);
  check("vpn: declined conflict releases rollback", await idle());
  answer = true; dialogs = [];
  await page.check('.vpn-steer-cb[data-mac="11:22:33:44:55:77"]');
  await page.click("#vpn-steer-apply"); await settle();
  pol = await j("/v1/policy");
  check("vpn: accepted conflict replaces rule", pol.find(r => r.source === "192.168.1.51").interface === "WAN", pol);
  check("vpn: remote-IP rule kept", pol.some(r => r.source === "192.168.1.60" && r.remote === "10.0.0.0/8"), pol);

  // Remove rules for selected.
  await page.check('.vpn-steer-cb[data-mac="11:22:33:44:55:77"]');
  await page.click("#vpn-steer-remove"); await settle();
  pol = await j("/v1/policy");
  check("vpn: remove rules for selected", !pol.some(r => r.source === "192.168.1.51"), pol);

  // Staged rule editor: add, toggle, then Apply once.
  await page.click("#vpn-rule-add");
  await page.fill("#rule-local", "192.168.1.0/28");
  await page.fill("#rule-desc", "iot");
  await page.selectOption("#rule-iface", "WGC1");
  await page.click("#rule-ok");
  await page.click('#vpn-rules-body .axos-rule-toggle[data-idx="0"]');
  check("vpn: staged edits flagged", await page.isVisible("#vpn-rules-dirty"));
  const before = (await j("/v1/policy")).length;
  check("vpn: staged edits not written yet", before === pol.length);
  await page.click("#vpn-rules-apply"); await settle();
  pol = await j("/v1/policy");
  check("vpn: rule list applied in one PUT", pol.some(r => r.source === "192.168.1.0/28" && r.interface === "WGC1") && pol[0].enabled === false, pol);
  check("vpn: dirty flag cleared", !(await page.isVisible("#vpn-rules-dirty")));

  // Editor rejects a MAC.
  dialogs = [];
  await page.click("#vpn-rule-add");
  await page.fill("#rule-local", "AA:BB:CC:DD:EE:FF");
  await page.click("#rule-ok");
  check("vpn: editor rejects MAC as Local IP", dialogs.some(d => d.includes("Local IP must be")), dialogs);
  await page.click("#rule-cancel");

  // Groups.
  await page.check('.vpn-steer-cb[data-mac="11:22:33:44:55:66"]');
  await page.check('.vpn-steer-cb[data-mac="11:22:33:44:55:77"]');
  await page.fill("#vpn-group-name", "Streaming");
  await page.click("#vpn-group-save"); await settle();
  let groups = (await j("/v1/vpn/client-groups")).groups;
  check("vpn: group saved", groups.length === 1 && groups[0].members.length === 2, groups);
  await page.reload({ waitUntil: "networkidle" });
  await page.selectOption("#vpn-group", "streaming"); await settle();
  const ticked = await page.$$eval(".vpn-steer-cb:checked", els => els.length);
  check("vpn: selecting group ticks members", ticked === 2, ticked);
  await page.click("#vpn-group-del"); await settle();
  groups = (await j("/v1/vpn/client-groups")).groups;
  check("vpn: group deleted", groups.length === 0, groups);

  // WireGuard paste + import into slot 2.
  await page.selectOption("#wg-unit", "2");
  await page.fill("#wg-paste", "[Interface]\nPrivateKey = aW1wb3J0ZWRwcml2YXRla2V5aW1wb3J0ZWRwcml2YQ==\nAddress = 10.8.0.2/32, fd00::2/128\nDNS = 10.8.0.1\n\n[Peer]\nPublicKey = cGVlcnB1YmxpY2tleXBlZXJwdWJsaWNrZXlwZWVycA==\nEndpoint = vpn.example.net:51821\nAllowedIPs = 0.0.0.0/0, ::/0\nPersistentKeepalive = 15\n");
  await page.click("#wg-parse");
  check("vpn: .conf parsed", await page.$eval("#wg-ep", e => e.value) === "vpn.example.net" && await page.$eval("#wg-port", e => e.value) === "51821" && await page.$eval("#wg-addr", e => e.value) === "10.8.0.2/32");
  await page.fill("#wg-desc", "Home VPN");
  await page.click("#wg-import"); await settle();
  let prof = await j("/v1/vpn/profiles");
  check("vpn: WireGuard imported to WGC2", prof.some(p => p.name === "wgc2" && p.description === "Home VPN"), prof);
  check("vpn: private key cleared from form", await page.$eval("#wg-priv", e => e.value) === "");

  // Toggle a tunnel.
  await page.click('.axos-vpn-toggle[data-name="wgc1"]'); await settle();
  prof = await j("/v1/vpn/profiles");
  check("vpn: connect WGC1 from status table", prof.find(p => p.name === "wgc1").enabled === true, prof);

  // Benchmark.
  await page.click("#vpn-bench-run"); await page.waitForTimeout(1500);
  check("vpn: benchmark shows results", (await page.textContent("#vpn-bench-title")).includes("fastest"), await page.textContent("#vpn-bench-title"));

  // ---------------- LAN
  await page.goto(M + "Advanced_APPList_Content.asp", { waitUntil: "networkidle" });
  await page.click('.axos-reserve[data-mac="11:22:33:44:55:77"]'); await settle();
  let res = await j("/v1/dhcp/reservations");
  check("lan: reserve from client list", res.some(r => r.ip === "192.168.1.51"), res);
  dialogs = [];
  await page.fill("#lan-res-mac", "AA:BB:CC:00:11:22"); await page.fill("#lan-res-ip", "192.168.1.51");
  await page.click("#lan-res-add"); await settle();
  check("lan: duplicate IP refused", dialogs.some(d => d.includes("already reserved")), dialogs);
  await page.fill("#lan-res-ip", "192.168.1.99"); await page.fill("#lan-res-name", "printer");
  await page.click("#lan-res-add"); await settle();
  res = await j("/v1/dhcp/reservations");
  check("lan: manual add", res.some(r => r.ip === "192.168.1.99"), res);
  await page.click('.axos-res-del[data-mac="AA:BB:CC:00:11:22"]'); await settle();
  res = await j("/v1/dhcp/reservations");
  check("lan: delete", !res.some(r => r.ip === "192.168.1.99"), res);
  await page.fill("#lan-filter", "tv");
  check("lan: filter", (await page.textContent("#lan-clients-title")).startsWith("Clients (1 /"));

  // ---------------- WAN / DNS
  await page.goto(M + "WAN_info.asp", { waitUntil: "networkidle" });
  await page.check('input[name="dns-auto"][value="0"]');
  await page.fill("#dns-wan1", "9.9.9.9"); await page.fill("#dns-wan2", "149.112.112.112");
  await page.selectOption("#dns-dot", "1");
  await page.selectOption("#dot-preset", "9.9.9.9|853|dns.quad9.net");
  await page.click("#dot-add");
  await page.click("#dns-apply"); await settle();
  let dns = await j("/v1/dns");
  check("dns: manual WAN + DoT applied", dns.wan_dns_auto === false && dns.wan_upstreams.join() === "9.9.9.9,149.112.112.112" && dns.dot_enabled && dns.dot_rules.includes("dns.quad9.net"), dns);
  await page.click("#dns-test"); await settle();
  check("dns: lookup test output", (await page.$eval("#dns-test-out", e => e.value)).length > 5);

  // ---------------- Firewall
  await page.goto(M + "Advanced_VPN_IPSec.asp", { waitUntil: "networkidle" });
  const fwBefore = (await j("/v1/firewall")).length;
  await page.fill("#fw-add-rule", "-p tcp --dport 8443 -j ACCEPT");
  await page.click("#fw-add"); await settle();
  let fw = await j("/v1/firewall");
  check("firewall: add rule", fw.length === fwBefore + 1, { before: fwBefore, after: fw.length });
  await page.selectOption("#fw-table", "");
  await page.fill("#fw-search", "8443");
  await page.click('#fw-body .axos-fw-del'); await settle();
  fw = await j("/v1/firewall");
  check("firewall: delete rule", fw.length === fwBefore, fw.length);

  // ---------------- QoS
  await page.goto(M + "Advanced_AiDisk_webdav.asp", { waitUntil: "networkidle" });
  const qos0 = (await j("/v1/qos")).enabled;
  await page.check(`input[name="qos-enable"][value="${qos0 ? 0 : 1}"]`);
  await page.click("#qos-apply"); await settle();
  check("qos: toggle applied", (await j("/v1/qos")).enabled === !qos0);
  await page.waitForTimeout(5500);
  check("qos: live throughput measured", !(await page.textContent("#qos-rate-body")).includes("measuring"));

  // ---------------- Tools
  await page.goto(M + "Guest_network.asp", { waitUntil: "networkidle" });
  for (const m of ["ping", "traceroute", "nslookup", "port", "iperf3"]) {
    await page.selectOption("#diag-method", m);
    await page.fill("#diag-target", "1.1.1.1");
    await page.click("#diag-run");
    await page.waitForFunction(() => !document.getElementById("diag-run").disabled);
    const out = await page.$eval("#diag-out", e => e.value);
    check("tools: " + m, out.length > 5 && !out.startsWith("Running"), out.slice(0, 120));
  }

  // ---------------- Overview
  await page.goto(M + "Main_GameServer_Content.asp", { waitUntil: "networkidle" });
  await page.click("#ov-backup"); await settle();
  const bks = await j("/v1/backups");
  check("overview: create backup", bks.length >= 1, bks);
  await page.click(".axos-restore"); await settle();
  check("overview: restore backup + rollback confirmed", await idle());
  check("overview: model shown in white value", (await page.textContent("#ov-model")) === "GT-AX6000");

  check("no page errors", errs.length === 0, errs);
  check("rollback idle at end", await idle());
  await browser.close();
  console.log(fails ? fails + " FAILED" : "ALL PASSED");
  process.exit(fails ? 1 : 0);
})().catch(e => { console.error(e); process.exit(2); });
