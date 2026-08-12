#!/bin/sh
set -eu

REPO="sylophi/dropcube"
DEST="${DROPCUBE_INSTALL_DIR:-$HOME/.local/bin}"

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$OS" in
  darwin|linux) ;;
  *) echo "Unsupported OS: $OS" >&2; exit 1 ;;
esac

ARCH=$(uname -m)
case "$ARCH" in
  arm64|aarch64) ARCH=arm64 ;;
  x86_64) ARCH=x64 ;;
  *) echo "Unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

ASSET="dropcube-${OS}-${ARCH}"
URL="https://github.com/${REPO}/releases/latest/download/${ASSET}"

mkdir -p "$DEST"
TMP=$(mktemp)
trap 'rm -f "$TMP"' EXIT

echo "Downloading $URL..." >&2
curl -fsSL "$URL" -o "$TMP"
chmod +x "$TMP"
mv "$TMP" "$DEST/dropcube"
echo "Installed dropcube to $DEST/dropcube" >&2

CONFIG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/dropcube"
CONFIG_FILE="$CONFIG_DIR/config.json"

# Config values come from (in order): DROPCUBE_ENDPOINT / DROPCUBE_TOKEN in
# the environment (making a fresh remote machine a single curl-pipe), an
# interactive prompt when a terminal is attached, or a starter config to
# fill in by hand. An existing config is only overwritten in the env case.
ENDPOINT="${DROPCUBE_ENDPOINT:-}"
TOKEN="${DROPCUBE_TOKEN:-}"

# Under `curl | sh` stdin is the script itself, so prompts go through
# /dev/tty. The subshell probe skips prompting when there is no terminal
# (CI, containers) rather than hanging or tripping set -e.
if [ -z "$ENDPOINT" ] && [ -z "$TOKEN" ] && [ ! -f "$CONFIG_FILE" ] \
  && ( : < /dev/tty ) 2>/dev/null; then
  printf "Endpoint URL (empty to fill in later): " > /dev/tty
  read -r ENDPOINT < /dev/tty || ENDPOINT=""
  if [ -n "$ENDPOINT" ]; then
    printf "Upload token: " > /dev/tty
    read -r TOKEN < /dev/tty || TOKEN=""
  fi
fi

if [ -n "$ENDPOINT" ] && [ -n "$TOKEN" ]; then
  MSG="Wrote config to $CONFIG_FILE"
elif [ ! -f "$CONFIG_FILE" ]; then
  MSG="Created starter config at $CONFIG_FILE, fill in endpoint and token"
else
  MSG=""
fi
if [ -n "$MSG" ]; then
  mkdir -p "$CONFIG_DIR"
  cat > "$CONFIG_FILE" <<EOF
{
  "schemaVersion": 1,
  "endpoint": "${ENDPOINT:-https://dropcube.<your-subdomain>.workers.dev}",
  "token": "${TOKEN:-<upload token>}"
}
EOF
  chmod 600 "$CONFIG_FILE"
  echo "$MSG" >&2
else
  echo "Config already exists at $CONFIG_FILE (left untouched)" >&2
fi

case ":$PATH:" in
  *":$DEST:"*) ;;
  *) echo "Note: $DEST is not in \$PATH. Add it to your shell profile to use dropcube." >&2 ;;
esac
