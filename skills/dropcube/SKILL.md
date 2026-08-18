---
name: dropcube
description: Send a file to the user by uploading it and replying with a view link. Use when the user asks you to send/share/upload a file for viewing.
---

# dropcube: send files to the user

Upload any file and get back a private, expiring view link:

```sh
dropcube upload <file>
```

Prints one URL per file to stdout (multiple files allowed, links in argument order). Give the link(s) to the user so they can view the file you uploaded.

## Notes

- **NEVER upload secrets** (API keys, credentials, .env files) or sensitive files through dropcube.
- Links work for 30 days after upload, then the file is deleted. You do not need to mention this to the user.
- Links are unguessable, and anyone holding one can view (or remove) that file until it expires.
- To delete an upload early (wrong file, stale version), run `dropcube remove <link>`.
- If the user wants a file to outlive the 30 days, `dropcube keep <link>` stops it expiring and the link stays the same. This is the user's call, not something to do on your own.
- You cannot list or read back previous uploads, and a lost link cannot be recovered. Keep the printed URL.
- The filename becomes part of the link and the browser's download name, so give files meaningful names before uploading.
- `command not found` / `config not found` / `still has placeholder values` / `HTTP 401`: notify the user.
