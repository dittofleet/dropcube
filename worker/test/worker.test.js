// Run with `bun test` from worker/. Stands in for the Workers runtime pieces
// the worker uses (R2 buckets, FixedLengthStream, timingSafeEqual) and for
// Access's signing-key endpoint, then drives the worker through fetch.

import { beforeEach, describe, expect, test } from "bun:test";
import { timingSafeEqual } from "node:crypto";
import worker from "../src/index.js";

crypto.subtle.timingSafeEqual ??= (a, b) => timingSafeEqual(a, b);
globalThis.FixedLengthStream ??= class extends TransformStream {
  constructor() {
    super();
  }
};

const TOKEN = "test-token";
const AUD = "test-aud";
const ORIGIN = "https://drop.example";

function memoryBucket() {
  const store = new Map();
  const bucket = {
    store,
    reads: 0,
    async put(key, body, opts = {}) {
      const data = new Uint8Array(await new Response(body).arrayBuffer());
      store.set(key, {
        data,
        httpMetadata: opts.httpMetadata ?? {},
        customMetadata: opts.customMetadata ?? {},
        uploaded: new Date(),
      });
    },
    async get(key) {
      bucket.reads++;
      const o = store.get(key);
      return o ? { ...describeObject(o), body: new Response(o.data).body } : null;
    },
    async head(key) {
      bucket.reads++;
      const o = store.get(key);
      return o ? describeObject(o) : null;
    },
    async delete(key) {
      store.delete(key);
    },
  };
  return bucket;
}

function describeObject(o) {
  return {
    size: o.data.length,
    uploaded: o.uploaded,
    httpEtag: '"etag"',
    httpMetadata: o.httpMetadata,
    customMetadata: o.customMetadata,
    writeHttpMetadata(headers) {
      if (o.httpMetadata.contentType) headers.set("Content-Type", o.httpMetadata.contentType);
    },
  };
}

// Each test gets its own team domain, which resets the worker's key cache,
// so no key carries over from one test into the next.
let teamCount = 0;
// Work the worker handed to ctx.waitUntil, so tests can wait for it.
const background = [];

function setup(vars = {}) {
  const env = {
    API_TOKEN: TOKEN,
    BUCKET: memoryBucket(),
    KEEP: memoryBucket(),
    ...vars,
  };
  const ctx = { waitUntil: (p) => background.push(p) };
  const call = (method, path, headers = {}, body) =>
    worker.fetch(new Request(`${ORIGIN}${path}`, { method, headers, body }), env, ctx);
  return { env, call };
}

const auth = { Authorization: `Bearer ${TOKEN}` };

async function upload(call, name = "report.txt", content = "hello") {
  const res = await call("PUT", `/${name}`, { ...auth, "Content-Type": "text/plain" }, content);
  expect(res.status).toBe(201);
  return new URL((await res.text()).trim()).pathname;
}

describe("public mode", () => {
  test("upload, view, keep, remove", async () => {
    const { env, call } = setup({ PUBLIC_LINKS: "true" });
    const link = await upload(call);
    expect(link).toMatch(/^\/f\/[0-9a-f]{16}\/report\.txt$/);

    const view = await call("GET", link);
    expect(view.status).toBe(200);
    expect(await view.text()).toBe("hello");

    const key = link.slice("/f/".length);
    expect((await call("POST", `/keep/${key}`)).status).toBe(401);
    expect((await call("POST", `/keep/${key}`, auth)).status).toBe(200);
    expect(env.KEEP.store.has(key)).toBe(true);
    expect(env.BUCKET.store.has(key)).toBe(false);
    expect((await call("GET", link)).status).toBe(200);

    expect((await call("POST", `/remove/${key}`)).status).toBe(401);
    expect((await call("POST", `/remove/${key}`, auth)).status).toBe(200);
    expect((await call("GET", link)).status).toBe(404);
  });

  test("actions are not under the view path", async () => {
    const { call } = setup({ PUBLIC_LINKS: "true" });
    const link = await upload(call);
    expect((await call("POST", `${link}/keep`, auth)).status).toBe(405);
    expect((await call("GET", link.replace("/f/", "/remove/"))).status).toBe(405);
  });

  test("an expired file reads as gone", async () => {
    const { env, call } = setup({ PUBLIC_LINKS: "true" });
    const link = await upload(call);
    env.BUCKET.store.get(link.slice(3)).uploaded = new Date(Date.now() - 31 * 86400 * 1000);
    expect((await call("GET", link)).status).toBe(410);
  });
});

describe("configuration", () => {
  test("without Access settings or PUBLIC_LINKS nothing is served", async () => {
    const { call } = setup();
    for (const [method, path] of [
      ["GET", "/f/0123456789abcdef/a.txt"],
      ["PUT", "/a.txt"],
      ["POST", "/remove/0123456789abcdef/a.txt"],
    ])
      expect((await call(method, path, auth)).status).toBe(503);
  });

  test("service worker scripts are refused", async () => {
    const { call } = setup({ PUBLIC_LINKS: "true" });
    const link = await upload(call, "sw.js", "self.onfetch = () => {}");
    expect((await call("GET", link, { "Service-Worker": "script" })).status).toBe(403);
  });
});

describe("id-only links", () => {
  test("links carry only the id, the name comes back as the download name", async () => {
    const { call } = setup({ PUBLIC_LINKS: "true", ID_ONLY_LINKS: "true" });
    const link = await upload(call, "secret plans.txt");
    expect(link).toMatch(/^\/f\/[0-9a-f]{16}$/);

    const view = await call("GET", link);
    expect(view.status).toBe(200);
    expect(view.headers.get("Content-Disposition")).toContain('filename="secret plans.txt"');

    const id = link.slice("/f/".length);
    expect((await call("POST", `/keep/${id}`, auth)).status).toBe(200);
    expect((await call("GET", link)).status).toBe(200);
    expect((await call("POST", `/remove/${id}`, auth)).status).toBe(200);
    expect((await call("GET", link)).status).toBe(404);
  });
});

describe("Access mode", () => {
  let keyPair, otherKeyPair, team, certFetches, certsDown;

  beforeEach(async () => {
    const params = {
      name: "RSASSA-PKCS1-v1_5",
      modulusLength: 2048,
      publicExponent: new Uint8Array([1, 0, 1]),
      hash: "SHA-256",
    };
    keyPair ??= await crypto.subtle.generateKey(params, true, ["sign", "verify"]);
    otherKeyPair ??= await crypto.subtle.generateKey(params, true, ["sign", "verify"]);
    team = `team${++teamCount}.cloudflareaccess.com`;
    certFetches = 0;
    certsDown = false;
    const jwk = { ...(await crypto.subtle.exportKey("jwk", keyPair.publicKey)), kid: "k1" };
    globalThis.fetch = async (url) => {
      expect(String(url)).toBe(`https://${team}/cdn-cgi/access/certs`);
      certFetches++;
      if (certsDown) return new Response("down", { status: 500 });
      return Response.json({ keys: [jwk] });
    };
  });

  const b64url = (bytes) =>
    btoa(String.fromCharCode(...new Uint8Array(bytes)))
      .replace(/\+/g, "-")
      .replace(/\//g, "_")
      .replace(/=+$/, "");
  const b64json = (value) => b64url(new TextEncoder().encode(JSON.stringify(value)));

  async function jwt(claims = {}, { kid = "k1", alg = "RS256", key = keyPair.privateKey } = {}) {
    const now = Math.floor(Date.now() / 1000);
    const head = b64json({ alg, kid, typ: "JWT" });
    const body = b64json({
      aud: [AUD],
      iss: `https://${team}`,
      exp: now + 600,
      iat: now,
      email: "me@example.com",
      ...claims,
    });
    const sig = await crypto.subtle.sign(
      "RSASSA-PKCS1-v1_5",
      key,
      new TextEncoder().encode(`${head}.${body}`),
    );
    return `${head}.${body}.${b64url(sig)}`;
  }

  const privateSetup = () => setup({ ACCESS_TEAM_DOMAIN: team, ACCESS_AUD: AUD });
  const login = (token) => ({ "Cf-Access-Jwt-Assertion": token });

  test("a valid Access login can view, and the browser must recheck it next time", async () => {
    const { call } = privateSetup();
    const link = await upload(call);
    const view = await call("GET", link, login(await jwt()));
    expect(view.status).toBe(200);
    expect(await view.text()).toBe("hello");
    expect(view.headers.get("Cache-Control")).toBe("private, no-cache");
  });

  test("the team domain is always fetched over https", async () => {
    const { call } = setup({ ACCESS_TEAM_DOMAIN: `http://${team}/`, ACCESS_AUD: AUD });
    const link = await upload(call);
    expect((await call("GET", link, login(await jwt()))).status).toBe(200);
  });

  test("bad logins are refused", async () => {
    const { call } = privateSetup();
    const link = await upload(call);
    const now = Math.floor(Date.now() / 1000);
    const cases = {
      missing: undefined,
      garbage: "not.a.jwt",
      "signed by another key": await jwt({}, { key: otherKeyPair.privateKey }),
      "unknown key id": await jwt({}, { kid: "k2" }),
      "alg none": (await jwt({}, { alg: "none" })).replace(/\.[^.]*$/, "."),
      expired: await jwt({ exp: now - 600 }),
      "not yet valid": await jwt({ nbf: now + 600 }),
      "another application": await jwt({ aud: ["other-aud"] }),
      "another team": await jwt({ iss: "https://evil.cloudflareaccess.com" }),
    };
    for (const [name, token] of Object.entries(cases)) {
      const res = await call("GET", link, token ? login(token) : {});
      expect({ name, status: res.status }).toEqual({ name, status: 403 });
    }
  });

  test("without a login a real link and a wrong one look the same, and nothing is read", async () => {
    const { env, call } = privateSetup();
    const link = await upload(call);
    const [id, name] = link.slice(3).split("/");
    const answers = [];
    for (const path of [link, `/f/${id}/wrong.txt`, `/f/ffffffffffffffff/${name}`, "/f/%zz"]) {
      const res = await call("GET", path);
      answers.push(`${res.status} ${await res.text()}`);
    }
    expect(new Set(answers)).toEqual(new Set(["403 forbidden\n"]));
    expect(env.BUCKET.reads + env.KEEP.reads).toBe(0);

    for (const path of [`/remove/${id}/${name}`, `/remove/${id}/wrong.txt`, "/keep/ffff/x"]) {
      const res = await call("POST", path);
      expect(`${res.status} ${await res.text()}`).toBe("401 unauthorized\n");
    }
    expect(env.BUCKET.reads + env.KEEP.reads).toBe(0);
    expect(env.BUCKET.store.size).toBe(1);
  });

  test("an Access login does not authorize keep or remove", async () => {
    const { call } = privateSetup();
    const link = await upload(call);
    const res = await call("POST", link.replace("/f/", "/remove/"), login(await jwt()));
    expect(res.status).toBe(401);
  });

  test("signing keys are cached, and unknown key ids do not refetch every time", async () => {
    const { call } = privateSetup();
    const link = await upload(call);
    for (let i = 0; i < 3; i++) await call("GET", link, login(await jwt()));
    for (let i = 0; i < 3; i++) await call("GET", link, login(await jwt({}, { kid: "nope" })));
    expect(certFetches).toBe(1);
  });

  test("unreachable signing keys fail closed, and failures are not retried every request", async () => {
    const { call } = privateSetup();
    const link = await upload(call);
    certsDown = true;
    const token = await jwt();
    for (let i = 0; i < 3; i++) expect((await call("GET", link, login(token))).status).toBe(503);
    expect(certFetches).toBe(1);
  });

  test("concurrent first views all get in", async () => {
    const { call } = privateSetup();
    const link = await upload(call);
    const token = await jwt();
    const views = await Promise.all([1, 2, 3].map(() => call("GET", link, login(token))));
    expect(views.map((v) => v.status)).toEqual([200, 200, 200]);
  });

  test("expired keys keep serving while they refresh in the background", async () => {
    const { call } = privateSetup();
    const link = await upload(call);
    const token = await jwt();
    expect((await call("GET", link, login(token))).status).toBe(200);
    const realNow = Date.now;
    Date.now = () => realNow() + 2 * 3600 * 1000;
    try {
      certsDown = true;
      expect((await call("GET", link, login(await jwt()))).status).toBe(200);
      await Promise.all(background.splice(0));
      expect(certFetches).toBe(2);
    } finally {
      Date.now = realNow;
    }
  });

  test("tokens a few seconds off the worker's clock still work", async () => {
    const { call } = privateSetup();
    const link = await upload(call);
    const now = Math.floor(Date.now() / 1000);
    expect((await call("GET", link, login(await jwt({ nbf: now + 5 })))).status).toBe(200);
    expect((await call("GET", link, login(await jwt({ exp: now - 5 })))).status).toBe(200);
  });

  test("the team domain tolerates case, whitespace and a trailing path", async () => {
    const { call } = setup({ ACCESS_TEAM_DOMAIN: ` ${team.toUpperCase()}/x\n`, ACCESS_AUD: AUD });
    const link = await upload(call);
    expect((await call("GET", link, login(await jwt()))).status).toBe(200);
  });
});
