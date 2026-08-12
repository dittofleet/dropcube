// dropcube: write-only file drop for agents, capability-URL viewing for me.
//
// PUT /<filename>        (Authorization: Bearer <UPLOAD_TOKEN>)  -> view URL
// GET /f/<id>/<filename>                                         -> file contents
// GET /f/<id>/<filename>/remove                                  -> delete the file
//
// A link is a pure capability: the unguessable random id is the whole
// secret, and holding it grants everything about that one file (view it,
// remove it) and nothing else. Everything lives for 30 days, then the
// bucket's lifecycle rule deletes it and the link dies with it.

// Keep equal to the bucket's lifecycle rule (see README setup).
const RETENTION_DAYS = 30;
const VIEW_PREFIX = "/f/";

const enc = new TextEncoder();

export default {
  async fetch(request, env) {
    // Fail closed when the worker is deployed without its secret. Without
    // this, an unset UPLOAD_TOKEN makes the expected header the literal
    // string "Bearer undefined", which anyone can send.
    if (!isNonEmptyString(env.UPLOAD_TOKEN))
      return new Response("worker not configured: set UPLOAD_TOKEN\n", { status: 503 });

    const url = new URL(request.url);

    if (request.method === "PUT") return handleUpload(request, url, env);
    if (url.pathname.startsWith(VIEW_PREFIX)) {
      if (request.method === "GET") {
        const key = removeKey(url.pathname);
        if (key !== null) {
          await env.BUCKET.delete(key);
          return new Response("removed\n", { status: 200 });
        }
      }
      if (request.method === "GET" || request.method === "HEAD")
        return handleView(request, url, env);
      return new Response("method not allowed\n", {
        status: 405,
        headers: { Allow: "GET, HEAD" },
      });
    }

    return new Response("not found\n", { status: 404 });
  },
};

async function handleUpload(request, url, env) {
  const auth = request.headers.get("Authorization") ?? "";
  if (!timingSafeEqual(auth, `Bearer ${env.UPLOAD_TOKEN}`))
    return new Response("unauthorized\n", { status: 401 });

  const raw = safeDecode(url.pathname.slice(1));
  if (raw === null) return new Response("malformed path\n", { status: 400 });
  const filename = sanitizeFilename(raw);
  if (!filename) return new Response("missing filename\n", { status: 400 });

  const id = randomId();
  await env.BUCKET.put(`${id}/${filename}`, request.body, {
    httpMetadata: {
      contentType: request.headers.get("Content-Type") ?? "application/octet-stream",
    },
  });

  return new Response(`${url.origin}${VIEW_PREFIX}${id}/${encodeURIComponent(filename)}\n`, {
    status: 201,
    headers: { "Content-Type": "text/plain" },
  });
}

async function handleView(request, url, env) {
  const key = safeDecode(url.pathname.slice(VIEW_PREFIX.length));
  if (key === null) return new Response("not found\n", { status: 404 });

  const object = await env.BUCKET.get(key, { onlyIf: request.headers });
  if (!object) return new Response("gone\n", { status: 404 });

  // The lifecycle rule deletes on its own schedule, not at the exact
  // second. Treat anything past retention as already gone.
  const age = Math.floor((Date.now() - object.uploaded.getTime()) / 1000);
  const remaining = RETENTION_DAYS * 24 * 3600 - age;
  if (remaining < 0) return new Response("gone\n", { status: 410 });

  const filename = key.slice(key.indexOf("/") + 1);
  const headers = new Headers();
  object.writeHttpMetadata(headers);
  headers.set("etag", object.httpEtag);
  headers.set("X-Content-Type-Options", "nosniff");
  headers.set("Cache-Control", `private, max-age=${remaining}`);
  // filename is already sanitized at upload time, but encode at the header
  // layer anyway so Content-Disposition stays valid whatever the sanitizer
  // allows in the future.
  headers.set(
    "Content-Disposition",
    `inline; filename="${filename.replace(/["\\]/g, "_")}"; filename*=UTF-8''${encodeURIComponent(filename)}`,
  );

  // R2 reports every failed precondition the same way (an object with no
  // body), so pick the status from which conditional header was sent.
  if (!object.body) {
    const strict =
      request.headers.has("If-Match") || request.headers.has("If-Unmodified-Since");
    return new Response(null, { status: strict ? 412 : 304, headers });
  }
  return new Response(object.body, { headers });
}

// A removal is a view URL plus "/remove". Returns the object key to
// delete, or null when the path is not a removal request. The key must
// keep its id/filename shape so that a file literally named "remove"
// still views normally (its removal URL simply has one more segment).
function removeKey(pathname) {
  if (!pathname.endsWith("/remove")) return null;
  const key = safeDecode(pathname.slice(VIEW_PREFIX.length, -"/remove".length));
  return key !== null && key.includes("/") ? key : null;
}

function safeDecode(s) {
  try {
    return decodeURIComponent(s);
  } catch {
    return null;
  }
}

function isNonEmptyString(v) {
  return typeof v === "string" && v.length > 0;
}

function sanitizeFilename(name) {
  return name
    .split("/")
    .pop()
    .replace(/[^\w.\- ()]/g, "_")
    .slice(0, 200);
}

// 64 bits of randomness: guessing a live link means hitting a 1-in-2^64
// id through a network round trip per attempt. Short enough to read.
function randomId() {
  return toHex(crypto.getRandomValues(new Uint8Array(8)));
}

function toHex(bytes) {
  return [...bytes].map((b) => b.toString(16).padStart(2, "0")).join("");
}

function timingSafeEqual(a, b) {
  const ab = enc.encode(a);
  const bb = enc.encode(b);
  if (ab.length !== bb.length) return false;
  return crypto.subtle.timingSafeEqual(ab, bb);
}
