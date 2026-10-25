#!/usr/bin/env bash
# verifyProfile.sh — two independent checks for the dsh-astrbox bundle.
#
#   --composition   Compose the generated bundle patch into an isolated DSH_HOME
#                   under .local/profile-test and assert the two insert entries.
#                   Never reads or writes the real profile, never calls a model.
#   --evidence FILE Validate the minimal evidence file a real install session
#                   must produce (K4). Prints MISSING: <field> and exits 1 when a
#                   field is absent.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CLI="${DSH_CLI:-/home/ruicat/Desktop/deepseek-harness/apps/cli/lib/bin.js}"
SOURCE_PROFILE="${DSH_SOURCE_PROFILE:-$HOME/.dsh/profiles/web}"

# K6 frozen tool names (lowercase).
FROZEN_TOOLS=(
	diagnostics_query entity_create entity_query entity_snapshot
	entity_state_get entity_state_set event_log inventory
	neko_script_apply neko_script_list neko_script_read
	neko_script_remove neko_script_validate neko_script_write
	state_machine_resolve state_machine_tick stats
)

usage() {
	cat <<'EOF'
用法:
  verifyProfile.sh --composition
  verifyProfile.sh --evidence <file>

  --composition  在 .local/profile-test 重建隔离 DSH_HOME，用 --patch 注入
                 dsh/cordis.patch.yml 并运行 --dump-config；断言退出 0、
                 mcp-astrbox 与 astrbox-bridge 各 1 行、command 可执行。
  --evidence     校验真装证据文件的最小字段集（K4）；缺字段打印 MISSING 并非 0。
环境变量:
  DSH_CLI              DSH CLI 入口（默认 /home/ruicat/Desktop/deepseek-harness/apps/cli/lib/bin.js）
  DSH_SOURCE_PROFILE   只读复制来源（默认 ~/.dsh/profiles/web）
EOF
}

# fail prints one reason and exits non-zero.
fail() {
	printf 'verifyProfile: %s\n' "$1" >&2
	exit 1
}

# runComposition builds the isolated profile and composes the patch layers.
runComposition() {
	local patch="$ROOT/dsh/cordis.patch.yml"
	if [ ! -f "$patch" ]; then
		printf 'verifyProfile: 生成缺失的 %s\n' "$patch"
		REPO_ROOT="$ROOT" bash "$ROOT/scripts/renderDshPatch.sh" || fail "renderDshPatch.sh 失败"
	fi
	[ -f "$patch" ] || fail "生成 patch 不存在: $patch"
	[ -f "$CLI" ] || fail "DSH CLI 不存在: $CLI（可用 DSH_CLI 覆盖）"
	[ -d "$SOURCE_PROFILE" ] || fail "源 profile 不存在: $SOURCE_PROFILE"

	local home="$ROOT/.local/profile-test"
	local web="$home/profiles/web"
	# Only the isolated copy under .local/ is ever rewritten.
	rm -rf "$home/profiles"
	mkdir -p "$web"
	local file
	for file in package.json cordis.patch.yml cordis.yml pnpm-workspace.yaml; do
		[ -f "$SOURCE_PROFILE/$file" ] || fail "源 profile 缺少 $file"
		cp "$SOURCE_PROFILE/$file" "$web/$file"
	done
	ln -s "$SOURCE_PROFILE/node_modules" "$web/node_modules"

	local dump="$home/dump.txt"
	local err="$home/dump.err"
	local status=0
	DSH_HOME="$home" node "$CLI" --profile web --patch "$patch" --dump-config \
		>"$dump" 2>"$err" || status=$?
	if [ "$status" -ne 0 ]; then
		cat "$err" >&2
		fail "--dump-config 退出码 $status（dump.err 见 $err）"
	fi

	local mcpCount bridgeCount
	mcpCount="$(grep -c 'id: mcp-astrbox' "$dump" || true)"
	bridgeCount="$(grep -c 'id: astrbox-bridge' "$dump" || true)"
	[ "$mcpCount" -eq 1 ] || fail "dump 中 'id: mcp-astrbox' = $mcpCount, 期望 1"
	[ "$bridgeCount" -eq 1 ] || fail "dump 中 'id: astrbox-bridge' = $bridgeCount, 期望 1"

	local commandPath
	commandPath="$(awk '
		/^[[:space:]]*-[[:space:]]*id:[[:space:]]*mcp-astrbox[[:space:]]*$/ { inBlock = 1; next }
		inBlock && /^[[:space:]]*-[[:space:]]*id:/ { exit }
		inBlock && /^[[:space:]]*command:/ {
			sub(/^[[:space:]]*command:[[:space:]]*/, "")
			sub(/[[:space:]]*$/, "")
			print
			exit
		}
	' "$dump")"
	commandPath="${commandPath#[\"\']}"
	commandPath="${commandPath%[\"\']}"
	[ -n "$commandPath" ] || fail "dump 中缺少 command 行"
	[ -x "$commandPath" ] || fail "command 不是可执行文件: $commandPath"

	printf 'OK: composition checks passed (mcp-astrbox=1, astrbox-bridge=1, command=%s, dump=%s)\n' \
		"$commandPath" "$dump"
}

# runEvidence validates the K4 minimal field set of a real-install evidence file.
runEvidence() {
	local file="${1:-}"
	[ -n "$file" ] || {
		usage >&2
		exit 2
	}
	if [ ! -r "$file" ]; then
		printf 'MISSING: evidence file %s（不存在或不可读）\n' "$file"
		exit 1
	fi
	local missing=0
	local checks=0

	# require runs one field check and records a MISSING line on failure.
	require() {
		local label="$1"
		shift
		checks=$((checks + 1))
		if "$@" >/dev/null 2>&1; then
			return 0
		fi
		printf 'MISSING: %s\n' "$label"
		missing=1
	}
	hasFixed() { grep -Fq -- "$1" "$file"; }
	hasRegex() { grep -Eq -- "$1" "$file"; }
	hasSocketMode() {
		awk '/dsh-control\.sock/ && /(^|[^0-9])600([^0-9]|$)/ { found = 1 } END { exit(found ? 0 : 1) }' "$file"
	}

	# ① composed tree: both insert ids.
	require 'dump-config id: mcp-astrbox' hasFixed 'id: mcp-astrbox'
	require 'dump-config id: astrbox-bridge' hasFixed 'id: astrbox-bridge'
	# ② control socket is 0600.
	require 'socket 0600 (stat line with dsh-control.sock)' hasSocketMode
	# ③ control log shows both request kinds.
	require 'dsh-control log POST /tick' hasFixed 'POST /tick'
	require 'dsh-control log POST /session-event' hasFixed 'POST /session-event'
	# ④ tools/list carries the frozen 17 names.
	local name
	for name in "${FROZEN_TOOLS[@]}"; do
		require "tools/list name: $name" hasFixed "$name"
	done
	# ⑤ live session injected a snapshot.
	require 'session snapshot [astrbox]' hasFixed '[astrbox]'

	if [ "$missing" -ne 0 ]; then
		exit 1
	fi
	printf 'OK: %s checks\n' "$checks"
}

case "${1:-}" in
--composition)
	[ "$#" -eq 1 ] || {
		usage >&2
		exit 2
	}
	runComposition
	;;
--evidence)
	runEvidence "${2:-}"
	;;
-h | --help)
	usage
	;;
*)
	usage >&2
	exit 2
	;;
esac
