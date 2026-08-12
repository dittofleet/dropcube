---
name: dropcube
description: Send a file to the user by uploading it and replying with a view link. Use when the user asks you to send/share/upload a file for viewing.
---

# dropcube: send files to the user

Upload any file and get back a private, expiring view link:

```sh
dropcube upload <file>
```

Prints one URL per file to stdout (multiple files allowed, links in argument order). Include the link(s) in your final message to the user, since the upload is pointless if the link isn't delivered.

## Notes

- Links work for 30 days after upload, then the file is deleted.
- Links are unguessable, and anyone holding one can view (or remove) that file until it expires. Therefore: **never upload secrets** (API keys, credentials, .env files) through dropcube.
- To delete an upload early (wrong file, stale version), run `dropcube remove <link>`.
- You cannot list or read back previous uploads, and a lost link cannot be recovered. Keep the printed URL.
- The filename becomes part of the link and the browser's download name, so give files meaningful names before uploading.

## Troubleshooting

- `command not found`: install it:
  ```sh
  curl -fsSL https://raw.githubusercontent.com/sylophi/dropcube/main/install.sh | sh
  ```
  then ensure `~/.local/bin` is on PATH.
- `config not found` / `still has placeholder values`: this machine isn't provisioned. Config lives at `~/.config/dropcube/config.json` (`endpoint` + `token`), or set `DROPCUBE_ENDPOINT` and `DROPCUBE_TOKEN`. Ask the user for the values rather than guessing them.
- `HTTP 401`: the token is wrong or was rotated, so ask the user for a current one.
