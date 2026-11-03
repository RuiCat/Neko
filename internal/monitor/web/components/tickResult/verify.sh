#!/usr/bin/env bash
# Tick.Result 元件独立验收：调用共享契约验收器（读取 design/components.json 真实契约）。
set -euo pipefail
componentDir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)"
projectRoot="$(CDPATH= cd -- "$componentDir/../../../../.." && pwd -P)"
command -v node >/dev/null 2>&1 || { echo "verify(Tick.Result): 需要 node 运行验收器" >&2; exit 2; }
exec node "$projectRoot/internal/monitor/web/components/test/verifyComponent.mjs" "Tick.Result"
