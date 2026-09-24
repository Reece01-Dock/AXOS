// ui-check: loads every AXOS page (Merlin harness + standalone), fails on
// JS errors or a Core API connection error, and saves full-page screenshots.
const { chromium } = require(process.env.PW || 'playwright');
(async () => {
  const out = process.argv[2];
  const browser = await chromium.launch();
  const pages = [
    ["merlin-all", "http://127.0.0.1:8080/Main_GameServer_Content.asp"],
    ["merlin-vpn", "http://127.0.0.1:8080/Advanced_VPN_PPTP.asp"],
    ["merlin-lan", "http://127.0.0.1:8080/Advanced_APPList_Content.asp"],
    ["merlin-dns", "http://127.0.0.1:8080/WAN_info.asp"],
    ["merlin-firewall", "http://127.0.0.1:8080/Advanced_VPN_IPSec.asp"],
    ["merlin-qos", "http://127.0.0.1:8080/Advanced_AiDisk_webdav.asp"],
    ["merlin-wifi", "http://127.0.0.1:8080/WiFi_Insight.asp"],
    ["merlin-diag", "http://127.0.0.1:8080/Guest_network.asp"],
    ["standalone-all", "http://127.0.0.1:9090/#all"],
    ["standalone-vpn", "http://127.0.0.1:9090/#vpn"],
  ];
  let failures = 0;
  for (const [name, url] of pages) {
    const ctx = await browser.newContext({ viewport: { width: 1100, height: 900 } });
    const page = await ctx.newPage();
    const errs = [];
    page.on("pageerror", e => errs.push("pageerror: " + e.message));
    page.on("console", m => { if (m.type() === "error") errs.push("console: " + m.text()); });
    page.on("dialog", d => { errs.push("dialog: " + d.message()); d.dismiss(); });
    await page.goto(url, { waitUntil: "networkidle" });
    await page.waitForTimeout(600);
    const conn = await page.$eval("#axos-conn", e => e.style.display === "none" ? "" : e.textContent).catch(() => "no #axos-conn");
    if (conn) errs.push("conn: " + conn);
    await page.screenshot({ path: `${out}/${name}.png`, fullPage: true });
    console.log(name, errs.length ? "ERRORS\n  " + errs.join("\n  ") : "ok");
    failures += errs.length;
    await ctx.close();
  }
  await browser.close();
  process.exit(failures ? 1 : 0);
})();
