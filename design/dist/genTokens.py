#!/usr/bin/env python3
"""从 design/tokens.json 生成 design/dist/tokens.css（唯一生成入口）。

用法: python3 design/dist/genTokens.py [--check]
契约: ui-composition K1（tier/type/value/ref/theme）；CSS 变量名 = 语义令牌名把点换成连字符，
primitive 原始色阶不落到 CSS，只在令牌层内部被 ref 引用。
"""

import json
import os
import sys

TOKENS_PATH = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "tokens.json")
CSS_PATH = os.path.join(os.path.dirname(os.path.abspath(__file__)), "tokens.css")
HEADER = "/* 由 design/tokens.json 经 design/dist/genTokens.py 生成，勿手改；重建: python3 design/dist/genTokens.py */"


def resolveValue(tokens, name, theme, stack=()):
    """解析令牌在指定主题下的最终 CSS 值；ref 链带环检测。"""
    if name in stack:
        raise SystemExit("tokens.json 存在 ref 环: %s" % " -> ".join(stack + (name,)))
    entry = tokens[name]
    override = (entry.get("theme") or {}).get(theme)
    if isinstance(override, dict) and ("ref" in override or "value" in override):
        ref, value = override.get("ref"), override.get("value")
    else:
        ref, value = entry.get("ref"), entry.get("value")
    if ref:
        return resolveValue(tokens, ref, theme, stack + (name,))
    if value is None or value == "":
        raise SystemExit("令牌 %s 缺 value/ref" % name)
    return str(value)


def renderCss(doc):
    """渲染 tokens.css：:root 为默认主题，其余主题用 [data-theme] 覆盖。"""
    tokens = doc["tokens"]
    semantic = [name for name, entry in tokens.items() if entry.get("tier") == "semantic"]
    defaultTheme = doc["defaultTheme"]
    lines = [HEADER, ":root {"]
    for name in semantic:
        lines.append("  --%s: %s;" % (name.replace(".", "-"), resolveValue(tokens, name, defaultTheme)))
    lines.append("}")
    for theme in doc["themes"]:
        if theme == defaultTheme:
            continue
        overrides = [n for n in semantic if theme in (tokens[n].get("theme") or {})]
        if not overrides:
            continue
        lines.append("")
        lines.append('[data-theme="%s"] {' % theme)
        for name in overrides:
            lines.append("  --%s: %s;" % (name.replace(".", "-"), resolveValue(tokens, name, theme)))
        lines.append("}")
    return "\n".join(lines) + "\n"


def main():
    """写入 tokens.css；--check 只比较不落盘，供 make design 复现校验。"""
    with open(TOKENS_PATH, encoding="utf-8") as handle:
        doc = json.load(handle)
    css = renderCss(doc)
    if "--check" in sys.argv[1:]:
        with open(CSS_PATH, encoding="utf-8") as handle:
            current = handle.read()
        if current != css:
            print("tokens.css 与 tokens.json 不同步，请重跑 design/dist/genTokens.py", file=sys.stderr)
            return 1
        print("tokens.css 与 tokens.json 同步")
        return 0
    with open(CSS_PATH, "w", encoding="utf-8") as handle:
        handle.write(css)
    print("wrote %s (%d bytes)" % (CSS_PATH, len(css.encode("utf-8"))))
    return 0


if __name__ == "__main__":
    sys.exit(main())
