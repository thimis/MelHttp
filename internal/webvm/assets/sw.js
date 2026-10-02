// MelHttp transport service worker.
//
// Same-origin GET requests are re-sent with "X-Malbolge-Accept: program".
// melhttpd then answers with the body *as Malbolge programs*, and this worker
// runs them in the MelHttp VM (Go compiled to WebAssembly) to recover the
// bytes before the page sees them. This is obfuscation, not encryption:
// anyone can run the programs. Use HTTPS for confidentiality.
"use strict";

importScripts("/_melhttp/wasm_exec.js");

const vmReady = (async () => {
  const go = new Go();
  const { instance } = await WebAssembly.instantiateStreaming(fetch("/_melhttp/melhttp.wasm"), go.importObject);
  go.run(instance); // defines melhttpDecode and keeps running
})();

self.addEventListener("install", () => self.skipWaiting());
self.addEventListener("activate", (event) => event.waitUntil(self.clients.claim()));

self.addEventListener("fetch", (event) => {
  const req = event.request;
  const url = new URL(req.url);
  if (req.method !== "GET" || url.origin !== self.location.origin ||
      url.pathname.startsWith("/_melhttp/") || req.headers.has("Range")) {
    return; // the network handles it as usual
  }
  event.respondWith(fetchDecoded(req));
});

async function fetchDecoded(req) {
  try {
    await vmReady;
  } catch (err) {
    console.warn("MelHttp: VM failed to start; falling back to plain requests", err);
    return fetch(req);
  }
  const headers = new Headers(req.headers);
  headers.set("X-Malbolge-Accept", "program");
  const res = await fetch(req.url, { headers, credentials: "same-origin", cache: "no-store", redirect: "follow" });
  if (res.headers.get("X-Malbolge-Encoding") !== "program") {
    return res;
  }
  const program = new Uint8Array(await res.arrayBuffer());
  const started = performance.now();
  const result = self.melhttpDecode(program);
  if (result.error) {
    return new Response("MelHttp: could not decode the Malbolge response: " + result.error,
      { status: 502, headers: { "Content-Type": "text/plain; charset=utf-8" } });
  }
  const out = new Headers(res.headers);
  out.set("Content-Type", res.headers.get("X-Malbolge-Content-Type") || "application/octet-stream");
  for (const h of ["X-Malbolge-Encoding", "X-Malbolge-Content-Type", "Content-Length", "Content-Encoding"]) {
    out.delete(h);
  }
  out.set("X-Malbolge-Decoded", "service-worker");
  out.set("X-Malbolge-Program-Bytes", String(program.length));
  out.set("X-Malbolge-Decode-Ms", (performance.now() - started).toFixed(2));
  return new Response(result.data, { status: res.status, statusText: res.statusText, headers: out });
}
