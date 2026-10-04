// dropcube: write-only file drop for agents, private viewing for me.
//
// PUT  /<filename>   (Authorization: Bearer <API_TOKEN>)  -> view URL
// GET  /f/<key>      (Access login, or nothing in public mode)  -> file contents
// POST /keep/<key>   (Authorization: Bearer <API_TOKEN>)  -> stop it expiring
// POST /remove/<key> (Authorization: Bearer <API_TOKEN>)  -> delete the file
//
// <key> is whatever follows /f/ in a link: "<id>/<filename>", or "<id>"
// alone when ID_ONLY_LINKS is set and the filename should stay out of links.
// The random id is unguessable either way.
//
// Viewing takes a Cloudflare Access login unless PUBLIC_LINKS is set, in
// which case holding a link is enough. Every route checks its credential
// before it looks at the path, so without one a real link, a wrong filename
// and a made-up id all get the same answer. Keep and remove sit outside /f/
// so an Access policy on /f/* covers viewing and nothing else.
//
// Uploads live for 30 days, then the bucket's lifecycle rule deletes them
// and the link dies with them. Keeping a file moves it to a second bucket
// that has no lifecycle rule. That rule is bucket-wide and cannot read
// anything off an object, so leaving the bucket is the only way to outlive
// it. The key does not change, so the link a file was shared with goes on
// working.

const DAY = 24 * 3600;
// Keep equal to the bucket's lifecycle rule (see README setup).
const RETENTION_DAYS = 30;
// A kept file has no deadline to cache against, but it can still be
// removed, so pick a ceiling that a removal does not have to outwait.
const KEEP_MAX_AGE = DAY;
const VIEW_PREFIX = "/f/";
const ACTIONS = ["keep", "remove"];
// How long fetched Access signing keys are trusted, and the least time
// between refetches when a token names a key we do not have. The floor stops
// a stream of made-up key ids from turning into a stream of fetches.
const ACCESS_KEYS_TTL = 3600 * 1000;
const ACCESS_KEYS_REFETCH = 60 * 1000;

const enc = new TextEncoder();

export default {
  async fetch(request, env) {
    // Fail closed when the worker is deployed without its secret. Without
    // this, an unset API_TOKEN makes the expected header the literal
    // string "Bearer undefined", which anyone can send.
    if (!isNonEmptyString(env.API_TOKEN))
      return new Response("worker not configured: set API_TOKEN\n", { status: 503 });
    // Same for the Access settings: viewing is private unless PUBLIC_LINKS
    // says otherwise, so a forgotten setting locks files away instead of
    // opening them to anyone with a link.
    if (!isPublic(env) && !(isNonEmptyString(env.ACCESS_TEAM_DOMAIN) && isNonEmptyString(env.ACCESS_AUD)))
      return new Response(
        "worker not configured: set ACCESS_TEAM_DOMAIN and ACCESS_AUD, or PUBLIC_LINKS\n",
        { status: 503 },
      );

    // A service worker registered from one upload would see every file
    // opened after it, and with id-only links its scope would be all of /f/.
    // No real file needs to be one, so refuse to serve any as one.
    if (request.headers.has("Service-Worker"))
      return new Response("forbidden\n", { status: 403 });

    const url = new URL(request.url);

    if (request.method === "PUT") return handleUpload(request, url, env);
    if (url.pathname.startsWith(VIEW_PREFIX)) {
      if (request.method === "GET" || request.method === "HEAD")
        return handleView(request, url, env);
      return new Response("method not allowed\n", {
        status: 405,
        headers: { Allow: "GET, HEAD" },
      });
    }
    const action = ACTIONS.find((a) => url.pathname.startsWith(`/${a}/`));
    if (action) {
      if (request.method === "POST") return handleAction(request, url, env, action);
      return new Response("method not allowed\n", { status: 405, headers: { Allow: "POST" } });
    }

    return new Response("not found\n", { status: 404 });
  },
};

function isPublic(env) {
  return String(env.PUBLIC_LINKS) === "true";
}

function idOnly(env) {
  return String(env.ID_ONLY_LINKS) === "true";
}

function authorized(request, env) {
  const auth = request.headers.get("Authorization") ?? "";
  return timingSafeEqual(auth, `Bearer ${env.API_TOKEN}`);
}

async function handleUpload(request, url, env) {
  if (!authorized(request, env)) return new Response("unauthorized\n", { status: 401 });

  const raw = safeDecode(url.pathname.slice(1));
  if (raw === null) return new Response("malformed path\n", { status: 400 });
  const filename = sanitizeFilename(raw);
  if (!filename) return new Response("missing filename\n", { status: 400 });

  // An id-only key has nowhere to carry the filename, so it rides along as
  // metadata and comes back out as the download name.
  const id = randomId();
  const key = idOnly(env) ? id : `${id}/${filename}`;
  await env.BUCKET.put(key, request.body, {
    httpMetadata: {
      contentType: request.headers.get("Content-Type") ?? "application/octet-stream",
    },
    customMetadata: { filename },
  });

  const path = idOnly(env) ? id : `${id}/${encodeURIComponent(filename)}`;
  return new Response(`${url.origin}${VIEW_PREFIX}${path}\n`, {
    status: 201,
    headers: { "Content-Type": "text/plain" },
  });
}

async function handleView(request, url, env) {
  if (!isPublic(env)) {
    const login = await accessLogin(request, env);
    if (login === null)
      return new Response("access keys unavailable\n", { status: 503 });
    if (!login) return new Response("forbidden\n", { status: 403 });
  }

  const key = safeDecode(url.pathname.slice(VIEW_PREFIX.length));
  if (key === null) return new Response("not found\n", { status: 404 });

  const { object, maxAge, status } = await find(key, request.headers, env);
  if (!object) return new Response("gone\n", { status });

  // Files uploaded before id-only links existed carry their name only in
  // the key, so fall back to that.
  const filename = object.customMetadata?.filename ?? key.slice(key.indexOf("/") + 1);
  const headers = new Headers();
  object.writeHttpMetadata(headers);
  headers.set("etag", object.httpEtag);
  headers.set("X-Content-Type-Options", "nosniff");
  // A private file must go back through the login check on every view, or
  // the browser would keep showing it after the Access session ends. The
  // etag still makes an unchanged file a cheap 304.
  headers.set(
    "Cache-Control",
    isPublic(env) ? `private, max-age=${maxAge}` : "private, no-cache",
  );
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

async function handleAction(request, url, env, action) {
  if (!authorized(request, env)) return new Response("unauthorized\n", { status: 401 });
  const key = safeDecode(url.pathname.slice(action.length + 2));
  if (!key) return new Response("not found\n", { status: 404 });
  return action === "keep" ? handleKeep(key, env) : handleRemove(key, env);
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

// Checks the token Cloudflare Access attaches to every request it lets
// through, rather than trusting that Access sits in front at all. Access
// matches paths its own way and can be switched off or reconfigured, and in
// any of those cases a request arriving without a valid token is refused
// here instead of being served. Returns true or false, or null when the
// signing keys cannot be fetched.
async function accessLogin(request, env) {
  const token = request.headers.get("Cf-Access-Jwt-Assertion");
  const parts = token?.split(".") ?? [];
  if (parts.length !== 3) return false;
  const header = parseJson(base64UrlDecode(parts[0]));
  const claims = parseJson(base64UrlDecode(parts[1]));
  const signature = base64UrlDecode(parts[2]);
  if (header?.alg !== "RS256" || typeof header.kid !== "string" || !claims || !signature)
    return false;

  const team = teamOrigin(env);
  const key = await accessKey(team, header.kid);
  if (key === null) return null;
  if (!key) return false;
  const signed = await crypto.subtle.verify(
    "RSASSA-PKCS1-v1_5",
    key,
    signature,
    enc.encode(`${parts[0]}.${parts[1]}`),
  );
  if (!signed) return false;

  // The signature only proves Access issued the token. It still has to be
  // for this application (any other app on the same team signs with the
  // same keys) and still be current.
  const now = Date.now() / 1000;
  const audiences = Array.isArray(claims.aud) ? claims.aud : [claims.aud];
  return (
    claims.iss === team &&
    audiences.includes(env.ACCESS_AUD) &&
    typeof claims.exp === "number" &&
    claims.exp > now &&
    (claims.nbf === undefined || claims.nbf <= now)
  );
}

// ACCESS_TEAM_DOMAIN may be given bare ("team.cloudflareaccess.com") or as
// a URL. Either way the keys are fetched over https, and tokens name the
// team as that https origin.
function teamOrigin(env) {
  const domain = env.ACCESS_TEAM_DOMAIN.replace(/^[a-z]+:\/\//i, "").replace(/\/+$/, "");
  return `https://${domain}`;
}

// Signing keys per team, cached for the isolate's lifetime. Access rotates
// keys from time to time, so a token naming a key we lack triggers a
// refetch. Attempts, failed ones included, are spaced ACCESS_KEYS_REFETCH
// apart, and requests arriving mid-fetch wait on the same one.
const accessKeys = new Map();

// Returns the key for kid, false when the team does not have it, or null
// when the keys cannot be fetched.
async function accessKey(team, kid) {
  let cached = accessKeys.get(team);
  if (!cached) {
    cached = { keys: null, fetchedAt: 0, attemptedAt: -Infinity, pending: null };
    accessKeys.set(team, cached);
  }
  const now = Date.now();
  const wanted = now - cached.fetchedAt > ACCESS_KEYS_TTL || !cached.keys?.has(kid);
  if (wanted && !cached.pending && now - cached.attemptedAt > ACCESS_KEYS_REFETCH) {
    cached.attemptedAt = now;
    cached.pending = fetchAccessKeys(team).then((keys) => {
      if (keys) Object.assign(cached, { keys, fetchedAt: Date.now() });
      cached.pending = null;
    });
  }
  if (cached.pending) await cached.pending;
  // When a refresh fails, the keys from the last good fetch keep serving
  // rather than locking every view out until Access answers again.
  if (!cached.keys) return null;
  return cached.keys.get(kid) ?? false;
}

async function fetchAccessKeys(team) {
  try {
    const response = await fetch(`${team}/cdn-cgi/access/certs`);
    if (!response.ok) return null;
    const { keys } = await response.json();
    const imported = new Map();
    for (const jwk of keys ?? []) {
      if (jwk.kty !== "RSA" || typeof jwk.kid !== "string") continue;
      imported.set(
        jwk.kid,
        await crypto.subtle.importKey(
          "jwk",
          jwk,
          { name: "RSASSA-PKCS1-v1_5", hash: "SHA-256" },
          false,
          ["verify"],
        ),
      );
    }
    return imported;
  } catch {
    return null;
  }
}

function base64UrlDecode(s) {
  try {
    const b64 = s.replace(/-/g, "+").replace(/_/g, "/");
    return Uint8Array.from(atob(b64 + "=".repeat((4 - (b64.length % 4)) % 4)), (c) =>
      c.charCodeAt(0),
    );
  } catch {
    return null;
  }
}

function parseJson(bytes) {
  if (!bytes) return null;
  try {
    const value = JSON.parse(new TextDecoder().decode(bytes));
    return value && typeof value === "object" ? value : null;
  } catch {
    return null;
  }
}
