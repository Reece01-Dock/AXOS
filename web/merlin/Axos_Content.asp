<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Transitional//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-transitional.dtd">
<html xmlns="http://www.w3.org/1999/xhtml">
<head>
<meta http-equiv="X-UA-Compatible" content="IE=Edge"/>
<meta http-equiv="Content-Type" content="text/html; charset=utf-8" />
<meta HTTP-EQUIV="Pragma" CONTENT="no-cache">
<meta HTTP-EQUIV="Expires" CONTENT="-1">
<link rel="shortcut icon" href="/images/favicon.png">
<link rel="icon" href="/images/favicon.png">
<title><#Web_Title#> - AXOS</title>
<link rel="stylesheet" type="text/css" href="/index_style.css">
<link rel="stylesheet" type="text/css" href="/form_style.css">
<link rel="stylesheet" type="text/css" href="/userRpm/axos-embed.css">
<script type="text/javascript" src="/js/jquery.js"></script>
<script type="text/javascript" src="/state.js"></script>
<script type="text/javascript" src="/general.js"></script>
<script type="text/javascript" src="/popup.js"></script>
<script type="text/javascript" src="/help.js"></script>
<script type="text/javascript" src="/form.js"></script>
<script type="text/javascript" src="/userRpm/token.js"></script>
<script type="text/javascript" src="/userRpm/axos-embed.js"></script>
<script>
function initial(){
	try { show_menu(); } catch (e) {
		if (window.console && console.warn) console.warn("show_menu:", e);
	}
	axosEmbedInit();
}
</script>
</head>
<body onload="initial();" class="bg">
<div id="TopBanner"></div>
<div id="Loading" class="popup_bg"></div>
<iframe name="hidden_frame" id="hidden_frame" src="" width="0" height="0" frameborder="0"></iframe>

<form method="post" name="form" action="/start_apply.htm" target="hidden_frame">
<input type="hidden" name="current_page" value="Main_GameServer_Content.asp">
<input type="hidden" name="next_page" value="">
<input type="hidden" name="action_mode" value="">
<input type="hidden" name="action_script" value="">
<input type="hidden" name="action_wait" value="">
<input type="hidden" name="preferred_lang" id="preferred_lang" value="<% nvram_get("preferred_lang"); %>">
<input type="hidden" name="firmver" value="<% nvram_get("firmver"); %>">

<table class="content" align="center" cellpadding="0" cellspacing="0">
<tr>
	<td width="17">&nbsp;</td>
	<td valign="top" width="202">
		<div id="mainMenu"></div>
		<div id="submenu"></div>
	</td>
	<td valign="top">
		<div id="tabMenu" class="submenuBlock"></div>
		<table width="98%" border="0" align="left" cellpadding="0" cellspacing="0">
		<tr>
			<td valign="top">
				<table width="760px" border="0" cellpadding="4" cellspacing="0" class="FormTitle" id="FormTitle">
				<tbody>
				<tr>
					<td bgcolor="#4D595D" valign="top">
						<div class="formfonttitle">AXOS</div>
						<div style="margin:10px 0 10px 5px;" class="splitLine"></div>
						<div class="formfontdesc">
							Same control plane as MCP / CLI. Hot-deployed from JFFS — edit
							<code>web/merlin/</code>, run <code>deploy-router.sh</code>, no firmware flash.
						</div>

						<div id="axos-root">
							<div class="axos-toolbar">
								<input type="button" class="button_gen" id="axos-refresh" value="Refresh">
								<span id="axos-status" class="axos-status">connecting…</span>
							</div>

							<table width="100%" border="1" align="center" cellpadding="4" cellspacing="0" bordercolor="#6b8fa3" class="FormTable axos-table">
								<thead><tr><td colspan="2">System</td></tr></thead>
								<tbody id="axos-sys-body">
									<tr><th>Status</th><td>Loading…</td></tr>
								</tbody>
							</table>

							<table width="100%" border="1" align="center" cellpadding="4" cellspacing="0" bordercolor="#6b8fa3" class="FormTable axos-table">
								<thead><tr><td colspan="2">Resources</td></tr></thead>
								<tbody id="axos-res-body"></tbody>
							</table>

							<table width="100%" border="1" align="center" cellpadding="4" cellspacing="0" bordercolor="#6b8fa3" class="FormTable axos-table">
								<thead><tr><td colspan="2">DNS</td></tr></thead>
								<tbody>
									<tr>
										<th>WAN upstreams</th>
										<td>
											<input type="text" id="axos-dns-wan" class="input_32_table" style="width:320px" />
											<span class="hint">space-separated</span>
										</td>
									</tr>
									<tr>
										<th>DoT</th>
										<td id="axos-dns-dot">—</td>
									</tr>
									<tr>
										<th>Apply</th>
										<td><input type="button" class="button_gen" id="axos-dns-apply" value="Apply DNS"></td>
									</tr>
								</tbody>
							</table>

							<table width="100%" border="1" align="center" cellpadding="4" cellspacing="0" bordercolor="#6b8fa3" class="FormTable axos-table">
								<thead><tr><td colspan="2">QoS</td></tr></thead>
								<tbody>
									<tr>
										<th>Enabled</th>
										<td>
											<input type="checkbox" id="axos-qos-enable">
											<input type="button" class="button_gen" id="axos-qos-apply" value="Apply QoS" style="margin-left:12px">
										</td>
									</tr>
									<tr><th>Mode</th><td id="axos-qos-mode">—</td></tr>
								</tbody>
							</table>

							<table width="100%" border="1" align="center" cellpadding="4" cellspacing="0" bordercolor="#6b8fa3" class="FormTable axos-table">
								<thead><tr><td colspan="2">Diagnostics</td></tr></thead>
								<tbody>
									<tr>
										<th>Host</th>
										<td>
											<input type="text" id="axos-host" value="1.1.1.1" class="input_32_table" />
											<input type="button" class="button_gen" id="axos-ping" value="Ping">
											<input type="button" class="button_gen" id="axos-dnslookup" value="DNS">
											<input type="button" class="button_gen" id="axos-port" value="Port 443">
										</td>
									</tr>
									<tr>
										<th>Result</th>
										<td><pre id="axos-diag" class="axos-pre">—</pre></td>
									</tr>
								</tbody>
							</table>

							<table width="100%" border="1" align="center" cellpadding="4" cellspacing="0" bordercolor="#6b8fa3" class="FormTable axos-table">
								<thead><tr><td colspan="4">Wi-Fi</td></tr>
								<tr><th>Radio</th><th>SSID</th><th>Channel</th><th>Clients</th></tr></thead>
								<tbody id="axos-wifi-body"></tbody>
							</table>

							<table width="100%" border="1" align="center" cellpadding="4" cellspacing="0" bordercolor="#6b8fa3" class="FormTable axos-table">
								<thead><tr><td colspan="4">Clients</td></tr>
								<tr><th>Hostname</th><th>IP</th><th>MAC</th><th>Iface</th></tr></thead>
								<tbody id="axos-clients-body"></tbody>
							</table>

							<table width="100%" border="1" align="center" cellpadding="4" cellspacing="0" bordercolor="#6b8fa3" class="FormTable axos-table">
								<thead><tr><td colspan="5">VPN profiles</td></tr>
								<tr><th>Name</th><th>Type</th><th>Endpoint</th><th>Enabled</th><th></th></tr></thead>
								<tbody id="axos-vpn-body"></tbody>
							</table>

							<table width="100%" border="1" align="center" cellpadding="4" cellspacing="0" bordercolor="#6b8fa3" class="FormTable axos-table">
								<thead><tr><td colspan="4">DHCP reservations</td></tr>
								<tr><th>MAC</th><th>IP</th><th>Hostname</th><th></th></tr></thead>
								<tbody id="axos-dhcp-body"></tbody>
								<tbody>
									<tr>
										<th>Add</th>
										<td colspan="3">
											<input type="text" id="axos-dhcp-mac" placeholder="AA:BB:CC:DD:EE:FF" class="input_20_table" />
											<input type="text" id="axos-dhcp-ip" placeholder="192.168.50.50" class="input_15_table" />
											<input type="text" id="axos-dhcp-name" placeholder="hostname" class="input_15_table" />
											<input type="button" class="button_gen" id="axos-dhcp-add" value="Add">
										</td>
									</tr>
								</tbody>
							</table>

							<table width="100%" border="1" align="center" cellpadding="4" cellspacing="0" bordercolor="#6b8fa3" class="FormTable axos-table">
								<thead><tr><td colspan="2">Config backup</td></tr></thead>
								<tbody>
									<tr>
										<th>Snapshot</th>
										<td>
											<input type="button" class="button_gen" id="axos-backup" value="Create backup">
											<span id="axos-backup-status" class="hint"></span>
										</td>
									</tr>
									<tr><th>Known backups</th><td><pre id="axos-backups" class="axos-pre">—</pre></td></tr>
								</tbody>
							</table>
						</div>
					</td>
				</tr>
				</tbody>
				</table>
			</td>
		</tr>
		</table>
	</td>
	<td width="10" align="center" valign="top">&nbsp;</td>
</tr>
</table>
<div id="footer"></div>
</form>
</body>
</html>
