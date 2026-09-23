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
	show_menu();
	axosEmbedInit();
}
</script>
</head>
<body onload="initial();" class="bg">
<div id="TopBanner"></div>
<div id="Loading" class="popup_bg"></div>
<iframe name="hidden_frame" id="hidden_frame" src="" width="0" height="0" frameborder="0"></iframe>

<form method="post" name="form" action="/start_apply.htm" target="hidden_frame">
<input type="hidden" name="current_page" value="userRpm/Axos_Content.asp">
<input type="hidden" name="next_page" value="">
<input type="hidden" name="group_id" value="">
<input type="hidden" name="modified" value="0">
<input type="hidden" name="action_mode" value="">
<input type="hidden" name="action_script" value="">
<input type="hidden" name="action_wait" value="">
<input type="hidden" name="first_time" value="">
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
							Live control plane shared with MCP and CLI. UI files hot-deploy from JFFS
							(<code>/jffs/axos/merlin-ui</code>) — no firmware rebuild to iterate.
						</div>
						<div id="axos-root">
							<div class="axos-toolbar">
								<button type="button" class="button_gen" id="axos-refresh">Refresh</button>
								<span id="axos-status" class="axos-status">connecting…</span>
							</div>
							<div class="axos-grid">
								<section><h3>System</h3><pre id="axos-sys">…</pre></section>
								<section><h3>Resources</h3><pre id="axos-res">…</pre></section>
								<section><h3>Network</h3><pre id="axos-net">…</pre></section>
								<section><h3>Wi-Fi</h3><pre id="axos-wifi">…</pre></section>
								<section><h3>VPN / DNS / QoS</h3><pre id="axos-vpn">…</pre></section>
								<section>
									<h3>Diagnostics</h3>
									<div class="axos-diag">
										<input type="text" id="axos-host" value="1.1.1.1" class="input_20_table" />
										<button type="button" class="button_gen" id="axos-ping">Ping</button>
									</div>
									<pre id="axos-diag">—</pre>
								</section>
							</div>
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
