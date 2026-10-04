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

## Private uploads

`dropcube upload --private <file>` sends the file to the user's private deployment, where opening the link also takes their login. Use it only when the user asks for a private upload. If it fails with "no private deployment", tell the user it isn't set up.

## After uploading

- Links stop working after 30 days, when the file is deleted. No need to mention this.
- Anyone with a regular (not private) link can open the file, so only post it where the user wants it.
- You can't list or read back uploads, so keep the link you were given.
- Uploaded the wrong file or an old version? Upload the right one, then delete the old one with `dropcube remove <link>`.
- `dropcube keep <link>` stops a file from expiring, and the link stays the same. Only do this when the user asks.

## When it fails

`command not found`, `config not found`, `still has placeholder values` or `HTTP 401` mean dropcube isn't installed or set up on this machine. Tell the user and stop. Don't go looking for tokens or edit the config yourself.
