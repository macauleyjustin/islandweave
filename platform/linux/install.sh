#!/usr/bin/env bash
set -euo pipefail

PREFIX="${PREFIX:-$HOME/.local/bin}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

mkdir -p "$PREFIX" "$HOME/.local/share/applications" "$HOME/.config/systemd/user"
cp "$ROOT_DIR"/bin/island "$ROOT_DIR"/bin/islandd "$ROOT_DIR"/bin/island-desktop "$ROOT_DIR"/bin/island-navigator "$PREFIX/"
chmod +x "$PREFIX"/island "$PREFIX"/islandd "$PREFIX"/island-desktop "$PREFIX"/island-navigator

cp "$SCRIPT_DIR/island-desktop.desktop" "$SCRIPT_DIR/island-navigator.desktop" "$HOME/.local/share/applications/"

cat > "$HOME/.config/systemd/user/islandd.service" <<EOF
[Unit]
Description=IslandWeave User Daemon
After=network.target

[Service]
ExecStart=$PREFIX/islandd
Restart=on-failure
RestartSec=3

[Install]
WantedBy=default.target
EOF

echo "Installed IslandWeave binaries to $PREFIX and desktop/systemd user files."
