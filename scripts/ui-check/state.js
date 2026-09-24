// Harness stand-in for Merlin's state.js (scripts/ui-check): show_menu,
// showLoading and hideLoading only: renders a Merlin-like menu + tab strip.
var AXOS_HARNESS_PAGES = {all:["Administration","Main_GameServer_Content.asp",["Operation Mode","System","Firmware Upgrade","AXOS","Restore/Save/Upload Setting"]],
 vpn:["VPN","Advanced_VPN_PPTP.asp",["VPN Status","VPN Server","VPN Client","VPN Director","AXOS"]],
 lan:["LAN","Advanced_APPList_Content.asp",["LAN IP","DHCP Server","AXOS","Route","IPTV"]],
 dns:["WAN","WAN_info.asp",["Internet Connection","Dual WAN","Port Trigger","Virtual Server / Port Forwarding","DMZ","DDNS","AXOS"]],
 firewall:["Firewall","Advanced_VPN_IPSec.asp",["General","AXOS","URL Filter","Keyword Filter","Network Services Filter"]],
 qos:["Adaptive QoS","Advanced_AiDisk_webdav.asp",["Bandwidth Monitor","QoS","AXOS","Web History"]],
 wifi:["Wireless","WiFi_Insight.asp",["General","AXOS","WPS","WDS","Wireless MAC Filter","RADIUS Setting","Professional"]],
 diag:["Network Tools","Guest_network.asp",["Network Analysis","AXOS","Netstat","Wake on LAN","Smart Connect Rule"]]};
function show_menu(){
  var cur = window.AXOS_SECTION || "all";
  var menu = '<div class="menu_Split" style="height:30px;line-height:30px">Advanced Settings</div>';
  for (var k in AXOS_HARNESS_PAGES){ var p=AXOS_HARNESS_PAGES[k];
    menu += '<div class="'+(k===cur?'menu_clicked':'menu')+'" style="line-height:46px;padding-left:10px" onclick="location.href=\'/'+p[1]+'\'">'+p[0]+'</div>'; }
  document.getElementById("mainMenu").innerHTML = menu;
  var tabs = ""; AXOS_HARNESS_PAGES[cur][2].forEach(function(t){ tabs += '<div class="'+(t==="AXOS"?'tabClicked':'tab')+'"><span>'+t+'</span></div>'; });
  document.getElementById("tabMenu").innerHTML = tabs;
  document.getElementById("TopBanner").innerHTML = '<div style="width:998px;margin:0 auto;height:60px;background:linear-gradient(#1C2A31,#2B373B);color:#fff;font:bold 20px Arial;line-height:60px;padding-left:20px;box-sizing:border-box">ROG GT-AX6000 (harness)</div>';
}
function showLoading(){ var l=document.getElementById("Loading"); l.style.cssText="display:block;position:fixed;inset:0;background:rgba(0,0,0,.5);z-index:500"; }
function hideLoading(){ document.getElementById("Loading").style.display="none"; }
