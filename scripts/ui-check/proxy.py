"""Static server for the Merlin harness that proxies /v1/* to axosd."""
import http.server, urllib.request, urllib.error, sys, os
ROOT=sys.argv[1]; API=os.environ.get("AXOS_API","http://127.0.0.1:9090")
class H(http.server.SimpleHTTPRequestHandler):
    def __init__(s,*a,**k): super().__init__(*a,directory=ROOT,**k)
    def log_message(s,*a): pass
    def guess_type(s,p):
        return "text/html" if p.endswith(".asp") else super().guess_type(p)
    def _proxy(s):
        n=int(s.headers.get("Content-Length") or 0); body=s.rfile.read(n) if n else None
        req=urllib.request.Request(API+s.path,data=body,method=s.command)
        for h in ("Content-Type","X-Axos-Actor","X-Axos-UI-Token","Accept"):
            if s.headers.get(h): req.add_header(h,s.headers[h])
        try: r=urllib.request.urlopen(req,timeout=60); code=r.status; data=r.read(); ct=r.headers.get("Content-Type","")
        except urllib.error.HTTPError as e: code=e.code; data=e.read(); ct=e.headers.get("Content-Type","")
        s.send_response(code); s.send_header("Content-Type",ct); s.send_header("Content-Length",str(len(data))); s.end_headers(); s.wfile.write(data)
    def do_GET(s):
        if s.path.startswith("/v1/"): return s._proxy()
        return super().do_GET()
    def do_POST(s): s._proxy()
    def do_PUT(s): s._proxy()
    def do_DELETE(s): s._proxy()
http.server.ThreadingHTTPServer(("127.0.0.1",int(os.environ.get("HARNESS_PORT","8080"))),H).serve_forever()
