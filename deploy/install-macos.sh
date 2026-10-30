#!/bin/sh
# One-time install of mcp-telegram under the dedicated service user _mcptg.
# Run from the repository root after `go build -o build/mcp-telegram ./cmd/mcp-telegram`:
#   sudo sh deploy/install-macos.sh
#
# Layout (all parents root-owned, so the agent's user cannot swap files):
#   /opt/mcp-telegram/bin/{mcp-telegram,run}  root:wheel   0755
#   /opt/mcp-telegram/etc/config.yaml         root:wheel   0644  (ACL; root edits only)
#   /opt/mcp-telegram/etc/secrets.env         root:_mcptg  0640  (TG_APP_ID, TG_API_HASH)
#   /opt/mcp-telegram/state/session.json      _mcptg       0600  (dir 0700)
#   /etc/sudoers.d/mcp-telegram               allows only `run serve` as _mcptg without a password
#
# config.yaml is taken from deploy/config.yaml (git-ignored, your real ACL)
# or, if it is missing, from deploy/config.example.yaml.
# Re-running updates the binary and wrapper; config.yaml and secrets.env are kept.
set -eu

if [ "$(id -u)" -ne 0 ]; then
	echo "run with sudo" >&2
	exit 1
fi
AGENT_USER="${SUDO_USER:?run via sudo from your own account}"

SVC=_mcptg
SVC_ID=451
PREFIX=/opt/mcp-telegram
SRC="$(cd "$(dirname "$0")/.." && pwd)"

# Check every source before changing the system, so a failed run leaves
# nothing half-installed.
if [ ! -x "$SRC/build/mcp-telegram" ]; then
	echo "missing $SRC/build/mcp-telegram: build it first" >&2
	exit 1
fi
CONFIG_SRC="$SRC/deploy/config.yaml"
if [ ! -f "$CONFIG_SRC" ]; then
	CONFIG_SRC="$SRC/deploy/config.example.yaml"
fi
if [ ! -f "$PREFIX/etc/config.yaml" ] && [ ! -f "$CONFIG_SRC" ]; then
	echo "missing $SRC/deploy/config.example.yaml" >&2
	exit 1
fi

# fail_if_taken <dir> <attr>: exit if a record other than $SVC in /<dir>
# already has <attr> $SVC_ID.
fail_if_taken() {
	for owner in $(dscl . -search "/$1" "$2" "$SVC_ID" 2>/dev/null | awk '/=/ {print $1}'); do
		if [ "$owner" != "$SVC" ]; then
			echo "$2 $SVC_ID is already used by $1/$owner: pick a free SVC_ID in $0" >&2
			exit 1
		fi
	done
}
fail_if_taken Groups PrimaryGroupID
fail_if_taken Users UniqueID

# Check attributes, not just the record: an interrupted run may leave a record
# without its ID, shell or home. For the user, IsHidden is set last, so its
# presence means the whole block ran. dscl -create is idempotent.
if ! dscl . -read "/Groups/$SVC" PrimaryGroupID >/dev/null 2>&1; then
	dscl . -create "/Groups/$SVC"
	dscl . -create "/Groups/$SVC" PrimaryGroupID "$SVC_ID"
	dscl . -create "/Groups/$SVC" RealName "mcp-telegram service"
	dscl . -create "/Groups/$SVC" Password '*'
fi
if ! dscl . -read "/Users/$SVC" UniqueID >/dev/null 2>&1 ||
	! dscl . -read "/Users/$SVC" IsHidden >/dev/null 2>&1; then
	dscl . -create "/Users/$SVC"
	dscl . -create "/Users/$SVC" UniqueID "$SVC_ID"
	dscl . -create "/Users/$SVC" PrimaryGroupID "$SVC_ID"
	dscl . -create "/Users/$SVC" UserShell /usr/bin/false
	dscl . -create "/Users/$SVC" NFSHomeDirectory /var/empty
	dscl . -create "/Users/$SVC" RealName "mcp-telegram service"
	dscl . -create "/Users/$SVC" Password '*'
	dscl . -create "/Users/$SVC" IsHidden 1
fi

install -d -o root -g wheel -m 0755 "$PREFIX" "$PREFIX/bin" "$PREFIX/etc"
install -d -o "$SVC" -g "$SVC" -m 0700 "$PREFIX/state"

install -o root -g wheel -m 0755 "$SRC/build/mcp-telegram" "$PREFIX/bin/mcp-telegram"
install -o root -g wheel -m 0755 "$SRC/deploy/run" "$PREFIX/bin/run"
if [ ! -f "$PREFIX/etc/config.yaml" ]; then
	install -o root -g wheel -m 0644 "$CONFIG_SRC" "$PREFIX/etc/config.yaml"
	echo "Installed $PREFIX/etc/config.yaml from $CONFIG_SRC"
fi

if [ ! -f "$PREFIX/etc/secrets.env" ]; then
	printf 'TG_APP_ID (from my.telegram.org/apps): '
	read -r app_id
	printf 'TG_API_HASH (input hidden): '
	trap 'stty echo' EXIT INT TERM
	stty -echo
	read -r api_hash
	stty echo
	echo
	case "$app_id" in '' | *[!0-9]*)
		echo "TG_APP_ID must be a number" >&2
		exit 1
		;;
	esac
	case "$api_hash" in '' | *[!0-9a-fA-F]*)
		echo "TG_API_HASH must be a hex string" >&2
		exit 1
		;;
	esac
	(umask 077 && : >"$PREFIX/etc/secrets.env")
	chown "root:$SVC" "$PREFIX/etc/secrets.env"
	chmod 0640 "$PREFIX/etc/secrets.env"
	printf 'TG_APP_ID=%s\nTG_API_HASH=%s\n' "$app_id" "$api_hash" >"$PREFIX/etc/secrets.env"
	unset app_id api_hash
fi

tmp="$(mktemp)"
cat >"$tmp" <<SUDOERS
# mcp-telegram: the MCP client may start only the server, only as $SVC.
Defaults!$PREFIX/bin/run !use_pty, !log_input, !log_output
$AGENT_USER ALL=($SVC) NOPASSWD: $PREFIX/bin/run serve
SUDOERS
visudo -cf "$tmp"
install -o root -g wheel -m 0440 "$tmp" /etc/sudoers.d/mcp-telegram
rm -f "$tmp"

echo "Installed. Next: sudo -u $SVC $PREFIX/bin/run auth"
echo "Then list chats with: sudo -u $SVC $PREFIX/bin/run dialogs"
echo "and set the ACL in $PREFIX/etc/config.yaml (sudo)."
