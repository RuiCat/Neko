#!/usr/bin/env bash
# Render dsh/cordis.patch.yml from dsh/cordis.patch.yml.template.
#
# The template is read-only input: this script never rewrites it. Only the
# YAML lines are substituted; the template's leading comments are kept verbatim
# so the rendered file still documents the placeholders.
set -euo pipefail

REPO_ROOT="${REPO_ROOT:-$PWD}"
DATA_DIR="${DATA_DIR:-$REPO_ROOT/data}"
SCRIPTS_DIR="${SCRIPTS_DIR:-$REPO_ROOT/scripts}"
CONFIG="${CONFIG:-$REPO_ROOT/config.json}"

TEMPLATE="$REPO_ROOT/dsh/cordis.patch.yml.template"
OUTPUT="$REPO_ROOT/dsh/cordis.patch.yml"

fail() {
	printf 'renderDshPatch: %s\n' "$1" >&2
	exit 1
}

[ -f "$TEMPLATE" ] || fail "模板不存在: $TEMPLATE"
[ -d "$REPO_ROOT/dsh" ] || fail "dsh 目录不存在: $REPO_ROOT/dsh"

# Substitute the four frozen placeholders line by line. Bash replacement is
# literal, so paths containing '/', '&' or spaces stay intact. Comment lines and
# blank lines pass through unchanged.
renderPlaceholders() {
	local line
	while IFS= read -r line || [ -n "$line" ]; do
		if [[ "$line" =~ ^[[:space:]]*# ]]; then
			printf '%s\n' "$line"
			continue
		fi
		line="${line//'${REPO_ROOT}'/$REPO_ROOT}"
		line="${line//'${DATA_DIR}'/$DATA_DIR}"
		line="${line//'${SCRIPTS_DIR}'/$SCRIPTS_DIR}"
		line="${line//'${CONFIG}'/$CONFIG}"
		printf '%s\n' "$line"
	done
}

if ! renderPlaceholders <"$TEMPLATE" >"$OUTPUT"; then
	rm -f "$OUTPUT"
	fail "渲染失败: $TEMPLATE"
fi
chmod 600 "$OUTPUT"

# Refuse to ship a half-rendered patch: every non-comment line must be free of
# ${...} placeholders (only the four names above are supported).
if grep -nE '^[^#]*\$\{[A-Za-z_]+\}' "$OUTPUT" >/dev/null 2>&1; then
	printf 'renderDshPatch: 渲染后仍有未替换占位符:\n' >&2
	grep -nE '^[^#]*\$\{[A-Za-z_]+\}' "$OUTPUT" >&2
	exit 1
fi

printf 'renderDshPatch: %s -> %s\n' "$TEMPLATE" "$OUTPUT"
