---
name: dropcube
description: Send the user a file by uploading it with dropcube and replying with a link. Use when the user asks you to send, share or upload a file for them to look at, such as a screenshot, report, log or build output, including when they want it sent privately.
---

# dropcube: send files to the user

```sh
dropcube upload report.html
```

This prints a link. Reply with it so the user can open the file. You can pass several files at once and get one link per line, in the same order.

## Before uploading

- Never upload secrets: API keys, tokens, credentials, `.env` files, or anything containing them. This goes for private deployments too.
- Give the file a clear name first. The user sees it as the download name, and often in the link too.
- Ask before uploading anything over 5 MB.

## Deployments

The user can run more than one dropcube, each on its own domain with its own links. These are called deployments. Uploads go to the default one, and most of the time that's all you need. Only look further when the user asks for a private upload or names a deployment. Then list them:

```sh
dropcube deployments
```

```
default  dropcube.example.com
private  dc-private.example.com  Use it when the user asks for a private upload
```

Each line is a name, the domain its links come from, and the user's note on when to use it. Pick the one whose name or note matches the request and pass its name with `--to`:

```sh
dropcube upload --to private report.html
```

If none of them fits, tell the user instead of picking the closest one. In particular, never send a file the user wants kept private to a deployment that doesn't look private.

### Private deployments

A private deployment puts a login in front of its links, so opening one takes the user's Cloudflare Access login as well as the link. Without the login, every link gets the same `forbidden` answer, real or made up. This means:

- You can't check a private link by fetching it. Trust the link the upload printed.
- When the user opens it, they may be asked to sign in first. That's expected.
- Uploading, keeping and removing work the same as anywhere else.

### Links without filenames

Some deployments leave the filename out of links, which then end in a random id like `/f/k3J9xQ2mPq`. The file still downloads under its own name. Since the links all look alike, say which file each one is for when you send more than one.

## After uploading

- Links stop working after 30 days, when the file is deleted. No need to mention this.
- On a deployment that isn't private, anyone with the link can open the file, so only post it where the user wants it.
- You can't list or read back uploads, so keep the link you were given.
- Uploaded the wrong file or an old version? Upload the right one, then delete the old one with `dropcube remove <link>`.
- `dropcube keep <link>` stops a file from expiring, and the link stays the same. Only do this when the user asks.
- `keep` and `remove` work out the deployment from the link itself, so they don't take `--to`.

## When it fails

Tell the user and stop, without trying to fix it yourself, when you see:

- `command not found`, `config not found`, `still has placeholder values`, `failed to parse` or `invalid`: dropcube isn't installed or set up properly on this machine. Don't go looking for tokens or edit the config.
- `HTTP 401`: the token in the config is wrong.
- `HTTP 503`: the deployment isn't fully set up yet, for example its login settings are missing.
- `no deployment named ...`: the user hasn't set that deployment up here. Don't fall back to another deployment on your own.
- `not a link from ...` or `not a dropcube link`: the link isn't from any deployment set up on this machine.
- `unknown command` or `unknown flag`: the installed dropcube doesn't match these instructions. The user can run `dropcube update`.
