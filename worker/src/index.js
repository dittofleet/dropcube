// dropcube: write-only file drop for agents, capability-URL viewing for me.
//
// PUT /<filename>        (Authorization: Bearer <UPLOAD_TOKEN>)  -> view URL
// GET /f/<id>/<filename>                                         -> file contents
// GET /f/<id>/<filename>/keep                                    -> stop it expiring
// GET /f/<id>/<filename>/remove                                  -> delete the file
//
// A link is a pure capability: the unguessable random id is the whole
// secret, and holding it grants everything about that one file (view it,
// keep it, remove it) and nothing else. Uploads live for 30 days, then the
// bucket's lifecycle rule deletes them and the link dies with them.
//
// Keeping a file moves it to a second bucket that has no lifecycle rule.
// That rule is bucket-wide and cannot read anything off an object, so
// leaving the bucket is the only way to outlive it. The id does not change,
// so the link a file was shared with goes on working.

const DAY = 24 * 3600;
// Keep equal to the bucket's lifecycle rule (see README setup).
const RETENTION_DAYS = 30;
// A kept file has no deadline to cache against, but it can still be
// removed, so pick a ceiling that a removal does not have to outwait.
const KEEP_MAX_AGE = DAY;
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
        const toRemove = actionKey(url.pathname, "remove");
        if (toRemove !== null) return handleRemove(toRemove, env);
        const toKeep = actionKey(url.pathname, "keep");
        if (toKeep !== null) return handleKeep(toKeep, env);
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

  const { object, maxAge, status } = await find(key, request.headers, env);
  if (!object) return new Response("gone\n", { status });

  const filename = key.slice(key.indexOf("/") + 1);
  const headers = new Headers();
  object.writeHttpMetadata(headers);
  headers.set("etag", object.httpEtag);
  headers.set("X-Content-Type-Options", "nosniff");
  headers.set("Cache-Control", `private, max-age=${maxAge}`);
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

// Resolve a key to the object to serve, looking in the expiring bucket
// first because that is where nearly every live file is. An expiring object
// past its deadline counts as a miss, so a file kept while its old copy was
// still awaiting the sweep serves from KEEP rather than reading as gone.
async function find(key, headers, env) {
  const expiring = await env.BUCKET.get(key, { onlyIf: headers });
  if (expiring) {
    const remaining = timeLeft(expiring);
    if (remaining > 0) return { object: expiring, maxAge: remaining };
  }

  const kept = await env.KEEP.get(key, { onlyIf: headers });
  if (kept) return { object: kept, maxAge: KEEP_MAX_AGE };

  // A copy left in the expiring bucket proves the file was once real, so
  // say it is over rather than that it never existed.
  return { object: null, status: expiring ? 410 : 404 };
}

// Seconds an expiring object has left, negative once its deadline passes.
// The lifecycle rule deletes on its own schedule rather than at the exact
// second, so anything past retention counts as gone however long the sweep
// takes to catch up with it.
function timeLeft(object) {
  const age = Math.floor((Date.now() - object.uploaded.getTime()) / 1000);
  return RETENTION_DAYS * DAY - age;
}

async function handleRemove(key, env) {
  await Promise.all([env.BUCKET.delete(key), env.KEEP.delete(key)]);
  return new Response("removed\n", { status: 200 });
}

// Copy to KEEP before deleting from BUCKET, so an interrupted keep leaves
// the file in both places rather than neither: the duplicate serves fine and
// the expiring copy eventually sweeps itself away.
//
// A removal landing mid-copy is the case this ordering loses. It clears both
// buckets while KEEP is still empty, and the copy then lands in the bucket
// nothing expires, so the file outlives its own deletion. Removing again
// clears it, and closing the window for real needs a lock across the two
// requests that nothing else here would use.
async function handleKeep(key, env) {
  const object = await env.BUCKET.get(key);
  // A file past its deadline reads as gone from every other route, so it
  // cannot be kept either. Otherwise whether a link could still be rescued
  // would come down to how recently the sweep happened to run.
  if (!object || timeLeft(object) <= 0) {
    // Nothing left to promote. If it is already in KEEP the caller got what
    // they asked for, so keeping twice succeeds rather than reading as a
    // file that vanished.
    const kept = await env.KEEP.head(key);
    return kept
      ? new Response("kept\n", { status: 200 })
      : new Response("gone\n", { status: 404 });
  }

  // R2 rejects streams whose length it cannot know. The body of a get is
  // one of those, so declare the size the object already reports.
  const { readable, writable } = new FixedLengthStream(object.size);
  await Promise.all([
    env.KEEP.put(key, readable, {
      httpMetadata: object.httpMetadata,
      customMetadata: object.customMetadata,
    }),
    object.body.pipeTo(writable),
  ]);
  await env.BUCKET.delete(key);
  return new Response("kept\n", { status: 200 });
}

// An action is a view URL plus "/keep" or "/remove". Returns the object key
// to act on, or null when the path is not that action. The key must keep its
// id/filename shape so that a file literally named "keep" or "remove" still
// views normally (its action URL simply has one more segment).
function actionKey(pathname, action) {
  const suffix = `/${action}`;
  if (!pathname.endsWith(suffix)) return null;
  const key = safeDecode(pathname.slice(VIEW_PREFIX.length, -suffix.length));
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
