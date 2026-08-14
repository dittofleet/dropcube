# dropcube

**NOTE** this project was created for personal use. I am unable to guarantee the quality or polish that one may expect from a properly maintained project.

Dropbox, minus almost everything: a write-only file drop for agents on remote machines, with capability-URL viewing for you.

Agents run `dropcube upload <file>` and get back a private link to send you. The upload token can only write. It can never read, list, or delete files, so a leaked token on a remote machine exposes nothing. View links are unguessable and expire server-side.

Three parts:

- **`worker/`**: a Cloudflare Worker in front of an R2 bucket. Handles authenticated `PUT`s and serves signed `GET`s.
- **the `dropcube` CLI** (this repo's Go module): what agents invoke to upload.
- **an agent skill** (`skills/dropcube/SKILL.md`): tells Claude Code how and when to use the CLI.

## One-time backend setup

Requires a Cloudflare account (free tier is plenty: R2 has 10 GB storage free with zero egress fees, Workers 100k requests/day).

```sh
cd worker
bunx wrangler login
bunx wrangler r2 bucket create dropcube
bunx wrangler deploy                      # prints https://dropcube.<you>.workers.dev
# Until the secret below is set, the worker refuses every request with 503.

# Upload token: generate, save for the machines, set as worker secret
openssl rand -hex 32                      # keep this value, it is DROPCUBE_TOKEN
bunx wrangler secret put UPLOAD_TOKEN     # paste it when prompted

# Auto-delete uploads after 30 days. Keep this equal to RETENTION_DAYS in
# worker/src/index.js, which is what the worker tells browsers and what it
# answers once the deadline passes but before the deletion sweep runs.
bunx wrangler r2 bucket lifecycle add dropcube expire-after-30d --expire-days 30
```

Optional: to serve from your own (sub)domain instead of workers.dev, add it in the Cloudflare dashboard under the worker's settings ("Domains and Routes"), then use it as the `endpoint`. Links inherit whatever domain the upload came through, so no other config changes.

## Installing the CLI

```sh
curl -fsSL https://raw.githubusercontent.com/dittofleet/dropcube/main/install.sh | sh
```

Installs the latest release to `~/.local/bin/dropcube` (override with `DROPCUBE_INSTALL_DIR`) and sets up `~/.config/dropcube/config.json`. When a terminal is attached the installer prompts for your endpoint and token (press Enter to skip), and otherwise it creates a starter config for you to fill in.

**Provisioning a remote machine in one line**: pass the endpoint and token to the installer and it writes a ready-to-use config instead of the starter. The assignments go on the `sh` side of the pipe, because a prefix before `curl` would never reach the shell running the script:

```sh
curl -fsSL https://raw.githubusercontent.com/dittofleet/dropcube/main/install.sh \
  | DROPCUBE_ENDPOINT=https://dropcube.<you>.workers.dev \
    DROPCUBE_TOKEN=<upload token> sh
```

Supported platforms: macOS (arm64, x64), Linux (arm64, x64).

## Usage

```sh
dropcube upload report.html    # prints one view link per file
dropcube remove <link>         # delete an upload early
```

Links print to stdout, one per file in argument order, pipe-friendly for agents. Anyone with a link can view for 30 days, then the file is deleted and the link dies with it. Nobody can enumerate or guess links.

Opening `<link>/remove` in a browser deletes the file too. The link is the whole capability: holding it grants viewing and removal of that one file, nothing more.

## Agent skill

`skills/` holds an instruction snippet that teaches coding agents to
upload artifacts and hand you links whenever you ask for a file (or
produce one you should see from elsewhere). Install with
[Vercel skills](https://github.com/vercel-labs/skills) (skills.sh):

```sh
npx skills add https://github.com/dittofleet/dropcube
```

## Updating

`dropcube` checks once per day for new releases and prints a hint to stderr when an update is available.

Run `dropcube update` to upgrade.

The check is automatically skipped when:

- `CI` is set
- `DROPCUBE_NO_UPDATE_CHECK` is set
- stderr is not a TTY (piped or redirected)
- the binary was built locally (version is `dev`)

## Uninstalling

```sh
dropcube uninstall          # prompts for confirmation
dropcube uninstall --yes    # skip prompt
```

Removes the binary, `~/.config/dropcube/`, and `~/.local/share/dropcube/` (update cache). The Cloudflare worker, bucket, and uploaded files are not touched.

## Configuration

`~/.config/dropcube/config.json` (respects `$XDG_CONFIG_HOME`):

```json
{
  "schemaVersion": 1,
  "endpoint": "https://dropcube.<you>.workers.dev",
  "token": "<upload token>"
}
```

`DROPCUBE_ENDPOINT` and `DROPCUBE_TOKEN` env vars override the file.
