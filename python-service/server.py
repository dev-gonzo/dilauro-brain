import json
import os
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path == "/health":
            payload = {"status": "ok", "service": "brain-python"}
        elif self.path == "/version":
            payload = {"service": "brain-python", "version": "1.0.0"}
        else:
            self.send_error(404)
            return

        payload = json.dumps(payload).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)

    def log_message(self, format, *args):
        return


port = int(os.getenv("BRAIN_PORT", "47181"))
ThreadingHTTPServer(("0.0.0.0", port), Handler).serve_forever()
