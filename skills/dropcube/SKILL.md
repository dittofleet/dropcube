---
name: dropcube
description: Send the user a file by uploading it with dropcube and replying with a link. Use when the user asks you to send, share or upload a file for them to look at, such as a screenshot, report, log or build output.
---

# dropcube: send files to the user

```sh
dropcube upload report.html
```

This prints a link. Reply with it so the user can open the file. You can pass several files at once and get one link per line, in the same order.

## Before uploading

- Never upload secrets: API keys, tokens, credentials, `.env` files, or anything containing them.
- Give the file a clear name first. The user sees it as the download name, and usually in the link too.
- Ask before uploading anything over 5 MB.

## Other deployments

Uploads go to the user's default deployment. The user may have others, such as a private one where opening a link also takes their login. To see them:

```sh
dropcube deployments
```

Each line is a name, the host its links come from, and a description of what it is for. To upload to one, pass its name with `--to`:

```sh
dropcube upload --to private report.html
```

Use a deployment other than the default only when the user asks for one, or when its description says it fits the request.

## After uploading

- Links stop working after 30 days, when the file is deleted. No need to mention this.
- On most deployments anyone with the link can open the file, so only post it where the user wants it.
- You can't list or read back uploads, so keep the link you were given.
- Uploaded the wrong file or an old version? Upload the right one, then delete the old one with `dropcube remove <link>`.
- `dropcube keep <link>` stops a file from expiring, and the link stays the same. Only do this when the user asks.

## When it fails

`command not found`, `config not found`, `still has placeholder values` or `HTTP 401` mean dropcube isn't installed or set up on this machine. Tell the user and stop. Don't go looking for tokens or edit the config yourself.

`no deployment named ...` means the user hasn't set that deployment up on this machine. Tell them, and don't fall back to another deployment on your own.
