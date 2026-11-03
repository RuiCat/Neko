"use strict";
(() => {
  var __defProp = Object.defineProperty;
  var __name = (target, value) => __defProp(target, "name", { value, configurable: true });

  // src/styles/transforms/variable-resolver.ts
  var DOLLAR_VAR_RE = /\$([a-zA-Z_][\w-]*)/g;
  var VAR_FN_RE = /var\((--[\w-]+)(?:,\s*([^)]+))?\)/g;
  function isPlainObject(value) {
    return value !== null && typeof value === "object" && !Array.isArray(value);
  }
  __name(isPlainObject, "isPlainObject");
  function isStyleFunctionCall(value) {
    return value !== null && typeof value === "object" && "__fn" in value && "__args" in value && Array.isArray(value.__args);
  }
  __name(isStyleFunctionCall, "isStyleFunctionCall");
  function resolveValue(value, context) {
    if (typeof value === "string") {
      let resolved = value;
      resolved = resolved.replace(DOLLAR_VAR_RE, (_match, varName) => {
        const key = "$" + varName;
        if (key in context.variables) {
          return context.variables[key];
        }
        context.warnings.push(`Variable "${key}" is not defined`);
        return _match;
      });
      resolved = resolved.replace(VAR_FN_RE, (_match, varName, fallback) => {
        if (varName in context.variables) {
          return context.variables[varName];
        }
        if (fallback !== void 0) {
          return fallback.trim();
        }
        context.warnings.push(`CSS variable "${varName}" is not defined`);
        return _match;
      });
      return resolved;
    }
    if (typeof value === "number") {
      return value;
    }
    if (isStyleFunctionCall(value)) {
      const resolvedArgs = value.__args.map((arg) => {
        if (typeof arg === "string") {
          let s = arg;
          s = s.replace(DOLLAR_VAR_RE, (_match, varName) => {
            const key = "$" + varName;
            if (key in context.variables) {
              return context.variables[key];
            }
            context.warnings.push(`Variable "${key}" is not defined`);
            return _match;
          });
          s = s.replace(VAR_FN_RE, (_match, varName, fallback) => {
            if (varName in context.variables) {
              return context.variables[varName];
            }
            if (fallback !== void 0) {
              return fallback.trim();
            }
            context.warnings.push(`CSS variable "${varName}" is not defined`);
            return _match;
          });
          return s;
        }
        if (typeof arg === "number") {
          return arg;
        }
        if (isStyleFunctionCall(arg)) {
          return resolveValue(arg, context);
        }
        return resolveValue(arg, context);
      });
      return { __fn: value.__fn, __args: resolvedArgs };
    }
    if (isPlainObject(value)) {
      return variableResolver(value, context);
    }
    return value;
  }
  __name(resolveValue, "resolveValue");
  function variableResolver(input, context) {
    const result = {};
    for (const key of Object.keys(input)) {
      const value = input[key];
      if (value === void 0 || value === null) {
        continue;
      }
      result[key] = resolveValue(value, context);
    }
    return result;
  }
  __name(variableResolver, "variableResolver");

  // src/styles/color-names.ts
  var NAMED_COLORS = {
    red: "#ff0000",
    green: "#008000",
    blue: "#0000ff",
    white: "#ffffff",
    black: "#000000",
    transparent: "#00000000",
    gray: "#808080",
    grey: "#808080",
    orange: "#ffa500",
    yellow: "#ffff00",
    purple: "#800080",
    pink: "#ffc0cb",
    cyan: "#00ffff",
    magenta: "#ff00ff",
    lime: "#00ff00",
    teal: "#008080",
    navy: "#000080",
    maroon: "#800000",
    olive: "#808000",
    silver: "#c0c0c0",
    aqua: "#00ffff",
    fuchsia: "#ff00ff"
  };
  var CSS_COLOR_KEYWORDS = /* @__PURE__ */ new Set([
    "transparent",
    "currentcolor",
    "black",
    "silver",
    "gray",
    "grey",
    "white",
    "maroon",
    "red",
    "purple",
    "fuchsia",
    "magenta",
    "green",
    "lime",
    "olive",
    "yellow",
    "navy",
    "blue",
    "teal",
    "aqua",
    "cyan",
    "orange",
    "pink",
    "brown",
    "gold",
    "indigo",
    "violet",
    "beige",
    "ivory",
    "khaki",
    "coral",
    "salmon",
    "crimson",
    "turquoise",
    "lavender",
    "plum",
    "orchid",
    "tan",
    "wheat",
    "azure",
    "mintcream",
    "whitesmoke",
    "gainsboro",
    "lightgray",
    "lightgrey",
    "darkgray",
    "darkgrey",
    "dimgray",
    "dimgrey",
    "slategray",
    "slategrey"
  ]);

  // src/styles/color-utils.ts
  function clamp(value, min, max) {
    if (value < min) return min;
    if (value > max) return max;
    return value;
  }
  __name(clamp, "clamp");
  function srgbToLinear(c) {
    if (c <= 0.04045) {
      return c / 12.92;
    }
    return Math.pow((c + 0.055) / 1.055, 2.4);
  }
  __name(srgbToLinear, "srgbToLinear");
  function hslToRgbChannel(h2, s, l, n) {
    const a = s * Math.min(l, 1 - l);
    const k = (n + h2 / 30) % 12;
    return l - a * Math.max(-1, Math.min(k - 3, 9 - k, 1));
  }
  __name(hslToRgbChannel, "hslToRgbChannel");
  var _Color = class _Color {
    /**
     * @param r - Red channel (0-255)
     * @param g - Green channel (0-255)
     * @param b - Blue channel (0-255)
     * @param a - Alpha channel (0-1), defaults to 1
     */
    constructor(r, g, b, a = 1) {
      this.r = clamp(Math.round(r), 0, 255);
      this.g = clamp(Math.round(g), 0, 255);
      this.b = clamp(Math.round(b), 0, 255);
      this.a = clamp(a, 0, 1);
    }
    /**
     * 将 CSS 颜色字符串解析为 Color 对象。
     * 支持："#RGB"、"#RRGGBB"、"#RRGGBBAA"、"rgb(r,g,b)"、"rgba(r,g,b,a)"、
     *        "hsl(h,s%,l%)"、"hsla(h,s%,l%,a)"、CSS 命名颜色。
     * @throws 如果输入无法解析则抛出错误
     */
    static parse(input) {
      if (!input || typeof input !== "string") {
        throw new Error(`Color.parse: invalid input "${input}"`);
      }
      const trimmed = input.trim().toLowerCase();
      if (NAMED_COLORS[trimmed] !== void 0) {
        return _Color.parse(NAMED_COLORS[trimmed]);
      }
      if (trimmed.startsWith("#")) {
        return _Color._parseHex(trimmed);
      }
      const rgbMatch = trimmed.match(
        /^rgba?\s*\(\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)\s*(?:,\s*([\d.]+))?\s*\)$/
      );
      if (rgbMatch) {
        return new _Color(
          parseInt(rgbMatch[1], 10),
          parseInt(rgbMatch[2], 10),
          parseInt(rgbMatch[3], 10),
          rgbMatch[4] !== void 0 ? parseFloat(rgbMatch[4]) : 1
        );
      }
      const hslMatch = trimmed.match(
        /^hsla?\s*\(\s*([\d.]+)\s*,\s*([\d.]+)%\s*,\s*([\d.]+)%\s*(?:,\s*([\d.]+))?\s*\)$/
      );
      if (hslMatch) {
        const h2 = parseFloat(hslMatch[1]) / 360;
        const s = parseFloat(hslMatch[2]) / 100;
        const l = parseFloat(hslMatch[3]) / 100;
        const a = hslMatch[4] !== void 0 ? parseFloat(hslMatch[4]) : 1;
        return _Color._fromHslRaw(h2, s, l, a);
      }
      throw new Error(`Color.parse: unable to parse "${input}"`);
    }
    /** 解析十六进制颜色字符串（内部方法） */
    static _parseHex(hex) {
      let h2 = hex.slice(1);
      if (h2.length === 3) {
        h2 = h2[0] + h2[0] + h2[1] + h2[1] + h2[2] + h2[2];
      }
      if (h2.length === 4) {
        h2 = h2[0] + h2[0] + h2[1] + h2[1] + h2[2] + h2[2] + h2[3] + h2[3];
      }
      if (h2.length === 6) {
        return new _Color(
          parseInt(h2.slice(0, 2), 16),
          parseInt(h2.slice(2, 4), 16),
          parseInt(h2.slice(4, 6), 16),
          1
        );
      }
      if (h2.length === 8) {
        return new _Color(
          parseInt(h2.slice(0, 2), 16),
          parseInt(h2.slice(2, 4), 16),
          parseInt(h2.slice(4, 6), 16),
          parseInt(h2.slice(6, 8), 16) / 255
        );
      }
      throw new Error(`Color._parseHex: invalid hex "${hex}"`);
    }
    /** 从原始 HSL 值（0-1）创建 Color */
    static _fromHslRaw(h2, s, l, a) {
      const r = hslToRgbChannel(h2 * 360, s, l, 3);
      const g = hslToRgbChannel(h2 * 360, s, l, 9);
      const b = hslToRgbChannel(h2 * 360, s, l, 5);
      return new _Color(r * 255, g * 255, b * 255, a);
    }
    /** 输出为 "#rrggbb" 十六进制字符串 */
    toHex() {
      const rh = this.r.toString(16).padStart(2, "0");
      const gh = this.g.toString(16).padStart(2, "0");
      const bh = this.b.toString(16).padStart(2, "0");
      return `#${rh}${gh}${bh}`;
    }
    /** 输出为 "rgb(r, g, b)" 字符串 */
    toRgb() {
      return `rgb(${this.r}, ${this.g}, ${this.b})`;
    }
    /** 输出为 "rgba(r, g, b, a)" 字符串 */
    toRgba() {
      const alpha2 = Math.round(this.a * 100) / 100;
      return `rgba(${this.r}, ${this.g}, ${this.b}, ${alpha2})`;
    }
    /** 输出为 "hsl(h, s%, l%)" 字符串 */
    toHsl() {
      const [h2, s, l] = this._toHslRaw();
      return `hsl(${h2}, ${s}%, ${l}%)`;
    }
    /** 输出为 "hsla(h, s%, l%, a)" 字符串 */
    toHsla() {
      const [h2, s, l] = this._toHslRaw();
      const alpha2 = Math.round(this.a * 100) / 100;
      return `hsla(${h2}, ${s}%, ${l}%, ${alpha2})`;
    }
    /** 转换为原始 HSL 元组（h: 0-360，s: 0-100，l: 0-100） */
    _toHslRaw() {
      const r = this.r / 255;
      const g = this.g / 255;
      const b = this.b / 255;
      const max = Math.max(r, g, b);
      const min = Math.min(r, g, b);
      const delta = max - min;
      let h2 = 0;
      let s = 0;
      const l = (max + min) / 2;
      if (delta !== 0) {
        s = l > 0.5 ? delta / (2 - max - min) : delta / (max + min);
        if (max === r) {
          h2 = ((g - b) / delta + (g < b ? 6 : 0)) / 6;
        } else if (max === g) {
          h2 = ((b - r) / delta + 2) / 6;
        } else {
          h2 = ((r - g) / delta + 4) / 6;
        }
      }
      return [Math.round(h2 * 360), Math.round(s * 100), Math.round(l * 100)];
    }
    /**
     * 返回按给定量（0-100）变暗后的新 Color 实例。
     * 在 HSL 空间中降低亮度。
     */
    darken(amount) {
      const [h2, s, l] = this._toHslRaw();
      const newL = clamp(l - amount, 0, 100);
      return _Color._fromHslRaw(h2 / 360, s / 100, newL / 100, this.a);
    }
    /**
     * 返回按给定量（0-100）变亮后的新 Color 实例。
     * 在 HSL 空间中提高亮度。
     */
    lighten(amount) {
      const [h2, s, l] = this._toHslRaw();
      const newL = clamp(l + amount, 0, 100);
      return _Color._fromHslRaw(h2 / 360, s / 100, newL / 100, this.a);
    }
    /**
     * 返回按给定量（0-100）提高饱和度后的新 Color 实例。
     */
    saturate(amount) {
      const [h2, s, l] = this._toHslRaw();
      const newS = clamp(s + amount, 0, 100);
      return _Color._fromHslRaw(h2 / 360, newS / 100, l / 100, this.a);
    }
    /**
     * 返回按给定量（0-100）降低饱和度后的新 Color 实例。
     */
    desaturate(amount) {
      const [h2, s, l] = this._toHslRaw();
      const newS = clamp(s - amount, 0, 100);
      return _Color._fromHslRaw(h2 / 360, newS / 100, l / 100, this.a);
    }
    /**
     * 按给定权重（0-1，默认 0.5）将此颜色与另一个 Color 混合。
     * 对 RGBA 通道进行线性插值。
     */
    mix(other, weight = 0.5) {
      const w = clamp(weight, 0, 1);
      const iw = 1 - w;
      return new _Color(
        this.r * iw + other.r * w,
        this.g * iw + other.g * w,
        this.b * iw + other.b * w,
        this.a * iw + other.a * w
      );
    }
    /**
     * 返回将透明度通道设为给定值（0-1）后的新 Color 实例。
     */
    alpha(a) {
      return new _Color(this.r, this.g, this.b, a);
    }
    /**
     * 按照 WCAG 2.0 计算相对亮度（0-1）。
     * 使用 sRGB 线性化方法。
     */
    luminance() {
      const rLin = srgbToLinear(this.r / 255);
      const gLin = srgbToLinear(this.g / 255);
      const bLin = srgbToLinear(this.b / 255);
      return 0.2126 * rLin + 0.7152 * gLin + 0.0722 * bLin;
    }
    /**
     * 根据此颜色的对比度返回黑色或白色。
     * 如果亮度高于 0.5，返回黑色；否则返回白色。
     */
    contrastColor() {
      return this.luminance() > 0.5 ? new _Color(0, 0, 0) : new _Color(255, 255, 255);
    }
    /** 返回此 Color 的深拷贝 */
    clone() {
      return new _Color(this.r, this.g, this.b, this.a);
    }
    /** 比较此 Color 与另一个 Color 是否相等（每个通道容差 ±1） */
    equals(other) {
      const tol = 1;
      return Math.abs(this.r - other.r) <= tol && Math.abs(this.g - other.g) <= tol && Math.abs(this.b - other.b) <= tol && Math.abs(this.a - other.a) <= 0.01;
    }
  };
  __name(_Color, "Color");
  var Color = _Color;
  function darken(color, amount) {
    return Color.parse(color).darken(amount).toHex();
  }
  __name(darken, "darken");
  function lighten(color, amount) {
    return Color.parse(color).lighten(amount).toHex();
  }
  __name(lighten, "lighten");
  function rgba(r, g, b, a) {
    return new Color(r, g, b, a).toRgba();
  }
  __name(rgba, "rgba");
  function hsla(h2, s, l, a) {
    return Color._fromHslRaw(h2 / 360, s / 100, l / 100, a).toHsla();
  }
  __name(hsla, "hsla");
  function mix(color1, color2, weight = 0.5) {
    return Color.parse(color2).mix(Color.parse(color1), weight).toHex();
  }
  __name(mix, "mix");
  function alpha(color) {
    return Color.parse(color).a;
  }
  __name(alpha, "alpha");

  // src/styles/unit-helpers.ts
  function px(n) {
    return `${n}px`;
  }
  __name(px, "px");
  function rem(n) {
    return `${n}rem`;
  }
  __name(rem, "rem");
  function em(n) {
    return `${n}em`;
  }
  __name(em, "em");
  function percent(n) {
    return `${n}%`;
  }
  __name(percent, "percent");
  function vh(n) {
    return `${n}vh`;
  }
  __name(vh, "vh");
  function vw(n) {
    return `${n}vw`;
  }
  __name(vw, "vw");
  function deg(n) {
    return `${n}deg`;
  }
  __name(deg, "deg");
  function calc(expr) {
    return `calc(${expr})`;
  }
  __name(calc, "calc");

  // src/styles/transforms/function-evaluator.ts
  var functionRegistry = /* @__PURE__ */ new Map();
  function registerFunction(name, fn) {
    functionRegistry.set(name, fn);
  }
  __name(registerFunction, "registerFunction");
  registerFunction("darken", (args, _context) => {
    if (args.length < 2) return "";
    return darken(String(args[0]), Number(args[1]));
  });
  registerFunction("lighten", (args, _context) => {
    if (args.length < 2) return "";
    return lighten(String(args[0]), Number(args[1]));
  });
  registerFunction("rgba", (args, _context) => {
    if (args.length < 3) return "rgba(0,0,0,1)";
    return rgba(Number(args[0]), Number(args[1]), Number(args[2]), args.length >= 4 ? Number(args[3]) : 1);
  });
  registerFunction("hsla", (args, _context) => {
    if (args.length < 3) return "hsla(0,0%,0%,1)";
    return hsla(Number(args[0]), Number(args[1]), Number(args[2]), args.length >= 4 ? Number(args[3]) : 1);
  });
  registerFunction("mix", (args, _context) => {
    if (args.length < 2) return "";
    return mix(String(args[0]), String(args[1]), args.length >= 3 ? Number(args[2]) : 0.5);
  });
  registerFunction("alpha", (args, _context) => {
    if (args.length < 1) return 1;
    return alpha(String(args[0]));
  });
  registerFunction("saturate", (args, _context) => {
    if (args.length < 2) return "";
    return Color.parse(String(args[0])).saturate(Number(args[1])).toHex();
  });
  registerFunction("desaturate", (args, _context) => {
    if (args.length < 2) return "";
    return Color.parse(String(args[0])).desaturate(Number(args[1])).toHex();
  });
  registerFunction("invertColor", (args, _context) => {
    if (args.length < 1) return "";
    const c = Color.parse(String(args[0]));
    return new Color(255 - c.r, 255 - c.g, 255 - c.b, c.a).toHex();
  });
  registerFunction("contrastColor", (args, _context) => {
    if (args.length < 1) return "";
    return Color.parse(String(args[0])).contrastColor().toHex();
  });
  function toNumber(v) {
    if (typeof v === "number") return v;
    if (typeof v === "string") {
      const n = parseFloat(v);
      return isNaN(n) ? 0 : n;
    }
    return 0;
  }
  __name(toNumber, "toNumber");
  registerFunction("add", (args, _context) => {
    const nums = args.map(toNumber);
    return nums.reduce((sum, a) => sum + a, 0);
  });
  registerFunction("sub", (args, _context) => {
    if (args.length === 0) return 0;
    const nums = args.map(toNumber);
    return nums.slice(1).reduce((acc, a) => acc - a, nums[0]);
  });
  registerFunction("multiply", (args, _context) => {
    const nums = args.map(toNumber);
    return nums.reduce((prod, a) => prod * a, 1);
  });
  registerFunction("div", (args, _context) => {
    if (args.length === 0) return 0;
    const nums = args.map(toNumber);
    return nums.slice(1).reduce((acc, a) => {
      return a === 0 ? acc : acc / a;
    }, nums[0]);
  });
  registerFunction("clamp", (args, _context) => {
    if (args.length < 3) return args.length > 0 ? toNumber(args[0]) : 0;
    const val = toNumber(args[0]);
    const min = toNumber(args[1]);
    const max = toNumber(args[2]);
    if (val < min) return min;
    if (val > max) return max;
    return val;
  });
  registerFunction("round", (args, _context) => {
    if (args.length === 0) return 0;
    const val = toNumber(args[0]);
    const decimals = args.length >= 2 ? Math.round(toNumber(args[1])) : 0;
    if (decimals <= 0) return Math.round(val);
    const factor = Math.pow(10, decimals);
    return Math.round(val * factor) / factor;
  });
  registerFunction("abs", (args, _context) => {
    if (args.length === 0) return 0;
    return Math.abs(toNumber(args[0]));
  });
  registerFunction("px", (args, _context) => {
    if (args.length === 0) return "0px";
    return px(toNumber(args[0]));
  });
  registerFunction("rem", (args, _context) => {
    if (args.length === 0) return "0rem";
    return rem(toNumber(args[0]));
  });
  registerFunction("em", (args, _context) => {
    if (args.length === 0) return "0em";
    return em(toNumber(args[0]));
  });
  registerFunction("percent", (args, _context) => {
    if (args.length === 0) return "0%";
    return percent(toNumber(args[0]));
  });
  registerFunction("vh", (args, _context) => {
    if (args.length === 0) return "0vh";
    return vh(toNumber(args[0]));
  });
  registerFunction("vw", (args, _context) => {
    if (args.length === 0) return "0vw";
    return vw(toNumber(args[0]));
  });
  registerFunction("deg", (args, _context) => {
    if (args.length === 0) return "0deg";
    return deg(toNumber(args[0]));
  });
  registerFunction("calc", (args, _context) => {
    if (args.length === 0) return "calc(0)";
    return calc(String(args[0]));
  });
  function parseStringFn(expr) {
    const trimmed = expr.trim();
    const match = trimmed.match(/^([a-zA-Z_]\w*)\s*\((.+)\)$/s);
    if (!match) return null;
    const name = match[1];
    const body = match[2];
    const args = [];
    let depth = 0;
    let current = "";
    for (let i = 0; i < body.length; i++) {
      const ch = body[i];
      if (ch === "(") {
        depth++;
        current += ch;
      } else if (ch === ")") {
        depth--;
        current += ch;
      } else if (ch === "," && depth === 0) {
        args.push(current.trim());
        current = "";
      } else {
        current += ch;
      }
    }
    if (current.trim().length > 0) {
      args.push(current.trim());
    }
    return { name, args };
  }
  __name(parseStringFn, "parseStringFn");
  function isStyleFunctionCall2(value) {
    return value !== null && typeof value === "object" && "__fn" in value && "__args" in value && Array.isArray(value.__args);
  }
  __name(isStyleFunctionCall2, "isStyleFunctionCall");
  function isPlainObject2(value) {
    return value !== null && typeof value === "object" && !Array.isArray(value) && !isStyleFunctionCall2(value);
  }
  __name(isPlainObject2, "isPlainObject");
  function evaluateValue(value, context) {
    if (value === null || value === void 0) {
      return value;
    }
    if (typeof value === "number") {
      return value;
    }
    if (typeof value === "string") {
      const parsed = parseStringFn(value);
      if (parsed) {
        const impl = functionRegistry.get(parsed.name);
        if (!impl) {
          context.warnings.push(`Function "${parsed.name}" is not registered`);
          return value;
        }
        const evaluatedArgs = parsed.args.map((arg) => {
          const ev = evaluateValue(arg, context);
          if (typeof ev === "string" || typeof ev === "number") {
            return ev;
          }
          return typeof ev === "string" ? ev : String(ev);
        });
        try {
          return impl(evaluatedArgs, context);
        } catch (e) {
          context.warnings.push(`Error evaluating function "${parsed.name}": ${e instanceof Error ? e.message : String(e)}`);
          return value;
        }
      }
      return value;
    }
    if (isStyleFunctionCall2(value)) {
      const impl = functionRegistry.get(value.__fn);
      if (!impl) {
        context.warnings.push(`Function "${value.__fn}" is not registered`);
        return value;
      }
      const evaluatedArgs = value.__args.map((arg) => {
        const ev = evaluateValue(arg, context);
        if (typeof ev === "string" || typeof ev === "number") {
          return ev;
        }
        return typeof ev === "string" ? ev : String(ev);
      });
      try {
        return impl(evaluatedArgs, context);
      } catch (e) {
        context.warnings.push(`Error evaluating function "${value.__fn}": ${e instanceof Error ? e.message : String(e)}`);
        return value;
      }
    }
    if (isPlainObject2(value)) {
      return functionEvaluator(value, context);
    }
    return value;
  }
  __name(evaluateValue, "evaluateValue");
  function functionEvaluator(input, context) {
    const result = {};
    for (const key of Object.keys(input)) {
      const value = input[key];
      if (value === void 0 || value === null) {
        continue;
      }
      const evaluated = evaluateValue(value, context);
      result[key] = evaluated;
    }
    return result;
  }
  __name(functionEvaluator, "functionEvaluator");

  // src/styles/transforms/nesting-expander.ts
  var SELECTOR_LIKE_RE = /^(\.|#|\[|::?|@)/;
  function isPlainObject3(value) {
    return value !== null && typeof value === "object" && !Array.isArray(value);
  }
  __name(isPlainObject3, "isPlainObject");
  function isStyleFunctionCall3(value) {
    return value !== null && typeof value === "object" && "__fn" in value && "__args" in value;
  }
  __name(isStyleFunctionCall3, "isStyleFunctionCall");
  function resolveSelectorKey(key, parentSelector) {
    if (key.startsWith("@")) {
      return key;
    }
    if (key.includes("&")) {
      return key.replace(/&/g, parentSelector);
    }
    if (SELECTOR_LIKE_RE.test(key)) {
      if (key.startsWith(":")) {
        return parentSelector + key;
      }
      return parentSelector + " " + key;
    }
    return key;
  }
  __name(resolveSelectorKey, "resolveSelectorKey");
  function expandNode(input, context) {
    const result = {};
    for (const key of Object.keys(input)) {
      const value = input[key];
      if (value === void 0 || value === null) {
        continue;
      }
      if (key.startsWith("@")) {
        if (isPlainObject3(value)) {
          const expandedChildren = nestingExpander(value, {
            ...context,
            parentSelector: context.parentSelector
            // 保持 at-rule 子元素的父选择器不变
          });
          result[key] = expandedChildren;
        } else {
          result[key] = value;
        }
        continue;
      }
      if (isPlainObject3(value) && !isStyleFunctionCall3(value)) {
        const newSelector = resolveSelectorKey(key, context.parentSelector);
        const childContext = {
          ...context,
          parentSelector: newSelector
        };
        const expandedChildren = nestingExpander(value, childContext);
        result[newSelector] = expandedChildren;
        continue;
      }
      result[key] = value;
    }
    return result;
  }
  __name(expandNode, "expandNode");
  function nestingExpander(input, context) {
    return expandNode(input, context);
  }
  __name(nestingExpander, "nestingExpander");

  // src/styles/transforms/css-generator.ts
  function isStyleFunctionCall4(value) {
    return value !== null && typeof value === "object" && "__fn" in value && "__args" in value;
  }
  __name(isStyleFunctionCall4, "isStyleFunctionCall");
  function isPlainObject4(value) {
    return value !== null && typeof value === "object" && !Array.isArray(value) && !isStyleFunctionCall4(value);
  }
  __name(isPlainObject4, "isPlainObject");
  function toKebabCase(str) {
    return str.replace(/[A-Z]/g, (match) => "-" + match.toLowerCase());
  }
  __name(toKebabCase, "toKebabCase");
  function formatValue(val) {
    if (val === null || val === void 0) {
      return "";
    }
    if (typeof val === "number") {
      return String(val);
    }
    return String(val);
  }
  __name(formatValue, "formatValue");
  function generateRuleBlock(properties) {
    const parts = [];
    for (const key of Object.keys(properties)) {
      const kebab = toKebabCase(key);
      const val = formatValue(properties[key]);
      if (val !== "") {
        parts.push(`${kebab}:${val};`);
      }
    }
    return parts.join("");
  }
  __name(generateRuleBlock, "generateRuleBlock");
  function isAtRule(key) {
    return key.startsWith("@");
  }
  __name(isAtRule, "isAtRule");
  function isSelectorKey(key) {
    return /^(\.|#|\[|::?|[a-z])/.test(key);
  }
  __name(isSelectorKey, "isSelectorKey");
  function generateFromObject(input, context, inline, rules, selector) {
    const directProps = {};
    const nestedEntries = [];
    for (const key of Object.keys(input)) {
      const value = input[key];
      if (value === void 0 || value === null) {
        continue;
      }
      if (isAtRule(key)) {
        if (isPlainObject4(value)) {
          const childRules = [];
          const childInline = {};
          generateFromObject(value, context, childInline, childRules, selector);
          if (childRules.length > 0) {
            rules.push(`${key} { ${childRules.join(" ")} }`);
          }
        }
        continue;
      }
      if (isPlainObject4(value)) {
        nestedEntries.push({ key, value });
        continue;
      }
      if (typeof value === "string" || typeof value === "number") {
        const formatted = formatValue(value);
        if (formatted !== "") {
          directProps[key] = value;
          inline[key] = String(value);
        }
      }
    }
    if (Object.keys(directProps).length > 0) {
      const blockStr = generateRuleBlock(directProps);
      if (selector) {
        rules.push(`${selector} { ${blockStr} }`);
      }
    }
    for (const entry of nestedEntries) {
      if (isSelectorKey(entry.key)) {
        generateFromObject(entry.value, context, inline, rules, entry.key);
      } else {
        generateFromObject(entry.value, context, inline, rules, selector);
      }
    }
  }
  __name(generateFromObject, "generateFromObject");
  function cssGenerator(input, context) {
    const inline = {};
    const rules = [];
    generateFromObject(input, context, inline, rules, context.parentSelector || "");
    const cssParts = [];
    for (const key of Object.keys(inline)) {
      const kebab = toKebabCase(key);
      cssParts.push(`${kebab}:${inline[key]};`);
    }
    const cssText = cssParts.join("");
    return {
      inline,
      cssText,
      rules,
      warnings: context.warnings
    };
  }
  __name(cssGenerator, "cssGenerator");

  // src/styles/StyleProcessor.ts
  var DEFAULT_TRANSFORMS = [
    variableResolver,
    functionEvaluator,
    nestingExpander,
    cssGenerator
  ];
  var variableRegistry = {};
  var customTransforms = [];
  function isStyleFunctionCall5(value) {
    return value !== null && typeof value === "object" && "__fn" in value && "__args" in value;
  }
  __name(isStyleFunctionCall5, "isStyleFunctionCall");
  function isPlainObject5(value) {
    return value !== null && typeof value === "object" && !Array.isArray(value) && !isStyleFunctionCall5(value);
  }
  __name(isPlainObject5, "isPlainObject");
  function isNested(obj) {
    for (const key of Object.keys(obj)) {
      const val = obj[key];
      if (val === null || val === void 0) continue;
      if (isPlainObject5(val)) return true;
      if (isStyleFunctionCall5(val)) return true;
      if (typeof val === "string") {
        if (val.includes("$") || val.includes("var(")) return true;
        if (/^[a-zA-Z_]\w*\s*\(/.test(val.trim())) return true;
      }
    }
    return false;
  }
  __name(isNested, "isNested");
  function buildPipeline() {
    const pipeline = [];
    const prependFns = customTransforms.filter((t) => t.position === "prepend").map((t) => t.fn);
    const appendFns = customTransforms.filter((t) => t.position === "append").map((t) => t.fn);
    return [...prependFns, ...DEFAULT_TRANSFORMS, ...appendFns];
  }
  __name(buildPipeline, "buildPipeline");
  function isStyleProcessResult(obj) {
    return obj !== null && typeof obj === "object" && "inline" in obj;
  }
  __name(isStyleProcessResult, "isStyleProcessResult");
  function loadCSSVariables() {
    const vars = {};
    if (typeof document === "undefined" || !document.styleSheets) {
      return vars;
    }
    try {
      for (let i = 0; i < document.styleSheets.length; i++) {
        const sheet = document.styleSheets[i];
        if (!sheet || !sheet.href) continue;
        if (sheet.href.includes("PetUI.color.css") || sheet.href.includes("PetUI.css")) {
          try {
            const rules = sheet.cssRules || sheet.rules;
            if (!rules) continue;
            for (let j = 0; j < rules.length; j++) {
              const rule = rules[j];
              if (rule instanceof CSSStyleRule && rule.selectorText === ":root") {
                const styleMap = rule.style;
                for (let k = 0; k < styleMap.length; k++) {
                  const prop = styleMap[k];
                  if (prop.startsWith("--")) {
                    const value = styleMap.getPropertyValue(prop).trim();
                    vars[prop] = value;
                    vars["$" + prop.slice(2)] = value;
                  }
                }
              }
              if (rule instanceof CSSStyleRule) {
                const styleMap = rule.style;
                for (let k = 0; k < styleMap.length; k++) {
                  const prop = styleMap[k];
                  if (prop.startsWith("--") && !(prop in vars)) {
                    const value = styleMap.getPropertyValue(prop).trim();
                    vars[prop] = value;
                    vars["$" + prop.slice(2)] = value;
                  }
                }
              }
            }
          } catch (_e) {
            console.debug("[PetUI StyleProcessor] \u8BFB\u53D6\u6837\u5F0F\u8868\u5931\u8D25\uFF08\u8DE8\u57DF\u6216\u5DF2\u5931\u6548\uFF09\uFF0C\u8DF3\u8FC7\u8BE5\u8868:", _e);
          }
        }
      }
    } catch (_e) {
      console.debug("[PetUI StyleProcessor] loadCSSVariables \u603B\u515C\u5E95\u89E6\u53D1\uFF0C\u8FD4\u56DE\u5DF2\u6536\u96C6\u7684\u90E8\u5206\u53D8\u91CF:", _e);
    }
    return vars;
  }
  __name(loadCSSVariables, "loadCSSVariables");
  var StyleProcessor = {
    /**
     * 通过完整管道处理 NestedStyleObject。
     *
     * @param input - 要处理的嵌套样式对象
     * @param options - 配置选项
     * @returns 包含内联样式、cssText、规则和警告的 StyleProcessResult
     */
    process(input, options) {
      const inputObj = input && typeof input === "object" ? input : {};
      const mergedVars = { ...variableRegistry };
      if (options?.variables) {
        for (const key of Object.keys(options.variables)) {
          mergedVars[key] = options.variables[key];
        }
      }
      if (options?.loadCSSVariables) {
        const cssVars = loadCSSVariables();
        for (const key of Object.keys(cssVars)) {
          if (!(key in mergedVars)) {
            mergedVars[key] = cssVars[key];
          }
        }
      }
      const warnings = [];
      const context = {
        variables: mergedVars,
        parentSelector: options?.parentSelector || (options?.resolveNestedSelectors !== false ? "&" : ""),
        warnings
      };
      let current = { ...inputObj };
      const pipeline = buildPipeline();
      for (const transform2 of pipeline) {
        if (isStyleProcessResult(current)) {
          break;
        }
        const transformed = transform2(current, context);
        if (isStyleProcessResult(transformed)) {
          current = transformed;
          break;
        }
        current = transformed;
      }
      if (!isStyleProcessResult(current)) {
        const finalResult = cssGenerator(current, context);
        return {
          ...finalResult,
          warnings: context.warnings
        };
      }
      if (options?.onWarning) {
        for (const w of current.warnings) {
          options.onWarning(w);
        }
      }
      return current;
    },
    /**
     * 注册一个 CSS 变量（支持 "$name" 和 "--name" 两种格式）。
     */
    registerVariable(name, value) {
      if (name.startsWith("$")) {
        variableRegistry[name] = value;
        variableRegistry["--" + name.slice(1)] = value;
      } else if (name.startsWith("--")) {
        variableRegistry[name] = value;
        variableRegistry["$" + name.slice(2)] = value;
      } else {
        variableRegistry["$" + name] = value;
        variableRegistry["--" + name] = value;
      }
    },
    /**
     * 注册一个自定义函数实现。
     * 委托到 function-evaluator 注册表。
     */
    registerFunction(name, fn) {
      registerFunction(name, fn);
    },
    /**
     * 从 PetUI.color.css / PetUI.css 样式表加载 CSS 自定义属性。
     * 返回提取的变量映射表。
     */
    loadCSSVariables() {
      const vars = loadCSSVariables();
      for (const key of Object.keys(vars)) {
        if (!(key in variableRegistry)) {
          variableRegistry[key] = vars[key];
        }
      }
      return vars;
    },
    /**
     * 向管道添加一个转换器。
     * @param fn - 转换器函数
     * @param position - 'prepend'（在默认转换器之前）或 'append'（在默认转换器之后）。默认为 'append'。
     */
    addTransform(fn, position = "append") {
      customTransforms.push({ fn, position });
    },
    /**
     * 通过函数名移除一个转换器。
     * 如果找到并移除了匹配的转换器则返回 true。
     */
    removeTransform(name) {
      const defaultIndex = DEFAULT_TRANSFORMS.findIndex(
        (t) => t.name === name
      );
      if (defaultIndex !== -1) {
        const customIndex2 = customTransforms.findIndex(
          (t) => t.fn.name === name
        );
        if (customIndex2 !== -1) {
          customTransforms.splice(customIndex2, 1);
          return true;
        }
        return false;
      }
      const customIndex = customTransforms.findIndex(
        (t) => t.fn.name === name
      );
      if (customIndex === -1) return false;
      customTransforms.splice(customIndex, 1);
      return true;
    },
    /**
     * 获取当前变量注册表（浅拷贝）。
     */
    getVariableRegistry() {
      return { ...variableRegistry };
    },
    /**
     * 将处理器重置为默认状态。
     * 清除所有已注册的变量和自定义转换器。
     */
    reset() {
      variableRegistry = {};
      customTransforms = [];
    },
    /**
     * 检查对象是否符合 NestedStyleObject
     * （具有嵌套键、函数调用或变量引用）。
     */
    isNested(obj) {
      return isNested(obj);
    }
  };

  // src/core/PetUIExpand.ts
  var PetUIExpand = {
    // 打字效果
    Typing: /* @__PURE__ */ __name(function() {
      const tyFunc = /* @__PURE__ */ __name(function(str, element, callback, interval) {
        const t = interval || 100;
        if (tyFunc.I <= str.length) {
          element.innerText = str.slice(0, tyFunc.I++);
          setTimeout(() => tyFunc(str, element, callback, t), t);
        } else {
          tyFunc.I = 0;
          if (callback != null) callback();
        }
      }, "tyFunc");
      tyFunc.I = 0;
      return tyFunc;
    }, "Typing"),
    // 从数据创建 object URL
    SrcFile: /* @__PURE__ */ __name(function(data) {
      const blob = new Blob([data], { type: "application/octet-binary" });
      const url = URL.createObjectURL(blob);
      url.Revoke = () => URL.revokeObjectURL(url);
      return url;
    }, "SrcFile"),
    // 切分 Blob（兼容旧前缀）
    SliceBlob: /* @__PURE__ */ __name(function(blob, start, end, type) {
      type = type || blob.type;
      if (blob.slice) return blob.slice(start, end, type);
      if (blob.mozSlice) return blob.mozSlice(start, end, type);
      if (blob.webkitSlice) return blob.webkitSlice(start, end, type);
      throw new Error("This doesn't work!");
    }, "SliceBlob"),
    // Web Worker 封装
    DaemonJS: /* @__PURE__ */ __name(function(JsData, CallBack) {
      const script = `var $ = this,$Call = console.log; 

            $.log = $.postMessage;
            $.onmessage = (e) => $Call(e.data,e); 
 
            $.call = (e) => $.postMessage({call:e}) 
` + JsData;
      const SrcFile = PetUIExpand.SrcFile(script);
      if (typeof Worker !== "undefined") {
        const worker = new Worker(SrcFile);
        worker.data = [];
        worker.CallBack = CallBack || ((...args) => console.log(...args));
        worker.SrcFile = SrcFile;
        worker.onmessage = function(e) {
          const data = e.data;
          if (data.call) {
            if (data.call === "Destroy") worker.Destroy();
          } else {
            worker.data.push(data);
            if (worker.CallBack) worker.CallBack(data);
          }
        };
        worker.Call = worker.postMessage.bind(worker);
        worker.onerror = function(error) {
          console.warn("\u811A\u672C\u6267\u884C\u9519\u8BEF:", error);
          worker.Destroy();
        };
        worker.Destroy = function() {
          worker.terminate();
          worker.data.length = 0;
        };
        return worker;
      } else {
        console.log("\u65E0\u6CD5\u521B\u5EFA\u540E\u53F0\u4EFB\u52A1");
        return void 0;
      }
    }, "DaemonJS"),
    // 单击/双击检测
    DoubleClick: /* @__PURE__ */ __name(function(Click, Double, Mousedown, Mouseup) {
      let mouseupArgs = null;
      const func = /* @__PURE__ */ __name(function(...e) {
        if (typeof Mousedown === "function") Mousedown(...e);
        if (func.look) {
          func.Dblclick = true;
          return;
        }
        func.look = true;
        setTimeout(() => {
          if (!func.look) return;
          if (func.Dblclick) {
            if (typeof Double === "function") Double(...e);
          } else {
            if (typeof Click === "function") Click(...e);
          }
          func.look = false;
          func.Dblclick = false;
          if (mouseupArgs !== null && typeof Mouseup === "function") {
            Mouseup(...mouseupArgs);
            mouseupArgs = null;
          }
        }, 250);
      }, "func");
      func.Mouseup = function(...e) {
        if (func.look && func.Dblclick) {
          if (typeof Mouseup === "function") Mouseup(...e);
        } else {
          mouseupArgs = e;
        }
      };
      func.look = false;
      func.Dblclick = false;
      return func;
    }, "DoubleClick"),
    // 判断点是否在多边形内（绕数法）
    IsPointInPolygon: /* @__PURE__ */ __name(function(x, y, coords, width, height) {
      let wn = 0;
      width = width || 1;
      height = height || 1;
      for (let i = 3; i < coords.length; i += 2) {
        const shiftPrev = coords[i - 1] * height > y;
        const shiftCurr = coords[i] * height > y;
        if (shiftPrev !== shiftCurr) {
          const n = (shiftPrev ? 1 : 0) - (shiftCurr ? 1 : 0);
          const cross = (coords[i - 3] * width - x) * (coords[i] * height - y) - (coords[i - 2] * height - y) * (coords[i - 1] * width - x);
          if (n * cross > 0) wn += n;
        }
      }
      return wn !== 0;
    }, "IsPointInPolygon"),
    // 生成 UUID v4
    Guid: /* @__PURE__ */ __name(function() {
      return "xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx".replace(/[xy]/g, function(c) {
        const r = Math.random() * 16 | 0;
        const v = c === "x" ? r : r & 3 | 8;
        return v.toString(16);
      });
    }, "Guid"),
    // 检测是否为 PC
    IsPC: /* @__PURE__ */ __name(function() {
      const ua = navigator.userAgent;
      const agents = ["Android", "iPhone", "SymbianOS", "Windows Phone", "iPad", "iPod"];
      return !agents.some((agent) => ua.indexOf(agent) > 0);
    }, "IsPC"),
    // 检测是否支持触摸
    HasTouch: /* @__PURE__ */ __name(function() {
      return "ontouchend" in document;
    }, "HasTouch"),
    // 为对象属性添加多监听器支持
    SetChangeEvent: /* @__PURE__ */ __name(function(obj, event) {
      const handlers = [];
      let defaultReturnIndex = null;
      let transmit = false;
      const container = {
        Proto: handlers,
        get TheDefaultReturn() {
          return defaultReturnIndex;
        },
        set TheDefaultReturn(v) {
          defaultReturnIndex = v;
        },
        get Transmit() {
          return transmit;
        },
        set Transmit(v) {
          transmit = v;
        },
        Destroy: /* @__PURE__ */ __name(function(handler) {
          const index = handlers.indexOf(handler);
          if (index !== -1) {
            handlers.splice(index, 1);
            return true;
          }
          return false;
        }, "Destroy")
      };
      if (typeof obj[event] === "function") {
        handlers.push(obj[event]);
      }
      const dispatcher = /* @__PURE__ */ __name(function(...args) {
        if (defaultReturnIndex !== null) {
          return handlers[defaultReturnIndex].call(obj, ...args);
        }
        for (let i = 0; i < handlers.length; i++) {
          try {
            handlers[i].call(obj, ...args);
            if (transmit) {
              transmit = false;
              return;
            }
          } catch (err) {
            console.warn(err, "\nID: " + i);
            handlers.splice(i, 1);
            i--;
          }
        }
      }, "dispatcher");
      Object.setPrototypeOf(dispatcher, container);
      Object.defineProperty(obj, event, {
        set: /* @__PURE__ */ __name(function(value) {
          if (typeof value === "function") {
            obj[event + "ID"] = handlers.length;
            handlers.push(value);
          }
        }, "set"),
        get: /* @__PURE__ */ __name(function() {
          if (handlers.length === 1) return handlers[0];
          return dispatcher;
        }, "get"),
        configurable: true,
        enumerable: true
      });
      return container;
    }, "SetChangeEvent"),
    // 自定义 onload 队列
    SetOnload: /* @__PURE__ */ __name(function(obj) {
      const state = {
        lock: false,
        Proto: [],
        Param: []
      };
      const onloadFunc = /* @__PURE__ */ __name(function(...args) {
        state.Param = args;
        state.lock = true;
        for (let i = 0; i < state.Proto.length; i++) {
          state.Proto[i](...args);
        }
        state.Proto.length = 0;
      }, "onloadFunc");
      Object.assign(onloadFunc, state);
      if (typeof obj.onload === "function") {
        state.Proto.push(obj.onload);
      }
      obj.onload = onloadFunc;
      Object.defineProperty(obj, "onload", {
        set: /* @__PURE__ */ __name(function(fn) {
          if (typeof fn !== "function") return;
          if (onloadFunc.lock) {
            fn(...onloadFunc.Param);
          } else {
            onloadFunc.Proto.push(fn);
          }
        }, "set"),
        get: /* @__PURE__ */ __name(function() {
          console.warn("\u51FD\u6570: onload -> \u9519\u8BEF\u8C03\u7528 \u91CD\u590D\u521D\u59CB\u5316");
        }, "get")
      });
    }, "SetOnload"),
    // XHR 通用请求
    HttpQuest: /* @__PURE__ */ __name(function(option, callback = (err, result) => {
    }) {
      const url = option.url;
      const method = option.method;
      const data = option.data;
      const timeout = option.timeout || 0;
      const responseType = option.responseType;
      let xhr;
      if (window.XMLHttpRequest) {
        xhr = new XMLHttpRequest();
      } else if (window.ActiveXObject) {
        try {
          xhr = new window.ActiveXObject("Msxml2.XMLHTTP");
        } catch (e) {
          try {
            xhr = new window.ActiveXObject("Microsoft.XMLHTTP");
          } catch (e2) {
          }
        }
      }
      if (!xhr) {
        callback("No XHR support");
        return;
      }
      xhr.onreadystatechange = function() {
        if (xhr.readyState === 4) {
          if (xhr.status >= 200 && xhr.status < 400) {
            let result;
            if (xhr.responseType === "text" || responseType === "text") {
              result = xhr.responseText;
              try {
                result = JSON.parse(result);
              } catch (e) {
                console.warn("[PetUI] \u54CD\u5E94\u89E3\u6790 JSON \u5931\u8D25\uFF0C\u6309\u539F\u6587\u8FD4\u56DE", e);
              }
            } else if (xhr.responseType === "document") {
              result = xhr.responseXML;
            } else {
              result = xhr.response;
            }
            callback(null, result);
          } else {
            callback("status: " + xhr.status);
          }
        }
      };
      xhr.open(method, url, true);
      if (responseType && ["text", "arraybuffer", "blob", "document"].includes(responseType)) {
        xhr.responseType = responseType;
      }
      xhr.setRequestHeader("Content-Type", "application/x-www-form-urlencoded;");
      if (timeout > 0) {
        xhr.timeout = timeout;
        xhr.ontimeout = function() {
          callback("timeout");
        };
      }
      xhr.send(data);
    }, "HttpQuest"),
    // GET 快捷方式
    HttpGet: /* @__PURE__ */ __name(function(url, callback) {
      let options = typeof url === "string" ? { url, method: "GET" } : { ...url };
      options.method = "GET";
      PetUIExpand.HttpQuest(options, callback);
    }, "HttpGet"),
    // POST 快捷方式
    HttpPost: /* @__PURE__ */ __name(function(options, callback) {
      options.method = "POST";
      PetUIExpand.HttpQuest(options, callback);
    }, "HttpPost"),
    // 基于 WebSocket 的前后端绑定
    Bind: /* @__PURE__ */ __name(function(callFn = {}, config = { onopen: void 0, url: "" }) {
      let obj = { url: "", value: {} };
      if (config.url) {
        obj.url = config.url;
      } else {
        const scripts = document.scripts;
        if (scripts.length > 0) {
          let scriptSrc = scripts[scripts.length - 1].src;
          scriptSrc = scriptSrc.substring(0, scriptSrc.lastIndexOf("."));
          const idx = scriptSrc.indexOf("://");
          if (idx > 0) scriptSrc = scriptSrc.substring(idx + 3);
          obj.url = scriptSrc;
        } else {
          console.log("\u83B7\u53D6 API \u8C03\u7528\u63A5\u53E3\u5931\u8D25");
          return void 0;
        }
      }
      const protocol = typeof window !== "undefined" && window.location.protocol === "https:" ? "wss://" : "ws://";
      const ws = new WebSocket(protocol + obj.url);
      obj.ws = ws;
      obj.Send = function(type, name, value) {
        ws.send(JSON.stringify({ type: String(type), name: String(name), value }));
      };
      ws.onerror = ws.onclose = function() {
        obj = void 0;
      };
      obj.Close = function() {
        ws.close();
      };
      ws.onmessage = function(e) {
        const msg = JSON.parse(e.data);
        switch (msg.type) {
          case "CallFunc":
            if (typeof callFn[msg.name] === "function") {
              callFn[msg.name](msg.value);
            } else {
              console.warn("[PetUI] Bind: \u672A\u6CE8\u518C\u7684\u524D\u7AEF\u51FD\u6570: " + msg.name);
            }
            break;
          case "SetValue":
            obj.value[msg.name] = msg.value;
            break;
          case "DeleteValue":
            delete obj.value[msg.name];
            break;
          case "SetFunc":
            obj.value[msg.name] = (...args) => obj.Send("CallFunc", msg.name, args);
            break;
          case "DeleteFunc":
            delete obj.value[msg.name];
            break;
          case "onopen":
            if (typeof config.onopen === "function") config.onopen();
            break;
        }
      };
      return new Proxy(obj.value, {
        getPrototypeOf() {
          return obj;
        },
        deleteProperty(_, prop) {
          if (typeof obj.value[prop] === "function") {
            obj.Send("DeleteFunc", prop);
          } else {
            obj.Send("DeleteValue", prop);
          }
          delete obj.value[prop];
          return true;
        },
        set(_, prop, value, receiver) {
          if (typeof obj.value[prop] !== "function") {
            obj.value[prop] = value;
            obj.Send("SetValue", prop, value);
          }
          return true;
        },
        get(_, prop) {
          return obj.value[prop];
        }
      });
    }, "Bind"),
    // 禁用右键菜单
    BanMenu: /* @__PURE__ */ __name(function() {
      document.oncontextmenu = function(e) {
        e.preventDefault();
        e.returnValue = false;
      };
    }, "BanMenu")
  };

  // src/core/base.ts
  var _styleSheet = null;
  var _styleRules = /* @__PURE__ */ new Set();
  function injectStyleRules(rules) {
    if (typeof document === "undefined" || !document.head) return;
    if (!_styleSheet) {
      _styleSheet = document.createElement("style");
      _styleSheet.id = "petui-generated-styles";
      document.head.appendChild(_styleSheet);
    }
    for (const rule of rules) {
      if (!_styleRules.has(rule)) {
        _styleRules.add(rule);
        _styleSheet.textContent += rule + "\n";
      }
    }
  }
  __name(injectStyleRules, "injectStyleRules");
  var $ = {
    // 拦截 style 属性访问
    UI_Tackl_Style: /* @__PURE__ */ __name(function(styleProp, callbacks = { set: /* @__PURE__ */ __name(() => {
    }, "set"), get: /* @__PURE__ */ __name(() => {
    }, "get") }) {
      Object.defineProperty(this.style, styleProp, {
        set: /* @__PURE__ */ __name((value) => {
          this.style.setProperty(styleProp, value);
          if (callbacks.set) callbacks.set();
        }, "set"),
        get: /* @__PURE__ */ __name(() => {
          const v = this.style.getPropertyValue(styleProp);
          if (v) return v;
          if (callbacks.get) callbacks.get();
          return "";
        }, "get"),
        configurable: true,
        enumerable: true
      });
      return callbacks;
    }, "UI_Tackl_Style"),
    // 应用 CSS（对象、嵌套对象或字符串）
    CSS: /* @__PURE__ */ __name(function(style) {
      if (typeof style === "object" && style !== null && !Array.isArray(style) && StyleProcessor.isNested(style)) {
        let scopedClass = "";
        if (this instanceof HTMLElement) {
          scopedClass = "petui-s-" + Math.random().toString(36).slice(2, 10);
          this.classList.add(scopedClass);
        }
        const result = StyleProcessor.process(style, {
          parentSelector: scopedClass ? "." + scopedClass : void 0
        });
        for (const prop in result.inline) {
          if (result.inline.hasOwnProperty(prop)) {
            this.style[prop] = result.inline[prop];
          }
        }
        if (result.cssText) {
          this.style.cssText += result.cssText;
        }
        if (result.rules.length > 0) {
          injectStyleRules(result.rules);
        }
        return this;
      }
      if (typeof style === "object") {
        let css = "";
        for (const prop in style) {
          if (style.hasOwnProperty(prop)) {
            css += `${prop}:${style[prop]};`;
          }
        }
        this.style.cssText += css;
      } else if (typeof style === "string") {
        const sanitized = style.replace(/url\s*\(/gi, "blocked-url(").replace(/@import/gi, "blocked-import").replace(/expression\s*\(/gi, "blocked-expression(").replace(/javascript\s*:/gi, "blocked-js:");
        this.style.cssText += ";" + sanitized;
      }
      return this;
    }, "CSS"),
    // 从 DOM 中移除元素
    Destroy: /* @__PURE__ */ __name(function() {
      if (this.parentNode) this.parentNode.removeChild(this);
    }, "Destroy"),
    // 清空所有子元素
    Clear: /* @__PURE__ */ __name(function() {
      while (this.firstChild) {
        this.removeChild(this.firstChild);
      }
      return this;
    }, "Clear"),
    // 添加子元素（state 为 true 则插入到开头）
    Child: /* @__PURE__ */ __name(function(aChild, state) {
      aChild?.Set_Element?.();
      if (aChild === void 0 || aChild === null) {
        if (state && document.body.firstChild) {
          document.body.insertBefore(this, document.body.firstChild);
        } else {
          document.body.appendChild(this);
        }
        return this;
      }
      if (this.appendChild) {
        if (state && this.firstChild) {
          this.insertBefore(aChild, this.firstChild);
        } else {
          this.appendChild(aChild);
        }
      }
      return this;
    }, "Child"),
    // 切换类名
    Class: /* @__PURE__ */ __name(function(style, state) {
      if (typeof style === "string") {
        this.classList.toggle(style, state);
      } else if (Array.isArray(style)) {
        for (let i = 0; i < style.length; i++) this.classList.toggle(style[i], state);
      }
    }, "Class"),
    // 跨浏览器事件监听
    AddHandler: /* @__PURE__ */ __name(function(type, handler, ...use) {
      if (this.addEventListener) {
        this.addEventListener(type, handler, use.length ? use[0] : false);
      } else if (this.attachEvent) {
        this.attachEvent("on" + type, handler);
      } else {
        this["on" + type] = handler;
      }
    }, "AddHandler"),
    // 显示/隐藏元素
    Display: /* @__PURE__ */ __name(function(state) {
      if (state === void 0) return this.style.display !== "none";
      this.style.display = state ? "inline" : "none";
      return state;
    }, "Display"),
    // 保证坐标（保存相对比例，窗口缩放时还原）
    GuaranteeCoords: /* @__PURE__ */ __name(function() {
      const element = this;
      const gua = {
        Offset: { Top: 0, Left: 0 },
        EnablementUndo: false,
        Save: /* @__PURE__ */ __name(function() {
        }, "Save"),
        Recovery: /* @__PURE__ */ __name(function() {
        }, "Recovery"),
        RemoveAdaptio: /* @__PURE__ */ __name(function() {
        }, "RemoveAdaptio")
      };
      let savedTop = 0, savedLeft = 0;
      let parentNode = null;
      gua.Save = function() {
        if (gua.EnablementUndo) {
          parentNode = element.UI_Ele_Father && element.UI_Ele_Father !== document.body ? element.UI_Ele_Father : {
            offsetHeight: element.Position.Win.Height,
            offsetWidth: element.Position.Win.Width
          };
          savedTop = (element.offsetTop + element.offsetHeight / 2) / parentNode.offsetHeight;
          savedLeft = (element.offsetLeft + element.offsetWidth / 2) / parentNode.offsetWidth;
        }
      };
      gua.Recovery = function() {
        if (gua.EnablementUndo) {
          parentNode = element.UI_Ele_Father && element.UI_Ele_Father !== document.body ? element.UI_Ele_Father : {
            offsetHeight: element.Position.Win.Height,
            offsetWidth: element.Position.Win.Width
          };
          const topPx = savedTop * parentNode.offsetHeight - element.offsetHeight / 2 + gua.Offset.Top;
          const leftPx = savedLeft * parentNode.offsetWidth - element.offsetWidth / 2 + gua.Offset.Left;
          element.style.cssText += `top:${Math.round(topPx)}px;left:${Math.round(leftPx)}px;`;
        }
      };
      gua.RemoveAdaptio = function() {
        if (element.Position && element.Position.AddAffair) {
          element.Position.AddAffair.Destroy(gua.Recovery);
        }
      };
      element.UI_Tackl_Style("top", { set: gua.Save });
      element.UI_Tackl_Style("left", { set: gua.Save });
      element.Position.Affair = gua.Recovery;
      element.GuaranteeCoords = gua;
      return gua;
    }, "GuaranteeCoords"),
    // 使元素可拖动
    MobileEvent: /* @__PURE__ */ __name(function() {
      const element = this;
      const mobile = {
        MouseDown: false,
        EnablementRemovable: false,
        TheMouseCallback: /* @__PURE__ */ __name(() => {
        }, "TheMouseCallback")
      };
      const page = { X: 0, Y: 0 };
      const handleMousemove = /* @__PURE__ */ __name(function(e) {
        if (e.preventDefault) e.preventDefault();
        if (!mobile.MouseDown) return;
        if (e.buttons === 1 || e.buttons === 3) {
          if (mobile.EnablementRemovable) {
            if (element.ElementInherit && element.ElementInherit.Mousemove) {
              element.ElementInherit.Mousemove.Transmit = true;
            }
            const parent = element.parentNode && element.parentNode !== document.body ? element.parentNode : {
              offsetWidth: element.Position.Win.Width,
              offsetHeight: element.Position.Win.Height,
              offsetTop: element.Position.WinSkewing.top,
              offsetLeft: element.Position.WinSkewing.left
            };
            let newX = e.clientX - page.X;
            let newY = e.clientY - page.Y;
            const maxX = parent.offsetWidth - element.offsetWidth;
            const maxY = parent.offsetHeight - element.offsetHeight;
            if (newX < 0) newX = 0;
            if (newY < 0) newY = 0;
            if (newX > maxX) newX = maxX;
            if (newY > maxY) newY = maxY;
            element.CSS({ top: newY + "px", left: newX + "px" });
            mobile.TheMouseCallback(e);
          }
          return;
        }
        if (element.GuaranteeCoords && element.GuaranteeCoords.Save) element.GuaranteeCoords.Save();
        mobile.MouseDown = false;
        e.releaseCapture && e.releaseCapture();
        if (document.removeEventListener) {
          document.removeEventListener("mousemove", handleMousemove);
        } else if (document.detachEvent) {
          document.detachEvent("onmousemove", handleMousemove);
        }
      }, "handleMousemove");
      const handleMouseup = /* @__PURE__ */ __name(function(e) {
        mobile.MouseDown = false;
        if (element.GuaranteeCoords && element.GuaranteeCoords.Save) element.GuaranteeCoords.Save();
        e.releaseCapture && e.releaseCapture();
        if (document.removeEventListener) {
          document.removeEventListener("mousemove", handleMousemove);
          document.removeEventListener("mouseup", handleMouseup);
        } else if (document.detachEvent) {
          document.detachEvent("onmousemove", handleMousemove);
          document.detachEvent("onmouseup", handleMouseup);
        }
      }, "handleMouseup");
      element.Mousedown = function(e) {
        if (mobile.MouseDown) {
          return;
        }
        if (mobile.EnablementRemovable) {
          e.setCapture && e.setCapture();
          mobile.MouseDown = true;
          page.X = e.clientX - element.offsetLeft;
          page.Y = e.clientY - element.offsetTop;
          if (document.addEventListener) {
            document.addEventListener("mousemove", handleMousemove);
            document.addEventListener("mouseup", handleMouseup);
          } else if (document.attachEvent) {
            document.attachEvent("onmousemove", handleMousemove);
            document.attachEvent("onmouseup", handleMouseup);
          }
        }
      };
      element.MobileEvent = mobile;
      return mobile;
    }, "MobileEvent"),
    // 创建元素（或增强已有元素）。
    // 传入字符串 → document.createElement 创建新元素；
    // 传入 HTMLElement → 直接增强（已增强过的元素原样返回）。
    Create: /* @__PURE__ */ __name(function(elementId) {
      let rawElement;
      if (typeof elementId === "string") {
        rawElement = document.createElement(elementId);
      } else {
        if (elementId && typeof elementId.Create !== "undefined") return elementId;
        rawElement = elementId;
      }
      if (!rawElement) return null;
      return enhance(rawElement);
    }, "Create"),
    // 按 ID 查找已存在元素并增强（找不到返回 null）。
    ById: /* @__PURE__ */ __name(function(id) {
      const rawElement = document.getElementById(id);
      if (!rawElement) return null;
      return enhance(rawElement);
    }, "ById")
  };
  var INSTANCE_METHODS = [
    "UI_Tackl_Style",
    "CSS",
    "Destroy",
    "Clear",
    "Child",
    "Class",
    "AddHandler",
    "Display",
    "GuaranteeCoords",
    "MobileEvent"
  ];
  function enhance(rawElement) {
    const ele = rawElement;
    for (const key of INSTANCE_METHODS) {
      ele[key] = $[key];
    }
    ele.Click = null;
    ele.Dblclick = null;
    ele.Mousedown = null;
    ele.Mouseup = null;
    ele.Mouseout = null;
    ele.Mousemove = null;
    ele.ElementInherit = {
      Click: PetUIExpand.SetChangeEvent(ele, "Click"),
      Dblclick: PetUIExpand.SetChangeEvent(ele, "Dblclick"),
      Mousedown: PetUIExpand.SetChangeEvent(ele, "Mousedown"),
      Mouseup: PetUIExpand.SetChangeEvent(ele, "Mouseup"),
      Mouseout: PetUIExpand.SetChangeEvent(ele, "Mouseout"),
      Mousemove: PetUIExpand.SetChangeEvent(ele, "Mousemove")
    };
    const clickHandler = PetUIExpand.DoubleClick(
      ele.Click,
      ele.Dblclick,
      ele.Mousedown,
      ele.Mouseup
    );
    if (PetUIExpand.HasTouch()) {
      ele.MobileEndEventPassing = {
        Click: clickHandler,
        Mouseup: clickHandler.Mouseup,
        Mousemove: ele.Mousemove
      };
      ele.TheMobileEventTakesEffect = false;
      ele.Set_Element = function() {
        ele.TheMobileEventTakesEffect = true;
      };
      ele.Remove_Element = function() {
        ele.TheMobileEventTakesEffect = false;
      };
    } else {
      ele.Set_Element = function() {
        if (ele.__handlersBound) return;
        const handlers = {
          click: clickHandler,
          mouseup: clickHandler.Mouseup,
          mouseout: ele.Mouseout,
          mousedown: ele.Mousedown,
          mousemove: ele.Mousemove
        };
        ele.__handlersBound = handlers;
        ele.AddHandler("click", handlers.click);
        ele.AddHandler("mouseup", handlers.mouseup);
        ele.AddHandler("mouseout", handlers.mouseout);
        ele.AddHandler("mousedown", handlers.mousedown);
        ele.AddHandler("mousemove", handlers.mousemove);
      };
      ele.Remove_Element = function() {
        const bound = ele.__handlersBound;
        if (!bound) return;
        if (ele.removeEventListener) {
          ele.removeEventListener("click", bound.click);
          ele.removeEventListener("mouseup", bound.mouseup);
          ele.removeEventListener("mouseout", bound.mouseout);
          ele.removeEventListener("mousedown", bound.mousedown);
          ele.removeEventListener("mousemove", bound.mousemove);
        } else if (ele.detachEvent) {
          ele.detachEvent("onclick", bound.click);
          ele.detachEvent("onmouseup", bound.mouseup);
          ele.detachEvent("onmouseout", bound.mouseout);
          ele.detachEvent("onmousedown", bound.mousedown);
          ele.detachEvent("onmousemove", bound.mousemove);
        } else {
          ele.onclick = null;
          ele.onmouseup = null;
          ele.onmouseout = null;
          ele.onmousedown = null;
          ele.onmousemove = null;
        }
        ele.__handlersBound = null;
      };
    }
    return ele;
  }
  __name(enhance, "enhance");

  // src/core/builder.ts
  function build(...args) {
    if (args.length === 0) {
      return (() => void 0);
    }
    let ele = this && this.Child ? this : void 0;
    let $ele = ele;
    for (const arg of args) {
      if (typeof arg === "string") {
        if (arg[0] === "#") {
          $ele = $.ById(arg.slice(1));
        } else {
          $ele = $.Create(arg);
        }
        if (ele) {
          ele = ele.Child($ele);
        } else {
          ele = $ele;
        }
      } else if (arg && typeof arg.nodeName === "string") {
        $ele = $.Create(arg);
        if (ele) {
          ele = ele.Child($ele);
        } else {
          ele = $ele;
        }
      } else if (Array.isArray(arg)) {
        if (!$ele) {
          $ele = {
            Create: /* @__PURE__ */ __name((param) => {
              ele = build.call(void 0, param);
              return ele;
            }, "Create")
          };
        }
        let node = $ele;
        let $node;
        for (const item of arg) {
          if (item instanceof Object && !Array.isArray(item) && !item.nodeName) {
            $node = build.call(node, item);
            node = $node;
            continue;
          } else {
            $node = build.call(void 0, item);
          }
          if (!ele) {
            ele = $node;
          } else {
            if (node) node.Child($node);
          }
          node = $node;
        }
      } else if (arg && typeof arg === "object") {
        const target = $ele || ele;
        if (target) {
          for (const key in arg) {
            if (arg.hasOwnProperty(key)) {
              if (key === "style" && typeof arg[key] === "object") {
                for (const styleProp in arg[key]) {
                  if (arg[key].hasOwnProperty(styleProp)) {
                    target.style[styleProp] = arg[key][styleProp];
                  }
                }
              } else if (key === "AddHandler" && typeof arg[key] === "function") {
                arg[key](target);
              } else {
                target[key] = arg[key];
              }
            }
          }
        }
      }
    }
    if (!ele) return this;
    ele.Create = build;
    return ele;
  }
  __name(build, "build");

  // src/themes/sizes.ts
  var SPACE_SIZES = Object.freeze({
    "--space-1": "1px",
    "--space-2": "2px",
    "--space-3": "3px",
    "--space-4": "4px",
    "--space-5": "5px",
    "--space-6": "6px",
    "--space-8": "8px",
    "--space-10": "10px",
    "--space-12": "12px",
    "--space-14": "14px",
    "--space-15": "15px",
    "--space-16": "16px",
    "--space-18": "18px",
    "--space-20": "20px",
    "--space-22": "22px",
    "--space-24": "24px",
    "--space-25": "25px",
    "--space-26": "26px",
    "--space-32": "32px",
    "--space-36": "36px",
    "--space-40": "40px",
    "--space-44": "44px",
    "--space-60": "60px",
    "--space-64": "64px",
    "--space-140": "140px",
    "--space-160": "160px",
    "--space-180": "180px",
    "--space-200": "200px",
    "--space-250": "250px",
    "--space-300": "300px",
    "--space-400": "400px",
    "--space-500": "500px"
  });
  var FONT_SIZES = Object.freeze({
    "--font-size-1": "1px",
    "--font-size-2": "2px",
    "--font-size-3": "3px",
    "--font-size-4": "4px",
    "--font-size-6": "6px",
    "--font-size-12": "12px",
    "--font-size-13": "13px",
    "--font-size-14": "14px",
    "--font-size-15": "15px",
    "--font-size-16": "16px",
    "--font-size-25": "25px",
    "--font-size-32": "32px",
    "--font-size-60": "60px",
    "--font-size-64": "64px",
    "--font-size-180": "180px"
  });
  var ROW_HEIGHTS = Object.freeze({
    "--row-height-compact": "22px",
    // 22px
    "--row-height-normal": "24px",
    // 24px
    "--row-height-comfortable": "26px",
    // 26px
    "--row-height-tall": "32px"
    // 32px
  });
  var Z_INDEX = Object.freeze({
    "--z-base": "0",
    "--z-raised": "1",
    "--z-sticky": "10",
    "--z-dropdown": "100",
    "--z-overlay": "500",
    "--z-modal": "1000",
    "--z-popover": "1200",
    "--z-toast": "2000",
    "--z-tooltip": "3000"
  });
  var THEME_INDEPENDENT_TOKENS = Object.freeze({
    ...SPACE_SIZES,
    ...FONT_SIZES,
    ...ROW_HEIGHTS,
    ...Z_INDEX
  });
  var THEME_INDEPENDENT_TOKEN_NAMES = Object.freeze(
    Object.keys(THEME_INDEPENDENT_TOKENS)
  );

  // src/themes/themes.ts
  var TOKEN_TIERS = Object.freeze({
    "--primitive-": "primitive",
    "--color-": "semantic",
    "--canvas-": "semantic",
    "--alpha-": "semantic",
    "--font-size-": "semantic",
    "--row-height-": "semantic",
    "--z-": "semantic",
    "--space-": "semantic",
    "--border-radius-": "semantic",
    "--shadow-": "semantic",
    "--spacing-": "semantic"
  });
  var PRIMITIVE_COLORS = Object.freeze({
    "--primitive-pink-300": "#FFB6C1",
    "--primitive-pink-100": "#ffd6dc",
    "--primitive-pink-50": "#fff0f3",
    "--primitive-pink-500": "#d4888f",
    "--primitive-pink-400": "#f48fb1",
    "--primitive-green-500": "#67c23a",
    "--primitive-amber-500": "#e6a23c",
    "--primitive-red-500": "#f56c6c",
    "--primitive-gray-500": "#909399",
    "--primitive-neutral-900": "#333333",
    "--primitive-neutral-800": "#444444",
    "--primitive-neutral-700": "#555555",
    "--primitive-neutral-600": "#666666",
    "--primitive-neutral-400": "#888888",
    "--primitive-neutral-300": "#999999",
    "--primitive-neutral-200": "#cccccc",
    "--primitive-neutral-100": "#e0e0e0",
    "--primitive-neutral-50": "#eeeeee",
    "--primitive-neutral-25": "#fafafa",
    "--primitive-white": "#ffffff"
  });
  var CANVAS_TOKENS = Object.freeze({
    default: Object.freeze({
      "--canvas-background": "#fafafa",
      "--canvas-surface": "#ffffff",
      "--canvas-grid": "#eeeeee",
      "--canvas-axis": "#909399",
      "--canvas-text": "#333333",
      "--canvas-selection": "#f48fb1",
      "--canvas-series-1": "#FFB6C1",
      "--canvas-series-2": "#5BC0EB"
    }),
    ocean: Object.freeze({
      "--canvas-background": "#f4fafc",
      "--canvas-surface": "#ffffff",
      "--canvas-grid": "#ddeff7",
      "--canvas-axis": "#3498db",
      "--canvas-text": "#2c3e50",
      "--canvas-selection": "#8ad4f5",
      "--canvas-series-1": "#5BC0EB",
      "--canvas-series-2": "#2ecc71"
    }),
    forest: Object.freeze({
      "--canvas-background": "#f4faf6",
      "--canvas-surface": "#ffffff",
      "--canvas-grid": "#dcebe3",
      "--canvas-axis": "#2980b9",
      "--canvas-text": "#2f3e34",
      "--canvas-selection": "#8fd4ab",
      "--canvas-series-1": "#6BBF8E",
      "--canvas-series-2": "#e67e22"
    }),
    sunset: Object.freeze({
      "--canvas-background": "#fdf8f4",
      "--canvas-surface": "#ffffff",
      "--canvas-grid": "#f5e0d4",
      "--canvas-axis": "#5c8bb5",
      "--canvas-text": "#4a3728",
      "--canvas-selection": "#ffab84",
      "--canvas-series-1": "#FF8C5A",
      "--canvas-series-2": "#58b368"
    }),
    dark: Object.freeze({
      "--canvas-background": "#1e1e2e",
      "--canvas-surface": "#2a2a3e",
      "--canvas-grid": "#3a3a52",
      "--canvas-axis": "#5bb8f0",
      "--canvas-text": "#e8e8f0",
      "--canvas-selection": "#cfc6fa",
      "--canvas-series-1": "#b4a7f6",
      "--canvas-series-2": "#5fd68a"
    }),
    mono: Object.freeze({
      "--canvas-background": "#f5f5f5",
      "--canvas-surface": "#ffffff",
      "--canvas-grid": "#dddddd",
      "--canvas-axis": "#3f6a8c",
      "--canvas-text": "#222222",
      "--canvas-selection": "#7a7a7a",
      "--canvas-series-1": "#5c5c5c",
      "--canvas-series-2": "#a8782c"
    })
  });
  var ALPHA_TOKENS = Object.freeze({
    "--alpha-scrim-strong": "rgba(0,0,0,0.75)",
    "--alpha-scrim-soft": "rgba(0,0,0,0.4)",
    "--alpha-scrim": "rgba(0,0,0,0.3)",
    "--alpha-veil": "rgba(0,0,0,0.2)",
    "--alpha-shade": "rgba(0,0,0,0.1)",
    "--alpha-ink": "rgba(0,0,0,1)",
    "--alpha-frost": "rgba(255,255,255,0.85)",
    "--alpha-glow": "rgba(255,255,255,0.3)",
    "--alpha-highlight": "rgba(255,255,255,0.2)",
    "--alpha-flat": "rgba(0,0,0,0)"
  });
  var LITERAL_COLOR_TOKENS = Object.freeze({
    "#1e1e1e": "--color-n-1e1e1e",
    "#d4d4d4": "--color-n-d4d4d4",
    "#e8f4fd": "--color-n-e8f4fd",
    "#91caf0": "--color-n-91caf0",
    "#1a6daa": "--color-n-1a6daa",
    "#e8f8e8": "--color-n-e8f8e8",
    "#8cd98c": "--color-n-8cd98c",
    "#2d7a2d": "--color-n-2d7a2d",
    "#fef8e7": "--color-n-fef8e7",
    "#f0c040": "--color-n-f0c040",
    "#8a6d20": "--color-n-8a6d20",
    "#fde8e8": "--color-n-fde8e8",
    "#f09898": "--color-n-f09898",
    "#a82d2d": "--color-n-a82d2d",
    "#95a5a6": "--color-n-95a5a6",
    "#aaa": "--color-n-aaa",
    "#e0d4d8": "--color-n-e0d4d8",
    "#666": "--color-n-666",
    "#f0f0f0": "--color-n-f0f0f0",
    "#fafafa": "--color-n-fafafa",
    "#e0e0e0": "--color-n-e0e0e0",
    "#fff": "--color-n-fff",
    "#eee": "--color-n-eee",
    "#333": "--color-n-333",
    "#555": "--color-n-555",
    "#fff0f3": "--color-n-fff0f3",
    "#f48fb1": "--color-n-f48fb1",
    "#ccc": "--color-n-ccc",
    "#FFB6C1": "--color-n-ffb6c1",
    "rgba(255,182,193,0.4)": "--color-alpha-primary-40",
    "rgba(0,0,0,0.08)": "--color-alpha-shade-08",
    "rgba(0,0,0,0.12)": "--color-alpha-shade-12",
    "rgba(0,0,0,0.15)": "--color-alpha-shade-15",
    "rgba(0,0,0,0.1)": "--color-alpha-shade-10",
    "rgba(0,0,0,0.2)": "--color-alpha-shade-20",
    "rgba(0,0,0,0.4)": "--color-alpha-shade-40",
    "rgba(0,0,0,0.75)": "--color-alpha-shade-75",
    "rgba(0,0,0,1)": "--color-alpha-shade-100",
    "rgba(255,255,255,0.2)": "--color-alpha-light-20",
    "rgba(255,255,255,0.3)": "--color-alpha-light-30",
    "rgba(255,255,255,0.85)": "--color-alpha-light-85",
    "rgba(255,182,193,0.5)": "--color-alpha-primary-50",
    "rgba(255,182,193,0.3)": "--color-alpha-primary-30"
  });
  var PETUI_THEMES = [
    {
      name: "default",
      label: "\u7ECF\u5178\u7C89",
      description: "\u7C89\u8272\u7CFB\u5C11\u5973\u98CE\uFF0C\u67D4\u548C\u53EF\u7231",
      variables: {
        "--color-primary": "#FFB6C1",
        "--color-primary-hover": "#ffd6dc",
        "--color-primary-light": "#fff0f3",
        "--color-primary-dark": "#d4888f",
        "--color-success": "#67c23a",
        "--color-warning": "#e6a23c",
        "--color-danger": "#f56c6c",
        "--color-info": "#909399",
        "--color-text": "#333333",
        "--color-text-secondary": "#888888",
        "--color-text-muted": "#999999",
        "--color-border": "#eeeeee",
        "--color-bg": "#fafafa",
        "--color-bg-light": "#ffffff",
        "--border-radius-sm": "4px",
        "--border-radius-md": "8px",
        "--border-radius-lg": "12px",
        "--shadow-light": "0 1px 4px rgba(0,0,0,0.08)",
        "--shadow-medium": "0 4px 16px rgba(0,0,0,0.12)",
        "--spacing-sm": "8px",
        "--spacing-md": "16px",
        "--spacing-lg": "24px",
        "--canvas-background": "#fafafa",
        "--canvas-surface": "#ffffff",
        "--canvas-grid": "#eeeeee",
        "--canvas-axis": "#909399",
        "--canvas-text": "#333333",
        "--canvas-selection": "#f48fb1",
        "--canvas-series-1": "#FFB6C1",
        "--canvas-series-2": "#5BC0EB",
        "--alpha-scrim-strong": "rgba(0,0,0,0.75)",
        "--alpha-scrim-soft": "rgba(0,0,0,0.4)",
        "--alpha-scrim": "rgba(0,0,0,0.3)",
        "--alpha-veil": "rgba(0,0,0,0.2)",
        "--alpha-shade": "rgba(0,0,0,0.1)",
        "--alpha-ink": "rgba(0,0,0,1)",
        "--alpha-frost": "rgba(255,255,255,0.85)",
        "--alpha-glow": "rgba(255,255,255,0.3)",
        "--alpha-highlight": "rgba(255,255,255,0.2)",
        "--alpha-flat": "rgba(0,0,0,0)",
        "--color-n-1e1e1e": "#1e1e1e",
        "--color-n-d4d4d4": "#d4d4d4",
        "--color-n-e8f4fd": "#e8f4fd",
        "--color-n-91caf0": "#91caf0",
        "--color-n-1a6daa": "#1a6daa",
        "--color-n-e8f8e8": "#e8f8e8",
        "--color-n-8cd98c": "#8cd98c",
        "--color-n-2d7a2d": "#2d7a2d",
        "--color-n-fef8e7": "#fef8e7",
        "--color-n-f0c040": "#f0c040",
        "--color-n-8a6d20": "#8a6d20",
        "--color-n-fde8e8": "#fde8e8",
        "--color-n-f09898": "#f09898",
        "--color-n-a82d2d": "#a82d2d",
        "--color-n-95a5a6": "#95a5a6",
        "--color-n-aaa": "#aaa",
        "--color-n-e0d4d8": "#e0d4d8",
        "--color-n-666": "#666",
        "--color-n-f0f0f0": "#f0f0f0",
        "--color-n-fafafa": "#fafafa",
        "--color-n-e0e0e0": "#e0e0e0",
        "--color-n-fff": "#fff",
        "--color-n-eee": "#eee",
        "--color-n-333": "#333",
        "--color-n-555": "#555",
        "--color-n-fff0f3": "#fff0f3",
        "--color-n-f48fb1": "#f48fb1",
        "--color-n-ccc": "#ccc",
        "--color-n-ffb6c1": "#FFB6C1",
        "--color-alpha-primary-40": "rgba(255,182,193,0.4)",
        "--color-alpha-shade-08": "rgba(0,0,0,0.08)",
        "--color-alpha-shade-12": "rgba(0,0,0,0.12)",
        "--color-alpha-shade-15": "rgba(0,0,0,0.15)",
        "--color-alpha-shade-10": "rgba(0,0,0,0.1)",
        "--color-alpha-shade-20": "rgba(0,0,0,0.2)",
        "--color-alpha-shade-40": "rgba(0,0,0,0.4)",
        "--color-alpha-shade-75": "rgba(0,0,0,0.75)",
        "--color-alpha-shade-100": "rgba(0,0,0,1)",
        "--color-alpha-light-20": "rgba(255,255,255,0.2)",
        "--color-alpha-light-30": "rgba(255,255,255,0.3)",
        "--color-alpha-light-85": "rgba(255,255,255,0.85)",
        "--color-alpha-primary-50": "rgba(255,182,193,0.5)",
        "--color-alpha-primary-30": "rgba(255,182,193,0.3)",
        "--font-size-12": "12px",
        "--font-size-13": "13px",
        "--font-size-14": "14px",
        "--space-1000": "1000px"
      }
    },
    {
      name: "ocean",
      label: "\u6D77\u6D0B\u84DD",
      description: "\u6D77\u6D0B\u84DD\u8C03\uFF0C\u5706\u6DA6\u6E05\u65B0",
      variables: {
        "--color-primary": "#5BC0EB",
        "--color-primary-hover": "#8ad4f5",
        "--color-primary-light": "#e8f6fd",
        "--color-primary-dark": "#3a8fc0",
        "--color-success": "#2ecc71",
        "--color-warning": "#f39c12",
        "--color-danger": "#e74c3c",
        "--color-info": "#3498db",
        "--color-text": "#2c3e50",
        "--color-text-secondary": "#7f8c8d",
        "--color-text-muted": "#95a5a6",
        "--color-border": "#ddeff7",
        "--color-bg": "#f4fafc",
        "--color-bg-light": "#ffffff",
        "--border-radius-sm": "8px",
        "--border-radius-md": "12px",
        "--border-radius-lg": "16px",
        "--shadow-light": "0 2px 8px rgba(52,152,219,0.12)",
        "--shadow-medium": "0 6px 20px rgba(52,152,219,0.16)",
        "--spacing-sm": "8px",
        "--spacing-md": "16px",
        "--spacing-lg": "24px",
        "--canvas-background": "#f4fafc",
        "--canvas-surface": "#ffffff",
        "--canvas-grid": "#ddeff7",
        "--canvas-axis": "#3498db",
        "--canvas-text": "#2c3e50",
        "--canvas-selection": "#8ad4f5",
        "--canvas-series-1": "#5BC0EB",
        "--canvas-series-2": "#2ecc71",
        "--alpha-scrim-strong": "rgba(0,0,0,0.75)",
        "--alpha-scrim-soft": "rgba(0,0,0,0.4)",
        "--alpha-scrim": "rgba(0,0,0,0.3)",
        "--alpha-veil": "rgba(0,0,0,0.2)",
        "--alpha-shade": "rgba(0,0,0,0.1)",
        "--alpha-ink": "rgba(0,0,0,1)",
        "--alpha-frost": "rgba(255,255,255,0.85)",
        "--alpha-glow": "rgba(255,255,255,0.3)",
        "--alpha-highlight": "rgba(255,255,255,0.2)",
        "--alpha-flat": "rgba(0,0,0,0)"
      }
    },
    {
      name: "forest",
      label: "\u68EE\u6797\u7EFF",
      description: "\u81EA\u7136\u68EE\u6797\u7CFB\uFF0C\u6C89\u7A33\u8212\u9002",
      variables: {
        "--color-primary": "#6BBF8E",
        "--color-primary-hover": "#8fd4ab",
        "--color-primary-light": "#eaf7f0",
        "--color-primary-dark": "#4a9c6e",
        "--color-success": "#27ae60",
        "--color-warning": "#e67e22",
        "--color-danger": "#c0392b",
        "--color-info": "#2980b9",
        "--color-text": "#2f3e34",
        "--color-text-secondary": "#7a8f82",
        "--color-text-muted": "#95a5a6",
        "--color-border": "#dcebe3",
        "--color-bg": "#f4faf6",
        "--color-bg-light": "#ffffff",
        "--border-radius-sm": "6px",
        "--border-radius-md": "10px",
        "--border-radius-lg": "14px",
        "--shadow-light": "0 1px 6px rgba(74,156,110,0.10)",
        "--shadow-medium": "0 4px 16px rgba(74,156,110,0.15)",
        "--spacing-sm": "8px",
        "--spacing-md": "16px",
        "--spacing-lg": "24px",
        "--canvas-background": "#f4faf6",
        "--canvas-surface": "#ffffff",
        "--canvas-grid": "#dcebe3",
        "--canvas-axis": "#2980b9",
        "--canvas-text": "#2f3e34",
        "--canvas-selection": "#8fd4ab",
        "--canvas-series-1": "#6BBF8E",
        "--canvas-series-2": "#e67e22",
        "--alpha-scrim-strong": "rgba(0,0,0,0.75)",
        "--alpha-scrim-soft": "rgba(0,0,0,0.4)",
        "--alpha-scrim": "rgba(0,0,0,0.3)",
        "--alpha-veil": "rgba(0,0,0,0.2)",
        "--alpha-shade": "rgba(0,0,0,0.1)",
        "--alpha-ink": "rgba(0,0,0,1)",
        "--alpha-frost": "rgba(255,255,255,0.85)",
        "--alpha-glow": "rgba(255,255,255,0.3)",
        "--alpha-highlight": "rgba(255,255,255,0.2)",
        "--alpha-flat": "rgba(0,0,0,0)"
      }
    },
    {
      name: "sunset",
      label: "\u843D\u65E5\u6A59",
      description: "\u6E29\u6696\u843D\u65E5\uFF0C\u70ED\u60C5\u6D3B\u529B",
      variables: {
        "--color-primary": "#FF8C5A",
        "--color-primary-hover": "#ffab84",
        "--color-primary-light": "#fff0e8",
        "--color-primary-dark": "#d96f3d",
        "--color-success": "#58b368",
        "--color-warning": "#f2a53a",
        "--color-danger": "#e4644f",
        "--color-info": "#5c8bb5",
        "--color-text": "#4a3728",
        "--color-text-secondary": "#9a7f6f",
        "--color-text-muted": "#95a5a6",
        "--color-border": "#f5e0d4",
        "--color-bg": "#fdf8f4",
        "--color-bg-light": "#ffffff",
        "--border-radius-sm": "8px",
        "--border-radius-md": "12px",
        "--border-radius-lg": "16px",
        "--shadow-light": "0 2px 8px rgba(217,111,61,0.14)",
        "--shadow-medium": "0 6px 20px rgba(217,111,61,0.18)",
        "--spacing-sm": "8px",
        "--spacing-md": "16px",
        "--spacing-lg": "24px",
        "--canvas-background": "#fdf8f4",
        "--canvas-surface": "#ffffff",
        "--canvas-grid": "#f5e0d4",
        "--canvas-axis": "#5c8bb5",
        "--canvas-text": "#4a3728",
        "--canvas-selection": "#ffab84",
        "--canvas-series-1": "#FF8C5A",
        "--canvas-series-2": "#58b368",
        "--alpha-scrim-strong": "rgba(0,0,0,0.75)",
        "--alpha-scrim-soft": "rgba(0,0,0,0.4)",
        "--alpha-scrim": "rgba(0,0,0,0.3)",
        "--alpha-veil": "rgba(0,0,0,0.2)",
        "--alpha-shade": "rgba(0,0,0,0.1)",
        "--alpha-ink": "rgba(0,0,0,1)",
        "--alpha-frost": "rgba(255,255,255,0.85)",
        "--alpha-glow": "rgba(255,255,255,0.3)",
        "--alpha-highlight": "rgba(255,255,255,0.2)",
        "--alpha-flat": "rgba(0,0,0,0)"
      }
    },
    {
      name: "dark",
      label: "\u6697\u591C\u7D2B",
      description: "\u6697\u8272\u7CFB\uFF0C\u7D2B\u8272\u9713\u8679",
      variables: {
        "--color-primary": "#b4a7f6",
        "--color-primary-hover": "#cfc6fa",
        "--color-primary-light": "#2a2a3e",
        "--color-primary-dark": "#8f7fe0",
        "--color-success": "#5fd68a",
        "--color-warning": "#ffc24d",
        "--color-danger": "#ff6b6b",
        "--color-info": "#5bb8f0",
        "--color-text": "#e8e8f0",
        "--color-text-secondary": "#9a9ab0",
        "--color-text-muted": "#95a5a6",
        "--color-border": "#3a3a52",
        "--color-bg": "#1e1e2e",
        "--color-bg-light": "#2a2a3e",
        "--border-radius-sm": "8px",
        "--border-radius-md": "12px",
        "--border-radius-lg": "16px",
        "--shadow-light": "0 2px 8px rgba(0,0,0,0.4)",
        "--shadow-medium": "0 6px 20px rgba(0,0,0,0.5)",
        "--spacing-sm": "8px",
        "--spacing-md": "16px",
        "--spacing-lg": "24px",
        "--canvas-background": "#1e1e2e",
        "--canvas-surface": "#2a2a3e",
        "--canvas-grid": "#3a3a52",
        "--canvas-axis": "#5bb8f0",
        "--canvas-text": "#e8e8f0",
        "--canvas-selection": "#cfc6fa",
        "--canvas-series-1": "#b4a7f6",
        "--canvas-series-2": "#5fd68a",
        "--alpha-scrim-strong": "rgba(0,0,0,0.75)",
        "--alpha-scrim-soft": "rgba(0,0,0,0.4)",
        "--alpha-scrim": "rgba(0,0,0,0.3)",
        "--alpha-veil": "rgba(0,0,0,0.2)",
        "--alpha-shade": "rgba(0,0,0,0.1)",
        "--alpha-ink": "rgba(0,0,0,1)",
        "--alpha-frost": "rgba(255,255,255,0.85)",
        "--alpha-glow": "rgba(255,255,255,0.3)",
        "--alpha-highlight": "rgba(255,255,255,0.2)",
        "--alpha-flat": "rgba(0,0,0,0)"
      }
    },
    {
      name: "mono",
      label: "\u6781\u7B80\u7070",
      description: "\u9ED1\u767D\u7070\u6781\u7B80\uFF0C\u7D27\u51D1\u9510\u5229",
      variables: {
        "--color-primary": "#5c5c5c",
        "--color-primary-hover": "#7a7a7a",
        "--color-primary-light": "#f2f2f2",
        "--color-primary-dark": "#404040",
        "--color-success": "#3f7d4e",
        "--color-warning": "#a8782c",
        "--color-danger": "#a8433c",
        "--color-info": "#3f6a8c",
        "--color-text": "#222222",
        "--color-text-secondary": "#777777",
        "--color-text-muted": "#95a5a6",
        "--color-border": "#dddddd",
        "--color-bg": "#f5f5f5",
        "--color-bg-light": "#ffffff",
        "--border-radius-sm": "2px",
        "--border-radius-md": "4px",
        "--border-radius-lg": "8px",
        "--shadow-light": "0 1px 3px rgba(0,0,0,0.08)",
        "--shadow-medium": "0 3px 10px rgba(0,0,0,0.10)",
        "--spacing-sm": "4px",
        "--spacing-md": "8px",
        "--spacing-lg": "16px",
        "--canvas-background": "#f5f5f5",
        "--canvas-surface": "#ffffff",
        "--canvas-grid": "#dddddd",
        "--canvas-axis": "#3f6a8c",
        "--canvas-text": "#222222",
        "--canvas-selection": "#7a7a7a",
        "--canvas-series-1": "#5c5c5c",
        "--canvas-series-2": "#a8782c",
        "--alpha-scrim-strong": "rgba(0,0,0,0.75)",
        "--alpha-scrim-soft": "rgba(0,0,0,0.4)",
        "--alpha-scrim": "rgba(0,0,0,0.3)",
        "--alpha-veil": "rgba(0,0,0,0.2)",
        "--alpha-shade": "rgba(0,0,0,0.1)",
        "--alpha-ink": "rgba(0,0,0,1)",
        "--alpha-frost": "rgba(255,255,255,0.85)",
        "--alpha-glow": "rgba(255,255,255,0.3)",
        "--alpha-highlight": "rgba(255,255,255,0.2)",
        "--alpha-flat": "rgba(0,0,0,0)"
      }
    }
  ];
  var THEME_INDEPENDENT_VARIABLES = Object.freeze({
    ...SPACE_SIZES,
    ...FONT_SIZES,
    ...ROW_HEIGHTS,
    ...Z_INDEX
  });

  // src/themes/index.ts
  var themeRegistry = /* @__PURE__ */ new Map();
  for (const theme of PETUI_THEMES) {
    themeRegistry.set(theme.name, theme);
  }
  var currentTheme = "default";
  var appliedKeys = [];
  var resolvedCache = /* @__PURE__ */ new Map();
  var declaredCache = null;
  function resolveRootStyle() {
    if (typeof document === "undefined") return null;
    const root = document.documentElement;
    return root ? root.style : null;
  }
  __name(resolveRootStyle, "resolveRootStyle");
  function isPetUITheme(value) {
    if (typeof value !== "object" || value === null) return false;
    const candidate = value;
    if (typeof candidate.name !== "string" || candidate.name.length === 0) return false;
    if (typeof candidate.variables !== "object" || candidate.variables === null) return false;
    return true;
  }
  __name(isPetUITheme, "isPetUITheme");
  function resolveVariables(name) {
    const cached = resolvedCache.get(name);
    if (cached) return cached;
    const theme = themeRegistry.get(name);
    const merged = /* @__PURE__ */ Object.create(null);
    if (theme) {
      for (const key of Object.keys(theme.variables)) merged[key] = theme.variables[key];
    }
    for (const key of Object.keys(THEME_INDEPENDENT_VARIABLES)) merged[key] = THEME_INDEPENDENT_VARIABLES[key];
    const frozen = Object.freeze(merged);
    resolvedCache.set(name, frozen);
    return frozen;
  }
  __name(resolveVariables, "resolveVariables");
  function deepFreezeVariables(vars) {
    return Object.freeze({ ...vars });
  }
  __name(deepFreezeVariables, "deepFreezeVariables");
  function makeSnapshot(name) {
    return Object.freeze({
      name,
      variables: deepFreezeVariables(resolveVariables(name))
    });
  }
  __name(makeSnapshot, "makeSnapshot");
  var subscribers = [];
  function notifySubscribers(name) {
    const snapshot2 = makeSnapshot(name);
    const round = subscribers.slice();
    for (const cb of round) {
      if (subscribers.indexOf(cb) < 0) continue;
      try {
        cb(snapshot2);
      } catch (err) {
        console.error(`[PetUI themes] \u4E3B\u9898\u8BA2\u9605\u56DE\u8C03\u629B\u9519\uFF08\u4E3B\u9898 "${name}"\uFF09\uFF0C\u5DF2\u9694\u79BB\u8BE5\u8BA2\u9605\u8005:`, err);
      }
    }
  }
  __name(notifySubscribers, "notifySubscribers");
  function familyOf(name) {
    let best = null;
    for (const prefix of Object.keys(TOKEN_TIERS)) {
      if (name.startsWith(prefix) && (best === null || prefix.length > best.length)) best = prefix;
    }
    return best;
  }
  __name(familyOf, "familyOf");
  function applyTheme(name) {
    const theme = themeRegistry.get(name);
    if (!theme) {
      console.warn(`[PetUI themes] \u672A\u77E5\u4E3B\u9898 "${name}"\uFF0C\u53EF\u7528\u4E3B\u9898: ${Array.from(themeRegistry.keys()).join(", ")}`);
      return;
    }
    const rootStyle = resolveRootStyle();
    if (!rootStyle) {
      currentTheme = name;
      console.warn(`[PetUI themes] document \u4E0D\u53EF\u7528\uFF08SSR/\u6D4B\u8BD5\u73AF\u5883\uFF09\uFF0C\u4EC5\u8BB0\u5F55\u5F53\u524D\u4E3B\u9898\u5E76\u901A\u77E5\u8BA2\u9605\u8005: ${name}`);
      notifySubscribers(name);
      return;
    }
    currentTheme = name;
    const vars = resolveVariables(name);
    const nextKeys = Object.keys(vars);
    for (const key of appliedKeys) {
      rootStyle.removeProperty(key);
    }
    for (const key of nextKeys) {
      rootStyle.setProperty(key, vars[key]);
    }
    appliedKeys = nextKeys;
    notifySubscribers(name);
  }
  __name(applyTheme, "applyTheme");
  var themes = {
    apply(name) {
      applyTheme(name);
    },
    getCurrent() {
      return currentTheme;
    },
    get(name) {
      return themeRegistry.get(name);
    },
    getThemes() {
      return Array.from(themeRegistry.values());
    },
    register(theme) {
      if (!isPetUITheme(theme)) {
        console.warn("[PetUI themes] register \u5931\u8D25\uFF1A\u4E3B\u9898\u5FC5\u987B\u662F { name: string, variables: Record<string, string> }");
        return;
      }
      themeRegistry.set(theme.name, theme);
      resolvedCache.delete(theme.name);
      declaredCache = null;
      if (theme.name === currentTheme) {
        applyTheme(theme.name);
      }
    },
    subscribe(cb) {
      if (typeof cb !== "function") {
        throw new TypeError("[PetUI themes] subscribe \u9700\u8981\u4E00\u4E2A\u51FD\u6570\u56DE\u8C03");
      }
      subscribers.push(cb);
      let active = true;
      const unsubscribe = /* @__PURE__ */ __name(() => {
        if (!active) return;
        active = false;
        const idx = subscribers.indexOf(cb);
        if (idx >= 0) subscribers.splice(idx, 1);
      }, "unsubscribe");
      try {
        cb(makeSnapshot(currentTheme));
      } catch (err) {
        console.error(`[PetUI themes] \u4E3B\u9898\u8BA2\u9605\u56DE\u8C03\u629B\u9519\uFF08\u56DE\u653E\uFF0C\u4E3B\u9898 "${currentTheme}"\uFF09\uFF0C\u5DF2\u9694\u79BB\u8BE5\u8BA2\u9605\u8005:`, err);
      }
      return unsubscribe;
    },
    snapshot() {
      return makeSnapshot(currentTheme);
    },
    appliedSnapshot() {
      const base = makeSnapshot(currentTheme);
      const themeKeys = Object.freeze(appliedKeys.slice());
      const themeIndependentKeys = Object.freeze(
        appliedKeys.filter((k) => Object.prototype.hasOwnProperty.call(THEME_INDEPENDENT_VARIABLES, k))
      );
      return Object.freeze({
        name: base.name,
        variables: base.variables,
        themeKeys,
        themeIndependentKeys
      });
    },
    declaredTokenNames() {
      if (declaredCache) return declaredCache;
      const names = /* @__PURE__ */ new Set();
      for (const theme of themeRegistry.values()) {
        for (const key of Object.keys(theme.variables)) names.add(key);
      }
      for (const key of Object.keys(THEME_INDEPENDENT_VARIABLES)) names.add(key);
      for (const key of Object.keys(PRIMITIVE_COLORS)) names.add(key);
      declaredCache = Object.freeze(Array.from(names).sort());
      return declaredCache;
    },
    tierOf(name) {
      const family = familyOf(name);
      return family === null ? null : TOKEN_TIERS[family];
    }
  };

  // src/core/PetUI.ts
  (function() {
    const bannerVars = themes.snapshot().variables;
    const bannerVal = /* @__PURE__ */ __name(function(token) {
      const v = bannerVars[token];
      return typeof v === "string" ? v : "";
    }, "bannerVal");
    const accent = bannerVal("--color-n-ffb6c1");
    console.log(
      "%c PetUI %c \u7F51\u9875UI\u6846\u67B6\n %c\u4F5C\u8005: RuiCat",
      'font-family: "Helvetica Neue",Helvetica, Arial, sans-serif;font-size:' + bannerVal("--space-64") + ";color:" + accent + ";-webkit-text-fill-color:" + accent + ";-webkit-text-stroke: " + bannerVal("--space-1") + " " + accent + ";",
      "font-size:" + bannerVal("--space-25") + ";color:" + accent + ";",
      "font-size:" + bannerVal("--space-15") + ";color:" + accent + ";"
    );
  })();
  var PetUI = (function() {
    PetUIExpand.SetOnload(window);
    (function() {
      const touchHandler = /* @__PURE__ */ __name(function(event) {
        if (!event.touches || !event.changedTouches) return;
        const touch = event.changedTouches[0];
        touch.buttons = 1;
        const target = touch.target;
        if (!target || !target.TheMobileEventTakesEffect) return;
        switch (event.type) {
          case "touchstart":
            target.MobileEndEventPassing.Click(touch);
            break;
          case "touchend":
            target.MobileEndEventPassing.Mouseup(touch);
            break;
          case "touchmove":
            if (event.cancelable && !event.defaultPrevented) event.preventDefault();
            target.MobileEndEventPassing.Mousemove(touch);
            break;
        }
      }, "touchHandler");
      if (document.addEventListener) {
        document.addEventListener("touchstart", touchHandler, false);
        document.addEventListener("touchmove", touchHandler, false);
        document.addEventListener("touchend", touchHandler, false);
      } else if (document.attachEvent) {
        document.attachEvent("ontouchstart", touchHandler);
        document.attachEvent("ontouchmove", touchHandler);
        document.attachEvent("ontouchend", touchHandler);
      } else {
        document.ontouchstart = touchHandler;
        document.ontouchmove = touchHandler;
        document.ontouchend = touchHandler;
      }
    })();
    $.Position = (function() {
      const pos = {
        Affair: void 0,
        Scroll: { X: 0, Y: 0 },
        Win: { Width: 0, Height: 0 },
        WinSkewing: { top: 0, left: 0, Width: 0, Height: 0 }
      };
      function update() {
        if (window.scrollY !== void 0) {
          pos.Scroll.Y = window.scrollY;
          pos.Scroll.X = window.scrollX;
        } else if (document.documentElement && document.documentElement.scrollTop) {
          pos.Scroll.Y = document.documentElement.scrollTop;
          pos.Scroll.X = document.documentElement.scrollLeft;
        } else if (document.body) {
          pos.Scroll.Y = document.body.scrollTop;
          pos.Scroll.X = document.body.scrollLeft;
        }
        if (window.innerWidth) pos.Win.Width = window.innerWidth;
        else if (document.body && document.body.clientWidth) pos.Win.Width = document.body.clientWidth;
        if (window.innerHeight) pos.Win.Height = window.innerHeight;
        else if (document.body && document.body.clientHeight) pos.Win.Height = document.body.clientHeight;
        if (document.documentElement && document.documentElement.clientWidth && document.documentElement.clientHeight) {
          pos.Win.Width = document.documentElement.clientWidth;
          pos.Win.Height = document.documentElement.clientHeight;
        }
        pos.Win.Width += pos.WinSkewing.Width;
        pos.Win.Height += pos.WinSkewing.Height;
        if (typeof pos.Affair === "function") pos.Affair();
      }
      __name(update, "update");
      update();
      window.addEventListener("load", update);
      window.addEventListener("scroll", update);
      window.addEventListener("resize", update);
      pos.AddAffair = PetUIExpand.SetChangeEvent(pos, "Affair");
      return pos;
    })();
    $.Mousemove = (function() {
      const ele = { Mousemove: null };
      const eventWrapper = /* @__PURE__ */ __name(function(e) {
        e = e || window.event;
        e.buttons = 1;
        if (e.changedTouches) {
          e.clientX = e.changedTouches[0].pageX;
          e.clientY = e.changedTouches[0].pageY;
        }
        return e;
      }, "eventWrapper");
      if (PetUIExpand.IsPC()) {
        $.AddHandler.call(document, "mousemove", (e) => ele.Mousemove(e));
      } else {
        $.AddHandler.call(document, "touchmove", (e) => ele.Mousemove(eventWrapper(e)), { passive: true });
      }
      ele.Mousemove = PetUIExpand.SetChangeEvent(ele, "Mousemove");
      return ele.Mousemove;
    })();
    const publicFunc = /* @__PURE__ */ __name(function(...args) {
      try {
        return build.call(void 0, ...args);
      } catch (err) {
        console.warn("PetUI\n\u9519\u8BEF\u7C7B\u578B:", err);
        return null;
      }
    }, "publicFunc");
    return new Proxy(publicFunc, {
      getPrototypeOf: /* @__PURE__ */ __name(() => $, "getPrototypeOf"),
      set: /* @__PURE__ */ __name((_target, _prop, _value, _receiver) => true, "set"),
      get: /* @__PURE__ */ __name((_target, prop) => {
        if (prop === "Style") return StyleProcessor;
        return $[prop];
      }, "get")
    });
  })();
  window.PetUIExpand = PetUIExpand;
  window.PetUI = PetUI;
  if (typeof document !== "undefined") {
    if (document.readyState === "loading") {
      document.addEventListener("DOMContentLoaded", () => {
        StyleProcessor.loadCSSVariables();
      });
    } else {
      StyleProcessor.loadCSSVariables();
    }
  }
  if (typeof StyleProcessor !== "undefined") {
    StyleProcessor.registerVariable("$primary", "var(--color-primary)");
    StyleProcessor.registerVariable("$success", "var(--color-success)");
    StyleProcessor.registerVariable("$warning", "var(--color-warning)");
    StyleProcessor.registerVariable("$danger", "var(--color-danger)");
    StyleProcessor.registerVariable("$info", "var(--color-info)");
    StyleProcessor.registerVariable("$radius-sm", "var(--border-radius-sm)");
    StyleProcessor.registerVariable("$radius-md", "var(--border-radius-md)");
    StyleProcessor.registerVariable("$radius-lg", "var(--border-radius-lg)");
    StyleProcessor.registerVariable("$shadow-light", "var(--shadow-light)");
    StyleProcessor.registerVariable("$shadow-medium", "var(--shadow-medium)");
    StyleProcessor.registerVariable("$spacing-sm", "var(--spacing-sm)");
    StyleProcessor.registerVariable("$spacing-md", "var(--spacing-md)");
    StyleProcessor.registerVariable("$spacing-lg", "var(--spacing-lg)");
  }

  // src/core/reactivity/scheduler.ts
  var _Scheduler = class _Scheduler {
    constructor() {
      this._queue = [];
      this._flushing = false;
    }
    /** 将 effect 加入队列 */
    push(effect2) {
      if (!this._queue.includes(effect2)) {
        this._queue.push(effect2);
      }
      if (!this._flushing) {
        this._flushing = true;
        Promise.resolve().then(() => this._flush());
      }
    }
    /** 批量执行队列中的所有 effect（防御性执行） */
    _flush() {
      try {
        let guard = 0;
        do {
          const batch = this._queue.slice();
          this._queue.length = 0;
          for (const e of batch) {
            try {
              e.execute();
            } catch (err) {
              console.error("[PetUI] effect \u6267\u884C\u51FA\u9519:", err);
            }
          }
          if (++guard > 1e3) {
            console.warn("[PetUI] \u8C03\u5EA6\u5FAA\u73AF\u8D85\u8FC7 1000 \u6B21\uFF0C\u7EC8\u6B62\u672C\u6B21 flush");
            this._queue.length = 0;
            break;
          }
        } while (this._queue.length > 0);
      } finally {
        this._flushing = false;
      }
    }
  };
  __name(_Scheduler, "Scheduler");
  var Scheduler = _Scheduler;
  var scheduler = new Scheduler();

  // src/core/reactivity/effect.ts
  var activeEffect;
  var _Effect = class _Effect {
    constructor(fn, options = {}) {
      this._deps = [];
      this._scheduled = false;
      this._active = true;
      this._fn = fn;
      this._lazy = options.lazy ?? false;
      this._scheduler = options.scheduler;
      if (!this._lazy) {
        this.run();
      }
    }
    /** 清理旧依赖 */
    _cleanup() {
      for (const deps of this._deps) {
        deps.delete(this);
      }
      this._deps.length = 0;
    }
    /** 执行函数并重新收集依赖 */
    run() {
      if (!this._active) return;
      this._cleanup();
      const prev = activeEffect;
      activeEffect = this;
      try {
        this._fn();
      } finally {
        activeEffect = prev;
      }
    }
    /** 调度执行（加入异步更新队列） */
    schedule() {
      if (this._scheduler) {
        this._scheduler();
      } else {
        if (!this._scheduled) {
          this._scheduled = true;
          scheduler.push(this);
        }
      }
    }
    /** 实际执行（由调度器调用，重置 scheduled 标记后运行） */
    execute() {
      this._scheduled = false;
      this.run();
    }
    /** 停止 effect，清除所有依赖 */
    stop() {
      if (this._active) {
        this._cleanup();
        this._active = false;
      }
    }
  };
  __name(_Effect, "Effect");
  var Effect = _Effect;
  function effect(fn) {
    return new Effect(fn);
  }
  __name(effect, "effect");

  // src/core/reactivity/dep.ts
  var targetMap = /* @__PURE__ */ new WeakMap();
  function track(target, key) {
    if (!activeEffect) return;
    let depsMap = targetMap.get(target);
    if (!depsMap) {
      targetMap.set(target, depsMap = /* @__PURE__ */ new Map());
    }
    let deps = depsMap.get(key);
    if (!deps) {
      depsMap.set(key, deps = /* @__PURE__ */ new Set());
    }
    if (!deps.has(activeEffect)) {
      deps.add(activeEffect);
      activeEffect._deps.push(deps);
    }
  }
  __name(track, "track");
  function trigger(target, key) {
    const depsMap = targetMap.get(target);
    if (!depsMap) return;
    const deps = depsMap.get(key);
    if (deps) {
      deps.forEach((effect2) => effect2.schedule());
    }
    const iterateDeps = depsMap.get(/* @__PURE__ */ Symbol.for("iterate"));
    if (iterateDeps) {
      iterateDeps.forEach((effect2) => effect2.schedule());
    }
  }
  __name(trigger, "trigger");

  // src/core/reactivity/reactive.ts
  var reactiveMap = /* @__PURE__ */ new WeakMap();
  var RAW = /* @__PURE__ */ Symbol("raw");
  var IS_REACTIVE = /* @__PURE__ */ Symbol("isReactive");
  var IS_READONLY = /* @__PURE__ */ Symbol("isReadonly");
  function shouldProxy(value) {
    if (value === null || typeof value !== "object") return false;
    if (Array.isArray(value)) return true;
    const proto = Object.getPrototypeOf(value);
    return proto === Object.prototype || proto === null;
  }
  __name(shouldProxy, "shouldProxy");
  var reactiveHandlers = {
    get(target, key, receiver) {
      if (key === RAW) return target;
      if (key === IS_REACTIVE) return true;
      const result = Reflect.get(target, key, receiver);
      track(target, key);
      if (shouldProxy(result)) {
        return reactive(result);
      }
      return result;
    },
    set(target, key, value, receiver) {
      const oldValue = target[key];
      const hadKey = Object.prototype.hasOwnProperty.call(target, key);
      const result = Reflect.set(target, key, value, receiver);
      if (!hadKey) {
        trigger(target, key);
      } else if (oldValue !== value) {
        trigger(target, key);
      }
      return result;
    },
    deleteProperty(target, key) {
      const had = Object.prototype.hasOwnProperty.call(target, key);
      const result = Reflect.deleteProperty(target, key);
      if (had) {
        trigger(target, key);
      }
      return result;
    },
    has(target, key) {
      track(target, key);
      return Reflect.has(target, key);
    },
    ownKeys(target) {
      track(target, /* @__PURE__ */ Symbol.for("iterate"));
      return Reflect.ownKeys(target);
    }
  };
  var shallowReactiveHandlers = {
    get(target, key, receiver) {
      if (key === RAW) return target;
      if (key === IS_REACTIVE) return true;
      const result = Reflect.get(target, key, receiver);
      track(target, key);
      return result;
    },
    set: reactiveHandlers.set,
    deleteProperty: reactiveHandlers.deleteProperty,
    has: reactiveHandlers.has,
    ownKeys: reactiveHandlers.ownKeys
  };
  function reactive(target) {
    if (target[IS_REACTIVE] || target[IS_READONLY]) return target;
    const existing = reactiveMap.get(target);
    if (existing) return existing;
    const proxy = new Proxy(target, reactiveHandlers);
    reactiveMap.set(target, proxy);
    return proxy;
  }
  __name(reactive, "reactive");

  // src/core/reactivity/ref.ts
  function toReactive(value) {
    if (shouldProxy(value)) {
      return reactive(value);
    }
    return value;
  }
  __name(toReactive, "toReactive");
  var _RefImpl = class _RefImpl {
    constructor(value) {
      this.__v_isRef = true;
      this._value = toReactive(value);
    }
    get value() {
      track(this, "value");
      return this._value;
    }
    set value(newValue) {
      if (!Object.is(newValue, this._value)) {
        this._value = toReactive(newValue);
        trigger(this, "value");
      }
    }
  };
  __name(_RefImpl, "RefImpl");
  var RefImpl = _RefImpl;
  function ref(value) {
    return new RefImpl(value);
  }
  __name(ref, "ref");
  function isRef(value) {
    return !!(value && value.__v_isRef);
  }
  __name(isRef, "isRef");
  var _ComputedImpl = class _ComputedImpl {
    constructor(getter) {
      this.__v_isRef = true;
      this._dirty = true;
      this._effect = new Effect(() => {
        this._value = getter();
      }, {
        lazy: true,
        scheduler: /* @__PURE__ */ __name(() => {
          this._dirty = true;
          trigger(this, "value");
        }, "scheduler")
      });
    }
    get value() {
      track(this, "value");
      if (this._dirty) {
        this._effect.run();
        this._dirty = false;
      }
      return this._value;
    }
  };
  __name(_ComputedImpl, "ComputedImpl");
  var ComputedImpl = _ComputedImpl;
  function computed(getter) {
    return new ComputedImpl(getter);
  }
  __name(computed, "computed");

  // src/core/compiler/parser.ts
  var VOID_ELEMENTS = /* @__PURE__ */ new Set([
    "area",
    "base",
    "br",
    "col",
    "embed",
    "hr",
    "img",
    "input",
    "link",
    "meta",
    "source",
    "track",
    "wbr"
  ]);
  var BOOLEAN_ATTRS = /* @__PURE__ */ new Set([
    "disabled",
    "checked",
    "selected",
    "hidden",
    "readonly",
    "required",
    "multiple",
    "autofocus",
    "autoplay",
    "controls",
    "loop",
    "muted",
    "novalidate",
    "open",
    "reversed"
  ]);
  function findInterpolationEnd(template, startIdx) {
    let depth = 0;
    let i = startIdx;
    const n = template.length;
    while (i < n) {
      const c = template[i];
      if (c === "'" || c === '"' || c === "`") {
        const quote = c;
        i++;
        while (i < n) {
          const cc = template[i];
          i++;
          if (cc === "\\" && i < n) {
            i++;
            continue;
          }
          if (cc === quote) break;
        }
        continue;
      }
      if (c === "{") {
        depth++;
      } else if (c === "}") {
        if (depth === 0 && template[i + 1] === "}") {
          return i;
        }
        if (depth > 0) depth--;
      }
      i++;
    }
    return -1;
  }
  __name(findInterpolationEnd, "findInterpolationEnd");
  function parse(template) {
    const nodes = [];
    let i = 0;
    const len = template.length;
    while (i < len) {
      const nextLt = template.indexOf("<", i);
      const nextInterp = template.indexOf("{{", i);
      if (nextLt === -1 && nextInterp === -1) {
        const text = template.slice(i);
        if (text.trim()) {
          nodes.push({ type: "text", content: text });
        }
        break;
      }
      let nextPos;
      let isInterpolation;
      if (nextLt === -1) {
        nextPos = nextInterp;
        isInterpolation = true;
      } else if (nextInterp === -1) {
        nextPos = nextLt;
        isInterpolation = false;
      } else {
        isInterpolation = nextInterp < nextLt;
        nextPos = isInterpolation ? nextInterp : nextLt;
      }
      if (nextPos > i) {
        const text = template.slice(i, nextPos);
        if (text.trim()) {
          nodes.push({ type: "text", content: text });
        }
      }
      if (isInterpolation) {
        const closePos = findInterpolationEnd(template, nextPos + 2);
        if (closePos === -1) {
          console.warn("[PetUI] \u672A\u95ED\u5408\u7684\u63D2\u503C\u8868\u8FBE\u5F0F\uFF0C\u4F4D\u7F6E:", nextPos);
          nodes.push({ type: "text", content: template.slice(nextPos) });
          break;
        }
        const expression = template.slice(nextPos + 2, closePos).trim();
        nodes.push({ type: "interpolation", expression });
        i = closePos + 2;
      } else {
        i = nextPos;
        if (template[i + 1] === "/") {
          break;
        }
        if (template.slice(i, i + 4) === "<!--") {
          const commentEnd = template.indexOf("-->", i + 4);
          if (commentEnd === -1) {
            i = len;
          } else {
            i = commentEnd + 3;
          }
          continue;
        }
        if (template.slice(i, i + 9).toLowerCase() === "<!doctype") {
          const gtPos = template.indexOf(">", i);
          i = gtPos === -1 ? len : gtPos + 1;
          continue;
        }
        if (template.slice(i, i + 9) === "<![CDATA[") {
          const cdataEnd = template.indexOf("]]>", i + 9);
          if (cdataEnd !== -1) {
            const content = template.slice(i + 9, cdataEnd);
            if (content.trim()) {
              nodes.push({ type: "text", content });
            }
            i = cdataEnd + 3;
          } else {
            i = len;
          }
          continue;
        }
        const result = parseOpenTag(template, i);
        if (!result) {
          nodes.push({ type: "text", content: "<" });
          i++;
          continue;
        }
        const { node, nextIndex, selfClosing } = result;
        i = nextIndex;
        if (selfClosing || VOID_ELEMENTS.has(node.tag.toLowerCase())) {
          node.selfClosing = true;
          nodes.push(node);
          continue;
        }
        const children = parseChildren(template, i, node.tag);
        node.children = children.nodes;
        i = children.nextIndex;
        nodes.push(node);
      }
    }
    return nodes;
  }
  __name(parse, "parse");
  function parseChildren(template, startIndex, parentTag) {
    const nodes = [];
    let i = startIndex;
    const len = template.length;
    while (i < len) {
      const ltPos = template.indexOf("<", i);
      const interpPos = template.indexOf("{{", i);
      if (ltPos === -1 && interpPos === -1) {
        const text = template.slice(i);
        if (text.trim()) {
          nodes.push({ type: "text", content: text });
        }
        console.warn("[PetUI] \u672A\u95ED\u5408\u7684\u6807\u7B7E:", parentTag);
        return { nodes, nextIndex: len };
      }
      let nextPos;
      let isInterp;
      if (ltPos === -1) {
        nextPos = interpPos;
        isInterp = true;
      } else if (interpPos === -1) {
        nextPos = ltPos;
        isInterp = false;
      } else {
        isInterp = interpPos < ltPos;
        nextPos = isInterp ? interpPos : ltPos;
      }
      if (nextPos > i) {
        const text = template.slice(i, nextPos);
        if (text.trim()) {
          nodes.push({ type: "text", content: text });
        }
      }
      if (isInterp) {
        const closePos = findInterpolationEnd(template, nextPos + 2);
        if (closePos === -1) {
          nodes.push({ type: "text", content: template.slice(nextPos) });
          return { nodes, nextIndex: len };
        }
        const expression = template.slice(nextPos + 2, closePos).trim();
        nodes.push({ type: "interpolation", expression });
        i = closePos + 2;
      } else {
        i = nextPos;
        if (template[i + 1] === "/") {
          const gtPos = template.indexOf(">", i + 2);
          if (gtPos === -1) {
            console.warn("[PetUI] \u672A\u95ED\u5408\u7684\u6807\u7B7E:", parentTag);
            return { nodes, nextIndex: len };
          }
          const closeTag = template.slice(i + 2, gtPos).trim().toLowerCase();
          if (closeTag !== parentTag.toLowerCase()) {
            console.warn("[PetUI] \u4E0D\u5339\u914D\u7684\u95ED\u5408\u6807\u7B7E: \u671F\u671B </" + parentTag + ">, \u5F97\u5230 </" + closeTag + ">");
          }
          return { nodes, nextIndex: gtPos + 1 };
        }
        if (template.slice(i, i + 4) === "<!--") {
          const commentEnd = template.indexOf("-->", i + 4);
          i = commentEnd === -1 ? len : commentEnd + 3;
          continue;
        }
        const result = parseOpenTag(template, i);
        if (!result) {
          nodes.push({ type: "text", content: "<" });
          i++;
          continue;
        }
        const { node, nextIndex, selfClosing } = result;
        i = nextIndex;
        if (selfClosing || VOID_ELEMENTS.has(node.tag.toLowerCase())) {
          node.selfClosing = true;
          nodes.push(node);
          continue;
        }
        const children = parseChildren(template, i, node.tag);
        node.children = children.nodes;
        i = children.nextIndex;
        nodes.push(node);
      }
    }
    console.warn("[PetUI] \u672A\u95ED\u5408\u7684\u6807\u7B7E:", parentTag);
    return { nodes, nextIndex: len };
  }
  __name(parseChildren, "parseChildren");
  function parseOpenTag(template, startIndex) {
    let i = startIndex + 1;
    while (i < template.length && template[i] === " " || template[i] === "	" || template[i] === "\n" || template[i] === "\r") {
      i++;
    }
    const tagStart = i;
    while (i < template.length && /[a-zA-Z0-9\-]/.test(template[i])) {
      i++;
    }
    const tag = template.slice(tagStart, i).toLowerCase();
    if (!tag) {
      return null;
    }
    const attrs = {};
    const directives = [];
    let selfClosing = false;
    while (i < template.length) {
      while (i < template.length && (template[i] === " " || template[i] === "	" || template[i] === "\n" || template[i] === "\r")) {
        i++;
      }
      if (i >= template.length) break;
      if (template[i] === ">") {
        i++;
        break;
      }
      if (template[i] === "/" && template[i + 1] === ">") {
        selfClosing = true;
        i += 2;
        break;
      }
      const attrStart = i;
      while (i < template.length && /[a-zA-Z0-9\-:@.#_\[\]]/.test(template[i])) {
        i++;
      }
      let attrName = template.slice(attrStart, i).toLowerCase();
      if (!attrName) {
        i++;
        continue;
      }
      let attrValue;
      let j = i;
      while (j < template.length && (template[j] === " " || template[j] === "	" || template[j] === "\n" || template[j] === "\r")) {
        j++;
      }
      if (template[j] === "=") {
        i = j + 1;
        while (i < template.length && (template[i] === " " || template[i] === "	" || template[i] === "\n" || template[i] === "\r")) {
          i++;
        }
        if (i >= template.length) break;
        const quote = template[i];
        if (quote === '"' || quote === "'") {
          i++;
          const valueStart = i;
          while (i < template.length && template[i] !== quote) {
            if (template[i] === "\\" && i + 1 < template.length) {
              i++;
            }
            i++;
          }
          attrValue = template.slice(valueStart, i).replace(/\\(.)/g, "$1");
          if (i < template.length) i++;
        } else {
          const valueStart = i;
          while (i < template.length && template[i] !== " " && template[i] !== ">" && template[i] !== "/" && template[i] !== "	" && template[i] !== "\n" && template[i] !== "\r") {
            i++;
          }
          attrValue = template.slice(valueStart, i);
        }
      }
      const dirResult = classifyAttr(attrName, attrValue);
      if (dirResult) {
        directives.push(dirResult);
      } else {
        if (attrValue === void 0 && BOOLEAN_ATTRS.has(attrName)) {
          attrValue = attrName;
        }
        if (attrValue !== void 0) {
          attrs[attrName] = attrValue;
        }
      }
    }
    return {
      node: {
        type: "element",
        tag,
        attrs,
        directives,
        children: [],
        selfClosing
      },
      nextIndex: i,
      selfClosing
    };
  }
  __name(parseOpenTag, "parseOpenTag");
  function classifyAttr(name, value) {
    if (name.startsWith("v-bind:") || name.startsWith("v-bind.")) {
      const colonIdx = name.indexOf(":", 6);
      const arg = name.slice(colonIdx !== -1 ? colonIdx + 1 : 7);
      return { name: "bind", rawName: name, value: value || "", arg };
    }
    if (name.startsWith(":")) {
      return { name: "bind", rawName: name, value: value || "", arg: name.slice(1) };
    }
    if (name.startsWith("v-on:") || name.startsWith("v-on.")) {
      const colonIdx = name.indexOf(":", 4);
      const arg = name.slice(colonIdx !== -1 ? colonIdx + 1 : 5);
      return { name: "on", rawName: name, value: value || "", arg };
    }
    if (name.startsWith("@")) {
      return { name: "on", rawName: name, value: value || "", arg: name.slice(1) };
    }
    if (name === "v-model") {
      return { name: "model", rawName: name, value: value || "" };
    }
    if (name === "v-if") {
      return { name: "if", rawName: name, value: value || "" };
    }
    if (name === "v-else-if") {
      return { name: "else-if", rawName: name, value: value || "" };
    }
    if (name === "v-else") {
      return { name: "else", rawName: name, value: value || "" };
    }
    if (name === "v-for") {
      return { name: "for", rawName: name, value: value || "" };
    }
    if (name.startsWith("v-")) {
      return null;
    }
    return null;
  }
  __name(classifyAttr, "classifyAttr");

  // src/core/compiler/transformer.ts
  function hasDirective(node, name) {
    return node.directives.some((d) => d.name === name);
  }
  __name(hasDirective, "hasDirective");
  function getDirective(node, name) {
    return node.directives.find((d) => d.name === name);
  }
  __name(getDirective, "getDirective");
  function parseForExpression(expr) {
    const inIdx = expr.lastIndexOf(" in ");
    if (inIdx === -1) {
      const inIdxNoSpace = expr.lastIndexOf(" in");
      if (inIdxNoSpace === -1) {
        console.warn("[PetUI] \u65E0\u6548\u7684 v-for \u8868\u8FBE\u5F0F:", expr);
        return { item: "item", source: expr };
      }
      return { item: expr.slice(0, inIdxNoSpace).trim(), source: expr.slice(inIdxNoSpace + 3).trim() };
    }
    const source = expr.slice(inIdx + 4).trim();
    const itemPart = expr.slice(0, inIdx).trim();
    if (itemPart.startsWith("(") && itemPart.endsWith(")")) {
      const inner = itemPart.slice(1, -1);
      const parts = inner.split(",").map((s) => s.trim());
      if (parts.length >= 2) {
        return { item: parts[0], index: parts[1], source };
      }
      return { item: parts[0], source };
    }
    return { item: itemPart, source };
  }
  __name(parseForExpression, "parseForExpression");
  function transformElement(node) {
    const staticAttrs = {};
    const dynamicAttrs = [];
    const events = [];
    let model;
    for (const key in node.attrs) {
      if (Object.prototype.hasOwnProperty.call(node.attrs, key)) {
        staticAttrs[key] = node.attrs[key];
      }
    }
    for (const dir of node.directives) {
      switch (dir.name) {
        case "bind":
          if (dir.arg) {
            dynamicAttrs.push({ name: dir.arg, value: dir.value });
          }
          break;
        case "on":
          if (dir.arg) {
            events.push({ event: dir.arg, handler: dir.value });
          }
          break;
        case "model":
          model = { expression: dir.value };
          const modelExpr = dir.value.trim();
          const isPathExpr = /^[a-zA-Z_$][\w$]*(\.[a-zA-Z_$][\w$]*)*$/.test(modelExpr);
          if (!isPathExpr) {
            console.warn('[PetUI] v-model \u4EC5\u652F\u6301\u7B80\u5355\u6807\u8BC6\u7B26\u6216\u70B9\u8DEF\u5F84\uFF0C\u6536\u5230: "' + dir.value + '"');
            break;
          }
          if (node.tag === "input") {
            const type = (node.attrs["type"] || "text").toLowerCase();
            if (type === "checkbox" || type === "radio") {
              dynamicAttrs.push({ name: "checked", value: dir.value });
              events.push({ event: "change", handler: "(" + modelExpr + ") = $event.target.checked" });
            } else {
              dynamicAttrs.push({ name: "value", value: dir.value });
              events.push({ event: "input", handler: "(" + modelExpr + ") = $event.target.value" });
            }
          } else if (node.tag === "textarea") {
            dynamicAttrs.push({ name: "value", value: dir.value });
            events.push({ event: "input", handler: "(" + modelExpr + ") = $event.target.value" });
          } else if (node.tag === "select") {
            dynamicAttrs.push({ name: "value", value: dir.value });
            events.push({ event: "change", handler: "(" + modelExpr + ") = $event.target.value" });
          }
          break;
        case "if":
        case "else-if":
        case "else":
        case "for":
          break;
        default:
          break;
      }
    }
    const transformedChildren = transform(node.children);
    return {
      type: "element",
      tag: node.tag,
      staticAttrs,
      dynamicAttrs,
      events,
      model,
      children: transformedChildren,
      selfClosing: node.selfClosing
    };
  }
  __name(transformElement, "transformElement");
  function isElement(node) {
    return node.type === "element";
  }
  __name(isElement, "isElement");
  function transform(nodes) {
    const result = [];
    let i = 0;
    while (i < nodes.length) {
      const node = nodes[i];
      if (!isElement(node)) {
        result.push(node);
        i++;
        continue;
      }
      if (hasDirective(node, "if")) {
        const conditions = [];
        while (i < nodes.length && isElement(nodes[i])) {
          const el = nodes[i];
          const ifDir = getDirective(el, "if");
          const elseIfDir = getDirective(el, "else-if");
          const elseDir = getDirective(el, "else");
          if (ifDir) {
            conditions.push({
              test: ifDir.value,
              children: transform(el.children)
            });
          } else if (elseIfDir) {
            conditions.push({
              test: elseIfDir.value,
              children: transform(el.children)
            });
          } else if (elseDir) {
            conditions.push({
              test: void 0,
              children: transform(el.children)
            });
            i++;
            break;
          } else {
            break;
          }
          i++;
        }
        result.push({ type: "if", conditions });
        continue;
      }
      if (hasDirective(node, "for")) {
        const dir = getDirective(node, "for");
        const parsed = parseForExpression(dir.value);
        const withoutFor = {
          ...node,
          directives: node.directives.filter((d) => d.name !== "for")
        };
        result.push({
          type: "for",
          item: parsed.item,
          index: parsed.index,
          source: parsed.source,
          children: transform([withoutFor])
        });
        i++;
        continue;
      }
      result.push(transformElement(node));
      i++;
    }
    return result;
  }
  __name(transform, "transform");

  // src/core/compiler/codegen.ts
  var EVENT_MAP = {
    "click": "Click",
    "dblclick": "Dblclick",
    "mousedown": "Mousedown",
    "mouseup": "Mouseup",
    "mousemove": "Mousemove",
    "mouseout": "Mouseout",
    "mouseover": "Mouseover",
    "mouseenter": "Mouseenter",
    "mouseleave": "Mouseleave",
    "keydown": "Keydown",
    "keyup": "Keyup",
    "keypress": "Keypress",
    "focus": "Focus",
    "blur": "Blur",
    "submit": "Submit",
    "scroll": "Scroll",
    "resize": "Resize",
    "contextmenu": "Contextmenu",
    "wheel": "Wheel"
  };
  var IDENT_START_RE = /[A-Za-z_$]/;
  var IDENT_CHAR_RE = /[A-Za-z0-9_$]/;
  var RESERVED = /* @__PURE__ */ new Set([
    "$event",
    "$set",
    "true",
    "false",
    "null",
    "undefined",
    "this",
    "typeof",
    "in",
    "of",
    "new",
    "delete",
    "void",
    "return",
    "function",
    "var",
    "let",
    "const",
    "if",
    "else",
    "for",
    "while",
    "do",
    "switch",
    "case",
    "break",
    "continue",
    "instanceof",
    "throw",
    "try",
    "catch",
    "finally",
    "yield",
    "await",
    "async",
    "class",
    "extends",
    "super",
    "import",
    "export",
    "default",
    "static",
    "get",
    "set"
  ]);
  function genAttrsString(staticAttrs, dynamicAttrs, events, indent) {
    const parts = [];
    const pad3 = "  ".repeat(indent + 1);
    for (const key in staticAttrs) {
      if (Object.prototype.hasOwnProperty.call(staticAttrs, key)) {
        const val = staticAttrs[key];
        const mappedKey = key === "class" ? "className" : key;
        parts.push(pad3 + mappedKey + ": " + JSON.stringify(val));
      }
    }
    for (const da of dynamicAttrs) {
      const mappedKey = da.name === "class" ? "className" : da.name;
      const expr = formatExpression(da.value);
      parts.push(pad3 + mappedKey + ": " + expr);
    }
    for (const ev of events) {
      const expr = formatExpression(ev.handler);
      const mapped = EVENT_MAP[ev.event.toLowerCase()];
      if (mapped) {
        parts.push(pad3 + mapped + ": " + expr);
      } else {
        parts.push(pad3 + "AddHandler: function(el) { el.AddHandler('" + ev.event + "', function($event) { " + expr + "; }); }");
      }
    }
    if (parts.length === 0) {
      return "{}";
    }
    return "{\n" + parts.join(",\n") + "\n" + "  ".repeat(indent) + "}";
  }
  __name(genAttrsString, "genAttrsString");
  function splitTopLevelPos(s) {
    const parts = [];
    let depth = 0;
    let cur = "";
    let curStart = -1;
    let i = 0;
    const n = s.length;
    const flush = /* @__PURE__ */ __name((endIdx) => {
      if (cur.trim()) parts.push({ text: cur, start: curStart, end: endIdx });
      cur = "";
      curStart = -1;
    }, "flush");
    while (i < n) {
      const c = s[i];
      if (c === "'" || c === '"' || c === "`") {
        if (curStart === -1) curStart = i;
        const quote = c;
        cur += c;
        i++;
        while (i < n) {
          const cc = s[i];
          cur += cc;
          i++;
          if (cc === "\\" && i < n) {
            cur += s[i];
            i++;
            continue;
          }
          if (cc === quote) break;
        }
        continue;
      }
      if (c === "(" || c === "{" || c === "[") depth++;
      else if (c === ")" || c === "}" || c === "]") depth--;
      if (c === "," && depth === 0) {
        flush(i);
      } else {
        if (curStart === -1 && !/\s/.test(c)) curStart = i;
        cur += c;
      }
      i++;
    }
    flush(n);
    return parts;
  }
  __name(splitTopLevelPos, "splitTopLevelPos");
  function splitTopLevel(s) {
    return splitTopLevelPos(s).map((p) => p.text);
  }
  __name(splitTopLevel, "splitTopLevel");
  function extractParamIdentifiers(p) {
    const t = p.trim();
    if (!t) return [];
    if (/^[a-zA-Z_$][a-zA-Z0-9_$]*$/.test(t)) return [t];
    if (t.startsWith("{") || t.startsWith("[")) return extractDestructuredIdentifiers(t);
    let topColon = -1;
    let topEq = -1;
    let depth = 0;
    for (let k = 0; k < t.length; k++) {
      const c = t[k];
      if (c === "(" || c === "{" || c === "[") depth++;
      else if (c === ")" || c === "}" || c === "]") depth--;
      else if (depth === 0) {
        if (c === ":" && topColon === -1) topColon = k;
        else if (c === "=" && topEq === -1) topEq = k;
      }
    }
    let target = t;
    if (topColon !== -1) target = t.slice(topColon + 1);
    const eq = target.indexOf("=");
    if (eq !== -1) target = target.slice(0, eq);
    target = target.trim();
    if (/^[a-zA-Z_$][a-zA-Z0-9_$]*$/.test(target)) return [target];
    if ((target.startsWith("{") || target.startsWith("[")) && target.length >= 2) {
      return extractDestructuredIdentifiers(target);
    }
    return [];
  }
  __name(extractParamIdentifiers, "extractParamIdentifiers");
  function extractDestructuredIdentifiers(pattern) {
    const result = [];
    const inner = pattern.slice(1, -1);
    for (const item of splitTopLevel(inner)) {
      result.push(...extractParamIdentifiers(item));
    }
    return result;
  }
  __name(extractDestructuredIdentifiers, "extractDestructuredIdentifiers");
  function collectArrowScopes(expr) {
    const scopes = [];
    const skipRanges = [];
    let i = 0;
    let openDepth = 0;
    const n = expr.length;
    while (i < n) {
      const ch = expr[i];
      if (ch === "'" || ch === '"') {
        const quote = ch;
        i++;
        while (i < n) {
          const c = expr[i];
          i++;
          if (c === "\\" && i < n) {
            i++;
            continue;
          }
          if (c === quote) break;
        }
        continue;
      }
      if (ch === "`") {
        i++;
        while (i < n) {
          const c = expr[i];
          i++;
          if (c === "\\" && i < n) {
            i++;
            continue;
          }
          if (c === "`") break;
        }
        continue;
      }
      if (ch === "(" || ch === "[" || ch === "{") {
        openDepth++;
        i++;
        continue;
      }
      if (ch === ")" || ch === "]" || ch === "}") {
        if (openDepth > 0) openDepth--;
        i++;
        continue;
      }
      if (ch === "=" && expr[i + 1] === ">") {
        const params = /* @__PURE__ */ new Set();
        let j = i - 1;
        while (j >= 0 && /\s/.test(expr[j])) j--;
        if (j >= 0 && expr[j] === ")") {
          let depth = 1, k = j - 1;
          while (k >= 0 && depth > 0) {
            if (expr[k] === ")") depth++;
            else if (expr[k] === "(") depth--;
            k--;
          }
          const list = expr.slice(k + 2, j);
          const listStart = k + 2;
          for (const item of splitTopLevelPos(list)) {
            const p = item.text.trim();
            if (!p) continue;
            const partStart = listStart + item.start;
            const partEnd = listStart + item.end;
            skipRanges.push({ start: partStart, end: partEnd });
            for (const id of extractParamIdentifiers(p)) params.add(id);
          }
        } else if (j >= 0 && /[a-zA-Z0-9_$]/.test(expr[j])) {
          let k = j;
          while (k >= 0 && /[a-zA-Z0-9_$]/.test(expr[k])) k--;
          const ident = expr.slice(k + 1, j + 1);
          if (/^[a-zA-Z_$][a-zA-Z0-9_$]*$/.test(ident)) {
            params.add(ident);
            skipRanges.push({ start: k + 1, end: j + 1 });
          }
        }
        let b = i + 2;
        while (b < n && /\s/.test(expr[b])) b++;
        let end = -1;
        if (b < n && expr[b] === "{") {
          let depth = 0;
          let p = b;
          while (p < n) {
            const c = expr[p];
            if (c === "'" || c === '"') {
              const quote = c;
              p++;
              while (p < n) {
                const cc = expr[p];
                p++;
                if (cc === "\\" && p < n) {
                  p++;
                  continue;
                }
                if (cc === quote) break;
              }
              continue;
            }
            if (c === "`") {
              p++;
              while (p < n) {
                const cc = expr[p];
                p++;
                if (cc === "\\" && p < n) {
                  p++;
                  continue;
                }
                if (cc === "`") break;
              }
              continue;
            }
            if (c === "{") depth++;
            else if (c === "}") {
              depth--;
              if (depth === 0) {
                end = p + 1;
                break;
              }
            }
            p++;
          }
        } else {
          let depth = openDepth;
          let p = b;
          while (p < n) {
            const c = expr[p];
            if (c === "'" || c === '"') {
              const quote = c;
              p++;
              while (p < n) {
                const cc = expr[p];
                p++;
                if (cc === "\\" && p < n) {
                  p++;
                  continue;
                }
                if (cc === quote) break;
              }
              continue;
            }
            if (c === "`") {
              p++;
              while (p < n) {
                const cc = expr[p];
                p++;
                if (cc === "\\" && p < n) {
                  p++;
                  continue;
                }
                if (cc === "`") break;
              }
              continue;
            }
            if (c === "(" || c === "[" || c === "{") {
              depth++;
            } else if (c === ")" || c === "]" || c === "}") {
              if (depth === 0) {
                end = p;
                break;
              }
              depth--;
            } else if (c === "," || c === ";") {
              if (depth === 0) {
                end = p;
                break;
              }
            }
            p++;
          }
          if (end === -1) end = n;
        }
        if (params.size > 0 && end > b) {
          scopes.push({ start: b, end, params });
        }
        i += 2;
        continue;
      }
      i++;
    }
    return { scopes, skipRanges };
  }
  __name(collectArrowScopes, "collectArrowScopes");
  function formatExpression(expr) {
    const trimmed = expr.trim();
    if (/^[a-zA-Z_$][a-zA-Z0-9_$]*$/.test(trimmed)) {
      if (RESERVED.has(trimmed)) return trimmed;
      return "ctx." + trimmed;
    }
    const { scopes, skipRanges } = collectArrowScopes(trimmed);
    const sortedScopes = scopes.sort((a, b) => a.start - b.start);
    const activeScopes = [];
    let scopePtr = 0;
    let ri = 0;
    let out = "";
    let i = 0;
    const n = trimmed.length;
    while (i < n) {
      const ch = trimmed[i];
      if (ch === "'" || ch === '"') {
        const quote = ch;
        out += ch;
        i++;
        while (i < n) {
          const c = trimmed[i];
          out += c;
          i++;
          if (c === "\\" && i < n) {
            out += trimmed[i];
            i++;
            continue;
          }
          if (c === quote) break;
        }
        continue;
      }
      if (ch === "`") {
        out += ch;
        i++;
        while (i < n) {
          const c = trimmed[i];
          out += c;
          i++;
          if (c === "\\" && i < n) {
            out += trimmed[i];
            i++;
            continue;
          }
          if (c === "`") break;
        }
        continue;
      }
      if (/\d/.test(ch)) {
        out += ch;
        i++;
        while (i < n && /[\d.a-zA-Z_%]/.test(trimmed[i])) {
          out += trimmed[i];
          i++;
        }
        continue;
      }
      if (IDENT_START_RE.test(ch)) {
        let j = i;
        while (j > 0 && /\s/.test(trimmed[j - 1])) j--;
        const prev = j > 0 ? trimmed[j - 1] : "";
        let k = i + 1;
        while (k < n && IDENT_CHAR_RE.test(trimmed[k])) k++;
        const ident = trimmed.slice(i, k);
        const isPropertyAccess = prev === ".";
        while (activeScopes.length && activeScopes[activeScopes.length - 1].end <= i) activeScopes.pop();
        while (scopePtr < sortedScopes.length && sortedScopes[scopePtr].start <= i) {
          const sc = sortedScopes[scopePtr++];
          if (!activeScopes.some((a) => a.start === sc.start)) activeScopes.push(sc);
        }
        const inScope = activeScopes.some((s) => s.params.has(ident));
        while (ri < skipRanges.length && skipRanges[ri].end <= i) ri++;
        const inSkipRange = ri < skipRanges.length && i >= skipRanges[ri].start && i < skipRanges[ri].end;
        if (isPropertyAccess || RESERVED.has(ident) || inScope || inSkipRange) {
          out += ident;
        } else {
          out += "ctx." + ident;
        }
        i = k;
        continue;
      }
      out += ch;
      i++;
    }
    return out;
  }
  __name(formatExpression, "formatExpression");
  function genNode(node, indent) {
    const pad3 = "  ".repeat(indent);
    switch (node.type) {
      case "text":
        return JSON.stringify(node.content);
      case "interpolation":
        return "String(" + formatExpression(node.expression) + ")";
      case "element":
        return genElement(node, indent);
      case "if":
        return genIf(node, indent);
      case "for":
        return genFor(node, indent);
      default:
        return "null";
    }
  }
  __name(genNode, "genNode");
  function genElement(node, indent) {
    const pad3 = "  ".repeat(indent);
    const attrsStr = genAttrsString(node.staticAttrs, node.dynamicAttrs, node.events, indent);
    if (node.children.length === 0 && node.staticAttrs && Object.keys(node.staticAttrs).length === 0 && node.dynamicAttrs.length === 0 && node.events.length === 0) {
      return pad3 + 'PetUI("' + node.tag + '", ' + attrsStr + ", [])";
    }
    const childNodes = node.children.map((c) => genNode(c, indent + 1));
    if (childNodes.length === 0) {
      return pad3 + 'PetUI("' + node.tag + '", ' + attrsStr + ", [])";
    }
    return pad3 + 'PetUI("' + node.tag + '", ' + attrsStr + ", [\n" + childNodes.join(",\n") + "\n" + pad3 + "])";
  }
  __name(genElement, "genElement");
  function genIf(node, indent) {
    const pad3 = "  ".repeat(indent);
    if (node.conditions.length === 0) return pad3 + "null";
    let code = "";
    for (let i = node.conditions.length - 1; i >= 0; i--) {
      const cond = node.conditions[i];
      const childrenCode = cond.children.map((c) => genNode(c, indent + 1)).join(",\n");
      if (cond.test !== void 0) {
        const testExpr = formatExpression(cond.test);
        if (i === node.conditions.length - 1) {
          if (i === 0) {
            code = pad3 + testExpr + " ? [\n  " + childrenCode + "\n" + pad3 + "] : null";
          } else {
            code = pad3 + testExpr + " ? [\n  " + childrenCode + "\n" + pad3 + "] : null";
          }
        } else {
          code = pad3 + testExpr + " ? [\n  " + childrenCode + "\n" + pad3 + "] : " + (code || "null");
        }
      } else {
        code = "[\n  " + childrenCode + "\n" + pad3 + "]";
      }
    }
    return code;
  }
  __name(genIf, "genIf");
  function genFor(node, indent) {
    const pad3 = "  ".repeat(indent);
    const sourceExpr = formatExpression(node.source);
    const childrenCode = node.children.map((c) => genNode(c, indent + 1)).join(",\n");
    const mapFnArgs = node.index ? node.item + ", " + node.index : node.item;
    return pad3 + "(" + sourceExpr + " || []).map(function(" + mapFnArgs + ") {\n" + pad3 + "  return [\n  " + childrenCode + "\n" + pad3 + "  ];\n" + pad3 + "})";
  }
  __name(genFor, "genFor");
  function generate(nodes) {
    const body = nodes.map((n) => genNode(n, 1)).filter(Boolean).join(",\n");
    return "function render(ctx, PetUI) {\n  return [" + body + "];\n}";
  }
  __name(generate, "generate");

  // src/core/compiler/compile.ts
  function compile(template) {
    const ast = parse(template);
    const transformed = transform(ast);
    const code = generate(transformed);
    try {
      const fn = new Function("return " + code)();
      return fn;
    } catch (e) {
      console.error("[PetUI] \u6A21\u677F\u7F16\u8BD1\u5931\u8D25:", e);
      console.error("[PetUI] \u751F\u6210\u7684\u4EE3\u7801:\n", code);
      return () => null;
    }
  }
  __name(compile, "compile");

  // src/core/vdom/h.ts
  function isVNode(val) {
    return !!(val && typeof val === "object" && "type" in val && "props" in val);
  }
  __name(isVNode, "isVNode");
  function normalizeChildren(children) {
    const result = [];
    for (const child of children.flat(Infinity)) {
      if (child == null || child === true || child === false) continue;
      if (isVNode(child)) {
        result.push(child);
      } else {
        result.push({
          type: "#text",
          props: {},
          children: [],
          el: void 0
        });
        result[result.length - 1]._text = String(child);
      }
    }
    return result;
  }
  __name(normalizeChildren, "normalizeChildren");
  function h(type, props, ...children) {
    const normalized = normalizeChildren(children);
    return {
      type,
      props: props || {},
      children: normalized,
      key: props?.key
    };
  }
  __name(h, "h");

  // src/core/vdom/mount.ts
  function skipReasonFor(el, key) {
    if (key === "key") return "vdom-internal-key: key \u662F VNode \u8EAB\u4EFD\u5B57\u6BB5\uFF0C\u4E0D\u6620\u5C04\u5230 DOM \u5C5E\u6027";
    if (el instanceof Text) return "text-node-attr: \u6587\u672C\u8282\u70B9\u53EA\u6709 _text \u4E00\u4E2A\u53EF\u5199\u69FD\u4F4D\uFF0C\u5176\u4F59\u5C5E\u6027\u65E0\u8F7D\u4F53";
    if (key === "value" || key === "checked" || key === "disabled" || key === "selected") {
      return "controlled-prop-reset: property\u300C" + key + "\u300D\u53EA\u80FD\u91CD\u7F6E\u4E3A\u9ED8\u8BA4\u503C\uFF0C\u65E0\u6CD5\u5199\u5165\u4F20\u5165\u7684\u7A7A\u503C";
    }
    return "style-non-object: style \u9700\u4E3A\u5BF9\u8C61\uFF08\u6216 null\uFF09\u624D\u80FD\u5199\u5165\uFF0C\u5176\u5B83\u5F62\u6001\u6CA1\u6709\u5BF9\u5E94\u7684\u53EF\u5199\u5F62\u6001";
  }
  __name(skipReasonFor, "skipReasonFor");
  function reportSkip(el, key, options) {
    const onEscape = options?.onEscape;
    if (!onEscape) return;
    onEscape({ key, reason: options?.skippedReason || skipReasonFor(el, key), onEscape });
  }
  __name(reportSkip, "reportSkip");
  function mount(vnode, container, options) {
    const el = createElement(vnode, options);
    container.appendChild(el);
    vnode.el = el;
  }
  __name(mount, "mount");
  function unmount(vnode) {
    const el = vnode.el;
    if (el && el.parentNode) {
      el.parentNode.removeChild(el);
    }
  }
  __name(unmount, "unmount");
  function createElement(vnode, options) {
    if (vnode.type === "#text") {
      return document.createTextNode(vnode._text || "");
    }
    const el = document.createElement(vnode.type);
    if (vnode.props) {
      for (const [key, value] of Object.entries(vnode.props)) {
        patchDOMProp(el, key, null, value, options);
      }
    }
    for (const child of vnode.children) {
      const childEl = createElement(child, options);
      el.appendChild(childEl);
      child.el = childEl;
    }
    return el;
  }
  __name(createElement, "createElement");
  function isEvent(key) {
    return key.startsWith("on") && key[2] === key[2].toUpperCase();
  }
  __name(isEvent, "isEvent");
  var eventRegistry = /* @__PURE__ */ new WeakMap();
  function patchDOMProp(el, key, _oldValue, newValue, options) {
    if (key === "key") {
      reportSkip(el, key, options);
      return;
    }
    if (el instanceof Text) {
      if (key === "_text") {
        el.textContent = String(newValue ?? "");
      } else {
        reportSkip(el, key, options);
      }
      return;
    }
    const htmlEl = el;
    if (isEvent(key)) {
      const eventName = key.slice(2).toLowerCase();
      const registry2 = eventRegistry.get(htmlEl);
      const oldHandler = registry2?.get(eventName);
      if (oldHandler) htmlEl.removeEventListener(eventName, oldHandler);
      if (typeof newValue === "function") {
        if (!registry2) eventRegistry.set(htmlEl, /* @__PURE__ */ new Map());
        eventRegistry.get(htmlEl).set(eventName, newValue);
        htmlEl.addEventListener(eventName, newValue);
      } else {
        registry2?.delete(eventName);
      }
      return;
    }
    if (key === "style") {
      if (newValue && typeof newValue === "object" && !Array.isArray(newValue)) {
        Object.assign(htmlEl.style, newValue);
      } else if (newValue == null) {
        htmlEl.style.cssText = "";
      } else {
        reportSkip(el, key, options);
      }
      return;
    }
    if (newValue == null || newValue === false) {
      if (key === "className") {
        htmlEl.removeAttribute("class");
      } else {
        htmlEl.removeAttribute(key);
        if (key === "value") {
          htmlEl.value = "";
          reportSkip(el, key, options);
        } else if (key === "checked") {
          htmlEl.checked = false;
          reportSkip(el, key, options);
        } else if (key === "disabled") {
          htmlEl.disabled = false;
          reportSkip(el, key, options);
        } else if (key === "selected") {
          htmlEl.selected = false;
          reportSkip(el, key, options);
        }
      }
      return;
    }
    if (key === "className") {
      htmlEl.setAttribute("class", String(newValue));
    } else if (key in htmlEl) {
      htmlEl[key] = newValue;
    } else {
      htmlEl.setAttribute(key, String(newValue));
    }
  }
  __name(patchDOMProp, "patchDOMProp");

  // src/core/vdom/patch.ts
  function patch(oldVNode, newVNode, parentEl) {
    if (!oldVNode) {
      if (parentEl) {
        mount(newVNode, parentEl);
      }
      return newVNode;
    }
    if (oldVNode.type !== newVNode.type) {
      replaceNode(oldVNode, newVNode);
      return newVNode;
    }
    const el = newVNode.el = oldVNode.el;
    if (newVNode.type === "#text") {
      const oldText = oldVNode._text;
      const newText = newVNode._text;
      if (oldText !== newText) {
        el.textContent = String(newText ?? "");
      }
      return newVNode;
    }
    patchProps(el, oldVNode.props, newVNode.props);
    patchChildren(oldVNode, newVNode, el);
    return newVNode;
  }
  __name(patch, "patch");
  function replaceNode(oldVNode, newVNode) {
    const oldEl = oldVNode.el;
    if (!oldEl || !oldEl.parentNode) return;
    const parent = oldEl.parentNode;
    const newEl = createElementFromVNode(newVNode);
    parent.insertBefore(newEl, oldEl);
    parent.removeChild(oldEl);
    newVNode.el = newEl;
  }
  __name(replaceNode, "replaceNode");
  function createElementFromVNode(vnode) {
    if (vnode.type === "#text") {
      return document.createTextNode(vnode._text || "");
    }
    const el = document.createElement(vnode.type);
    for (const [key, value] of Object.entries(vnode.props)) {
      patchDOMProp(el, key, null, value);
    }
    for (const child of vnode.children) {
      const childEl = createElementFromVNode(child);
      el.appendChild(childEl);
      child.el = childEl;
    }
    return el;
  }
  __name(createElementFromVNode, "createElementFromVNode");
  function patchProps(el, oldProps, newProps) {
    for (const key of Object.keys(oldProps)) {
      if (!(key in newProps)) {
        patchDOMProp(el, key, oldProps[key], null);
      }
    }
    for (const [key, value] of Object.entries(newProps)) {
      const oldValue = oldProps[key];
      if (oldValue !== value) {
        patchDOMProp(el, key, oldValue, value);
      }
    }
  }
  __name(patchProps, "patchProps");
  function patchChildren(oldVNode, newVNode, parentEl) {
    const oldChildren = oldVNode.children;
    const newChildren = newVNode.children;
    const hasKeys = newChildren.some((c) => c.key != null) || oldChildren.some((c) => c.key != null);
    if (!hasKeys) {
      const commonLen = Math.min(oldChildren.length, newChildren.length);
      for (let i = 0; i < commonLen; i++) {
        patch(oldChildren[i], newChildren[i], parentEl);
      }
      for (let i = commonLen; i < oldChildren.length; i++) {
        unmount(oldChildren[i]);
      }
      for (let i = commonLen; i < newChildren.length; i++) {
        mount(newChildren[i], parentEl);
      }
      return;
    }
    const oldKeyMap = /* @__PURE__ */ new Map();
    for (let i = 0; i < oldChildren.length; i++) {
      const k = oldChildren[i].key;
      if (k != null) {
        const list = oldKeyMap.get(k);
        if (list) list.push(i);
        else oldKeyMap.set(k, [i]);
      }
    }
    const matched = new Array(oldChildren.length).fill(false);
    let lastEl = null;
    for (let i = 0; i < newChildren.length; i++) {
      const newChild = newChildren[i];
      let oldIndex = -1;
      if (newChild.key != null) {
        const list = oldKeyMap.get(newChild.key);
        if (list && list.length > 0) oldIndex = list.shift();
      }
      let el;
      if (oldIndex >= 0) {
        patch(oldChildren[oldIndex], newChild, parentEl);
        el = newChild.el;
        matched[oldIndex] = true;
      } else {
        mount(newChild, parentEl);
        el = newChild.el;
      }
      const refNode = lastEl ? lastEl.nextSibling : parentEl.firstChild;
      if (el.parentNode === parentEl && el !== refNode) {
        if (refNode) parentEl.insertBefore(el, refNode);
        else parentEl.appendChild(el);
      }
      lastEl = el;
    }
    for (let i = 0; i < oldChildren.length; i++) {
      if (!matched[i]) {
        unmount(oldChildren[i]);
      }
    }
  }
  __name(patchChildren, "patchChildren");
  function createApp(container, renderFn) {
    const el = typeof container === "string" ? document.querySelector(container) : container;
    if (!el) throw new Error(`[PetUI] \u5BB9\u5668\u4E0D\u5B58\u5728`);
    let prevVNode = null;
    function update() {
      const newVNode = renderFn();
      prevVNode = patch(prevVNode, newVNode, el);
    }
    __name(update, "update");
    function unmountApp() {
      if (prevVNode) {
        unmount(prevVNode);
        prevVNode = null;
      }
    }
    __name(unmountApp, "unmountApp");
    update();
    return { update, unmount: unmountApp };
  }
  __name(createApp, "createApp");

  // src/core/component.ts
  var componentRegistry = /* @__PURE__ */ new Map();
  function defineComponent(options) {
    const name = options.name;
    if (name) {
      if (componentRegistry.has(name)) {
        console.warn(`[PetUI] \u7EC4\u4EF6 "${name}" \u5DF2\u6CE8\u518C\uFF0C\u5C06\u88AB\u8986\u76D6`);
      }
      componentRegistry.set(name, options);
    }
    return options;
  }
  __name(defineComponent, "defineComponent");
  function createComponent(options) {
    const ctx = {};
    const instance = {
      data: {},
      methods: {},
      computed: {},
      $options: options,
      mount(container) {
        if (options.beforeMount) options.beforeMount.call(ctx);
        const target = typeof container === "string" ? document.querySelector(container) : container;
        if (!target) {
          console.error("[PetUI] \u6302\u8F7D\u76EE\u6807\u4E0D\u5B58\u5728");
          return;
        }
        if (options.render && typeof options.render === "function") {
          const initialResult = options.render.call(ctx, ctx, h);
          if (initialResult && initialResult.type) {
            let prevVNode = null;
            let firstMount = true;
            const renderEffect = effect(() => {
              const result = options.render.call(ctx, ctx, h);
              if (!result || !result.type) return;
              if (firstMount) {
                mount(result, target);
                prevVNode = result;
                firstMount = false;
              } else {
                prevVNode = patch(prevVNode, result);
              }
              this.$el = target;
            });
            this._renderEffect = renderEffect;
          } else {
            this.$el = initialResult;
            if (target instanceof HTMLElement && this.$el instanceof HTMLElement) {
              target.appendChild(this.$el);
            }
          }
        }
        if (options.mounted) options.mounted.call(ctx);
      },
      unmount() {
        if (options.beforeUnmount) options.beforeUnmount.call(ctx);
        if (this._renderEffect) {
          if (typeof this._renderEffect.stop === "function") {
            this._renderEffect.stop();
          }
          this._renderEffect = null;
        }
        if (this.$el?.parentNode) {
          this.$el.parentNode.removeChild(this.$el);
        }
        if (options.unmounted) options.unmounted.call(ctx);
      }
    };
    if (options.beforeCreate) options.beforeCreate.call(ctx);
    if (options.data) {
      const rawData = options.data();
      instance.data = reactive(rawData);
      for (const key of Object.keys(instance.data)) {
        Object.defineProperty(ctx, key, {
          get: /* @__PURE__ */ __name(() => instance.data[key], "get"),
          set: /* @__PURE__ */ __name((v) => {
            instance.data[key] = v;
          }, "set"),
          enumerable: true
        });
      }
    }
    if (options.methods) {
      for (const [key, fn] of Object.entries(options.methods)) {
        const bound = fn.bind(ctx);
        instance.methods[key] = bound;
        ctx[key] = bound;
      }
    }
    if (options.computed) {
      for (const [key, getter] of Object.entries(options.computed)) {
        const comp = computed(getter.bind(ctx));
        instance.computed[key] = comp;
        Object.defineProperty(ctx, key, {
          get: /* @__PURE__ */ __name(() => {
            if (isRef(comp)) return comp.value;
            return comp;
          }, "get"),
          enumerable: true
        });
      }
    }
    if (options.watch) {
      for (const [key, watcher] of Object.entries(options.watch)) {
        const isOpts = typeof watcher === "object" && watcher !== null && "handler" in watcher;
        const handler = isOpts ? watcher.handler : watcher;
        const immediate = isOpts ? watcher.immediate : false;
        let oldVal;
        let first = true;
        effect(() => {
          const val = ctx[key];
          if (first) {
            first = false;
            oldVal = val;
            if (immediate) handler.call(ctx, val, void 0);
          } else {
            if (!Object.is(val, oldVal)) {
              handler.call(ctx, val, oldVal);
              oldVal = val;
            }
          }
        });
      }
    }
    if (options.created) options.created.call(ctx);
    return instance;
  }
  __name(createComponent, "createComponent");
  function mountComponent(options, container) {
    const instance = createComponent(options);
    instance.mount(container);
    return instance;
  }
  __name(mountComponent, "mountComponent");

  // src/lib/safe-html.ts
  var DEFAULT_GRID_COLOR = "var(--color-n-e0d4d8)";
  var DEFAULT_GRID_BG = "var(--color-bg)";
  var ALLOWED_TAGS = /* @__PURE__ */ new Set([
    // 行内文本语义
    "b",
    "i",
    "em",
    "strong",
    "u",
    "s",
    "del",
    "ins",
    "mark",
    "small",
    "sub",
    "sup",
    // 块/结构
    "p",
    "div",
    "span",
    "br",
    "hr",
    // 列表
    "ul",
    "ol",
    "li",
    "dl",
    "dt",
    "dd",
    // 标题
    "h1",
    "h2",
    "h3",
    "h4",
    "h5",
    "h6",
    // 引用/代码
    "blockquote",
    "pre",
    "code",
    // 链接/资源
    "a",
    "img",
    // 表格
    "table",
    "thead",
    "tbody",
    "tfoot",
    "tr",
    "th",
    "td",
    "caption"
  ]);
  var DEFAULT_ALLOWED_ATTRS = /* @__PURE__ */ new Set(["class", "title"]);
  function buildAllowedAttrs() {
    const table = /* @__PURE__ */ new Map();
    ALLOWED_TAGS.forEach((tag) => {
      table.set(tag, DEFAULT_ALLOWED_ATTRS);
    });
    table.set("a", /* @__PURE__ */ new Set(["href", "title", "class"]));
    table.set("img", /* @__PURE__ */ new Set(["src", "alt", "title", "class", "width", "height"]));
    table.set("td", /* @__PURE__ */ new Set(["class", "title", "colspan", "rowspan", "scope"]));
    table.set("th", /* @__PURE__ */ new Set(["class", "title", "colspan", "rowspan", "scope"]));
    return table;
  }
  __name(buildAllowedAttrs, "buildAllowedAttrs");
  var ALLOWED_ATTRS = buildAllowedAttrs();
  var XHTML_NS = "http://www.w3.org/1999/xhtml";
  var MAX_INPUT_LENGTH = 256 * 1024;
  var MAX_NODE_COUNT = 1e4;
  var MAX_ATTR_VALUE_LENGTH = 4 * 1024;
  var A_HREF_SCHEMES = /* @__PURE__ */ new Set(["http", "https", "mailto", "tel"]);
  var IMG_SRC_SCHEMES = /* @__PURE__ */ new Set(["http", "https"]);
  var HEX = /^#(?:[0-9a-fA-F]{3,4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$/;
  var FUNC_RGB = /^rgba?\(\s*[0-9]{1,3}%?(?:\s*,\s*[0-9]{1,3}%?){2}(?:\s*,\s*(?:0|1|0?\.[0-9]{1,4}))?\s*\)$/;
  var FUNC_HSL = /^hsla?\(\s*-?[0-9]{1,3}(?:\.[0-9]{1,4})?(?:deg|turn|rad|grad)?(?:\s*,\s*[0-9]{1,3}%?){2}(?:\s*,\s*(?:0|1|0?\.[0-9]{1,4}))?\s*\)$/;
  var VAR_TOKEN = /^--[A-Za-z0-9_-]{1,64}$/;
  var VAR_SHAPE = /^var\(\s*(--[A-Za-z0-9_-]{1,64})\s*(?:,\s*([\s\S]+?))?\s*\)$/;
  var NAMED = /^[a-z]{3,24}$/;
  function isSafeColorLiteral(text) {
    if (HEX.test(text) || FUNC_RGB.test(text) || FUNC_HSL.test(text)) return true;
    if (NAMED.test(text)) return CSS_COLOR_KEYWORDS.has(text.toLowerCase());
    return false;
  }
  __name(isSafeColorLiteral, "isSafeColorLiteral");
  function isSafeColorVariable(text, depth) {
    const m = VAR_SHAPE.exec(text);
    if (m === null) return false;
    if (!VAR_TOKEN.test(m[1])) return false;
    if (m[2] === void 0) return true;
    if (depth >= 2) return false;
    const inner = m[2].trim();
    return isSafeColorLiteral(inner) || isSafeColorVariable(inner, depth + 1);
  }
  __name(isSafeColorVariable, "isSafeColorVariable");
  function isSafeColor(v) {
    if (typeof v !== "string") return false;
    if (v.length === 0 || v.length > 128) return false;
    if (/[;"'\\<>\n\r\t\f\v]/.test(v)) return false;
    if (v.includes("/*") || v.includes("*/") || v.includes("!")) return false;
    const t = v.trim();
    if (t.length === 0) return false;
    return isSafeColorLiteral(t) || isSafeColorVariable(t, 0);
  }
  __name(isSafeColor, "isSafeColor");
  function describeElements(root) {
    const parts = [];
    const walk = /* @__PURE__ */ __name((node) => {
      const children = node.childNodes;
      for (let i = 0; i < children.length; i++) {
        const child = children[i];
        if (child.nodeType !== 1) continue;
        const el = child;
        const attrs = [];
        const list = el.attributes;
        for (let j = 0; j < list.length; j++) {
          const attr = list[j];
          const name = attr.name.toLowerCase();
          if (name === "xmlns" || name.indexOf("xmlns:") === 0) continue;
          attrs.push(name + "=" + JSON.stringify(attr.value));
        }
        attrs.sort();
        parts.push(el.tagName.toLowerCase() + "[" + attrs.join(",") + "]");
        walk(child);
      }
    }, "walk");
    walk(root);
    return JSON.stringify(parts);
  }
  __name(describeElements, "describeElements");
  function collectTagSet(root, out) {
    const children = root.childNodes;
    for (let i = 0; i < children.length; i++) {
      const child = children[i];
      if (child.nodeType !== 1) continue;
      out.add(child.tagName.toLowerCase());
      collectTagSet(child, out);
    }
  }
  __name(collectTagSet, "collectTagSet");
  function tagSetOf(root) {
    const set = /* @__PURE__ */ new Set();
    collectTagSet(root, set);
    return set;
  }
  __name(tagSetOf, "tagSetOf");
  function contextIntroducesNewTag(context, serialized, base) {
    const host = document.createElement(context);
    host.innerHTML = serialized;
    const seen = tagSetOf(host);
    for (const tag of seen) {
      if (!base.has(tag)) return true;
    }
    return false;
  }
  __name(contextIntroducesNewTag, "contextIntroducesNewTag");
  function measureSize(root) {
    let nodes = 0;
    let oversized = false;
    const walk = /* @__PURE__ */ __name((node) => {
      const children = node.childNodes;
      for (let i = 0; i < children.length; i++) {
        const child = children[i];
        nodes++;
        if (nodes > MAX_NODE_COUNT) return;
        if (child.nodeType !== 1) continue;
        const list = child.attributes;
        for (let j = 0; j < list.length; j++) {
          if (list[j].value.length > MAX_ATTR_VALUE_LENGTH) {
            oversized = true;
            return;
          }
        }
        walk(child);
      }
    }, "walk");
    walk(root);
    return { nodes, oversizedAttr: oversized };
  }
  __name(measureSize, "measureSize");
  function stripUrlNoise(raw) {
    const step1 = String(raw).replace(/[\x00-\x20]/g, "");
    return step1.replace(/[\t\n\r\f\0]/g, "").trim();
  }
  __name(stripUrlNoise, "stripUrlNoise");
  function decodeEntitiesOnce(text) {
    const ta = document.createElement("textarea");
    ta.innerHTML = text;
    return ta.value;
  }
  __name(decodeEntitiesOnce, "decodeEntitiesOnce");
  function cleanUrl(raw, schemes) {
    const step1 = stripUrlNoise(raw);
    const decoded = decodeEntitiesOnce(step1);
    const checked = decoded.replace(/[\x00-\x20\x7f]/g, "").trim();
    if (checked.length === 0) return null;
    const m = /^([a-zA-Z][a-zA-Z0-9+.\-]*):/.exec(checked);
    if (m !== null) {
      if (!schemes.has(m[1].toLowerCase())) return null;
      return checked;
    }
    if (checked.indexOf("//") === 0) return null;
    return checked;
  }
  __name(cleanUrl, "cleanUrl");
  function judgeAttribute(el, attr) {
    if (attr.namespaceURI !== null) return { ok: false, value: attr.value };
    const name = attr.name.trim().toLowerCase();
    if (name.length === 0) return { ok: false, value: attr.value };
    if (/[\x00-\x1f\x7f/]/.test(name)) return { ok: false, value: attr.value };
    if (name.indexOf("on") === 0) return { ok: false, value: attr.value };
    if (name === "style") return { ok: false, value: attr.value };
    const allowed = ALLOWED_ATTRS.get(el.tagName.toLowerCase());
    if (allowed === void 0 || !allowed.has(name)) return { ok: false, value: attr.value };
    if (name === "href" || name === "src") {
      const raw = attr.value;
      if (raw.length > MAX_ATTR_VALUE_LENGTH) return { ok: false, value: raw };
      const cleaned = cleanUrl(raw, name === "href" ? A_HREF_SCHEMES : IMG_SRC_SCHEMES);
      if (cleaned === null) return { ok: false, value: raw };
      return { ok: true, value: cleaned };
    }
    return { ok: true, value: attr.value };
  }
  __name(judgeAttribute, "judgeAttribute");
  function rebuildTree(root) {
    const walk = /* @__PURE__ */ __name((parent) => {
      const kids = [];
      const list = parent.childNodes;
      for (let i = 0; i < list.length; i++) kids.push(list[i]);
      for (let i = 0; i < kids.length; i++) {
        const child = kids[i];
        if (child.nodeType === 8) {
          parent.removeChild(child);
          continue;
        }
        if (child.nodeType !== 1) continue;
        const el = child;
        const tag = el.tagName.toLowerCase();
        if (el.namespaceURI !== XHTML_NS || !ALLOWED_TAGS.has(tag)) {
          const promoted = [];
          const inner = el.childNodes;
          for (let k = 0; k < inner.length; k++) promoted.push(inner[k]);
          el.replaceWith(...promoted);
          walk(parent);
          return;
        }
        const attrs = [];
        const attrList = el.attributes;
        for (let k = 0; k < attrList.length; k++) attrs.push(attrList[k]);
        for (let k = 0; k < attrs.length; k++) {
          const verdict = judgeAttribute(el, attrs[k]);
          if (!verdict.ok) {
            el.removeAttributeNode(attrs[k]);
          } else if (attrs[k].value !== verdict.value) {
            el.setAttribute(attrs[k].name, verdict.value);
          }
        }
        walk(el);
      }
    }, "walk");
    walk(root);
  }
  __name(rebuildTree, "rebuildTree");
  function clearElement(el) {
    try {
      el.replaceChildren();
    } catch (err) {
      console.warn("[PetUI safe-html] replaceChildren() \u4E0D\u53EF\u7528\uFF0CclearElement \u964D\u7EA7\u4E3A\u9010\u4E2A removeChild:", err);
      try {
        while (el.firstChild !== null) el.removeChild(el.firstChild);
      } catch (err2) {
        console.error("[PetUI safe-html] clearElement \u7684\u517C\u5BB9\u964D\u7EA7\u94FE\u5168\u90E8\u5931\u8D25\uFF0C\u5143\u7D20\u53EF\u80FD\u672A\u88AB\u6E05\u7A7A:", err2);
      }
    }
  }
  __name(clearElement, "clearElement");
  function degradeToText(el, original) {
    try {
      const doc = el.ownerDocument !== null && el.ownerDocument !== void 0 ? el.ownerDocument : document;
      el.replaceChildren(doc.createTextNode(original));
    } catch (err) {
      console.warn("[PetUI safe-html] replaceChildren() \u4E0D\u53EF\u7528\uFF0CdegradeToText \u964D\u7EA7\u4E3A textContent \u8D4B\u503C:", err);
      try {
        el.textContent = original;
      } catch (err2) {
        console.error("[PetUI safe-html] degradeToText \u7684\u517C\u5BB9\u964D\u7EA7\u94FE\u5168\u90E8\u5931\u8D25\uFF0C\u6587\u672C\u964D\u7EA7\u53EF\u80FD\u672A\u751F\u6548:", err2);
      }
    }
  }
  __name(degradeToText, "degradeToText");
  var REPARSE_CONTEXTS = ["div", "table", "tbody", "select", "svg"];
  function setSafeHTML(el, html) {
    if (el === null || el === void 0) return;
    if (typeof document === "undefined") return;
    try {
      if (html === null || html === void 0 || html === "") {
        clearElement(el);
        return;
      }
      if (typeof html !== "string") {
        clearElement(el);
        return;
      }
      if (html.length > MAX_INPUT_LENGTH) {
        clearElement(el);
        return;
      }
      const parsed = document.createElement("template");
      parsed.innerHTML = html;
      const size = measureSize(parsed.content);
      if (size.nodes > MAX_NODE_COUNT || size.oversizedAttr) {
        clearElement(el);
        return;
      }
      rebuildTree(parsed.content);
      const holder = document.createElement("template");
      holder.content.appendChild(parsed.content);
      const serialized = holder.innerHTML;
      const base = describeElements(holder.content);
      const baseTags = tagSetOf(holder.content);
      const again = document.createElement("template");
      again.innerHTML = serialized;
      if (describeElements(again.content) !== base) {
        degradeToText(el, html);
        return;
      }
      for (let i = 0; i < REPARSE_CONTEXTS.length; i++) {
        if (contextIntroducesNewTag(REPARSE_CONTEXTS[i], serialized, baseTags)) {
          degradeToText(el, html);
          return;
        }
      }
      const out = [];
      const kids = holder.content.childNodes;
      for (let i = 0; i < kids.length; i++) out.push(kids[i].cloneNode(true));
      el.replaceChildren(...out);
    } catch (err) {
      console.error("[PetUI safe-html] \u6D88\u6BD2\u91CD\u5EFA\u5931\u8D25\uFF0C\u5DF2\u9000\u5316\u4E3A\u6E05\u7A7A\uFF08\u6E32\u67D3\u672A\u4E2D\u65AD\uFF09:", err);
      clearElement(el);
    }
  }
  __name(setSafeHTML, "setSafeHTML");

  // src/core/movable-control.ts
  PetUIExpand._movableControls = PetUIExpand._movableControls || {};
  PetUIExpand._mcZIndex = PetUIExpand._mcZIndex || 1e4;
  function MovableControl(options) {
    const opts = Object.assign({
      id: PetUIExpand.Guid(),
      title: "Control",
      x: 0,
      y: 0,
      width: 200,
      height: 150,
      content: "",
      container: null,
      minWidth: 100,
      minHeight: 60,
      maxWidth: 0,
      maxHeight: 0,
      onDragEnd: null,
      onResizeEnd: null,
      onClose: null
    }, options);
    const container = opts.container || document.body;
    const minW = Math.max(opts.minWidth, 40);
    const minH = Math.max(opts.minHeight, 30);
    const maxW = opts.maxWidth > 0 ? Math.max(opts.maxWidth, minW) : null;
    const maxH = opts.maxHeight > 0 ? Math.max(opts.maxHeight, minH) : null;
    const el = document.createElement("div");
    el.className = "petui-mc";
    el.style.cssText = [
      "position:absolute",
      "left:" + opts.x + "px",
      "top:" + opts.y + "px",
      "width:" + opts.width + "px",
      "height:" + opts.height + "px",
      "min-width:" + minW + "px",
      "min-height:" + minH + "px",
      "background:var(--color-n-fff)",
      "border-radius:var(--space-8)",
      "box-shadow:0 var(--space-2) var(--font-size-12) var(--color-alpha-shade-12)",
      "border:var(--space-1) solid var(--color-n-e0e0e0)",
      "display:flex",
      "flex-direction:column",
      "overflow:hidden",
      "z-index:" + (PetUIExpand._mcZIndex || 1e4),
      "user-select:none"
    ].join(";");
    const titleBar = document.createElement("div");
    titleBar.className = "petui-mc-titlebar";
    titleBar.style.cssText = "cursor:move;display:flex;align-items:center;justify-content:space-between;padding:var(--space-6) var(--space-10);background:linear-gradient(135deg,var(--color-n-ffb6c1),var(--color-n-f48fb1));color:var(--color-bg-light);user-select:none;border-radius:var(--space-6) var(--space-6) 0 0;flex-shrink:0";
    const titleSpan = document.createElement("span");
    titleSpan.className = "petui-mc-title";
    titleSpan.textContent = opts.title;
    const closeBtn = document.createElement("span");
    closeBtn.className = "petui-mc-close";
    closeBtn.style.cssText = "cursor:pointer;font-size:var(--space-16);line-height:1;padding:0 var(--space-2)";
    closeBtn.textContent = "\u2715";
    titleBar.appendChild(titleSpan);
    titleBar.appendChild(closeBtn);
    const contentEl = document.createElement("div");
    contentEl.className = "petui-mc-content";
    contentEl.style.cssText = "flex:1;overflow:auto;padding:var(--space-8);min-height:0";
    setSafeHTML(contentEl, opts.content);
    const resizeHandle = document.createElement("div");
    resizeHandle.className = "petui-mc-resize";
    resizeHandle.style.cssText = "position:absolute;bottom:0;right:0;width:var(--font-size-14);height:var(--font-size-14);cursor:nwse-resize;background:linear-gradient(135deg,transparent 50%,var(--color-n-ccc) 50%);border-radius:0 0 var(--space-6) 0";
    el.appendChild(titleBar);
    el.appendChild(contentEl);
    el.appendChild(resizeHandle);
    let dragState = null;
    let resizeState = null;
    if (typeof PetUIExpand._mcZIndex === "undefined") {
      PetUIExpand._mcZIndex = 1e4;
    }
    function bringToFront() {
      PetUIExpand._mcZIndex++;
      el.style.zIndex = PetUIExpand._mcZIndex;
    }
    __name(bringToFront, "bringToFront");
    function onTitleMouseDown(e) {
      if (e.button !== 0) return;
      e.preventDefault();
      bringToFront();
      dragState = {
        startMouseX: e.clientX,
        startMouseY: e.clientY,
        startLeft: el.offsetLeft,
        startTop: el.offsetTop,
        startWidth: el.offsetWidth,
        startHeight: el.offsetHeight
      };
      document.addEventListener("mousemove", onDocMouseMove);
      document.addEventListener("mouseup", onDocMouseUp);
    }
    __name(onTitleMouseDown, "onTitleMouseDown");
    function onResizeMouseDown(e) {
      if (e.button !== 0) return;
      e.preventDefault();
      e.stopPropagation();
      bringToFront();
      resizeState = {
        startMouseX: e.clientX,
        startMouseY: e.clientY,
        startLeft: el.offsetLeft,
        startTop: el.offsetTop,
        startWidth: el.offsetWidth,
        startHeight: el.offsetHeight
      };
      document.addEventListener("mousemove", onDocMouseMove);
      document.addEventListener("mouseup", onDocMouseUp);
    }
    __name(onResizeMouseDown, "onResizeMouseDown");
    function onDocMouseMove(e) {
      const cw = container === document.body ? window.innerWidth : container.offsetWidth;
      const ch = container === document.body ? window.innerHeight : container.offsetHeight;
      if (dragState) {
        const dx = e.clientX - dragState.startMouseX;
        const dy = e.clientY - dragState.startMouseY;
        let newW = dragState.startWidth;
        let newH = dragState.startHeight;
        if (newW < minW) newW = minW;
        if (newH < minH) newH = minH;
        if (maxW && newW > maxW) newW = maxW;
        if (maxH && newH > maxH) newH = maxH;
        let newLeft = dragState.startLeft + dx;
        let newTop = dragState.startTop + dy;
        if (newLeft < 0) newLeft = 0;
        if (newTop < 0) newTop = 0;
        const maxLeft = Math.max(0, cw - newW);
        const maxTop = Math.max(0, ch - newH);
        if (newLeft > maxLeft) newLeft = maxLeft;
        if (newTop > maxTop) newTop = maxTop;
        el.style.left = newLeft + "px";
        el.style.top = newTop + "px";
      } else if (resizeState) {
        const dx = e.clientX - resizeState.startMouseX;
        const dy = e.clientY - resizeState.startMouseY;
        let newW = resizeState.startWidth + dx;
        let newH = resizeState.startHeight + dy;
        if (newW < minW) newW = minW;
        if (newH < minH) newH = minH;
        if (maxW && newW > maxW) newW = maxW;
        if (maxH && newH > maxH) newH = maxH;
        let newLeft = resizeState.startLeft;
        let newTop = resizeState.startTop;
        if (newLeft < 0) newLeft = 0;
        if (newTop < 0) newTop = 0;
        const maxLeft = Math.max(0, cw - newW);
        const maxTop = Math.max(0, ch - newH);
        if (newLeft > maxLeft) newLeft = maxLeft;
        if (newTop > maxTop) newTop = maxTop;
        el.style.left = newLeft + "px";
        el.style.top = newTop + "px";
        el.style.width = newW + "px";
        el.style.height = newH + "px";
      }
    }
    __name(onDocMouseMove, "onDocMouseMove");
    function onDocMouseUp(e) {
      if (dragState) {
        const pos = { x: el.offsetLeft, y: el.offsetTop, width: el.offsetWidth, height: el.offsetHeight };
        dragState = null;
        document.removeEventListener("mousemove", onDocMouseMove);
        document.removeEventListener("mouseup", onDocMouseUp);
        if (typeof opts.onDragEnd === "function") {
          opts.onDragEnd(pos);
        }
      } else if (resizeState) {
        const pos = { x: el.offsetLeft, y: el.offsetTop, width: el.offsetWidth, height: el.offsetHeight };
        resizeState = null;
        document.removeEventListener("mousemove", onDocMouseMove);
        document.removeEventListener("mouseup", onDocMouseUp);
        if (typeof opts.onResizeEnd === "function") {
          opts.onResizeEnd(pos);
        }
      }
    }
    __name(onDocMouseUp, "onDocMouseUp");
    function onCloseClick(e) {
      e.stopPropagation();
      cleanup();
      if (el.parentNode) el.parentNode.removeChild(el);
      if (typeof opts.onClose === "function") {
        opts.onClose();
      }
      delete PetUIExpand._movableControls[opts.id];
    }
    __name(onCloseClick, "onCloseClick");
    el.addEventListener("click", function(e) {
      bringToFront();
    });
    titleBar.addEventListener("mousedown", onTitleMouseDown);
    resizeHandle.addEventListener("mousedown", onResizeMouseDown);
    closeBtn.addEventListener("click", onCloseClick);
    container.appendChild(el);
    function cleanup() {
      titleBar.removeEventListener("mousedown", onTitleMouseDown);
      resizeHandle.removeEventListener("mousedown", onResizeMouseDown);
      closeBtn.removeEventListener("click", onCloseClick);
      document.removeEventListener("mousemove", onDocMouseMove);
      document.removeEventListener("mouseup", onDocMouseUp);
      dragState = null;
      resizeState = null;
    }
    __name(cleanup, "cleanup");
    const api = {
      element: el,
      id: opts.id,
      setContent: /* @__PURE__ */ __name(function(html) {
        setSafeHTML(contentEl, html);
      }, "setContent"),
      setTitle: /* @__PURE__ */ __name(function(title) {
        titleSpan.textContent = title;
      }, "setTitle"),
      moveTo: /* @__PURE__ */ __name(function(x, y) {
        if (typeof x === "number") el.style.left = x + "px";
        if (typeof y === "number") el.style.top = y + "px";
      }, "moveTo"),
      resizeTo: /* @__PURE__ */ __name(function(w, h2) {
        if (typeof w === "number") {
          let width = Math.max(w, minW);
          if (maxW && width > maxW) width = maxW;
          el.style.width = width + "px";
        }
        if (typeof h2 === "number") {
          let height = Math.max(h2, minH);
          if (maxH && height > maxH) height = maxH;
          el.style.height = height + "px";
        }
      }, "resizeTo"),
      getPosition: /* @__PURE__ */ __name(function() {
        return { x: el.offsetLeft, y: el.offsetTop, width: el.offsetWidth, height: el.offsetHeight };
      }, "getPosition"),
      close: /* @__PURE__ */ __name(function() {
        cleanup();
        if (el.parentNode) el.parentNode.removeChild(el);
        if (typeof opts.onClose === "function") {
          opts.onClose();
        }
        delete PetUIExpand._movableControls[opts.id];
      }, "close"),
      focus: /* @__PURE__ */ __name(function() {
        bringToFront();
      }, "focus"),
      destroy: /* @__PURE__ */ __name(function() {
        cleanup();
        if (el.parentNode) el.parentNode.removeChild(el);
        delete PetUIExpand._movableControls[opts.id];
      }, "destroy")
    };
    PetUIExpand._movableControls[opts.id] = api;
    return api;
  }
  __name(MovableControl, "MovableControl");
  PetUIExpand.MovableControl = MovableControl;

  // src/lib/disposer.ts
  function defaultTimerHost() {
    const g = globalThis;
    if (typeof g.setTimeout !== "function" || typeof g.clearTimeout !== "function") {
      throw new Error("[PetUI disposer] \u73AF\u5883\u7F3A\u5C11 setTimeout/clearTimeout\uFF0C\u65E0\u6CD5\u521B\u5EFA\u5B9A\u65F6\u5668\u5BBF\u4E3B");
    }
    return g;
  }
  __name(defaultTimerHost, "defaultTimerHost");
  var _DisposeErrors = class _DisposeErrors extends Error {
    constructor(errors, message) {
      super(message);
      this.name = "DisposeErrors";
      this.errors = errors.slice();
    }
  };
  __name(_DisposeErrors, "DisposeErrors");
  var DisposeErrors = _DisposeErrors;
  function createDisposer(timers = defaultTimerHost(), errorSink = (err) => console.error("[PetUI disposer] \u91CA\u653E\u52A8\u4F5C\u629B\u9519:", err)) {
    const actions = [];
    const collectedErrors = [];
    let disposed = false;
    const onError = /* @__PURE__ */ __name((err) => {
      collectedErrors.push(err);
      try {
        errorSink(err);
      } catch (secondary) {
        console.error("[PetUI disposer] errorSink \u56DE\u8C03\u81EA\u8EAB\u629B\u9519:", secondary);
      }
    }, "onError");
    const register = /* @__PURE__ */ __name((action) => {
      if (disposed) {
        console.warn(`[PetUI disposer] dispose() \u4E4B\u540E\u4ECD\u767B\u8BB0\u8D44\u6E90\uFF08${action.kind}: ${action.label}\uFF09\uFF0C\u5DF2\u7ACB\u5373\u91CA\u653E`);
        try {
          action.run();
        } catch (err) {
          onError(err);
        }
        return;
      }
      actions.push(action);
    }, "register");
    const disposer = {
      add(fn) {
        if (typeof fn !== "function") {
          throw new TypeError("[PetUI disposer] add \u9700\u8981\u4E00\u4E2A\u51FD\u6570");
        }
        register({
          kind: "fn",
          label: fn.name || "<anonymous>",
          run: fn
        });
      },
      addEventListener(target, type, fn, opts) {
        if (target === null || target === void 0 || typeof target.addEventListener !== "function") {
          throw new TypeError("[PetUI disposer] addEventListener \u9700\u8981\u4E00\u4E2A EventTarget");
        }
        if (typeof fn !== "function") {
          throw new TypeError("[PetUI disposer] addEventListener \u9700\u8981\u4E00\u4E2A EventListener");
        }
        const label = `${type}:${fn.name || "<anonymous>"}`;
        const duplicate = actions.some(
          (a) => a.kind === "listener" && a.label === label && a.target === target
        );
        if (duplicate) return;
        const action = {
          kind: "listener",
          label,
          target,
          run: /* @__PURE__ */ __name(() => {
            target.removeEventListener(type, fn, opts);
          }, "run")
        };
        target.addEventListener(type, fn, opts);
        register(action);
      },
      setTimeout(fn, ms) {
        if (typeof fn !== "function") throw new TypeError("[PetUI disposer] setTimeout \u9700\u8981\u4E00\u4E2A\u51FD\u6570");
        const handle = timers.setTimeout(() => {
          if (disposed) return;
          fn();
        }, ms);
        register({
          kind: "timeout",
          label: fn.name || "<anonymous>",
          run: /* @__PURE__ */ __name(() => timers.clearTimeout(handle), "run")
        });
        return handle;
      },
      setInterval(fn, ms) {
        if (typeof fn !== "function") throw new TypeError("[PetUI disposer] setInterval \u9700\u8981\u4E00\u4E2A\u51FD\u6570");
        const handle = timers.setInterval(() => {
          if (disposed) return;
          fn();
        }, ms);
        register({
          kind: "interval",
          label: fn.name || "<anonymous>",
          run: /* @__PURE__ */ __name(() => timers.clearInterval(handle), "run")
        });
        return handle;
      },
      raf(fn) {
        if (typeof fn !== "function") throw new TypeError("[PetUI disposer] raf \u9700\u8981\u4E00\u4E2A\u51FD\u6570");
        const raf = timers.requestAnimationFrame;
        const caf = timers.cancelAnimationFrame;
        if (typeof raf === "function" && typeof caf === "function") {
          const handle2 = raf((time) => {
            if (disposed) return;
            fn(time);
          });
          register({ kind: "raf", label: fn.name || "<anonymous>", run: /* @__PURE__ */ __name(() => caf(handle2), "run") });
          return handle2;
        }
        console.warn("[PetUI disposer] \u73AF\u5883\u65E0 requestAnimationFrame\uFF0Craf() \u9000\u5316\u4E3A 16ms \u5B9A\u65F6\u5668");
        const handle = timers.setTimeout(() => {
          if (disposed) return;
          fn(Date.now());
        }, 16);
        register({ kind: "raf", label: fn.name || "<anonymous>", run: /* @__PURE__ */ __name(() => timers.clearTimeout(handle), "run") });
        return handle;
      },
      observe(ob) {
        if (ob === null || ob === void 0 || typeof ob.disconnect !== "function") {
          throw new TypeError("[PetUI disposer] observe \u9700\u8981\u4E00\u4E2A\u5E26 disconnect() \u7684\u5BF9\u8C61");
        }
        register({
          kind: "observer",
          label: ob.constructor ? ob.constructor.name : "<observer>",
          run: /* @__PURE__ */ __name(() => ob.disconnect(), "run")
        });
      },
      dispose() {
        if (disposed) return;
        disposed = true;
        while (actions.length > 0) {
          const action = actions.pop();
          try {
            action.run();
          } catch (err) {
            onError(err);
          }
        }
        if (collectedErrors.length > 0) {
          throw new DisposeErrors(
            collectedErrors.slice(),
            `[PetUI disposer] ${collectedErrors.length} \u4E2A\u91CA\u653E\u52A8\u4F5C\u5931\u8D25`
          );
        }
      },
      get disposed() {
        return disposed;
      },
      get errors() {
        return Object.freeze(collectedErrors.slice());
      }
    };
    return disposer;
  }
  __name(createDisposer, "createDisposer");

  // src/lib/persist.ts
  var STORE_OPTION_KEYS = ["prefix", "storage", "now"];
  function readErrorField(err, field) {
    if (err === null || typeof err !== "object") return "";
    const raw = err[field];
    return typeof raw === "string" || typeof raw === "number" ? String(raw) : "";
  }
  __name(readErrorField, "readErrorField");
  function classify(err) {
    const name = readErrorField(err, "name");
    const code = readErrorField(err, "code");
    if (name === "SecurityError") return "access-denied";
    const numericCode = code === "" ? Number.NaN : Number(code);
    if (name === "QuotaExceededError" || name === "NS_ERROR_DOM_QUOTA_REACHED") return "quota-exceeded";
    if (numericCode === 22 || numericCode === 1014 || numericCode === 2147942414) return "quota-exceeded";
    if (name === "InvalidStateError") return "storage-unavailable";
    if (name === "SyntaxError") return "serialization-error";
    return "unknown";
  }
  __name(classify, "classify");
  function adviceFor(reason) {
    switch (reason) {
      case "no-storage":
        return "\u5F53\u524D\u5BBF\u4E3B\u6CA1\u6709\u53EF\u7528\u7684 localStorage\uFF08node/SSR/\u88AB\u7B56\u7565\u7981\u7528\uFF09\u3002\u51FA\u8DEF\uFF1A\u2460\u5728 createStore \u6CE8\u5165\u5185\u5B58\u5B9E\u73B0 {getItem,setItem,removeItem}\uFF08\u6D4B\u8BD5\u4E0E SSR \u5747\u53EF\uFF09\uFF1B\u2461\u7531\u5BBF\u4E3B\u6302\u8F7D\u771F\u6B63\u7684\u6301\u4E45\u5C42\u540E\u91CD\u5EFA Store\uFF1B\u2462\u63A5\u53D7\u300C\u4EC5\u672C\u6B21\u4F1A\u8BDD\u6709\u6548\u300D\u7684\u964D\u7EA7\u3002";
      case "access-denied":
        return "\u5B58\u50A8\u8BBF\u95EE\u88AB\u62D2\u7EDD\uFF08\u9690\u79C1\u6A21\u5F0F/\u7B2C\u4E09\u65B9\u4E0A\u4E0B\u6587/SecurityError\uFF09\u3002\u51FA\u8DEF\uFF1A\u2460\u63D0\u793A\u7528\u6237\u9000\u51FA\u9690\u79C1\u6A21\u5F0F\u6216\u653E\u5F00\u7AD9\u70B9\u6570\u636E\u6743\u9650\uFF1B\u2461\u6CE8\u5165\u5BBF\u4E3B\u63D0\u4F9B\u7684\u6301\u4E45\u5C42\uFF1B\u2462\u63A5\u53D7\u300C\u4EC5\u672C\u6B21\u4F1A\u8BDD\u6709\u6548\u300D\u7684\u964D\u7EA7\u3002";
      case "quota-exceeded":
        return "\u5199\u5165\u8D85\u51FA\u5B58\u50A8\u914D\u989D\uFF08QuotaExceededError\uFF09\u3002\u51FA\u8DEF\uFF1A\u2460\u8C03\u7528 remove() \u6E05\u7406\u4E0D\u518D\u9700\u8981\u7684\u952E\uFF1B\u2461\u6539\u5B58\u66F4\u5C0F\u7684\u503C\uFF08\u672C\u6A21\u5757\u4E0D\u505A\u538B\u7F29\uFF09\uFF1B\u2462\u628A\u5927\u5BF9\u8C61\u4EA4\u7ED9\u5BBF\u4E3B\u4FA7\u5B58\u50A8\u3002";
      case "storage-unavailable":
        return "\u5B58\u50A8\u5BF9\u8C61\u5F53\u524D\u4E0D\u53EF\u7528\uFF08InvalidStateError \u7B49\uFF09\u3002\u51FA\u8DEF\uFF1A\u2460\u7A0D\u540E\u91CD\u8BD5\uFF08\u672C Store \u5DF2\u964D\u7EA7\uFF0C\u4E0D\u56DE\u9000\uFF0C\u9700\u91CD\u5EFA\u5B9E\u4F8B\uFF09\uFF1B\u2461\u6CE8\u5165\u66FF\u4EE3\u5B58\u50A8\u5B9E\u73B0\u3002";
      case "serialization-error":
        return "\u5B58\u50A8\u4E2D\u7684\u503C\u65E0\u6CD5\u88AB\u8C03\u7528\u65B9\u89E3\u6790\uFF08SyntaxError\uFF09\u3002\u51FA\u8DEF\uFF1A\u2460\u8C03\u7528\u65B9\u6309\u4E1A\u52A1\u89C4\u5219 remove() \u8BE5\u952E\u540E\u91CD\u5EFA\u9ED8\u8BA4\u503C\uFF1B\u2461\u5199\u5165\u524D\u6821\u9A8C\u5E8F\u5217\u5316\u8F93\u51FA\u3002";
      case "unknown":
        return "\u672A\u77E5\u5B58\u50A8\u5F02\u5E38\uFF08\u5DF2\u4FDD\u7559\u539F\u59CB lastError\uFF09\u3002\u51FA\u8DEF\uFF1A\u2460\u8BFB\u53D6 describe().lastError \u5F52\u56E0\uFF1B\u2461\u6CE8\u5165\u66FF\u4EE3\u5B58\u50A8\uFF1B\u2462\u63A5\u53D7\u4F1A\u8BDD\u5185\u964D\u7EA7\u3002";
      default:
        return "\u672A\u77E5\u72B6\u6001\uFF1A\u8BF7\u8BFB\u53D6 describe().lastError \u5F52\u56E0\u3002";
    }
  }
  __name(adviceFor, "adviceFor");
  function readOwn(obj, key) {
    return Object.prototype.hasOwnProperty.call(obj, key) ? obj[key] : void 0;
  }
  __name(readOwn, "readOwn");
  function createStore(opts) {
    if (opts === null || typeof opts !== "object") {
      throw new TypeError(`createStore: opts \u5FC5\u987B\u662F\u5BF9\u8C61\uFF0C\u6536\u5230 ${String(opts)}`);
    }
    for (const key of Object.keys(opts)) {
      if (!STORE_OPTION_KEYS.includes(key)) {
        throw new RangeError(`createStore: \u672A\u77E5\u9009\u9879 "${key}"\uFF08\u5141\u8BB8\uFF1Aprefix/storage/now\uFF09`);
      }
    }
    const prefix = opts.prefix;
    if (typeof prefix !== "string" || prefix.length === 0) {
      throw new RangeError(`createStore: prefix \u5FC5\u987B\u662F\u975E\u7A7A\u5B57\u7B26\u4E32\uFF0C\u6536\u5230 ${String(prefix)}`);
    }
    const injected = readOwn(opts, "storage");
    if (injected !== void 0 && injected !== null && typeof injected !== "object") {
      throw new TypeError(`createStore: storage \u5FC5\u987B\u662F\u5B9E\u73B0 getItem/setItem/removeItem \u7684\u5BF9\u8C61\uFF0C\u6536\u5230 ${typeof injected}`);
    }
    const fallback = /* @__PURE__ */ new Map();
    let resolved = injected === void 0 ? null : injected;
    let probed = injected !== void 0;
    let broken = false;
    let reason = null;
    let lastError = null;
    let hint = null;
    let warned = false;
    const warnOnce = /* @__PURE__ */ __name(() => {
      if (warned) return;
      warned = true;
      console.warn(
        `[petui.persist] \u6301\u4E45\u5316\u4E0D\u53EF\u7528\uFF08prefix="${prefix}", reason=${reason ?? "unknown"}\uFF09\uFF1A\u5199\u5165\u5DF2\u964D\u7EA7\u4E3A\u300C\u4EC5\u672C\u6B21\u4F1A\u8BDD\u6709\u6548\u300D\u3002${hint ?? ""}`
      );
    }, "warnOnce");
    const markBroken = /* @__PURE__ */ __name((err, why) => {
      if (!broken) {
        broken = true;
        reason = why;
        lastError = err;
        hint = adviceFor(why);
      }
      warnOnce();
    }, "markBroken");
    const storage = /* @__PURE__ */ __name(() => {
      if (broken) return resolved;
      if (!probed) {
        probed = true;
        const g = globalThis;
        let globalStorage;
        try {
          globalStorage = g.localStorage;
        } catch (err) {
          if (!warned) console.error("[petui.persist] \u8BFB\u53D6\u5168\u5C40 localStorage \u5931\u8D25\uFF08\u7279\u6027\u63A2\u6D4B\u964D\u7EA7\uFF09", err);
          resolved = null;
          markBroken(err, classify(err));
          return null;
        }
        if (globalStorage === void 0 || globalStorage === null) {
          resolved = null;
          markBroken(null, "no-storage");
          return null;
        }
        resolved = globalStorage;
      }
      if (resolved === null) {
        markBroken(null, "no-storage");
        return null;
      }
      return resolved;
    }, "storage");
    const fullKey = /* @__PURE__ */ __name((key) => prefix + key, "fullKey");
    return {
      get(key) {
        const st = storage();
        if (st !== null) {
          try {
            const raw = st.getItem(fullKey(key));
            if (raw !== null) fallback.set(key, raw);
            return raw;
          } catch (err) {
            if (!warned) console.error(`[petui.persist] get("${key}") \u8BFB\u53D6\u5931\u8D25\uFF0C\u964D\u7EA7\u4E3A\u4F1A\u8BDD\u5185\u5B58\u503C`, err);
            markBroken(err, classify(err));
          }
        }
        return fallback.has(key) ? fallback.get(key) : null;
      },
      set(key, value) {
        fallback.set(key, value);
        const st = storage();
        if (st === null) return;
        try {
          st.setItem(fullKey(key), value);
        } catch (err) {
          if (!warned) console.error(`[petui.persist] set("${key}") \u5199\u5165\u5931\u8D25\uFF0C\u964D\u7EA7\u4E3A\u4F1A\u8BDD\u5185\u5B58\u5199`, err);
          markBroken(err, classify(err));
        }
      },
      remove(key) {
        fallback.delete(key);
        const st = storage();
        if (st === null) return;
        try {
          st.removeItem(fullKey(key));
        } catch (err) {
          if (!warned) console.error(`[petui.persist] remove("${key}") \u5220\u9664\u5931\u8D25\uFF0C\u964D\u7EA7\u4E3A\u4F1A\u8BDD\u5185\u5B58\u5220\u9664`, err);
          markBroken(err, classify(err));
        }
      },
      describe() {
        storage();
        return Object.freeze({
          prefix,
          broken,
          reason,
          lastError,
          hint,
          memoryOnly: broken,
          sessionKeys: fallback.size
        });
      },
      keys() {
        return Array.from(fallback.keys());
      },
      get broken() {
        return broken;
      },
      get lastError() {
        return lastError;
      },
      get warned() {
        return warned;
      }
    };
  }
  __name(createStore, "createStore");

  // src/core/split-pane-reflow.ts
  function panelPct(size) {
    if (typeof size !== "string") return null;
    const m = /^\s*([\d.]+)\s*%\s*$/.exec(size);
    if (m === null) return null;
    const value = Number(m[1]);
    return Number.isFinite(value) ? value / 100 : null;
  }
  __name(panelPct, "panelPct");
  function clamp2(value, min, max) {
    let out = value;
    if (min > 0 && out < min) out = min;
    if (max !== null && out > max) out = max;
    return out;
  }
  __name(clamp2, "clamp");
  function planReflow(input) {
    const { space, specs, current } = input;
    if (!(space > 0) || specs.length === 0 || current.length !== specs.length) return null;
    const mins = specs.map((s) => Math.max(0, s.min || 0));
    const fits = specs.map((s) => s.fit === true);
    let next;
    if (fits.some(Boolean)) {
      const fitPx = specs.map((s, i) => fits[i] ? Math.max(mins[i], Math.round(input.natural(i)) || mins[i]) : 0);
      const rest = Math.max(0, space - fitPx.reduce((a, b) => a + b, 0));
      const flexIdx = specs.map((s, i) => i).filter((i) => !fits[i]);
      const wsum = flexIdx.reduce((a, i) => a + (specs[i].pct !== null ? specs[i].pct : 0), 0);
      next = specs.map((s, i) => {
        if (fits[i]) return fitPx[i];
        const share = flexIdx.length <= 1 ? 1 : wsum > 0 && s.pct !== null ? s.pct / wsum : 1 / flexIdx.length;
        return Math.max(mins[i], Math.floor(rest * share));
      });
    } else if (!input.userAdjusted && specs.every((s) => s.pct !== null)) {
      next = specs.map((s, i) => Math.max(mins[i], Math.round(space * s.pct)));
    } else {
      const sum = current.reduce((a, b) => a + b, 0) || 1;
      const k = space / sum;
      next = current.map((size, i) => Math.max(mins[i], Math.floor(size * k)));
    }
    next = next.map((size, i) => clamp2(size, mins[i], specs[i].max));
    let over = next.reduce((a, b) => a + b, 0) - space;
    for (let guard = 0; over > 0 && guard < 64; guard += 1) {
      const order = next.map((size, i) => i).sort((a, b) => next[b] - next[a]);
      let cut = false;
      for (const i of order) {
        if (over <= 0) break;
        const room = next[i] - mins[i];
        if (room <= 0) continue;
        const take = Math.min(room, over);
        next[i] -= take;
        over -= take;
        cut = true;
      }
      if (!cut) break;
    }
    if (next.every((size, i) => Math.abs(size - current[i]) < 1)) return null;
    return next;
  }
  __name(planReflow, "planReflow");

  // src/core/split-pane.ts
  function resolveLength(value, el) {
    if (typeof value === "number") return Number.isFinite(value) ? value : void 0;
    if (typeof value !== "string") return void 0;
    const text = value.trim();
    const wrapped = /^var\(\s*(--[a-z0-9-]+)\s*\)$/.exec(text);
    if (wrapped) {
      const name = wrapped[1];
      const fromEl = computedValue(el, name);
      if (fromEl !== null) return parseLength(fromEl);
      return parseLength(computedValue(rootElement(), name) ?? "");
    }
    return parseLength(text);
  }
  __name(resolveLength, "resolveLength");
  function computedValue(el, name) {
    if (el === null) return null;
    const gcs = globalThis.getComputedStyle;
    if (typeof gcs !== "function") return null;
    const computed5 = gcs(el);
    if (!computed5 || typeof computed5.getPropertyValue !== "function") return null;
    const raw = computed5.getPropertyValue(name);
    if (typeof raw !== "string") return null;
    const trimmed = raw.trim();
    return trimmed.length === 0 ? null : trimmed;
  }
  __name(computedValue, "computedValue");
  function rootElement() {
    const doc = globalThis.document;
    return doc && doc.documentElement ? doc.documentElement : null;
  }
  __name(rootElement, "rootElement");
  function parseLength(text) {
    const px3 = /^(-?\d+(?:\.\d+)?)px$/.exec(text);
    if (px3) {
      const n = Number(px3[1]);
      return Number.isFinite(n) ? n : void 0;
    }
    const bare = /^(-?\d+(?:\.\d+)?)$/.exec(text);
    if (bare) {
      const n = Number(bare[1]);
      return Number.isFinite(n) ? n : void 0;
    }
    return void 0;
  }
  __name(parseLength, "parseLength");
  var DEFAULT_DIVIDER_TOKEN = "var(--space-4)";
  var DEFAULT_KEYBOARD_STEP_TOKEN = "var(--space-16)";
  var DEFAULT_CONTAINER_HEIGHT_TOKEN = "var(--space-400)";
  var SIZE_KEY = { horizontal: "offsetWidth", vertical: "offsetHeight" };
  PetUIExpand._dockZIndex = PetUIExpand._dockZIndex || 1e3;
  function SplitPane(options) {
    const opts = Object.assign({
      container: null,
      direction: "horizontal",
      panels: [],
      dividerSize: DEFAULT_DIVIDER_TOKEN,
      containerHeight: void 0,
      dividerColor: "var(--color-border)",
      dividerActiveColor: "var(--color-primary)",
      reflow: false,
      onResize: null,
      onReset: null,
      persistKey: null,
      store: null,
      keyboardStep: DEFAULT_KEYBOARD_STEP_TOKEN,
      ariaLabel: "\u9762\u677F\u5206\u5272\u7EBF"
    }, options);
    if (!opts.container) {
      console.warn("SplitPane: container is required");
      return null;
    }
    const container = opts.container;
    const isHorizontal = opts.direction === "horizontal";
    const dividerSize = Math.max(resolveLength(opts.dividerSize, container) ?? 4, 2);
    const keyboardStep = Math.max(resolveLength(opts.keyboardStep, container) ?? 16, 1);
    const disposer = createDisposer(globalThis);
    let store = opts.store ?? null;
    if (store === null && typeof opts.persistKey === "string" && opts.persistKey.length > 0) {
      store = createStore({ prefix: "petui.split-pane." });
    }
    container.innerHTML = "";
    container.style.display = "flex";
    container.style.flexDirection = isHorizontal ? "row" : "column";
    container.style.alignItems = "stretch";
    if (!container.style.width && container.offsetWidth === 0) {
      container.style.width = "100%";
    }
    if (opts.containerHeight === null) {
    } else if (opts.containerHeight !== void 0) {
      container.style.height = typeof opts.containerHeight === "number" ? opts.containerHeight + "px" : String(opts.containerHeight);
    } else if (!container.style.height && container.offsetHeight === 0) {
      container.style.height = DEFAULT_CONTAINER_HEIGHT_TOKEN;
    }
    container.style.overflow = "hidden";
    container.style.position = "relative";
    const panelEls = [];
    const dividerEls = [];
    const panelMeta = [];
    const sizes = [];
    const initialSizes = [];
    let userAdjusted = false;
    const gestureListeners = [];
    function addGestureListener(target, type, fn) {
      const dupe = gestureListeners.some((l) => l.target === target && l.type === type && l.fn === fn);
      if (dupe) return;
      target.addEventListener(type, fn);
      gestureListeners.push({ target, type, fn });
    }
    __name(addGestureListener, "addGestureListener");
    function releaseGesture() {
      while (gestureListeners.length > 0) {
        const item = gestureListeners.pop();
        try {
          item.target.removeEventListener(item.type, item.fn, item.opts);
        } catch (err) {
          console.warn("[PetUI split-pane] \u624B\u52BF\u76D1\u542C\u89E3\u7ED1\u5931\u8D25:", err);
        }
      }
    }
    __name(releaseGesture, "releaseGesture");
    const state = {
      dragDivider: null,
      dragDividerIndex: -1,
      direction: "horizontal",
      startMousePos: 0,
      startPrevSize: 0,
      startNextSize: 0,
      pointerId: null,
      captured: false
    };
    function measure(index) {
      if (typeof sizes[index] === "number" && sizes[index] > 0) return sizes[index];
      const el = panelEls[index];
      const key = isHorizontal ? SIZE_KEY.horizontal : SIZE_KEY.vertical;
      const raw = el ? el[key] : 0;
      const measured = typeof raw === "number" && raw > 0 ? raw : 0;
      if (measured > 0) sizes[index] = measured;
      return measured;
    }
    __name(measure, "measure");
    function applySize(index) {
      const el = panelEls[index];
      if (!el) return;
      el.style.flex = "0 0 " + sizes[index] + "px";
    }
    __name(applySize, "applySize");
    function applyPair(idx, prevSize, nextSize) {
      sizes[idx] = prevSize;
      sizes[idx + 1] = nextSize;
      applySize(idx);
      applySize(idx + 1);
      refreshAria();
    }
    __name(applyPair, "applyPair");
    function resizePair(idx, targetPrev) {
      const prevMeta = panelMeta[idx];
      const nextMeta = panelMeta[idx + 1];
      const available = state.startPrevSize + state.startNextSize;
      let newPrev = targetPrev;
      if (prevMeta.min && newPrev < prevMeta.min) newPrev = prevMeta.min;
      if (prevMeta.max && newPrev > prevMeta.max) newPrev = prevMeta.max;
      let newNext = available - newPrev;
      if (nextMeta.min && newNext < nextMeta.min) newNext = nextMeta.min;
      if (nextMeta.max && newNext > nextMeta.max) newNext = nextMeta.max;
      newPrev = available - newNext;
      if (newPrev < 0) newPrev = 0;
      if (newNext < 0) newNext = 0;
      return { prev: newPrev, next: newNext };
    }
    __name(resizePair, "resizePair");
    function beginPair(idx) {
      state.startPrevSize = measure(idx);
      state.startNextSize = measure(idx + 1);
    }
    __name(beginPair, "beginPair");
    function refreshAria() {
      for (let i = 0; i < dividerEls.length; i += 1) {
        dividerEls[i].setAttribute("aria-valuenow", String(Math.round(sizes[i])));
      }
    }
    __name(refreshAria, "refreshAria");
    function movePair(idx, targetPrev) {
      if (idx < 0 || idx + 1 >= panelEls.length) return [];
      const next = resizePair(idx, targetPrev);
      userAdjusted = true;
      applyPair(idx, next.prev, next.next);
      return [{ id: panelMeta[idx].id, size: next.prev }, { id: panelMeta[idx + 1].id, size: next.next }];
    }
    __name(movePair, "movePair");
    const persistKey = typeof opts.persistKey === "string" && opts.persistKey.length > 0 ? opts.persistKey : null;
    function persist() {
      if (store === null || persistKey === null) return;
      const record = {};
      for (let i = 0; i < panelMeta.length; i += 1) record[panelMeta[i].id] = Math.round(sizes[i]);
      store.set(persistKey, JSON.stringify(record));
    }
    __name(persist, "persist");
    function restore() {
      if (store === null || persistKey === null) return null;
      const raw = store.get(persistKey);
      if (typeof raw !== "string" || raw.length === 0) return null;
      let parsed;
      try {
        parsed = JSON.parse(raw);
      } catch (err) {
        console.warn("[PetUI split-pane] \u6301\u4E45\u5316\u5C3A\u5BF8\u89E3\u6790\u5931\u8D25\uFF0C\u6309\u521D\u59CB\u5C3A\u5BF8\u7EE7\u7EED:", err);
        return null;
      }
      if (parsed === null || typeof parsed !== "object" || Array.isArray(parsed)) return null;
      const out = {};
      for (const [key, value] of Object.entries(parsed)) {
        if (typeof value === "number" && Number.isFinite(value)) out[key] = value;
      }
      return out;
    }
    __name(restore, "restore");
    const usePointer = typeof globalThis.PointerEvent === "function";
    for (let i = 0; i < opts.panels.length; i++) {
      const panel = opts.panels[i];
      const panelEl = document.createElement("div");
      panelEl.className = "petui-sp-panel";
      panelEl.setAttribute("data-panel-id", panel.id || "panel_" + i);
      const size = panel.size || "1fr";
      let flexBasis;
      if (typeof size === "string" && size.indexOf("fr") >= 0) {
        flexBasis = null;
      } else {
        flexBasis = typeof size === "number" ? size + "px" : size;
      }
      const minW = panel.min || 0;
      const maxW = panel.max || null;
      let flexStyle;
      if (flexBasis) {
        flexStyle = "flex:0 0 " + flexBasis + ";";
      } else {
        flexStyle = "flex:1 1 0%;";
      }
      if (isHorizontal) {
        flexStyle += "min-width:" + minW + "px;";
        if (maxW) flexStyle += "max-width:" + maxW + "px;";
      } else {
        flexStyle += "min-height:" + minW + "px;";
        if (maxW) flexStyle += "max-height:" + maxW + "px;";
      }
      flexStyle += "overflow:auto;";
      panelEl.style.cssText = flexStyle;
      if (panel.content !== void 0 && panel.content !== null) {
        if (typeof panel.content === "string") setSafeHTML(panelEl, panel.content);
        else panelEl.appendChild(panel.content);
      }
      container.appendChild(panelEl);
      panelEls.push(panelEl);
      if (i < opts.panels.length - 1) createDivider2(i);
      panelMeta.push({
        id: panel.id || "panel_" + i,
        min: minW,
        max: maxW
      });
      const basisNumber = typeof size === "number" ? size : parseLength(String(size).trim()) ?? 0;
      sizes.push(basisNumber);
      initialSizes.push(basisNumber);
    }
    for (let i = 0; i < panelEls.length; i += 1) {
      const measured = measure(i);
      if (measured > 0 && sizes[i] <= 0) sizes[i] = measured;
    }
    const restored = restore();
    if (restored !== null) {
      for (let i = 0; i < panelMeta.length; i += 1) {
        const saved = restored[panelMeta[i].id];
        if (typeof saved === "number" && saved >= 0) {
          sizes[i] = saved;
          applySize(i);
        }
      }
    }
    for (let i = 0; i < panelMeta.length; i += 1) initialSizes[i] = sizes[i];
    if (restored !== null) userAdjusted = true;
    const reflowSpecs = opts.panels.map((panel) => ({
      pct: panelPct(panel.size),
      fit: panel.fit === true,
      min: panel.min || 0,
      max: panel.max || null
    }));
    function reflowNow() {
      if (disposer.disposed) return;
      const total = container[isHorizontal ? "offsetWidth" : "offsetHeight"] || 0;
      const space = Math.max(0, total - Math.max(0, panelEls.length - 1) * dividerSize);
      const next = planReflow({
        space,
        specs: reflowSpecs,
        current: sizes.slice(),
        userAdjusted,
        natural: /* @__PURE__ */ __name((i) => panelEls[i] ? panelEls[i].scrollHeight || 0 : 0, "natural")
      });
      if (next === null) return;
      for (let i = 0; i < next.length; i += 1) {
        sizes[i] = next[i];
        applySize(i);
      }
      refreshAria();
    }
    __name(reflowNow, "reflowNow");
    if (opts.reflow === true) {
      if (typeof ResizeObserver === "function") {
        const RO = ResizeObserver;
        try {
          const ro = new RO(() => reflowNow());
          ro.observe(container);
          disposer.observe(ro);
        } catch (err) {
          console.warn("[PetUI split-pane] ResizeObserver \u4E0D\u53EF\u7528\uFF0Creflow \u964D\u7EA7\u4E3A\u4E0D\u91CD\u6392:", err);
        }
      } else {
        console.warn("[PetUI split-pane] \u73AF\u5883\u65E0 ResizeObserver\uFF0Creflow \u964D\u7EA7\u4E3A\u4E0D\u91CD\u6392");
      }
    }
    function handlePointerMove(e) {
      if (!state.dragDivider) return;
      const event = e;
      const delta = state.direction === "horizontal" ? (event.clientX ?? 0) - state.startMousePos : (event.clientY ?? 0) - state.startMousePos;
      movePair(state.dragDividerIndex, state.startPrevSize + delta);
    }
    __name(handlePointerMove, "handlePointerMove");
    function handlePointerUp(e) {
      if (!state.dragDivider) return;
      const idx = state.dragDividerIndex;
      const divider = state.dragDivider;
      const prevSize = measure(idx);
      if (state.captured && state.pointerId !== null && typeof divider.releasePointerCapture === "function") {
        try {
          divider.releasePointerCapture(state.pointerId);
        } catch (err) {
          console.debug("[PetUI split-pane] releasePointerCapture \u5931\u8D25\uFF08\u591A\u4E3A\u5DF2\u88AB\u91CA\u653E\uFF09:", err);
        }
      }
      state.captured = false;
      state.pointerId = null;
      releaseGesture();
      divider.style.background = opts.dividerColor;
      state.dragDivider = null;
      state.dragDividerIndex = -1;
      persist();
      const movedPx = Math.abs(prevSize - state.startPrevSize) >= 1;
      if (movedPx && typeof opts.onResize === "function") {
        try {
          opts.onResize(panelMeta[idx].id, prevSize);
        } catch (err) {
          console.warn("[PetUI split-pane] onResize \u56DE\u8C03\u629B\u9519\uFF0C\u5DF2\u9694\u79BB:", err);
        }
      }
      void e;
    }
    __name(handlePointerUp, "handlePointerUp");
    function handlePointerDown(idx, dividerEl, e) {
      const event = e;
      if (event.button !== void 0 && event.button !== 0) return;
      if (typeof event.preventDefault === "function") event.preventDefault();
      state.dragDivider = dividerEl;
      state.dragDividerIndex = idx;
      state.direction = isHorizontal ? "horizontal" : "vertical";
      dividerEl.style.background = opts.dividerActiveColor;
      state.startMousePos = isHorizontal ? event.clientX ?? 0 : event.clientY ?? 0;
      beginPair(idx);
      const move = /* @__PURE__ */ __name((ev) => handlePointerMove(ev), "move");
      const up = /* @__PURE__ */ __name((ev) => handlePointerUp(ev), "up");
      addGestureListener(document, usePointer ? "pointermove" : "mousemove", move);
      addGestureListener(document, usePointer ? "pointerup" : "mouseup", up);
      if (usePointer) {
        addGestureListener(document, "pointercancel", up);
        const pid = event.pointerId;
        if (typeof pid === "number" && typeof dividerEl.setPointerCapture === "function") {
          try {
            dividerEl.setPointerCapture(pid);
            state.pointerId = pid;
            state.captured = true;
          } catch (err) {
            console.debug("[PetUI split-pane] setPointerCapture \u4E0D\u53EF\u7528\uFF0C\u9000\u56DE document \u7EA7\u76D1\u542C:", err);
          }
        }
      }
    }
    __name(handlePointerDown, "handlePointerDown");
    function handleLostCapture(e) {
      if (!state.dragDivider) return;
      handlePointerUp(e);
    }
    __name(handleLostCapture, "handleLostCapture");
    function panelIdOf(index) {
      const panel = opts.panels[index];
      return panel && panel.id ? panel.id : "panel_" + index;
    }
    __name(panelIdOf, "panelIdOf");
    function createDivider2(i) {
      const dividerEl = document.createElement("div");
      dividerEl.className = "petui-sp-divider";
      const cursorStyle = isHorizontal ? "col-resize" : "row-resize";
      dividerEl.style.cssText = "flex:0 0 " + dividerSize + "px;cursor:" + cursorStyle + ";background:" + opts.dividerColor + ";transition:background 0.15s;z-index:" + (PetUIExpand._dockZIndex + 1) + ";";
      dividerEl.setAttribute("data-divider-index", String(i));
      dividerEl.setAttribute("role", "separator");
      dividerEl.setAttribute("aria-orientation", isHorizontal ? "vertical" : "horizontal");
      dividerEl.setAttribute("aria-label", opts.ariaLabel + " " + (i + 1));
      dividerEl.setAttribute("aria-controls", panelIdOf(i) + " " + panelIdOf(i + 1));
      dividerEl.setAttribute("aria-valuemin", "0");
      dividerEl.setAttribute("aria-valuemax", String(Math.round(container[SIZE_KEY[isHorizontal ? "horizontal" : "vertical"]] || 0)));
      dividerEl.setAttribute("aria-valuenow", String(Math.round(sizes[i] || 0)));
      dividerEl.setAttribute("tabindex", "0");
      const index = i;
      disposer.addEventListener(dividerEl, "mouseenter", (() => {
        if (state.dragDivider !== dividerEl) {
          dividerEl.style.background = opts.dividerActiveColor;
        }
      }));
      disposer.addEventListener(dividerEl, "mouseleave", (() => {
        if (state.dragDivider !== dividerEl) {
          dividerEl.style.background = opts.dividerColor;
        }
      }));
      disposer.addEventListener(dividerEl, usePointer ? "pointerdown" : "mousedown", ((e) => {
        handlePointerDown(index, dividerEl, e);
      }));
      if (usePointer) {
        disposer.addEventListener(dividerEl, "lostpointercapture", ((e) => {
          handleLostCapture(e);
        }));
      }
      disposer.addEventListener(dividerEl, "dblclick", ((e) => {
        if (typeof e.preventDefault === "function") {
          e.preventDefault();
        }
        reset();
      }));
      disposer.addEventListener(dividerEl, "keydown", ((e) => {
        const event = e;
        const key = event.key;
        if (key !== "ArrowLeft" && key !== "ArrowRight" && key !== "ArrowUp" && key !== "ArrowDown" && key !== "Home" && key !== "End") {
          return;
        }
        if (typeof event.preventDefault === "function") event.preventDefault();
        beginPair(index);
        let target;
        if (key === "Home") target = panelMeta[index].min;
        else if (key === "End") target = panelMeta[index].max ?? measure(index) + measure(index + 1);
        else target = measure(index) + (key === "ArrowLeft" || key === "ArrowUp" ? -keyboardStep : keyboardStep);
        const moved = movePair(index, target);
        if (moved.length > 0) {
          persist();
          if (typeof opts.onResize === "function") {
            try {
              opts.onResize(panelMeta[index].id, moved[0].size);
            } catch (err) {
              console.warn("[PetUI split-pane] onResize \u56DE\u8C03\u629B\u9519\uFF0C\u5DF2\u9694\u79BB:", err);
            }
          }
        }
      }));
      container.appendChild(dividerEl);
      dividerEls.push(dividerEl);
    }
    __name(createDivider2, "createDivider");
    function getSizes() {
      const out = [];
      for (let i = 0; i < panelEls.length; i++) {
        out.push({
          id: panelMeta[i].id,
          size: measure(i)
        });
      }
      return out;
    }
    __name(getSizes, "getSizes");
    function setPanelSize(panelId, sizePx) {
      for (let i = 0; i < panelMeta.length; i++) {
        if (panelMeta[i].id === panelId) {
          const meta = panelMeta[i];
          let size = sizePx;
          if (meta.min && size < meta.min) size = meta.min;
          if (meta.max && size > meta.max) size = meta.max;
          sizes[i] = size;
          applySize(i);
          refreshAria();
          persist();
          return;
        }
      }
    }
    __name(setPanelSize, "setPanelSize");
    function reset() {
      if (state.dragDivider) handlePointerUp(null);
      for (let i = 0; i < panelMeta.length; i += 1) {
        sizes[i] = initialSizes[i];
        applySize(i);
      }
      refreshAria();
      if (store !== null && persistKey !== null) store.remove(persistKey);
      if (typeof opts.onReset === "function") {
        try {
          opts.onReset(getSizes());
        } catch (err) {
          console.warn("[PetUI split-pane] onReset \u56DE\u8C03\u629B\u9519\uFF0C\u5DF2\u9694\u79BB:", err);
        }
      }
    }
    __name(reset, "reset");
    function destroy() {
      if (state.dragDivider) handlePointerUp(null);
      releaseGesture();
      disposer.dispose();
      container.innerHTML = "";
      container.style.cssText = "";
    }
    __name(destroy, "destroy");
    refreshAria();
    return {
      container,
      panels: panelEls.map(function(el, i) {
        return { el, id: panelMeta[i].id };
      }),
      dividers: dividerEls.map(function(el, i) {
        return { el, index: i };
      }),
      getSizes,
      setPanelSize,
      reset,
      destroy
    };
  }
  __name(SplitPane, "SplitPane");
  PetUIExpand.SplitPane = SplitPane;

  // src/core/dock-layout.ts
  PetUIExpand._dockZIndex = PetUIExpand._dockZIndex ?? 10;
  function DockLayout(options) {
    const opts = Object.assign({
      container: null,
      zones: ["left", "center", "right"],
      direction: "horizontal",
      // K13 D 族：裸 px → 主题无关尺寸令牌（`--space-N` 的键就是值，见 src/themes/sizes.ts）
      zoneSizes: { left: "var(--space-200)", center: "1fr", right: "var(--space-250)" },
      zoneMins: { left: 100, center: 200, right: 150 },
      dividerSize: 5,
      panels: [],
      onPanelMove: null,
      onZoneResize: null
    }, options);
    if (!opts.container) {
      console.warn("DockLayout: container is required");
      return null;
    }
    const container = opts.container;
    const isHorizontal = opts.direction === "horizontal";
    const dividerSize = Math.max(opts.dividerSize, 2);
    const zones = opts.zones;
    container.innerHTML = "";
    container.style.display = "flex";
    container.style.flexDirection = isHorizontal ? "row" : "column";
    container.style.width = "100%";
    if (!container.style.height && container.offsetHeight === 0) {
      container.style.height = "var(--space-400)";
    }
    container.style.overflow = "hidden";
    container.style.position = "relative";
    const zoneEls = {};
    const zoneDividerEls = [];
    const zonePanels = {};
    const panelElMap = {};
    for (let i = 0; i < zones.length; i++) {
      zonePanels[zones[i]] = [];
    }
    for (let i = 0; i < opts.panels.length; i++) {
      const p = opts.panels[i];
      const z = p.zone && zonePanels[p.zone] ? p.zone : zones[0];
      zonePanels[z].push({ id: p.id, title: p.title, content: p.content, zone: z });
    }
    const splitState = {
      dragDivider: null,
      dragDividerIndex: -1,
      startMousePos: 0,
      startPrevSize: 0,
      startNextSize: 0
    };
    const dragState = {
      dragging: false,
      panelId: null,
      fromZone: null,
      ghost: null,
      startMouseX: 0,
      startMouseY: 0,
      gripEl: null
    };
    function renderZoneStructure() {
      const savedPanels = {};
      for (const z in zonePanels) {
        savedPanels[z] = zonePanels[z].slice();
      }
      container.innerHTML = "";
      zoneEls.left = null;
      zoneEls.center = null;
      zoneEls.right = null;
      for (let i = 0; i < zones.length; i++) {
        const zoneId = zones[i];
        const zoneEl = document.createElement("div");
        zoneEl.className = "petui-dock-zone";
        zoneEl.setAttribute("data-zone", zoneId);
        const size = opts.zoneSizes[zoneId] || "1fr";
        let flexBasis;
        if (typeof size === "string" && size.indexOf("fr") >= 0) {
          flexBasis = null;
        } else {
          flexBasis = size;
        }
        let flexStyle;
        if (flexBasis) {
          flexStyle = "flex:0 0 " + flexBasis + ";";
        } else {
          flexStyle = "flex:1 1 0%;";
        }
        if (isHorizontal) {
          const minZone = opts.zoneMins[zoneId] || 0;
          flexStyle += "min-width:" + minZone + "px;";
        } else {
          const minZone = opts.zoneMins[zoneId] || 0;
          flexStyle += "min-height:" + minZone + "px;";
        }
        flexStyle += "overflow:auto;background:var(--color-n-fafafa);padding:var(--space-4);display:flex;flex-direction:column;";
        zoneEl.style.cssText = flexStyle;
        zonePanels[zoneId] = savedPanels[zoneId] || [];
        for (let j = 0; j < zonePanels[zoneId].length; j++) {
          const pd = zonePanels[zoneId][j];
          const panelEl = createPanelElement(pd);
          zoneEl.appendChild(panelEl);
          panelElMap[pd.id] = panelEl;
        }
        container.appendChild(zoneEl);
        zoneEls[zoneId] = zoneEl;
        if (i < zones.length - 1) {
          const dividerEl = document.createElement("div");
          dividerEl.className = "petui-dock-divider";
          const cursorStyle = isHorizontal ? "col-resize" : "row-resize";
          dividerEl.style.cssText = "flex:0 0 " + dividerSize + "px;cursor:" + cursorStyle + ";background:var(--color-n-e0e0e0);transition:background 0.15s;z-index:" + ((PetUIExpand._dockZIndex ?? 10) + 1) + ";";
          dividerEl.setAttribute("data-divider-index", String(i));
          dividerEl.addEventListener("mouseenter", function() {
            if (splitState.dragDivider !== this) {
              this.style.background = "var(--color-n-ffb6c1)";
            }
          });
          dividerEl.addEventListener("mouseleave", function() {
            if (splitState.dragDivider !== this) {
              this.style.background = "var(--color-n-e0e0e0)";
            }
          });
          dividerEl.addEventListener("mousedown", /* @__PURE__ */ (function(idx) {
            return function(e) {
              if (e.button !== 0) return;
              e.preventDefault();
              splitState.dragDivider = this;
              splitState.dragDividerIndex = idx;
              this.style.background = "var(--color-n-ffb6c1)";
              if (isHorizontal) {
                splitState.startMousePos = e.clientX;
              } else {
                splitState.startMousePos = e.clientY;
              }
              const prevZoneEl = zoneEls[zones[idx]];
              const nextZoneEl = zoneEls[zones[idx + 1]];
              splitState.startPrevSize = isHorizontal ? prevZoneEl.offsetWidth : prevZoneEl.offsetHeight;
              splitState.startNextSize = isHorizontal ? nextZoneEl.offsetWidth : nextZoneEl.offsetHeight;
              document.addEventListener("mousemove", onSplitMouseMove);
              document.addEventListener("mouseup", onSplitMouseUp);
            };
          })(i));
          container.appendChild(dividerEl);
          zoneDividerEls.push(dividerEl);
        }
      }
    }
    __name(renderZoneStructure, "renderZoneStructure");
    function createPanelElement(panelData) {
      const panelEl = document.createElement("div");
      panelEl.className = "petui-dock-panel";
      panelEl.setAttribute("data-id", panelData.id);
      panelEl.style.cssText = "background:var(--color-n-fff);border-radius:var(--space-6);margin:var(--space-4);box-shadow:0 var(--space-1) var(--space-4) var(--color-alpha-shade-08);display:flex;flex-direction:column;overflow:hidden;flex-shrink:0;";
      const gripEl = document.createElement("div");
      gripEl.className = "petui-dock-panel-grip";
      gripEl.style.cssText = "height:var(--space-8);background:var(--color-n-eee);cursor:grab;flex-shrink:0;transition:background 0.15s;";
      gripEl.addEventListener("mouseenter", function() {
        if (!dragState.dragging) {
          this.style.background = "var(--color-n-ffb6c1)";
        }
      });
      gripEl.addEventListener("mouseleave", function() {
        if (!dragState.dragging || dragState.gripEl !== this) {
          this.style.background = "var(--color-n-eee)";
        }
      });
      gripEl.addEventListener("mousedown", function(e) {
        if (e.button !== 0) return;
        e.preventDefault();
        dragState.dragging = true;
        dragState.panelId = panelData.id;
        dragState.gripEl = this;
        this.style.background = "var(--color-n-ffb6c1)";
        this.style.cursor = "grabbing";
        for (const z in zoneEls) {
          if (zoneEls[z] && zoneEls[z].contains(panelEl)) {
            dragState.fromZone = z;
            break;
          }
        }
        dragState.startMouseX = e.clientX;
        dragState.startMouseY = e.clientY;
        const ghost = document.createElement("div");
        ghost.className = "petui-dock-panel-ghost";
        ghost.style.cssText = "position:fixed;z-index:" + ((PetUIExpand._dockZIndex ?? 10) + 999) + ";background:var(--color-alpha-light-85);border:var(--space-2) dashed var(--color-n-ffb6c1);border-radius:var(--space-6);padding:var(--space-8) var(--space-16);pointer-events:none;font-size:var(--font-size-13);color:var(--color-n-333);box-shadow:0 var(--space-4) var(--space-16) var(--color-alpha-primary-30);";
        ghost.textContent = panelData.title || panelData.id;
        ghost.style.left = e.clientX + "px";
        ghost.style.top = e.clientY + "px";
        ghost.style.transform = "translate(-50%, -50%)";
        document.body.appendChild(ghost);
        dragState.ghost = ghost;
        document.addEventListener("mousemove", onPanelDragMove);
        document.addEventListener("mouseup", onPanelDragUp);
      });
      const titleEl = document.createElement("div");
      titleEl.className = "petui-dock-panel-title";
      titleEl.style.cssText = "padding:var(--space-4) var(--space-10);font-size:var(--font-size-13);font-weight:600;color:var(--color-n-555);flex-shrink:0;";
      titleEl.textContent = panelData.title;
      const contentEl = document.createElement("div");
      contentEl.className = "petui-dock-panel-content";
      contentEl.style.cssText = "padding:var(--space-8);overflow:auto;min-height:0;flex:1;";
      setSafeHTML(contentEl, panelData.content);
      panelEl.appendChild(gripEl);
      panelEl.appendChild(titleEl);
      panelEl.appendChild(contentEl);
      return panelEl;
    }
    __name(createPanelElement, "createPanelElement");
    function findZoneAtPos(x, y) {
      for (const z in zoneEls) {
        const el = zoneEls[z];
        if (!el) continue;
        const rect = el.getBoundingClientRect();
        if (x >= rect.left && x <= rect.right && y >= rect.top && y <= rect.bottom) {
          return z;
        }
      }
      return null;
    }
    __name(findZoneAtPos, "findZoneAtPos");
    function onPanelDragMove(e) {
      if (!dragState.dragging || !dragState.ghost) return;
      dragState.ghost.style.left = e.clientX + "px";
      dragState.ghost.style.top = e.clientY + "px";
      const targetZone = findZoneAtPos(e.clientX, e.clientY);
      for (const z in zoneEls) {
        if (zoneEls[z]) {
          zoneEls[z].classList.remove("petui-dock-zone-hover");
          if (z === targetZone) {
            zoneEls[z].classList.add("petui-dock-zone-hover");
            zoneEls[z].style.background = "var(--color-n-fff0f3)";
          } else {
            zoneEls[z].style.background = "var(--color-n-fafafa)";
          }
        }
      }
    }
    __name(onPanelDragMove, "onPanelDragMove");
    function onPanelDragUp(e) {
      if (!dragState.dragging) return;
      document.removeEventListener("mousemove", onPanelDragMove);
      document.removeEventListener("mouseup", onPanelDragUp);
      const targetZone = findZoneAtPos(e.clientX, e.clientY);
      if (dragState.ghost && dragState.ghost.parentNode) {
        dragState.ghost.parentNode.removeChild(dragState.ghost);
      }
      dragState.ghost = null;
      if (dragState.gripEl) {
        dragState.gripEl.style.background = "var(--color-n-eee)";
        dragState.gripEl.style.cursor = "grab";
      }
      for (const z in zoneEls) {
        if (zoneEls[z]) {
          zoneEls[z].classList.remove("petui-dock-zone-hover");
          zoneEls[z].style.background = "var(--color-n-fafafa)";
        }
      }
      const panelId = dragState.panelId;
      const fromZone = dragState.fromZone;
      dragState.dragging = false;
      dragState.panelId = null;
      dragState.fromZone = null;
      dragState.gripEl = null;
      if (fromZone && targetZone && fromZone !== targetZone && zonePanels[targetZone]) {
        let panelData = null;
        for (let i = 0; i < zonePanels[fromZone].length; i++) {
          if (zonePanels[fromZone][i].id === panelId) {
            panelData = zonePanels[fromZone][i];
            zonePanels[fromZone].splice(i, 1);
            break;
          }
        }
        if (panelData) {
          panelData.zone = targetZone;
          zonePanels[targetZone].push(panelData);
          const panelEl = panelElMap[panelId];
          if (panelEl && panelEl.parentNode) {
            panelEl.parentNode.removeChild(panelEl);
          }
          const newPanelEl = createPanelElement(panelData);
          zoneEls[targetZone].appendChild(newPanelEl);
          panelElMap[panelId] = newPanelEl;
          if (typeof opts.onPanelMove === "function") {
            opts.onPanelMove(panelId, fromZone, targetZone);
          }
        }
      }
    }
    __name(onPanelDragUp, "onPanelDragUp");
    function onSplitMouseMove(e) {
      if (!splitState.dragDivider) return;
      const idx = splitState.dragDividerIndex;
      const delta = isHorizontal ? e.clientX - splitState.startMousePos : e.clientY - splitState.startMousePos;
      const prevZoneId = zones[idx];
      const nextZoneId = zones[idx + 1];
      const prevMin = opts.zoneMins[prevZoneId] || 0;
      const nextMin = opts.zoneMins[nextZoneId] || 0;
      const available = splitState.startPrevSize + splitState.startNextSize;
      let newPrev = splitState.startPrevSize + delta;
      if (newPrev < prevMin) newPrev = prevMin;
      let newNext = available - newPrev;
      if (newNext < nextMin) newNext = nextMin;
      newPrev = available - newNext;
      if (newPrev < 0) newPrev = 0;
      if (newNext < 0) newNext = 0;
      zoneEls[prevZoneId].style.flex = "0 0 " + newPrev + "px";
      zoneEls[nextZoneId].style.flex = "0 0 " + newNext + "px";
    }
    __name(onSplitMouseMove, "onSplitMouseMove");
    function onSplitMouseUp(e) {
      if (!splitState.dragDivider) return;
      const idx = splitState.dragDividerIndex;
      const prevZoneId = zones[idx];
      const prevSize = isHorizontal ? zoneEls[prevZoneId].offsetWidth : zoneEls[prevZoneId].offsetHeight;
      splitState.dragDivider.style.background = "var(--color-n-e0e0e0)";
      splitState.dragDivider = null;
      splitState.dragDividerIndex = -1;
      document.removeEventListener("mousemove", onSplitMouseMove);
      document.removeEventListener("mouseup", onSplitMouseUp);
      if (typeof opts.onZoneResize === "function") {
        opts.onZoneResize(prevZoneId, prevSize);
      }
    }
    __name(onSplitMouseUp, "onSplitMouseUp");
    renderZoneStructure();
    function addPanel(panelData) {
      const z = panelData.zone && zonePanels[panelData.zone] ? panelData.zone : zones[0];
      if (!zonePanels[z]) zonePanels[z] = [];
      zonePanels[z].push({ id: panelData.id, title: panelData.title, content: panelData.content, zone: z });
      const panelEl = createPanelElement({ id: panelData.id, title: panelData.title, content: panelData.content, zone: z });
      panelElMap[panelData.id] = panelEl;
      if (zoneEls[z]) {
        zoneEls[z].appendChild(panelEl);
      }
    }
    __name(addPanel, "addPanel");
    function removePanel(panelId) {
      const panelEl = panelElMap[panelId];
      if (panelEl && panelEl.parentNode) {
        panelEl.parentNode.removeChild(panelEl);
      }
      delete panelElMap[panelId];
      for (const z in zonePanels) {
        for (let i = 0; i < zonePanels[z].length; i++) {
          if (zonePanels[z][i].id === panelId) {
            zonePanels[z].splice(i, 1);
            return;
          }
        }
      }
    }
    __name(removePanel, "removePanel");
    function movePanel(panelId, toZone) {
      if (!zonePanels[toZone]) return;
      let panelData = null;
      let fromZone = null;
      for (const z in zonePanels) {
        for (let i = 0; i < zonePanels[z].length; i++) {
          if (zonePanels[z][i].id === panelId) {
            panelData = zonePanels[z][i];
            fromZone = z;
            zonePanels[z].splice(i, 1);
            break;
          }
        }
        if (panelData) break;
      }
      if (!panelData) return;
      panelData.zone = toZone;
      zonePanels[toZone].push(panelData);
      const panelEl = panelElMap[panelId];
      if (panelEl && panelEl.parentNode) {
        panelEl.parentNode.removeChild(panelEl);
      }
      const newPanelEl = createPanelElement(panelData);
      zoneEls[toZone].appendChild(newPanelEl);
      panelElMap[panelId] = newPanelEl;
      if (typeof opts.onPanelMove === "function") {
        opts.onPanelMove(panelId, fromZone, toZone);
      }
    }
    __name(movePanel, "movePanel");
    function getPanels() {
      const result = [];
      for (const z in zonePanels) {
        for (let i = 0; i < zonePanels[z].length; i++) {
          result.push({
            id: zonePanels[z][i].id,
            title: zonePanels[z][i].title,
            zone: z,
            content: zonePanels[z][i].content
          });
        }
      }
      return result;
    }
    __name(getPanels, "getPanels");
    function setZoneSize(zoneId, sizePx) {
      if (zoneEls[zoneId]) {
        zoneEls[zoneId].style.flex = "0 0 " + sizePx + "px";
      }
    }
    __name(setZoneSize, "setZoneSize");
    function getZoneSizes() {
      const result = {};
      for (let i = 0; i < zones.length; i++) {
        const z = zones[i];
        if (zoneEls[z]) {
          result[z] = isHorizontal ? zoneEls[z].offsetWidth : zoneEls[z].offsetHeight;
        }
      }
      return result;
    }
    __name(getZoneSizes, "getZoneSizes");
    function destroy() {
      document.removeEventListener("mousemove", onSplitMouseMove);
      document.removeEventListener("mouseup", onSplitMouseUp);
      document.removeEventListener("mousemove", onPanelDragMove);
      document.removeEventListener("mouseup", onPanelDragUp);
      if (dragState.ghost && dragState.ghost.parentNode) {
        dragState.ghost.parentNode.removeChild(dragState.ghost);
      }
      dragState.ghost = null;
      dragState.dragging = false;
      container.innerHTML = "";
      container.style.cssText = "";
    }
    __name(destroy, "destroy");
    return {
      container,
      addPanel,
      removePanel,
      movePanel,
      getPanels,
      setZoneSize,
      getZoneSizes,
      destroy
    };
  }
  __name(DockLayout, "DockLayout");
  PetUIExpand.DockLayout = DockLayout;

  // src/components/button.ts
  var DEFAULT_DISABLED_REASON = "disabled";
  function describeDisabledState(disabled, reason) {
    if (!disabled) return null;
    return { reason: reason || DEFAULT_DISABLED_REASON, escapeAction: "set-disabled-false" };
  }
  __name(describeDisabledState, "describeDisabledState");
  var TYPE_COLORS = {
    primary: "var(--color-primary)",
    secondary: "var(--color-n-95a5a6)",
    success: "var(--color-success)",
    warning: "var(--color-warning)",
    danger: "var(--color-danger)",
    info: "var(--color-info)"
  };
  function createButton(options = {}) {
    const state = reactive({
      text: "\u6309\u94AE",
      type: "primary",
      disabled: false,
      disabledReason: "",
      onDisabled: null,
      onClick: null,
      ...options
    });
    const btnStyle = computed(() => ({
      background: TYPE_COLORS[state.type || "primary"] || "var(--color-primary)",
      opacity: state.disabled ? 0.5 : 1,
      cursor: state.disabled ? "not-allowed" : "pointer",
      border: "none",
      borderRadius: "var(--space-8)",
      padding: "var(--space-10) var(--space-20)",
      color: "var(--color-bg-light)",
      fontWeight: "600",
      fontSize: "0.95rem",
      boxShadow: state.disabled ? "none" : "0 var(--space-2) var(--space-8) var(--color-alpha-primary-40)",
      transition: "all 0.2s"
    }));
    const el = document.createElement("span");
    let vnode = null;
    const render = /* @__PURE__ */ __name(() => h(
      "button",
      {
        style: btnStyle.value,
        Click: state.disabled ? void 0 : state.onClick
      },
      state.text || ""
    ), "render");
    const _effects = [];
    const _addEffect = /* @__PURE__ */ __name((e) => {
      _effects.push(e);
      return e;
    }, "_addEffect");
    _addEffect(effect(() => {
      const blocked = describeDisabledState(state.disabled === true, state.disabledReason);
      if (blocked && state.onDisabled) state.onDisabled(blocked);
      const newVNode = render();
      if (!vnode) {
        const innerEl = document.createElement("button");
        Object.assign(innerEl.style, btnStyle.value);
        innerEl.textContent = state.text || "";
        if (!state.disabled && state.onClick) {
          innerEl.onclick = state.onClick;
        }
        el.appendChild(innerEl);
        newVNode.el = innerEl;
        vnode = newVNode;
      } else {
        const innerEl = vnode.el;
        Object.assign(innerEl.style, btnStyle.value);
        innerEl.textContent = state.text || "";
        if (state.disabled) {
          innerEl.setAttribute("disabled", "");
        } else {
          innerEl.removeAttribute("disabled");
        }
        vnode = newVNode;
        vnode.el = innerEl;
      }
    }));
    return Object.assign(el, {
      setProps(props) {
        Object.assign(state, props);
      },
      destroy() {
        _effects.forEach((e) => e.stop());
        _effects.length = 0;
        el.remove();
      }
    });
  }
  __name(createButton, "createButton");

  // src/components/switch.ts
  var DEFAULT_DISABLED_REASON2 = "disabled";
  function describeDisabledState2(disabled, reason) {
    if (!disabled) return null;
    return { reason: reason || DEFAULT_DISABLED_REASON2, escapeAction: "set-disabled-false" };
  }
  __name(describeDisabledState2, "describeDisabledState");
  function createSwitch(options = {}) {
    const state = reactive({
      checked: true,
      disabled: false,
      disabledReason: "",
      onDisabled: null,
      onChange: null,
      ...options
    });
    const el = document.createElement("span");
    const outer = document.createElement("div");
    const inner = document.createElement("div");
    Object.assign(outer.style, {
      width: "var(--space-44)",
      height: "var(--row-height-normal)",
      borderRadius: "var(--font-size-12)",
      cursor: "pointer",
      position: "relative",
      display: "inline-block",
      transition: "background 0.3s",
      opacity: state.disabled ? 0.5 : 1,
      background: state.checked ? "var(--color-success)" : "var(--color-border)"
    });
    Object.assign(inner.style, {
      width: "var(--space-20)",
      height: "var(--space-20)",
      borderRadius: "50%",
      background: "var(--color-bg-light)",
      position: "absolute",
      top: "var(--space-2)",
      left: state.checked ? "var(--row-height-compact)" : "var(--space-2)",
      transition: "left 0.3s",
      boxShadow: "0 var(--space-1) var(--space-3) var(--color-alpha-shade-20)"
    });
    outer.appendChild(inner);
    outer.onclick = state.disabled ? null : (() => {
      state.checked = !state.checked;
      if (state.onChange) state.onChange(state.checked);
    });
    el.appendChild(outer);
    const _effects = [];
    const _addEffect = /* @__PURE__ */ __name((e) => {
      _effects.push(e);
      return e;
    }, "_addEffect");
    _addEffect(effect(() => {
      const blocked = describeDisabledState2(state.disabled === true, state.disabledReason);
      if (blocked && state.onDisabled) state.onDisabled(blocked);
      outer.style.background = state.checked ? "var(--color-success)" : "var(--color-border)";
      outer.style.opacity = state.disabled ? "0.5" : "1";
      inner.style.left = state.checked ? "var(--row-height-compact)" : "var(--space-2)";
      if (state.disabled) {
        outer.style.cursor = "not-allowed";
        outer.onclick = null;
      } else {
        outer.style.cursor = "pointer";
        outer.onclick = (() => {
          state.checked = !state.checked;
          if (state.onChange) state.onChange(state.checked);
        });
      }
    }));
    return Object.assign(el, {
      setProps(props) {
        Object.assign(state, props);
      },
      toggle() {
        state.checked = !state.checked;
        if (state.onChange) state.onChange(state.checked);
      },
      destroy() {
        _effects.forEach((e) => e.stop());
        _effects.length = 0;
        el.remove();
      }
    });
  }
  __name(createSwitch, "createSwitch");

  // src/components/progress.ts
  function lighten2(hex) {
    if (!/^#[0-9a-fA-F]{6}$/.test(hex)) return hex;
    const r = Math.min(255, parseInt(hex.slice(1, 3), 16) + 40);
    const g = Math.min(255, parseInt(hex.slice(3, 5), 16) + 40);
    const b = Math.min(255, parseInt(hex.slice(5, 7), 16) + 40);
    return "#" + [r, g, b].map((v) => v.toString(16).padStart(2, "0")).join("");
  }
  __name(lighten2, "lighten");
  function resolveColor(el, color) {
    const m = /^var\(\s*(--[a-z0-9-]+)\s*\)$/.exec(color);
    if (!m) return color;
    const gcs = globalThis.getComputedStyle;
    if (typeof gcs !== "function") return color;
    const raws = [el, el.ownerDocument && el.ownerDocument.documentElement];
    for (const node of raws) {
      if (!node) continue;
      const computed5 = gcs(node);
      if (!computed5 || typeof computed5.getPropertyValue !== "function") continue;
      const raw = computed5.getPropertyValue(m[1]).trim();
      if (raw) return raw;
    }
    return color;
  }
  __name(resolveColor, "resolveColor");
  function createProgress(options = {}) {
    const state = reactive({
      value: 0,
      color: "var(--color-n-ffb6c1)",
      height: 20,
      animated: true,
      ...options
    });
    const el = document.createElement("div");
    const _effects = [];
    const _addEffect = /* @__PURE__ */ __name((e) => {
      _effects.push(e);
      return e;
    }, "_addEffect");
    _addEffect(effect(() => {
      const pct = Math.min(100, Math.max(0, state.value));
      const h2 = state.height;
      el.innerHTML = "";
      el.style.cssText = "width:100%";
      const bar = document.createElement("div");
      bar.style.cssText = [
        "width:100%",
        "height:" + h2 + "px",
        "background:var(--color-border)",
        "border-radius:" + h2 / 2 + "px",
        "overflow:hidden"
      ].join(";");
      const gradientFrom = resolveColor(el, state.color);
      const fill = document.createElement("div");
      fill.style.cssText = [
        "width:" + pct + "%",
        "height:100%",
        "background:linear-gradient(90deg," + gradientFrom + "," + lighten2(gradientFrom) + ")",
        "border-radius:" + h2 / 2 + "px",
        "transition:" + (state.animated ? "width 0.4s ease" : "none")
      ].join(";");
      bar.appendChild(fill);
      el.appendChild(bar);
    }));
    let _value = options.value ?? 0;
    return Object.assign(el, {
      setProps(props) {
        Object.assign(state, props);
      },
      setValue(v) {
        _value = v;
        state.value = v;
      },
      getValue() {
        return _value;
      },
      destroy() {
        _effects.forEach((e) => e.stop());
        _effects.length = 0;
        el.remove();
      }
    });
  }
  __name(createProgress, "createProgress");

  // src/components/input.ts
  var DEFAULT_DISABLED_REASON3 = "disabled";
  function describeDisabledState3(disabled, reason) {
    if (!disabled) return null;
    return { reason: reason || DEFAULT_DISABLED_REASON3, escapeAction: "set-disabled-false" };
  }
  __name(describeDisabledState3, "describeDisabledState");
  function createInput(options = {}) {
    const state = reactive({
      value: "",
      placeholder: "\u8BF7\u8F93\u5165",
      type: "text",
      disabled: false,
      disabledReason: "",
      onDisabled: null,
      label: "",
      onChange: null,
      ...options
    });
    const el = document.createElement("div");
    el.style.display = "flex";
    el.style.flexDirection = "column";
    el.style.gap = "var(--space-4)";
    const labelEl = document.createElement("label");
    labelEl.style.cssText = "font-size:0.85rem;color:var(--color-text-secondary);font-weight:500";
    labelEl.style.display = state.label ? "" : "none";
    if (state.label) labelEl.textContent = state.label;
    el.appendChild(labelEl);
    const inputEl = document.createElement("input");
    inputEl.type = state.type;
    inputEl.placeholder = state.placeholder;
    inputEl.value = state.value;
    inputEl.disabled = state.disabled;
    inputEl.style.cssText = [
      "border:var(--space-2) solid var(--color-primary-hover)",
      "border-radius:var(--space-8)",
      "padding:var(--space-8) var(--font-size-12)",
      "font-size:0.95rem",
      "outline:none",
      "transition:border-color 0.2s",
      "opacity:" + (state.disabled ? 0.5 : 1)
    ].join(";");
    el.appendChild(inputEl);
    inputEl.addEventListener("input", () => {
      state.value = inputEl.value;
      if (state.onChange) state.onChange(inputEl.value);
    });
    inputEl.addEventListener("focus", () => {
      inputEl.style.borderColor = "var(--color-primary)";
    });
    inputEl.addEventListener("blur", () => {
      inputEl.style.borderColor = "var(--color-primary-hover)";
    });
    const _effects = [];
    const _addEffect = /* @__PURE__ */ __name((e) => {
      _effects.push(e);
      return e;
    }, "_addEffect");
    _addEffect(effect(() => {
      const blocked = describeDisabledState3(state.disabled === true, state.disabledReason);
      if (blocked && state.onDisabled) state.onDisabled(blocked);
      if (document.activeElement !== inputEl) {
        inputEl.value = state.value;
      }
      inputEl.disabled = state.disabled;
      inputEl.placeholder = state.placeholder;
      inputEl.type = state.type;
      inputEl.style.opacity = state.disabled ? "0.5" : "1";
      if (state.label) {
        labelEl.textContent = state.label;
        labelEl.style.display = "";
      } else {
        labelEl.style.display = "none";
      }
    }));
    return Object.assign(el, {
      setProps(props) {
        Object.assign(state, props);
      },
      getValue() {
        return state.value;
      },
      setValue(v) {
        state.value = v;
        if (inputEl) inputEl.value = v;
      },
      focus() {
        if (inputEl) inputEl.focus();
      },
      destroy() {
        _effects.forEach((e) => e.stop());
        _effects.length = 0;
        el.remove();
      }
    });
  }
  __name(createInput, "createInput");

  // src/components/modal.ts
  var FOCUSABLE_SELECTOR = 'a[href],button:not([disabled]),input:not([disabled]),select:not([disabled]),textarea:not([disabled]),[tabindex]:not([tabindex="-1"])';
  function focusables(root) {
    const list = root.querySelectorAll(FOCUSABLE_SELECTOR);
    const out = [];
    for (let i = 0; i < list.length; i += 1) {
      const el = list[i];
      if (el.getAttribute && el.getAttribute("tabindex") === "-1") continue;
      out.push(el);
    }
    return out;
  }
  __name(focusables, "focusables");
  function isInDocument(el) {
    return !!(el && typeof el.focus === "function" && el.isConnected !== false);
  }
  __name(isInDocument, "isInDocument");
  var modalSeq = 0;
  function createModal(options = {}) {
    const state = reactive({
      title: "\u63D0\u793A",
      content: "",
      visible: false,
      onClose: null,
      onCancel: null,
      onOk: null,
      ...options
    });
    const el = document.createElement("div");
    let currentOverlay = null;
    let previouslyFocused = null;
    let pendingFocusCapture = false;
    const titleId = "petui-modal-title-" + ++modalSeq;
    const _effects = [];
    const _addEffect = /* @__PURE__ */ __name((e) => {
      _effects.push(e);
      return e;
    }, "_addEffect");
    const cancel = /* @__PURE__ */ __name(() => {
      state.visible = false;
      if (state.onCancel) state.onCancel();
    }, "cancel");
    const onTrapKeydown = /* @__PURE__ */ __name((e) => {
      if (e.key === "Escape") {
        if (typeof e.preventDefault === "function") e.preventDefault();
        cancel();
        return;
      }
      if (e.key !== "Tab") return;
      const dialogEl = e.currentTarget && e.currentTarget.querySelector ? e.currentTarget : null;
      if (!dialogEl) return;
      const list = focusables(dialogEl);
      if (list.length === 0) {
        if (typeof e.preventDefault === "function") e.preventDefault();
        dialogEl.focus && dialogEl.focus();
        return;
      }
      const first = list[0];
      const last = list[list.length - 1];
      const active = document.activeElement;
      if (e.shiftKey) {
        if (active === first || !dialogEl.contains(active)) {
          if (typeof e.preventDefault === "function") e.preventDefault();
          last.focus();
        }
      } else if (active === last || !dialogEl.contains(active)) {
        if (typeof e.preventDefault === "function") e.preventDefault();
        first.focus();
      }
    }, "onTrapKeydown");
    _addEffect(effect(() => {
      if (currentOverlay) {
        currentOverlay.remove();
        currentOverlay = null;
      }
      el.innerHTML = "";
      if (!state.visible) {
        if (isInDocument(previouslyFocused)) {
          previouslyFocused.focus();
        }
        previouslyFocused = null;
        return;
      }
      if (pendingFocusCapture) {
        pendingFocusCapture = false;
        const active = document.activeElement;
        previouslyFocused = active && active !== document.body ? active : null;
      }
      const overlay = document.createElement("div");
      overlay.style.cssText = [
        "position:fixed",
        "inset:0",
        "background:var(--color-alpha-shade-40)",
        "display:flex",
        "align-items:center",
        "justify-content:center",
        "z-index:var(--z-modal)",
        "animation:fadeIn 0.2s"
      ].join(";");
      overlay.addEventListener("click", (e) => {
        if (e.target === overlay) {
          state.visible = false;
          if (state.onClose) state.onClose();
        }
      });
      overlay.addEventListener("keydown", onTrapKeydown, true);
      const dialog = document.createElement("div");
      dialog.setAttribute("role", "dialog");
      dialog.setAttribute("aria-modal", "true");
      dialog.setAttribute("aria-labelledby", titleId);
      dialog.setAttribute("tabindex", "-1");
      if (!state.title) dialog.setAttribute("aria-label", "\u5BF9\u8BDD\u6846");
      dialog.style.cssText = [
        "background:var(--color-bg-light)",
        "border-radius:var(--space-14)",
        "padding:var(--space-24)",
        "min-width:var(--space-300)",
        "max-width:var(--space-500)",
        "box-shadow:var(--space-8) var(--space-32) var(--color-alpha-shade-20)",
        "animation:slideIn 0.3s"
      ].join(";");
      const titleEl = document.createElement("h3");
      titleEl.id = titleId;
      titleEl.textContent = state.title;
      titleEl.style.cssText = "margin:0 0 var(--space-16);color:var(--color-primary-dark);font-size:1.1rem";
      const contentEl = document.createElement("div");
      setSafeHTML(contentEl, state.content);
      contentEl.style.cssText = "color:var(--color-text-secondary);line-height:1.6;margin-bottom:var(--space-20)";
      const closeBtn = document.createElement("button");
      closeBtn.textContent = "\u5173\u95ED";
      closeBtn.style.cssText = [
        "border:none",
        "border-radius:var(--space-8)",
        "padding:var(--space-8) var(--space-24)",
        "background:var(--color-primary)",
        "color:var(--color-n-fff)",
        "font-weight:600",
        "cursor:pointer",
        "float:right"
      ].join(";");
      closeBtn.addEventListener("click", () => {
        state.visible = false;
        if (state.onClose) state.onClose();
      });
      dialog.appendChild(titleEl);
      dialog.appendChild(contentEl);
      dialog.appendChild(closeBtn);
      overlay.appendChild(dialog);
      document.body.appendChild(overlay);
      currentOverlay = overlay;
      const list = focusables(dialog);
      const target = list.length > 0 ? list[0] : dialog;
      if (target && typeof target.focus === "function") target.focus();
    }));
    const styleId = "petui-modal-styles";
    if (!document.getElementById(styleId)) {
      const style = document.createElement("style");
      style.id = styleId;
      style.textContent = [
        "@keyframes fadeIn { from { opacity:0 } to { opacity:1 } }",
        "@keyframes slideIn { from { opacity:0; transform:translateY(calc(-1 * var(--space-20))) }",
        "  to { opacity:1; transform:translateY(0) } }"
      ].join("\n");
      document.head.appendChild(style);
    }
    return Object.assign(el, {
      setProps(props) {
        Object.assign(state, props);
      },
      open() {
        pendingFocusCapture = true;
        state.visible = true;
      },
      close() {
        state.visible = false;
        if (state.onClose) state.onClose();
      },
      confirm() {
        state.visible = false;
        if (state.onOk) state.onOk();
      },
      destroy() {
        _effects.forEach((e) => e.stop());
        _effects.length = 0;
        if (currentOverlay) currentOverlay.remove();
        el.remove();
      }
    });
  }
  __name(createModal, "createModal");

  // src/components/checkbox-radio.ts
  var DEFAULT_DISABLED_REASON4 = "disabled";
  function describeDisabledState4(disabled, reason) {
    if (!disabled) return null;
    return { reason: reason || DEFAULT_DISABLED_REASON4, escapeAction: "set-disabled-false" };
  }
  __name(describeDisabledState4, "describeDisabledState");
  function createCheckbox(options = {}) {
    const state = reactive({
      checked: false,
      disabled: false,
      disabledReason: "",
      onDisabled: null,
      label: "",
      onChange: null,
      ...options
    });
    const el = document.createElement("div");
    el.style.cssText = "display:inline-flex;align-items:center;gap:var(--space-8);cursor:pointer";
    const box = document.createElement("div");
    const size = "var(--space-20)";
    Object.assign(box.style, {
      width: size,
      height: size,
      borderRadius: "var(--space-4)",
      display: "flex",
      alignItems: "center",
      justifyContent: "center",
      transition: "all 0.2s",
      flexShrink: "0"
    });
    const check = document.createElement("span");
    check.textContent = "\u2713";
    check.style.cssText = "color:var(--color-bg-light);font-size:var(--font-size-14);font-weight:bold;line-height:1";
    box.appendChild(check);
    el.appendChild(box);
    const labelEl = document.createElement("span");
    labelEl.style.cssText = "font-size:0.95rem;color:var(--color-text)";
    el.appendChild(labelEl);
    el.addEventListener("click", () => {
      if (!state.disabled) {
        state.checked = !state.checked;
        if (state.onChange) state.onChange(state.checked);
      }
    });
    const _effects = [];
    const _addEffect = /* @__PURE__ */ __name((e) => {
      _effects.push(e);
      return e;
    }, "_addEffect");
    _addEffect(effect(() => {
      const blocked = describeDisabledState4(state.disabled === true, state.disabledReason);
      if (blocked && state.onDisabled) state.onDisabled(blocked);
      box.style.border = state.checked ? "var(--space-2) solid var(--color-primary)" : "var(--space-2) solid var(--color-border)";
      box.style.background = state.checked ? "var(--color-primary)" : "var(--color-bg-light)";
      box.style.opacity = state.disabled ? "0.5" : "1";
      box.style.cursor = state.disabled ? "not-allowed" : "pointer";
      check.style.display = state.checked ? "" : "none";
      el.style.cursor = state.disabled ? "not-allowed" : "pointer";
      if (state.label) {
        labelEl.textContent = state.label;
        labelEl.style.display = "";
      } else {
        labelEl.style.display = "none";
      }
    }));
    return Object.assign(el, {
      setProps(props) {
        Object.assign(state, props);
      },
      getChecked() {
        return state.checked;
      },
      setChecked(v) {
        state.checked = v;
        if (state.onChange) state.onChange(v);
      },
      toggle() {
        state.checked = !state.checked;
        if (state.onChange) state.onChange(state.checked);
      },
      destroy() {
        _effects.forEach((e) => e.stop());
        _effects.length = 0;
        el.remove();
      }
    });
  }
  __name(createCheckbox, "createCheckbox");
  function createRadioGroup(options = {}) {
    const state = reactive({
      name: "radio-group-" + Math.random().toString(36).slice(2, 8),
      options: [],
      value: "",
      onChange: null,
      ...options
    });
    const el = document.createElement("div");
    el.style.cssText = "display:flex;flex-direction:column;gap:var(--space-8)";
    let rows = [];
    function optionsChanged(prevRows, newOpts) {
      if (prevRows.length !== newOpts.length) return true;
      for (let i = 0; i < newOpts.length; i++) {
        if (prevRows[i].optValue !== newOpts[i].value) return true;
      }
      return false;
    }
    __name(optionsChanged, "optionsChanged");
    const _effects = [];
    const _addEffect = /* @__PURE__ */ __name((e) => {
      _effects.push(e);
      return e;
    }, "_addEffect");
    _addEffect(effect(() => {
      const groupName = state.name;
      const newOpts = state.options;
      if (optionsChanged(rows, newOpts)) {
        el.innerHTML = "";
        rows = [];
        newOpts.forEach((opt) => {
          const row = document.createElement("label");
          row.style.cssText = "display:flex;align-items:center;gap:var(--space-8);cursor:pointer;font-size:0.95rem";
          const radio = document.createElement("div");
          const checked = state.value === opt.value;
          Object.assign(radio.style, {
            width: "var(--space-18)",
            height: "var(--space-18)",
            borderRadius: "50%",
            border: checked ? "var(--space-5) solid var(--color-primary)" : "var(--space-2) solid var(--color-border)",
            background: "var(--color-bg-light)",
            transition: "all 0.2s",
            boxSizing: "border-box",
            flexShrink: "0"
          });
          const text = document.createElement("span");
          text.textContent = opt.label;
          text.style.cssText = "color:var(--color-text)";
          row.appendChild(radio);
          row.appendChild(text);
          row.addEventListener("click", () => {
            state.value = opt.value;
            if (state.onChange) state.onChange(opt.value);
          });
          el.appendChild(row);
          rows.push({ row, radio, text, optValue: opt.value });
        });
      } else {
        for (const { radio, optValue } of rows) {
          const checked = state.value === optValue;
          radio.style.border = checked ? "var(--space-5) solid var(--color-primary)" : "var(--space-2) solid var(--color-border)";
        }
      }
    }));
    return Object.assign(el, {
      setProps(props) {
        Object.assign(state, props);
      },
      getValue() {
        return state.value;
      },
      setValue(v) {
        state.value = v;
      },
      destroy() {
        _effects.forEach((e) => e.stop());
        _effects.length = 0;
        el.remove();
      }
    });
  }
  __name(createRadioGroup, "createRadioGroup");

  // src/components/select.ts
  function createSelect(options = {}) {
    const state = reactive({
      options: [],
      value: "",
      placeholder: "\u8BF7\u9009\u62E9",
      open: false,
      onChange: null,
      ...options
    });
    const el = document.createElement("div");
    el.style.cssText = "position:relative;display:inline-block;min-width:var(--space-160);user-select:none";
    let dropdownEl = null;
    function getSelectedLabel() {
      const found = state.options.find((o) => o.value === state.value);
      return found ? found.label : state.placeholder || "\u8BF7\u9009\u62E9";
    }
    __name(getSelectedLabel, "getSelectedLabel");
    function closeDropdown() {
      state.open = false;
    }
    __name(closeDropdown, "closeDropdown");
    const globalClick = /* @__PURE__ */ __name((e) => {
      if (!el.contains(e.target)) {
        closeDropdown();
      }
    }, "globalClick");
    document.addEventListener("click", globalClick);
    const trigger2 = document.createElement("div");
    Object.assign(trigger2.style, {
      border: "var(--space-2) solid var(--color-primary-hover)",
      borderRadius: "var(--space-8)",
      padding: "var(--space-8) var(--font-size-12)",
      cursor: "pointer",
      background: "var(--color-bg-light)",
      display: "flex",
      alignItems: "center",
      justifyContent: "space-between",
      fontSize: "0.95rem",
      color: "var(--color-text)",
      transition: "border-color 0.2s"
    });
    trigger2.onmouseenter = () => {
      trigger2.style.borderColor = "var(--color-primary)";
    };
    trigger2.onmouseleave = () => {
      trigger2.style.borderColor = "var(--color-primary-hover)";
    };
    const textSpan = document.createElement("span");
    textSpan.textContent = getSelectedLabel();
    trigger2.appendChild(textSpan);
    const arrow = document.createElement("span");
    arrow.style.cssText = "font-size:0.7rem;margin-left:var(--space-8);color:var(--color-text-secondary)";
    trigger2.appendChild(arrow);
    trigger2.addEventListener("click", (e) => {
      e.stopPropagation();
      state.open = !state.open;
    });
    el.appendChild(trigger2);
    const dropdown = document.createElement("div");
    Object.assign(dropdown.style, {
      position: "absolute",
      top: "100%",
      left: "0",
      right: "0",
      background: "var(--color-bg-light)",
      border: "var(--space-1) solid var(--color-border)",
      borderRadius: "var(--space-8)",
      marginTop: "var(--space-4)",
      boxShadow: "0 var(--space-4) var(--space-16) var(--color-alpha-shade-10)",
      zIndex: "1000",
      maxHeight: "var(--space-200)",
      overflowY: "auto"
    });
    el.appendChild(dropdown);
    dropdownEl = dropdown;
    const _effects = [];
    const _addEffect = /* @__PURE__ */ __name((e) => {
      _effects.push(e);
      return e;
    }, "_addEffect");
    _addEffect(effect(() => {
      textSpan.textContent = getSelectedLabel();
      arrow.textContent = state.open ? "\u25B2" : "\u25BC";
      dropdown.innerHTML = "";
      dropdown.style.display = state.open ? "" : "none";
      if (state.open) {
        state.options.forEach((opt) => {
          const item = document.createElement("div");
          item.textContent = opt.label;
          const isActive = state.value === opt.value;
          Object.assign(item.style, {
            padding: "var(--space-8) var(--font-size-12)",
            cursor: "pointer",
            background: isActive ? "var(--color-primary-light)" : "transparent",
            color: isActive ? "var(--color-primary)" : "var(--color-text)",
            fontSize: "0.95rem",
            transition: "background 0.15s"
          });
          item.onmouseenter = () => {
            if (!isActive) item.style.background = "var(--color-bg)";
          };
          item.onmouseleave = () => {
            if (!isActive) item.style.background = "transparent";
          };
          item.addEventListener("click", (e) => {
            e.stopPropagation();
            state.value = opt.value;
            state.open = false;
            if (state.onChange) state.onChange(opt.value);
          });
          dropdown.appendChild(item);
        });
      }
    }));
    return Object.assign(el, {
      setProps(props) {
        Object.assign(state, props);
      },
      getValue() {
        return state.value;
      },
      setValue(v) {
        state.value = v;
      },
      open() {
        state.open = true;
      },
      close() {
        closeDropdown();
      },
      destroy() {
        _effects.forEach((e) => e.stop());
        _effects.length = 0;
        document.removeEventListener("click", globalClick);
        el.remove();
      }
    });
  }
  __name(createSelect, "createSelect");

  // src/components/slider.ts
  function createSlider(options = {}) {
    const state = reactive({
      min: 0,
      max: 100,
      value: 50,
      step: 1,
      onChange: null,
      ...options
    });
    const el = document.createElement("div");
    el.style.cssText = "display:flex;align-items:center;gap:var(--font-size-12);width:100%";
    const input = document.createElement("input");
    input.type = "range";
    input.style.cssText = "flex:1;accent-color:var(--color-primary);height:var(--space-6);cursor:pointer";
    input.addEventListener("input", () => {
      state.value = Number(input.value);
      if (state.onChange) state.onChange(state.value);
    });
    const label = document.createElement("span");
    label.style.cssText = "min-width:var(--space-36);text-align:center;font-size:0.85rem;color:var(--color-n-666);font-weight:500";
    el.appendChild(input);
    el.appendChild(label);
    const _effects = [];
    const _addEffect = /* @__PURE__ */ __name((e) => {
      _effects.push(e);
      return e;
    }, "_addEffect");
    _addEffect(effect(() => {
      input.min = String(state.min);
      input.max = String(state.max);
      input.step = String(state.step);
      if (document.activeElement !== input) {
        input.value = String(state.value);
      }
      label.textContent = String(state.value);
    }));
    return Object.assign(el, {
      setProps(props) {
        Object.assign(state, props);
      },
      getValue() {
        return state.value;
      },
      setValue(v) {
        state.value = Math.min(state.max, Math.max(state.min, v));
      },
      destroy() {
        _effects.forEach((e) => e.stop());
        _effects.length = 0;
        el.remove();
      }
    });
  }
  __name(createSlider, "createSlider");

  // src/components/spinner.ts
  var SPINNER_STYLE_ID = "petui-spinner-styles";
  function injectSpinnerStyles() {
    if (document.getElementById(SPINNER_STYLE_ID)) return;
    const style = document.createElement("style");
    style.id = SPINNER_STYLE_ID;
    style.textContent = `
        @keyframes petui-spin {
            to { transform: rotate(360deg); }
        }
        @keyframes petui-dot-bounce {
            0%, 80%, 100% { transform: scale(0); }
            40% { transform: scale(1); }
        }
    `;
    document.head.appendChild(style);
  }
  __name(injectSpinnerStyles, "injectSpinnerStyles");
  function createSpinner(options = {}) {
    injectSpinnerStyles();
    const state = reactive({
      type: "border",
      size: 40,
      color: "var(--color-primary)",
      text: "",
      ...options
    });
    const el = document.createElement("div");
    el.style.cssText = "display:inline-flex;flex-direction:column;align-items:center;gap:var(--space-8)";
    const _effects = [];
    const _addEffect = /* @__PURE__ */ __name((e) => {
      _effects.push(e);
      return e;
    }, "_addEffect");
    _addEffect(effect(() => {
      el.innerHTML = "";
      if (state.type === "border") {
        const ring = document.createElement("div");
        const s = state.size + "px";
        Object.assign(ring.style, {
          width: s,
          height: s,
          border: "var(--space-4) solid var(--color-border)",
          borderTopColor: state.color,
          borderRadius: "50%",
          animation: "petui-spin 0.8s linear infinite"
        });
        el.appendChild(ring);
      } else {
        const dotsContainer = document.createElement("div");
        dotsContainer.style.cssText = "display:flex;gap:var(--space-6);align-items:center";
        for (let i = 0; i < 3; i++) {
          const dot = document.createElement("div");
          const dotSize = Math.max(8, state.size / 4) + "px";
          Object.assign(dot.style, {
            width: dotSize,
            height: dotSize,
            borderRadius: "50%",
            background: state.color,
            animation: `petui-dot-bounce 1.4s infinite ${i * 0.2}s`
          });
          dotsContainer.appendChild(dot);
        }
        el.appendChild(dotsContainer);
      }
      if (state.text) {
        const textEl = document.createElement("span");
        textEl.textContent = state.text;
        textEl.style.cssText = `color:var(--color-text-secondary);font-size:0.85rem`;
        el.appendChild(textEl);
      }
    }));
    return Object.assign(el, {
      setProps(props) {
        Object.assign(state, props);
      },
      destroy() {
        _effects.forEach((e) => e.stop());
        _effects.length = 0;
        el.remove();
      }
    });
  }
  __name(createSpinner, "createSpinner");

  // src/components/badge-tag-alert.ts
  var TONE_TOKENS = Object.freeze({
    info: Object.freeze({ bg: "--color-n-e8f4fd", border: "--color-n-91caf0", text: "--color-n-1a6daa" }),
    success: Object.freeze({ bg: "--color-n-e8f8e8", border: "--color-n-8cd98c", text: "--color-n-2d7a2d" }),
    warning: Object.freeze({ bg: "--color-n-fef8e7", border: "--color-n-f0c040", text: "--color-n-8a6d20" }),
    error: Object.freeze({ bg: "--color-n-fde8e8", border: "--color-n-f09898", text: "--color-n-a82d2d" })
  });
  var TONE_KEYS = Object.freeze(Object.keys(TONE_TOKENS));
  var FALLBACK_TONE = TONE_TOKENS[TONE_KEYS[0]];
  var _warnedTones = /* @__PURE__ */ new Set();
  var TOKEN_NAME_RE = /^--[a-z0-9-]+$/;
  var _LiteralColorError = class _LiteralColorError extends Error {
    constructor(slot, value) {
      super("[K11] toneMap \u7684 " + slot + " \u5FC5\u987B\u662F\u4EE4\u724C\u540D\uFF08--...\uFF09\uFF0C\u6536\u5230\u7684\u662F\u5B57\u9762\u8272\u503C <" + value + ">");
      this.name = "LiteralColorError";
      this.slot = slot;
      this.value = value;
    }
  };
  __name(_LiteralColorError, "LiteralColorError");
  var LiteralColorError = _LiteralColorError;
  function assertTokenSlots(table, where) {
    for (const slot of ["bg", "border", "text"]) {
      const v = table[slot];
      if (v === void 0) continue;
      if (typeof v !== "string" || !TOKEN_NAME_RE.test(v)) {
        throw new LiteralColorError(where + "." + slot, String(v));
      }
    }
  }
  __name(assertTokenSlots, "assertTokenSlots");
  function resolveTone(tone, toneMap) {
    if (toneMap) {
      for (const key2 of Object.keys(toneMap)) {
        const entry = toneMap[key2];
        if (entry) assertTokenSlots(entry, "toneMap." + key2);
      }
    }
    const key = typeof tone === "string" ? tone : "";
    const base = TONE_TOKENS[key];
    if (!base) {
      if (!_warnedTones.has(key)) {
        _warnedTones.add(key);
        console.warn("[K11] \u672A\u77E5 tone <" + key + ">\uFF0C\u843D\u5230\u515C\u5E95 tone <" + TONE_KEYS[0] + ">");
      }
    }
    const picked = base || FALLBACK_TONE;
    const override = toneMap ? toneMap[key] : void 0;
    return override ? { ...picked, ...override } : picked;
  }
  __name(resolveTone, "resolveTone");
  function tokVar(name) {
    return "var(" + name + ")";
  }
  __name(tokVar, "tokVar");
  function createBadge(options = {}) {
    const state = reactive({
      content: "",
      color: "var(--color-primary)",
      dot: false,
      ...options
    });
    const el = document.createElement("span");
    const _effects = [];
    const _addEffect = /* @__PURE__ */ __name((e) => {
      _effects.push(e);
      return e;
    }, "_addEffect");
    _addEffect(effect(() => {
      el.innerHTML = "";
      if (state.dot) {
        Object.assign(el.style, {
          display: "inline-block",
          width: "var(--space-10)",
          height: "var(--space-10)",
          borderRadius: "50%",
          background: state.color,
          verticalAlign: "middle"
        });
      } else {
        const text = state.content !== void 0 ? String(state.content) : "";
        el.textContent = text;
        Object.assign(el.style, {
          display: "inline-block",
          padding: "var(--space-2) var(--space-8)",
          borderRadius: "var(--space-10)",
          background: state.color,
          color: "var(--color-bg-light)",
          fontSize: "0.75rem",
          fontWeight: "600",
          lineHeight: "1.4",
          textAlign: "center",
          minWidth: text.length <= 1 ? "var(--space-18)" : "auto"
        });
      }
    }));
    return Object.assign(el, {
      setProps(props) {
        Object.assign(state, props);
      },
      setContent(v) {
        state.content = v;
      },
      destroy() {
        _effects.forEach((e) => e.stop());
        _effects.length = 0;
        el.remove();
      }
    });
  }
  __name(createBadge, "createBadge");
  function createTag(options = {}) {
    const state = reactive({
      text: "",
      color: "var(--color-primary-hover)",
      closable: false,
      visible: true,
      onClose: null,
      ...options
    });
    const el = document.createElement("span");
    const _effects = [];
    const _addEffect = /* @__PURE__ */ __name((e) => {
      _effects.push(e);
      return e;
    }, "_addEffect");
    _addEffect(effect(() => {
      el.innerHTML = "";
      if (!state.visible) return;
      el.style.cssText = [
        "display:inline-flex",
        "align-items:center",
        "gap:var(--space-4)",
        "padding:var(--space-2) var(--space-10)",
        "border-radius:var(--space-4)",
        "background:" + state.color,
        "color:var(--color-text-secondary)",
        "font-size:0.8rem",
        "border:var(--space-1) solid " + state.color
      ].join(";");
      const textEl = document.createElement("span");
      textEl.textContent = state.text;
      el.appendChild(textEl);
      if (state.closable) {
        const closeBtn = document.createElement("span");
        closeBtn.textContent = "\u2715";
        closeBtn.style.cssText = "cursor:pointer;font-size:0.7rem;margin-left:var(--space-2);opacity:0.6";
        closeBtn.addEventListener("click", (e) => {
          e.stopPropagation();
          state.visible = false;
          if (state.onClose) state.onClose();
        });
        el.appendChild(closeBtn);
      }
    }));
    return Object.assign(el, {
      setProps(props) {
        Object.assign(state, props);
      },
      close() {
        state.visible = false;
        if (state.onClose) state.onClose();
      },
      destroy() {
        _effects.forEach((e) => e.stop());
        _effects.length = 0;
        el.remove();
      }
    });
  }
  __name(createTag, "createTag");
  function createAlert(options = {}) {
    const state = reactive({
      message: "",
      type: "info",
      closable: false,
      visible: true,
      onClose: null,
      ...options
    });
    const el = document.createElement("div");
    const _effects = [];
    const _addEffect = /* @__PURE__ */ __name((e) => {
      _effects.push(e);
      return e;
    }, "_addEffect");
    _addEffect(effect(() => {
      el.innerHTML = "";
      if (!state.visible) return;
      const colors = resolveTone(state.type || "info");
      const background = state.color && state.color !== "var(--color-primary)" ? TOKEN_NAME_RE.test(state.color) ? tokVar(state.color) : state.color : tokVar(colors.bg);
      el.style.cssText = [
        "display:flex",
        "align-items:center",
        "justify-content:space-between",
        "gap:var(--space-12)",
        "padding:var(--space-10) var(--space-16)",
        "border-radius:var(--space-8)",
        "background:" + background,
        "border:var(--space-1) solid " + tokVar(colors.border),
        "color:" + tokVar(colors.text),
        "font-size:0.95rem"
      ].join(";");
      const msgEl = document.createElement("span");
      msgEl.textContent = state.message || "";
      el.appendChild(msgEl);
      if (state.closable) {
        const closeBtn = document.createElement("span");
        closeBtn.textContent = "\u2715";
        closeBtn.style.cssText = "cursor:pointer;opacity:0.7;font-size:0.9rem";
        closeBtn.addEventListener("click", () => {
          state.visible = false;
          if (state.onClose) state.onClose();
        });
        el.appendChild(closeBtn);
      }
    }));
    return Object.assign(el, {
      setProps(props) {
        Object.assign(state, props);
      },
      close() {
        state.visible = false;
        if (state.onClose) state.onClose();
      },
      destroy() {
        _effects.forEach((e) => e.stop());
        _effects.length = 0;
        el.remove();
      }
    });
  }
  __name(createAlert, "createAlert");

  // src/components/avatar-divider-code.ts
  function createAvatar(options = {}) {
    const state = reactive({
      src: "",
      text: "",
      size: 40,
      shape: "circle",
      ...options
    });
    const el = document.createElement("span");
    const _effects = [];
    const _addEffect = /* @__PURE__ */ __name((e) => {
      _effects.push(e);
      return e;
    }, "_addEffect");
    _addEffect(effect(() => {
      el.innerHTML = "";
      const s = state.size + "px";
      const borderRadius = state.shape === "circle" ? "50%" : "var(--space-8)";
      if (state.src) {
        const img = document.createElement("img");
        img.src = state.src;
        img.alt = state.text || "";
        img.style.cssText = [
          "width:" + s,
          "height:" + s,
          "border-radius:" + borderRadius,
          "object-fit:cover",
          "display:block"
        ].join(";");
        el.appendChild(img);
      } else {
        el.style.cssText = [
          "display:inline-flex",
          "align-items:center",
          "justify-content:center",
          "width:" + s,
          "height:" + s,
          "border-radius:" + borderRadius,
          "background:var(--color-primary-hover)",
          "color:var(--color-primary-dark)",
          "font-weight:600",
          "font-size:" + state.size * 0.4 + "px",
          "overflow:hidden"
        ].join(";");
        el.textContent = state.text ? state.text.charAt(0).toUpperCase() : "";
      }
    }));
    return Object.assign(el, {
      setProps(props) {
        Object.assign(state, props);
      },
      destroy() {
        _effects.forEach((e) => e.stop());
        _effects.length = 0;
        el.remove();
      }
    });
  }
  __name(createAvatar, "createAvatar");
  function createDivider(options = {}) {
    const state = reactive({
      text: "",
      direction: "horizontal",
      ...options
    });
    const el = document.createElement("div");
    const _effects = [];
    const _addEffect = /* @__PURE__ */ __name((e) => {
      _effects.push(e);
      return e;
    }, "_addEffect");
    _addEffect(effect(() => {
      el.innerHTML = "";
      if (state.direction === "vertical") {
        el.style.cssText = "display:inline-block;width:var(--space-1);height:100%;background:var(--color-border);vertical-align:middle";
      } else if (state.text) {
        el.style.cssText = "display:flex;align-items:center;gap:var(--space-12);width:100%";
        const left = document.createElement("div");
        left.style.cssText = "flex:1;height:var(--space-1);background:var(--color-border)";
        const center = document.createElement("span");
        center.textContent = state.text;
        center.style.cssText = "color:var(--color-text-secondary);font-size:0.85rem;white-space:nowrap";
        const right = document.createElement("div");
        right.style.cssText = "flex:1;height:var(--space-1);background:var(--color-border)";
        el.appendChild(left);
        el.appendChild(center);
        el.appendChild(right);
      } else {
        el.style.cssText = "width:100%;height:var(--space-1);background:var(--color-border)";
      }
    }));
    return Object.assign(el, {
      setProps(props) {
        Object.assign(state, props);
      },
      destroy() {
        _effects.forEach((e) => e.stop());
        _effects.length = 0;
        el.remove();
      }
    });
  }
  __name(createDivider, "createDivider");
  injectStyleRules([
    ".petui-code-line:hover{background:var(--color-border)}",
    ".petui-code-root:focus-visible{outline:var(--space-1) solid var(--color-primary);outline-offset:calc(var(--space-1) * -1)}",
    // 行号可点击（与代码行同语义）⇒ 光标提示 + 悬停反馈；**不加**鼠标监听（纯 CSS）。
    ".petui-code-gutter-line{cursor:pointer}",
    ".petui-code-gutter-line:hover{color:var(--color-primary)}"
  ]);
  var PETUI_CODE_ACTIVE_CLASS = "petui-code-active";
  function splitCodeLines(code) {
    return String(code == null ? "" : code).split("\n");
  }
  __name(splitCodeLines, "splitCodeLines");
  var CODE_JS_KEYWORDS = [
    "as",
    "async",
    "await",
    "break",
    "case",
    "catch",
    "class",
    "const",
    "continue",
    "debugger",
    "default",
    "delete",
    "do",
    "else",
    "export",
    "extends",
    "false",
    "finally",
    "for",
    "from",
    "function",
    "get",
    "if",
    "import",
    "in",
    "instanceof",
    "let",
    "new",
    "null",
    "of",
    "return",
    "set",
    "static",
    "super",
    "switch",
    "this",
    "throw",
    "true",
    "try",
    "typeof",
    "undefined",
    "var",
    "void",
    "while",
    "with",
    "yield"
  ];
  var CODE_TS_KEYWORDS = CODE_JS_KEYWORDS.concat([
    "abstract",
    "any",
    "boolean",
    "declare",
    "enum",
    "implements",
    "interface",
    "is",
    "keyof",
    "namespace",
    "never",
    "number",
    "private",
    "protected",
    "public",
    "readonly",
    "string",
    "symbol",
    "type",
    "unknown"
  ]);
  var CODE_PY_KEYWORDS = [
    "False",
    "None",
    "True",
    "and",
    "as",
    "assert",
    "async",
    "await",
    "break",
    "class",
    "continue",
    "def",
    "del",
    "elif",
    "else",
    "except",
    "finally",
    "for",
    "from",
    "global",
    "if",
    "import",
    "in",
    "is",
    "lambda",
    "nonlocal",
    "not",
    "or",
    "pass",
    "raise",
    "return",
    "try",
    "while",
    "with",
    "yield"
  ];
  var CODE_CSS_KEYWORDS = [
    "charset",
    "container",
    "font-face",
    "import",
    "inherit",
    "initial",
    "important",
    "keyframes",
    "layer",
    "media",
    "namespace",
    "none",
    "page",
    "property",
    "supports",
    "unset"
  ];
  var CODE_HTML_KEYWORDS = [
    "a",
    "body",
    "button",
    "code",
    "div",
    "em",
    "footer",
    "form",
    "head",
    "header",
    "html",
    "img",
    "input",
    "label",
    "li",
    "link",
    "main",
    "meta",
    "nav",
    "ol",
    "option",
    "p",
    "pre",
    "script",
    "section",
    "select",
    "span",
    "strong",
    "style",
    "table",
    "tbody",
    "td",
    "th",
    "thead",
    "title",
    "tr",
    "ul"
  ];
  var CODE_LANGUAGES = {
    js: { lineComments: ["//"], blocks: [["/*", "*/"]], quotes: ["`", '"', "'"], multiline: ["`"], keywords: CODE_JS_KEYWORDS },
    ts: { lineComments: ["//"], blocks: [["/*", "*/"]], quotes: ["`", '"', "'"], multiline: ["`"], keywords: CODE_TS_KEYWORDS },
    json: { lineComments: [], blocks: [], quotes: ['"'], multiline: [], keywords: ["true", "false", "null"] },
    python: { lineComments: ["#"], blocks: [], quotes: ['"""', "'''", '"', "'"], multiline: ['"""', "'''"], keywords: CODE_PY_KEYWORDS },
    css: { lineComments: [], blocks: [["/*", "*/"]], quotes: ['"', "'"], multiline: [], keywords: CODE_CSS_KEYWORDS },
    html: { lineComments: [], blocks: [["<!--", "-->"]], quotes: ['"', "'"], multiline: [], keywords: CODE_HTML_KEYWORDS }
  };
  var CODE_LANGUAGE_ALIASES = {
    js: "js",
    javascript: "js",
    mjs: "js",
    cjs: "js",
    jsx: "js",
    ts: "ts",
    typescript: "ts",
    tsx: "ts",
    json: "json",
    py: "python",
    python: "python",
    python3: "python",
    css: "css",
    html: "html",
    htm: "html"
  };
  var CODE_TOKEN_COLORS = {
    kw: "var(--color-primary)",
    str: "var(--color-text-secondary)",
    num: "var(--color-primary)",
    com: "var(--color-text-muted)",
    pun: "var(--color-text-muted)"
  };
  var CODE_PUNCT = "{}()[];,.:?=+-*/%&|^!<>~@";
  var CODE_NUMBER_RE = /^(?:0[xX][0-9a-fA-F_]+|0[bB][01_]+|0[oO][0-7_]+|(?:[0-9][0-9_]*\.?[0-9_]*|\.[0-9][0-9_]*)(?:[eE][+-]?[0-9]+)?)/;
  var warnedCodeLanguages = /* @__PURE__ */ new Set();
  function warnUnknownCodeLanguage(raw) {
    const name = String(raw == null ? "" : raw).trim();
    if (name.length === 0 || warnedCodeLanguages.has(name)) return;
    warnedCodeLanguages.add(name);
    console.warn('[PetUI code] \u672A\u77E5 language "' + name + '"\uFF1A\u4E0D\u731C\u6D4B\u8BED\u6CD5\uFF0C\u5DF2\u9000\u5316\u4E3A\u7EAF\u6587\u672C');
  }
  __name(warnUnknownCodeLanguage, "warnUnknownCodeLanguage");
  function resolveCodeLanguage(raw) {
    const key = String(raw == null ? "" : raw).trim().toLowerCase();
    if (key.length === 0) return null;
    const canonical = CODE_LANGUAGE_ALIASES[key];
    if (canonical === void 0) return null;
    const lang = CODE_LANGUAGES[canonical];
    return lang === void 0 ? null : lang;
  }
  __name(resolveCodeLanguage, "resolveCodeLanguage");
  function isCodeDigit(ch) {
    return ch >= "0" && ch <= "9";
  }
  __name(isCodeDigit, "isCodeDigit");
  function isCodeIdentStart(ch) {
    return ch >= "a" && ch <= "z" || ch >= "A" && ch <= "Z" || ch === "_" || ch === "$";
  }
  __name(isCodeIdentStart, "isCodeIdentStart");
  function isCodeIdentPart(ch) {
    return isCodeIdentStart(ch) || isCodeDigit(ch);
  }
  __name(isCodeIdentPart, "isCodeIdentPart");
  function scanCodeRun(line, from, terminator) {
    let text = "";
    let i = from;
    while (i < line.length) {
      const ch = line[i];
      if (ch === "\\") {
        text += line.slice(i, i + 2);
        i += 2;
        continue;
      }
      if (terminator !== null && line.startsWith(terminator, i)) {
        return { text: text + terminator, next: i + terminator.length, closed: true };
      }
      text += ch;
      i += 1;
    }
    return { text, next: i, closed: terminator === null };
  }
  __name(scanCodeRun, "scanCodeRun");
  function tokenizeCodeLine(line, lang, scan) {
    const out = [];
    let plain = "";
    const flush = /* @__PURE__ */ __name(() => {
      if (plain.length > 0) {
        out.push({ kind: "txt", text: plain });
        plain = "";
      }
    }, "flush");
    let i = 0;
    while (i < line.length) {
      if (scan.quote !== null) {
        const run = scanCodeRun(line, i, scan.quote);
        flush();
        out.push({ kind: "str", text: run.text });
        i = run.next;
        if (run.closed) scan.quote = null;
        continue;
      }
      if (scan.block !== null) {
        const run = scanCodeRun(line, i, scan.block);
        flush();
        out.push({ kind: "com", text: run.text });
        i = run.next;
        if (run.closed) scan.block = null;
        continue;
      }
      const lineComment = lang.lineComments.find((c) => line.startsWith(c, i));
      if (lineComment !== void 0) {
        flush();
        out.push({ kind: "com", text: line.slice(i) });
        i = line.length;
        continue;
      }
      const block = lang.blocks.find((b) => line.startsWith(b[0], i));
      if (block !== void 0) {
        scan.block = block[1];
        flush();
        out.push({ kind: "com", text: block[0] });
        i += block[0].length;
        continue;
      }
      const quote = lang.quotes.find((q) => line.startsWith(q, i));
      if (quote !== void 0) {
        const run = scanCodeRun(line, i + quote.length, quote);
        flush();
        out.push({ kind: "str", text: quote + run.text });
        i = run.next;
        if (!run.closed && lang.multiline.indexOf(quote) >= 0) scan.quote = quote;
        continue;
      }
      const ch = line[i];
      if (isCodeIdentStart(ch)) {
        let j = i;
        while (j < line.length && isCodeIdentPart(line[j])) j += 1;
        const word = line.slice(i, j);
        if (lang.keywords.indexOf(word) >= 0) {
          flush();
          out.push({ kind: "kw", text: word });
        } else plain += word;
        i = j;
        continue;
      }
      if (isCodeDigit(ch) || ch === "." && isCodeDigit(line[i + 1] || "")) {
        const m = CODE_NUMBER_RE.exec(line.slice(i));
        if (m !== null && m[0].length > 0) {
          flush();
          out.push({ kind: "num", text: m[0] });
          i += m[0].length;
          continue;
        }
      }
      if (CODE_PUNCT.indexOf(ch) >= 0) {
        flush();
        out.push({ kind: "pun", text: ch });
        i += 1;
        continue;
      }
      plain += ch;
      i += 1;
    }
    flush();
    return out;
  }
  __name(tokenizeCodeLine, "tokenizeCodeLine");
  function writeCodeLine(host, source, tokens) {
    if (tokens === null) {
      host.textContent = source;
      return;
    }
    for (let i = 0; i < tokens.length; i += 1) {
      const token = tokens[i];
      if (token.kind === "txt") {
        host.appendChild(document.createTextNode(token.text));
        continue;
      }
      const span = document.createElement("span");
      span.setAttribute("data-token", token.kind);
      span.textContent = token.text;
      span.style.setProperty("color", CODE_TOKEN_COLORS[token.kind]);
      host.appendChild(span);
    }
  }
  __name(writeCodeLine, "writeCodeLine");
  function clampCodeLine(v, lineCount) {
    if (typeof v !== "number" || !Number.isFinite(v)) return null;
    const max = lineCount < 1 ? 1 : lineCount;
    const n = Math.floor(v);
    if (n < 1) return 1;
    if (n > max) return max;
    return n;
  }
  __name(clampCodeLine, "clampCodeLine");
  function createCode(options = {}) {
    const startLineCount = splitCodeLines(options.code == null ? "" : String(options.code)).length;
    const state = reactive({
      code: typeof options.code === "string" ? options.code : "",
      language: typeof options.language === "string" ? options.language : "",
      highlight: options.highlight !== false,
      showLineNumbers: options.showLineNumbers !== false,
      // 回调**不**进响应式状态：它不参与渲染，进了只会让构造器触发一次无意义的调度
      onLineClick: void 0,
      activeLine: clampCodeLine(options.activeLine, startLineCount)
    });
    const onLineClick = typeof options.onLineClick === "function" ? options.onLineClick : null;
    const el = document.createElement("div");
    el.className = "petui-code-root";
    el.setAttribute("tabindex", "0");
    const disposer = createDisposer();
    let disposed = false;
    let lineCount = startLineCount;
    let activeLine = clampCodeLine(options.activeLine, startLineCount);
    let lineEls = [];
    let gutterEls = [];
    function clearChildren5(host) {
      while (host.firstChild) host.removeChild(host.firstChild);
    }
    __name(clearChildren5, "clearChildren");
    function applyActive() {
      for (let i = 0; i < lineEls.length; i += 1) {
        const on = activeLine !== null && i + 1 === activeLine;
        if (on) lineEls[i].style.setProperty("background", "var(--color-border)");
        else lineEls[i].style.removeProperty("background");
        if (on) lineEls[i].classList.add(PETUI_CODE_ACTIVE_CLASS);
        else lineEls[i].classList.remove(PETUI_CODE_ACTIVE_CLASS);
        lineEls[i].style.setProperty("border-left-color", on ? "var(--color-primary)" : "transparent");
        const gut = gutterEls[i];
        if (gut) {
          if (on) {
            gut.style.setProperty("color", "var(--color-primary)");
            gut.style.setProperty("font-weight", "600");
          } else {
            gut.style.removeProperty("color");
            gut.style.removeProperty("font-weight");
          }
        }
      }
    }
    __name(applyActive, "applyActive");
    function applyActiveLine(next) {
      activeLine = clampCodeLine(next, lineCount);
      state.activeLine = activeLine;
      applyActive();
    }
    __name(applyActiveLine, "applyActiveLine");
    const _effects = [];
    const _addEffect = /* @__PURE__ */ __name((e) => {
      _effects.push(e);
      return e;
    }, "_addEffect");
    _addEffect(effect(() => {
      if (disposed) return;
      clearChildren5(el);
      el.style.cssText = [
        "background:var(--color-bg)",
        "border:var(--space-1) solid var(--color-border)",
        "border-radius:var(--border-radius-md)",
        "padding:var(--space-16)",
        "overflow-x:auto",
        'font-family:"Fira Code",Consolas,monospace',
        "font-size:var(--font-size-13)",
        "color:var(--color-text)",
        "line-height:1.6",
        "position:relative"
      ].join(";");
      const body = document.createElement("div");
      body.style.cssText = "display:flex;align-items:flex-start;width:max-content;min-width:100%;margin:calc(var(--space-16) * -1)";
      if (state.language) {
        const langTag = document.createElement("div");
        langTag.setAttribute("data-role", "code-lang");
        langTag.textContent = state.language;
        langTag.style.cssText = "position:absolute;top:var(--space-6);right:var(--font-size-12);font-size:0.7rem;color:var(--color-text-secondary)";
        el.appendChild(langTag);
      }
      const lines = splitCodeLines(state.code);
      lineCount = lines.length;
      activeLine = clampCodeLine(state.activeLine, lineCount);
      const highlightOn = state.highlight !== false;
      const lang = highlightOn ? resolveCodeLanguage(state.language) : null;
      if (highlightOn && lang === null && String(state.language == null ? "" : state.language).length > 0) {
        warnUnknownCodeLanguage(String(state.language));
      }
      const scan = { block: null, quote: null };
      const gutter = state.showLineNumbers !== false ? document.createElement("div") : null;
      if (gutter) {
        gutter.setAttribute("data-role", "code-gutter");
        const gutterDigits = Math.max(2, String(lineCount).length);
        gutter.style.cssText = `flex:0 0 auto;text-align:center;color:var(--color-text-muted);user-select:none;white-space:pre;box-sizing:border-box;width:calc(${gutterDigits}ch + var(--space-8) * 2);padding-left:var(--space-6);padding-right:var(--space-6);border-right:var(--space-1) solid var(--color-border);margin-right:var(--space-12)`;
      }
      const codeCol = document.createElement("pre");
      codeCol.setAttribute("data-role", "code-pre");
      codeCol.style.cssText = "margin:0;white-space:pre;flex:1 1 auto";
      gutterEls = [];
      if (gutter) {
        for (let i = 0; i < lines.length; i += 1) {
          const gut = document.createElement("span");
          gut.setAttribute("data-role", "code-gutter-line");
          gut.setAttribute("data-line", String(i + 1));
          gut.textContent = String(i + 1) + "\n";
          gutter.appendChild(gut);
          gutterEls.push(gut);
        }
      }
      lineEls = [];
      for (let i = 0; i < lines.length; i += 1) {
        const lineNo = i + 1;
        const line = document.createElement("div");
        line.setAttribute("data-role", "code-line");
        line.setAttribute("data-line", String(lineNo));
        line.style.cssText = "white-space:pre;border-left-style:solid;border-left-width:var(--space-1);border-left-color:transparent";
        writeCodeLine(line, lines[i], lang === null ? null : tokenizeCodeLine(lines[i], lang, scan));
        codeCol.appendChild(line);
        lineEls.push(line);
      }
      if (gutter) body.appendChild(gutter);
      body.appendChild(codeCol);
      el.appendChild(body);
      applyActive();
    }));
    function lineElFromEvent(ev) {
      const t = ev.target;
      if (!t || typeof t.closest !== "function") return null;
      return t.closest('[data-role="code-line"]');
    }
    __name(lineElFromEvent, "lineElFromEvent");
    function gutterLineFromEvent(ev) {
      const t = ev.target;
      if (!t || typeof t.closest !== "function") return null;
      const gut = t.closest('[data-role="code-gutter-line"]');
      if (!gut) return null;
      const v = Number(gut.getAttribute("data-line"));
      return Number.isFinite(v) ? Math.floor(v) : null;
    }
    __name(gutterLineFromEvent, "gutterLineFromEvent");
    function onClick(ev) {
      if (disposed) return;
      let n = null;
      const lineEl = lineElFromEvent(ev);
      if (lineEl) {
        const v = Number(lineEl.getAttribute("data-line"));
        if (Number.isFinite(v)) n = v;
      } else {
        n = gutterLineFromEvent(ev);
      }
      if (n === null) return;
      applyActiveLine(n);
      if (onLineClick) {
        try {
          onLineClick(n);
        } catch (err) {
          console.error("[PetUI code] onLineClick \u56DE\u8C03\u629B\u9519\uFF08\u5DF2\u6355\u83B7\uFF0C\u5F53\u524D\u884C\u4E0D\u53D7\u5F71\u54CD\uFF09:", err);
        }
      }
    }
    __name(onClick, "onClick");
    const CODE_PAGE_STEP = 10;
    function onKeydown(ev) {
      if (disposed) return;
      const ke = ev;
      const key = ke.key;
      const from = /* @__PURE__ */ __name((fallbackBeforeFirst) => {
        if (activeLine !== null) return activeLine;
        return fallbackBeforeFirst ? 0 : lineCount + 1;
      }, "from");
      let next = null;
      if (key === "ArrowDown") next = from(true) + 1;
      else if (key === "ArrowUp") next = from(false) - 1;
      else if (key === "PageDown") next = from(true) + CODE_PAGE_STEP;
      else if (key === "PageUp") next = from(false) - CODE_PAGE_STEP;
      else if (key === "Home") next = 1;
      else if (key === "End") next = lineCount;
      else return;
      if (typeof ke.preventDefault === "function") ke.preventDefault();
      applyActiveLine(next);
    }
    __name(onKeydown, "onKeydown");
    disposer.addEventListener(el, "click", onClick);
    disposer.addEventListener(el, "keydown", onKeydown);
    Object.defineProperty(el, "activeLine", {
      get: /* @__PURE__ */ __name(() => activeLine, "get"),
      enumerable: true,
      configurable: true
    });
    Object.defineProperty(el, "lineCount", {
      get: /* @__PURE__ */ __name(() => lineCount, "get"),
      enumerable: true,
      configurable: true
    });
    return Object.assign(el, {
      setProps(props) {
        Object.assign(state, props);
      },
      setCode(v) {
        state.code = v;
        const next = splitCodeLines(v).length;
        state.activeLine = clampCodeLine(state.activeLine, next);
      },
      setActiveLine(n) {
        applyActiveLine(n);
      },
      destroy() {
        if (disposed) return;
        disposed = true;
        _effects.forEach((e) => e.stop());
        _effects.length = 0;
        lineEls = [];
        gutterEls = [];
        try {
          disposer.dispose();
        } catch (err) {
          console.error("[PetUI code] destroy \u91CA\u653E\u52A8\u4F5C\u629B\u9519:", err);
        }
        el.remove();
      }
    });
  }
  __name(createCode, "createCode");

  // src/components/navbar.ts
  function createNavbar(options = {}) {
    const state = reactive({
      items: [],
      activeIndex: 0,
      onItemClick: null,
      ...options
    });
    const el = document.createElement("nav");
    const _effects = [];
    const _addEffect = /* @__PURE__ */ __name((e) => {
      _effects.push(e);
      return e;
    }, "_addEffect");
    _addEffect(effect(() => {
      el.innerHTML = "";
      el.style.cssText = [
        "display:flex",
        "gap:0",
        "border-bottom:var(--space-1) solid var(--color-border)",
        "padding:0",
        "margin:0",
        "overflow-x:auto"
      ].join(";");
      state.items.forEach((label, idx) => {
        const item = document.createElement("div");
        const isActive = idx === state.activeIndex;
        item.textContent = label;
        item.style.cssText = [
          "padding:var(--space-10) var(--space-16)",
          "cursor:pointer",
          "font-size:0.95rem",
          "color:" + (isActive ? "var(--color-primary)" : "var(--color-text-secondary)"),
          "border-bottom:" + (isActive ? "var(--space-3) solid var(--color-primary)" : "var(--space-3) solid transparent"),
          "transition:all 0.2s",
          "white-space:nowrap",
          "font-weight:" + (isActive ? "600" : "400")
        ].join(";");
        item.addEventListener("click", () => {
          state.activeIndex = idx;
          if (state.onItemClick) state.onItemClick(idx, label);
        });
        item.onmouseenter = () => {
          if (!isActive) item.style.color = "var(--color-primary)";
        };
        item.onmouseleave = () => {
          if (!isActive) item.style.color = "var(--color-text-secondary)";
        };
        el.appendChild(item);
      });
    }));
    return Object.assign(el, {
      setProps(props) {
        Object.assign(state, props);
      },
      setActiveItem(index) {
        state.activeIndex = index;
      },
      getActiveItem() {
        return state.activeIndex;
      },
      destroy() {
        _effects.forEach((e) => e.stop());
        _effects.length = 0;
        el.remove();
      }
    });
  }
  __name(createNavbar, "createNavbar");

  // src/components/tabs-bar.ts
  function buttonStyle(isActive) {
    return [
      "border:none",
      "background:" + (isActive ? "var(--color-primary)" : "transparent"),
      "color:" + (isActive ? "var(--color-bg-light)" : "var(--color-text-secondary)"),
      "padding:var(--space-8) var(--space-18)",
      "cursor:pointer",
      "font-size:0.9rem",
      "border-radius:var(--space-6) var(--space-6) 0 0",
      "transition:all 0.2s",
      "margin-right:var(--space-2)"
    ].join(";");
  }
  __name(buttonStyle, "buttonStyle");
  function createTabBar() {
    const el = document.createElement("div");
    el.style.cssText = "display:flex;border-bottom:var(--space-2) solid var(--color-primary-hover);gap:0";
    el.setAttribute("role", "tablist");
    const btns = [];
    let pick = null;
    const onClick = /* @__PURE__ */ __name((ev) => {
      const target = ev.target;
      const btn = target && typeof target.closest === "function" ? target.closest("button") : null;
      const index = btn ? btns.indexOf(btn) : -1;
      if (index >= 0 && pick) pick(index);
    }, "onClick");
    el.addEventListener("click", onClick);
    function ensure(count) {
      while (btns.length < count) {
        const btn = document.createElement("button");
        btn.setAttribute("type", "button");
        btn.setAttribute("role", "tab");
        btns.push(btn);
        el.appendChild(btn);
      }
      while (btns.length > count) {
        const gone = btns.pop();
        if (gone && gone.parentNode) gone.parentNode.removeChild(gone);
      }
    }
    __name(ensure, "ensure");
    function render(labels, active) {
      ensure(labels.length);
      btns.forEach((btn, idx) => {
        const isActive = idx === active;
        btn.textContent = labels[idx] === void 0 ? "" : String(labels[idx]);
        btn.style.cssText = buttonStyle(isActive);
        btn.setAttribute("aria-selected", isActive ? "true" : "false");
        btn.setAttribute("data-index", String(idx));
      });
    }
    __name(render, "render");
    return {
      el,
      render,
      onPick(fn) {
        pick = fn;
      },
      destroy() {
        el.removeEventListener("click", onClick);
        btns.length = 0;
        pick = null;
      }
    };
  }
  __name(createTabBar, "createTabBar");

  // src/components/tabs-content.ts
  function clearChildren(host) {
    while (host.firstChild) host.removeChild(host.firstChild);
  }
  __name(clearChildren, "clearChildren");
  function createTabPanes() {
    const el = document.createElement("div");
    el.style.cssText = [
      "padding:var(--space-16)",
      "color:var(--color-text-secondary)",
      "font-size:0.95rem",
      "line-height:1.6",
      "min-height:var(--space-60)"
    ].join(";");
    const list = [];
    function ensure(count) {
      while (list.length < count) {
        const pane = document.createElement("div");
        pane.setAttribute("data-role", "tab-pane");
        pane.setAttribute("data-index", String(list.length));
        list.push(pane);
        el.appendChild(pane);
      }
      return list;
    }
    __name(ensure, "ensure");
    function show(active) {
      list.forEach((pane, idx) => {
        pane.style.display = idx === active ? "" : "none";
      });
    }
    __name(show, "show");
    return {
      el,
      ensure,
      panes: /* @__PURE__ */ __name(() => list, "panes"),
      show,
      destroy() {
        clearChildren(el);
        list.length = 0;
      }
    };
  }
  __name(createTabPanes, "createTabPanes");

  // src/components/tabs.ts
  function createTabs(options = {}) {
    const state = reactive({
      tabs: [],
      activeIndex: 0,
      onChange: null,
      ...options
    });
    const el = document.createElement("div");
    const bar = createTabBar();
    const panes = createTabPanes();
    const lastText = /* @__PURE__ */ new WeakMap();
    const _effects = [];
    const _addEffect = /* @__PURE__ */ __name((e) => {
      _effects.push(e);
      return e;
    }, "_addEffect");
    el.appendChild(bar.el);
    el.appendChild(panes.el);
    bar.onPick((index) => {
      state.activeIndex = index;
      if (state.onChange) state.onChange(index);
    });
    function writePane(index, content) {
      const pane = panes.ensure(index + 1)[index];
      if (typeof content === "string") {
        if (lastText.get(pane) === content) return;
        lastText.set(pane, content);
        setSafeHTML(pane, content);
        return;
      }
      lastText.delete(pane);
      if (pane.firstChild !== content) {
        clearChildren(pane);
        pane.appendChild(content);
      }
    }
    __name(writePane, "writePane");
    _addEffect(effect(() => {
      bar.render(state.tabs.map((tab) => tab.label), state.activeIndex);
      state.tabs.forEach((tab, idx) => writePane(idx, tab.content));
      panes.show(state.activeIndex);
    }));
    return Object.assign(el, {
      setProps(props) {
        Object.assign(state, props);
      },
      setActive(index) {
        state.activeIndex = index;
      },
      getActive() {
        return state.activeIndex;
      },
      setTabContent(index, content) {
        const tab = state.tabs[index];
        if (tab) tab.content = content;
        else state.tabs[index] = { label: "", content };
      },
      getPane(index) {
        return panes.panes()[index] || null;
      },
      destroy() {
        _effects.forEach((e) => e.stop());
        _effects.length = 0;
        bar.destroy();
        panes.destroy();
        el.innerHTML = "";
        el.remove();
      }
    });
  }
  __name(createTabs, "createTabs");

  // src/components/breadcrumb-pagination-steps.ts
  function createBreadcrumb(options = {}) {
    const state = reactive({
      items: [],
      separator: "/",
      ...options
    });
    const el = document.createElement("nav");
    el.setAttribute("aria-label", "\u9762\u5305\u5C51\u5BFC\u822A");
    const _effects = [];
    const _addEffect = /* @__PURE__ */ __name((e) => {
      _effects.push(e);
      return e;
    }, "_addEffect");
    _addEffect(effect(() => {
      el.innerHTML = "";
      el.style.cssText = "display:flex;align-items:center;gap:var(--space-6);flex-wrap:wrap;font-size:0.9rem";
      state.items.forEach((item, idx) => {
        if (idx > 0) {
          const sep = document.createElement("span");
          sep.textContent = state.separator;
          sep.style.cssText = "color:var(--color-border);margin:0 var(--space-2)";
          el.appendChild(sep);
        }
        const isLast = idx === state.items.length - 1;
        if (item.href && !isLast) {
          const link = document.createElement("a");
          link.textContent = item.label;
          link.href = item.href;
          link.style.cssText = "color:var(--color-primary);text-decoration:none;cursor:pointer";
          el.appendChild(link);
        } else {
          const span = document.createElement("span");
          span.textContent = item.label;
          span.style.cssText = isLast ? "color:var(--color-text);font-weight:500" : "color:var(--color-text-secondary)";
          el.appendChild(span);
        }
      });
    }));
    return Object.assign(el, {
      setProps(props) {
        Object.assign(state, props);
      },
      destroy() {
        _effects.forEach((e) => e.stop());
        _effects.length = 0;
        el.remove();
      }
    });
  }
  __name(createBreadcrumb, "createBreadcrumb");
  function createPagination(options = {}) {
    const state = reactive({
      total: 0,
      current: 1,
      pageSize: 10,
      onChange: null,
      ...options
    });
    const el = document.createElement("div");
    const _effects = [];
    const _addEffect = /* @__PURE__ */ __name((e) => {
      _effects.push(e);
      return e;
    }, "_addEffect");
    _addEffect(effect(() => {
      el.innerHTML = "";
      const totalPages = Math.max(1, Math.ceil(state.total / state.pageSize));
      el.style.cssText = "display:flex;align-items:center;gap:var(--space-4);flex-wrap:wrap";
      const navAvail = /* @__PURE__ */ __name((direction) => {
        if (direction < 0 && state.current <= 1) {
          return { disabled: true, disabledReason: state.disabledReason || "\u5DF2\u5728\u7B2C\u4E00\u9875", onEscape: state.onChange };
        }
        if (direction > 0 && state.current >= totalPages) {
          return { disabled: true, disabledReason: state.disabledReason || "\u5DF2\u5728\u6700\u540E\u4E00\u9875", onEscape: state.onChange };
        }
        return { disabled: false, disabledReason: "" };
      }, "navAvail");
      function makeBtn(text, page, nav = { disabled: false, disabledReason: "" }) {
        const btn = document.createElement("button");
        btn.textContent = text;
        const isActive = page === state.current;
        const disabled = nav.disabled;
        btn.style.cssText = [
          "border:var(--space-1) solid " + (isActive ? "var(--color-primary)" : "var(--color-border)"),
          "border-radius:var(--space-4)",
          "padding:var(--space-4) var(--space-10)",
          "background:" + (isActive ? "var(--color-primary)" : "var(--color-bg-light)"),
          "color:" + (isActive ? "var(--color-bg-light)" : "var(--color-text-secondary)"),
          "cursor:" + (disabled ? "not-allowed" : "pointer"),
          "font-size:0.85rem",
          "min-width:var(--space-32)",
          "text-align:center",
          "opacity:" + (disabled ? 0.4 : 1)
        ].join(";");
        if (!disabled) {
          btn.addEventListener("click", () => {
            state.current = page;
            if (state.onChange) state.onChange(page);
          });
        }
        return btn;
      }
      __name(makeBtn, "makeBtn");
      el.appendChild(makeBtn("\u4E0A\u4E00\u9875", state.current - 1, navAvail(-1)));
      const maxVisible = 7;
      let startPage = Math.max(1, state.current - Math.floor(maxVisible / 2));
      let endPage = Math.min(totalPages, startPage + maxVisible - 1);
      if (endPage - startPage < maxVisible - 1) {
        startPage = Math.max(1, endPage - maxVisible + 1);
      }
      if (startPage > 1) {
        el.appendChild(makeBtn("1", 1));
        if (startPage > 2) {
          const ellipsis = document.createElement("span");
          ellipsis.textContent = "...";
          ellipsis.style.cssText = "padding:var(--space-4) var(--space-6);color:var(--color-text-secondary);font-size:0.85rem";
          el.appendChild(ellipsis);
        }
      }
      for (let p = startPage; p <= endPage; p++) {
        el.appendChild(makeBtn(String(p), p));
      }
      if (endPage < totalPages) {
        if (endPage < totalPages - 1) {
          const ellipsis = document.createElement("span");
          ellipsis.textContent = "...";
          ellipsis.style.cssText = "padding:var(--space-4) var(--space-6);color:var(--color-text-secondary);font-size:0.85rem";
          el.appendChild(ellipsis);
        }
        el.appendChild(makeBtn(String(totalPages), totalPages));
      }
      el.appendChild(makeBtn("\u4E0B\u4E00\u9875", state.current + 1, navAvail(1)));
    }));
    return Object.assign(el, {
      setProps(props) {
        Object.assign(state, props);
      },
      setPage(p) {
        const totalPages = Math.max(1, Math.ceil(state.total / state.pageSize));
        state.current = Math.min(totalPages, Math.max(1, p));
      },
      getPage() {
        return state.current;
      },
      destroy() {
        _effects.forEach((e) => e.stop());
        _effects.length = 0;
        el.remove();
      }
    });
  }
  __name(createPagination, "createPagination");
  function createSteps(options = {}) {
    const state = reactive({
      steps: [],
      current: 0,
      ...options
    });
    const el = document.createElement("div");
    const _effects = [];
    const _addEffect = /* @__PURE__ */ __name((e) => {
      _effects.push(e);
      return e;
    }, "_addEffect");
    _addEffect(effect(() => {
      el.innerHTML = "";
      el.style.cssText = "display:flex;align-items:flex-start;gap:0;flex-wrap:wrap";
      state.steps.forEach((step, idx) => {
        const stepWrap = document.createElement("div");
        stepWrap.style.cssText = "display:flex;align-items:flex-start;gap:0";
        const circle = document.createElement("div");
        const isDone = idx < state.current;
        const isActive = idx === state.current;
        const bgColor = isDone ? "var(--color-success)" : isActive ? "var(--color-primary)" : "var(--color-border)";
        const borderColor = isDone ? "var(--color-success)" : isActive ? "var(--color-primary)" : "var(--color-border)";
        const textColor = isDone || isActive ? "var(--color-bg-light)" : "var(--color-text-secondary)";
        circle.style.cssText = [
          "width:var(--space-26)",
          "height:var(--space-26)",
          "border-radius:50%",
          "display:flex",
          "align-items:center",
          "justify-content:center",
          "background:" + bgColor,
          "color:" + textColor,
          "font-size:0.75rem",
          "font-weight:600",
          "flex-shrink:0",
          "transition:all 0.3s"
        ].join(";");
        circle.textContent = isDone ? "\u2713" : String(idx + 1);
        const textWrap = document.createElement("div");
        textWrap.style.cssText = "margin-left:var(--space-8);display:flex;flex-direction:column";
        const titleEl = document.createElement("div");
        titleEl.textContent = step.title;
        titleEl.style.cssText = [
          "font-size:0.9rem",
          "font-weight:" + (isActive ? "600" : "400"),
          "color:" + (isActive ? "var(--color-text)" : "var(--color-text-secondary)")
        ].join(";");
        textWrap.appendChild(titleEl);
        if (step.description) {
          const descEl = document.createElement("div");
          descEl.textContent = step.description;
          descEl.style.cssText = "font-size:0.75rem;color:var(--color-border);margin-top:var(--space-2)";
          textWrap.appendChild(descEl);
        }
        stepWrap.appendChild(circle);
        stepWrap.appendChild(textWrap);
        if (idx < state.steps.length - 1) {
          const line = document.createElement("div");
          line.style.cssText = [
            "width:var(--space-40)",
            "height:var(--space-2)",
            "background:" + (idx < state.current ? "var(--color-success)" : "var(--color-border)"),
            "margin:var(--space-12) var(--space-4) 0",
            "flex-shrink:0",
            "transition:background 0.3s"
          ].join(";");
          stepWrap.appendChild(line);
        }
        el.appendChild(stepWrap);
      });
    }));
    return Object.assign(el, {
      setProps(props) {
        Object.assign(state, props);
      },
      setCurrent(index) {
        state.current = index;
      },
      destroy() {
        _effects.forEach((e) => e.stop());
        _effects.length = 0;
        el.remove();
      }
    });
  }
  __name(createSteps, "createSteps");

  // src/components/toast.ts
  var TOAST_COLORS = {
    info: "var(--color-info)",
    success: "var(--color-success)",
    warning: "var(--color-warning)",
    error: "var(--color-danger)"
  };
  var TOAST_STYLE_ID = "petui-toast-styles";
  function injectToastStyles() {
    if (document.getElementById(TOAST_STYLE_ID)) return;
    const style = document.createElement("style");
    style.id = TOAST_STYLE_ID;
    style.textContent = `
        @keyframes petui-toast-in {
            from { opacity:0; transform:translateY(-20px); }
            to { opacity:1; transform:translateY(0); }
        }
        @keyframes petui-toast-out {
            from { opacity:1; transform:translateY(0); }
            to { opacity:0; transform:translateY(-20px); }
        }
    `;
    document.head.appendChild(style);
  }
  __name(injectToastStyles, "injectToastStyles");
  function createToast(options = {}) {
    injectToastStyles();
    const state = reactive({
      message: "",
      type: "info",
      duration: 3e3,
      visible: false,
      ...options
    });
    const el = document.createElement("div");
    let timer = null;
    const _effects = [];
    const _addEffect = /* @__PURE__ */ __name((e) => {
      _effects.push(e);
      return e;
    }, "_addEffect");
    _addEffect(effect(() => {
      el.innerHTML = "";
      if (!state.visible) {
        el.style.display = "none";
        return;
      }
      el.style.cssText = [
        "position:fixed",
        "top:var(--space-20)",
        "left:50%",
        "transform:translateX(-50%)",
        "z-index:99999",
        "padding:var(--space-12) var(--space-24)",
        "border-radius:var(--space-8)",
        "background:var(--color-bg-light)",
        "color:var(--color-text)",
        "font-size:0.95rem",
        "box-shadow:0 4px 20px var(--color-alpha-shade-15)",
        "border-left:var(--space-4) solid " + (TOAST_COLORS[state.type || "info"] || "var(--color-info)"),
        "animation:petui-toast-in 0.3s",
        "align-items:center",
        "gap:var(--space-8)"
      ].join(";");
      el.style.display = "flex";
      const icon = document.createElement("span");
      const icons = {
        info: "\u2139",
        success: "\u2713",
        warning: "\u26A0",
        error: "\u2715"
      };
      icon.textContent = icons[state.type || "info"] || "";
      icon.style.cssText = "font-size:1rem";
      el.appendChild(icon);
      const msg = document.createElement("span");
      msg.textContent = state.message || "";
      el.appendChild(msg);
    }));
    function clearTimer() {
      if (timer) {
        clearTimeout(timer);
        timer = null;
      }
    }
    __name(clearTimer, "clearTimer");
    return Object.assign(el, {
      setProps(props) {
        Object.assign(state, props);
      },
      show() {
        clearTimer();
        state.visible = true;
        if (state.duration && state.duration > 0) {
          timer = setTimeout(() => {
            state.visible = false;
          }, state.duration);
        }
      },
      close() {
        clearTimer();
        state.visible = false;
      },
      destroy() {
        _effects.forEach((e) => e.stop());
        _effects.length = 0;
        clearTimer();
        el.remove();
      }
    });
  }
  __name(createToast, "createToast");

  // src/components/tooltip.ts
  function createTooltip(options) {
    let target = options.target;
    const { target: _target, ...restOptions } = options;
    const state = reactive({
      content: "\u63D0\u793A\u5185\u5BB9",
      position: "top",
      visible: false,
      ...restOptions
    });
    const el = document.createElement("div");
    const _effects = [];
    const _addEffect = /* @__PURE__ */ __name((e) => {
      _effects.push(e);
      return e;
    }, "_addEffect");
    _addEffect(effect(() => {
      el.innerHTML = "";
      if (!state.visible) {
        el.style.display = "none";
        return;
      }
      el.style.display = "block";
      el.textContent = state.content;
      const targetRect = target.getBoundingClientRect();
      const tooltipW = el.offsetWidth || 120;
      el.style.cssText = [
        "position:fixed",
        "z-index:99999",
        "padding:var(--space-6) var(--space-12)",
        "border-radius:var(--space-6)",
        "font-size:0.85rem",
        "background:var(--color-alpha-shade-75)",
        "color:var(--color-bg-light)",
        "white-space:nowrap",
        "pointer-events:none"
      ].join(";");
      const gap = 8;
      let left = 0, top = 0;
      switch (state.position) {
        case "top":
          left = targetRect.left + targetRect.width / 2 - tooltipW / 2;
          top = targetRect.top - el.offsetHeight - gap;
          break;
        case "bottom":
          left = targetRect.left + targetRect.width / 2 - tooltipW / 2;
          top = targetRect.bottom + gap;
          break;
        case "left":
          left = targetRect.left - tooltipW - gap;
          top = targetRect.top + targetRect.height / 2 - el.offsetHeight / 2;
          break;
        case "right":
          left = targetRect.right + gap;
          top = targetRect.top + targetRect.height / 2 - el.offsetHeight / 2;
          break;
      }
      el.style.left = Math.max(4, left) + "px";
      el.style.top = Math.max(4, top) + "px";
    }));
    function onMouseEnter() {
      state.visible = true;
    }
    __name(onMouseEnter, "onMouseEnter");
    function onMouseLeave() {
      state.visible = false;
    }
    __name(onMouseLeave, "onMouseLeave");
    target.addEventListener("mouseenter", onMouseEnter);
    target.addEventListener("mouseleave", onMouseLeave);
    document.body.appendChild(el);
    return Object.assign(el, {
      setProps(props) {
        const { target: newTarget, ...rest } = props;
        if (newTarget !== void 0 && newTarget !== target) {
          target.removeEventListener("mouseenter", onMouseEnter);
          target.removeEventListener("mouseleave", onMouseLeave);
          target = newTarget;
          target.addEventListener("mouseenter", onMouseEnter);
          target.addEventListener("mouseleave", onMouseLeave);
        }
        Object.assign(state, rest);
      },
      show() {
        state.visible = true;
      },
      hide() {
        state.visible = false;
      },
      destroy() {
        _effects.forEach((e) => e.stop());
        _effects.length = 0;
        target.removeEventListener("mouseenter", onMouseEnter);
        target.removeEventListener("mouseleave", onMouseLeave);
        el.remove();
      }
    });
  }
  __name(createTooltip, "createTooltip");

  // src/components/dropdown-popconfirm.ts
  var DROPDOWN_STYLE_ID = "petui-dropdown-styles";
  function injectDropdownStyles() {
    if (document.getElementById(DROPDOWN_STYLE_ID)) return;
    const style = document.createElement("style");
    style.id = DROPDOWN_STYLE_ID;
    style.textContent = `
        @keyframes petui-dropdown-in {
            from { opacity:0; transform:scale(0.95); }
            to { opacity:1; transform:scale(1); }
        }
    `;
    document.head.appendChild(style);
  }
  __name(injectDropdownStyles, "injectDropdownStyles");
  var FOCUSABLE_SELECTOR2 = 'a[href],button:not([disabled]),input:not([disabled]),select:not([disabled]),textarea:not([disabled]),[tabindex]:not([tabindex="-1"])';
  function focusables2(root) {
    const list = root.querySelectorAll(FOCUSABLE_SELECTOR2);
    const out = [];
    for (let i = 0; i < list.length; i += 1) {
      const el = list[i];
      if (el.getAttribute && el.getAttribute("tabindex") === "-1") continue;
      out.push(el);
    }
    return out;
  }
  __name(focusables2, "focusables");
  function isInDocument2(el) {
    return !!(el && typeof el.focus === "function" && el.isConnected !== false);
  }
  __name(isInDocument2, "isInDocument");
  function createDropdown(options = {}) {
    injectDropdownStyles();
    let trigger2 = options.trigger || document.createElement("span");
    const { trigger: _trigger, ...restOptions } = options;
    const state = reactive({
      items: [],
      placement: "bottom-left",
      open: false,
      ...restOptions
    });
    const el = document.createElement("div");
    el.style.cssText = "position:fixed;display:inline-block;z-index:var(--z-dropdown)";
    document.body.appendChild(el);
    const globalClick = /* @__PURE__ */ __name((e) => {
      if (!el.contains(e.target) && !trigger2.contains?.(e.target)) {
        state.open = false;
      }
    }, "globalClick");
    const _effects = [];
    const _addEffect = /* @__PURE__ */ __name((e) => {
      _effects.push(e);
      return e;
    }, "_addEffect");
    _addEffect(effect(() => {
      el.innerHTML = "";
      if (!state.open) {
        el.style.display = "none";
        return;
      }
      const triggerRect = trigger2.getBoundingClientRect();
      el.style.display = "inline-block";
      el.style.top = triggerRect.bottom + 4 + "px";
      if (state.placement === "bottom-left") {
        el.style.left = triggerRect.left + "px";
        el.style.right = "auto";
      } else {
        el.style.right = window.innerWidth - triggerRect.right + "px";
        el.style.left = "auto";
      }
      const menu = document.createElement("div");
      menu.style.cssText = [
        "min-width:var(--space-140)",
        "background:var(--color-bg-light)",
        "border-radius:var(--space-8)",
        "box-shadow:0 var(--space-4) var(--space-20) var(--color-alpha-shade-12)",
        "border:var(--space-1) solid var(--color-border)",
        "overflow:hidden",
        "animation:petui-dropdown-in 0.15s ease"
      ].join(";");
      state.items.forEach((item) => {
        const menuItem = document.createElement("div");
        menuItem.textContent = item.label;
        menuItem.style.cssText = [
          "padding:var(--space-8) var(--space-16)",
          "cursor:pointer",
          "font-size:0.9rem",
          "color:var(--color-text)",
          "transition:background 0.15s",
          "white-space:nowrap"
        ].join(";");
        menuItem.onmouseenter = () => {
          menuItem.style.background = "var(--color-primary-light)";
        };
        menuItem.onmouseleave = () => {
          menuItem.style.background = "transparent";
        };
        menuItem.addEventListener("click", () => {
          state.open = false;
          if (item.onClick) item.onClick();
        });
        menu.appendChild(menuItem);
      });
      if (state.items.length === 0) {
        const empty = document.createElement("div");
        empty.textContent = "\u65E0\u83DC\u5355\u9879";
        empty.style.cssText = "padding:var(--space-12) var(--space-16);color:var(--color-border);font-size:0.85rem;text-align:center";
        menu.appendChild(empty);
      }
      el.appendChild(menu);
    }));
    function triggerClick() {
      state.open = !state.open;
    }
    __name(triggerClick, "triggerClick");
    trigger2.addEventListener("click", triggerClick);
    document.addEventListener("click", globalClick);
    return Object.assign(el, {
      setProps(props) {
        const { trigger: newTrigger, ...rest } = props;
        if (newTrigger !== void 0 && newTrigger !== trigger2) {
          trigger2.removeEventListener("click", triggerClick);
          trigger2 = newTrigger;
          trigger2.addEventListener("click", triggerClick);
        }
        Object.assign(state, rest);
      },
      open() {
        state.open = true;
      },
      close() {
        state.open = false;
      },
      destroy() {
        _effects.forEach((e) => e.stop());
        _effects.length = 0;
        trigger2.removeEventListener("click", triggerClick);
        document.removeEventListener("click", globalClick);
        el.remove();
      }
    });
  }
  __name(createDropdown, "createDropdown");
  var popconfirmSeq = 0;
  function createPopconfirm(options) {
    let target = options.target;
    const { target: _target, ...restOptions } = options;
    const state = reactive({
      title: "\u786E\u8BA4\u6267\u884C\u6B64\u64CD\u4F5C\uFF1F",
      visible: false,
      onConfirm: null,
      onCancel: null,
      ...restOptions
    });
    const el = document.createElement("div");
    el.style.cssText = "position:fixed;display:inline-block;z-index:var(--z-popover)";
    document.body.appendChild(el);
    let previouslyFocused = null;
    let pendingFocusCapture = false;
    const titleId = "petui-popconfirm-title-" + ++popconfirmSeq;
    const _effects = [];
    const _addEffect = /* @__PURE__ */ __name((e) => {
      _effects.push(e);
      return e;
    }, "_addEffect");
    const cancel = /* @__PURE__ */ __name(() => {
      state.visible = false;
      if (state.onCancel) state.onCancel();
    }, "cancel");
    function onTrapKeydown(event) {
      if (!state.visible) return;
      if (event.key === "Escape") {
        if (typeof event.preventDefault === "function") event.preventDefault();
        cancel();
        return;
      }
      if (event.key !== "Tab") return;
      if (!el.contains(event.target)) return;
      const list = focusables2(el);
      if (list.length === 0) return;
      const first = list[0];
      const last = list[list.length - 1];
      const active = document.activeElement;
      if (event.shiftKey) {
        if (active === first) {
          if (typeof event.preventDefault === "function") event.preventDefault();
          last.focus();
        }
      } else if (active === last) {
        if (typeof event.preventDefault === "function") event.preventDefault();
        first.focus();
      }
    }
    __name(onTrapKeydown, "onTrapKeydown");
    _addEffect(effect(() => {
      el.innerHTML = "";
      if (!state.visible) {
        el.style.display = "none";
        if (isInDocument2(previouslyFocused)) previouslyFocused.focus();
        previouslyFocused = null;
        return;
      }
      if (pendingFocusCapture) {
        pendingFocusCapture = false;
        const active = document.activeElement;
        previouslyFocused = active && active !== document.body ? active : null;
      }
      const targetRect = target.getBoundingClientRect();
      el.style.display = "inline-block";
      el.style.top = targetRect.bottom + 8 + "px";
      el.style.left = targetRect.left + "px";
      const popover = document.createElement("div");
      popover.setAttribute("role", "dialog");
      popover.setAttribute("aria-modal", "true");
      popover.setAttribute("aria-labelledby", titleId);
      popover.setAttribute("tabindex", "-1");
      if (!state.title) popover.setAttribute("aria-label", "\u786E\u8BA4\u5BF9\u8BDD");
      popover.style.cssText = [
        "padding:var(--space-12) var(--space-16)",
        "background:var(--color-bg-light)",
        "border-radius:var(--space-8)",
        "box-shadow:0 var(--space-4) var(--space-20) var(--color-alpha-shade-15)",
        "border:var(--space-1) solid var(--color-border)",
        "font-size:0.9rem",
        "min-width:var(--space-180)"
      ].join(";");
      const titleEl = document.createElement("div");
      titleEl.id = titleId;
      titleEl.textContent = state.title;
      titleEl.style.cssText = "margin-bottom:var(--space-10);color:var(--color-text-secondary)";
      const btnRow = document.createElement("div");
      btnRow.style.cssText = "display:flex;gap:var(--space-8);justify-content:flex-end";
      const cancelBtn = document.createElement("button");
      cancelBtn.textContent = "\u53D6\u6D88";
      cancelBtn.style.cssText = "border:var(--space-1) solid var(--color-border);border-radius:var(--space-4);padding:var(--space-4) var(--space-12);background:var(--color-bg-light);color:var(--color-text-secondary);cursor:pointer;font-size:0.85rem";
      cancelBtn.addEventListener("click", () => {
        cancel();
      });
      const confirmBtn = document.createElement("button");
      confirmBtn.textContent = "\u786E\u8BA4";
      confirmBtn.style.cssText = "border:none;border-radius:var(--space-4);padding:var(--space-4) var(--space-12);background:var(--color-primary);color:var(--color-n-fff);cursor:pointer;font-size:0.85rem;font-weight:600";
      confirmBtn.addEventListener("click", () => {
        state.visible = false;
        if (state.onConfirm) state.onConfirm();
      });
      btnRow.appendChild(cancelBtn);
      btnRow.appendChild(confirmBtn);
      popover.appendChild(titleEl);
      popover.appendChild(btnRow);
      popover.addEventListener("keydown", onTrapKeydown, true);
      el.appendChild(popover);
      const list = focusables2(popover);
      const focusTarget = list.length > 0 ? list[0] : popover;
      if (focusTarget && typeof focusTarget.focus === "function") focusTarget.focus();
    }));
    function targetClick() {
      pendingFocusCapture = !state.visible;
      state.visible = !state.visible;
    }
    __name(targetClick, "targetClick");
    target.addEventListener("click", targetClick);
    target.addEventListener("keydown", onTrapKeydown, true);
    return Object.assign(el, {
      setProps(props) {
        const { target: newTarget, ...rest } = props;
        if (newTarget !== void 0 && newTarget !== target) {
          target.removeEventListener("click", targetClick);
          target.removeEventListener("keydown", onTrapKeydown, true);
          target = newTarget;
          target.addEventListener("click", targetClick);
          target.addEventListener("keydown", onTrapKeydown, true);
        }
        Object.assign(state, rest);
      },
      destroy() {
        _effects.forEach((e) => e.stop());
        _effects.length = 0;
        target.removeEventListener("click", targetClick);
        target.removeEventListener("keydown", onTrapKeydown, true);
        el.remove();
      }
    });
  }
  __name(createPopconfirm, "createPopconfirm");

  // src/components/rate.ts
  var DEFAULT_READONLY_REASON = "readonly";
  function describeDisabledState5(readonly2, reason) {
    if (!readonly2) return null;
    return { reason: reason || DEFAULT_READONLY_REASON, escapeAction: "set-readonly-false" };
  }
  __name(describeDisabledState5, "describeDisabledState");
  function createRate(options = {}) {
    const state = reactive({
      count: 5,
      value: 0,
      readonly: false,
      readonlyReason: "",
      onDisabled: null,
      onChange: null,
      ...options
    });
    const el = document.createElement("div");
    const _effects = [];
    const _addEffect = /* @__PURE__ */ __name((e) => {
      _effects.push(e);
      return e;
    }, "_addEffect");
    _addEffect(effect(() => {
      const blocked = describeDisabledState5(state.readonly === true, state.readonlyReason);
      if (blocked && state.onDisabled) state.onDisabled(blocked);
      el.innerHTML = "";
      el.style.cssText = "display:inline-flex;gap:var(--space-4);align-items:center";
      for (let i = 1; i <= state.count; i++) {
        const star = document.createElement("span");
        const filled = i <= state.value;
        star.textContent = filled ? "\u2605" : "\u2606";
        star.style.cssText = [
          "font-size:1.4rem",
          "color:" + (filled ? "var(--color-warning)" : "var(--color-border)"),
          "cursor:" + (state.readonly ? "default" : "pointer"),
          "transition:color 0.2s,transform 0.15s",
          "user-select:none"
        ].join(";");
        if (!state.readonly) {
          star.addEventListener("click", () => {
            state.value = i === state.value ? 0 : i;
            if (state.onChange) state.onChange(state.value);
          });
          star.onmouseenter = () => {
            if (!state.readonly) star.style.color = "var(--color-warning)";
            star.style.transform = "scale(1.15)";
          };
          star.onmouseleave = () => {
            const filledAfter = i <= state.value;
            star.style.color = filledAfter ? "var(--color-warning)" : "var(--color-border)";
            star.style.transform = "scale(1)";
          };
        }
        el.appendChild(star);
      }
    }));
    return Object.assign(el, {
      setProps(props) {
        Object.assign(state, props);
      },
      getValue() {
        return state.value;
      },
      setValue(v) {
        state.value = Math.min(state.count, Math.max(0, v));
      },
      destroy() {
        _effects.forEach((e) => e.stop());
        _effects.length = 0;
        el.remove();
      }
    });
  }
  __name(createRate, "createRate");

  // src/components/card.ts
  function createCard(options = {}) {
    const state = reactive({
      title: "",
      subtitle: "",
      content: "",
      image: "",
      footer: "",
      hoverable: false,
      onClick: null,
      ...options
    });
    const el = document.createElement("div");
    let currentClickHandler = null;
    const _effects = [];
    const _addEffect = /* @__PURE__ */ __name((e) => {
      _effects.push(e);
      return e;
    }, "_addEffect");
    _addEffect(effect(() => {
      if (currentClickHandler) {
        el.removeEventListener("click", currentClickHandler);
        currentClickHandler = null;
      }
      el.innerHTML = "";
      el.style.cssText = [
        "border-radius:var(--space-12)",
        "overflow:hidden",
        "background:var(--color-bg-light)",
        "box-shadow:0 var(--space-2) var(--font-size-12) var(--color-alpha-shade-08)",
        "border:var(--space-1) solid var(--color-border)",
        "transition:transform 0.2s,box-shadow 0.2s",
        "cursor:" + (state.onClick ? "pointer" : "default"),
        "max-width:var(--space-400)"
      ].join(";");
      if (state.hoverable) {
        el.onmouseenter = () => {
          el.style.transform = "translateY(calc(-1 * var(--space-2)))";
          el.style.boxShadow = "0 var(--space-4) var(--space-20) var(--color-alpha-shade-12)";
        };
        el.onmouseleave = () => {
          el.style.transform = "none";
          el.style.boxShadow = "0 var(--space-2) var(--font-size-12) var(--color-alpha-shade-08)";
        };
      }
      if (state.onClick) {
        currentClickHandler = state.onClick;
        el.addEventListener("click", state.onClick);
      }
      if (state.image) {
        const img = document.createElement("img");
        img.src = state.image;
        img.style.cssText = "width:100%;height:var(--space-200);object-fit:cover;display:block";
        el.appendChild(img);
      }
      if (state.title || state.subtitle) {
        const header = document.createElement("div");
        header.style.cssText = "padding:var(--space-16) var(--space-16) 0";
        if (state.title) {
          const titleEl = document.createElement("h3");
          titleEl.textContent = state.title;
          titleEl.style.cssText = "margin:0;font-size:1rem;color:var(--color-text);font-weight:600";
          header.appendChild(titleEl);
        }
        if (state.subtitle) {
          const subEl = document.createElement("div");
          subEl.textContent = state.subtitle;
          subEl.style.cssText = "margin-top:var(--space-4);font-size:0.8rem;color:var(--color-border)";
          header.appendChild(subEl);
        }
        el.appendChild(header);
      }
      if (state.content) {
        const body = document.createElement("div");
        setSafeHTML(body, state.content);
        body.style.cssText = "padding:var(--space-16);color:var(--color-text-secondary);font-size:0.9rem;line-height:1.6";
        el.appendChild(body);
      }
      if (state.footer) {
        const footerEl = document.createElement("div");
        setSafeHTML(footerEl, state.footer);
        footerEl.style.cssText = "padding:var(--space-12) var(--space-16);border-top:var(--space-1) solid var(--color-border);font-size:0.85rem;color:var(--color-text-secondary)";
        el.appendChild(footerEl);
      }
    }));
    return Object.assign(el, {
      setProps(props) {
        Object.assign(state, props);
      },
      setContent(v) {
        state.content = v;
      },
      destroy() {
        _effects.forEach((e) => e.stop());
        _effects.length = 0;
        el.remove();
      }
    });
  }
  __name(createCard, "createCard");

  // src/components/accordion.ts
  function createAccordion(options = {}) {
    const state = reactive({
      items: (options.items || []).map((item) => ({
        ...item,
        open: item.open || false
      }))
    });
    const el = document.createElement("div");
    const _effects = [];
    const _addEffect = /* @__PURE__ */ __name((e) => {
      _effects.push(e);
      return e;
    }, "_addEffect");
    _addEffect(effect(() => {
      el.innerHTML = "";
      el.style.cssText = "border:var(--space-1) solid var(--color-border);border-radius:var(--space-8);overflow:hidden";
      state.items.forEach((item, idx) => {
        const header = document.createElement("div");
        header.style.cssText = [
          "display:flex",
          "align-items:center",
          "justify-content:space-between",
          "padding:var(--space-12) var(--space-16)",
          "cursor:pointer",
          "background:" + (item.open ? "var(--color-primary-light)" : "var(--color-bg)"),
          "border-bottom:var(--space-1) solid var(--color-border)",
          "transition:background 0.2s",
          "user-select:none"
        ].join(";");
        header.onmouseenter = () => {
          header.style.background = "var(--color-primary-light)";
        };
        header.onmouseleave = () => {
          header.style.background = item.open ? "var(--color-primary-light)" : "var(--color-bg)";
        };
        const titleEl = document.createElement("span");
        titleEl.textContent = item.title;
        titleEl.style.cssText = "font-weight:500;font-size:0.95rem;color:var(--color-text)";
        header.appendChild(titleEl);
        const arrow = document.createElement("span");
        arrow.textContent = item.open ? "\u25B2" : "\u25BC";
        arrow.style.cssText = "font-size:0.7rem;color:var(--color-border);transition:transform 0.2s";
        header.appendChild(arrow);
        header.addEventListener("click", () => {
          item.open = !item.open;
          state.items = [...state.items];
        });
        el.appendChild(header);
        if (item.open) {
          const body = document.createElement("div");
          setSafeHTML(body, item.content);
          body.style.cssText = [
            "padding:var(--space-12) var(--space-16)",
            "color:var(--color-text-secondary)",
            "font-size:0.9rem",
            "line-height:1.6",
            "border-bottom:var(--space-1) solid var(--color-border)",
            "animation:petui-acc-in 0.2s ease"
          ].join(";");
          el.appendChild(body);
        }
      });
    }));
    const styleId = "petui-acc-styles";
    if (!document.getElementById(styleId)) {
      const style = document.createElement("style");
      style.id = styleId;
      style.textContent = "@keyframes petui-acc-in{from{opacity:0;max-height:0}to{opacity:1;max-height:var(--space-500)}}";
      document.head.appendChild(style);
    }
    return Object.assign(el, {
      setProps(props) {
        if (props.items) {
          state.items = props.items.map((item) => ({
            ...item,
            open: item.open || false
          }));
        }
      },
      toggle(index) {
        const item = state.items[index];
        if (item) {
          item.open = !item.open;
          state.items = [...state.items];
        }
      },
      open(index) {
        const item = state.items[index];
        if (item) {
          item.open = true;
          state.items = [...state.items];
        }
      },
      close(index) {
        const item = state.items[index];
        if (item) {
          item.open = false;
          state.items = [...state.items];
        }
      },
      destroy() {
        _effects.forEach((e) => e.stop());
        _effects.length = 0;
        el.remove();
      }
    });
  }
  __name(createAccordion, "createAccordion");

  // src/components/timeline.ts
  function createTimeline(options = {}) {
    const state = reactive({
      items: options.items || []
    });
    const el = document.createElement("div");
    const _effects = [];
    const _addEffect = /* @__PURE__ */ __name((e) => {
      _effects.push(e);
      return e;
    }, "_addEffect");
    _addEffect(effect(() => {
      el.innerHTML = "";
      el.style.cssText = "position:relative;padding-left:var(--space-24)";
      state.items.forEach((item, idx) => {
        const row = document.createElement("div");
        row.style.cssText = "position:relative;padding-bottom:" + (idx < state.items.length - 1 ? "var(--space-20)" : "0");
        const dot = document.createElement("div");
        const dotColor = item.color || "var(--color-primary)";
        Object.assign(dot.style, {
          position: "absolute",
          left: "calc(-1 * var(--space-20))",
          top: "var(--space-4)",
          width: "var(--font-size-12)",
          height: "var(--font-size-12)",
          borderRadius: "50%",
          background: dotColor,
          border: "var(--space-2) solid " + dotColor,
          boxSizing: "border-box",
          zIndex: "1"
        });
        row.appendChild(dot);
        if (idx < state.items.length - 1) {
          const line = document.createElement("div");
          Object.assign(line.style, {
            position: "absolute",
            left: "calc(-1 * var(--space-15))",
            top: "var(--space-18)",
            width: "var(--space-2)",
            height: "calc(100% - var(--space-4))",
            background: "var(--color-border)"
          });
          row.appendChild(line);
        }
        if (item.time) {
          const timeEl = document.createElement("div");
          timeEl.textContent = item.time;
          timeEl.style.cssText = "font-size:0.75rem;color:var(--color-border);margin-bottom:var(--space-4)";
          row.appendChild(timeEl);
        }
        const contentEl = document.createElement("div");
        setSafeHTML(contentEl, item.content);
        contentEl.style.cssText = "color:var(--color-text-secondary);font-size:0.9rem;line-height:1.5";
        row.appendChild(contentEl);
        el.appendChild(row);
      });
    }));
    return Object.assign(el, {
      setProps(props) {
        Object.assign(state, props);
      },
      destroy() {
        _effects.forEach((e) => e.stop());
        _effects.length = 0;
        el.remove();
      }
    });
  }
  __name(createTimeline, "createTimeline");

  // src/components/table.ts
  function createTable(options = {}) {
    const state = reactive({
      columns: [],
      data: [],
      ...options
    });
    const el = document.createElement("div");
    el.style.cssText = "overflow-x:auto";
    const _effects = [];
    const _addEffect = /* @__PURE__ */ __name((e) => {
      _effects.push(e);
      return e;
    }, "_addEffect");
    _addEffect(effect(() => {
      el.innerHTML = "";
      const table = document.createElement("table");
      table.style.cssText = [
        "width:100%",
        "border-collapse:collapse",
        "font-size:0.9rem",
        "color:var(--color-text)"
      ].join(";");
      const thead = document.createElement("thead");
      const headerRow = document.createElement("tr");
      state.columns.forEach((col) => {
        const th = document.createElement("th");
        th.textContent = col.title;
        th.style.cssText = [
          "padding:var(--space-10) var(--font-size-12)",
          "text-align:left",
          "background:var(--color-bg)",
          "border-bottom:var(--space-2) solid var(--color-primary-hover)",
          "font-weight:600",
          "color:var(--color-n-666)",
          "white-space:nowrap",
          col.width ? "width:" + col.width : ""
        ].join(";");
        headerRow.appendChild(th);
      });
      thead.appendChild(headerRow);
      table.appendChild(thead);
      const tbody = document.createElement("tbody");
      state.data.forEach((row, rowIdx) => {
        const tr = document.createElement("tr");
        tr.style.cssText = "transition:background 0.15s";
        tr.onmouseenter = () => {
          tr.style.background = "var(--color-primary-light)";
        };
        tr.onmouseleave = () => {
          tr.style.background = "transparent";
        };
        state.columns.forEach((col) => {
          const td = document.createElement("td");
          const val = row[col.key];
          td.textContent = val !== void 0 && val !== null ? String(val) : "";
          td.style.cssText = [
            "padding:var(--space-10) var(--font-size-12)",
            "border-bottom:var(--space-1) solid var(--color-n-f0f0f0)"
          ].join(";");
          tr.appendChild(td);
        });
        tbody.appendChild(tr);
      });
      table.appendChild(tbody);
      if (state.data.length === 0) {
        const tr = document.createElement("tr");
        const td = document.createElement("td");
        td.colSpan = state.columns.length || 1;
        td.textContent = "\u6682\u65E0\u6570\u636E";
        td.style.cssText = "padding:var(--space-24);text-align:center;color:var(--color-border);font-size:0.9rem";
        tr.appendChild(td);
        tbody.appendChild(tr);
      }
      el.appendChild(table);
    }));
    return Object.assign(el, {
      setProps(props) {
        Object.assign(state, props);
      },
      setData(data) {
        state.data = data;
      },
      getData() {
        return [...state.data];
      },
      destroy() {
        _effects.forEach((e) => e.stop());
        _effects.length = 0;
        el.remove();
      }
    });
  }
  __name(createTable, "createTable");

  // src/components/frosted-glass.ts
  function createFrostedGlass(options = {}) {
    const state = reactive({
      content: "",
      blur: 10,
      background: "var(--color-alpha-light-20)",
      ...options
    });
    const el = document.createElement("div");
    const _effects = [];
    const _addEffect = /* @__PURE__ */ __name((e) => {
      _effects.push(e);
      return e;
    }, "_addEffect");
    _addEffect(effect(() => {
      setSafeHTML(el, state.content);
      el.style.cssText = [
        "background:" + state.background,
        "backdrop-filter:blur(" + state.blur + "px)",
        "-webkit-backdrop-filter:blur(" + state.blur + "px)",
        "border-radius:var(--font-size-12)",
        "padding:var(--row-height-normal)",
        "border:var(--space-1) solid var(--color-alpha-light-30)",
        "color:var(--color-text)",
        "font-size:0.95rem",
        "line-height:1.6"
      ].join(";");
    }));
    return Object.assign(el, {
      setProps(props) {
        Object.assign(state, props);
      },
      setContent(v) {
        state.content = v;
      },
      destroy() {
        _effects.forEach((e) => e.stop());
        _effects.length = 0;
        el.remove();
      }
    });
  }
  __name(createFrostedGlass, "createFrostedGlass");

  // src/components/grid.ts
  var SVG_NS = "http://www.w3.org/2000/svg";
  var _gridIdCounter = 0;
  function resolveSize(v) {
    if (!v) return "";
    if (/^\s*-?\d+(\.\d+)?\s*$/.test(v)) {
      return v.trim() + "px";
    }
    return v;
  }
  __name(resolveSize, "resolveSize");
  function safeGridSize(v) {
    return typeof v === "number" && Number.isFinite(v) && v > 0 ? v : 20;
  }
  __name(safeGridSize, "safeGridSize");
  function createSvgElement(tag) {
    return document.createElementNS(SVG_NS, tag);
  }
  __name(createSvgElement, "createSvgElement");
  function buildGridSVG(gsRaw, gcRaw, patternId) {
    const gs = safeGridSize(gsRaw);
    const gc = isSafeColor(gcRaw) ? String(gcRaw).trim() : DEFAULT_GRID_COLOR;
    const size = String(gs);
    const svg = createSvgElement("svg");
    svg.setAttribute("width", "100%");
    svg.setAttribute("height", "100%");
    svg.setAttribute("style", "display:block");
    const defs = createSvgElement("defs");
    const pattern = createSvgElement("pattern");
    pattern.setAttribute("id", patternId);
    pattern.setAttribute("patternUnits", "userSpaceOnUse");
    pattern.setAttribute("width", size);
    pattern.setAttribute("height", size);
    const cell = createSvgElement("rect");
    cell.setAttribute("width", size);
    cell.setAttribute("height", size);
    cell.setAttribute("fill", "none");
    cell.setAttribute("stroke", gc);
    cell.setAttribute("stroke-width", "1");
    cell.setAttribute("x", "0");
    cell.setAttribute("y", "0");
    pattern.appendChild(cell);
    defs.appendChild(pattern);
    svg.appendChild(defs);
    const surface = createSvgElement("rect");
    surface.setAttribute("width", "100%");
    surface.setAttribute("height", "100%");
    surface.setAttribute("stroke", "var(--color-n-aaa)");
    surface.setAttribute("fill", "url(#" + patternId + ")");
    svg.appendChild(surface);
    return svg;
  }
  __name(buildGridSVG, "buildGridSVG");
  function createGrid(options = {}) {
    const state = reactive({
      backgroundColor: "var(--color-bg)",
      gridColor: "var(--color-n-e0d4d8)",
      gridSize: 20,
      width: "100%",
      height: "100%",
      ...options
    });
    const el = document.createElement("div");
    const patternId = "petui-grid-" + ++_gridIdCounter;
    const _effects = [];
    const _addEffect = /* @__PURE__ */ __name((e) => {
      _effects.push(e);
      return e;
    }, "_addEffect");
    _addEffect(effect(() => {
      const gs = state.gridSize;
      const gc = state.gridColor;
      const bg = state.backgroundColor;
      const w = resolveSize(state.width);
      const h2 = resolveSize(state.height);
      el.replaceChildren();
      const safeBg = isSafeColor(bg) ? String(bg).trim() : DEFAULT_GRID_BG;
      el.style.cssText = "width:" + w + ";height:" + h2 + ";background-color:" + safeBg + ";overflow:hidden";
      el.appendChild(buildGridSVG(gs, gc, patternId));
    }));
    return Object.assign(el, {
      setProps(props) {
        Object.assign(state, props);
      },
      destroy() {
        _effects.forEach((e) => e.stop());
        _effects.length = 0;
        el.remove();
      }
    });
  }
  __name(createGrid, "createGrid");

  // src/components/nine-grid-resizer.ts
  function createNineGridResizer(options) {
    let target = options.target;
    const { target: _target, ...restOptions } = options;
    const state = reactive({
      showHandles: true,
      resizable: true,
      movable: true,
      onResizeEnd: null,
      onMoveEnd: null,
      ...restOptions
    });
    const HANDLE_SIZE = 12;
    const positions = ["nw", "n", "ne", "e", "se", "s", "sw", "w", "center"];
    const handles = [];
    let activeDocMove = null;
    let activeDocUp = null;
    const el = document.createElement("div");
    function ensureParentPositioned() {
      const parent = target.parentNode;
      if (parent && parent !== document.body) {
        const cs = getComputedStyle(parent);
        if (cs.position === "static") {
          parent.style.position = "relative";
        }
      }
    }
    __name(ensureParentPositioned, "ensureParentPositioned");
    function createHandle(pos) {
      const h2 = document.createElement("div");
      h2.setAttribute("data-handle", pos);
      h2.style.cssText = [
        "position:absolute",
        "width:" + HANDLE_SIZE + "px",
        "height:" + HANDLE_SIZE + "px",
        "background:var(--color-primary)",
        "border:var(--space-2) solid var(--color-bg-light)",
        "border-radius:var(--space-3)",
        // K13 回流登记：本行的阴影色 rgba(0,0,0,0.3) 未收录进 LITERAL_TOKEN_MAP，
        // 但令牌源 ALPHA_TOKENS 里 --alpha-scrim 在六套主题下**同值**（= 该字面量），
        // 与 token-map.ts 头部「中性 alpha 复用 --alpha-*」的口径一致，故按同值令牌偿还（零视觉变更）。
        "box-shadow:0 0 0 var(--space-1) var(--color-alpha-primary-50), 0 var(--space-2) var(--space-6) var(--alpha-scrim)",
        "box-sizing:border-box",
        "z-index:999999",
        "pointer-events:auto"
      ].join(";");
      if (pos === "center") {
        h2.style.width = "var(--font-size-14)";
        h2.style.height = "var(--font-size-14)";
        h2.style.borderRadius = "50%";
        h2.style.cursor = "move";
      } else {
        h2.style.cursor = pos + "-resize";
      }
      return h2;
    }
    __name(createHandle, "createHandle");
    function updateHandlePositions() {
      if (!target.parentNode) return;
      const left = target.offsetLeft;
      const top = target.offsetTop;
      const w = target.offsetWidth;
      const h2 = target.offsetHeight;
      function setPos(handle, pos) {
        handle.style.display = state.showHandles && (pos === "center" ? state.movable : state.resizable) ? "block" : "none";
        switch (pos) {
          case "nw":
            handle.style.left = left - HANDLE_SIZE / 2 + "px";
            handle.style.top = top - HANDLE_SIZE / 2 + "px";
            break;
          case "n":
            handle.style.left = left + w / 2 - HANDLE_SIZE / 2 + "px";
            handle.style.top = top - HANDLE_SIZE / 2 + "px";
            break;
          case "ne":
            handle.style.left = left + w - HANDLE_SIZE / 2 + "px";
            handle.style.top = top - HANDLE_SIZE / 2 + "px";
            break;
          case "e":
            handle.style.left = left + w - HANDLE_SIZE / 2 + "px";
            handle.style.top = top + h2 / 2 - HANDLE_SIZE / 2 + "px";
            break;
          case "se":
            handle.style.left = left + w - HANDLE_SIZE / 2 + "px";
            handle.style.top = top + h2 - HANDLE_SIZE / 2 + "px";
            break;
          case "s":
            handle.style.left = left + w / 2 - HANDLE_SIZE / 2 + "px";
            handle.style.top = top + h2 - HANDLE_SIZE / 2 + "px";
            break;
          case "sw":
            handle.style.left = left - HANDLE_SIZE / 2 + "px";
            handle.style.top = top + h2 - HANDLE_SIZE / 2 + "px";
            break;
          case "w":
            handle.style.left = left - HANDLE_SIZE / 2 + "px";
            handle.style.top = top + h2 / 2 - HANDLE_SIZE / 2 + "px";
            break;
          case "center":
            handle.style.left = left + w / 2 - 7 + "px";
            handle.style.top = top + h2 / 2 - 7 + "px";
            break;
        }
      }
      __name(setPos, "setPos");
      handles.forEach((handle, i) => setPos(handle, positions[i]));
    }
    __name(updateHandlePositions, "updateHandlePositions");
    function bindDrag(handle, pos) {
      let dragging = false;
      let startX = 0, startY = 0;
      let startLeft = 0, startTop = 0, startWidth = 0, startHeight = 0;
      function onMouseDown(e) {
        if (pos !== "center" && !state.resizable || pos === "center" && !state.movable) return;
        e.preventDefault();
        e.stopPropagation();
        dragging = true;
        startX = e.clientX;
        startY = e.clientY;
        startLeft = target.offsetLeft;
        startTop = target.offsetTop;
        startWidth = target.offsetWidth;
        startHeight = target.offsetHeight;
        activeDocMove = onMouseMove;
        activeDocUp = onMouseUp;
        document.addEventListener("mousemove", onMouseMove);
        document.addEventListener("mouseup", onMouseUp);
      }
      __name(onMouseDown, "onMouseDown");
      function onMouseMove(e) {
        if (!dragging) return;
        const dx = e.clientX - startX;
        const dy = e.clientY - startY;
        const parent = target.offsetParent;
        const pw = parent ? parent.clientWidth : window.innerWidth;
        const ph = parent ? parent.clientHeight : window.innerHeight;
        if (pos === "center") {
          const nl = startLeft + dx;
          const nt = startTop + dy;
          target.style.left = Math.max(0, Math.min(nl, pw - startWidth)) + "px";
          target.style.top = Math.max(0, Math.min(nt, ph - startHeight)) + "px";
        } else {
          let nl = startLeft, nt = startTop, nw = startWidth, nh = startHeight;
          if (pos.includes("e")) nw = Math.max(20, startWidth + dx);
          if (pos.includes("w")) {
            nw = Math.max(20, startWidth - dx);
            nl = startLeft + dx;
          }
          if (pos.includes("s")) nh = Math.max(20, startHeight + dy);
          if (pos.includes("n")) {
            nh = Math.max(20, startHeight - dy);
            nt = startTop + dy;
          }
          if (pos === "n" || pos === "s") {
            nl = startLeft;
            nw = startWidth;
          }
          if (pos === "e" || pos === "w") {
            nt = startTop;
            nh = startHeight;
          }
          nl = Math.max(0, nl);
          nt = Math.max(0, nt);
          if (nl + nw > pw) {
            nw = Math.max(20, pw - nl);
            if (pos.includes("w")) nl = Math.max(0, pw - nw);
          }
          if (nt + nh > ph) {
            nh = Math.max(20, ph - nt);
            if (pos.includes("n")) nt = Math.max(0, ph - nh);
          }
          target.style.left = nl + "px";
          target.style.top = nt + "px";
          target.style.width = nw + "px";
          target.style.height = nh + "px";
        }
        updateHandlePositions();
      }
      __name(onMouseMove, "onMouseMove");
      function onMouseUp() {
        if (!dragging) return;
        dragging = false;
        activeDocMove = null;
        activeDocUp = null;
        document.removeEventListener("mousemove", onMouseMove);
        document.removeEventListener("mouseup", onMouseUp);
        if (pos === "center" && state.onMoveEnd) {
          state.onMoveEnd({ left: target.offsetLeft, top: target.offsetTop });
        } else if (pos !== "center" && state.onResizeEnd) {
          state.onResizeEnd({ width: target.offsetWidth, height: target.offsetHeight, left: target.offsetLeft, top: target.offsetTop });
        }
      }
      __name(onMouseUp, "onMouseUp");
      handle.addEventListener("mousedown", onMouseDown);
    }
    __name(bindDrag, "bindDrag");
    function rebuildHandles() {
      handles.forEach((h2) => {
        if (h2.parentNode) h2.parentNode.removeChild(h2);
      });
      handles.length = 0;
      if (activeDocMove) {
        document.removeEventListener("mousemove", activeDocMove);
        activeDocMove = null;
      }
      if (activeDocUp) {
        document.removeEventListener("mouseup", activeDocUp);
        activeDocUp = null;
      }
      ensureParentPositioned();
      const parent = target.parentNode;
      if (!parent) return;
      positions.forEach((pos) => {
        const h2 = createHandle(pos);
        bindDrag(h2, pos);
        handles.push(h2);
        parent.appendChild(h2);
      });
      updateHandlePositions();
    }
    __name(rebuildHandles, "rebuildHandles");
    rebuildHandles();
    const _effects = [];
    const _addEffect = /* @__PURE__ */ __name((e) => {
      _effects.push(e);
      return e;
    }, "_addEffect");
    _addEffect(effect(() => {
      updateHandlePositions();
    }));
    return Object.assign(el, {
      setProps(props) {
        const { target: newTarget, ...rest } = props;
        if (newTarget !== void 0 && newTarget !== target) {
          target = newTarget;
          rebuildHandles();
        }
        Object.assign(state, rest);
      },
      update() {
        updateHandlePositions();
      },
      showHandles() {
        updateHandlePositions();
      },
      destroy() {
        _effects.forEach((e) => e.stop());
        _effects.length = 0;
        if (activeDocMove) {
          document.removeEventListener("mousemove", activeDocMove);
          activeDocMove = null;
        }
        if (activeDocUp) {
          document.removeEventListener("mouseup", activeDocUp);
          activeDocUp = null;
        }
        handles.forEach((h2) => {
          if (h2.parentNode) h2.parentNode.removeChild(h2);
        });
        handles.length = 0;
        el.remove();
      }
    });
  }
  __name(createNineGridResizer, "createNineGridResizer");

  // src/components/split-pane.ts
  function createSplitPane(options) {
    const api = SplitPane(options);
    if (api === null) return null;
    const el = api.container;
    el.getSizes = api.getSizes;
    el.setPanelSize = api.setPanelSize;
    el.reset = api.reset;
    el.destroy = api.destroy;
    return el;
  }
  __name(createSplitPane, "createSplitPane");

  // src/lib/token-bridge.ts
  var _TokenNotRegisteredError = class _TokenNotRegisteredError extends Error {
    constructor(name, registered, suggestion) {
      const hint = suggestion === null ? "" : `\uFF1B\u6700\u8FD1\u90BB\u5DF2\u767B\u8BB0\u4EE4\u724C: ${suggestion}`;
      super(`[PetUI token-bridge] \u4EE4\u724C "${name}" \u672A\u5728\u4EE4\u724C\u6E90\uFF08src/themes/*.ts\uFF09\u767B\u8BB0${hint}`);
      this.name = "TokenNotRegisteredError";
      this.tokenName = name;
      this.registered = registered.slice();
      this.suggestion = suggestion;
    }
  };
  __name(_TokenNotRegisteredError, "TokenNotRegisteredError");
  var TokenNotRegisteredError = _TokenNotRegisteredError;
  var _TokenContextError = class _TokenContextError extends Error {
    constructor(reason) {
      const detail = reason === "no-document" ? "\u5F53\u524D\u73AF\u5883\u6CA1\u6709 document\uFF08SSR / node \u6D4B\u8BD5\u672A\u88C5 DOM stub\uFF09" : "\u4F20\u5165\u7684 el \u672A\u63D2\u5165\u6587\u6863\uFF0C\u4EE4\u724C\u7EE7\u627F\u94FE\u4E0D\u53EF\u7528";
      super(`[PetUI token-bridge] \u4EE4\u724C\u4E0A\u4E0B\u6587\u4E0D\u6EE1\u8DB3: ${detail}`);
      this.name = "TokenContextError";
      this.reason = reason;
      this.escapeAction = "insert-element-then-retry";
    }
  };
  __name(_TokenContextError, "TokenContextError");
  var TokenContextError = _TokenContextError;
  var _PrimitiveTokenBypassError = class _PrimitiveTokenBypassError extends Error {
    constructor(name) {
      super(
        `[PetUI token-bridge] \u7981\u6B62\u5143\u4EF6\u5C42\u76F4\u63A5\u6D88\u8D39\u539F\u59CB\u8272\u9636\u4EE4\u724C "${name}"\uFF08C3\uFF09\uFF1B\u8BF7\u5148\u7531 m01 \u5728\u4EE4\u724C\u6E90\u767B\u8BB0\u4E00\u4E2A\u8BED\u4E49\u4EE4\u724C\uFF08\`--color-*\` / \`--canvas-*\` / \u2026\uFF09\u518D\u8BFB\u53D6`
      );
      this.name = "PrimitiveTokenBypassError";
      this.tokenName = name;
      this.escapeAction = "request-semantic-token-from-m01";
    }
  };
  __name(_PrimitiveTokenBypassError, "PrimitiveTokenBypassError");
  var PrimitiveTokenBypassError = _PrimitiveTokenBypassError;
  var registry = /* @__PURE__ */ new Map();
  var themeVersion = 0;
  var cache = /* @__PURE__ */ new Map();
  var themeChangeListener = null;
  var themeSubscribers = [];
  var themesBridgeUnsub = null;
  function assertTokenNameShape(name) {
    if (typeof name !== "string" || !name.startsWith("--") || !/^--[a-z0-9-]+$/.test(name)) {
      throw new TokenNotRegisteredError(String(name), declaredTokens(), null);
    }
  }
  __name(assertTokenNameShape, "assertTokenNameShape");
  function hasToken(name) {
    if (registry.has(name)) return true;
    return themes.declaredTokenNames().indexOf(name) >= 0;
  }
  __name(hasToken, "hasToken");
  function isPrimitiveToken(name) {
    return name.startsWith("--primitive-");
  }
  __name(isPrimitiveToken, "isPrimitiveToken");
  function declaredTokens() {
    const merged = new Set(themes.declaredTokenNames());
    for (const name of registry.keys()) merged.add(name);
    return Object.freeze(Array.from(merged).sort());
  }
  __name(declaredTokens, "declaredTokens");
  function isRegistered(name) {
    return typeof name === "string" && hasToken(name);
  }
  __name(isRegistered, "isRegistered");
  function editDistance(a, b) {
    const m = a.length;
    const n = b.length;
    const prev = new Array(n + 1);
    const cur = new Array(n + 1);
    for (let j = 0; j <= n; j += 1) prev[j] = j;
    for (let i = 1; i <= m; i += 1) {
      cur[0] = i;
      for (let j = 1; j <= n; j += 1) {
        const cost = a.charCodeAt(i - 1) === b.charCodeAt(j - 1) ? 0 : 1;
        cur[j] = Math.min(prev[j] + 1, cur[j - 1] + 1, prev[j - 1] + cost);
      }
      for (let j = 0; j <= n; j += 1) prev[j] = cur[j];
    }
    return prev[n];
  }
  __name(editDistance, "editDistance");
  function nearestToken(name, candidates) {
    let best = null;
    let bestScore = Number.POSITIVE_INFINITY;
    for (const c of candidates) {
      const d = editDistance(name, c);
      if (d < bestScore) {
        bestScore = d;
        best = c;
      }
    }
    return best;
  }
  __name(nearestToken, "nearestToken");
  var KNOWN_FAMILIES = Object.freeze([
    "--primitive-",
    "--color-",
    "--canvas-",
    "--alpha-",
    "--font-size-",
    "--row-height-",
    "--z-",
    "--space-",
    "--border-radius-",
    "--shadow-",
    "--spacing-"
  ]);
  function assertRegistered(name) {
    assertTokenNameShape(name);
    if (!hasToken(name)) {
      const all = declaredTokens();
      throw new TokenNotRegisteredError(name, all, nearestToken(name, all));
    }
  }
  __name(assertRegistered, "assertRegistered");
  function hasDocument() {
    return typeof document !== "undefined" && document !== null;
  }
  __name(hasDocument, "hasDocument");
  function readInline(el, name) {
    const style = el.style;
    if (!style || typeof style.getPropertyValue !== "function") return null;
    const raw = style.getPropertyValue(name);
    if (typeof raw !== "string") return null;
    const trimmed = raw.trim();
    return trimmed.length === 0 ? null : trimmed;
  }
  __name(readInline, "readInline");
  function readComputed(el, name) {
    const gcs = globalThis.getComputedStyle;
    if (typeof gcs !== "function") return null;
    const computed5 = gcs(el);
    if (!computed5 || typeof computed5.getPropertyValue !== "function") return null;
    const raw = computed5.getPropertyValue(name);
    if (typeof raw !== "string") return null;
    const trimmed = raw.trim();
    return trimmed.length === 0 ? null : trimmed;
  }
  __name(readComputed, "readComputed");
  function onThemeChange(cb) {
    if (typeof cb !== "function") {
      throw new TypeError("[PetUI token-bridge] onThemeChange \u9700\u8981\u4E00\u4E2A\u51FD\u6570\u56DE\u8C03");
    }
    ensureThemesBridge();
    themeSubscribers.push(cb);
    let active = true;
    const unsubscribe = /* @__PURE__ */ __name(() => {
      if (!active) return;
      active = false;
      const idx = themeSubscribers.indexOf(cb);
      if (idx >= 0) themeSubscribers.splice(idx, 1);
    }, "unsubscribe");
    try {
      cb();
    } catch (err) {
      console.error("[PetUI token-bridge] onThemeChange \u56DE\u8C03\u629B\u9519\uFF08\u56DE\u653E\uFF09\uFF0C\u5DF2\u9694\u79BB\u8BE5\u8BA2\u9605\u8005:", err);
    }
    return unsubscribe;
  }
  __name(onThemeChange, "onThemeChange");
  function handleThemeChanged(topicName) {
    themeVersion += 1;
    cache.clear();
    if (themeChangeListener) {
      try {
        themeChangeListener();
      } catch (err) {
        console.error(`[PetUI token-bridge] setThemeChangeListener \u6CE8\u5165\u7684\u76D1\u542C\u629B\u9519\uFF08\u4E3B\u9898 ${topicName}\uFF09:`, err);
      }
    }
    for (const cb of themeSubscribers.slice()) {
      if (themeSubscribers.indexOf(cb) < 0) continue;
      try {
        cb();
      } catch (err) {
        console.error("[PetUI token-bridge] onThemeChange \u56DE\u8C03\u629B\u9519\uFF0C\u5DF2\u9694\u79BB\u8BE5\u8BA2\u9605\u8005:", err);
      }
    }
  }
  __name(handleThemeChanged, "handleThemeChanged");
  function ensureThemesBridge() {
    if (themesBridgeUnsub !== null) return;
    themesBridgeUnsub = themes.subscribe(() => {
      handleThemeChanged("themes.apply");
    });
  }
  __name(ensureThemesBridge, "ensureThemesBridge");
  var VAR_EXPRESSION_RE = /^var\(\s*(--[a-z0-9-]+)\s*(?:,([\s\S]*))?\)$/;
  function parseVarExpression(input) {
    const m = VAR_EXPRESSION_RE.exec(input);
    if (m !== null) {
      if (m[2] === void 0) return { kind: "parsed", name: m[1], fallback: null };
      const fallback = m[2].trim();
      return { kind: "parsed", name: m[1], fallback: fallback.length === 0 ? null : fallback };
    }
    return input.includes("var(") ? { kind: "malformed" } : { kind: "none" };
  }
  __name(parseVarExpression, "parseVarExpression");
  function readCustomProperty(node, name) {
    try {
      const computed5 = readComputed(node, name);
      if (computed5 !== null) return computed5;
    } catch (err) {
      console.warn(`[PetUI token-bridge] resolveTokenValue \u8BFB\u8BA1\u7B97\u6837\u5F0F\u5931\u8D25\uFF0C\u7EE7\u7EED\u515C\u5E95\uFF08${name}\uFF09:`, err);
    }
    return readInline(node, name);
  }
  __name(readCustomProperty, "readCustomProperty");
  function resolveTokenValue(el, value) {
    if (typeof value !== "string") {
      throw new TypeError("[PetUI token-bridge] resolveTokenValue \u7684 value \u5FC5\u987B\u662F\u5B57\u7B26\u4E32");
    }
    const trimmed = value.trim();
    const parsed = parseVarExpression(trimmed);
    if (parsed.kind === "none") {
      return Object.freeze({ value, resolvedFrom: "literal", wasVar: false, tokenName: null });
    }
    if (parsed.kind === "malformed") {
      throw new TypeError(
        `[PetUI token-bridge] resolveTokenValue \u6536\u5230\u975E\u6CD5 var() \u8868\u8FBE\u5F0F: ${JSON.stringify(value)}\uFF1B\u5408\u6CD5\u5F62\u5F0F\u4E3A var(<\u4EE4\u724C\u540D>) \u6216 var(<\u4EE4\u724C\u540D>, fallback)`
      );
    }
    const name = parsed.name;
    if (!hasToken(name)) {
      if (parsed.fallback !== null) {
        return Object.freeze({ value: parsed.fallback, resolvedFrom: "literal", wasVar: true, tokenName: name });
      }
      assertRegistered(name);
    }
    if (isPrimitiveToken(name)) throw new PrimitiveTokenBypassError(name);
    if (!hasDocument()) throw new TokenContextError("no-document");
    let root = null;
    try {
      root = document.documentElement ?? null;
    } catch (err) {
      console.warn("[PetUI token-bridge] resolveTokenValue \u8BFB\u53D6 documentElement \u5931\u8D25:", err);
      root = null;
    }
    const candidates = [];
    if (el !== null && el !== void 0) candidates.push({ node: el, from: "el" });
    if (root !== null && root !== el) candidates.push({ node: root, from: "root" });
    for (const c of candidates) {
      if (c.node === null) continue;
      const found = readCustomProperty(c.node, name);
      if (found !== null) {
        return Object.freeze({ value: found, resolvedFrom: c.from, wasVar: true, tokenName: name });
      }
    }
    throw new TokenContextError("detached-element");
  }
  __name(resolveTokenValue, "resolveTokenValue");

  // src/components/state-box.ts
  var KIND_TO_TONE = Object.freeze({
    empty: "neutral",
    loading: "info",
    error: "error",
    warn: "warning"
  });
  var BASE_TOKENS = Object.freeze({
    bg: "--color-bg-light",
    border: "--color-border",
    text: "--color-text-muted",
    height: "--space-10"
  });
  function toneTokensFor(tone) {
    const t = TONE_TOKENS[tone];
    if (!t) return BASE_TOKENS;
    return Object.freeze({ bg: t.bg, border: t.border, text: t.text, height: BASE_TOKENS.height });
  }
  __name(toneTokensFor, "toneTokensFor");
  var STATE_TOKENS = Object.freeze({
    empty: BASE_TOKENS,
    loading: BASE_TOKENS,
    error: toneTokensFor("error"),
    warn: toneTokensFor("warning")
  });
  var _warnedKinds = /* @__PURE__ */ new Set();
  var _LiteralColorError2 = class _LiteralColorError2 extends Error {
    constructor(slot, value) {
      super("[K11] kindToToneMap \u7684 " + slot + " \u5FC5\u987B\u662F\u4EE4\u724C\u540D\uFF08--...\uFF09\uFF0C\u6536\u5230\u7684\u662F\u5B57\u9762\u8272\u503C <" + value + ">");
      this.name = "LiteralColorError";
      this.slot = slot;
      this.value = value;
    }
  };
  __name(_LiteralColorError2, "LiteralColorError");
  var LiteralColorError2 = _LiteralColorError2;
  var _StateBoxDisposedError = class _StateBoxDisposedError extends Error {
    constructor() {
      super("[m07 state-box] \u5B9E\u4F8B\u5DF2 destroy()\uFF0C\u4E0D\u80FD\u518D\u8C03\u7528\u53D8\u66F4\u65B9\u6CD5");
      this.name = "StateBoxDisposedError";
    }
  };
  __name(_StateBoxDisposedError, "StateBoxDisposedError");
  var StateBoxDisposedError = _StateBoxDisposedError;
  var TOKEN_NAME_RE2 = /^--[a-z0-9-]+$/;
  var LI_HOSTS = /* @__PURE__ */ new Set(["UL", "OL", "TBODY"]);
  function tagForHost(host) {
    const name = host && host.tagName ? String(host.tagName).toUpperCase() : "";
    return LI_HOSTS.has(name) ? "li" : "div";
  }
  __name(tagForHost, "tagForHost");
  function tokVar2(name) {
    return "var(" + name + ")";
  }
  __name(tokVar2, "tokVar");
  function tokVar22(name, fallbackToken) {
    return "var(" + name + ", " + tokVar2(fallbackToken) + ")";
  }
  __name(tokVar22, "tokVar2");
  var _warnedTokens = /* @__PURE__ */ new Set();
  function normalizeTokenValue(name, fallbackToken, warn) {
    if (!TOKEN_NAME_RE2.test(name)) return name;
    if (isRegistered(name)) return tokVar2(name);
    if (!_warnedTokens.has(name)) {
      _warnedTokens.add(name);
      warn("[m07 state-box] \u4EE4\u724C <" + name + "> \u672A\u5728\u4EE4\u724C\u6E90\u767B\u8BB0\uFF0C\u5DF2\u56DE\u843D\u5230 <" + fallbackToken + ">\uFF08\u9700 K1 \u56DE\u6D41\uFF09");
    }
    return tokVar22(name, fallbackToken);
  }
  __name(normalizeTokenValue, "normalizeTokenValue");
  var STYLE_ID = "petui-state-box-styles";
  function injectStateBoxStyles(doc) {
    if (doc.getElementById(STYLE_ID)) return;
    const style = doc.createElement("style");
    style.id = STYLE_ID;
    style.textContent = [
      "@keyframes petui-state-box-sk { to { background-position: -200% 0; } }",
      "@media (prefers-reduced-motion: reduce) { .petui-state-box-skeleton { animation: none; } }"
    ].join("\n");
    const head = doc.head || doc.documentElement;
    if (head && typeof head.appendChild === "function") head.appendChild(style);
  }
  __name(injectStateBoxStyles, "injectStateBoxStyles");
  function createStateBox(options = {}) {
    const doc = options.host && options.host.ownerDocument ? options.host.ownerDocument : document;
    injectStateBoxStyles(doc);
    const disposer = createDisposer();
    const warn = options.onWarn || ((m) => console.warn(m));
    const el = doc.createElement(options.tag || tagForHost(options.host));
    const state = {
      kind: options.kind || "empty",
      text: options.text == null ? "" : String(options.text),
      next: options.next == null ? "" : String(options.next),
      spinner: options.spinner !== false,
      skeletonWidths: options.skeletonWidths && options.skeletonWidths.length > 0 ? options.skeletonWidths.slice() : ["60%", "35%"]
    };
    let disposed = false;
    function assertAlive() {
      if (disposed) throw new StateBoxDisposedError();
    }
    __name(assertAlive, "assertAlive");
    function assertToneMap(map) {
      if (!map) return;
      for (const key of Object.keys(map)) {
        const entry = map[key];
        if (!entry) continue;
        for (const slot of ["bg", "border", "text"]) {
          const v = entry[slot];
          if (v === void 0) continue;
          if (typeof v !== "string" || !TOKEN_NAME_RE2.test(v)) {
            throw new LiteralColorError2("kindToToneMap." + key + "." + slot, String(v));
          }
        }
      }
    }
    __name(assertToneMap, "assertToneMap");
    function tokensFor(kind) {
      const base = STATE_TOKENS[kind] || BASE_TOKENS;
      const override = options.kindToToneMap ? options.kindToToneMap[KIND_TO_TONE[kind]] : void 0;
      if (!override) return base;
      return Object.freeze({
        bg: override.bg || base.bg,
        border: override.border || base.border,
        text: override.text || base.text,
        height: base.height
      });
    }
    __name(tokensFor, "tokensFor");
    function buildTextNode() {
      const node = doc.createElement("span");
      node.className = "petui-state-box-text";
      node.textContent = state.text;
      return node;
    }
    __name(buildTextNode, "buildTextNode");
    function buildSkeleton() {
      const bars = [];
      for (const w of state.skeletonWidths) {
        const bar = doc.createElement("div");
        bar.className = "petui-state-box-skeleton";
        bar.style.cssText = [
          "height:" + tokVar2(BASE_TOKENS.height),
          "width:" + String(w),
          "border-radius:" + tokVar2("--border-radius-sm"),
          "background:linear-gradient(90deg, " + tokVar2("--color-border") + ", " + tokVar2("--color-bg-light") + ", " + tokVar2("--color-border") + ")",
          "background-size:200% 100%",
          "animation:petui-state-box-sk 1.2s linear infinite"
        ].join(";");
        bars.push(bar);
      }
      return bars;
    }
    __name(buildSkeleton, "buildSkeleton");
    function buildNextNode() {
      if (!state.next) return null;
      const hint = doc.createElement("span");
      hint.className = "petui-state-box-next";
      if (options.nextHtml) {
        try {
          setSafeHTML(hint, state.next);
        } catch (err) {
          console.error("[m07 state-box] next \u7247\u6BB5\u6D88\u6BD2\u5931\u8D25\uFF0C\u5DF2\u6574\u6BB5\u4E22\u5F03:", err);
          hint.textContent = "";
        }
      } else {
        hint.textContent = state.next;
      }
      return hint;
    }
    __name(buildNextNode, "buildNextNode");
    function render() {
      const tokens = tokensFor(state.kind);
      el.className = "petui-state-box petui-state-box-" + state.kind;
      el.style.cssText = [
        "display:flex",
        "flex-wrap:wrap",
        "align-items:center",
        "gap:" + tokVar2("--space-8"),
        "padding:" + tokVar2("--space-10") + " " + tokVar2("--space-12"),
        "border-radius:" + tokVar2("--border-radius-sm"),
        "border:" + tokVar2("--space-1") + " " + (state.kind === "empty" || state.kind === "loading" ? "dashed" : "solid") + " " + normalizeTokenValue(tokens.border, "--color-border", warn),
        "background:" + normalizeTokenValue(tokens.bg, "--color-bg-light", warn),
        "color:" + normalizeTokenValue(tokens.text, "--color-text-muted", warn),
        "font-size:" + tokVar2("--font-size-12")
      ].join(";");
      if (state.kind === "error" || state.kind === "warn") {
        el.setAttribute("role", "alert");
        el.setAttribute("aria-live", "assertive");
      } else {
        el.setAttribute("role", "status");
        el.setAttribute("aria-live", "polite");
      }
      el.setAttribute("data-state-kind", state.kind);
      while (el.childNodes.length > 0) el.removeChild(el.childNodes[0]);
      if (state.kind === "loading" && state.spinner) {
        const bars = buildSkeleton();
        for (const bar of bars) el.appendChild(bar);
      }
      const textNode = buildTextNode();
      const nextNode = buildNextNode();
      if (nextNode) {
        textNode.appendChild(doc.createTextNode(" "));
        textNode.appendChild(nextNode);
      }
      el.appendChild(textNode);
    }
    __name(render, "render");
    function apply() {
      assertAlive();
      assertToneMap(options.kindToToneMap);
      render();
    }
    __name(apply, "apply");
    if (!STATE_TOKENS[state.kind] || !KIND_TO_TONE[state.kind]) {
      const bad = String(state.kind);
      state.kind = "empty";
      if (!_warnedKinds.has(bad)) {
        _warnedKinds.add(bad);
        warn("[K11] \u672A\u77E5 kind <" + bad + ">\uFF0C\u843D\u5230\u515C\u5E95 kind <empty>");
      }
    }
    if (options.host && typeof options.host.appendChild === "function") {
      options.host.appendChild(el);
    }
    render();
    return Object.assign(el, {
      setState(next) {
        assertAlive();
        if (next.kind !== void 0) {
          const k = next.kind;
          if (!STATE_TOKENS[k] || !KIND_TO_TONE[k]) {
            const bad = String(k);
            if (!_warnedKinds.has(bad)) {
              _warnedKinds.add(bad);
              warn("[K11] \u672A\u77E5 kind <" + bad + ">\uFF0C\u843D\u5230\u515C\u5E95 kind <empty>");
            }
            state.kind = "empty";
          } else {
            state.kind = k;
          }
        }
        if (next.text !== void 0) state.text = next.text == null ? "" : String(next.text);
        if (next.next !== void 0) state.next = next.next == null ? "" : String(next.next);
        if (next.spinner !== void 0) state.spinner = next.spinner !== false;
        if (next.skeletonWidths !== void 0 && next.skeletonWidths.length > 0) {
          state.skeletonWidths = next.skeletonWidths.slice();
        }
        if (next.kindToToneMap !== void 0) {
          options.kindToToneMap = next.kindToToneMap;
        }
        if (next.nextHtml !== void 0) options.nextHtml = next.nextHtml;
        apply();
      },
      getState() {
        return Object.freeze({
          kind: state.kind,
          text: state.text,
          next: state.next,
          spinner: state.spinner,
          skeletonBars: state.kind === "loading" && state.spinner ? state.skeletonWidths.length : 0,
          disposed
        });
      },
      setText(text) {
        this.setState({ text });
      },
      setNext(next) {
        this.setState({ next });
      },
      destroy() {
        if (disposed) return;
        disposed = true;
        disposer.dispose();
        if (el.parentNode && typeof el.parentNode.removeChild === "function") {
          el.parentNode.removeChild(el);
        }
      }
    });
  }
  __name(createStateBox, "createStateBox");

  // src/components/status-pill.ts
  var KIND_TO_TONE2 = Object.freeze({
    ok: "success",
    warn: "warning",
    danger: "error",
    default: "neutral"
  });
  var FALLBACK_KEY = Object.keys(TONE_TOKENS)[0];
  var _warnedKinds2 = /* @__PURE__ */ new Set();
  function fallbackTokens() {
    const t = TONE_TOKENS[FALLBACK_KEY];
    if (t === void 0) {
      throw new Error("[K11] TONE_TOKENS \u65E0\u4EFB\u4F55\u767B\u8BB0\u9879\uFF0C\u72B6\u6001\u80F6\u56CA\u65E0\u515C\u5E95 tone \u53EF\u7528");
    }
    return t;
  }
  __name(fallbackTokens, "fallbackTokens");
  function resolveTone2(kind) {
    const raw = typeof kind === "string" ? kind.trim().toLowerCase() : "";
    const tone = raw === "" ? "info" : KIND_TO_TONE2[raw] ?? raw;
    const tokens = TONE_TOKENS[tone];
    if (tokens === void 0) {
      if (!_warnedKinds2.has(tone)) {
        _warnedKinds2.add(tone);
        console.warn("[K11] \u672A\u77E5 tone <" + tone + ">\uFF0C\u843D\u5230\u515C\u5E95 tone <" + FALLBACK_KEY + ">");
      }
      return { tone: FALLBACK_KEY, tokens: fallbackTokens() };
    }
    return { tone, tokens };
  }
  __name(resolveTone2, "resolveTone");
  function tokVar3(name) {
    return "var(" + name + ")";
  }
  __name(tokVar3, "tokVar");
  function createStatusPill(msg, kind = "info") {
    const disposer = createDisposer();
    const root = document.createElement("div");
    root.className = "petui-status-pill";
    root.setAttribute("role", "status");
    root.setAttribute("aria-live", "polite");
    const dot = document.createElement("span");
    dot.className = "petui-status-pill__dot";
    dot.setAttribute("aria-hidden", "true");
    const label = document.createElement("span");
    label.className = "petui-status-pill__label";
    root.appendChild(dot);
    root.appendChild(label);
    root.style.setProperty("display", "inline-flex");
    root.style.setProperty("align-items", "center");
    root.style.setProperty("gap", "var(--space-6)");
    root.style.setProperty("padding", "var(--space-2) var(--space-10)");
    root.style.setProperty("border-style", "solid");
    root.style.setProperty("border-width", "var(--space-1)");
    root.style.setProperty("border-radius", "var(--border-radius-lg)");
    root.style.setProperty("font-size", "var(--font-size-12)");
    dot.style.setProperty("width", "var(--space-8)");
    dot.style.setProperty("height", "var(--space-8)");
    dot.style.setProperty("border-radius", "50%");
    let destroyed = false;
    function render(nextMsg, nextKind) {
      if (destroyed) {
        console.warn("[PetUI status-pill] destroy() \u4E4B\u540E\u8C03\u7528 set()/clear()\uFF0C\u5DF2\u5FFD\u7565");
        return;
      }
      const { tone, tokens } = resolveTone2(nextKind);
      root.setAttribute("data-tone", tone);
      root.style.setProperty("background-color", tokVar3(tokens.bg));
      root.style.setProperty("color", tokVar3(tokens.text));
      root.style.setProperty("border-color", tokVar3(tokens.border));
      dot.style.setProperty("background-color", tokVar3(tokens.text));
      setSafeHTML(label, typeof nextMsg === "string" ? nextMsg : "");
    }
    __name(render, "render");
    root.set = /* @__PURE__ */ __name(function set(nextMsg, nextKind) {
      render(nextMsg, nextKind);
    }, "set");
    root.clear = /* @__PURE__ */ __name(function clear() {
      if (destroyed) {
        console.warn("[PetUI status-pill] destroy() \u4E4B\u540E\u8C03\u7528 clear()\uFF0C\u5DF2\u5FFD\u7565");
        return;
      }
      setSafeHTML(label, "");
    }, "clear");
    root.destroy = /* @__PURE__ */ __name(function destroy() {
      if (destroyed) return;
      destroyed = true;
      disposer.dispose();
    }, "destroy");
    disposer.add(/* @__PURE__ */ __name(function teardown() {
      if (root.parentNode !== null) root.parentNode.removeChild(root);
      setSafeHTML(label, "");
    }, "teardown"));
    render(msg, kind);
    return root;
  }
  __name(createStatusPill, "createStatusPill");

  // src/components/log-console.ts
  var _LogConsoleDisposedError = class _LogConsoleDisposedError extends Error {
    constructor() {
      super("[m09 log-console] \u5B9E\u4F8B\u5DF2 destroy()\uFF0C\u4E0D\u80FD\u518D\u8C03\u7528 log/detail/clear");
      this.name = "LogConsoleDisposedError";
    }
  };
  __name(_LogConsoleDisposedError, "LogConsoleDisposedError");
  var LogConsoleDisposedError = _LogConsoleDisposedError;
  var LEVEL_TOKENS = Object.freeze({
    debug: "--color-text-muted",
    info: "--color-text",
    success: "--color-success",
    warn: "--color-warning",
    error: "--color-danger"
  });
  var LEVELS = ["debug", "info", "success", "warn", "error"];
  var DEFAULT_MAX = 200;
  function tokVar4(name) {
    return "var(" + name + ")";
  }
  __name(tokVar4, "tokVar");
  var _warnedLevels = /* @__PURE__ */ new Set();
  function normalizeLevel(level, warn) {
    if (level === void 0 || level === null) return "info";
    const v = String(level);
    if (LEVELS.indexOf(v) >= 0) return v;
    if (!_warnedLevels.has(v)) {
      _warnedLevels.add(v);
      warn("[m09 log-console] \u672A\u77E5 level <" + v + ">\uFF0C\u843D\u5230\u515C\u5E95 level <info>");
    }
    return "info";
  }
  __name(normalizeLevel, "normalizeLevel");
  function toText(value) {
    if (typeof value === "string") return value;
    if (value instanceof Error) return value.name + ": " + value.message;
    if (value === void 0) return "undefined";
    if (value === null) return "null";
    try {
      const json = JSON.stringify(value);
      return json === void 0 ? String(value) : json;
    } catch {
      return String(value);
    }
  }
  __name(toText, "toText");
  function pad(n, width) {
    let s = String(n);
    while (s.length < width) s = "0" + s;
    return s;
  }
  __name(pad, "pad");
  function stamp(now) {
    return pad(now.getHours(), 2) + ":" + pad(now.getMinutes(), 2) + ":" + pad(now.getSeconds(), 2) + "." + pad(now.getMilliseconds(), 3);
  }
  __name(stamp, "stamp");
  function createLogConsole(options = {}) {
    const doc = document;
    const max = typeof options.max === "number" && isFinite(options.max) && options.max >= 0 ? Math.floor(options.max) : DEFAULT_MAX;
    const disposer = createDisposer();
    const warn = /* @__PURE__ */ __name((m) => console.warn(m), "warn");
    const el = doc.createElement("div");
    el.className = "petui-log-console";
    el.style.cssText = [
      "display:flex",
      "flex-direction:column",
      "background:" + tokVar4("--color-bg"),
      "color:" + tokVar4("--color-text"),
      "border:" + tokVar4("--space-1") + " solid " + tokVar4("--color-border"),
      "border-radius:" + tokVar4("--border-radius-sm"),
      "font-size:" + tokVar4("--font-size-12"),
      "overflow:hidden"
    ].join(";");
    if (options.label) el.setAttribute("aria-label", options.label);
    el.setAttribute("role", "log");
    el.setAttribute("aria-live", "polite");
    const bar = doc.createElement("div");
    bar.style.cssText = [
      "display:flex",
      "align-items:center",
      "justify-content:space-between",
      "gap:" + tokVar4("--space-8"),
      "padding:" + tokVar4("--space-4") + " " + tokVar4("--space-8"),
      "background:" + tokVar4("--color-bg-light"),
      "border-bottom:" + tokVar4("--space-1") + " solid " + tokVar4("--color-border")
    ].join(";");
    const meta = doc.createElement("span");
    meta.className = "petui-log-console-meta";
    meta.style.color = tokVar4("--color-text-muted");
    const clearBtn = doc.createElement("button");
    clearBtn.type = "button";
    clearBtn.className = "petui-log-console-clear";
    clearBtn.textContent = "clear";
    clearBtn.style.cssText = [
      "background:" + tokVar4("--color-bg-light"),
      "color:" + tokVar4("--color-text"),
      "border:" + tokVar4("--space-1") + " solid " + tokVar4("--color-border"),
      "border-radius:" + tokVar4("--border-radius-sm"),
      "padding:" + tokVar4("--space-2") + " " + tokVar4("--space-8"),
      "font-size:" + tokVar4("--font-size-12"),
      "cursor:pointer"
    ].join(";");
    bar.appendChild(meta);
    bar.appendChild(clearBtn);
    const list = doc.createElement("ol");
    list.className = "petui-log-console-list";
    list.style.cssText = [
      "display:flex",
      "flex-direction:column",
      "margin:0",
      "padding:" + tokVar4("--space-4") + " " + tokVar4("--space-8"),
      "list-style:none",
      "max-height:" + tokVar4("--space-300"),
      "overflow:auto"
    ].join(";");
    el.appendChild(bar);
    el.appendChild(list);
    let disposed = false;
    function assertAlive() {
      if (disposed) throw new LogConsoleDisposedError();
    }
    __name(assertAlive, "assertAlive");
    function entries() {
      return list.childNodes.length;
    }
    __name(entries, "entries");
    function syncMeta() {
      meta.textContent = "log " + String(entries()) + "/" + String(max);
    }
    __name(syncMeta, "syncMeta");
    function trim() {
      while (list.childNodes.length > max && list.firstChild) {
        list.removeChild(list.firstChild);
      }
    }
    __name(trim, "trim");
    function push(row) {
      list.appendChild(row);
      trim();
      syncMeta();
      list.scrollTop = list.scrollHeight;
    }
    __name(push, "push");
    syncMeta();
    disposer.addEventListener(clearBtn, "click", () => {
      if (!disposed) api.clear();
    });
    const api = {
      log(msg, level) {
        assertAlive();
        const lv = normalizeLevel(level, warn);
        const row = doc.createElement("li");
        row.className = "petui-log-console-row";
        row.setAttribute("data-level", lv);
        row.style.cssText = [
          "color:" + tokVar4(LEVEL_TOKENS[lv]),
          "white-space:pre-wrap",
          "word-break:break-word",
          "padding:" + tokVar4("--space-1") + " 0"
        ].join(";");
        const time = doc.createElement("span");
        time.className = "petui-log-console-time";
        time.style.marginRight = tokVar4("--space-8");
        time.textContent = stamp(/* @__PURE__ */ new Date());
        const text = doc.createElement("span");
        text.textContent = toText(msg);
        row.appendChild(time);
        row.appendChild(text);
        push(row);
      },
      detail(title, lines) {
        assertAlive();
        const item = doc.createElement("li");
        const box = doc.createElement("details");
        box.className = "petui-log-console-detail";
        box.open = true;
        box.style.cssText = [
          "border-left:" + tokVar4("--space-2") + " solid " + tokVar4("--color-border"),
          "padding-left:" + tokVar4("--space-8"),
          "margin:" + tokVar4("--space-4") + " 0"
        ].join(";");
        const summary = doc.createElement("summary");
        summary.style.color = tokVar4("--color-primary");
        summary.textContent = toText(title);
        box.appendChild(summary);
        const body = doc.createElement("div");
        const items = Array.isArray(lines) ? lines : [];
        for (let i = 0; i < items.length; i += 1) {
          const line = doc.createElement("div");
          line.className = "petui-log-console-line";
          line.style.cssText = "white-space:pre-wrap;word-break:break-word";
          line.textContent = toText(items[i]);
          body.appendChild(line);
        }
        box.appendChild(body);
        item.appendChild(box);
        push(item);
      },
      clear() {
        assertAlive();
        while (list.firstChild) list.removeChild(list.firstChild);
        syncMeta();
      },
      destroy() {
        if (disposed) return;
        disposed = true;
        disposer.dispose();
        while (list.firstChild) list.removeChild(list.firstChild);
        if (el.parentNode && typeof el.parentNode.removeChild === "function") {
          el.parentNode.removeChild(el);
        }
      },
      get count() {
        return entries();
      }
    };
    Object.assign(el, api);
    Object.defineProperty(el, "count", {
      configurable: true,
      enumerable: false,
      get: /* @__PURE__ */ __name(() => entries(), "get")
    });
    return el;
  }
  __name(createLogConsole, "createLogConsole");

  // src/components/result-box.ts
  var RESULT_FALLBACK_TEXT = Object.freeze({
    ok: "\u64CD\u4F5C\u6210\u529F",
    err: "\u64CD\u4F5C\u5931\u8D25\uFF08\u65E0\u8BE6\u60C5\uFF09"
  });
  var RESULT_BOX_CLASS = "petui-result-box";
  var RESULT_BLOCK_CLASS = "petui-result-block";
  var _ResultBoxDisposedError = class _ResultBoxDisposedError extends Error {
    constructor() {
      super("[m10 result-box] \u5B9E\u4F8B\u5DF2 destroy()\uFF0C\u4E0D\u80FD\u518D\u8C03\u7528 show/append/replace/clear");
      this.name = "ResultBoxDisposedError";
    }
  };
  __name(_ResultBoxDisposedError, "ResultBoxDisposedError");
  var ResultBoxDisposedError = _ResultBoxDisposedError;
  var NEUTRAL_TOKENS = Object.freeze({
    bg: "--color-bg-light",
    border: "--color-border",
    text: "--color-text-muted"
  });
  function resultToneTokens(ok) {
    const t = ok ? TONE_TOKENS.success : TONE_TOKENS.error;
    return t || NEUTRAL_TOKENS;
  }
  __name(resultToneTokens, "resultToneTokens");
  var FINDING_TONE = Object.freeze({
    info: "info",
    warn: "warning",
    error: "error"
  });
  var FINDING_MARK = Object.freeze({
    info: "i",
    warn: "!",
    error: "\xD7"
  });
  var STATUS_MARK = Object.freeze({ ok: "\u2713", err: "\u2717" });
  function tv(name) {
    return "var(" + name + ")";
  }
  __name(tv, "tv");
  var _warned = /* @__PURE__ */ new Set();
  function asText(v) {
    return v == null ? "" : String(v);
  }
  __name(asText, "asText");
  function nonEmptyText(v) {
    return asText(v).length > 0;
  }
  __name(nonEmptyText, "nonEmptyText");
  function nonEmptyArray(v) {
    return Array.isArray(v) && v.length > 0;
  }
  __name(nonEmptyArray, "nonEmptyArray");
  function createResultBox(options = {}) {
    const doc = options.host && options.host.ownerDocument ? options.host.ownerDocument : document;
    const disposer = createDisposer();
    const warn = options.onWarn || ((m) => console.warn(m));
    const el = doc.createElement("div");
    el.className = RESULT_BOX_CLASS;
    el.style.cssText = [
      "display:none",
      // K10 不变量①：默认隐藏（由 syncVisibility 接管）
      "box-sizing:border-box",
      "min-width:0",
      "max-width:100%",
      "font-size:" + tv("--font-size-12")
    ].join(";");
    const body = doc.createElement("div");
    body.className = RESULT_BOX_CLASS + "-body";
    body.style.cssText = [
      "display:flex",
      "flex-direction:column",
      "gap:" + tv("--space-8"),
      "min-width:0",
      "max-width:100%"
    ].join(";");
    el.appendChild(body);
    const blocks = [];
    const actions = /* @__PURE__ */ new WeakMap();
    let disposed = false;
    function assertAlive() {
      if (disposed) throw new ResultBoxDisposedError();
    }
    __name(assertAlive, "assertAlive");
    function warnOnce(key, message) {
      if (_warned.has(key)) return;
      _warned.add(key);
      warn(message);
    }
    __name(warnOnce, "warnOnce");
    function syncVisibility() {
      el.style.display = blocks.length > 0 ? "block" : "none";
    }
    __name(syncVisibility, "syncVisibility");
    function isDisplayable(env) {
      return typeof env.ok === "boolean" || nonEmptyText(env.title) || nonEmptyText(env.out) || nonEmptyArray(env.log) || nonEmptyArray(env.findings) || nonEmptyArray(env.actions);
    }
    __name(isDisplayable, "isDisplayable");
    function actionFor(target) {
      let node = target;
      let guard = 0;
      while (node && node !== el && guard < 64) {
        const found = actions.get(node);
        if (found) return found;
        node = node.parentNode || null;
        guard += 1;
      }
      return void 0;
    }
    __name(actionFor, "actionFor");
    disposer.addEventListener(el, "click", (event) => {
      const fn = actionFor(event.target);
      if (!fn) return;
      try {
        fn();
      } catch (err) {
        console.error("[m10 result-box] actions[i].onActivate \u629B\u9519\uFF08\u5DF2\u9694\u79BB\u8BE5\u6309\u94AE\uFF09:", err);
      }
    });
    function buildTitle(env, ok) {
      const node = doc.createElement("strong");
      node.className = RESULT_BOX_CLASS + "-title";
      node.style.cssText = ["min-width:0", "overflow-wrap:anywhere", "word-break:break-word"].join(";");
      const raw = nonEmptyText(env.title) ? asText(env.title) : ok ? RESULT_FALLBACK_TEXT.ok : RESULT_FALLBACK_TEXT.err;
      if (options.titleHtml) {
        try {
          setSafeHTML(node, raw);
        } catch (err) {
          console.error("[m10 result-box] title \u5BCC\u6587\u672C\u6D88\u6BD2\u5931\u8D25\uFF0C\u5DF2\u6574\u6BB5\u4E22\u5F03:", err);
          node.textContent = "";
        }
      } else {
        node.textContent = raw;
      }
      return node;
    }
    __name(buildTitle, "buildTitle");
    function buildMultiline(className, text, extraCss) {
      const node = doc.createElement("div");
      node.className = className;
      node.style.cssText = [
        "white-space:pre-wrap",
        "overflow-wrap:anywhere",
        "word-break:break-word",
        "min-width:0",
        "max-width:100%"
      ].concat(extraCss).join(";");
      node.textContent = text;
      return node;
    }
    __name(buildMultiline, "buildMultiline");
    function buildLog(env, tokens) {
      const lines = Array.isArray(env.log) ? env.log : [];
      if (lines.length === 0) return null;
      const box = buildMultiline(RESULT_BOX_CLASS + "-log", "", [
        "margin-top:" + tv("--space-6"),
        "padding-top:" + tv("--space-6"),
        "border-top:" + tv("--space-1") + " solid " + tv(tokens.border),
        "overflow-x:auto",
        "color:" + tv("--color-text")
      ]);
      for (const line of lines) {
        const row = buildMultiline(RESULT_BOX_CLASS + "-log-line", asText(line), []);
        box.appendChild(row);
      }
      return box;
    }
    __name(buildLog, "buildLog");
    function buildFindings(env) {
      const items = Array.isArray(env.findings) ? env.findings : [];
      if (items.length === 0) return null;
      const box = doc.createElement("div");
      box.className = RESULT_BOX_CLASS + "-findings";
      box.style.cssText = [
        "margin-top:" + tv("--space-6"),
        "display:flex",
        "flex-direction:column",
        "gap:" + tv("--space-4"),
        "min-width:0"
      ].join(";");
      for (const item of items) {
        const level = item && item.level;
        const tone = level ? FINDING_TONE[level] : void 0;
        const slot = tone ? TONE_TOKENS[tone] : void 0;
        if (!tone || !slot) {
          warnOnce(
            "finding-level:" + asText(level),
            "[m10 result-box] \u672A\u77E5 findings.level <" + asText(level) + ">\uFF0C\u8BE5\u884C\u843D\u5230\u4E2D\u6027\u6837\u5F0F"
          );
        }
        const row = doc.createElement("div");
        row.className = RESULT_BOX_CLASS + "-finding";
        row.setAttribute("data-level", asText(level));
        row.style.cssText = [
          "display:flex",
          "align-items:flex-start",
          "gap:" + tv("--space-6"),
          "min-width:0",
          "max-width:100%",
          "overflow-wrap:anywhere",
          "word-break:break-word",
          "color:" + tv((slot || NEUTRAL_TOKENS).text)
        ].join(";");
        const mark = doc.createElement("span");
        mark.className = RESULT_BOX_CLASS + "-finding-mark";
        mark.setAttribute("aria-hidden", "true");
        mark.style.cssText = ["flex:none"].join(";");
        mark.textContent = FINDING_MARK[level] || FINDING_MARK.info;
        const text = doc.createElement("span");
        text.className = RESULT_BOX_CLASS + "-finding-text";
        text.style.cssText = [
          "white-space:pre-wrap",
          "min-width:0",
          "overflow-wrap:anywhere",
          "word-break:break-word"
        ].join(";");
        text.textContent = asText(item && item.text);
        row.appendChild(mark);
        row.appendChild(text);
        box.appendChild(row);
      }
      return box;
    }
    __name(buildFindings, "buildFindings");
    function buildActions2(env, tokens) {
      const items = Array.isArray(env.actions) ? env.actions : [];
      if (items.length === 0) return null;
      const bar = doc.createElement("div");
      bar.className = RESULT_BOX_CLASS + "-actions";
      bar.style.cssText = [
        "margin-top:" + tv("--space-8"),
        "display:flex",
        "flex-wrap:wrap",
        "gap:" + tv("--space-6"),
        "min-width:0"
      ].join(";");
      for (const item of items) {
        const label = asText(item && item.label);
        const fn = item && item.onActivate;
        if (!nonEmptyText(label) || typeof fn !== "function") {
          warnOnce(
            "action-item:" + label + ":" + typeof fn,
            "[m10 result-box] \u8DF3\u8FC7\u975E\u6CD5 actions \u9879\uFF08label=" + label + ", onActivate=" + typeof fn + "\uFF09"
          );
          continue;
        }
        const btn = doc.createElement("button");
        btn.type = "button";
        btn.className = RESULT_BOX_CLASS + "-action";
        btn.textContent = label;
        btn.style.cssText = [
          "font:inherit",
          "font-size:" + tv("--font-size-12"),
          "cursor:pointer",
          "border:" + tv("--space-1") + " solid " + tv(tokens.border),
          "border-radius:" + tv("--border-radius-sm"),
          "background:" + tv("--color-bg"),
          "color:" + tv(tokens.text),
          "padding:" + tv("--space-4") + " " + tv("--space-10"),
          "max-width:100%",
          "overflow-wrap:anywhere"
        ].join(";");
        actions.set(btn, fn);
        bar.appendChild(btn);
      }
      return bar.childNodes.length > 0 ? bar : null;
    }
    __name(buildActions2, "buildActions");
    function renderEnvelope(env) {
      if (!isDisplayable(env)) return null;
      const ok = env.ok === true;
      const tokens = resultToneTokens(ok);
      const block = doc.createElement("div");
      block.className = RESULT_BLOCK_CLASS;
      block.setAttribute("data-ok", ok ? "true" : "false");
      block.style.cssText = [
        "display:block",
        "box-sizing:border-box",
        "min-width:0",
        "max-width:100%",
        "overflow-wrap:anywhere",
        "word-break:break-word",
        "border:" + tv("--space-1") + " solid " + tv(tokens.border),
        "border-left:" + tv("--space-4") + " solid " + tv(tokens.border),
        "border-radius:" + tv("--border-radius-sm"),
        "background:" + tv(tokens.bg),
        "color:" + tv(tokens.text),
        "padding:" + tv("--space-8") + " " + tv("--space-12")
      ].join(";");
      const head = doc.createElement("div");
      head.className = RESULT_BOX_CLASS + "-head";
      head.style.cssText = [
        "display:flex",
        "align-items:center",
        "gap:" + tv("--space-6"),
        "min-width:0"
      ].join(";");
      const mark = doc.createElement("span");
      mark.className = RESULT_BOX_CLASS + "-mark";
      mark.setAttribute("aria-hidden", "true");
      mark.style.cssText = ["flex:none"].join(";");
      mark.textContent = ok ? STATUS_MARK.ok : STATUS_MARK.err;
      head.appendChild(mark);
      head.appendChild(buildTitle(env, ok));
      block.appendChild(head);
      if (nonEmptyText(env.out)) {
        block.appendChild(buildMultiline(RESULT_BOX_CLASS + "-out", asText(env.out), [
          "margin-top:" + tv("--space-6"),
          "color:" + tv("--color-text")
        ]));
      }
      const log = buildLog(env, tokens);
      if (log) block.appendChild(log);
      const findings = buildFindings(env);
      if (findings) block.appendChild(findings);
      const bar = buildActions2(env, tokens);
      if (bar) block.appendChild(bar);
      block.setAttribute("role", ok ? "status" : "alert");
      block.setAttribute("aria-live", ok ? "polite" : "assertive");
      return block;
    }
    __name(renderEnvelope, "renderEnvelope");
    function mount2(env) {
      const block = renderEnvelope(env);
      if (!block) return;
      body.appendChild(block);
      blocks.push(block);
      syncVisibility();
    }
    __name(mount2, "mount");
    if (options.host && typeof options.host.appendChild === "function") {
      options.host.appendChild(el);
    }
    syncVisibility();
    return Object.assign(el, {
      show(env) {
        assertAlive();
        if (!env || typeof env !== "object") {
          throw new TypeError("[m10 result-box] show/append \u9700\u8981\u4E00\u4E2A ResultEnvelope \u5BF9\u8C61");
        }
        body.replaceChildren();
        blocks.length = 0;
        mount2(env);
        syncVisibility();
      },
      append(env) {
        assertAlive();
        if (!env || typeof env !== "object") {
          throw new TypeError("[m10 result-box] show/append \u9700\u8981\u4E00\u4E2A ResultEnvelope \u5BF9\u8C61");
        }
        mount2(env);
      },
      replace(env) {
        this.show(env);
      },
      clear() {
        assertAlive();
        body.replaceChildren();
        blocks.length = 0;
        syncVisibility();
      },
      destroy() {
        if (disposed) return;
        disposed = true;
        disposer.dispose();
        body.replaceChildren();
        blocks.length = 0;
        if (el.parentNode && typeof el.parentNode.removeChild === "function") {
          el.parentNode.removeChild(el);
        }
      }
    });
  }
  __name(createResultBox, "createResultBox");

  // src/lib/clipboard.ts
  var FAILURE_LOG_LIMIT = 50;
  var FAILURE_LOG = [];
  var DEFAULT_FAILURE_HANDLER = /* @__PURE__ */ __name((reason, error) => {
    console.error(`[petui:clipboard] ${reason}`, error);
  }, "DEFAULT_FAILURE_HANDLER");
  function removeQuietly(node, label) {
    if (!node || typeof node.remove !== "function") return;
    try {
      node.remove();
    } catch (err) {
      console.error(`[petui:clipboard] \u4E34\u65F6\u8282\u70B9\u79FB\u9664\u5931\u8D25\uFF08${label}\uFF09`, err);
    }
  }
  __name(removeQuietly, "removeQuietly");
  function recordFailure(op, reason, error) {
    FAILURE_LOG.push(Object.freeze({ op, reason, errorText: errorText(error) }));
    if (FAILURE_LOG.length > FAILURE_LOG_LIMIT) FAILURE_LOG.shift();
  }
  __name(recordFailure, "recordFailure");
  var failureHandler = DEFAULT_FAILURE_HANDLER;
  var reporter = {
    failure(op, reason, error) {
      recordFailure(op, reason, error);
      failureHandler(reason, error);
    }
  };
  function errorText(err) {
    if (err instanceof Error) return err.message;
    if (err === void 0) return "undefined";
    return String(err);
  }
  __name(errorText, "errorText");
  function hostNavigator() {
    return typeof navigator === "undefined" ? void 0 : navigator;
  }
  __name(hostNavigator, "hostNavigator");
  function hostDocument() {
    return typeof document === "undefined" ? void 0 : document;
  }
  __name(hostDocument, "hostDocument");
  function resolveClipboard(injected) {
    if (injected !== void 0) return injected;
    const nav = hostNavigator();
    const api = nav && nav.clipboard ? nav.clipboard : null;
    if (api && typeof api.writeText === "function") return api;
    return null;
  }
  __name(resolveClipboard, "resolveClipboard");
  function resolveDocument(injected) {
    if (injected !== void 0) return injected;
    const doc = hostDocument();
    return doc && typeof doc.createElement === "function" ? doc : null;
  }
  __name(resolveDocument, "resolveDocument");
  async function copyText(text, opts = {}) {
    if (typeof text !== "string") {
      throw new TypeError(`[clipboard] copyText \u7684 text \u5FC5\u987B\u662F\u5B57\u7B26\u4E32\uFF0C\u6536\u5230 ${typeof text}`);
    }
    const fail = /* @__PURE__ */ __name((reason, error) => {
      if (typeof opts.onError === "function") opts.onError(reason, error);
      reporter.failure("copyText", reason, error);
      return false;
    }, "fail");
    const clipboard = resolveClipboard(opts.clipboard);
    if (clipboard) {
      try {
        await clipboard.writeText(text);
        return true;
      } catch (err) {
        reporter.failure("copyText", "clipboard-rejected", err);
      }
    } else {
      reporter.failure("copyText", "clipboard-unavailable", new Error("\u5BBF\u4E3B\u672A\u63D0\u4F9B navigator.clipboard"));
    }
    const doc = resolveDocument(opts.doc);
    const host = doc ? doc.body || doc.documentElement || null : null;
    if (!doc || !host || typeof doc.createElement !== "function") {
      return fail("document-unavailable", new Error("\u6CA1\u6709\u53EF\u7528\u7684 document/body\uFF0C\u65E0\u6CD5\u8D70 textarea + execCommand \u515C\u5E95"));
    }
    let textarea;
    try {
      textarea = doc.createElement("textarea");
      textarea.value = text;
      textarea.setAttribute("readonly", "");
      const style = textarea.style;
      if (style) {
        if (typeof style.setProperty === "function") {
          style.setProperty("position", "fixed");
          style.setProperty("opacity", "0");
          style.setProperty("pointer-events", "none");
        } else {
          style.position = "fixed";
          style.opacity = "0";
          style.pointerEvents = "none";
        }
      }
      host.appendChild(textarea);
      const active = doc.activeElement;
      if (active && typeof active.focus === "function") active.focus();
    } catch (err) {
      removeQuietly(textarea, "copyText:create-failed");
      reporter.failure("copyText", "fallback-create-failed", err);
      return fail("fallback-create-failed", err);
    }
    try {
      if (typeof textarea.select === "function") textarea.select();
      if (typeof doc.execCommand !== "function") {
        return fail("exec-command-failed", new Error("document.execCommand \u4E0D\u53EF\u7528"));
      }
      let accepted = false;
      try {
        accepted = doc.execCommand("copy") === true;
      } catch (err) {
        reporter.failure("copyText", "exec-command-failed", err);
        return fail("exec-command-failed", err);
      }
      if (!accepted) return fail("exec-command-failed", new Error("document.execCommand('copy') \u8FD4\u56DE false"));
      return true;
    } finally {
      removeQuietly(textarea, "copyText:done");
    }
  }
  __name(copyText, "copyText");

  // src/lib/format.ts
  var BINARY_UNITS = ["B", "KiB", "MiB", "GiB", "TiB", "PiB", "EiB", "ZiB", "YiB"];
  var DECIMAL_UNITS = ["B", "kB", "MB", "GB", "TB", "PB", "EB", "ZB", "YB"];
  var NOT_A_NUMBER = "\u2014";
  var ALLOWED_BASES = [1024, 1e3];
  function requireIntInRange(label, value, min, max) {
    if (typeof value !== "number" || !Number.isFinite(value) || !Number.isInteger(value)) {
      throw new RangeError(`[format] ${label} \u5FC5\u987B\u662F [${min}, ${max}] \u5185\u7684\u6574\u6570\uFF0C\u6536\u5230 ${String(value)}`);
    }
    if (value < min || value > max) {
      throw new RangeError(`[format] ${label} \u8D85\u51FA\u8303\u56F4 [${min}, ${max}]\uFF0C\u6536\u5230 ${value}`);
    }
    return value;
  }
  __name(requireIntInRange, "requireIntInRange");
  function resolveBase(base) {
    if (base === void 0) return 1024;
    if (base !== 1024 && base !== 1e3) {
      throw new RangeError(`[format] base \u53EA\u80FD\u662F ${ALLOWED_BASES.join(" \u6216 ")}\uFF0C\u6536\u5230 ${String(base)}`);
    }
    return base;
  }
  __name(resolveBase, "resolveBase");
  function resolveDigits(label, digits, fallback) {
    if (digits === void 0) return fallback;
    return requireIntInRange(label, digits, 0, 100);
  }
  __name(resolveDigits, "resolveDigits");
  function formatBytes(n, opts = {}) {
    if (typeof n !== "number" || Number.isNaN(n)) return NOT_A_NUMBER;
    const base = resolveBase(opts.base);
    const digits = resolveDigits("digits", opts.digits, 1);
    if (n === Infinity || n === -Infinity) return n > 0 ? "\u221E" : "-\u221E";
    const units = base === 1024 ? BINARY_UNITS : DECIMAL_UNITS;
    const sign = n < 0 ? "-" : "";
    let magnitude = Math.abs(n);
    let unit = 0;
    while (magnitude >= base && unit < units.length - 1) {
      magnitude /= base;
      unit += 1;
    }
    const shown = unit === 0 ? String(magnitude) : magnitude.toFixed(digits);
    return `${sign}${shown} ${units[unit]}`;
  }
  __name(formatBytes, "formatBytes");
  function hexColumnWidth(bytesPerLine, gap = 1) {
    const width = requireIntInRange("bytesPerLine", bytesPerLine, 1, 4096);
    const sep = requireIntInRange("gap", gap, 1, 16);
    return width * 2 + (width - 1) * sep;
  }
  __name(hexColumnWidth, "hexColumnWidth");
  function byteToHex(value) {
    const b = requireIntInRange("byte", value, 0, 255);
    return b.toString(16).padStart(2, "0");
  }
  __name(byteToHex, "byteToHex");
  function formatHexRow(bytes, opts) {
    if (!opts || typeof opts !== "object") {
      throw new RangeError("[format] formatHexRow \u5FC5\u987B\u6536\u5230 { bytesPerLine } \u9009\u9879\uFF08\u6BCF\u884C\u5B57\u8282\u6570\u4E0D\u5F97\u9690\u5F0F\u9ED8\u8BA4\uFF09");
    }
    const bytesPerLine = requireIntInRange("bytesPerLine", opts.bytesPerLine, 1, 4096);
    const gap = requireIntInRange("gap", opts.gap === void 0 ? 1 : opts.gap, 1, 16);
    if (!Array.isArray(bytes)) {
      throw new RangeError("[format] formatHexRow \u7684 bytes \u5FC5\u987B\u662F\u6570\u7EC4");
    }
    if (bytes.length > bytesPerLine) {
      throw new RangeError(
        `[format] \u672C\u884C ${bytes.length} \u5B57\u8282\u8D85\u8FC7 bytesPerLine=${bytesPerLine}\uFF08\u4E0D\u5F97\u9759\u9ED8\u622A\u65AD\uFF09`
      );
    }
    const pad3 = opts.pad === true;
    const sep = " ".repeat(gap);
    const parts = bytes.map(byteToHex);
    let line = parts.join(sep);
    if (pad3 && bytes.length < bytesPerLine) {
      const width = hexColumnWidth(bytesPerLine, gap);
      line = line.padEnd(width, " ");
    }
    return line;
  }
  __name(formatHexRow, "formatHexRow");
  function formatHexRows(bytes, opts) {
    if (!opts || typeof opts !== "object") {
      throw new RangeError("[format] formatHexRows \u5FC5\u987B\u6536\u5230 { bytesPerLine } \u9009\u9879\uFF08\u6BCF\u884C\u5B57\u8282\u6570\u4E0D\u5F97\u9690\u5F0F\u9ED8\u8BA4\uFF09");
    }
    const bytesPerLine = requireIntInRange("bytesPerLine", opts.bytesPerLine, 1, 4096);
    if (!Array.isArray(bytes)) {
      throw new RangeError("[format] formatHexRows \u7684 bytes \u5FC5\u987B\u662F\u6570\u7EC4");
    }
    const rows = [];
    for (let i = 0; i < bytes.length; i += bytesPerLine) {
      rows.push(formatHexRow(bytes.slice(i, i + bytesPerLine), opts));
    }
    return rows;
  }
  __name(formatHexRows, "formatHexRows");

  // src/components/write-report.ts
  var ELLIPSIS = "\u2026";
  var PREVIEW_LIMIT = 160;
  var SHA_HEAD = 8;
  var SHA_TAIL = 8;
  var COPY_RESET_MS = 1600;
  function truncateSha256(value) {
    const v = (value ?? "").trim();
    if (!v) return "\u2014";
    if (v.length <= SHA_HEAD + SHA_TAIL) return v;
    return `${v.slice(0, SHA_HEAD)}${ELLIPSIS}${v.slice(-SHA_TAIL)}`;
  }
  __name(truncateSha256, "truncateSha256");
  function formatBytesDelta(delta) {
    if (typeof delta !== "number" || !Number.isFinite(delta)) return "\u2014";
    const n = Math.trunc(delta);
    if (n > 0) return `+${formatBytes(n)}`;
    return formatBytes(n);
  }
  __name(formatBytesDelta, "formatBytesDelta");
  function modeText(dryRun) {
    return dryRun ? { badge: "DRY RUN", text: "\u9884\u6F14\u6A21\u5F0F\uFF1A\u672A\u53D1\u751F\u4EFB\u4F55\u6587\u4EF6\u5199\u5165", title: "\u5199\u5165\u62A5\u544A\uFF08\u9884\u6F14\uFF09" } : { badge: "APPLIED", text: "\u771F\u5B9E\u5199\u5165\uFF1A\u5DF2\u63D0\u4EA4\u5230\u76EE\u6807\u6587\u4EF6", title: "\u5199\u5165\u62A5\u544A\uFF08\u5DF2\u5199\u5165\uFF09" };
  }
  __name(modeText, "modeText");
  function stableStringify(value) {
    try {
      const out = JSON.stringify(value);
      return out === void 0 ? String(value) : out;
    } catch (err) {
      console.error("[m11 write-report] manifest \u5E8F\u5217\u5316\u5931\u8D25\uFF0C\u9000\u56DE String():", err);
      return String(value);
    }
  }
  __name(stableStringify, "stableStringify");
  function preview(value) {
    if (value === null || value === void 0) return "\u2014";
    const text = typeof value === "string" ? value : stableStringify(value);
    const flat = text.replace(/\s+/g, " ").trim();
    return flat.length <= PREVIEW_LIMIT ? flat : `${flat.slice(0, PREVIEW_LIMIT)}${ELLIPSIS}`;
  }
  __name(preview, "preview");
  var WRITE_REPORT_TOKENS = Object.freeze({
    surface: "--color-bg-light",
    surfaceAlt: "--color-bg",
    border: "--color-border",
    text: "--color-text",
    textMuted: "--color-text-muted",
    textSecondary: "--color-text-secondary",
    radiusSm: "--border-radius-sm",
    radiusMd: "--border-radius-md",
    hairline: "--space-1",
    gap: "--space-8",
    pad: "--space-12",
    padSm: "--space-6",
    labelWidth: "--space-64",
    fontSize: "--font-size-13",
    fontSizeSm: "--font-size-12",
    fontSizeMd: "--font-size-15"
  });
  var _warnedTokens2 = /* @__PURE__ */ new Set();
  function tok(name) {
    if (isRegistered(name)) return `var(${name})`;
    if (!_warnedTokens2.has(name)) {
      _warnedTokens2.add(name);
      console.warn(
        `[m11 write-report] \u4EE4\u724C <${name}> \u672A\u5728\u4EE4\u724C\u6E90\u767B\u8BB0\uFF0C\u5DF2\u56DE\u843D\u5230 <${WRITE_REPORT_TOKENS.surface}>\uFF08\u9700 K1 \u56DE\u6D41\uFF09`
      );
    }
    return `var(${WRITE_REPORT_TOKENS.surface})`;
  }
  __name(tok, "tok");
  function toneDecls(t) {
    return [
      `border:${tok(WRITE_REPORT_TOKENS.hairline)} solid ${tok(t.border)}`,
      `background:${tok(t.bg)}`,
      `color:${tok(t.text)}`
    ].join(";");
  }
  __name(toneDecls, "toneDecls");
  var T = WRITE_REPORT_TOKENS;
  var STYLE_ROOT = [
    "display:flex",
    "flex-direction:column",
    `gap:${tok(T.gap)}`,
    `padding:${tok(T.pad)}`,
    `border-radius:${tok(T.radiusMd)}`,
    `border:${tok(T.hairline)} solid ${tok(T.border)}`,
    `background:${tok(T.surface)}`,
    `color:${tok(T.text)}`,
    `font-size:${tok(T.fontSize)}`
  ].join(";");
  var STYLE_ROW = ["display:flex", "flex-wrap:wrap", "align-items:baseline", `gap:${tok(T.gap)}`].join(";");
  var STYLE_LABEL = [`color:${tok(T.textSecondary)}`, `min-width:${tok(T.labelWidth)}`].join(";");
  var STYLE_VALUE = [`color:${tok(T.text)}`, "word-break:break-word"].join(";");
  var STYLE_VALUE_DIM = [`color:${tok(T.textSecondary)}`, "word-break:break-word"].join(";");
  var STYLE_BADGE = [
    `padding:${tok(T.hairline)} ${tok(T.padSm)}`,
    `border-radius:${tok(T.radiusMd)}`,
    `border:${tok(T.hairline)} solid ${tok(T.border)}`,
    `background:${tok(T.surfaceAlt)}`,
    `color:${tok(T.textMuted)}`,
    `font-size:${tok(T.fontSizeSm)}`
  ].join(";");
  function styleBadge(t) {
    return [
      `padding:${tok(T.hairline)} ${tok(T.padSm)}`,
      `border-radius:${tok(T.radiusMd)}`,
      `font-size:${tok(T.fontSizeSm)}`,
      toneDecls(t)
    ].join(";");
  }
  __name(styleBadge, "styleBadge");
  function styleBanner(t) {
    return [
      "display:flex",
      "align-items:center",
      `gap:${tok(T.gap)}`,
      `padding:${tok(T.padSm)}`,
      `border-radius:${tok(T.radiusSm)}`,
      `font-weight:600`,
      toneDecls(t)
    ].join(";");
  }
  __name(styleBanner, "styleBanner");
  function styleButton(disabled) {
    const base = [
      `padding:${tok(T.padSm)} ${tok(T.pad)}`,
      `border-radius:${tok(T.radiusSm)}`,
      `border:${tok(T.hairline)} solid ${tok(T.border)}`,
      `background:${tok(T.surfaceAlt)}`,
      `font-size:${tok(T.fontSizeSm)}`
    ];
    return base.concat(disabled ? [`color:${tok(T.textMuted)}`, "cursor:not-allowed"] : [`color:${tok(T.text)}`, "cursor:pointer"]).join(";");
  }
  __name(styleButton, "styleButton");
  var seq = 0;
  function createWriteReport(options) {
    const doc = document;
    const disposer = createDisposer();
    const dryRun = options.dry_run === true;
    const isInput = options.target_is_input === true;
    const delta = typeof options.bytes_delta === "number" && Number.isFinite(options.bytes_delta) ? Math.trunc(options.bytes_delta) : null;
    const hasRollbackChannel = typeof options.onRollback === "function";
    const canRollback = !dryRun && hasRollbackChannel;
    seq += 1;
    const titleId = `petui-write-report-title-${seq}`;
    const root = doc.createElement("section");
    let destroyed = false;
    let copyNote = "";
    const el = /* @__PURE__ */ __name((tag, attrs = {}, cssText = "") => {
      const node = doc.createElement(tag);
      if (cssText) node.style.cssText = cssText;
      for (const key of Object.keys(attrs)) {
        const value = attrs[key];
        if (key === "text") node.textContent = value;
        else if (key === "disabled") node.setAttribute("disabled", "disabled");
        else node.setAttribute(key, value);
      }
      return node;
    }, "el");
    const copyInto = /* @__PURE__ */ __name((value, button, label) => {
      const original = button.textContent || label;
      void copyText(value).then((okCopy) => {
        if (destroyed) return;
        button.textContent = okCopy ? "\u5DF2\u590D\u5236" : "\u590D\u5236\u5931\u8D25";
        copyNote = okCopy ? "" : "\u526A\u8D34\u677F\u4E24\u6761\u8DEF\u5F84\u90FD\u5931\u8D25\uFF0C\u5B8C\u6574\u503C\u4ECD\u53EF\u7531 title \u5C5E\u6027\u67E5\u770B";
        disposer.setTimeout(() => {
          if (!destroyed) button.textContent = original;
        }, COPY_RESET_MS);
      });
    }, "copyInto");
    function row(label, valueNode, rowRole) {
      const wrap = el("div", { "data-role": rowRole }, STYLE_ROW);
      wrap.appendChild(el("span", { "data-role": `${rowRole}-label`, text: label }, STYLE_LABEL));
      wrap.appendChild(valueNode);
      return wrap;
    }
    __name(row, "row");
    function shaRow(label, value, tag) {
      const full = (value ?? "").trim();
      const code = el(
        "code",
        {
          "data-role": `sha-${tag}`,
          "data-full": full,
          title: full || "\uFF08\u8BE5\u9636\u6BB5\u6458\u8981\u7F3A\u5931\uFF09",
          text: truncateSha256(full)
        },
        STYLE_VALUE
      );
      const copy = el(
        "button",
        {
          type: "button",
          "data-role": `copy-${tag}`,
          "data-copy-length": String(full.length),
          "aria-label": `\u590D\u5236\u5B8C\u6574 ${label}`,
          text: "\u590D\u5236\u5B8C\u6574\u503C"
        },
        styleButton(false)
      );
      disposer.addEventListener(copy, "click", () => copyInto(full, copy, "\u590D\u5236\u5B8C\u6574\u503C"));
      const wrap = row(label, code, `sha-row-${tag}`);
      wrap.appendChild(copy);
      return wrap;
    }
    __name(shaRow, "shaRow");
    function reportText() {
      const mode = modeText(dryRun);
      return [
        `# \u5199\u5165\u62A5\u544A\uFF08${mode.badge}\uFF09`,
        `\u5F62\u6001: ${mode.text}`,
        `\u8DEF\u5F84: ${options.path}`,
        `\u76EE\u6807\u5305: ${options.target_pkg || "\u2014"}${isInput ? "\uFF08\u8F93\u5165\u6587\u4EF6\uFF09" : "\uFF08\u975E\u8F93\u5165\u6587\u4EF6\uFF09"}`,
        `\u5B57\u8282\u589E\u91CF: ${formatBytesDelta(delta)}`,
        `pre_sha256: ${options.pre_sha256 || "\u2014"}`,
        `post_sha256: ${options.post_sha256 || "\u2014"}`,
        `\u6E90\u73B0\u503C: ${preview(options.old)}`,
        `\u5199\u5165\u76EE\u6807: ${preview(options.new)}`,
        `\u6E05\u5355\u884C: ${preview(options.manifest_row)}`
      ].join("\n");
    }
    __name(reportText, "reportText");
    function render() {
      if (destroyed) return;
      const mode = modeText(dryRun);
      const warnTone = TONE_TOKENS.warning;
      const okTone = TONE_TOKENS.success;
      while (root.childNodes.length > 0) root.removeChild(root.childNodes[0]);
      root.setAttribute("data-role", "write-report");
      root.setAttribute("data-dry-run", dryRun ? "true" : "false");
      root.setAttribute("data-can-rollback", canRollback ? "true" : "false");
      root.setAttribute("aria-labelledby", titleId);
      root.style.cssText = STYLE_ROOT;
      const banner = el(
        "div",
        { "data-role": "mode-banner", "data-mode": dryRun ? "dry_run" : "applied" },
        styleBanner(dryRun ? warnTone : okTone)
      );
      banner.appendChild(el("span", { "data-role": "mode-badge", text: mode.badge }, STYLE_BADGE));
      banner.appendChild(el("span", { "data-role": "mode-text", text: mode.text }, STYLE_VALUE));
      root.appendChild(banner);
      root.appendChild(
        el(
          "h3",
          { id: titleId, "data-role": "title", text: mode.title },
          ["margin:0", `font-size:${tok(T.fontSizeMd)}`, "font-weight:600", `color:${tok(T.text)}`].join(";")
        )
      );
      root.appendChild(
        row("\u8DEF\u5F84", el("code", { "data-role": "path", text: options.path }, STYLE_VALUE), "path-row")
      );
      const targetRow = row(
        "\u76EE\u6807\u5305",
        el("code", { "data-role": "target-pkg", text: options.target_pkg || "\u2014" }, STYLE_VALUE),
        "target-row"
      );
      targetRow.appendChild(
        isInput ? el("span", { "data-role": "target-is-input", "data-value": "true", text: "\u8F93\u5165\u6587\u4EF6" }, styleBadge(TONE_TOKENS.info)) : el("span", { "data-role": "target-is-input", "data-value": "false", text: "\u975E\u8F93\u5165\u6587\u4EF6" }, STYLE_BADGE)
      );
      root.appendChild(targetRow);
      const deltaText = formatBytesDelta(delta);
      const deltaStyle = delta !== null && delta > 0 ? `${STYLE_VALUE};color:${tok(TONE_TOKENS.success.text)}` : delta !== null && delta < 0 ? `${STYLE_VALUE};color:${tok(TONE_TOKENS.error.text)}` : STYLE_VALUE;
      root.appendChild(
        row("\u5B57\u8282\u589E\u91CF", el("code", { "data-role": "bytes-delta", "data-value": deltaText, text: deltaText }, deltaStyle), "bytes-row")
      );
      root.appendChild(shaRow("\u5199\u5165\u524D SHA-256", options.pre_sha256, "pre"));
      root.appendChild(shaRow("\u5199\u5165\u540E SHA-256", options.post_sha256, "post"));
      const contentRow = el("div", { "data-role": "content-row" }, STYLE_ROW);
      contentRow.appendChild(el("span", { "data-role": "content-row-label", text: "\u5185\u5BB9\u53D8\u66F4" }, STYLE_LABEL));
      contentRow.appendChild(el("span", { "data-role": "old-preview", text: preview(options.old) }, STYLE_VALUE_DIM));
      contentRow.appendChild(el("span", { "data-role": "content-arrow", text: "\u2192" }, STYLE_VALUE_DIM));
      contentRow.appendChild(el("span", { "data-role": "new-preview", text: preview(options.new) }, STYLE_VALUE));
      root.appendChild(contentRow);
      root.appendChild(
        row("\u6E05\u5355\u884C", el("code", { "data-role": "manifest-row-value", text: preview(options.manifest_row) }, STYLE_VALUE), "manifest-row")
      );
      if (options.manifest !== void 0) {
        root.appendChild(
          row("\u6E05\u5355", el("code", { "data-role": "manifest", text: preview(options.manifest) }, STYLE_VALUE), "manifest-summary-row")
        );
      }
      const actions = el("div", { "data-role": "actions" }, STYLE_ROW);
      const rollback = el(
        "button",
        dryRun ? {
          type: "button",
          "data-role": "rollback",
          "data-reason": "dry_run",
          disabled: "disabled",
          "aria-disabled": "true",
          title: "\u9884\u6F14\u6CA1\u6709\u5199\u5165\uFF0C\u65E0\u9700\u56DE\u6EDA\uFF1B\u51FA\u8DEF = \u771F\u5B9E\u5199\u5165\u540E\u518D\u56DE\u6EDA",
          text: "\u56DE\u6EDA\uFF08\u9884\u6F14\uFF1A\u65E0\u5199\u5165\uFF09"
        } : !hasRollbackChannel ? {
          type: "button",
          "data-role": "rollback",
          "data-reason": "no_onRollback",
          disabled: "disabled",
          "aria-disabled": "true",
          title: "\u672A\u63D0\u4F9B onRollback \u56DE\u8C03\uFF0C\u65E0\u56DE\u6EDA\u901A\u9053\uFF1B\u51FA\u8DEF = \u4F20\u5165 onRollback",
          text: "\u56DE\u6EDA\uFF08\u7F3A\u56DE\u8C03\uFF09"
        } : {
          type: "button",
          "data-role": "rollback",
          "data-reason": "ready",
          title: "\u89E6\u53D1\u4E0A\u5C42\u56DE\u6EDA\u56DE\u8C03\uFF1B\u672C\u7EC4\u4EF6\u4E0D\u6267\u884C\u4EFB\u4F55\u5199\u5165",
          text: "\u56DE\u6EDA\u6B64\u6587\u4EF6"
        },
        styleButton(!canRollback)
      );
      if (canRollback) {
        disposer.addEventListener(rollback, "click", () => {
          try {
            options.onRollback({
              path: options.path,
              target_pkg: options.target_pkg ?? null,
              target_is_input: isInput,
              pre_sha256: options.pre_sha256 ?? null,
              post_sha256: options.post_sha256 ?? null,
              bytes_delta: delta,
              manifest_row: options.manifest_row,
              dry_run: false
            });
          } catch (err) {
            console.error("[m11 write-report] onRollback \u56DE\u8C03\u629B\u9519\uFF08\u5DF2\u6355\u83B7\uFF0C\u9762\u677F\u4E0D\u53D7\u5F71\u54CD\uFF09:", err);
          }
        });
      }
      actions.appendChild(rollback);
      const copyReport = el(
        "button",
        { type: "button", "data-role": "copy-report", "aria-label": "\u590D\u5236\u5199\u5165\u62A5\u544A\u6587\u672C", text: "\u590D\u5236\u62A5\u544A" },
        styleButton(false)
      );
      disposer.addEventListener(copyReport, "click", () => copyInto(reportText(), copyReport, "\u590D\u5236\u62A5\u544A"));
      actions.appendChild(copyReport);
      root.appendChild(actions);
      if (copyNote) {
        root.appendChild(el("p", { "data-role": "copy-note", text: copyNote }, STYLE_VALUE_DIM));
      }
    }
    __name(render, "render");
    function destroy() {
      if (destroyed) return;
      destroyed = true;
      disposer.dispose();
      while (root.childNodes.length > 0) root.removeChild(root.childNodes[0]);
    }
    __name(destroy, "destroy");
    root.render = render;
    root.destroy = destroy;
    render();
    return root;
  }
  __name(createWriteReport, "createWriteReport");

  // src/components/data-table-util.ts
  var ALIGN_VALUES = /* @__PURE__ */ new Set(["left", "center", "right"]);
  var ROW_HEIGHT_VALUES = /* @__PURE__ */ new Set(["compact", "normal", "comfortable", "tall"]);
  var WIDTH_SHAPE = /^var\(\s*(--[A-Za-z0-9_-]{1,64})\s*\)$/;
  function normalizeAlign(align) {
    return typeof align === "string" && ALIGN_VALUES.has(align) ? align : "left";
  }
  __name(normalizeAlign, "normalizeAlign");
  function normalizeRowHeight(value) {
    return typeof value === "string" && ROW_HEIGHT_VALUES.has(value) ? value : "normal";
  }
  __name(normalizeRowHeight, "normalizeRowHeight");
  function normalizeWidth(width) {
    if (typeof width !== "string") return null;
    const m = WIDTH_SHAPE.exec(width.trim());
    if (m === null) return null;
    return isRegistered(m[1]) ? `var(${m[1]})` : null;
  }
  __name(normalizeWidth, "normalizeWidth");
  function cellText(value) {
    if (value === null || value === void 0) return "";
    if (typeof value === "string") return value;
    if (typeof value === "number" || typeof value === "boolean" || typeof value === "bigint") return String(value);
    if (value instanceof Date) return value.toISOString();
    try {
      const out = JSON.stringify(value);
      return out === void 0 ? String(value) : out;
    } catch (err) {
      console.error("[m14 data-table] \u5355\u5143\u683C\u503C\u5E8F\u5217\u5316\u5931\u8D25\uFF0C\u56DE\u843D String():", err);
      return String(value);
    }
  }
  __name(cellText, "cellText");
  function compareCell(a, b) {
    const ea = a === null || a === void 0 || a === "";
    const eb = b === null || b === void 0 || b === "";
    if (ea && eb) return 0;
    if (ea) return 1;
    if (eb) return -1;
    if (typeof a === "number" && typeof b === "number") return a < b ? -1 : a > b ? 1 : 0;
    const sa = cellText(a);
    const sb = cellText(b);
    return sa < sb ? -1 : sa > sb ? 1 : 0;
  }
  __name(compareCell, "compareCell");
  function sortRows(rows, sort) {
    const copy = rows.slice();
    if (sort === null) return copy;
    const indexed = copy.map((row, index) => ({ row, index }));
    indexed.sort((x, y) => {
      const c = compareCell(x.row[sort.key], y.row[sort.key]);
      if (c !== 0) return sort.direction === "asc" ? c : -c;
      return x.index - y.index;
    });
    return indexed.map((entry) => entry.row);
  }
  __name(sortRows, "sortRows");
  function nextSort(current, key) {
    if (current === null || current.key !== key) return { key, direction: "asc" };
    if (current.direction === "asc") return { key, direction: "desc" };
    return null;
  }
  __name(nextSort, "nextSort");

  // src/components/data-table-col-resize.ts
  var COL_GRIP_CLASS = "petui-data-table__col-grip";
  function applyColumnWidths(ths, keys, widths) {
    for (let i = 0; i < ths.length; i += 1) {
      const th = ths[i];
      const px3 = widths[keys[i]];
      if (th && typeof px3 === "number" && Number.isFinite(px3) && px3 > 0) th.style.width = px3 + "px";
    }
  }
  __name(applyColumnWidths, "applyColumnWidths");
  function clampPair(value, total, min) {
    const upper = Math.max(min, total - min);
    return Math.max(min, Math.min(upper, value));
  }
  __name(clampPair, "clampPair");
  function attachColumnResize(o) {
    const doc = globalThis.document;
    let attached = 0;
    for (let i = 0; i < o.ths.length - 1; i += 1) {
      const th = o.ths[i];
      const nextTh = o.ths[i + 1];
      const k0 = o.keys[i];
      const k1 = o.keys[i + 1];
      if (!th || !nextTh || k0 === void 0 || k1 === void 0) continue;
      th.style.position = "relative";
      const grip = document.createElement("span");
      grip.className = COL_GRIP_CLASS;
      grip.setAttribute("aria-hidden", "true");
      grip.style.cssText = "position:absolute;top:0;right:0;width:var(--space-8);height:100%;cursor:col-resize;touch-action:none;z-index:1";
      const start = /* @__PURE__ */ __name((ev) => {
        const event = ev;
        if (event.button !== void 0 && event.button !== 0) return;
        if (typeof event.preventDefault === "function") event.preventDefault();
        if (typeof event.stopPropagation === "function") event.stopPropagation();
        const w0 = o.widths[k0] || th.offsetWidth || 0;
        const w1 = o.widths[k1] || nextTh.offsetWidth || 0;
        const total = w0 + w1;
        if (total <= 0) return;
        const startX = Number(event.clientX) || 0;
        let last0 = w0;
        let last1 = w1;
        const move = /* @__PURE__ */ __name((mv) => {
          const dx = (Number(mv.clientX) || 0) - startX;
          last0 = Math.round(clampPair(w0 + dx, total, o.minPx));
          last1 = total - last0;
          th.style.width = last0 + "px";
          nextTh.style.width = last1 + "px";
        }, "move");
        const end = /* @__PURE__ */ __name(() => {
          if (doc) {
            doc.removeEventListener("pointermove", move);
            doc.removeEventListener("pointerup", end);
            doc.removeEventListener("pointercancel", end);
          }
          o.widths[k0] = last0;
          o.widths[k1] = last1;
          o.onChange(Object.assign({}, o.widths));
        }, "end");
        if (doc) {
          o.register(doc, "pointermove", move);
          o.register(doc, "pointerup", end);
          o.register(doc, "pointercancel", end);
        }
      }, "start");
      o.register(grip, "pointerdown", start);
      th.appendChild(grip);
      attached += 1;
    }
    return attached;
  }
  __name(attachColumnResize, "attachColumnResize");

  // src/components/data-table.ts
  var DATA_TABLE_TOKENS = Object.freeze({
    surface: "--color-bg-light",
    headerBg: "--color-bg",
    border: "--color-border",
    hairline: "--color-n-f0f0f0",
    text: "--color-text",
    textSecondary: "--color-text-secondary",
    textMuted: "--color-text-muted",
    primary: "--color-primary",
    primaryLight: "--color-primary-light",
    radiusSm: "--border-radius-sm",
    padX: "--space-10",
    padY: "--space-6",
    gap: "--space-8",
    selectCol: "--space-24",
    fontSm: "--font-size-12",
    fontMd: "--font-size-13",
    // ── 编辑 / 新增（M14 扩展）用到的令牌；全部为已登记令牌 ──
    reject: "--color-danger",
    accent: "--color-warning",
    radiusMd: "--border-radius-md",
    hairlineWidth: "--space-1",
    padBtnX: "--space-12",
    editHeight: "--space-32"
  });
  var DATA_TABLE_ROW_HEIGHTS = Object.freeze({
    compact: "--row-height-compact",
    normal: "--row-height-normal",
    comfortable: "--row-height-comfortable",
    tall: "--row-height-tall"
  });
  var _warnedTokens3 = /* @__PURE__ */ new Set();
  function tok2(name) {
    if (isRegistered(name)) return `var(${name})`;
    if (!_warnedTokens3.has(name)) {
      _warnedTokens3.add(name);
      console.warn(
        `[m14 data-table] \u4EE4\u724C <${name}> \u672A\u5728\u4EE4\u724C\u6E90\u767B\u8BB0\uFF0C\u5DF2\u56DE\u843D\u5230 <${DATA_TABLE_TOKENS.text}>\uFF08\u9700 K1 \u56DE\u6D41\uFF09`
      );
    }
    return `var(${DATA_TABLE_TOKENS.text})`;
  }
  __name(tok2, "tok");
  var T2 = DATA_TABLE_TOKENS;
  var STYLE_ROOT2 = [
    "display:block",
    "overflow-x:auto",
    // 宽度必须是**长度令牌**。原先写的是 `${tok(T.hairline)}`，而本模块的 `T.hairline`
    // 是**颜色**（`--color-n-f0f0f0`）⇒ `border: <color> solid <color>` 非法，浏览器**整条丢弃**
    // ⇒ 表格既没有外框也没有内线（实机实测 borderBottomWidth=0px none）。
    `border:${tok2(T2.hairlineWidth)} solid ${tok2(T2.border)}`,
    `border-radius:${tok2(T2.radiusSm)}`,
    `background:${tok2(T2.surface)}`,
    `color:${tok2(T2.text)}`,
    `font-size:${tok2(T2.fontMd)}`
  ].join(";");
  var STYLE_TABLE = ["width:100%", "border-collapse:collapse", "table-layout:fixed"].join(";");
  function styleHeaderCell(align, rowHeightToken, lastColumn = false) {
    const rules = [
      `height:${tok2(rowHeightToken)}`,
      `padding:${tok2(T2.padY)} ${tok2(T2.padX)}`,
      `text-align:${align}`,
      `background:${tok2(T2.headerBg)}`,
      // 宽度用长度令牌（原来误用颜色令牌 ⇒ 声明非法、边框全丢）
      `border-bottom:${tok2(T2.hairlineWidth)} solid ${tok2(T2.border)}`,
      "font-weight:600",
      `color:${tok2(T2.textSecondary)}`,
      "white-space:nowrap",
      `font-size:${tok2(T2.fontSm)}`
    ];
    if (!lastColumn) rules.push(`border-right:${tok2(T2.hairlineWidth)} solid ${tok2(T2.border)}`);
    return rules.join(";");
  }
  __name(styleHeaderCell, "styleHeaderCell");
  function styleSortButton(align) {
    return [
      "display:inline-flex",
      "align-items:center",
      `gap:${tok2(T2.gap)}`,
      "width:100%",
      "padding:0",
      "border:0",
      "background:transparent",
      `color:inherit`,
      "font:inherit",
      "cursor:pointer",
      `justify-content:${align === "right" ? "flex-end" : align === "center" ? "center" : "flex-start"}`
    ].join(";");
  }
  __name(styleSortButton, "styleSortButton");
  function styleCell(align, rowHeightToken, lastColumn = false) {
    const rules = [
      `height:${tok2(rowHeightToken)}`,
      `padding:${tok2(T2.padY)} ${tok2(T2.padX)}`,
      `text-align:${align}`,
      `border-bottom:${tok2(T2.hairlineWidth)} solid ${tok2(T2.border)}`,
      `color:${tok2(T2.text)}`,
      "white-space:nowrap",
      "overflow:hidden",
      "text-overflow:ellipsis",
      "vertical-align:middle"
    ];
    if (!lastColumn) rules.push(`border-right:${tok2(T2.hairlineWidth)} solid ${tok2(T2.border)}`);
    return rules.join(";");
  }
  __name(styleCell, "styleCell");
  function styleRow(clickable) {
    return [`cursor:${clickable ? "pointer" : "default"}`].join(";");
  }
  __name(styleRow, "styleRow");
  function styleEmptyCell(rowHeightToken) {
    return [
      `height:${tok2(rowHeightToken)}`,
      `padding:${tok2(T2.padX)}`,
      "text-align:center",
      `color:${tok2(T2.textMuted)}`,
      `font-size:${tok2(T2.fontSm)}`
    ].join(";");
  }
  __name(styleEmptyCell, "styleEmptyCell");
  var STYLE_SELECT_CELL = [
    `width:${tok2(T2.selectCol)}`,
    "text-align:center",
    `padding:${tok2(T2.padY)} 0`
  ].join(";");
  function styleEditInput(align) {
    return [
      "box-sizing:border-box",
      "width:100%",
      `height:${tok2(T2.editHeight)}`,
      `padding:0 ${tok2(T2.padX)}`,
      `text-align:${align}`,
      `color:${tok2(T2.text)}`,
      `background:${tok2(T2.surface)}`,
      `border-style:solid`,
      `border-width:${tok2(T2.hairlineWidth)}`,
      `border-color:${tok2(T2.primary)}`,
      `border-radius:${tok2(T2.radiusSm)}`,
      "font:inherit",
      "outline:none"
    ].join(";");
  }
  __name(styleEditInput, "styleEditInput");
  function styleEditInputRejected(align) {
    return `${styleEditInput(align)};border-color:${tok2(T2.reject)}`;
  }
  __name(styleEditInputRejected, "styleEditInputRejected");
  var STYLE_EDIT_STATUS = [
    `padding:${tok2(T2.padY)} ${tok2(T2.padX)}`,
    `color:${tok2(T2.reject)}`,
    `font-size:${tok2(T2.fontSm)}`
  ].join(";");
  var STYLE_ADD_BUTTON = [
    `margin:${tok2(T2.gap)}`,
    `padding:${tok2(T2.padY)} ${tok2(T2.padBtnX)}`,
    `border-width:${tok2(T2.hairlineWidth)}`,
    "border-style:solid",
    `border-color:${tok2(T2.border)}`,
    `border-radius:${tok2(T2.radiusMd)}`,
    `background:${tok2(T2.headerBg)}`,
    `color:${tok2(T2.primary)}`,
    "font:inherit",
    "cursor:pointer"
  ].join(";");
  var seq2 = 0;
  function createDataTable(options = {}) {
    if (typeof document === "undefined") {
      throw new Error("[m14 data-table] \u9700\u8981 DOM \u73AF\u5883\uFF1A\u5F53\u524D\u6CA1\u6709 document");
    }
    const doc = document;
    const disposer = createDisposer();
    seq2 += 1;
    const instanceId = `petui-data-table-${seq2}`;
    const rowHeight = normalizeRowHeight(options.rowHeight);
    const rowHeightToken = DATA_TABLE_ROW_HEIGHTS[rowHeight];
    const selectable = options.selectable === true;
    const emptyText = typeof options.emptyText === "string" && options.emptyText.length > 0 ? options.emptyText : "\u6682\u65E0\u6570\u636E";
    const columns = Array.isArray(options.columns) ? options.columns.filter((c) => c !== null && c !== void 0 && typeof c.key === "string") : [];
    let rows = Array.isArray(options.rows) ? options.rows.slice() : [];
    let sort = options.sort && typeof options.sort.key === "string" && (options.sort.direction === "asc" || options.sort.direction === "desc") ? { key: options.sort.key, direction: options.sort.direction } : null;
    const selected = /* @__PURE__ */ new Set();
    let destroyed = false;
    let renderSeq = 0;
    const highlightOnClick = options.highlightOnClick === true;
    let activeKey = typeof options.activeRowKey === "string" ? options.activeRowKey : null;
    const ACTIVE_ROW_STYLE = `background:${tok2(T2.primaryLight)};color:${tok2(T2.primary)}`;
    const rowEls = /* @__PURE__ */ new Map();
    const colWidths = {};
    if (options.columnWidths !== null && typeof options.columnWidths === "object") {
      for (const [key, px3] of Object.entries(options.columnWidths)) {
        if (typeof px3 === "number" && Number.isFinite(px3) && px3 > 0) colWidths[key] = px3;
      }
    }
    const columnMinWidth = typeof options.columnMinWidth === "number" && options.columnMinWidth > 0 ? options.columnMinWidth : 48;
    function setActiveRow(key) {
      activeKey = typeof key === "string" && key.length > 0 ? key : null;
      for (const [rowKey, item] of rowEls) {
        const on = activeKey !== null && rowKey === activeKey;
        item.tr.style.cssText = on ? item.base + ";" + ACTIVE_ROW_STYLE : item.base;
        if (on) item.tr.setAttribute("aria-current", "true");
        else item.tr.removeAttribute("aria-current");
      }
    }
    __name(setActiveRow, "setActiveRow");
    const editable = options.editable === true;
    const addable = options.addable === true && columns.length > 0;
    let editingKey = null;
    let editingColumn = null;
    let draft = "";
    let rejectReason = null;
    const touched = /* @__PURE__ */ new Map();
    let focusPending = false;
    let pendingFocusInput = null;
    let committing = false;
    let lastEditChange = null;
    function currentSession() {
      if (editingKey === null || editingColumn === null) return null;
      const key = editingKey;
      const columnKey = editingColumn;
      const entry = view().find((item) => item.key === key);
      if (entry === void 0) return null;
      const column = columns.find((c) => c.key === columnKey);
      if (column === void 0) return null;
      return {
        index: entry.index,
        key: entry.key,
        column,
        row: entry.row,
        previous: entry.row[column.key],
        draft,
        input: null
      };
    }
    __name(currentSession, "currentSession");
    function clearSession(change) {
      if (editingKey === null) return false;
      if (change !== null) {
        const copy = Object.assign({}, change.row);
        copy[change.column] = change.value;
        touched.set(change.key, copy);
        lastEditChange = Object.freeze({
          key: change.key,
          index: change.index,
          column: change.column,
          previous: change.previous,
          value: change.value,
          row: copy
        });
      }
      editingKey = null;
      editingColumn = null;
      draft = "";
      rejectReason = null;
      focusPending = false;
      return true;
    }
    __name(clearSession, "clearSession");
    function rejectionFor(session, next) {
      if (typeof options.validate !== "function") return null;
      try {
        const out = options.validate(session.row, session.column, next);
        return typeof out === "string" && out.length > 0 ? out : null;
      } catch (err) {
        const message = err instanceof Error ? err.message : String(err);
        console.error(`[m14 data-table] \u5217 <${session.column.key}> \u7684 validate() \u629B\u9519\uFF0C\u6309\u62D2\u7EDD\u5904\u7406:`, err);
        return `validate \u629B\u9519\uFF1A${message}`;
      }
    }
    __name(rejectionFor, "rejectionFor");
    function rejectEdit(session, reason) {
      rejectReason = reason;
      console.warn(
        `[m14 data-table] \u6821\u9A8C\u672A\u901A\u8FC7(\u5217 <${session.column.key}>\uFF0C\u539F\u56E0 <${reason}>)\uFF1A\u5DF2\u62D2\u7EDD\u63D0\u4EA4\u5E76\u4FDD\u6301\u7F16\u8F91\u6001`
      );
      focusPending = true;
      render();
      notify("onBlockedEdit", () => options.onBlockedEdit(reason, session.column.key));
    }
    __name(rejectEdit, "rejectEdit");
    function commitEdit() {
      if (destroyed || committing) return null;
      const session = currentSession();
      if (session === null) {
        clearSession(null);
        return null;
      }
      const next = draft;
      const nextText = cellText(session.previous);
      if (next === nextText) {
        clearSession(null);
        render();
        return null;
      }
      const reason = rejectionFor(session, next);
      if (reason !== null) {
        rejectEdit(session, reason);
        return null;
      }
      const change = {
        key: session.key,
        index: session.index,
        column: session.column.key,
        previous: session.previous,
        value: next,
        row: session.row
      };
      clearSession(change);
      const snapshot2 = lastEditChange;
      committing = true;
      try {
        render();
        notify("onEdit", () => {
          if (typeof options.onEdit === "function" && snapshot2 !== null) options.onEdit(snapshot2);
        });
      } finally {
        committing = false;
      }
      return snapshot2;
    }
    __name(commitEdit, "commitEdit");
    function cancelEdit() {
      if (destroyed || editingKey === null) return;
      clearSession(null);
      render();
    }
    __name(cancelEdit, "cancelEdit");
    function beginEdit(rowIndex, columnKey) {
      if (destroyed || !editable) return false;
      const index = typeof rowIndex === "number" && Number.isFinite(rowIndex) ? Math.trunc(rowIndex) : -1;
      const entries = view();
      if (index < 0 || index >= entries.length) return false;
      const entry = entries[index];
      const column = columns.find((c) => c.key === columnKey);
      if (column === void 0) return false;
      editingKey = entry.key;
      editingColumn = column.key;
      draft = cellText(entry.row[column.key]);
      rejectReason = null;
      focusPending = true;
      render();
      return true;
    }
    __name(beginEdit, "beginEdit");
    function syncDraft(session) {
      const input = session.input;
      if (input !== null && typeof input.value === "string" && editingKey !== null) draft = input.value;
    }
    __name(syncDraft, "syncDraft");
    function editInput(session) {
      const align = normalizeAlign(session.column.align);
      const input = doc.createElement("input");
      input.setAttribute("type", "text");
      input.setAttribute("data-role", "cell-input");
      input.setAttribute("data-column-key", session.column.key);
      input.setAttribute("aria-label", `\u7F16\u8F91\u7B2C ${session.index + 1} \u884C\u300C${session.column.title}\u300D`);
      input.value = session.draft;
      input.style.cssText = (rejectReason === null ? styleEditInput(align) : styleEditInputRejected(align)) + ";user-select:text;-webkit-user-select:text";
      if (rejectReason !== null) {
        input.setAttribute("aria-invalid", "true");
        input.setAttribute("title", `\u6821\u9A8C\u672A\u901A\u8FC7\uFF1A${rejectReason}`);
      }
      session.input = input;
      disposer.addEventListener(input, "input", () => syncDraft(session));
      disposer.addEventListener(input, "blur", () => {
        if (destroyed || committing || editingKey === null) return;
        syncDraft(session);
        commitEdit();
      });
      disposer.addEventListener(input, "keydown", (event) => {
        const key = event.key;
        if (key === "Enter") {
          event.preventDefault();
          syncDraft(session);
          commitEdit();
          return;
        }
        if (key === "Escape") {
          event.preventDefault();
          cancelEdit();
        }
      });
      if (focusPending) pendingFocusInput = input;
      return input;
    }
    __name(editInput, "editInput");
    function addRow(row) {
      if (destroyed) {
        console.warn("[m14 data-table] destroy() \u4E4B\u540E\u8C03\u7528 addRow() \u5DF2\u5FFD\u7565\uFF08\u9700\u91CD\u65B0 createDataTable\uFF09");
        return {};
      }
      const created = row !== void 0 ? row : buildNewRow();
      const safe = created !== null && typeof created === "object" ? created : {};
      rows = rows.concat([safe]);
      render();
      const index = view().findIndex((entry) => entry.row === safe);
      notify("onAdd", () => {
        if (typeof options.onAdd === "function") options.onAdd(safe, index < 0 ? rows.length - 1 : index);
      });
      if (editable && columns.length > 0 && index >= 0) beginEdit(index, columns[0].key);
      return safe;
    }
    __name(addRow, "addRow");
    function emptyRowShape() {
      const shape = {};
      for (const column of columns) shape[column.key] = "";
      return shape;
    }
    __name(emptyRowShape, "emptyRowShape");
    function buildNewRow() {
      if (typeof options.newRow !== "function") return emptyRowShape();
      let produced;
      try {
        produced = options.newRow();
      } catch (err) {
        console.error("[m14 data-table] newRow() \u629B\u9519\uFF0C\u56DE\u843D\u7A7A\u4E32\u884C:", err);
        return emptyRowShape();
      }
      if (produced === null || produced === void 0 || typeof produced !== "object") {
        console.warn("[m14 data-table] newRow() \u672A\u8FD4\u56DE\u5BF9\u8C61\uFF0C\u5DF2\u56DE\u843D\u6309 columns[].key \u751F\u6210\u7684\u7A7A\u4E32\u884C");
        return emptyRowShape();
      }
      return produced;
    }
    __name(buildNewRow, "buildNewRow");
    const root = doc.createElement("div");
    function rowKeyOf(row, index) {
      const spec = options.rowKey;
      if (typeof spec === "function") {
        const custom = spec(row, index);
        return typeof custom === "string" && custom.length > 0 ? custom : `row-${index}`;
      }
      const field = typeof spec === "string" && spec.length > 0 ? spec : "id";
      const value = row[field];
      return value === null || value === void 0 || value === "" ? `row-${index}` : String(value);
    }
    __name(rowKeyOf, "rowKeyOf");
    function view() {
      return sortRows(rows, sort).map((original, index) => {
        const key = rowKeyOf(original, index);
        const local = touched.get(key);
        return { row: local === void 0 ? original : local, key, index };
      });
    }
    __name(view, "view");
    function pruneSelection() {
      const alive = new Set(view().map((entry) => entry.key));
      for (const key of Array.from(selected)) {
        if (!alive.has(key)) selected.delete(key);
      }
    }
    __name(pruneSelection, "pruneSelection");
    function selectedSnapshot() {
      const entries = view();
      const keys = [];
      const picked = [];
      for (const entry of entries) {
        if (selected.has(entry.key)) {
          keys.push(entry.key);
          picked.push(entry.row);
        }
      }
      return { keys, rows: picked };
    }
    __name(selectedSnapshot, "selectedSnapshot");
    function notify(label, fn) {
      try {
        fn();
      } catch (err) {
        console.error(`[m14 data-table] ${label} \u56DE\u8C03\u629B\u9519\uFF08\u5DF2\u6355\u83B7\uFF0C\u8868\u683C\u4E0D\u53D7\u5F71\u54CD\uFF09:`, err);
      }
    }
    __name(notify, "notify");
    const el = /* @__PURE__ */ __name((tag, attrs = {}, cssText = "") => {
      const node = doc.createElement(tag);
      if (cssText) node.style.cssText = cssText;
      for (const key of Object.keys(attrs)) {
        if (key === "text") node.textContent = attrs[key];
        else node.setAttribute(key, attrs[key]);
      }
      return node;
    }, "el");
    function clearChildren5(node) {
      while (node.childNodes.length > 0) node.removeChild(node.childNodes[0]);
    }
    __name(clearChildren5, "clearChildren");
    function toggleSort(key) {
      if (destroyed) return;
      sort = nextSort(sort, key);
      render();
      notify("onSort", () => {
        if (typeof options.onSort === "function") {
          options.onSort(sort === null ? null : { key: sort.key, direction: sort.direction });
        }
      });
    }
    __name(toggleSort, "toggleSort");
    function emitSelection() {
      notify("onSelect", () => {
        if (typeof options.onSelect === "function") {
          const snap = selectedSnapshot();
          options.onSelect({ keys: Object.freeze(snap.keys.slice()), rows: Object.freeze(snap.rows.slice()) });
        }
      });
    }
    __name(emitSelection, "emitSelection");
    function headerCell(column, lastColumn = false) {
      const align = normalizeAlign(column.align);
      const active = sort !== null && sort.key === column.key;
      const direction = active ? sort.direction : "none";
      const th = el(
        "th",
        {
          "data-role": "header-cell",
          "data-column-key": column.key,
          "data-sort-direction": direction,
          "aria-sort": direction === "asc" ? "ascending" : direction === "desc" ? "descending" : "none",
          scope: "col"
        },
        styleHeaderCell(align, rowHeightToken, lastColumn) + (editable ? ";user-select:none;-webkit-user-select:none" : "")
      );
      const width = normalizeWidth(column.width);
      if (width !== null) th.style.width = width;
      else if (column.width !== void 0 && column.width !== null && column.width !== "") {
        console.warn(
          `[m14 data-table] \u5217 <${column.key}> \u7684 width=<${String(column.width)}> \u4E0D\u662F\u5DF2\u767B\u8BB0\u4EE4\u724C\u7684 var() \u5F62\u6001\uFF0C\u5DF2\u5FFD\u7565\uFF08K1 \u56DE\u6D41\uFF09`
        );
      }
      const button = el(
        "button",
        {
          type: "button",
          "data-role": "sort-toggle",
          "data-column-key": column.key,
          "data-sort-direction": direction,
          title: `\u6309\u300C${column.title}\u300D\u6392\u5E8F\uFF08asc \u2192 desc \u2192 \u53D6\u6D88\uFF09`
        },
        styleSortButton(align)
      );
      button.appendChild(el("span", { "data-role": "header-title", text: column.title }));
      button.appendChild(
        el(
          "span",
          { "data-role": "sort-indicator", "data-direction": direction, text: direction === "asc" ? "\u25B2" : direction === "desc" ? "\u25BC" : "\u2195" },
          [`color:${tok2(active ? T2.primary : T2.textMuted)}`, `font-size:${tok2(T2.fontSm)}`].join(";")
        )
      );
      disposer.addEventListener(button, "click", () => toggleSort(column.key));
      th.appendChild(button);
      return th;
    }
    __name(headerCell, "headerCell");
    function editingKeyFor(row, viewIndex, columnKey) {
      if (editingKey === null || editingColumn !== columnKey) return null;
      return editingKey === rowKeyOf(row, viewIndex) ? editingKey : null;
    }
    __name(editingKeyFor, "editingKeyFor");
    function bodyCell(column, row, viewIndex, lastColumn = false) {
      const align = normalizeAlign(column.align);
      const cellStyle = styleCell(align, rowHeightToken, lastColumn) + (editable ? ";user-select:none;-webkit-user-select:none" : "");
      const td = el("td", { "data-role": "cell", "data-column-key": column.key }, cellStyle);
      const raw = row[column.key];
      if (editingKeyFor(row, viewIndex, column.key) !== null) {
        const session = currentSession();
        if (session !== null) {
          td.appendChild(editInput(session));
          return td;
        }
      }
      if (typeof column.render !== "function") {
        td.textContent = cellText(raw);
        return td;
      }
      let html;
      let threw = false;
      try {
        html = column.render(row, column);
      } catch (err) {
        threw = true;
        console.error(`[m14 data-table] \u5217 <${column.key}> \u7684 render() \u629B\u9519\uFF0C\u5DF2\u56DE\u843D\u7EAF\u6587\u672C:`, err);
      }
      if (threw || html === null || html === void 0 || html === "") {
        td.textContent = cellText(raw);
        if (threw) td.setAttribute("data-render-error", "true");
        return td;
      }
      const host = el("span", { "data-role": "cell-render", "data-column-key": column.key });
      setSafeHTML(host, String(html));
      td.setAttribute("data-rendered", "true");
      td.appendChild(host);
      return td;
    }
    __name(bodyCell, "bodyCell");
    function renderEmptyRow(tbody) {
      const tr = el("tr", { "data-role": "empty-row" }, styleRow(false));
      const td = el(
        "td",
        { "data-role": "empty", "data-empty-text": emptyText, colspan: String(Math.max(1, columns.length + (selectable ? 1 : 0))) },
        styleEmptyCell(rowHeightToken)
      );
      td.textContent = columns.length === 0 ? "\u672A\u5B9A\u4E49\u4EFB\u4F55\u5217\uFF08columns \u4E3A\u7A7A\uFF09" : emptyText;
      tr.appendChild(td);
      tbody.appendChild(tr);
    }
    __name(renderEmptyRow, "renderEmptyRow");
    function render() {
      if (destroyed) return;
      renderSeq += 1;
      clearChildren5(root);
      const isEmpty = columns.length === 0 || rows.length === 0;
      const clickable = typeof options.onRowClick === "function";
      root.setAttribute("data-role", "data-table");
      root.setAttribute("data-instance", instanceId);
      root.setAttribute("data-row-height", rowHeight);
      root.setAttribute("data-selectable", selectable ? "true" : "false");
      root.setAttribute("data-editable", editable ? "true" : "false");
      root.setAttribute("data-addable", addable ? "true" : "false");
      root.setAttribute("data-empty", isEmpty ? "true" : "false");
      root.setAttribute("data-render-seq", String(renderSeq));
      if (sort === null) {
        root.removeAttribute("data-sort-key");
        root.removeAttribute("data-sort-direction");
      } else {
        root.setAttribute("data-sort-key", sort.key);
        root.setAttribute("data-sort-direction", sort.direction);
      }
      root.style.cssText = STYLE_ROOT2;
      if (editable) {
        disposer.addEventListener(root, "dblclick", (event) => {
          const target = event.target;
          if (target === null || typeof target.closest !== "function") return;
          const cell = target.closest('[data-role="cell"]');
          if (cell === null) return;
          const columnKey = cell.getAttribute("data-column-key");
          if (columnKey === null || columnKey === "") return;
          const rowEl = cell.closest('[data-role="row"]');
          if (rowEl === null) return;
          const rowKey = rowEl.getAttribute("data-row-key");
          const at = view().findIndex((e) => e.key === rowKey);
          if (at < 0) return;
          beginEdit(at, columnKey);
        });
      }
      const table = el("table", { "data-role": "table" }, STYLE_TABLE);
      rowEls.clear();
      const thead = el("thead");
      const headRow = el("tr", { "data-role": "header-row" });
      if (selectable) {
        const th = el("th", { "data-role": "select-header", scope: "col" }, `${styleHeaderCell("center", rowHeightToken)};${STYLE_SELECT_CELL}`);
        const entries = view();
        const allSelected = entries.length > 0 && entries.every((entry) => selected.has(entry.key));
        const box = el("input", {
          type: "checkbox",
          "data-role": "select-all",
          "data-checked": allSelected ? "true" : "false",
          "aria-label": "\u5168\u9009/\u5168\u4E0D\u9009"
        });
        if (allSelected) box.setAttribute("checked", "checked");
        disposer.addEventListener(box, "change", () => {
          const list = view();
          const on = !(list.length > 0 && list.every((entry) => selected.has(entry.key)));
          for (const entry of list) {
            if (on) selected.add(entry.key);
            else selected.delete(entry.key);
          }
          render();
          emitSelection();
        });
        th.appendChild(box);
        headRow.appendChild(th);
      }
      const headerCells = [];
      for (let ci = 0; ci < columns.length; ci += 1) {
        const th = headerCell(columns[ci], ci === columns.length - 1);
        headerCells.push(th);
        headRow.appendChild(th);
      }
      thead.appendChild(headRow);
      applyColumnWidths(headerCells, columns.map((c) => c.key), colWidths);
      if (options.resizableColumns === true && headerCells.length > 1) {
        attachColumnResize({
          ths: headerCells,
          keys: columns.map((c) => c.key),
          widths: colWidths,
          minPx: columnMinWidth,
          // 不用 disposer.addEventListener（按 type:函数名 去重 ⇒ 第二次拖动会被吞）；这里真挂 + add 兜底
          register: /* @__PURE__ */ __name((target, type, fn) => {
            target.addEventListener(type, fn);
            disposer.add(() => target.removeEventListener(type, fn));
          }, "register"),
          onChange: /* @__PURE__ */ __name((widths) => notify("onColumnResize", () => options.onColumnResize(widths)), "onChange")
        });
      }
      table.appendChild(thead);
      const tbody = el("tbody");
      if (isEmpty) {
        renderEmptyRow(tbody);
      } else {
        const entries = view();
        for (const entry of entries) {
          const picked = selected.has(entry.key);
          const baseStyle = styleRow(clickable);
          const tr = el(
            "tr",
            {
              "data-role": "row",
              "data-row-key": entry.key,
              "data-row-index": String(entry.index),
              "data-selected": picked ? "true" : "false"
            },
            baseStyle
          );
          const isActive = activeKey !== null && entry.key === activeKey;
          if (isActive) {
            tr.style.cssText = baseStyle + ";" + ACTIVE_ROW_STYLE;
            tr.setAttribute("aria-current", "true");
          }
          rowEls.set(entry.key, { tr, base: baseStyle });
          if (clickable) {
            disposer.addEventListener(tr, "click", (event) => {
              const target = event.target;
              if (target !== null && typeof target.getAttribute === "function" && target.getAttribute("data-role") === "row-select") return;
              if (highlightOnClick) setActiveRow(entry.key);
              notify("onRowClick", () => options.onRowClick(entry.row, entry.index));
            });
          }
          if (selectable) {
            const td = el("td", { "data-role": "select-cell" }, STYLE_SELECT_CELL);
            const box = el("input", {
              type: "checkbox",
              "data-role": "row-select",
              "data-row-key": entry.key,
              "data-checked": picked ? "true" : "false",
              "aria-label": `\u9009\u4E2D\u7B2C ${entry.index + 1} \u884C`
            });
            if (picked) box.setAttribute("checked", "checked");
            disposer.addEventListener(box, "change", () => {
              const on = !selected.has(entry.key);
              if (on) selected.add(entry.key);
              else selected.delete(entry.key);
              tr.setAttribute("data-selected", on ? "true" : "false");
              if (on) box.setAttribute("checked", "checked");
              else box.removeAttribute("checked");
              emitSelection();
            });
            td.appendChild(box);
            tr.appendChild(td);
          }
          for (let ci = 0; ci < columns.length; ci += 1) {
            tr.appendChild(bodyCell(columns[ci], entry.row, entry.index, ci === columns.length - 1));
          }
          tbody.appendChild(tr);
        }
      }
      table.appendChild(tbody);
      root.appendChild(table);
      if (editable || addable) {
        const status = el("div", { "data-role": "edit-status", "data-state": rejectReason === null ? "idle" : "error" }, STYLE_EDIT_STATUS);
        status.setAttribute("role", "status");
        status.textContent = rejectReason === null ? "" : `\u6821\u9A8C\u672A\u901A\u8FC7\uFF1A${rejectReason}`;
        root.appendChild(status);
      }
      if (addable) {
        const remaining = editingKey === null ? "\uFF08\u5F53\u524D\u65E0\u5355\u5143\u683C\u5904\u4E8E\u7F16\u8F91\u6001\uFF09" : `\uFF08\u5F53\u524D\u7F16\u8F91\uFF1A${editingColumn}\uFF09`;
        const addButton = el("button", {
          type: "button",
          "data-role": "add-row",
          title: `\u8FFD\u52A0\u4E00\u884C\u5E76\u8FDB\u5165\u9996\u5217\u7F16\u8F91${remaining}`
        }, STYLE_ADD_BUTTON);
        addButton.textContent = "\uFF0B \u6DFB\u52A0";
        disposer.addEventListener(addButton, "click", () => {
          addRow();
        });
        root.appendChild(addButton);
      }
      if (pendingFocusInput !== null) {
        const target = pendingFocusInput;
        pendingFocusInput = null;
        focusPending = false;
        if (typeof target.focus === "function") {
          target.focus();
          if (typeof target.select === "function") target.select();
        }
      }
    }
    __name(render, "render");
    function setData(nextRows) {
      if (destroyed) {
        console.warn("[m14 data-table] destroy() \u4E4B\u540E\u8C03\u7528 setData() \u5DF2\u5FFD\u7565\uFF08\u9700\u91CD\u65B0 createDataTable\uFF09");
        return;
      }
      rows = Array.isArray(nextRows) ? nextRows.slice() : [];
      editingKey = null;
      editingColumn = null;
      draft = "";
      rejectReason = null;
      focusPending = false;
      touched.clear();
      pruneSelection();
      render();
    }
    __name(setData, "setData");
    function destroy() {
      if (destroyed) return;
      destroyed = true;
      editingKey = null;
      editingColumn = null;
      draft = "";
      rejectReason = null;
      focusPending = false;
      pendingFocusInput = null;
      disposer.dispose();
      clearChildren5(root);
      root.setAttribute("data-destroyed", "true");
    }
    __name(destroy, "destroy");
    Object.defineProperty(root, "setData", { value: setData, writable: false, enumerable: false, configurable: false });
    Object.defineProperty(root, "destroy", { value: destroy, writable: false, enumerable: false, configurable: false });
    Object.defineProperty(root, "beginEdit", { value: beginEdit, writable: false, enumerable: false, configurable: false });
    Object.defineProperty(root, "commitEdit", { value: commitEdit, writable: false, enumerable: false, configurable: false });
    Object.defineProperty(root, "cancelEdit", { value: cancelEdit, writable: false, enumerable: false, configurable: false });
    Object.defineProperty(root, "addRow", { value: addRow, writable: false, enumerable: false, configurable: false });
    Object.defineProperty(root, "rowCount", { get: /* @__PURE__ */ __name(() => rows.length, "get"), enumerable: false, configurable: false });
    Object.defineProperty(root, "columnCount", { get: /* @__PURE__ */ __name(() => columns.length, "get"), enumerable: false, configurable: false });
    Object.defineProperty(root, "selectedKeys", { get: /* @__PURE__ */ __name(() => Object.freeze(selectedSnapshot().keys), "get"), enumerable: false, configurable: false });
    Object.defineProperty(root, "sortState", {
      get: /* @__PURE__ */ __name(() => sort === null ? null : Object.freeze({ key: sort.key, direction: sort.direction }), "get"),
      enumerable: false,
      configurable: false
    });
    Object.defineProperty(root, "editState", {
      get: /* @__PURE__ */ __name(() => {
        if (editingKey === null || editingColumn === null) return null;
        const index = currentSession();
        if (index === null) return null;
        return Object.freeze({ rowIndex: index.index, columnKey: editingColumn, draft });
      }, "get"),
      enumerable: false,
      configurable: false
    });
    Object.defineProperty(root, "getRows", { value: /* @__PURE__ */ __name(() => rows.slice(), "value"), writable: false, enumerable: false, configurable: false });
    Object.defineProperty(root, "setActiveRow", { value: setActiveRow, writable: false, enumerable: false, configurable: false });
    Object.defineProperty(root, "getActiveRow", { value: /* @__PURE__ */ __name(() => activeKey, "value"), writable: false, enumerable: false, configurable: false });
    Object.defineProperty(root, "getColumnWidths", { value: /* @__PURE__ */ __name(() => Object.assign({}, colWidths), "value"), writable: false, enumerable: false, configurable: false });
    render();
    return root;
  }
  __name(createDataTable, "createDataTable");

  // src/components/tree-view.ts
  injectStyleRules([
    ".petui-tree-view-row:hover{background:var(--color-primary-light)}",
    ".petui-tree-view-row:focus-visible{outline:2px solid var(--color-primary);outline-offset:-2px}"
  ]);
  var DEFAULT_EXPAND_DEPTH = 1;
  var DEFAULT_MAX_INDENT = 8;
  var TREE_VIEW_INDENT_TOKEN = "--space-16";
  var TREE_VIEW_READONLY_REASON = "tree-view \u5904\u4E8E\u53EA\u8BFB\u6A21\u5F0F\uFF08readonly: true\uFF09\uFF1A\u5C55\u5F00/\u6536\u8D77\u88AB\u7981\u7528";
  var _TreeViewReadonlyError = class _TreeViewReadonlyError extends Error {
    constructor(action) {
      super(`${TREE_VIEW_READONLY_REASON}\uFF1B\u88AB\u62D2\u7EDD\u7684\u52A8\u4F5C=${action}`);
      this.name = "TreeViewReadonlyError";
      this.code = "TREEVIEW_READONLY";
      this.reason = TREE_VIEW_READONLY_REASON;
    }
  };
  __name(_TreeViewReadonlyError, "TreeViewReadonlyError");
  var TreeViewReadonlyError = _TreeViewReadonlyError;
  var _TreeViewDisposedError = class _TreeViewDisposedError extends Error {
    constructor(action) {
      super(`[PetUI tree-view] destroy() \u4E4B\u540E\u4E0D\u53EF\u518D\u8C03\u7528 ${action}()`);
      this.name = "TreeViewDisposedError";
      this.code = "TREEVIEW_DISPOSED";
    }
  };
  __name(_TreeViewDisposedError, "TreeViewDisposedError");
  var TreeViewDisposedError = _TreeViewDisposedError;
  var INDENT_VAR = `var(${TREE_VIEW_INDENT_TOKEN})`;
  function normalizeNodes(nodes) {
    if (!Array.isArray(nodes)) return [];
    return nodes.filter((n) => !!n && typeof n === "object");
  }
  __name(normalizeNodes, "normalizeNodes");
  function normalizeDepth(v) {
    if (typeof v !== "number" || !Number.isFinite(v) || v < 0) {
      return typeof v === "number" && v < 0 ? 0 : DEFAULT_EXPAND_DEPTH;
    }
    return Math.floor(v);
  }
  __name(normalizeDepth, "normalizeDepth");
  function normalizeMaxIndent(v) {
    if (typeof v !== "number" || !Number.isFinite(v) || v < 0) {
      return typeof v === "number" && v < 0 ? 0 : DEFAULT_MAX_INDENT;
    }
    return Math.floor(v);
  }
  __name(normalizeMaxIndent, "normalizeMaxIndent");
  function normalizeFilter(v) {
    return typeof v === "string" ? v.trim() : "";
  }
  __name(normalizeFilter, "normalizeFilter");
  function createTreeView(nodes = [], options = {}) {
    const disposer = createDisposer();
    const el = document.createElement("div");
    el.className = "petui-tree-view";
    el.setAttribute("role", "tree");
    el.style.cssText = [
      "display:flex",
      "flex-direction:column",
      "flex:1 1 auto",
      // `min-height:0` 只解除 flex 项的自动最小高度；**还要 `overflow:hidden` 才能切断
      // min-content 的向上传播**（子项内容再高也不把本组件撑出父块）。实测：只写 min-height:0
      // 时组件高度仍被行内容顶到 538px（面板里表现为"内容被推到可视区外"）。
      "min-height:0",
      "overflow:hidden",
      "padding:var(--space-2)",
      "border:var(--space-1) solid var(--color-border)",
      "border-radius:var(--border-radius-md)",
      "color:var(--color-text)",
      "font-size:var(--font-size-14)"
    ].join(";");
    const status = document.createElement("div");
    status.className = "petui-tree-view-status";
    status.setAttribute("role", "status");
    status.style.cssText = [
      "flex:0 0 auto",
      "color:var(--color-text-muted)",
      "font-size:var(--font-size-12)",
      "padding:var(--space-2) var(--space-4)"
    ].join(";");
    const list = document.createElement("div");
    list.className = "petui-tree-view-list";
    list.setAttribute("role", "group");
    list.style.cssText = "display:block;flex:1 1 auto;min-height:0;overflow:auto";
    el.appendChild(status);
    el.appendChild(list);
    let tree = normalizeNodes(nodes);
    let readOnly = options.readonly === true;
    let filterText = normalizeFilter(options.filter);
    let depthLimit = normalizeDepth(options.defaultExpandDepth);
    let indentLimit = normalizeMaxIndent(options.maxIndent);
    const expanded = /* @__PURE__ */ new Set();
    let expandedPaths = [];
    let selectedPath = null;
    let rows = [];
    let byPath = /* @__PURE__ */ new Map();
    let nodeTotal = 0;
    let disposed = false;
    const renderRowOpt = typeof options.renderRow === "function" ? options.renderRow : null;
    let renderRowWarned = false;
    function applyDefaultExpansion() {
      expanded.clear();
      const walk = /* @__PURE__ */ __name((list_, parent, depth) => {
        for (const n of normalizeNodes(list_)) {
          const path = `${parent}/${n.key}`;
          const kids = Array.isArray(n.children) ? n.children : [];
          if (kids.length > 0 && depth < depthLimit) expanded.add(path);
          walk(kids, path, depth + 1);
        }
      }, "walk");
      walk(tree, "", 0);
    }
    __name(applyDefaultExpansion, "applyDefaultExpansion");
    function countNodes(list_) {
      let n = 0;
      for (const node of normalizeNodes(list_)) {
        n += 1;
        n += countNodes(Array.isArray(node.children) ? node.children : []);
      }
      return n;
    }
    __name(countNodes, "countNodes");
    function clearChildren5(host) {
      while (host.firstChild) host.removeChild(host.firstChild);
    }
    __name(clearChildren5, "clearChildren");
    function buildRows() {
      const out = [];
      const q = filterText.toLowerCase();
      const active = q.length > 0;
      const matches = /* @__PURE__ */ __name((n) => String(n.label == null ? "" : n.label).toLowerCase().indexOf(q) >= 0, "matches");
      const retained = /* @__PURE__ */ __name((n) => {
        if (!active) return true;
        if (matches(n)) return true;
        const kids = Array.isArray(n.children) ? n.children : [];
        return kids.some((k) => retained(k));
      }, "retained");
      const walk = /* @__PURE__ */ __name((list_, parent, depth) => {
        for (const n of normalizeNodes(list_)) {
          if (!retained(n)) continue;
          const path = `${parent}/${n.key}`;
          const kids = Array.isArray(n.children) ? n.children : [];
          const hasChildren = kids.length > 0;
          const isOpen = active ? hasChildren : expanded.has(path);
          out.push({
            path,
            label: String(n.label == null ? "" : n.label),
            depth,
            hasChildren,
            expanded: isOpen,
            matched: active && matches(n),
            node: n
          });
          if (hasChildren && isOpen) walk(kids, path, depth + 1);
        }
      }, "walk");
      walk(tree, "", 0);
      return out;
    }
    __name(buildRows, "buildRows");
    function render() {
      clearChildren5(list);
      rows = buildRows();
      byPath = /* @__PURE__ */ new Map();
      nodeTotal = countNodes(tree);
      if (rows.length === 0) {
        const empty = document.createElement("div");
        empty.className = "petui-tree-view-empty";
        empty.style.cssText = "color:var(--color-text-muted);padding:var(--space-4)";
        empty.textContent = filterText ? "\u65E0\u5339\u914D\u8282\u70B9" : "\uFF08\u7A7A\u6811\uFF09";
        list.appendChild(empty);
      }
      for (const row of rows) {
        byPath.set(row.path, row);
        const rowEl = document.createElement("div");
        rowEl.className = "petui-tree-view-row";
        rowEl.setAttribute("role", "treeitem");
        rowEl.setAttribute("aria-level", String(row.depth + 1));
        rowEl.setAttribute("data-path", row.path);
        rowEl.setAttribute("tabindex", "0");
        if (row.hasChildren) rowEl.setAttribute("aria-expanded", row.expanded ? "true" : "false");
        if (readOnly) rowEl.setAttribute("aria-disabled", "true");
        const isSelected = selectedPath === row.path;
        if (isSelected) rowEl.setAttribute("aria-selected", "true");
        const rowRules = [
          "display:flex",
          "align-items:center",
          "gap:var(--space-4)",
          "padding-top:var(--space-2)",
          "padding-bottom:var(--space-2)",
          "padding-right:var(--space-4)",
          "border-radius:var(--border-radius-sm)",
          "color:" + (readOnly ? "var(--color-text-muted)" : "var(--color-text)"),
          "cursor:" + (readOnly ? "default" : "pointer")
        ];
        if (isSelected) rowRules.push("background:var(--color-alpha-primary-40)");
        rowEl.style.cssText = rowRules.join(";");
        const level = Math.min(row.depth, indentLimit);
        rowEl.style.paddingLeft = `calc(${INDENT_VAR} * ${level})`;
        const toggle = document.createElement("span");
        toggle.className = "petui-tree-view-toggle";
        toggle.style.cssText = [
          "flex:0 0 auto",
          "width:var(--space-12)",
          "text-align:center",
          "color:var(--color-text-secondary)"
        ].join(";");
        toggle.textContent = row.hasChildren ? row.expanded ? "\u25BE" : "\u25B8" : "\xB7";
        rowEl.appendChild(toggle);
        const labelEl = document.createElement("span");
        labelEl.className = "petui-tree-view-label";
        labelEl.textContent = row.label;
        labelEl.style.cssText = row.matched ? "color:var(--color-primary)" : "color:inherit";
        rowEl.appendChild(labelEl);
        if (renderRowOpt !== null) {
          try {
            renderRowOpt(row, labelEl, rowEl);
          } catch (err) {
            if (!renderRowWarned) {
              renderRowWarned = true;
              console.warn("[PetUI tree-view] renderRow \u94A9\u5B50\u629B\u9519\uFF0C\u5DF2\u9694\u79BB\u5E76\u9000\u56DE\u7EAF\u6587\u672C\u884C\uFF08\u53EA\u62A5\u4E00\u6B21\uFF09:", err);
            }
          }
        }
        list.appendChild(rowEl);
      }
      expandedPaths = Array.from(expanded).sort();
      status.textContent = readOnly ? TREE_VIEW_READONLY_REASON : filterText ? `\u8FC7\u6EE4\u300C${filterText}\u300D\uFF1A\u4FDD\u7559\u547D\u4E2D\u9879\u53CA\u5176\u7956\u5148\u94FE\uFF0C\u5171 ${rows.length} \u884C` : "";
    }
    __name(render, "render");
    function rowFromEvent(ev) {
      let n = ev && ev.target || null;
      while (n && n !== list) {
        const asEl = n;
        if (typeof asEl.getAttribute === "function" && asEl.getAttribute("data-path") !== null) return asEl;
        n = n.parentNode;
      }
      return null;
    }
    __name(rowFromEvent, "rowFromEvent");
    function rejectReadonly(action) {
      const err = new TreeViewReadonlyError(action);
      console.warn(err.message);
      status.textContent = err.reason;
    }
    __name(rejectReadonly, "rejectReadonly");
    function activate(path, action) {
      const row = byPath.get(path);
      if (!row) return;
      if (readOnly) {
        rejectReadonly(action);
        return;
      }
      selectedPath = path;
      if (typeof options.onSelect === "function") {
        try {
          options.onSelect(path);
        } catch (err) {
          console.error("[m15 tree-view] onSelect \u56DE\u8C03\u629B\u9519\uFF08\u5DF2\u6355\u83B7\uFF0C\u6811\u72B6\u6001\u4E0D\u53D7\u5F71\u54CD\uFF09:", err);
        }
      }
      if (row.hasChildren) {
        if (expanded.has(path)) expanded.delete(path);
        else expanded.add(path);
      }
      render();
      status.textContent = "\u5DF2\u9009\u4E2D\uFF1A" + path;
    }
    __name(activate, "activate");
    function onClick(ev) {
      const row = rowFromEvent(ev);
      if (!row) return;
      activate(String(row.getAttribute("data-path") || ""), "click");
    }
    __name(onClick, "onClick");
    function onKeydown(ev) {
      const ke = ev;
      if (ke.key !== "Enter" && ke.key !== " " && ke.key !== "Spacebar") return;
      const row = rowFromEvent(ev);
      if (!row) return;
      if (typeof ke.preventDefault === "function") ke.preventDefault();
      activate(String(row.getAttribute("data-path") || ""), "keydown");
    }
    __name(onKeydown, "onKeydown");
    disposer.addEventListener(list, "click", onClick);
    disposer.addEventListener(list, "keydown", onKeydown);
    function assertAlive(action) {
      if (disposed) throw new TreeViewDisposedError(action);
    }
    __name(assertAlive, "assertAlive");
    function requireMutable(action) {
      assertAlive(action);
      if (readOnly) throw new TreeViewReadonlyError(action);
    }
    __name(requireMutable, "requireMutable");
    function setNodes(next) {
      assertAlive("setNodes");
      tree = normalizeNodes(next);
      applyDefaultExpansion();
      render();
    }
    __name(setNodes, "setNodes");
    function expandAll() {
      requireMutable("expandAll");
      const add = /* @__PURE__ */ __name((list_, parent) => {
        for (const n of normalizeNodes(list_)) {
          const path = `${parent}/${n.key}`;
          const kids = Array.isArray(n.children) ? n.children : [];
          if (kids.length > 0) expanded.add(path);
          add(kids, path);
        }
      }, "add");
      add(tree, "");
      render();
    }
    __name(expandAll, "expandAll");
    function collapseAll() {
      requireMutable("collapseAll");
      expanded.clear();
      render();
    }
    __name(collapseAll, "collapseAll");
    function destroy() {
      if (disposed) return;
      disposed = true;
      byPath.clear();
      rows = [];
      expandedPaths = [];
      try {
        disposer.dispose();
      } finally {
        if (el.parentNode) el.parentNode.removeChild(el);
      }
    }
    __name(destroy, "destroy");
    applyDefaultExpansion();
    render();
    Object.defineProperty(el, "visibleCount", { get: /* @__PURE__ */ __name(() => rows.length, "get"), enumerable: true, configurable: true });
    Object.defineProperty(el, "nodeCount", { get: /* @__PURE__ */ __name(() => nodeTotal, "get"), enumerable: true, configurable: true });
    Object.defineProperty(el, "readonly", { get: /* @__PURE__ */ __name(() => readOnly, "get"), enumerable: true, configurable: true });
    Object.defineProperty(el, "filter", { get: /* @__PURE__ */ __name(() => filterText, "get"), enumerable: true, configurable: true });
    Object.defineProperty(el, "expandedKeys", {
      get: /* @__PURE__ */ __name(() => Object.freeze(expandedPaths.slice()), "get"),
      enumerable: true,
      configurable: true
    });
    Object.defineProperty(el, "selectedPath", {
      get: /* @__PURE__ */ __name(() => selectedPath, "get"),
      enumerable: true,
      configurable: true
    });
    Object.defineProperty(el, "maxIndent", { get: /* @__PURE__ */ __name(() => indentLimit, "get"), enumerable: true, configurable: true });
    Object.defineProperty(el, "defaultExpandDepth", {
      get: /* @__PURE__ */ __name(() => depthLimit, "get"),
      enumerable: true,
      configurable: true
    });
    Object.defineProperty(el, "disposed", { get: /* @__PURE__ */ __name(() => disposed, "get"), enumerable: true, configurable: true });
    const api = Object.assign(el, { setNodes, expandAll, collapseAll, destroy });
    return api;
  }
  __name(createTreeView, "createTreeView");

  // src/components/description-list.ts
  var DEFAULT_MIN_TRACK = "var(--space-200)";
  var PX_ANCHOR_TOKEN = "var(--space-1)";
  var ROOT_CLASS = "petui-description-list";
  var ITEM_CLASS = "petui-description-list__item";
  var LABEL_CLASS = "petui-description-list__label";
  var VALUE_CLASS = "petui-description-list__value";
  function toMinTrack(minColumnWidth) {
    if (typeof minColumnWidth === "number" && Number.isFinite(minColumnWidth) && minColumnWidth > 0) {
      const n = Math.max(1, Math.round(minColumnWidth));
      return "calc(" + PX_ANCHOR_TOKEN + " * " + n + ")";
    }
    return DEFAULT_MIN_TRACK;
  }
  __name(toMinTrack, "toMinTrack");
  function normalizeSpan(span) {
    if (typeof span !== "number" || !Number.isFinite(span)) return 1;
    const n = Math.floor(span);
    return n > 1 ? n : 1;
  }
  __name(normalizeSpan, "normalizeSpan");
  function normalizeItems(list) {
    const out = [];
    if (!Array.isArray(list)) return out;
    for (let i = 0; i < list.length; i++) {
      const raw = list[i];
      if (raw === null || raw === void 0 || typeof raw !== "object") continue;
      const item = {
        label: raw.label === null || raw.label === void 0 ? "" : String(raw.label),
        value: raw.value
      };
      const span = normalizeSpan(raw.span);
      if (span > 1) item.span = span;
      out.push(item);
    }
    return out;
  }
  __name(normalizeItems, "normalizeItems");
  function applyValue(target, value) {
    if (value === null || value === void 0) {
      target.textContent = "";
      return;
    }
    if (typeof value === "string") {
      target.textContent = value;
      return;
    }
    if (typeof value === "number" || typeof value === "boolean" || typeof value === "bigint") {
      target.textContent = String(value);
      return;
    }
    if (typeof value.nodeType === "number") {
      target.appendChild(value);
      return;
    }
    const html = value.html;
    if (typeof html === "string") {
      setSafeHTML(target, html);
      return;
    }
    target.textContent = String(value);
  }
  __name(applyValue, "applyValue");
  function buildRow(item, index) {
    const row = document.createElement("div");
    row.className = ITEM_CLASS;
    row.setAttribute("data-index", String(index));
    const span = normalizeSpan(item.span);
    row.setAttribute("data-span", String(span));
    row.style.cssText = [
      "display:flex",
      "flex-direction:column",
      "gap:var(--space-2)",
      "min-width:0",
      "padding:var(--space-8) var(--space-12)",
      "background:var(--color-bg-light)",
      "border:var(--space-1) solid var(--color-border)",
      "border-radius:var(--border-radius-sm)"
    ].join(";");
    if (span > 1) row.style.gridColumn = "span " + span;
    const dt = document.createElement("dt");
    dt.className = LABEL_CLASS;
    dt.textContent = item.label;
    dt.style.cssText = "margin:0;font-size:var(--font-size-12);color:var(--color-text-muted)";
    const dd = document.createElement("dd");
    dd.className = VALUE_CLASS;
    dd.style.cssText = "margin:0;font-size:var(--font-size-14);color:var(--color-text);word-break:break-word";
    applyValue(dd, item.value);
    row.appendChild(dt);
    row.appendChild(dd);
    return row;
  }
  __name(buildRow, "buildRow");
  function createDescriptionList(itemsOrOptions = [], options = {}) {
    const opts = Array.isArray(itemsOrOptions) ? options : itemsOrOptions;
    const initialItems = Array.isArray(itemsOrOptions) ? itemsOrOptions : opts.items;
    let currentItems = normalizeItems(initialItems);
    const currentMinColumnWidth = typeof opts.minColumnWidth === "number" && Number.isFinite(opts.minColumnWidth) && opts.minColumnWidth > 0 ? opts.minColumnWidth : void 0;
    const el = document.createElement("dl");
    el.className = ROOT_CLASS;
    el.style.cssText = [
      "display:grid",
      "grid-template-columns:repeat(auto-fit, minmax(" + toMinTrack(currentMinColumnWidth) + ", 1fr))",
      "gap:var(--space-12) var(--space-16)",
      "margin:0",
      "padding:0",
      "align-items:start",
      "font-size:var(--font-size-14)",
      "color:var(--color-text)"
    ].join(";");
    const disposer = createDisposer();
    disposer.add(() => {
      const anyEl = el;
      if (typeof anyEl.remove === "function") {
        anyEl.remove();
      } else if (anyEl.parentNode && typeof anyEl.parentNode.removeChild === "function") {
        anyEl.parentNode.removeChild(el);
      }
    });
    const render = /* @__PURE__ */ __name(() => {
      const rows = [];
      for (let i = 0; i < currentItems.length; i++) rows.push(buildRow(currentItems[i], i));
      el.replaceChildren(...rows);
      el.setAttribute("data-count", String(rows.length));
      el.setAttribute("data-empty", rows.length === 0 ? "true" : "false");
    }, "render");
    render();
    Object.defineProperties(el, {
      setItems: {
        value: /* @__PURE__ */ __name((next) => {
          currentItems = normalizeItems(next);
          render();
        }, "value"),
        writable: true,
        configurable: true
      },
      destroy: {
        value: /* @__PURE__ */ __name(() => {
          disposer.dispose();
        }, "value"),
        writable: true,
        configurable: true
      },
      items: {
        get: /* @__PURE__ */ __name(() => currentItems.map((it) => {
          const copy = { label: it.label, value: it.value };
          if (it.span !== void 0) copy.span = it.span;
          return copy;
        }), "get"),
        configurable: true
      },
      minColumnWidth: {
        get: /* @__PURE__ */ __name(() => currentMinColumnWidth, "get"),
        configurable: true
      }
    });
    return el;
  }
  __name(createDescriptionList, "createDescriptionList");

  // src/components/virtual-list.ts
  var _VirtualListConfigError = class _VirtualListConfigError extends TypeError {
    constructor(message) {
      super("[PetUI virtual-list] " + message);
      this.name = "VirtualListConfigError";
    }
  };
  __name(_VirtualListConfigError, "VirtualListConfigError");
  var VirtualListConfigError = _VirtualListConfigError;
  var _VirtualListDisposedError = class _VirtualListDisposedError extends Error {
    constructor() {
      super("[PetUI virtual-list] \u5B9E\u4F8B\u5DF2 destroy()\uFF0C\u4E0D\u80FD\u518D\u8C03\u7528 refresh/scrollToIndex/appendRows/failPending");
      this.name = "VirtualListDisposedError";
    }
  };
  __name(_VirtualListDisposedError, "VirtualListDisposedError");
  var VirtualListDisposedError = _VirtualListDisposedError;
  var DEFAULT_OVERSCAN = 2;
  var DEFAULT_PAGE_SIZE = 20;
  var MAX_RENDER_PASSES = 8;
  var ERROR_PREFIX = "[PetUI virtual-list]";
  function tokVar5(name) {
    return "var(" + name + ")";
  }
  __name(tokVar5, "tokVar");
  function px2(n) {
    return n + "px";
  }
  __name(px2, "px");
  function resolveDocument2() {
    const d = globalThis.document;
    if (d === void 0 || typeof d.createElement !== "function") {
      throw new VirtualListConfigError("\u73AF\u5883\u7F3A\u5C11 document\uFF0C\u65E0\u6CD5\u521B\u5EFA\u865A\u62DF\u5217\u8868");
    }
    return d;
  }
  __name(resolveDocument2, "resolveDocument");
  function isRowSequence(value) {
    return value !== null && typeof value === "object" && typeof value.length === "number";
  }
  __name(isRowSequence, "isRowSequence");
  function assertNonNegativeInt(value, what) {
    if (typeof value !== "number" || !Number.isFinite(value) || value < 0 || Math.floor(value) !== value) {
      throw new VirtualListConfigError(what + " \u5FC5\u987B\u662F\u975E\u8D1F\u6574\u6570\uFF0C\u6536\u5230: " + String(value));
    }
    return value;
  }
  __name(assertNonNegativeInt, "assertNonNegativeInt");
  function assertPositiveNumber(value, what) {
    if (typeof value !== "number" || !Number.isFinite(value) || value <= 0) {
      throw new VirtualListConfigError(what + " \u5FC5\u987B\u662F > 0 \u7684\u6709\u9650\u6570\u503C\uFF0C\u6536\u5230: " + String(value));
    }
    return value;
  }
  __name(assertPositiveNumber, "assertPositiveNumber");
  function createVirtualList(options) {
    if (options === null || typeof options !== "object") {
      throw new VirtualListConfigError("createVirtualList \u9700\u8981\u4E00\u4E2A\u914D\u7F6E\u5BF9\u8C61");
    }
    if (!isRowSequence(options.items)) {
      throw new VirtualListConfigError("items \u5FC5\u987B\u662F\u6570\u7EC4\uFF08\u6216\u5E26 length \u7684\u7C7B\u6570\u7EC4\uFF09");
    }
    const rowHeight = assertPositiveNumber(options.rowHeight, "rowHeight");
    const overscan = options.overscan === void 0 ? DEFAULT_OVERSCAN : assertNonNegativeInt(options.overscan, "overscan");
    const pageSize = options.pageSize === void 0 ? DEFAULT_PAGE_SIZE : assertNonNegativeInt(options.pageSize, "pageSize");
    if (pageSize < 1) throw new VirtualListConfigError("pageSize \u5FC5\u987B >= 1");
    if (typeof options.renderRow !== "function") {
      throw new VirtualListConfigError("renderRow \u5FC5\u586B\u4E14\u5FC5\u987B\u662F\u51FD\u6570");
    }
    if (typeof options.keyOf !== "function") {
      throw new VirtualListConfigError("keyOf \u5FC5\u586B\u4E14\u5FC5\u987B\u662F\u51FD\u6570");
    }
    if (options.onNeedRows !== void 0 && typeof options.onNeedRows !== "function") {
      throw new VirtualListConfigError("onNeedRows \u5FC5\u987B\u662F\u51FD\u6570");
    }
    if (options.onRendered !== void 0 && typeof options.onRendered !== "function") {
      throw new VirtualListConfigError("onRendered \u5FC5\u987B\u662F\u51FD\u6570");
    }
    const renderRow = options.renderRow;
    const keyOf = options.keyOf;
    const onNeedRows = options.onNeedRows;
    const onRendered = options.onRendered;
    const rows = [];
    rows.length = options.items.length;
    for (let i = 0; i < options.items.length; i += 1) {
      if (i in options.items) rows[i] = options.items[i];
    }
    let disposed = false;
    let rendering = false;
    let dirty = false;
    let requestErrors = 0;
    let keyErrors = 0;
    const requested = /* @__PURE__ */ new Set();
    const failed = /* @__PURE__ */ new Set();
    const cache2 = /* @__PURE__ */ new Map();
    const disposer = createDisposer();
    const doc = resolveDocument2();
    const root = doc.createElement("div");
    root.className = "petui-virtual-list";
    root.style.width = "100%";
    root.setAttribute("role", "list");
    root.style.height = tokVar5("--space-300");
    root.style.overflowY = "auto";
    root.style.position = "relative";
    root.style.boxSizing = "border-box";
    root.style.background = tokVar5("--color-bg");
    root.style.color = tokVar5("--color-text");
    root.style.border = "solid " + tokVar5("--space-1") + " " + tokVar5("--color-border");
    root.style.borderRadius = tokVar5("--border-radius-sm");
    const spacer = doc.createElement("div");
    spacer.className = "petui-virtual-list-spacer";
    spacer.style.position = "relative";
    spacer.style.height = px2(0);
    root.appendChild(spacer);
    const api = {
      get renderedCount() {
        return cache2.size;
      },
      get totalCount() {
        return rows.length;
      },
      get pendingCount() {
        let n = 0;
        for (let i = 0; i < rows.length; i += 1) {
          if (!(i in rows) && !failed.has(i)) n += 1;
        }
        return n;
      },
      get failedCount() {
        return failed.size;
      },
      get requestErrors() {
        return requestErrors;
      },
      get keyErrors() {
        return keyErrors;
      },
      get disposed() {
        return disposed;
      },
      refresh() {
        if (disposed) throw new VirtualListDisposedError();
        render("refresh");
      },
      scrollToIndex(index) {
        if (disposed) throw new VirtualListDisposedError();
        if (typeof index !== "number" || !Number.isFinite(index)) {
          throw new VirtualListConfigError("scrollToIndex \u9700\u8981\u4E00\u4E2A\u6709\u9650\u6570\u503C\uFF0C\u6536\u5230: " + String(index));
        }
        const total = rows.length;
        if (total === 0) {
          render("scroll");
          return;
        }
        const target = Math.max(0, Math.min(total - 1, Math.trunc(index)));
        root.scrollTop = target * rowHeight;
        render("scroll");
      },
      appendRows(nextRows, startIndex) {
        if (disposed) throw new VirtualListDisposedError();
        if (!isRowSequence(nextRows)) {
          throw new VirtualListConfigError("appendRows \u9700\u8981\u4E00\u4E2A\u6570\u7EC4\uFF08\u6216\u5E26 length \u7684\u7C7B\u6570\u7EC4\uFF09");
        }
        const start = startIndex === void 0 ? rows.length : assertNonNegativeInt(startIndex, "appendRows \u7684 startIndex");
        for (let k = 0; k < nextRows.length; k += 1) {
          const at = start + k;
          rows[at] = nextRows[k];
          failed.delete(at);
          requested.delete(at);
        }
        render("append");
      },
      failPending() {
        if (disposed) throw new VirtualListDisposedError();
        let marked = 0;
        const targets = [];
        requested.forEach((i) => {
          if (!(i in rows) && !failed.has(i)) {
            failed.add(i);
            targets.push(i);
            marked += 1;
          }
        });
        if (marked > 0) {
          requested.clear();
          render("fail");
        }
        return marked;
      },
      destroy() {
        if (disposed) return;
        disposed = true;
        try {
          disposer.dispose();
        } catch (err) {
          console.error(ERROR_PREFIX + " destroy() \u671F\u95F4\u91CA\u653E\u52A8\u4F5C\u629B\u9519:", err);
        }
        cache2.forEach((entry) => removeChildSafe(spacer, entry.wrapper));
        cache2.clear();
        failed.clear();
        requested.clear();
        removeChildSafe(root, spacer);
        if (root.parentNode !== null) removeChildSafe(root.parentNode, root);
      }
    };
    const GETTERS = [
      "renderedCount",
      "totalCount",
      "pendingCount",
      "failedCount",
      "requestErrors",
      "keyErrors",
      "disposed"
    ];
    for (const name of GETTERS) {
      Object.defineProperty(root, name, {
        get: /* @__PURE__ */ __name(() => api[name], "get"),
        enumerable: true,
        configurable: true
      });
    }
    const METHODS = [
      "refresh",
      "scrollToIndex",
      "appendRows",
      "failPending",
      "destroy"
    ];
    for (const name of METHODS) {
      Object.defineProperty(root, name, {
        value: api[name].bind(api),
        writable: true,
        enumerable: true,
        configurable: true
      });
    }
    function viewportRows() {
      const h2 = typeof root.clientHeight === "number" ? root.clientHeight : 0;
      if (h2 > 0) return Math.max(1, Math.ceil(h2 / rowHeight));
      return pageSize;
    }
    __name(viewportRows, "viewportRows");
    function scrollOffset() {
      const top = typeof root.scrollTop === "number" ? root.scrollTop : 0;
      return top > 0 ? top : 0;
    }
    __name(scrollOffset, "scrollOffset");
    function computeWindow() {
      const total = rows.length;
      if (total === 0) return { start: 0, end: 0 };
      const first = Math.floor(scrollOffset() / rowHeight);
      const start = Math.max(0, Math.min(total, first - overscan));
      const end = Math.max(start, Math.min(total, first + viewportRows() + overscan));
      return { start, end };
    }
    __name(computeWindow, "computeWindow");
    function placeWrapper(wrapper, index) {
      wrapper.style.position = "absolute";
      wrapper.style.left = "0";
      wrapper.style.right = "0";
      wrapper.style.top = px2(index * rowHeight);
      wrapper.style.height = px2(rowHeight);
      wrapper.style.overflow = "hidden";
    }
    __name(placeWrapper, "placeWrapper");
    function newWrapper(index, state) {
      const wrapper = doc.createElement("div");
      wrapper.className = "petui-virtual-list-row";
      wrapper.setAttribute("role", "listitem");
      wrapper.setAttribute("data-index", String(index));
      wrapper.setAttribute("data-state", state);
      placeWrapper(wrapper, index);
      return wrapper;
    }
    __name(newWrapper, "newWrapper");
    function buildPendingRow(index) {
      const wrapper = newWrapper(index, "pending");
      wrapper.style.display = "flex";
      wrapper.style.alignItems = "center";
      wrapper.style.paddingLeft = tokVar5("--space-8");
      wrapper.style.color = tokVar5("--color-text-muted");
      wrapper.style.fontSize = tokVar5("--font-size-12");
      const text = doc.createElement("span");
      text.className = "petui-virtual-list-pending-text";
      text.textContent = "\u6B63\u5728\u53D6\u56DE\u7B2C " + index + " \u884C\u2026";
      wrapper.appendChild(text);
      return wrapper;
    }
    __name(buildPendingRow, "buildPendingRow");
    function buildFailedRow(index) {
      const wrapper = newWrapper(index, "failed");
      wrapper.style.display = "flex";
      wrapper.style.alignItems = "center";
      wrapper.style.gap = tokVar5("--space-4");
      wrapper.style.paddingLeft = tokVar5("--space-8");
      wrapper.style.color = tokVar5("--color-danger");
      wrapper.style.fontSize = tokVar5("--font-size-13");
      wrapper.style.background = tokVar5("--color-bg-light");
      wrapper.style.border = "solid " + tokVar5("--space-1") + " " + tokVar5("--color-danger");
      const text = doc.createElement("span");
      text.className = "petui-virtual-list-failed-text";
      text.textContent = "\u7B2C " + index + " \u884C\u53D6\u56DE\u5931\u8D25";
      wrapper.appendChild(text);
      const retry = doc.createElement("button");
      retry.className = "petui-virtual-list-retry";
      retry.setAttribute("type", "button");
      retry.setAttribute("data-action", "retry");
      retry.setAttribute("data-index", String(index));
      retry.setAttribute("aria-label", "\u91CD\u8BD5\u53D6\u56DE\u7B2C " + index + " \u884C");
      retry.textContent = "\u91CD\u8BD5";
      retry.style.color = tokVar5("--color-primary");
      retry.style.background = tokVar5("--color-bg");
      retry.style.border = "solid " + tokVar5("--space-1") + " " + tokVar5("--color-primary");
      retry.style.borderRadius = tokVar5("--border-radius-sm");
      retry.style.fontSize = tokVar5("--font-size-12");
      retry.style.padding = tokVar5("--space-2") + " " + tokVar5("--space-8");
      wrapper.appendChild(retry);
      return wrapper;
    }
    __name(buildFailedRow, "buildFailedRow");
    function buildReadyRow(index, item, key) {
      let content = null;
      try {
        content = renderRow(item, index);
      } catch (err) {
        requestErrors += 1;
        console.error(ERROR_PREFIX + " renderRow \u629B\u9519\uFF08index=" + index + "\uFF09:", err);
      }
      const wrapper = newWrapper(index, "ready");
      wrapper.setAttribute("data-key", key);
      if (content === null || content === void 0 || typeof content.nodeType !== "number") {
        requestErrors += 1;
        console.error(ERROR_PREFIX + " renderRow \u672A\u8FD4\u56DE\u5143\u7D20\uFF08index=" + index + "\uFF09\uFF0C\u672C\u884C\u9000\u5316\u4E3A\u5177\u540D\u5360\u4F4D");
        const fallback = doc.createElement("span");
        fallback.className = "petui-virtual-list-fallback";
        fallback.textContent = "\u7B2C " + index + " \u884C\u6E32\u67D3\u5668\u672A\u8FD4\u56DE\u5143\u7D20";
        wrapper.appendChild(fallback);
        return wrapper;
      }
      wrapper.appendChild(content);
      return wrapper;
    }
    __name(buildReadyRow, "buildReadyRow");
    function replaceEntry(key, index, state, next) {
      const previous = cache2.get(key);
      if (previous !== void 0) removeChildSafe(spacer, previous.wrapper);
      const entry = { key, index, state, wrapper: next };
      cache2.set(key, entry);
      spacer.appendChild(next);
      return entry;
    }
    __name(replaceEntry, "replaceEntry");
    function callOnNeedRows(start, end) {
      if (onNeedRows === void 0) return;
      try {
        onNeedRows(start, end);
      } catch (err) {
        requestErrors += 1;
        console.error(ERROR_PREFIX + " onNeedRows(" + start + "," + end + ") \u629B\u9519:", err);
      }
    }
    __name(callOnNeedRows, "callOnNeedRows");
    function requestMissing(start, end) {
      if (onNeedRows === void 0) return;
      const total = rows.length;
      if (total === 0) return;
      const pageStart = Math.max(0, Math.floor(start / pageSize) * pageSize);
      const pageEnd = Math.min(total, Math.ceil(end / pageSize) * pageSize);
      let runStart = -1;
      for (let i = pageStart; i <= pageEnd; i += 1) {
        const missing = i < pageEnd && !(i in rows) && !requested.has(i) && !failed.has(i);
        if (missing) {
          if (runStart < 0) runStart = i;
        } else if (runStart >= 0) {
          for (let j = runStart; j < i; j += 1) requested.add(j);
          callOnNeedRows(runStart, i);
          runStart = -1;
        }
      }
    }
    __name(requestMissing, "requestMissing");
    function renderOnce(reason) {
      const total = rows.length;
      spacer.style.height = px2(total * rowHeight);
      const win = computeWindow();
      const active = /* @__PURE__ */ new Set();
      for (let i = win.start; i < win.end; i += 1) {
        const isPending = !(i in rows) && !failed.has(i);
        const isFailed = failed.has(i);
        const state = isFailed ? "failed" : isPending ? "pending" : "ready";
        if (state === "pending") {
          const key2 = "#pending:" + i;
          active.add(key2);
          const hit2 = cache2.get(key2);
          if (hit2 === void 0 || hit2.index !== i || hit2.state !== "pending") {
            replaceEntry(key2, i, "pending", buildPendingRow(i));
          } else {
            placeWrapper(hit2.wrapper, i);
          }
          continue;
        }
        if (state === "failed") {
          const key2 = "#failed:" + i;
          active.add(key2);
          const hit2 = cache2.get(key2);
          if (hit2 === void 0 || hit2.index !== i || hit2.state !== "failed") {
            replaceEntry(key2, i, "failed", buildFailedRow(i));
          } else {
            placeWrapper(hit2.wrapper, i);
          }
          continue;
        }
        const item = rows[i];
        let key;
        try {
          key = String(keyOf(item, i));
        } catch (err) {
          keyErrors += 1;
          console.error(ERROR_PREFIX + " keyOf \u629B\u9519\uFF08index=" + i + "\uFF09\uFF0C\u672C\u884C\u9000\u5316\u4E3A\u4F4D\u7F6E key:", err);
          key = "#index:" + i;
        }
        active.add(key);
        const hit = cache2.get(key);
        if (hit !== void 0 && hit.state === "ready") {
          hit.index = i;
          hit.wrapper.setAttribute("data-index", String(i));
          placeWrapper(hit.wrapper, i);
          continue;
        }
        const wrapper = buildReadyRow(i, item, key);
        if (key.indexOf("#index:") === 0) wrapper.setAttribute("data-key-error", "1");
        replaceEntry(key, i, "ready", wrapper);
      }
      const stale = [];
      cache2.forEach((entry, key) => {
        if (entry.index < win.start || entry.index >= win.end || !active.has(key)) stale.push(key);
      });
      for (const key of stale) {
        const entry = cache2.get(key);
        if (entry !== void 0) removeChildSafe(spacer, entry.wrapper);
        cache2.delete(key);
      }
      requestMissing(win.start, win.end);
      if (onRendered !== void 0) {
        try {
          onRendered({
            start: win.start,
            end: win.end,
            rendered: cache2.size,
            total: rows.length,
            reason
          });
        } catch (err) {
          requestErrors += 1;
          console.error(ERROR_PREFIX + " onRendered \u629B\u9519:", err);
        }
      }
    }
    __name(renderOnce, "renderOnce");
    function render(reason) {
      if (rendering) {
        dirty = true;
        return;
      }
      rendering = true;
      try {
        let pass = 0;
        let current = reason;
        dirty = false;
        do {
          dirty = false;
          renderOnce(current);
          current = "append";
          pass += 1;
        } while (dirty && pass < MAX_RENDER_PASSES);
        if (dirty) {
          requestErrors += 1;
          console.warn(ERROR_PREFIX + " \u5355\u6B21\u8C03\u7528\u5185\u91CD\u6E32\u67D3\u8D85\u8FC7 " + MAX_RENDER_PASSES + " \u8F6E\uFF0C\u5DF2\u505C\u6B62\u7EE7\u7EED\u8865\u6570");
        }
      } finally {
        rendering = false;
      }
    }
    __name(render, "render");
    disposer.addEventListener(root, "scroll", () => {
      if (disposed) return;
      render("scroll");
    });
    disposer.addEventListener(root, "click", (ev) => {
      if (disposed) return;
      const button = retryButtonFrom(ev.target);
      if (button === null) return;
      const raw = button.getAttribute("data-index");
      const index = raw === null ? NaN : Number(raw);
      if (!Number.isFinite(index)) {
        requestErrors += 1;
        console.error(ERROR_PREFIX + " \u91CD\u8BD5\u6309\u94AE\u7F3A\u5C11\u53EF\u89E3\u6790\u7684 data-index\uFF0C\u5DF2\u5FFD\u7565\u672C\u6B21\u70B9\u51FB");
        return;
      }
      failed.delete(index);
      requested.delete(index);
      render("retry");
    });
    function retryButtonFrom(target) {
      let node = target;
      while (node !== null && node !== void 0 && node !== root) {
        const get = typeof node.getAttribute === "function" ? node.getAttribute.bind(node) : null;
        if (get !== null && get("data-action") === "retry") return node;
        node = node.parentNode;
      }
      return null;
    }
    __name(retryButtonFrom, "retryButtonFrom");
    render("initial");
    return root;
  }
  __name(createVirtualList, "createVirtualList");
  function removeChildSafe(parent, child) {
    if (child.parentNode !== parent) return;
    parent.removeChild(child);
  }
  __name(removeChildSafe, "removeChildSafe");

  // src/components/selectable-list.ts
  var _SelectableListConfigError = class _SelectableListConfigError extends TypeError {
    constructor(reason, message, escapeAction) {
      super(`[PetUI selectable-list] ${message}\uFF08reason=${reason}\uFF1B\u51FA\u8DEF=${escapeAction}\uFF09`);
      this.name = "SelectableListConfigError";
      this.reason = reason;
      this.escapeAction = escapeAction;
    }
  };
  __name(_SelectableListConfigError, "SelectableListConfigError");
  var SelectableListConfigError = _SelectableListConfigError;
  var _DuplicateKeyError = class _DuplicateKeyError extends Error {
    constructor(key) {
      super(`[PetUI selectable-list] \u91CD\u590D key "${key}"\uFF1Akey \u662F\u884C\u8EAB\u4EFD\uFF0C\u5FC5\u987B\u552F\u4E00\uFF08\u51FA\u8DEF=\u8BA9 key \u552F\u4E00\uFF09`);
      this.name = "DuplicateKeyError";
      this.key = key;
      this.escapeAction = "make-key-unique";
    }
  };
  __name(_DuplicateKeyError, "DuplicateKeyError");
  var DuplicateKeyError = _DuplicateKeyError;
  var _SelectableListDisposedError = class _SelectableListDisposedError extends Error {
    constructor(method) {
      super(`[PetUI selectable-list] destroy() \u4E4B\u540E\u8C03\u7528\u4E86 ${method}\uFF08\u51FA\u8DEF=\u91CD\u65B0 createSelectableList\uFF09`);
      this.name = "SelectableListDisposedError";
      this.escapeAction = "create-a-new-list";
    }
  };
  __name(_SelectableListDisposedError, "SelectableListDisposedError");
  var SelectableListDisposedError = _SelectableListDisposedError;
  var _SelectableListContextError = class _SelectableListContextError extends Error {
    constructor() {
      super("[PetUI selectable-list] \u73AF\u5883\u6CA1\u6709 document\uFF08\u51FA\u8DEF=\u5728\u6D4F\u89C8\u5668/DOM \u73AF\u5883\u91CC\u521B\u5EFA\uFF09");
      this.name = "SelectableListContextError";
      this.reason = "no-document";
      this.escapeAction = "create-in-a-document-environment";
    }
  };
  __name(_SelectableListContextError, "SelectableListContextError");
  var SelectableListContextError = _SelectableListContextError;
  var LIMIT_REASON = "SelectionLimitReached";
  var LIMIT_ESCAPE = "deselect-a-row-or-raise-limit";
  var instanceSeq = 0;
  function hasContent(value) {
    return typeof value === "string" && value.trim().length > 0;
  }
  __name(hasContent, "hasContent");
  function normalizeKey(key) {
    if (typeof key === "string") return key;
    if (typeof key === "number" && Number.isFinite(key)) return String(key);
    throw new SelectableListConfigError(
      "invalid-item",
      `item.key \u5FC5\u987B\u662F\u5B57\u7B26\u4E32\u6216\u6709\u9650\u6570\u5B57\uFF08\u6536\u5230 ${typeof key}\uFF09`,
      "pass-string-key"
    );
  }
  __name(normalizeKey, "normalizeKey");
  function createSelectableList(options) {
    if (typeof document === "undefined" || document === null) throw new SelectableListContextError();
    if (options === null || typeof options !== "object") {
      throw new SelectableListConfigError("invalid-options", "options \u5FC5\u987B\u662F\u5BF9\u8C61", "pass-an-options-object");
    }
    if (!Array.isArray(options.items)) {
      throw new SelectableListConfigError("invalid-items", "items \u5FC5\u987B\u662F\u6570\u7EC4", "pass-an-array-of-items");
    }
    if (options.onSelect !== void 0 && typeof options.onSelect !== "function") {
      throw new SelectableListConfigError("invalid-on-select", "onSelect \u5FC5\u987B\u662F\u51FD\u6570", "pass-a-function-or-omit");
    }
    if (options.onLimitReached !== void 0 && typeof options.onLimitReached !== "function") {
      throw new SelectableListConfigError(
        "invalid-on-limit-reached",
        "onLimitReached \u5FC5\u987B\u662F\u51FD\u6570",
        "pass-a-function-or-omit"
      );
    }
    let limit = 1;
    if (options.limit !== void 0) {
      if (typeof options.limit !== "number" || !Number.isInteger(options.limit) || options.limit < 1) {
        throw new SelectableListConfigError(
          "invalid-limit",
          `limit \u5FC5\u987B\u662F >= 1 \u7684\u6574\u6570\uFF08\u6536\u5230 ${JSON.stringify(options.limit)}\uFF09`,
          "pass-integer-limit-gte-1"
        );
      }
      limit = options.limit;
    }
    const onSelect = options.onSelect ?? null;
    const onLimitReached = options.onLimitReached ?? null;
    const disposer = createDisposer();
    const instanceId = `petui-selectable-list-${instanceSeq += 1}`;
    const selected = /* @__PURE__ */ new Set();
    const rows = /* @__PURE__ */ new Map();
    const keyOfNode = /* @__PURE__ */ new Map();
    const warnedLimit = /* @__PURE__ */ new Set();
    const callbackErrors = [];
    let items = [];
    let activeKey = null;
    let destroyed = false;
    const el = document.createElement("div");
    el.style.cssText = "display:flex;flex-direction:column;gap:var(--space-4);min-width:var(--space-160);font-size:var(--font-size-14);color:var(--color-text)";
    const listbox = document.createElement("div");
    listbox.setAttribute("role", "listbox");
    listbox.setAttribute("tabindex", "0");
    if (limit > 1) listbox.setAttribute("aria-multiselectable", "true");
    listbox.style.cssText = "display:flex;flex-direction:column;gap:var(--space-2);overflow-y:auto;max-height:var(--space-300);padding:var(--space-4);border:var(--space-1) solid var(--color-border);border-radius:var(--border-radius-sm);background:var(--color-bg-light)";
    el.appendChild(listbox);
    const notice = document.createElement("div");
    notice.setAttribute("data-part", "limit-notice");
    notice.style.cssText = "display:flex;align-items:center;gap:var(--space-8);padding:var(--space-6);border:var(--space-1) solid var(--color-warning);border-radius:var(--border-radius-sm);background:var(--color-bg-light);color:var(--color-text);font-size:var(--font-size-12)";
    notice.hidden = true;
    const noticeText = document.createElement("span");
    noticeText.setAttribute("role", "status");
    noticeText.setAttribute("data-part", "limit-reason");
    noticeText.textContent = "";
    const dropOldest = document.createElement("button");
    dropOldest.setAttribute("type", "button");
    dropOldest.setAttribute("data-action", "drop-oldest");
    dropOldest.setAttribute("aria-label", "\u53D6\u6D88\u6700\u65E9\u9009\u4E2D\u7684\u4E00\u9879");
    dropOldest.textContent = "\u53D6\u6D88\u6700\u65E9\u9009\u4E2D";
    dropOldest.style.cssText = "border:var(--space-1) solid var(--color-border);border-radius:var(--border-radius-sm);padding:var(--space-4) var(--space-8);background:var(--color-bg-light);color:var(--color-primary);cursor:pointer;font-size:var(--font-size-12)";
    notice.appendChild(noticeText);
    notice.appendChild(dropOldest);
    el.appendChild(notice);
    const readSelectedKeys = /* @__PURE__ */ __name(() => Object.freeze(Array.from(selected)), "readSelectedKeys");
    const readLimit = /* @__PURE__ */ __name(() => limit, "readLimit");
    const api = {
      setItems: /* @__PURE__ */ __name((next) => {
        assertAlive("setItems");
        setItems(next);
      }, "setItems"),
      setSelected: /* @__PURE__ */ __name((next) => {
        assertAlive("setSelected");
        setSelected(next);
      }, "setSelected"),
      destroy: /* @__PURE__ */ __name(() => {
        destroy();
      }, "destroy"),
      get selectedKeys() {
        return readSelectedKeys();
      },
      get limit() {
        return readLimit();
      }
    };
    function assertAlive(method) {
      if (destroyed) throw new SelectableListDisposedError(method);
    }
    __name(assertAlive, "assertAlive");
    function callbackErrorCount() {
      return callbackErrors.length;
    }
    __name(callbackErrorCount, "callbackErrorCount");
    function callSafely(label, fn) {
      try {
        fn();
      } catch (err) {
        callbackErrors.push(`${label}: ${String(err)}`);
        console.error(`[PetUI selectable-list] ${label} \u56DE\u8C03\u629B\u9519\uFF08\u5DF2\u9694\u79BB\uFF09:`, err);
      }
    }
    __name(callSafely, "callSafely");
    function normalizeItems2(next) {
      if (!Array.isArray(next)) {
        throw new SelectableListConfigError("invalid-items", "setItems \u9700\u8981\u6570\u7EC4", "pass-an-array-of-items");
      }
      const seen = /* @__PURE__ */ new Set();
      const out = [];
      for (let i = 0; i < next.length; i += 1) {
        const raw = next[i];
        if (raw === null || typeof raw !== "object") {
          throw new SelectableListConfigError(
            "invalid-item",
            `items[${i}] \u5FC5\u987B\u662F\u5BF9\u8C61`,
            "pass-{key,primary}-objects"
          );
        }
        const key = normalizeKey(raw.key);
        if (key.length === 0) {
          throw new SelectableListConfigError(
            "invalid-item",
            `items[${i}].key \u4E0D\u5F97\u4E3A\u7A7A\u4E32`,
            "pass-non-empty-key"
          );
        }
        if (seen.has(key)) throw new DuplicateKeyError(key);
        seen.add(key);
        if (!hasContent(raw.primary)) {
          throw new SelectableListConfigError(
            "invalid-item",
            `items[${i}].primary \u4E0D\u5F97\u4E3A\u7A7A\u767D\uFF08key="${key}"\uFF09`,
            "pass-non-empty-primary"
          );
        }
        out.push({
          key,
          primary: String(raw.primary),
          secondary: raw.secondary,
          meta: raw.meta
        });
      }
      return out;
    }
    __name(normalizeItems2, "normalizeItems");
    function createRow2(key) {
      const row = document.createElement("div");
      row.setAttribute("role", "option");
      row.setAttribute("data-key", key);
      row.style.cssText = "display:flex;align-items:center;gap:var(--space-8);padding:var(--space-6) var(--space-8);min-height:var(--row-height-normal);border-radius:var(--border-radius-sm);cursor:pointer;background:var(--alpha-flat);color:var(--color-text)";
      const primary = document.createElement("span");
      primary.setAttribute("data-part", "primary");
      primary.style.cssText = "flex:1 1 auto";
      row.appendChild(primary);
      rows.set(key, row);
      keyOfNode.set(row, key);
      return row;
    }
    __name(createRow2, "createRow");
    function updateRow(row, item, index) {
      row.setAttribute("id", `${instanceId}-row-${index}`);
      if (row.dataset) row.dataset.petuiKey = item.key;
      row.setAttribute("data-key", item.key);
      const isSelected = selected.has(item.key);
      row.setAttribute("aria-selected", isSelected ? "true" : "false");
      row.setAttribute("data-selected", isSelected ? "true" : "false");
      row.setAttribute("data-active", activeKey === item.key ? "true" : "false");
      row.style.background = isSelected ? "var(--color-primary-light)" : "var(--alpha-flat)";
      row.style.color = isSelected ? "var(--color-primary)" : "var(--color-text)";
      const primary = row.childNodes[0];
      primary.textContent = item.primary;
      syncOptionalPart(row, "secondary", item.secondary, "var(--color-text-secondary)", "var(--font-size-13)");
      syncOptionalPart(row, "meta", item.meta, "var(--color-text-muted)", "var(--font-size-12)");
    }
    __name(updateRow, "updateRow");
    function syncOptionalPart(row, part, value, color, fontSize) {
      let node = null;
      for (const child of row.childNodes) {
        if (child.nodeType === 1 && child.getAttribute("data-part") === part) {
          node = child;
          break;
        }
      }
      if (!hasContent(value)) {
        if (node !== null && node.parentNode === row) row.removeChild(node);
        return;
      }
      if (node === null) {
        node = document.createElement("span");
        node.setAttribute("data-part", part);
        node.style.cssText = `color:${color};font-size:${fontSize};white-space:nowrap`;
        row.appendChild(node);
      }
      node.textContent = String(value);
    }
    __name(syncOptionalPart, "syncOptionalPart");
    function render() {
      const alive = /* @__PURE__ */ new Set();
      for (let i = 0; i < items.length; i += 1) {
        const item = items[i];
        alive.add(item.key);
        let row = rows.get(item.key);
        if (row === void 0) row = createRow2(item.key);
        updateRow(row, item, i);
        listbox.appendChild(row);
      }
      for (const [key, row] of Array.from(rows.entries())) {
        if (alive.has(key)) continue;
        rows.delete(key);
        keyOfNode.delete(row);
        if (row.parentNode === listbox) listbox.removeChild(row);
      }
      syncLimitState();
      syncActiveDescendant();
    }
    __name(render, "render");
    function syncLimitState() {
      const reached = limit > 1 && selected.size >= limit;
      el.setAttribute("data-limit-state", reached ? "reached" : "ok");
      for (const [key, row] of rows) {
        const blocked = reached && !selected.has(key);
        if (blocked) {
          row.setAttribute("data-limit-blocked", "true");
          row.setAttribute("aria-disabled", "true");
          row.style.color = "var(--color-text-muted)";
        } else {
          row.removeAttribute("data-limit-blocked");
          row.removeAttribute("aria-disabled");
          row.style.color = selected.has(key) ? "var(--color-primary)" : "var(--color-text)";
        }
      }
      if (!reached) {
        notice.hidden = true;
        el.removeAttribute("data-limit-reason");
      }
    }
    __name(syncLimitState, "syncLimitState");
    function announceLimit(key) {
      const message = `\u5DF2\u8FBE\u9009\u4E2D\u4E0A\u9650 limit=${limit}\uFF1Akey="${key}" \u672A\u88AB\u9009\u4E2D\u3002\u539F\u56E0=${LIMIT_REASON}\uFF1B\u51FA\u8DEF=${LIMIT_ESCAPE}\uFF08\u6FC0\u6D3B\u5DF2\u9009\u4E2D\u9879\u53D6\u6D88\u4E00\u9879\uFF0C\u6216\u6309"\u53D6\u6D88\u6700\u65E9\u9009\u4E2D"\uFF0C\u6216\u7528 setSelected() \u91CD\u8BBE\uFF09`;
      el.setAttribute("data-limit-reason", LIMIT_REASON);
      noticeText.textContent = message;
      notice.hidden = false;
      const stamp2 = `${key}|${limit}`;
      if (!warnedLimit.has(stamp2)) {
        warnedLimit.add(stamp2);
        console.warn(`[PetUI selectable-list] ${message}`);
      }
      const info = Object.freeze({
        reason: LIMIT_REASON,
        escapeAction: LIMIT_ESCAPE,
        key,
        limit,
        selectedKeys: Object.freeze(Array.from(selected))
      });
      if (onLimitReached !== null) callSafely("onLimitReached", () => onLimitReached(info));
    }
    __name(announceLimit, "announceLimit");
    function commitSelection() {
      render();
      if (onSelect !== null) {
        const changed = lastChangedKey;
        if (changed !== null) callSafely("onSelect", () => onSelect(changed));
      }
    }
    __name(commitSelection, "commitSelection");
    let lastChangedKey = null;
    function activate(key) {
      lastChangedKey = null;
      if (selected.has(key)) {
        selected.delete(key);
        lastChangedKey = key;
        activeKey = key;
        commitSelection();
        return;
      }
      if (limit === 1) {
        selected.clear();
        selected.add(key);
        lastChangedKey = key;
        activeKey = key;
        commitSelection();
        return;
      }
      if (selected.size >= limit) {
        activeKey = key;
        render();
        announceLimit(key);
        return;
      }
      selected.add(key);
      lastChangedKey = key;
      activeKey = key;
      commitSelection();
    }
    __name(activate, "activate");
    function setSelected(next) {
      const wanted = next === null ? [] : Array.isArray(next) ? next.slice() : [next];
      const known = new Set(items.map((it) => it.key));
      const deduped = [];
      for (const raw of wanted) {
        const key = normalizeKey(raw);
        if (!known.has(key)) {
          throw new SelectableListConfigError(
            "unknown-selected-key",
            `setSelected \u6536\u5230\u4E0D\u5728 items \u91CC\u7684 key "${key}"`,
            "pass-a-key-from-items"
          );
        }
        if (!deduped.includes(key)) deduped.push(key);
      }
      if (deduped.length > limit) {
        throw new SelectableListConfigError(
          "selection-over-limit",
          `setSelected \u7ED9\u4E86 ${deduped.length} \u4E2A key\uFF0C\u8D85\u8FC7 limit=${limit}`,
          "pass-at-most-limit-keys"
        );
      }
      selected.clear();
      for (const key of deduped) selected.add(key);
      activeKey = deduped.length > 0 ? deduped[deduped.length - 1] : null;
      lastChangedKey = null;
      render();
    }
    __name(setSelected, "setSelected");
    function setItems(next) {
      const normalized = normalizeItems2(next);
      items = normalized;
      const known = new Set(normalized.map((it) => it.key));
      for (const key of Array.from(selected)) if (!known.has(key)) selected.delete(key);
      if (activeKey === null || !known.has(activeKey)) {
        activeKey = normalized.length > 0 ? normalized[0].key : null;
      }
      render();
    }
    __name(setItems, "setItems");
    function syncActiveDescendant() {
      const row = activeKey === null ? void 0 : rows.get(activeKey);
      if (row === void 0) {
        listbox.removeAttribute("aria-activedescendant");
        listbox.removeAttribute("data-active-key");
        return;
      }
      listbox.setAttribute("aria-activedescendant", row.getAttribute("id") ?? "");
      listbox.setAttribute("data-active-key", activeKey);
    }
    __name(syncActiveDescendant, "syncActiveDescendant");
    function moveActive(delta) {
      if (items.length === 0) return;
      const current = activeKey === null ? -1 : items.findIndex((it) => it.key === activeKey);
      let index = current < 0 ? delta > 0 ? 0 : items.length - 1 : current + delta;
      if (index < 0) index = 0;
      if (index > items.length - 1) index = items.length - 1;
      activeKey = items[index].key;
      const row = rows.get(activeKey);
      if (row && typeof row.scrollIntoView === "function") {
        row.scrollIntoView({ block: "nearest" });
      }
      render();
    }
    __name(moveActive, "moveActive");
    function onKeyDown(ev) {
      const key = ev.key;
      if (key === "ArrowDown") {
        ev.preventDefault();
        moveActive(1);
        return;
      }
      if (key === "ArrowUp") {
        ev.preventDefault();
        moveActive(-1);
        return;
      }
      if (key === "Home") {
        ev.preventDefault();
        if (items.length > 0) {
          activeKey = items[0].key;
          render();
        }
        return;
      }
      if (key === "End") {
        ev.preventDefault();
        if (items.length > 0) {
          activeKey = items[items.length - 1].key;
          render();
        }
        return;
      }
      if (key === "Enter") {
        ev.preventDefault();
        if (activeKey !== null) activate(activeKey);
      }
    }
    __name(onKeyDown, "onKeyDown");
    function onClick(ev) {
      const target = ev.target;
      let cursor = target;
      while (cursor !== null && cursor !== void 0) {
        if (cursor === listbox) return;
        const found = keyOfNode.get(cursor);
        if (found !== void 0) {
          activate(found);
          return;
        }
        cursor = cursor.parentNode ?? null;
      }
    }
    __name(onClick, "onClick");
    function onDropOldest(ev) {
      ev.preventDefault();
      const first = selected.values().next();
      if (first.done === true) return;
      const key = first.value;
      selected.delete(key);
      lastChangedKey = key;
      commitSelection();
    }
    __name(onDropOldest, "onDropOldest");
    function destroy() {
      if (destroyed) return;
      destroyed = true;
      try {
        disposer.dispose();
      } catch (err) {
        if (err instanceof DisposeErrors) {
          console.error(`[PetUI selectable-list] destroy() \u6709 ${err.errors.length} \u4E2A\u91CA\u653E\u52A8\u4F5C\u5931\u8D25:`, err);
        } else {
          console.error("[PetUI selectable-list] destroy() \u91CA\u653E\u5931\u8D25:", err);
        }
      }
      rows.clear();
      keyOfNode.clear();
      selected.clear();
      if (el.parentNode !== null && typeof el.remove === "function") {
        el.remove();
      }
    }
    __name(destroy, "destroy");
    disposer.addEventListener(listbox, "keydown", onKeyDown);
    disposer.addEventListener(listbox, "click", onClick);
    disposer.addEventListener(dropOldest, "click", onDropOldest);
    const initial = normalizeItems2(options.items);
    items = initial;
    if (options.selectedKey !== void 0 && options.selectedKey !== null) {
      const key = normalizeKey(options.selectedKey);
      if (!initial.some((it) => it.key === key)) {
        throw new SelectableListConfigError(
          "unknown-selected-key",
          `\u521D\u59CB selectedKey "${key}" \u4E0D\u5728 items \u91CC`,
          "pass-a-key-from-items"
        );
      }
      selected.add(key);
      activeKey = key;
    } else if (initial.length > 0) {
      activeKey = initial[0].key;
    }
    if (selected.size > limit) {
      throw new SelectableListConfigError(
        "selection-over-limit",
        `\u521D\u59CB\u9009\u4E2D ${selected.size} \u9879\uFF0C\u8D85\u8FC7 limit=${limit}`,
        "pass-at-most-limit-keys"
      );
    }
    render();
    Object.defineProperties(el, {
      setItems: { value: api.setItems, writable: false, enumerable: true, configurable: false },
      setSelected: { value: api.setSelected, writable: false, enumerable: true, configurable: false },
      destroy: { value: api.destroy, writable: false, enumerable: true, configurable: false },
      selectedKeys: { get: readSelectedKeys, enumerable: true, configurable: false },
      limit: { get: readLimit, enumerable: true, configurable: false },
      callbackErrorCount: { value: callbackErrorCount, writable: false, enumerable: true, configurable: false }
    });
    return el;
  }
  __name(createSelectableList, "createSelectableList");
  var SELECTABLE_LIST_PARTS = Object.freeze({
    root: "[data-limit-state]",
    listbox: '[role="listbox"]',
    row: '[role="option"]',
    primary: '[data-part="primary"]',
    secondary: '[data-part="secondary"]',
    meta: '[data-part="meta"]',
    notice: '[data-part="limit-notice"]',
    dropOldest: '[data-action="drop-oldest"]'
  });

  // src/lib/debounce.ts
  var TIMING_OPTION_KEYS = ["leading", "trailing", "clock"];
  function defaultClock() {
    const g = globalThis;
    if (typeof g.setTimeout !== "function" || typeof g.clearTimeout !== "function") {
      throw new Error(
        "TimingClockUnavailableError: \u5F53\u524D\u5BBF\u4E3B\u6CA1\u6709 setTimeout/clearTimeout\uFF0C\u65E0\u6CD5\u5EFA\u7ACB\u5B9A\u65F6\u5668\uFF1B\u8BF7\u5728 opts.clock \u6CE8\u5165\u65F6\u949F\u5B9E\u73B0"
      );
    }
    const readNow = g.Date && typeof g.Date.now === "function" ? () => g.Date.now() : () => 0;
    return {
      now: readNow,
      setTimer: /* @__PURE__ */ __name((fn, ms) => g.setTimeout(fn, ms), "setTimer"),
      clearTimer: /* @__PURE__ */ __name((handle) => {
        if (handle !== void 0 && handle !== null) g.clearTimeout(handle);
      }, "clearTimer")
    };
  }
  __name(defaultClock, "defaultClock");
  function resolveDelay(ms) {
    if (typeof ms !== "number" || Number.isNaN(ms)) {
      throw new RangeError(`TimingDelayError: ms \u5FC5\u987B\u662F\u6570\u5B57\uFF0C\u6536\u5230 ${String(ms)}`);
    }
    if (!Number.isFinite(ms)) {
      throw new RangeError(`TimingDelayError: ms \u5FC5\u987B\u662F\u6709\u9650\u6570\uFF0C\u6536\u5230 ${String(ms)}`);
    }
    if (ms < 0) {
      throw new RangeError(`TimingDelayError: ms \u5FC5\u987B >= 0\uFF08\u8D1F\u5EF6\u8FDF\u65E0\u786E\u5B9A\u8BED\u4E49\uFF09\uFF0C\u6536\u5230 ${ms}`);
    }
    return ms;
  }
  __name(resolveDelay, "resolveDelay");
  function resolveTarget(fn) {
    if (typeof fn !== "function") {
      throw new TypeError(`TimingTargetError: \u7B2C\u4E00\u4E2A\u53C2\u6570\u5FC5\u987B\u662F\u51FD\u6570\uFF0C\u6536\u5230 ${typeof fn}`);
    }
    return fn;
  }
  __name(resolveTarget, "resolveTarget");
  function readTimingOptions(opts, defaultLeading, caller) {
    if (opts !== void 0 && (opts === null || typeof opts !== "object")) {
      throw new TypeError(`${caller}: opts \u5FC5\u987B\u662F\u5BF9\u8C61\uFF0C\u6536\u5230 ${String(opts)}`);
    }
    for (const key of Object.keys(opts ?? {})) {
      if (!TIMING_OPTION_KEYS.includes(key)) {
        throw new RangeError(`${caller}: \u672A\u77E5\u9009\u9879 "${key}"\uFF08\u5141\u8BB8\uFF1Aleading/trailing/clock\uFF09`);
      }
    }
    return Object.freeze({
      leading: opts?.leading ?? defaultLeading,
      trailing: opts?.trailing ?? true,
      clock: opts?.clock ?? defaultClock()
    });
  }
  __name(readTimingOptions, "readTimingOptions");
  function debounce(fn, ms, opts) {
    const target = resolveTarget(fn);
    const delay = resolveDelay(ms);
    const timing = readTimingOptions(opts, false, "debounce");
    let handle = null;
    let pending = false;
    let flushArmed = false;
    let fireEpoch = 0;
    let armedAt = 0;
    let lastArgs = [];
    let lastThis = void 0;
    let lastResult;
    const fireTrailing = /* @__PURE__ */ __name(() => {
      handle = null;
      pending = false;
      flushArmed = false;
      fireEpoch++;
      if (!timing.trailing) {
        lastArgs = [];
        lastThis = void 0;
        return;
      }
      lastResult = target.apply(lastThis, lastArgs);
      lastArgs = [];
      lastThis = void 0;
    }, "fireTrailing");
    const wrapped = /* @__PURE__ */ __name(function(...args) {
      const isLeadingCall = handle === null && !pending;
      lastArgs = args;
      lastThis = this;
      flushArmed = true;
      armedAt = fireEpoch;
      if (handle !== null) timing.clock.clearTimer(handle);
      pending = true;
      handle = timing.clock.setTimer(fireTrailing, delay);
      if (isLeadingCall && timing.leading) {
        lastResult = target.apply(this, args);
      }
      return lastResult;
    }, "wrapped");
    const cancel = /* @__PURE__ */ __name(() => {
      if (handle !== null) timing.clock.clearTimer(handle);
      handle = null;
      pending = false;
      flushArmed = false;
      lastArgs = [];
      lastThis = void 0;
    }, "cancel");
    const flush = /* @__PURE__ */ __name(() => {
      if (!timing.trailing) return void 0;
      if (!pending || !flushArmed || armedAt !== fireEpoch) return void 0;
      if (handle !== null) timing.clock.clearTimer(handle);
      handle = null;
      pending = false;
      flushArmed = false;
      lastResult = target.apply(lastThis, lastArgs);
      lastArgs = [];
      lastThis = void 0;
      return lastResult;
    }, "flush");
    const out = wrapped;
    out.cancel = cancel;
    out.flush = flush;
    return out;
  }
  __name(debounce, "debounce");

  // src/components/filterable-listbox.ts
  var _FilterableListboxConfigError = class _FilterableListboxConfigError extends TypeError {
    constructor(reason, message, escapeAction) {
      super(`[PetUI filterable-listbox] ${message}\uFF08reason=${reason}\uFF1B\u51FA\u8DEF=${escapeAction}\uFF09`);
      this.name = "FilterableListboxConfigError";
      this.reason = reason;
      this.escapeAction = escapeAction;
    }
  };
  __name(_FilterableListboxConfigError, "FilterableListboxConfigError");
  var FilterableListboxConfigError = _FilterableListboxConfigError;
  var _DuplicateKeyError2 = class _DuplicateKeyError2 extends Error {
    constructor(key) {
      super(`[PetUI filterable-listbox] \u91CD\u590D key "${key}"\uFF1Akey \u662F\u884C\u8EAB\u4EFD\uFF0C\u5FC5\u987B\u552F\u4E00\uFF08\u51FA\u8DEF=\u8BA9 key \u552F\u4E00\uFF09`);
      this.name = "DuplicateKeyError";
      this.key = key;
      this.escapeAction = "make-key-unique";
    }
  };
  __name(_DuplicateKeyError2, "DuplicateKeyError");
  var DuplicateKeyError2 = _DuplicateKeyError2;
  var _RenderRowTypeError = class _RenderRowTypeError extends TypeError {
    constructor(key, actual) {
      super(
        `[PetUI filterable-listbox] renderRow \u8FD4\u56DE\u4E86 ${actual}\uFF08key="${key}"\uFF09\uFF1AK7 \u53EA\u63A5\u53D7 Node \u6216 null\uFF08\u51FA\u8DEF=\u8FD4\u56DE\u5143\u7D20\uFF0C\u6587\u672C\u7528 textContent \u5199\u5165\u8BE5\u5143\u7D20\uFF09`
      );
      this.name = "RenderRowTypeError";
      this.key = key;
      this.escapeAction = "return-a-node-with-textContent";
    }
  };
  __name(_RenderRowTypeError, "RenderRowTypeError");
  var RenderRowTypeError = _RenderRowTypeError;
  var _FilterableListboxDisposedError = class _FilterableListboxDisposedError extends Error {
    constructor(method) {
      super(`[PetUI filterable-listbox] destroy() \u4E4B\u540E\u8C03\u7528\u4E86 ${method}\uFF08\u51FA\u8DEF=\u91CD\u65B0 createFilterableListbox\uFF09`);
      this.name = "FilterableListboxDisposedError";
      this.escapeAction = "create-a-new-listbox";
    }
  };
  __name(_FilterableListboxDisposedError, "FilterableListboxDisposedError");
  var FilterableListboxDisposedError = _FilterableListboxDisposedError;
  var _FilterableListboxContextError = class _FilterableListboxContextError extends Error {
    constructor() {
      super("[PetUI filterable-listbox] \u73AF\u5883\u6CA1\u6709 document\uFF08\u51FA\u8DEF=\u5728\u6D4F\u89C8\u5668/DOM \u73AF\u5883\u91CC\u521B\u5EFA\uFF09");
      this.name = "FilterableListboxContextError";
      this.reason = "no-document";
      this.escapeAction = "create-in-a-document-environment";
    }
  };
  __name(_FilterableListboxContextError, "FilterableListboxContextError");
  var FilterableListboxContextError = _FilterableListboxContextError;
  var PICK_REASON_DEFAULT = "ItemDisabledByHost";
  var PICK_ESCAPE = "move-to-an-enabled-row-or-let-the-host-enable-it";
  var NO_ENABLED_ALTERNATIVE = "NoEnabledAlternative";
  var NO_ENABLED_ALTERNATIVE_ESCAPE = "change-the-query-or-let-the-host-enable-an-item";
  var NO_MATCH_REASON = "NoMatch";
  var NO_MATCH_ESCAPE = "change-or-clear-the-query";
  var FILTER_ERROR_REASON = "FilterSyntaxError";
  var FILTER_ERROR_ESCAPE = "fix-the-filter-syntax-or-the-query";
  var DEFAULT_FILTER_DELAY_MS = 120;
  var ERROR_PREFIX2 = "[PetUI filterable-listbox]";
  var instanceSeq2 = 0;
  function hasText(value) {
    return typeof value === "string" && value.trim().length > 0;
  }
  __name(hasText, "hasText");
  function normalizeKey2(key, where) {
    if (typeof key === "string" && key.length > 0) return key;
    if (typeof key === "number" && Number.isFinite(key)) return String(key);
    throw new FilterableListboxConfigError(
      "invalid-key",
      `${where} \u7684 key \u5FC5\u987B\u662F\u975E\u7A7A\u5B57\u7B26\u4E32\u6216\u6709\u9650\u6570\u5B57\uFF08\u6536\u5230 ${key === null ? "null" : typeof key}\uFF09`,
      "pass-a-string-key"
    );
  }
  __name(normalizeKey2, "normalizeKey");
  function itemTextOf(item) {
    if (typeof item === "string") return item;
    if (typeof item === "number" && Number.isFinite(item)) return String(item);
    if (item !== null && typeof item === "object") {
      const rec = item;
      const fields = ["text", "label", "key"];
      for (let i = 0; i < fields.length; i += 1) {
        const value = rec[fields[i]];
        if (typeof value === "string") return value;
        if (typeof value === "number" && Number.isFinite(value)) return String(value);
      }
    }
    throw new FilterableListboxConfigError(
      "unresolvable-item-text",
      "\u5185\u7F6E\u5B57\u7B26\u4E32\u8FC7\u6EE4\u8BED\u6CD5\u9700\u8981\u80FD\u4ECE item \u53D6\u5230\u6587\u672C\uFF08string item\uFF0C\u6216 item.text / item.label / item.key\uFF09",
      "pass-a-predicate-function"
    );
  }
  __name(itemTextOf, "itemTextOf");
  function buildPredicate(syntax) {
    if (typeof syntax === "function") return syntax;
    if (syntax === "substring") {
      return (item, query) => itemTextOf(item).toLowerCase().includes(query.toLowerCase());
    }
    if (syntax === "prefix") {
      return (item, query) => itemTextOf(item).toLowerCase().startsWith(query.toLowerCase());
    }
    if (syntax === "exact") {
      return (item, query) => itemTextOf(item).toLowerCase() === query.toLowerCase();
    }
    if (syntax === "regex") {
      return (item, query) => new RegExp(query, "i").test(itemTextOf(item));
    }
    throw new FilterableListboxConfigError(
      "invalid-filter-syntax",
      `filterSyntax \u5FC5\u987B\u662F\u8C13\u8BCD\u51FD\u6570\u6216 'substring'|'prefix'|'exact'|'regex'\uFF08\u6536\u5230 ${JSON.stringify(syntax)}\uFF09`,
      "pass-a-predicate-function"
    );
  }
  __name(buildPredicate, "buildPredicate");
  function createFilterableListbox(options) {
    if (typeof document === "undefined" || document === null) throw new FilterableListboxContextError();
    if (options === null || typeof options !== "object") {
      throw new FilterableListboxConfigError("invalid-options", "options \u5FC5\u987B\u662F\u5BF9\u8C61", "pass-an-options-object");
    }
    if (!Array.isArray(options.items)) {
      throw new FilterableListboxConfigError("invalid-items", "items \u5FC5\u987B\u662F\u6570\u7EC4", "pass-an-array-of-items");
    }
    if (options.renderRow === void 0 || typeof options.renderRow !== "function") {
      throw new FilterableListboxConfigError(
        "invalid-render-row",
        "renderRow \u5FC5\u586B\u4E14\u5FC5\u987B\u662F\u51FD\u6570",
        "pass-a-function-returning-a-node"
      );
    }
    if (options.onPick !== void 0 && typeof options.onPick !== "function") {
      throw new FilterableListboxConfigError("invalid-on-pick", "onPick \u5FC5\u987B\u662F\u51FD\u6570", "pass-a-function-or-omit");
    }
    if (options.onDisabled !== void 0 && typeof options.onDisabled !== "function") {
      throw new FilterableListboxConfigError("invalid-on-disabled", "onDisabled \u5FC5\u987B\u662F\u51FD\u6570", "pass-a-function-or-omit");
    }
    if (options.placeholder !== void 0 && typeof options.placeholder !== "string") {
      throw new FilterableListboxConfigError("invalid-placeholder", "placeholder \u5FC5\u987B\u662F\u5B57\u7B26\u4E32", "pass-a-string-or-omit");
    }
    const predicate = buildPredicate(options.filterSyntax);
    const renderRow = options.renderRow;
    const onPick = options.onPick ?? null;
    const onDisabled = options.onDisabled ?? null;
    const placeholder = options.placeholder ?? "\u8F93\u5165\u4EE5\u8FC7\u6EE4\u5217\u8868\u9879";
    const disposer = createDisposer();
    const instanceId = `petui-filterable-listbox-${instanceSeq2 += 1}`;
    let entries = [];
    let matches = [];
    let query = "";
    let activeKey = null;
    let filterError = null;
    let destroyed = false;
    const rows = /* @__PURE__ */ new Map();
    const renderedItem = /* @__PURE__ */ new Map();
    const nodeKey = /* @__PURE__ */ new Map();
    const warned = /* @__PURE__ */ new Set();
    const callbackErrors = [];
    const el = document.createElement("div");
    el.setAttribute("data-part", "filterable-listbox");
    el.style.cssText = "display:flex;flex-direction:column;gap:var(--space-4);min-width:var(--space-160);font-size:var(--font-size-14);color:var(--color-text)";
    const filterInput = document.createElement("input");
    filterInput.setAttribute("data-part", "filter");
    filterInput.setAttribute("type", "search");
    filterInput.setAttribute("aria-label", placeholder);
    filterInput.setAttribute("aria-controls", `${instanceId}-listbox`);
    filterInput.setAttribute("autocomplete", "off");
    filterInput.setAttribute("placeholder", placeholder);
    filterInput.value = "";
    filterInput.style.cssText = "padding:var(--space-4) var(--space-8);border:var(--space-1) solid var(--color-border);border-radius:var(--border-radius-sm);background:var(--color-bg-light);color:var(--color-text);font-size:var(--font-size-13)";
    el.appendChild(filterInput);
    const listbox = document.createElement("div");
    listbox.setAttribute("role", "listbox");
    listbox.setAttribute("id", `${instanceId}-listbox`);
    listbox.setAttribute("tabindex", "0");
    listbox.style.cssText = "display:flex;flex-direction:column;gap:var(--space-2);overflow-y:auto;max-height:var(--space-300);padding:var(--space-4);border:var(--space-1) solid var(--color-border);border-radius:var(--border-radius-sm);background:var(--color-bg-light)";
    el.appendChild(listbox);
    const empty = document.createElement("div");
    empty.setAttribute("data-part", "empty");
    empty.setAttribute("role", "status");
    empty.style.cssText = "padding:var(--space-6);color:var(--color-text-muted);font-size:var(--font-size-12)";
    empty.hidden = true;
    el.appendChild(empty);
    const notice = document.createElement("div");
    notice.setAttribute("data-part", "pick-notice");
    notice.style.cssText = "display:flex;align-items:center;gap:var(--space-8);padding:var(--space-6);border:var(--space-1) solid var(--color-warning);border-radius:var(--border-radius-sm);background:var(--color-bg-light);color:var(--color-text);font-size:var(--font-size-12)";
    notice.hidden = true;
    const noticeText = document.createElement("span");
    noticeText.setAttribute("role", "status");
    noticeText.setAttribute("data-part", "notice-reason");
    noticeText.textContent = "";
    const nextEnabled = document.createElement("button");
    nextEnabled.setAttribute("type", "button");
    nextEnabled.setAttribute("data-action", "next-enabled");
    nextEnabled.setAttribute("aria-label", "\u628A\u6D3B\u52A8\u9879\u79FB\u5230\u4E0B\u4E00\u4E2A\u672A\u88AB\u7981\u7528\u7684\u547D\u4E2D\u9879");
    nextEnabled.textContent = "\u79FB\u5230\u4E0B\u4E00\u4E2A\u53EF\u7528\u9879";
    nextEnabled.style.cssText = "border:var(--space-1) solid var(--color-border);border-radius:var(--border-radius-sm);padding:var(--space-4) var(--space-8);background:var(--color-bg-light);color:var(--color-primary);cursor:pointer;font-size:var(--font-size-12)";
    notice.appendChild(noticeText);
    notice.appendChild(nextEnabled);
    el.appendChild(notice);
    const readQuery = /* @__PURE__ */ __name(() => query, "readQuery");
    const readActiveKey = /* @__PURE__ */ __name(() => activeKey, "readActiveKey");
    const readMatchCount = /* @__PURE__ */ __name(() => matches.length, "readMatchCount");
    function assertAlive(method) {
      if (destroyed) throw new FilterableListboxDisposedError(method);
    }
    __name(assertAlive, "assertAlive");
    function callSafely(label, fn) {
      try {
        fn();
      } catch (err) {
        callbackErrors.push(`${label}: ${String(err)}`);
        console.error(`${ERROR_PREFIX2} ${label} \u56DE\u8C03\u629B\u9519\uFF08\u5DF2\u9694\u79BB\uFF09:`, err);
      }
    }
    __name(callSafely, "callSafely");
    function warnOnce(stamp2, message) {
      if (warned.has(stamp2)) return;
      warned.add(stamp2);
      console.warn(`${ERROR_PREFIX2} ${message}`);
    }
    __name(warnOnce, "warnOnce");
    function showNotice(kind, reason, escapeAction, message) {
      notice.setAttribute("data-notice-kind", kind);
      notice.setAttribute("data-notice-reason", reason);
      notice.setAttribute("data-notice-escape", escapeAction);
      noticeText.textContent = message;
      notice.hidden = false;
    }
    __name(showNotice, "showNotice");
    function clearPickState() {
      el.removeAttribute("data-pick-state");
      el.removeAttribute("data-pick-reason");
      el.removeAttribute("data-pick-escape");
      el.removeAttribute("data-pick-key");
      if (notice.getAttribute("data-notice-kind") === "pick") {
        notice.hidden = true;
        noticeText.textContent = "";
        notice.removeAttribute("data-notice-kind");
        notice.removeAttribute("data-notice-reason");
        notice.removeAttribute("data-notice-escape");
      }
    }
    __name(clearPickState, "clearPickState");
    function disabledReasonOf(item, index, key) {
      if (onDisabled === null) return null;
      let raw;
      try {
        raw = onDisabled(item, index, key);
      } catch (err) {
        console.error(`${ERROR_PREFIX2} onDisabled \u629B\u9519\uFF08key="${key}"\uFF09\uFF0C\u8BE5\u9879\u6309**\u7981\u7528**\u5904\u7406:`, err);
        warnOnce(`onDisabled-threw|${key}`, `onDisabled \u629B\u9519\uFF08key="${key}"\uFF09\u21D2 \u8BE5\u9879\u6309\u7981\u7528\u5904\u7406\uFF08\u51FA\u8DEF=${PICK_ESCAPE}\uFF09`);
        return `onDisabledThrew`;
      }
      if (raw === true) return PICK_REASON_DEFAULT;
      if (typeof raw === "string") return hasText(raw) ? raw : null;
      return raw ? PICK_REASON_DEFAULT : null;
    }
    __name(disabledReasonOf, "disabledReasonOf");
    function matchIndexOf(key) {
      for (let i = 0; i < matches.length; i += 1) if (matches[i].key === key) return i;
      return -1;
    }
    __name(matchIndexOf, "matchIndexOf");
    function createRow2(key) {
      const row = document.createElement("div");
      row.setAttribute("role", "option");
      row.setAttribute("data-key", key);
      row.style.cssText = "display:flex;align-items:center;gap:var(--space-8);padding:var(--space-6) var(--space-8);min-height:var(--row-height-normal);border-radius:var(--border-radius-sm);cursor:pointer;background:var(--alpha-flat);color:var(--color-text)";
      rows.set(key, row);
      nodeKey.set(row, key);
      return row;
    }
    __name(createRow2, "createRow");
    function renderRowInto(row, item, index, key) {
      let node;
      let degraded = null;
      try {
        node = renderRow(item, index, key);
        if (typeof node === "string") throw new RenderRowTypeError(key, "string");
        if (node !== null && node !== void 0 && typeof node.nodeType !== "number") {
          throw new RenderRowTypeError(key, typeof node);
        }
      } catch (err) {
        if (err instanceof RenderRowTypeError) throw err;
        console.error(`${ERROR_PREFIX2} renderRow \u629B\u9519\uFF08key="${key}"\uFF09:`, err);
        node = null;
        degraded = "renderRow-threw";
      }
      if (node === null || node === void 0) {
        if (degraded === null) degraded = "renderRow-returned-null";
        const holder = document.createElement("span");
        holder.setAttribute("data-part", "row-placeholder");
        holder.setAttribute("data-degraded", degraded);
        holder.style.cssText = "color:var(--color-text-muted)";
        holder.textContent = `\u8BE5\u884C\u672A\u6E32\u67D3\uFF08key="${key}"\uFF1B\u539F\u56E0=${degraded}\uFF1B\u51FA\u8DEF=\u8BA9 renderRow \u8FD4\u56DE\u5143\u7D20\uFF09`;
        node = holder;
      }
      while (row.firstChild) row.removeChild(row.firstChild);
      row.appendChild(node);
      renderedItem.set(key, item);
    }
    __name(renderRowInto, "renderRowInto");
    function updateRow(row, match, position) {
      const { key, item, index } = match;
      row.setAttribute("id", `${instanceId}-row-${position}`);
      if (row.dataset) row.dataset.petuiKey = key;
      row.setAttribute("data-key", key);
      const isActive = activeKey === key;
      row.setAttribute("aria-selected", isActive ? "true" : "false");
      row.setAttribute("data-active", isActive ? "true" : "false");
      const reason = disabledReasonOf(item, index, key);
      if (reason === null) {
        row.removeAttribute("data-disabled");
        row.removeAttribute("data-disabled-reason");
        row.removeAttribute("aria-disabled");
        row.style.background = isActive ? "var(--color-primary-light)" : "var(--alpha-flat)";
        row.style.color = isActive ? "var(--color-primary)" : "var(--color-text)";
      } else {
        row.setAttribute("data-disabled", "true");
        row.setAttribute("data-disabled-reason", reason);
        row.setAttribute("aria-disabled", "true");
        row.style.background = "var(--alpha-flat)";
        row.style.color = "var(--color-text-muted)";
      }
      if (!renderedItem.has(key) || renderedItem.get(key) !== item) {
        renderRowInto(row, item, index, key);
      }
    }
    __name(updateRow, "updateRow");
    function render() {
      const alive = /* @__PURE__ */ new Set();
      for (let i = 0; i < matches.length; i += 1) {
        const match = matches[i];
        alive.add(match.key);
        let row = rows.get(match.key);
        if (row === void 0) row = createRow2(match.key);
        updateRow(row, match, i);
        listbox.appendChild(row);
      }
      for (const [key, row] of Array.from(rows.entries())) {
        if (alive.has(key)) continue;
        if (row.parentNode === listbox) listbox.removeChild(row);
      }
      el.setAttribute("data-query", query);
      el.setAttribute("data-item-count", String(entries.length));
      el.setAttribute("data-match-count", String(matches.length));
      const activeRow = activeKey === null ? void 0 : rows.get(activeKey);
      if (activeRow === void 0 || activeRow.parentNode !== listbox) {
        listbox.removeAttribute("aria-activedescendant");
        listbox.removeAttribute("data-active-key");
      } else {
        listbox.setAttribute("aria-activedescendant", activeRow.getAttribute("id") ?? "");
        listbox.setAttribute("data-active-key", activeKey);
      }
    }
    __name(render, "render");
    function syncEmptyState() {
      if (matches.length > 0) {
        empty.hidden = true;
        el.removeAttribute("data-empty");
        return;
      }
      const reason = filterError === null ? NO_MATCH_REASON : FILTER_ERROR_REASON;
      const escapeAction = filterError === null ? NO_MATCH_ESCAPE : FILTER_ERROR_ESCAPE;
      el.setAttribute("data-empty", reason);
      empty.setAttribute("data-empty-reason", reason);
      empty.setAttribute("data-empty-escape", escapeAction);
      empty.textContent = filterError === null ? `\u65E0\u547D\u4E2D\u9879\uFF1Aquery="${query}" \u547D\u4E2D 0 \u9879\uFF08\u539F\u56E0=${reason}\uFF1B\u51FA\u8DEF=${escapeAction}\uFF09` : `\u67E5\u8BE2\u5931\u8D25\uFF1A\u539F\u56E0=${reason}\uFF08${String(filterError)}\uFF09\uFF1B\u51FA\u8DEF=${escapeAction}`;
      empty.hidden = false;
    }
    __name(syncEmptyState, "syncEmptyState");
    function applyQuery(next) {
      query = next;
      filterError = null;
      const trimmed = next.trim();
      const out = [];
      if (trimmed === "") {
        for (let i = 0; i < entries.length; i += 1) out.push(entries[i]);
      } else {
        for (let i = 0; i < entries.length; i += 1) {
          const match = entries[i];
          try {
            if (predicate(match.item, next, match.index) === true) out.push(match);
          } catch (err) {
            filterError = err;
            out.length = 0;
            break;
          }
        }
      }
      matches = out;
      if (filterError !== null) {
        const message = `\u8FC7\u6EE4\u67E5\u8BE2\u5931\u8D25\uFF1A\u539F\u56E0=${FILTER_ERROR_REASON}\uFF08${String(filterError)}\uFF09\uFF1B\u51FA\u8DEF=${FILTER_ERROR_ESCAPE}`;
        showNotice("filter", FILTER_ERROR_REASON, FILTER_ERROR_ESCAPE, message);
        warnOnce(`filter-error|${String(filterError)}`, message);
      } else if (notice.getAttribute("data-notice-kind") === "filter") {
        notice.hidden = true;
        noticeText.textContent = "";
        notice.removeAttribute("data-notice-kind");
        notice.removeAttribute("data-notice-reason");
        notice.removeAttribute("data-notice-escape");
      }
      if (activeKey !== null && matchIndexOf(activeKey) < 0) {
        activeKey = matches.length > 0 ? matches[0].key : null;
        clearPickState();
      }
      if (activeKey === null && matches.length > 0) activeKey = matches[0].key;
      render();
      syncEmptyState();
    }
    __name(applyQuery, "applyQuery");
    function setQuery(next) {
      if (typeof next !== "string") {
        throw new FilterableListboxConfigError(
          "invalid-query",
          `setQuery \u9700\u8981\u5B57\u7B26\u4E32\uFF08\u6536\u5230 ${typeof next}\uFF09`,
          "pass-a-string"
        );
      }
      runFilter.cancel();
      if (filterInput.value !== next) filterInput.value = next;
      applyQuery(next);
    }
    __name(setQuery, "setQuery");
    function rejectPick(key, reason) {
      render();
      el.setAttribute("data-pick-state", "rejected");
      el.setAttribute("data-pick-reason", reason);
      el.setAttribute("data-pick-escape", PICK_ESCAPE);
      el.setAttribute("data-pick-key", key);
      const message = `\u5DF2\u62D2\u7EDD\uFF1Akey="${key}" \u547D\u4E2D\u4F46\u88AB\u7981\u7528\uFF0C\u672A\u89E6\u53D1 onPick\uFF08\u539F\u56E0=${reason}\uFF1B\u51FA\u8DEF=${PICK_ESCAPE}\uFF09\uFF1A\u7528 \u2191\u2193 \u79FB\u5230\u672A\u7981\u7528\u9879\u540E Enter\uFF0C\u6216\u6309\u300C\u79FB\u5230\u4E0B\u4E00\u4E2A\u53EF\u7528\u9879\u300D\uFF0C\u6216\u8BA9\u5BBF\u4E3B\u89E3\u9664\u7981\u7528\u540E setItems()`;
      showNotice("pick", reason, PICK_ESCAPE, message);
      warnOnce(`pick-rejected|${key}|${reason}`, message);
    }
    __name(rejectPick, "rejectPick");
    function activate(key) {
      const position = matchIndexOf(key);
      if (position < 0) return;
      const match = matches[position];
      activeKey = key;
      const reason = disabledReasonOf(match.item, match.index, key);
      if (reason !== null) {
        rejectPick(key, reason);
        return;
      }
      clearPickState();
      el.setAttribute("data-pick-state", "picked");
      el.setAttribute("data-pick-key", key);
      render();
      if (onPick !== null) callSafely("onPick", () => onPick(key, match.item, match.index));
    }
    __name(activate, "activate");
    function moveActive(delta) {
      if (matches.length === 0) return;
      const current = activeKey === null ? -1 : matchIndexOf(activeKey);
      let index = current < 0 ? delta > 0 ? 0 : matches.length - 1 : current + delta;
      if (index < 0) index = 0;
      if (index > matches.length - 1) index = matches.length - 1;
      activeKey = matches[index].key;
      const row = rows.get(activeKey);
      if (row && typeof row.scrollIntoView === "function") {
        row.scrollIntoView({ block: "nearest" });
      }
      clearPickState();
      render();
    }
    __name(moveActive, "moveActive");
    function moveToNextEnabled() {
      if (matches.length === 0) return;
      const current = activeKey === null ? -1 : matchIndexOf(activeKey);
      for (let step = 1; step <= matches.length; step += 1) {
        const candidate = matches[(current + step + matches.length) % matches.length];
        if (candidate === void 0) break;
        if (disabledReasonOf(candidate.item, candidate.index, candidate.key) === null) {
          activeKey = candidate.key;
          clearPickState();
          render();
          return;
        }
      }
      const message = `\u6CA1\u6709\u53EF\u7528\u7684\u672A\u7981\u7528\u66FF\u4EE3\u9879\uFF08\u539F\u56E0=${NO_ENABLED_ALTERNATIVE}\uFF1B\u51FA\u8DEF=${NO_ENABLED_ALTERNATIVE_ESCAPE}\uFF09`;
      el.setAttribute("data-pick-escape", NO_ENABLED_ALTERNATIVE_ESCAPE);
      showNotice("pick", NO_ENABLED_ALTERNATIVE, NO_ENABLED_ALTERNATIVE_ESCAPE, message);
      warnOnce("no-enabled-alternative", message);
    }
    __name(moveToNextEnabled, "moveToNextEnabled");
    function onKeyDown(ev) {
      const key = ev.key;
      if (key === "ArrowDown") {
        ev.preventDefault();
        moveActive(1);
        return;
      }
      if (key === "ArrowUp") {
        ev.preventDefault();
        moveActive(-1);
        return;
      }
      if (key === "Home") {
        ev.preventDefault();
        if (matches.length > 0) {
          activeKey = matches[0].key;
          clearPickState();
          render();
        }
        return;
      }
      if (key === "End") {
        ev.preventDefault();
        if (matches.length > 0) {
          activeKey = matches[matches.length - 1].key;
          clearPickState();
          render();
        }
        return;
      }
      if (key === "Enter") {
        ev.preventDefault();
        if (activeKey !== null) activate(activeKey);
        return;
      }
      if (key === "Escape") {
        ev.preventDefault();
        clearPickState();
        setQuery("");
      }
    }
    __name(onKeyDown, "onKeyDown");
    function onClick(ev) {
      const target = ev.target;
      let cursor = target;
      while (cursor !== null && cursor !== void 0) {
        if (cursor === listbox) return;
        const found = nodeKey.get(cursor);
        if (found !== void 0) {
          activate(found);
          return;
        }
        cursor = cursor.parentNode ?? null;
      }
    }
    __name(onClick, "onClick");
    const runFilter = debounce(
      (next) => {
        if (destroyed) return;
        applyQuery(next);
      },
      options.filterDelayMs === void 0 ? DEFAULT_FILTER_DELAY_MS : options.filterDelayMs,
      options.clock === void 0 ? void 0 : { clock: options.clock }
    );
    function onInput() {
      const value = typeof filterInput.value === "string" ? filterInput.value : "";
      runFilter(value);
    }
    __name(onInput, "onInput");
    function onNextEnabled(ev) {
      ev.preventDefault();
      moveToNextEnabled();
    }
    __name(onNextEnabled, "onNextEnabled");
    function normalizeItems2(next) {
      if (!Array.isArray(next)) {
        throw new FilterableListboxConfigError("invalid-items", "setItems \u9700\u8981\u6570\u7EC4", "pass-an-array-of-items");
      }
      const seen = /* @__PURE__ */ new Set();
      const out = [];
      for (let i = 0; i < next.length; i += 1) {
        const item = next[i];
        let rawKey;
        if (item !== null && typeof item === "object") {
          rawKey = item.key;
          if (rawKey === void 0) {
            throw new FilterableListboxConfigError(
              "missing-key",
              `items[${i}] \u7F3A key\uFF08key \u662F\u884C\u8EAB\u4EFD\uFF09`,
              "pass-{key}-items"
            );
          }
        } else {
          throw new FilterableListboxConfigError(
            "invalid-item",
            `items[${i}] \u5FC5\u987B\u662F\u5E26 key \u7684\u5BF9\u8C61\uFF08\u6536\u5230 ${item === null ? "null" : typeof item}\uFF09`,
            "pass-{key}-items"
          );
        }
        const key = normalizeKey2(rawKey, `items[${i}]`);
        if (seen.has(key)) throw new DuplicateKeyError2(key);
        seen.add(key);
        out.push({ key, item, index: i });
      }
      return out;
    }
    __name(normalizeItems2, "normalizeItems");
    function setItems(next) {
      const normalized = normalizeItems2(next);
      entries = normalized;
      const known = new Set(normalized.map((m) => m.key));
      for (const [key, row] of Array.from(rows.entries())) {
        if (known.has(key)) continue;
        rows.delete(key);
        nodeKey.delete(row);
        renderedItem.delete(key);
        if (row.parentNode === listbox) listbox.removeChild(row);
      }
      applyQuery(query);
    }
    __name(setItems, "setItems");
    function destroy() {
      if (destroyed) return;
      destroyed = true;
      try {
        disposer.dispose();
      } catch (err) {
        if (err instanceof DisposeErrors) {
          console.error(`${ERROR_PREFIX2} destroy() \u6709 ${err.errors.length} \u4E2A\u91CA\u653E\u52A8\u4F5C\u5931\u8D25:`, err);
        } else {
          console.error(`${ERROR_PREFIX2} destroy() \u91CA\u653E\u5931\u8D25:`, err);
        }
      }
      rows.clear();
      nodeKey.clear();
      renderedItem.clear();
      matches = [];
      entries = [];
      if (el.parentNode !== null && typeof el.remove === "function") {
        el.remove();
      }
    }
    __name(destroy, "destroy");
    disposer.addEventListener(el, "keydown", onKeyDown);
    disposer.addEventListener(el, "click", onClick);
    disposer.addEventListener(filterInput, "input", onInput);
    disposer.addEventListener(nextEnabled, "click", onNextEnabled);
    disposer.add(() => runFilter.cancel());
    entries = normalizeItems2(options.items);
    if (options.activeKey !== void 0 && options.activeKey !== null) {
      const key = normalizeKey2(options.activeKey, "activeKey");
      if (!entries.some((m) => m.key === key)) {
        throw new FilterableListboxConfigError(
          "unknown-active-key",
          `\u521D\u59CB activeKey "${key}" \u4E0D\u5728 items \u91CC`,
          "pass-a-key-from-items"
        );
      }
      activeKey = key;
    }
    applyQuery("");
    Object.defineProperties(el, {
      setItems: { value: /* @__PURE__ */ __name((next) => {
        assertAlive("setItems");
        setItems(next);
      }, "value"), writable: false, enumerable: true, configurable: false },
      setQuery: { value: /* @__PURE__ */ __name((next) => {
        assertAlive("setQuery");
        setQuery(next);
      }, "value"), writable: false, enumerable: true, configurable: false },
      destroy: { value: destroy, writable: false, enumerable: true, configurable: false },
      query: { get: readQuery, enumerable: true, configurable: false },
      activeKey: { get: readActiveKey, enumerable: true, configurable: false },
      matchCount: { get: readMatchCount, enumerable: true, configurable: false },
      callbackErrorCount: { value: /* @__PURE__ */ __name(() => callbackErrors.length, "value"), writable: false, enumerable: true, configurable: false }
    });
    return el;
  }
  __name(createFilterableListbox, "createFilterableListbox");
  var FILTERABLE_LISTBOX_PARTS = Object.freeze({
    root: '[data-part="filterable-listbox"]',
    filter: '[data-part="filter"]',
    listbox: '[role="listbox"]',
    row: '[role="option"]',
    empty: '[data-part="empty"]',
    notice: '[data-part="pick-notice"]',
    noticeReason: '[data-part="notice-reason"]',
    nextEnabled: '[data-action="next-enabled"]'
  });

  // src/components/form-row.ts
  var ROOT_CLASS2 = "petui-form-row";
  var LABEL_CLASS2 = "petui-form-row__label";
  var CONTROL_CLASS = "petui-form-row__control";
  var OUTPUT_CLASS = "petui-form-row__output";
  var DEFAULT_LABEL_TRACK = "var(--space-140)";
  var PX_ANCHOR_TOKEN2 = "var(--space-1)";
  var TOKEN_NAME_RE3 = /^--[a-z0-9-]+$/;
  var BARE_PX_RE = /^(\d+(?:\.\d+)?)(?:px)?$/;
  var SAFE_TRACK_RE = /^[a-zA-Z0-9\s.,%()/*+-]+$/;
  var LABELABLE_TAGS = /* @__PURE__ */ new Set(["INPUT", "SELECT", "TEXTAREA", "BUTTON", "METER", "OUTPUT", "PROGRESS"]);
  var instanceSeq3 = 0;
  function nextId(doc) {
    for (let guard = 0; guard < 1e4; guard += 1) {
      instanceSeq3 += 1;
      const id = "petui-form-row-" + instanceSeq3;
      const taken = typeof doc.getElementById === "function" ? doc.getElementById(id) : null;
      if (!taken) return id;
    }
    instanceSeq3 += 1;
    return "petui-form-row-" + instanceSeq3;
  }
  __name(nextId, "nextId");
  function isElement2(node) {
    return !!node && typeof node === "object" && node.nodeType === 1;
  }
  __name(isElement2, "isElement");
  function isLabelable(el) {
    if (!LABELABLE_TAGS.has(el.tagName)) return false;
    if (el.tagName !== "INPUT") return true;
    const type = (el.getAttribute("type") || "").toLowerCase();
    return type !== "hidden";
  }
  __name(isLabelable, "isLabelable");
  function resolveLabelTrack(labelWidth) {
    if (labelWidth === void 0 || labelWidth === null) return DEFAULT_LABEL_TRACK;
    if (typeof labelWidth !== "string") {
      console.warn("[PetUI form-row] labelWidth \u5FC5\u987B\u662F\u5B57\u7B26\u4E32\uFF08\u4EE4\u724C\u540D\u6216 CSS \u8F68\u9053\u8868\u8FBE\u5F0F\uFF09\uFF0C\u5DF2\u9000\u56DE\u4EE4\u724C\u9ED8\u8BA4");
      return DEFAULT_LABEL_TRACK;
    }
    const raw = labelWidth.trim();
    if (raw === "") return DEFAULT_LABEL_TRACK;
    if (TOKEN_NAME_RE3.test(raw)) return "var(" + raw + ")";
    const bare = BARE_PX_RE.exec(raw);
    if (bare) return "calc(" + PX_ANCHOR_TOKEN2 + " * " + bare[1] + ")";
    if (SAFE_TRACK_RE.test(raw)) return raw;
    console.warn("[PetUI form-row] labelWidth \u542B\u4E0D\u5B89\u5168\u7684\u5B57\u7B26\uFF0C\u5DF2\u9000\u56DE\u4EE4\u724C\u9ED8\u8BA4\uFF1A" + JSON.stringify(raw));
    return DEFAULT_LABEL_TRACK;
  }
  __name(resolveLabelTrack, "resolveLabelTrack");
  function isEmptyLabel(text) {
    return typeof text !== "string" || text.trim() === "";
  }
  __name(isEmptyLabel, "isEmptyLabel");
  function createFormRow(options) {
    const doc = document;
    const disposer = createDisposer();
    let currentLabel = isEmptyLabel(options.label) ? "" : String(options.label);
    let currentTrack = resolveLabelTrack(options.labelWidth);
    const controlNode = options.control;
    const outputNode = options.output;
    const controlEl = isElement2(controlNode) ? controlNode : null;
    let controlId = null;
    if (controlEl) {
      const existing = controlEl.getAttribute("id");
      controlId = existing && existing.trim() !== "" ? existing : nextId(doc);
      if (controlId !== existing) controlEl.setAttribute("id", controlId);
    }
    const labelId = nextId(doc);
    const el = doc.createElement("div");
    el.className = ROOT_CLASS2;
    const controlSlot = doc.createElement("div");
    controlSlot.className = CONTROL_CLASS;
    controlSlot.appendChild(controlNode);
    let outputSlot = null;
    if (outputNode !== void 0) {
      outputSlot = doc.createElement("div");
      outputSlot.className = OUTPUT_CLASS;
      outputSlot.setAttribute("data-role", "output");
      outputSlot.appendChild(outputNode);
    }
    const labelEl = doc.createElement("label");
    labelEl.className = LABEL_CLASS2;
    labelEl.id = labelId;
    labelEl.setAttribute("data-role", "label");
    labelEl.style.cssText = [
      "font-size:var(--font-size-14)",
      "color:var(--color-text-secondary)",
      "min-width:0"
    ].join(";");
    let labelMounted = false;
    disposer.addEventListener(labelEl, "click", () => {
      if (!controlEl || typeof controlEl.focus !== "function") return;
      controlEl.focus();
    });
    const applyTracks = /* @__PURE__ */ __name(() => {
      const tracks = [];
      if (labelMounted) tracks.push(currentTrack);
      tracks.push("minmax(0, 1fr)");
      if (outputSlot) tracks.push("auto");
      el.style.gridTemplateColumns = tracks.join(" ");
    }, "applyTracks");
    const link = /* @__PURE__ */ __name(() => {
      if (controlId && controlEl) {
        if (isLabelable(controlEl)) {
          labelEl.setAttribute("for", controlId);
          controlEl.removeAttribute("aria-labelledby");
        } else {
          labelEl.removeAttribute("for");
          controlEl.setAttribute("aria-labelledby", labelId);
        }
        return;
      }
      console.warn("[PetUI form-row] control \u4E0D\u662F Element\uFF0C\u65E0\u6CD5\u5EFA\u7ACB label\u2194control \u5173\u8054\uFF08K9\uFF09");
    }, "link");
    const mountLabel = /* @__PURE__ */ __name((text) => {
      labelEl.textContent = text;
      if (!labelMounted) {
        el.insertBefore(labelEl, controlSlot);
        labelMounted = true;
      }
      link();
      applyTracks();
    }, "mountLabel");
    const unmountLabel = /* @__PURE__ */ __name(() => {
      if (!labelMounted) return;
      el.removeChild(labelEl);
      labelMounted = false;
      if (controlEl) controlEl.removeAttribute("aria-labelledby");
      applyTracks();
    }, "unmountLabel");
    el.style.cssText = [
      "display:grid",
      "align-items:center",
      "gap:var(--space-8) var(--space-12)",
      "min-width:0"
    ].join(";");
    controlSlot.style.cssText = ["display:flex", "align-items:center", "min-width:0"].join(";");
    if (outputSlot) {
      outputSlot.style.cssText = [
        "display:flex",
        "align-items:center",
        "justify-content:flex-end",
        "gap:var(--space-4)",
        "font-size:var(--font-size-12)",
        "color:var(--color-text-muted)",
        "min-width:0"
      ].join(";");
    }
    applyTracks();
    el.appendChild(controlSlot);
    if (outputSlot) el.appendChild(outputSlot);
    if (currentLabel !== "") mountLabel(currentLabel);
    disposer.add(() => {
      const removable = el;
      if (typeof removable.remove === "function") {
        removable.remove();
      } else if (removable.parentNode && typeof removable.parentNode.removeChild === "function") {
        removable.parentNode.removeChild(el);
      }
    });
    Object.defineProperties(el, {
      setLabel: {
        value: /* @__PURE__ */ __name((text) => {
          if (disposer.disposed) return;
          const next = isEmptyLabel(text) ? "" : String(text);
          if (next === currentLabel) return;
          currentLabel = next;
          if (next === "") {
            unmountLabel();
            return;
          }
          if (labelMounted) {
            labelEl.textContent = next;
            link();
            return;
          }
          mountLabel(next);
        }, "value"),
        writable: true,
        configurable: true
      },
      destroy: {
        value: /* @__PURE__ */ __name(() => {
          if (disposer.disposed) return;
          disposer.dispose();
        }, "value"),
        writable: true,
        configurable: true
      },
      label: { get: /* @__PURE__ */ __name(() => currentLabel, "get"), configurable: true },
      labelWidth: {
        get: /* @__PURE__ */ __name(() => options.labelWidth === void 0 ? void 0 : currentTrack, "get"),
        configurable: true
      },
      control: { get: /* @__PURE__ */ __name(() => controlNode, "get"), configurable: true },
      output: { get: /* @__PURE__ */ __name(() => outputNode, "get"), configurable: true }
    });
    return el;
  }
  __name(createFormRow, "createFormRow");

  // src/components/validated-field.ts
  var DEFAULT_ESCAPE_ACTION = "edit-input-to-valid-value";
  var DEFAULT_ESCAPE_TEXT = "\u51FA\u8DEF\uFF1A\u628A\u8F93\u5165\u6539\u6210\u5408\u6CD5\u503C\uFF0C\u4E3B\u64CD\u4F5C\u4F1A\u81EA\u52A8\u6062\u590D\u53EF\u7528";
  function describeBlockedState(result) {
    if (result.ok) return null;
    const hint = result.hint === void 0 || result.hint === null ? "" : String(result.hint);
    return {
      reason: result.code + ": " + result.message,
      escapeAction: hint === "" ? DEFAULT_ESCAPE_ACTION : "follow-hint",
      escapeText: hint === "" ? DEFAULT_ESCAPE_TEXT : "\u51FA\u8DEF\uFF1A" + hint
    };
  }
  __name(describeBlockedState, "describeBlockedState");
  var ROOT_CLASS3 = "petui-validated-field";
  var INPUT_CLASS = "petui-validated-field__input";
  var MESSAGE_CLASS = "petui-validated-field__message";
  var ESCAPE_CLASS = "petui-validated-field__escape";
  var PENDING_CLASS = "petui-validated-field__pending";
  var APPLY_CLASS = "petui-validated-field__apply";
  var ACTION_CLASS = "petui-validated-field__action";
  var STYLE_ID2 = "petui-validated-field-styles";
  var STYLE_RULES = [
    "." + ROOT_CLASS3 + "{display:flex;flex-direction:column;gap:var(--space-6)}",
    "." + INPUT_CLASS + "{padding:var(--space-8) var(--space-10);border-style:solid;border-width:var(--space-1);border-color:var(--color-border);border-radius:var(--border-radius-md);font-size:var(--font-size-14);color:var(--color-text);background-color:var(--color-bg);outline:none}",
    "." + INPUT_CLASS + '[aria-invalid="true"]{border-color:var(--color-danger)}',
    "." + MESSAGE_CLASS + "{margin:0;font-size:var(--font-size-12);color:var(--color-text-muted)}",
    "." + MESSAGE_CLASS + '[data-state="error"]{color:var(--color-danger)}',
    "." + ESCAPE_CLASS + "{margin:0;font-size:var(--font-size-12);color:var(--color-text-secondary)}",
    "." + PENDING_CLASS + "{margin:0;font-size:var(--font-size-12);color:var(--color-warning)}",
    "." + PENDING_CLASS + '[data-pending="none"]{display:none}',
    "." + ACTION_CLASS + "{align-self:flex-start;padding:var(--space-6) var(--space-12);border-style:solid;border-width:var(--space-1);border-color:var(--color-primary);border-radius:var(--border-radius-md);font-size:var(--font-size-13);color:var(--color-bg);background-color:var(--color-primary)}",
    "." + ACTION_CLASS + "[disabled]{color:var(--color-text-muted);background-color:var(--color-bg-light);border-color:var(--color-border)}",
    "." + APPLY_CLASS + "{align-self:flex-start;padding:var(--space-4) var(--space-10);border-style:solid;border-width:var(--space-1);border-color:var(--color-warning);border-radius:var(--border-radius-md);font-size:var(--font-size-12);color:var(--color-bg);background-color:var(--color-warning)}"
  ];
  var autoDescribeSeq = 0;
  function toRaw2(value) {
    if (value === null || value === void 0) return "";
    return String(value);
  }
  __name(toRaw2, "toRaw");
  function injectFieldStyles(doc) {
    if (typeof doc.getElementById === "function" && doc.getElementById(STYLE_ID2) !== null) return;
    const style = doc.createElement("style");
    style.id = STYLE_ID2;
    style.textContent = STYLE_RULES.join("\n");
    const head = doc.head || doc.documentElement;
    if (head && typeof head.appendChild === "function") head.appendChild(style);
  }
  __name(injectFieldStyles, "injectFieldStyles");
  function createValidatedField(options) {
    if (options === null || options === void 0) {
      throw new TypeError("[PetUI validated-field] \u9700\u8981\u4E00\u4E2A options \u5BF9\u8C61");
    }
    if (typeof options.validate !== "function") {
      throw new TypeError("[PetUI validated-field] validate \u5FC5\u987B\u662F\u51FD\u6570\uFF08\u672C\u6A21\u5757\u4E0D\u81EA\u5E26\u4E1A\u52A1\u89C4\u5219\uFF09");
    }
    if (typeof options.onValid !== "function") {
      throw new TypeError("[PetUI validated-field] onValid \u5FC5\u987B\u662F\u51FD\u6570");
    }
    if (typeof options.actionLabel !== "string" || options.actionLabel.length === 0) {
      throw new RangeError("[PetUI validated-field] actionLabel \u4E0D\u80FD\u4E3A\u7A7A\uFF08\u4E3B\u64CD\u4F5C\u9700\u8981\u975E\u7A7A\u6587\u6848\uFF09");
    }
    const doc = document;
    injectFieldStyles(doc);
    const disposer = createDisposer();
    let currentRaw = toRaw2(options.value);
    let currentResult = { ok: false, code: "Unvalidated", message: "\u5C1A\u672A\u6821\u9A8C", hint: "" };
    let queuedPending = null;
    let flushScheduled = false;
    const root = doc.createElement("div");
    root.className = ROOT_CLASS3;
    const inputEl = doc.createElement("input");
    inputEl.type = "text";
    inputEl.className = INPUT_CLASS;
    inputEl.value = currentRaw;
    root.appendChild(inputEl);
    const messageId = options.describeId === void 0 || options.describeId === null || String(options.describeId) === "" ? "petui-validated-field-msg-" + String(++autoDescribeSeq) : String(options.describeId);
    const messageEl = doc.createElement("p");
    messageEl.className = MESSAGE_CLASS;
    messageEl.id = messageId;
    messageEl.setAttribute("aria-live", "polite");
    messageEl.setAttribute("data-state", "ok");
    root.appendChild(messageEl);
    const escapeEl = doc.createElement("p");
    escapeEl.className = ESCAPE_CLASS;
    escapeEl.id = messageId + "-escape";
    root.appendChild(escapeEl);
    const pendingEl = doc.createElement("p");
    pendingEl.className = PENDING_CLASS;
    pendingEl.setAttribute("data-pending", "none");
    root.appendChild(pendingEl);
    const applyBtn = doc.createElement("button");
    applyBtn.type = "button";
    applyBtn.className = APPLY_CLASS;
    applyBtn.textContent = "\u786E\u8BA4\u5F85\u786E\u8BA4\u503C\uFF08\u4FDD\u7559\u5728\u8F93\u5165\u6846\uFF09";
    root.appendChild(applyBtn);
    const actionBtn = doc.createElement("button");
    actionBtn.type = "button";
    actionBtn.className = ACTION_CLASS;
    actionBtn.textContent = options.actionLabel;
    root.appendChild(actionBtn);
    const runValidate = /* @__PURE__ */ __name((raw) => {
      let produced;
      try {
        produced = options.validate(raw);
      } catch (err) {
        console.error("[PetUI validated-field] \u6821\u9A8C\u5668\u629B\u9519\uFF0C\u5DF2\u964D\u7EA7\u4E3A ValidatorError:", err);
        return { ok: false, code: "ValidatorError", message: "\u6821\u9A8C\u5668\u5F02\u5E38", hint: "\u4FEE\u6B63\u8F93\u5165\u540E\u91CD\u8BD5" };
      }
      if (produced !== null && typeof produced === "object" && produced.ok === true) {
        const value = produced.value;
        if (typeof value === "number" || typeof value === "string") return { ok: true, value };
        console.error("[PetUI validated-field] validate \u8FD4\u56DE {ok:true} \u4F46 value \u975E number|string:", produced);
        return { ok: false, code: "ValidatorError", message: "\u6821\u9A8C\u5668\u5F02\u5E38", hint: "\u4FEE\u6B63\u8F93\u5165\u540E\u91CD\u8BD5" };
      }
      if (produced !== null && typeof produced === "object" && produced.ok === false) {
        const bad = produced;
        const result = {
          ok: false,
          code: bad.code === void 0 || bad.code === null ? "UnknownError" : String(bad.code),
          message: bad.message === void 0 || bad.message === null ? "\u6821\u9A8C\u672A\u901A\u8FC7" : String(bad.message)
        };
        if (bad.hint !== void 0 && bad.hint !== null) result.hint = String(bad.hint);
        return result;
      }
      console.error("[PetUI validated-field] validate \u8FD4\u56DE\u975E ValidationResult \u5F62\u6001:", produced);
      return { ok: false, code: "ValidatorError", message: "\u6821\u9A8C\u5668\u5F02\u5E38", hint: "\u4FEE\u6B63\u8F93\u5165\u540E\u91CD\u8BD5" };
    }, "runValidate");
    const render = /* @__PURE__ */ __name(() => {
      const result = runValidate(currentRaw);
      currentResult = result;
      const blocked = describeBlockedState(result);
      if (inputEl.value !== currentRaw) inputEl.value = currentRaw;
      inputEl.setAttribute("aria-invalid", result.ok ? "false" : "true");
      inputEl.setAttribute("aria-describedby", messageEl.id);
      if (result.ok) {
        messageEl.textContent = "";
        messageEl.setAttribute("data-state", "ok");
        messageEl.removeAttribute("data-error-code");
      } else {
        messageEl.textContent = result.message + "\uFF08\u9519\u8BEF\u7801\uFF1A" + result.code + "\uFF09";
        messageEl.setAttribute("data-state", "error");
        messageEl.setAttribute("data-error-code", result.code);
      }
      if (blocked === null) {
        escapeEl.textContent = "";
        escapeEl.removeAttribute("data-state");
        escapeEl.removeAttribute("data-escape");
      } else {
        escapeEl.textContent = blocked.escapeText;
        escapeEl.setAttribute("data-state", "error");
        escapeEl.setAttribute("data-escape", blocked.escapeAction);
      }
      actionBtn.disabled = !result.ok;
      actionBtn.setAttribute("data-blocked", result.ok ? "false" : "true");
      if (result.ok) {
        actionBtn.removeAttribute("title");
        actionBtn.removeAttribute("data-blocked-reason");
      } else {
        const reason = result.code + ": " + result.message;
        actionBtn.setAttribute("title", reason + "\uFF1B" + (blocked === null ? DEFAULT_ESCAPE_TEXT : blocked.escapeText));
        actionBtn.setAttribute("data-blocked-reason", result.code);
      }
      root.setAttribute("data-valid", result.ok ? "true" : "false");
      root.setAttribute("data-error-code", result.ok ? "" : result.code);
      root.setAttribute("data-escape", blocked === null ? "" : blocked.escapeAction);
      return result;
    }, "render");
    const flushPending = /* @__PURE__ */ __name(() => {
      flushScheduled = false;
      if (disposer.disposed) return;
      const job = queuedPending;
      if (job === null) return;
      queuedPending = null;
      currentRaw = job.raw;
      inputEl.value = job.raw;
      const result = render();
      pendingEl.setAttribute("data-pending", "applied");
      pendingEl.setAttribute("data-pending-value", job.raw);
      pendingEl.setAttribute("data-previous-value", job.previous);
      pendingEl.setAttribute("data-pending-applied", "true");
      pendingEl.textContent = "\u5F85\u786E\u8BA4\u65B0\u503C\uFF1A" + job.raw + "\uFF08\u5BF9\u7167\u65E7\u503C\uFF1A" + job.previous + "\uFF09\u2014\u2014 " + (result.ok ? "\u6821\u9A8C\u901A\u8FC7\uFF0C\u53EF\u63D0\u4EA4" : "\u6821\u9A8C\u672A\u901A\u8FC7\uFF08" + result.code + "\uFF09\uFF0C\u8BF7\u5148\u4FEE\u6B63\u6216\u786E\u8BA4");
    }, "flushPending");
    const scheduleFlush = /* @__PURE__ */ __name(() => {
      if (flushScheduled || disposer.disposed) return;
      flushScheduled = true;
      disposer.setTimeout(flushPending, 0);
    }, "scheduleFlush");
    const submit = /* @__PURE__ */ __name(() => {
      const result = render();
      if (!result.ok) {
        console.warn("[PetUI validated-field] \u975E\u6CD5\u503C\u88AB\u63D0\u4EA4\uFF0C\u5DF2\u62D2\u7EDD\uFF1A" + result.code);
        return;
      }
      if (pendingEl.getAttribute("data-pending") === "applied") {
        pendingEl.setAttribute("data-pending", "confirmed");
      }
      options.onValid(String(result.value));
    }, "submit");
    const onInputEvent = /* @__PURE__ */ __name(() => {
      currentRaw = toRaw2(inputEl.value);
      render();
    }, "onInputEvent");
    const onActionClick = /* @__PURE__ */ __name(() => {
      submit();
    }, "onActionClick");
    const onInputKeydown = /* @__PURE__ */ __name((ev) => {
      const key = ev.key;
      if (key === "Enter") {
        ev.preventDefault();
        submit();
      }
    }, "onInputKeydown");
    const onApplyClick = /* @__PURE__ */ __name(() => {
      if (pendingEl.getAttribute("data-pending") === "applied") {
        pendingEl.setAttribute("data-pending", "confirmed");
        pendingEl.textContent = (pendingEl.textContent || "") + "\uFF08\u5DF2\u786E\u8BA4\uFF0C\u503C\u4FDD\u7559\u5728\u8F93\u5165\u6846\uFF09";
      }
    }, "onApplyClick");
    disposer.addEventListener(inputEl, "input", onInputEvent);
    disposer.addEventListener(inputEl, "keydown", onInputKeydown);
    disposer.addEventListener(actionBtn, "click", onActionClick);
    disposer.addEventListener(applyBtn, "click", onApplyClick);
    disposer.add(() => {
      const removable = root;
      if (typeof removable.remove === "function") {
        removable.remove();
      } else if (removable.parentNode && typeof removable.parentNode.removeChild === "function") {
        removable.parentNode.removeChild(root);
      }
    });
    render();
    const enqueuePending = /* @__PURE__ */ __name((raw) => {
      if (disposer.disposed) {
        console.warn("[PetUI validated-field] destroy() \u4E4B\u540E setPending \u88AB\u5FFD\u7565");
        return;
      }
      const job = { raw: toRaw2(raw), previous: currentRaw };
      queuedPending = job;
      pendingEl.setAttribute("data-pending", "queued");
      pendingEl.setAttribute("data-pending-value", job.raw);
      pendingEl.setAttribute("data-previous-value", job.previous);
      pendingEl.textContent = "\u5F85\u786E\u8BA4\u65B0\u503C\uFF1A" + job.raw + "\uFF08\u5BF9\u7167\u5F53\u524D\u503C\uFF1A" + job.previous + "\uFF09\u2014\u2014 \u6E32\u67D3\u5B8C\u6210\u540E\u5E94\u7528\u4E00\u6B21";
      scheduleFlush();
    }, "enqueuePending");
    Object.defineProperties(root, {
      setValue: {
        value: /* @__PURE__ */ __name((raw) => {
          if (disposer.disposed) {
            console.warn("[PetUI validated-field] destroy() \u4E4B\u540E setValue \u88AB\u5FFD\u7565");
            return;
          }
          currentRaw = toRaw2(raw);
          inputEl.value = currentRaw;
          render();
        }, "value"),
        writable: true,
        configurable: true
      },
      getValue: {
        value: /* @__PURE__ */ __name(() => currentRaw, "value"),
        writable: true,
        configurable: true
      },
      isValid: {
        value: /* @__PURE__ */ __name(() => currentResult.ok, "value"),
        writable: true,
        configurable: true
      },
      setPending: {
        value: /* @__PURE__ */ __name((raw) => {
          enqueuePending(raw);
        }, "value"),
        writable: true,
        configurable: true
      },
      destroy: {
        value: /* @__PURE__ */ __name(() => {
          if (disposer.disposed) return;
          queuedPending = null;
          disposer.dispose();
        }, "value"),
        writable: true,
        configurable: true
      }
    });
    if (options.pendingValue !== void 0 && options.pendingValue !== null) {
      enqueuePending(options.pendingValue);
    }
    return root;
  }
  __name(createValidatedField, "createValidatedField");

  // src/components/property-grid.ts
  var DEFAULT_READONLY_REASON2 = "\u8BE5\u5B57\u6BB5\u7531 schema \u58F0\u660E\u4E3A\u53EA\u8BFB";
  var NOT_DELETABLE_REASON = "\u8BE5\u5B57\u6BB5\u7531 schema \u58F0\u660E\u4E3A\u4E0D\u53EF\u5220\u9664";
  var _PropertyGridDisposedError = class _PropertyGridDisposedError extends Error {
    constructor(action) {
      super(`[PetUI property-grid] \u5DF2 destroy()\uFF0C\u4E0D\u80FD\u518D\u8C03\u7528 ${action}()`);
      this.name = "PropertyGridDisposedError";
      this.action = action;
    }
  };
  __name(_PropertyGridDisposedError, "PropertyGridDisposedError");
  var PropertyGridDisposedError = _PropertyGridDisposedError;
  var FORBIDDEN_SEGMENTS = /* @__PURE__ */ new Set(["__proto__", "constructor", "prototype"]);
  function splitPath(path) {
    const raw = String(path === null || path === void 0 ? "" : path).trim();
    if (raw.length === 0) return null;
    const parts = [];
    for (const seg of raw.split(".")) {
      const s = seg.trim();
      if (s.length === 0) continue;
      if (FORBIDDEN_SEGMENTS.has(s)) return null;
      parts.push(s);
    }
    return parts.length === 0 ? null : parts;
  }
  __name(splitPath, "splitPath");
  function cloneValue(value) {
    if (value === null || typeof value !== "object") return value;
    try {
      return JSON.parse(JSON.stringify(value));
    } catch {
      return value;
    }
  }
  __name(cloneValue, "cloneValue");
  function valueEquals(a, b) {
    if (Object.is(a, b)) return true;
    if (a === null || b === null || typeof a !== "object" || typeof b !== "object") return false;
    try {
      return JSON.stringify(a) === JSON.stringify(b);
    } catch {
      return false;
    }
  }
  __name(valueEquals, "valueEquals");
  function readAt(source, path, parts) {
    if (Object.prototype.hasOwnProperty.call(source, path)) return source[path];
    let cur = source;
    for (const seg of parts) {
      if (cur === null || typeof cur !== "object") return void 0;
      cur = cur[seg];
    }
    return cur;
  }
  __name(readAt, "readAt");
  function writeAt(target, path, parts, value) {
    if (parts.length === 1 || Object.prototype.hasOwnProperty.call(target, path)) {
      target[path] = value;
      return;
    }
    let cur = target;
    for (let i = 0; i < parts.length - 1; i += 1) {
      const seg = parts[i];
      const next = cur[seg];
      if (next === null || typeof next !== "object" || Array.isArray(next)) {
        cur[seg] = {};
        cur = cur[seg];
      } else {
        cur = next;
      }
    }
    cur[parts[parts.length - 1]] = value;
  }
  __name(writeAt, "writeAt");
  function cloneStore(source) {
    const out = {};
    if (source === null || source === void 0) return out;
    for (const key of Object.keys(source)) out[key] = cloneValue(source[key]);
    return out;
  }
  __name(cloneStore, "cloneStore");
  function effectiveValue(type, raw) {
    if (type === "boolean") return raw === true;
    if (type === "number") {
      if (raw === null || raw === void 0 || raw === "") return null;
      const n = Number(raw);
      return Number.isFinite(n) ? n : null;
    }
    return raw === null || raw === void 0 ? "" : String(raw);
  }
  __name(effectiveValue, "effectiveValue");
  function parseNumberText(raw) {
    const t = String(raw === null || raw === void 0 ? "" : raw).trim();
    if (t.length === 0) return null;
    const n = Number(t);
    return Number.isFinite(n) ? n : null;
  }
  __name(parseNumberText, "parseNumberText");
  var instanceSeq4 = 0;
  function normalizeFields(schema) {
    const out = [];
    const list = Array.isArray(schema) ? schema : [];
    for (const raw of list) {
      if (raw === null || raw === void 0 || typeof raw !== "object") {
        console.warn("[PetUI property-grid] \u5FFD\u7565\u975E\u6CD5\u5B57\u6BB5\uFF08\u4E0D\u662F\u5BF9\u8C61\uFF09:", raw);
        continue;
      }
      const parts = splitPath(raw.path);
      if (parts === null) {
        console.warn("[PetUI property-grid] \u5FFD\u7565\u8DEF\u5F84\u4E0D\u53EF\u7528\u7684\u5B57\u6BB5:", raw.path);
        continue;
      }
      const path = String(raw.path).trim();
      const type = raw.type === "number" || raw.type === "boolean" ? raw.type : "text";
      const isReadonly2 = raw.readonly === true;
      out.push({
        path,
        parts,
        label: raw.label === void 0 || raw.label === null || String(raw.label).length === 0 ? path : String(raw.label),
        type,
        readonly: isReadonly2,
        readonlyReason: isReadonly2 ? raw.readonlyReason === void 0 || raw.readonlyReason === null || String(raw.readonlyReason).length === 0 ? DEFAULT_READONLY_REASON2 : String(raw.readonlyReason) : "",
        deletable: raw.deletable !== false,
        placeholder: raw.placeholder === void 0 || raw.placeholder === null ? "" : String(raw.placeholder),
        help: raw.help === void 0 || raw.help === null ? "" : String(raw.help),
        helpHtml: raw.helpHtml === void 0 || raw.helpHtml === null ? "" : String(raw.helpHtml)
      });
    }
    return out;
  }
  __name(normalizeFields, "normalizeFields");
  function createPropertyGrid(schema = [], values = {}, handlers = {}, onDelete) {
    const callbacks = typeof handlers === "function" ? { onChange: handlers, onDelete: onDelete === void 0 ? null : onDelete } : { onChange: handlers.onChange ?? null, onDelete: handlers.onDelete ?? null };
    const disposer = createDisposer();
    const uid = `petui-pg-${instanceSeq4 += 1}`;
    const el = document.createElement("div");
    el.className = "petui-property-grid";
    el.setAttribute("data-petui", "property-grid");
    el.style.cssText = [
      "display:block",
      "padding:var(--space-2)",
      "border:var(--space-1) solid var(--color-border)",
      "border-radius:var(--border-radius-md)",
      "background:var(--color-bg-light)",
      "color:var(--color-text)",
      "font-size:var(--font-size-14)",
      "box-sizing:border-box"
    ].join(";");
    const status = document.createElement("div");
    status.className = "petui-property-grid-status";
    status.setAttribute("role", "status");
    status.setAttribute("aria-live", "polite");
    status.style.cssText = [
      "color:var(--color-text-muted)",
      "font-size:var(--font-size-12)",
      "padding:var(--space-2) var(--space-4)"
    ].join(";");
    const empty = document.createElement("div");
    empty.className = "petui-property-grid-empty";
    empty.setAttribute("role", "note");
    empty.style.cssText = [
      "color:var(--color-text-muted)",
      "padding:var(--space-4)",
      "text-align:center"
    ].join(";");
    empty.textContent = "\uFF08\u65E0\u53EF\u7F16\u8F91\u5B57\u6BB5\uFF1Aschema \u4E3A\u7A7A\uFF09";
    const rows = document.createElement("div");
    rows.className = "petui-property-grid-rows";
    rows.style.cssText = "display:block";
    el.appendChild(status);
    el.appendChild(empty);
    el.appendChild(rows);
    let fields = normalizeFields(schema);
    let store = cloneStore(values);
    let baseline = cloneStore(values);
    let disposed = false;
    const byPath = /* @__PURE__ */ new Map();
    function clearChildren5(host) {
      while (host.firstChild) host.removeChild(host.firstChild);
    }
    __name(clearChildren5, "clearChildren");
    function effectiveOf(field, source) {
      return effectiveValue(field.type, readAt(source, field.path, field.parts));
    }
    __name(effectiveOf, "effectiveOf");
    function computeDirty() {
      const out = {};
      for (const field of fields) {
        const cur = effectiveOf(field, store);
        if (!valueEquals(cur, effectiveOf(field, baseline))) out[field.path] = cur;
      }
      return out;
    }
    __name(computeDirty, "computeDirty");
    function computeValues() {
      const out = {};
      for (const field of fields) out[field.path] = effectiveOf(field, store);
      return out;
    }
    __name(computeValues, "computeValues");
    function refreshStatus() {
      const n = Object.keys(computeDirty()).length;
      if (fields.length === 0) status.textContent = "\u65E0\u53EF\u7F16\u8F91\u5B57\u6BB5";
      else if (n === 0) status.textContent = `${fields.length} \u4E2A\u5B57\u6BB5\uFF0C\u672A\u4FEE\u6539`;
      else status.textContent = `${fields.length} \u4E2A\u5B57\u6BB5\uFF0C\u5DF2\u4FEE\u6539 ${n} \u4E2A`;
    }
    __name(refreshStatus, "refreshStatus");
    function appendReadonlyReason(host, field) {
      const note = document.createElement("span");
      note.className = "petui-property-grid-readonly";
      note.setAttribute("data-readonly-reason", field.readonlyReason);
      note.setAttribute("role", "note");
      note.textContent = `\u53EA\u8BFB\uFF1A${field.readonlyReason}`;
      note.style.cssText = "color:var(--color-text-secondary);font-size:var(--font-size-12);padding-left:var(--space-2)";
      host.appendChild(note);
    }
    __name(appendReadonlyReason, "appendReadonlyReason");
    function appendHelp(host, field) {
      if (field.helpHtml.length === 0 && field.help.length === 0) return;
      const help = document.createElement("div");
      help.className = "petui-property-grid-help";
      help.setAttribute("data-help-kind", field.helpHtml.length > 0 ? "html" : "text");
      help.style.cssText = "color:var(--color-text-muted);font-size:var(--font-size-12);padding-top:var(--space-2)";
      if (field.helpHtml.length > 0) {
        setSafeHTML(help, field.helpHtml);
      } else {
        help.textContent = field.help;
      }
      host.appendChild(help);
    }
    __name(appendHelp, "appendHelp");
    function buildControl(field, index) {
      const input = document.createElement("input");
      const isBoolean = field.type === "boolean";
      input.className = isBoolean ? "petui-property-grid-checkbox" : "petui-property-grid-input";
      input.setAttribute("type", isBoolean ? "checkbox" : field.type === "number" ? "number" : "text");
      input.setAttribute("data-path", field.path);
      input.setAttribute("id", `${uid}-field-${index}`);
      input.style.cssText = isBoolean ? "width:auto;margin:0" : [
        "width:100%",
        "box-sizing:border-box",
        "padding:var(--space-2)",
        "border:var(--space-1) solid var(--color-border)",
        "border-radius:var(--border-radius-sm)",
        "background:var(--color-bg)",
        "color:var(--color-text)",
        "font-size:var(--font-size-14)"
      ].join(";");
      const current = effectiveOf(field, store);
      if (isBoolean) {
        input.checked = current === true;
      } else {
        input.value = current === null ? "" : String(current);
        if (field.placeholder.length > 0) input.setAttribute("placeholder", field.placeholder);
      }
      if (field.readonly) {
        input.disabled = true;
        input.setAttribute("aria-disabled", "true");
        input.setAttribute("title", field.readonlyReason);
      }
      return input;
    }
    __name(buildControl, "buildControl");
    function buildDelete(field) {
      const btn = document.createElement("button");
      btn.className = "petui-property-grid-delete";
      btn.setAttribute("type", "button");
      btn.setAttribute("data-delete-path", field.path);
      btn.textContent = "\u5220\u9664";
      btn.style.cssText = [
        "padding:var(--space-2) var(--space-4)",
        "border:var(--space-1) solid var(--color-border)",
        "border-radius:var(--border-radius-sm)",
        "background:var(--color-bg)",
        "color:var(--color-danger)",
        "font-size:var(--font-size-12)",
        "cursor:pointer"
      ].join(";");
      if (field.readonly) {
        btn.disabled = true;
        btn.setAttribute("aria-disabled", "true");
        btn.setAttribute("title", field.readonlyReason);
      } else if (!field.deletable) {
        btn.disabled = true;
        btn.setAttribute("aria-disabled", "true");
        btn.setAttribute("title", NOT_DELETABLE_REASON);
      }
      return btn;
    }
    __name(buildDelete, "buildDelete");
    function render() {
      clearChildren5(rows);
      byPath.clear();
      for (const field of fields) byPath.set(field.path, field);
      if (fields.length === 0) {
        empty.hidden = false;
        empty.style.display = "block";
        refreshStatus();
        return;
      }
      empty.hidden = true;
      empty.style.display = "none";
      for (let index = 0; index < fields.length; index += 1) {
        const field = fields[index];
        const row = document.createElement("div");
        row.className = "petui-property-grid-row";
        row.setAttribute("data-path", field.path);
        row.setAttribute("data-readonly", field.readonly ? "true" : "false");
        row.style.cssText = [
          "display:flex",
          "align-items:flex-start",
          "gap:var(--space-4)",
          "padding:var(--space-2) 0",
          "border-bottom:var(--space-1) solid var(--color-border)"
        ].join(";");
        const caption = document.createElement("div");
        caption.className = "petui-property-grid-caption";
        caption.style.cssText = "flex:1;min-width:0";
        const label = document.createElement("label");
        label.className = "petui-property-grid-label";
        label.setAttribute("for", `${uid}-field-${index}`);
        label.textContent = field.label;
        label.style.cssText = "display:block;color:var(--color-text);padding-bottom:var(--space-2)";
        const pathNote = document.createElement("span");
        pathNote.className = "petui-property-grid-path";
        pathNote.setAttribute("data-field-path", field.path);
        pathNote.textContent = field.path;
        pathNote.style.cssText = "color:var(--color-text-muted);font-size:var(--font-size-12);padding-left:var(--space-2)";
        const control = document.createElement("div");
        control.className = "petui-property-grid-control";
        control.style.cssText = "display:flex;align-items:center;gap:var(--space-2)";
        const input = buildControl(field, index);
        control.appendChild(input);
        if (field.readonly) appendReadonlyReason(control, field);
        caption.appendChild(label);
        caption.appendChild(pathNote);
        caption.appendChild(control);
        appendHelp(caption, field);
        row.appendChild(caption);
        row.appendChild(buildDelete(field));
        rows.appendChild(row);
      }
      refreshStatus();
    }
    __name(render, "render");
    function commit(path, next) {
      const field = byPath.get(path);
      if (field === void 0) return;
      const parts = field.parts;
      const prev = effectiveOf(field, store);
      const value = effectiveValue(field.type, next);
      if (valueEquals(prev, value)) return;
      writeAt(store, path, parts, value);
      refreshStatus();
      const cb = callbacks.onChange;
      if (cb === void 0 || cb === null) return;
      try {
        cb(path, cloneValue(value));
      } catch (err) {
        console.error("[PetUI property-grid] onChange \u56DE\u8C03\u629B\u9519:", err);
      }
    }
    __name(commit, "commit");
    function onInput(ev) {
      if (disposed) return;
      const target = ev.target;
      if (target === null || target === void 0 || typeof target.getAttribute !== "function") return;
      const path = target.getAttribute("data-path");
      if (path === null || path.length === 0) return;
      const field = byPath.get(path);
      if (field === void 0) return;
      if (field.readonly) return;
      if (field.type === "boolean") commit(path, target.checked === true);
      else if (field.type === "number") commit(path, parseNumberText(target.value));
      else commit(path, String(target.value));
    }
    __name(onInput, "onInput");
    function onClick(ev) {
      if (disposed) return;
      const target = ev.target;
      if (target === null || target === void 0 || typeof target.getAttribute !== "function") return;
      const path = target.getAttribute("data-delete-path");
      if (path === null || path.length === 0) return;
      const field = byPath.get(path);
      if (field === void 0) return;
      if (field.readonly || !field.deletable) return;
      const cb = callbacks.onDelete;
      if (cb === void 0 || cb === null) return;
      try {
        cb(path);
      } catch (err) {
        console.error("[PetUI property-grid] onDelete \u56DE\u8C03\u629B\u9519:", err);
      }
    }
    __name(onClick, "onClick");
    disposer.addEventListener(el, "input", onInput);
    disposer.addEventListener(el, "click", onClick);
    function assertAlive(action) {
      if (disposed) throw new PropertyGridDisposedError(action);
    }
    __name(assertAlive, "assertAlive");
    function setSchema(next) {
      assertAlive("setSchema");
      fields = normalizeFields(next);
      render();
    }
    __name(setSchema, "setSchema");
    function setValues(next) {
      assertAlive("setValues");
      store = cloneStore(next);
      baseline = cloneStore(next);
      render();
    }
    __name(setValues, "setValues");
    function getDirty() {
      return Object.freeze(cloneStore(computeDirty()));
    }
    __name(getDirty, "getDirty");
    function destroy() {
      if (disposed) return;
      disposed = true;
      byPath.clear();
      try {
        disposer.dispose();
      } finally {
        if (el.parentNode) el.parentNode.removeChild(el);
      }
    }
    __name(destroy, "destroy");
    render();
    Object.defineProperty(el, "values", {
      get: /* @__PURE__ */ __name(() => Object.freeze(cloneStore(computeValues())), "get"),
      enumerable: true,
      configurable: true
    });
    Object.defineProperty(el, "dirtyPaths", {
      get: /* @__PURE__ */ __name(() => Object.freeze(Object.keys(computeDirty())), "get"),
      enumerable: true,
      configurable: true
    });
    Object.defineProperty(el, "fieldCount", { get: /* @__PURE__ */ __name(() => fields.length, "get"), enumerable: true, configurable: true });
    Object.defineProperty(el, "disposed", { get: /* @__PURE__ */ __name(() => disposed, "get"), enumerable: true, configurable: true });
    const api = Object.assign(el, { setSchema, setValues, getDirty, destroy });
    return api;
  }
  __name(createPropertyGrid, "createPropertyGrid");

  // src/components/dangerous-mode.ts
  var TONE_FOR_DIRECTION = Object.freeze({
    enable: "error",
    disable: "warning"
  });
  var TONE_KEYS2 = Object.freeze(Object.keys(TONE_TOKENS));
  function toneTokensFor2(direction) {
    const key = TONE_FOR_DIRECTION[direction];
    const tokens = TONE_TOKENS[key];
    if (tokens === void 0) {
      throw new Error(
        `[m24 dangerous-mode] K11 \u7684 TONE_TOKENS \u672A\u767B\u8BB0 tone\u300C${key}\u300D\uFF08\u5DF2\u767B\u8BB0: ${TONE_KEYS2.join(", ")}\uFF09\uFF0C\u5371\u9669\u65B9\u5411 ${direction} \u65E0\u53EF\u7528\u4EE4\u724C\uFF0C\u62D2\u7EDD\u6E32\u67D3`
      );
    }
    return tokens;
  }
  __name(toneTokensFor2, "toneTokensFor");
  function operationSummary(labels) {
    return `\u5F71\u54CD ${labels.length} \u9879\u64CD\u4F5C`;
  }
  __name(operationSummary, "operationSummary");
  var ROOT_CLASS4 = "petui-dangerous-mode";
  var SWITCH_CLASS = "petui-dangerous-mode-switch";
  var STATE_CLASS = "petui-dangerous-mode-state";
  var OPS_CLASS = "petui-dangerous-mode-ops";
  var OP_CLASS = "petui-dangerous-mode-op";
  var OPS_EMPTY_CLASS = "petui-dangerous-mode-ops-empty";
  var OPS_TITLE_CLASS = "petui-dangerous-mode-ops-title";
  var CONFIRM_CLASS = "petui-dangerous-mode-confirm";
  var CONFIRM_TEXT_CLASS = "petui-dangerous-mode-confirm-text";
  var CONFIRM_OK_CLASS = "petui-dangerous-mode-confirm-ok";
  var CONFIRM_CANCEL_CLASS = "petui-dangerous-mode-confirm-cancel";
  var NOTICE_CLASS = "petui-dangerous-mode-notice";
  var REASON_PENDING = "\u5371\u9669\u6A21\u5F0F\u5207\u6362\u5F85\u786E\u8BA4\uFF1A\u8BF7\u5148\u5728\u4E0A\u65B9\u786E\u8BA4\u533A\u9009\u62E9\u300C\u786E\u8BA4\u300D\u6216\u300C\u53D6\u6D88\u300D";
  var REASON_DESTROYED = "\u5371\u9669\u6A21\u5F0F\u5DF2 destroy()\uFF1A\u9700\u91CD\u65B0 createDangerousMode \u624D\u80FD\u518D\u6B21\u4EA4\u4E92";
  function toneVar(tokens, slot) {
    return `var(${tokens[slot]})`;
  }
  __name(toneVar, "toneVar");
  function createDangerousMode(options = {}) {
    const doc = requireDocument();
    const disposer = createDisposer();
    let enabled = options.enabled === true;
    let pending = null;
    let destroyed = false;
    const affectedOperations = Object.freeze(
      normalizeOperations(options.affectedOperations)
    );
    const confirmText = typeof options.confirmText === "string" && options.confirmText.length > 0 ? options.confirmText : "\u6B64\u64CD\u4F5C\u5C06\u76F4\u63A5\u6539\u5199\u4E8B\u5B9E\u6E90\uFF0C\u53EF\u80FD\u4E0D\u53EF\u64A4\u9500\uFF1B\u8BF7\u5148\u5907\u4EFD\u3002";
    const root = doc.createElement("div");
    root.className = ROOT_CLASS4;
    root.setAttribute("data-enabled", enabled ? "true" : "false");
    const switchBtn = doc.createElement("button");
    switchBtn.className = SWITCH_CLASS;
    switchBtn.setAttribute("type", "button");
    switchBtn.setAttribute("role", "switch");
    switchBtn.setAttribute("data-role", "dangerous-mode-switch");
    const stateNote = doc.createElement("span");
    stateNote.className = STATE_CLASS;
    stateNote.setAttribute("data-role", "dangerous-mode-state");
    const notice = doc.createElement("p");
    notice.className = NOTICE_CLASS;
    notice.setAttribute("data-role", "dangerous-mode-notice");
    const opsWrap = doc.createElement("ul");
    opsWrap.className = OPS_CLASS;
    opsWrap.setAttribute("data-role", "dangerous-mode-ops");
    root.appendChild(switchBtn);
    root.appendChild(stateNote);
    root.appendChild(notice);
    root.appendChild(opsWrap);
    function renderOperations() {
      clearChildren2(opsWrap);
      if (affectedOperations.length === 0) {
        const empty = doc.createElement("li");
        empty.className = OPS_EMPTY_CLASS;
        empty.setAttribute("data-role", "dangerous-mode-ops-empty");
        empty.textContent = "\u5F53\u524D\u6CA1\u6709\u4EFB\u4F55\u88AB\u5F71\u54CD\u7684\u64CD\u4F5C\uFF08\u5F00\u542F\u540E\u4E0D\u4F1A\u6539\u53D8\u4EFB\u4F55\u5199\u5165\u884C\u4E3A\uFF09";
        opsWrap.appendChild(empty);
        return;
      }
      const title = doc.createElement("li");
      title.className = OPS_TITLE_CLASS;
      title.setAttribute("data-role", "dangerous-mode-ops-title");
      title.textContent = `\u5F00\u542F\u540E\u4F1A\u88AB\u5F71\u54CD\u7684\u64CD\u4F5C\uFF08${affectedOperations.length} \u9879\uFF09:`;
      opsWrap.appendChild(title);
      affectedOperations.forEach((label, index) => {
        const item = doc.createElement("li");
        item.className = OP_CLASS;
        item.setAttribute("data-role", "dangerous-mode-op");
        item.setAttribute("data-index", String(index));
        item.setAttribute("data-operation", label);
        item.textContent = label;
        opsWrap.appendChild(item);
      });
    }
    __name(renderOperations, "renderOperations");
    let confirmPanel = null;
    function clearConfirmPanel() {
      pending = null;
      if (confirmPanel === null) return;
      const panel = confirmPanel;
      confirmPanel = null;
      if (panel.parentNode) {
        panel.parentNode.removeChild(panel);
      }
    }
    __name(clearConfirmPanel, "clearConfirmPanel");
    function openConfirmPanel(direction) {
      clearConfirmPanel();
      pending = direction;
      const tokens = toneTokensFor2(direction);
      const dangerous = direction === "enable";
      const panel = doc.createElement("div");
      panel.className = CONFIRM_CLASS;
      panel.setAttribute("data-role", "dangerous-mode-confirm");
      panel.setAttribute("data-direction", direction);
      panel.setAttribute("role", "alertdialog");
      panel.style.cssText = [
        "display:flex",
        "flex-direction:column",
        `background:${toneVar(tokens, "bg")}`,
        `border:var(--space-1) solid ${toneVar(tokens, "border")}`,
        "border-radius:var(--border-radius-md)",
        "padding:var(--space-10)"
      ].join(";");
      const heading = doc.createElement("div");
      heading.className = CONFIRM_TEXT_CLASS;
      heading.setAttribute("data-role", "dangerous-mode-confirm-text");
      heading.style.cssText = `color:${toneVar(tokens, "text")};font-size:var(--font-size-13)`;
      heading.textContent = dangerous ? `${confirmText}\uFF08\u5373\u5C06\u5F00\u542F\u5371\u9669\u6A21\u5F0F\uFF0C${operationSummary(affectedOperations)}\uFF09` : "\u5173\u95ED\u5371\u9669\u6A21\u5F0F\u540E\uFF0C\u540E\u7EED\u5199\u5165\u5C06\u56DE\u5230\u5B89\u5168\u9ED8\u8BA4\u884C\u4E3A\uFF1B\u786E\u8BA4\u8981\u5173\u95ED\u5417\uFF1F";
      panel.appendChild(heading);
      const okBtn = doc.createElement("button");
      okBtn.className = CONFIRM_OK_CLASS;
      okBtn.setAttribute("type", "button");
      okBtn.setAttribute("data-role", "dangerous-mode-confirm-ok");
      okBtn.textContent = dangerous ? "\u786E\u8BA4\u5F00\u542F" : "\u786E\u8BA4\u5173\u95ED";
      okBtn.style.cssText = [
        "width:100%",
        `background:${toneVar(tokens, "bg")}`,
        `border:var(--space-1) solid ${toneVar(tokens, "border")}`,
        `color:${toneVar(tokens, "text")}`,
        "border-radius:var(--border-radius-sm)",
        "padding:var(--space-6)",
        "cursor:pointer"
      ].join(";");
      disposer.addEventListener(okBtn, "click", () => {
        if (destroyed) return;
        const target = pending;
        if (target === null) return;
        clearConfirmPanel();
        commit(target === "enable");
      });
      const cancelBtn = doc.createElement("button");
      cancelBtn.className = CONFIRM_CANCEL_CLASS;
      cancelBtn.setAttribute("type", "button");
      cancelBtn.setAttribute("data-role", "dangerous-mode-confirm-cancel");
      cancelBtn.textContent = "\u53D6\u6D88";
      cancelBtn.style.cssText = "cursor:pointer;border-radius:var(--border-radius-sm);padding:var(--space-6)";
      disposer.addEventListener(cancelBtn, "click", () => {
        if (destroyed) return;
        clearConfirmPanel();
        applyState();
      });
      panel.appendChild(okBtn);
      panel.appendChild(cancelBtn);
      confirmPanel = panel;
      root.appendChild(panel);
      applyState();
    }
    __name(openConfirmPanel, "openConfirmPanel");
    function disabledReason() {
      return destroyed ? REASON_DESTROYED : pending === null ? null : REASON_PENDING;
    }
    __name(disabledReason, "disabledReason");
    function applyState() {
      root.setAttribute("data-enabled", enabled ? "true" : "false");
      root.setAttribute("data-pending", pending === null ? "none" : pending);
      switchBtn.textContent = enabled ? "\u70B9\u51FB\u5173\u95ED\u5371\u9669\u6A21\u5F0F" : "\u70B9\u51FB\u5F00\u542F\u5371\u9669\u6A21\u5F0F";
      switchBtn.setAttribute("aria-checked", enabled ? "true" : "false");
      stateNote.textContent = enabled ? "\u5371\u9669\u6A21\u5F0F\uFF1A\u5DF2\u5F00\u542F" : `\u5371\u9669\u6A21\u5F0F\uFF1A\u5DF2\u5173\u95ED\uFF08${operationSummary(affectedOperations)}\u53D7\u4FDD\u62A4\uFF09`;
      notice.textContent = enabled ? "\u5DF2\u5F00\u542F\uFF1A\u5199\u5165\u76F4\u63A5\u4F5C\u7528\u4E8E\u4E8B\u5B9E\u6E90\uFF0C\u4E0D\u843D\u526F\u672C\u3002" : "\u672A\u5F00\u542F\uFF1A\u5199\u5165\u843D\u5728\u9694\u79BB\u526F\u672C\uFF0C\u4E8B\u5B9E\u6E90\u4E0D\u88AB\u6539\u52A8\u3002";
      const reason = disabledReason();
      if (reason === null) {
        switchBtn.removeAttribute("aria-disabled");
        switchBtn.removeAttribute("data-disabled-reason");
      } else {
        switchBtn.setAttribute("aria-disabled", "true");
        switchBtn.setAttribute("data-disabled-reason", reason);
        switchBtn.setAttribute("aria-disabled-reason", reason);
      }
    }
    __name(applyState, "applyState");
    function commit(next) {
      if (next === enabled) return;
      enabled = next;
      applyState();
      notifyToggle(next);
    }
    __name(commit, "commit");
    function notifyToggle(next) {
      const cb = options.onToggle;
      if (typeof cb !== "function") return;
      try {
        cb(next);
      } catch (err) {
        console.error(`[m24 dangerous-mode] onToggle(${String(next)}) \u56DE\u8C03\u629B\u9519\uFF08\u5DF2\u6355\u83B7\uFF0C\u5F00\u5173\u72B6\u6001\u4E0D\u56DE\u9000\uFF09:`, err);
      }
    }
    __name(notifyToggle, "notifyToggle");
    function requestToggle(next) {
      if (typeof next !== "boolean") {
        console.warn("[m24 dangerous-mode] setEnabled \u53EA\u63A5\u53D7 boolean\uFF0C\u6536\u5230\u975E\u5E03\u5C14\u503C\u5DF2\u5FFD\u7565");
        return;
      }
      if (destroyed) {
        console.warn(`[m24 dangerous-mode] destroy() \u4E4B\u540E\u8BF7\u6C42\u5207\u6362\u5230 ${String(next)} \u5DF2\u5FFD\u7565\uFF08\u9700\u91CD\u65B0 createDangerousMode\uFF09`);
        notifyDisabled();
        return;
      }
      if (next === enabled) {
        if (pending !== null) notifyDisabled();
        return;
      }
      if (pending !== null) {
        notifyDisabled();
        return;
      }
      openConfirmPanel(next ? "enable" : "disable");
    }
    __name(requestToggle, "requestToggle");
    function notifyDisabled() {
      const reason = disabledReason();
      if (reason === null) return;
      const cb = options.onDisabled;
      if (typeof cb !== "function") return;
      try {
        cb(reason);
      } catch (err) {
        console.error("[m24 dangerous-mode] onDisabled \u56DE\u8C03\u629B\u9519\uFF08\u5DF2\u6355\u83B7\uFF0C\u5F00\u5173\u72B6\u6001\u4E0D\u53D7\u5F71\u54CD\uFF09:", err);
      }
    }
    __name(notifyDisabled, "notifyDisabled");
    disposer.addEventListener(switchBtn, "click", () => {
      requestToggle(!enabled);
    });
    function setEnabled(next) {
      requestToggle(next);
    }
    __name(setEnabled, "setEnabled");
    function isEnabled() {
      return enabled;
    }
    __name(isEnabled, "isEnabled");
    function destroy() {
      if (destroyed) return;
      destroyed = true;
      clearConfirmPanel();
      disposer.dispose();
      clearChildren2(root);
      root.setAttribute("data-destroyed", "true");
      switchBtn.setAttribute("aria-disabled", "true");
      switchBtn.setAttribute("data-disabled-reason", REASON_DESTROYED);
    }
    __name(destroy, "destroy");
    Object.defineProperty(root, "isEnabled", { value: isEnabled, writable: false, enumerable: false, configurable: false });
    Object.defineProperty(root, "setEnabled", { value: setEnabled, writable: false, enumerable: false, configurable: false });
    Object.defineProperty(root, "destroy", { value: destroy, writable: false, enumerable: false, configurable: false });
    Object.defineProperty(root, "enabled", { get: /* @__PURE__ */ __name(() => enabled, "get"), enumerable: false, configurable: false });
    Object.defineProperty(root, "disabledReason", { get: disabledReason, enumerable: false, configurable: false });
    Object.defineProperty(root, "pending", {
      get: /* @__PURE__ */ __name(() => pending === null ? "none" : pending, "get"),
      enumerable: false,
      configurable: false
    });
    Object.defineProperty(root, "confirmText", {
      get: /* @__PURE__ */ __name(() => confirmText, "get"),
      enumerable: false,
      configurable: false
    });
    Object.defineProperty(root, "getAffectedOperations", {
      value: /* @__PURE__ */ __name(() => Object.freeze(affectedOperations.slice()), "value"),
      writable: false,
      enumerable: false,
      configurable: false
    });
    renderOperations();
    applyState();
    return root;
  }
  __name(createDangerousMode, "createDangerousMode");
  function normalizeOperations(input) {
    if (!Array.isArray(input)) return [];
    const out = [];
    for (const item of input) {
      if (typeof item !== "string") continue;
      const label = item.trim();
      if (label.length > 0) out.push(label);
    }
    return out;
  }
  __name(normalizeOperations, "normalizeOperations");
  function requireDocument() {
    const doc = globalThis.document;
    if (doc === void 0 || typeof doc.createElement !== "function") {
      throw new Error("[m24 dangerous-mode] \u73AF\u5883\u7F3A\u5C11 document.createElement\uFF0C\u65E0\u6CD5\u521B\u5EFA\u5371\u9669\u6A21\u5F0F\u5F00\u5173\uFF08\u9700\u8981 DOM \u5BBF\u4E3B\uFF09");
    }
    return doc;
  }
  __name(requireDocument, "requireDocument");
  function clearChildren2(el) {
    while (el.firstChild) {
      el.removeChild(el.firstChild);
    }
  }
  __name(clearChildren2, "clearChildren");

  // src/components/file-picker.ts
  var _FilePickerError = class _FilePickerError extends Error {
    constructor(reason, message, cause) {
      super(message === void 0 ? reason : message);
      this.name = "FilePickerError";
      this.reason = reason;
      this.cause = cause;
    }
  };
  __name(_FilePickerError, "FilePickerError");
  var FilePickerError = _FilePickerError;
  var DEFAULT_CHUNK_SIZE = 1048576;
  function normalizeChunkSize(value) {
    if (value === void 0) return DEFAULT_CHUNK_SIZE;
    if (typeof value !== "number" || !Number.isFinite(value) || value <= 0) {
      throw new RangeError("chunkSize must be a positive finite number");
    }
    return Math.floor(value);
  }
  __name(normalizeChunkSize, "normalizeChunkSize");
  function bytesToBinaryString(bytes) {
    let out = "";
    const STEP = 32768;
    for (let i = 0; i < bytes.length; i += STEP) {
      const slice = bytes.subarray(i, Math.min(i + STEP, bytes.length));
      out += String.fromCharCode.apply(null, Array.prototype.slice.call(slice));
    }
    return out;
  }
  __name(bytesToBinaryString, "bytesToBinaryString");
  function encodeBase64(bytes) {
    if (bytes.length === 0) return "";
    const btoaFn = globalThis.btoa;
    if (typeof btoaFn !== "function") {
      throw new FilePickerError("read-failed", "base64 encoder unavailable");
    }
    return btoaFn(bytesToBinaryString(bytes));
  }
  __name(encodeBase64, "encodeBase64");
  var defaultReadChunk = /* @__PURE__ */ __name(async (blob, start, end) => {
    const slice = blob.slice(start, end);
    const buffer = await slice.arrayBuffer();
    return new Uint8Array(buffer);
  }, "defaultReadChunk");
  async function readAsBase64(file, chunkSize, readChunk) {
    const size = typeof file.size === "number" && Number.isFinite(file.size) ? file.size : 0;
    let carry = new Uint8Array(0);
    let base64 = "";
    let offset = 0;
    while (offset < size) {
      const end = Math.min(offset + chunkSize, size);
      const part = await readChunk(file, offset, end);
      if (!(part instanceof Uint8Array)) {
        throw new FilePickerError("read-failed", "reader returned non-bytes");
      }
      let merged;
      if (carry.length > 0) {
        merged = new Uint8Array(carry.length + part.length);
        merged.set(carry, 0);
        merged.set(part, carry.length);
      } else {
        merged = part;
      }
      const usable = merged.length - merged.length % 3;
      if (usable > 0) base64 += encodeBase64(merged.subarray(0, usable));
      carry = merged.subarray(usable).slice();
      offset = end;
    }
    if (carry.length > 0) base64 += encodeBase64(carry);
    return base64;
  }
  __name(readAsBase64, "readAsBase64");
  function createFilePicker(options) {
    if (typeof document === "undefined" || typeof document.createElement !== "function") {
      throw new Error("createFilePicker requires a DOM document");
    }
    const chunkSize = normalizeChunkSize(options.chunkSize);
    const readChunk = options.hooks && options.hooks.readChunk || defaultReadChunk;
    const readFiles = options.hooks && options.hooks.files;
    const disposer = createDisposer();
    let destroyed = false;
    let reading = false;
    let awaitingPick = false;
    const root = document.createElement("span");
    root.className = "petui-file-picker";
    const input = document.createElement("input");
    input.type = "file";
    input.className = "petui-file-picker__input";
    input.setAttribute("accept", options.accept);
    input.hidden = true;
    const status = document.createElement("span");
    status.className = "petui-file-picker__status";
    status.setAttribute("aria-live", "polite");
    status.textContent = "\u672A\u9009\u62E9\u6587\u4EF6";
    root.appendChild(input);
    root.appendChild(status);
    function fail(reason, cause) {
      const error = cause instanceof FilePickerError ? cause : new FilePickerError(reason, void 0, cause);
      if (typeof options.onError === "function") {
        options.onError(error);
        return;
      }
      const CustomEventCtor = globalThis.CustomEvent;
      if (typeof CustomEventCtor === "function" && typeof root.dispatchEvent === "function") {
        root.dispatchEvent(new CustomEventCtor("filepickererror", { detail: error }));
      }
    }
    __name(fail, "fail");
    function onWindowFocus() {
      awaitingPick = false;
      fail("canceled");
    }
    __name(onWindowFocus, "onWindowFocus");
    async function consume(file) {
      reading = true;
      let base64;
      try {
        base64 = await readAsBase64(file, chunkSize, readChunk);
      } catch (err) {
        reading = false;
        fail(err instanceof FilePickerError ? err.reason : "read-failed", err);
        return;
      }
      reading = false;
      status.textContent = file.name + " \xB7 " + (file.type || "\u672A\u77E5\u7C7B\u578B") + " \xB7 base64 " + base64.length + " \u5B57\u7B26";
      options.onPick(file, base64);
    }
    __name(consume, "consume");
    function onChange() {
      awaitingPick = false;
      const list = readFiles ? readFiles(input) : input.files;
      const file = list && list.length > 0 ? list[0] : null;
      if (!file) {
        fail("no-files");
        return;
      }
      void consume(file);
    }
    __name(onChange, "onChange");
    disposer.addEventListener(input, "change", onChange);
    function open() {
      if (destroyed) {
        fail("destroyed");
        return;
      }
      if (reading) {
        fail("busy");
        return;
      }
      input.value = "";
      if (typeof input.click !== "function") {
        fail("unavailable");
        return;
      }
      if (typeof window !== "undefined" && typeof window.addEventListener === "function") {
        awaitingPick = true;
        disposer.addEventListener(window, "focus", onWindowFocus, { once: true });
      }
      input.click();
    }
    __name(open, "open");
    function destroy() {
      if (destroyed) return;
      destroyed = true;
      awaitingPick = false;
      disposer.dispose();
      while (root.firstChild) root.removeChild(root.firstChild);
    }
    __name(destroy, "destroy");
    root.open = open;
    root.destroy = destroy;
    Object.defineProperty(root, "accept", { get: /* @__PURE__ */ __name(() => options.accept, "get"), configurable: true });
    Object.defineProperty(root, "chunkSize", { get: /* @__PURE__ */ __name(() => chunkSize, "get"), configurable: true });
    Object.defineProperty(root, "reading", { get: /* @__PURE__ */ __name(() => reading, "get"), configurable: true });
    Object.defineProperty(root, "busy", { get: /* @__PURE__ */ __name(() => reading, "get"), configurable: true });
    Object.defineProperty(root, "fileInput", { get: /* @__PURE__ */ __name(() => input, "get"), configurable: true });
    Object.defineProperty(root, "statusText", { get: /* @__PURE__ */ __name(() => status.textContent, "get"), configurable: true });
    return root;
  }
  __name(createFilePicker, "createFilePicker");

  // src/components/canvas-viewport.ts
  var DEFAULT_ZOOM_MIN = 0.1;
  var DEFAULT_ZOOM_MAX = 10;
  var DEFAULT_ANCHOR = "cursor";
  var DRAG_THRESHOLD = 3;
  var MIN_GRID_SPACING = 36;
  var MAX_GRID_SPACING = 144;
  var CANVAS_TOKENS2 = Object.freeze({
    background: "var(--canvas-background)",
    surface: "var(--canvas-surface)",
    grid: "var(--canvas-grid)",
    axis: "var(--canvas-axis)",
    text: "var(--canvas-text)",
    selection: "var(--canvas-selection)",
    series1: "var(--canvas-series-1)",
    series2: "var(--canvas-series-2)",
    lineWidth: "var(--space-1)",
    lineWidthHeavy: "var(--space-2)",
    fontSize: "var(--font-size-12)"
  });
  function readNumber(value, fallback) {
    const n = typeof value === "number" ? value : Number(value);
    return Number.isFinite(n) ? n : fallback;
  }
  __name(readNumber, "readNumber");
  function normalizeZoomRange(input) {
    let lo = DEFAULT_ZOOM_MIN;
    let hi = DEFAULT_ZOOM_MAX;
    if (Array.isArray(input) && input.length >= 2) {
      lo = readNumber(input[0], DEFAULT_ZOOM_MIN);
      hi = readNumber(input[1], DEFAULT_ZOOM_MAX);
    } else if (input !== null && typeof input === "object") {
      const boxed = input;
      lo = readNumber(boxed.min, DEFAULT_ZOOM_MIN);
      hi = readNumber(boxed.max, DEFAULT_ZOOM_MAX);
    }
    if (!(lo > 0)) lo = DEFAULT_ZOOM_MIN;
    if (!(hi > 0)) hi = DEFAULT_ZOOM_MAX;
    return lo <= hi ? [lo, hi] : [hi, lo];
  }
  __name(normalizeZoomRange, "normalizeZoomRange");
  function gridStepFor(scale) {
    const s = scale > 0 ? scale : 1;
    const raw = MIN_GRID_SPACING / s;
    const magnitude = Math.pow(10, Math.floor(Math.log10(raw > 0 ? raw : 1)));
    let step = magnitude * 10;
    for (const m of [1, 2, 5, 10]) {
      if (magnitude * m * s >= MIN_GRID_SPACING) {
        step = magnitude * m;
        break;
      }
    }
    let guard = 0;
    while (step * s > MAX_GRID_SPACING && guard < 64) {
      step *= 2;
      guard += 1;
    }
    return step;
  }
  __name(gridStepFor, "gridStepFor");
  function createCanvasViewport(options) {
    const opts = options === null || options === void 0 ? {} : options;
    const host = opts.host === void 0 || opts.host === null ? null : opts.host;
    let createdCanvas = false;
    let canvas = opts.canvas === void 0 ? null : opts.canvas;
    if (canvas === null) {
      if (host === null) {
        console.warn("[PetUI canvas-viewport] \u65E2\u6CA1\u6709 canvas \u4E5F\u6CA1\u6709 host\uFF1A\u89C6\u53E3\u4E0D\u53EF\u521B\u5EFA\uFF0C\u8FD4\u56DE null");
        return null;
      }
      const doc = globalThis.document;
      if (doc === void 0 || typeof doc.createElement !== "function") {
        console.warn("[PetUI canvas-viewport] \u73AF\u5883\u65E0 document\uFF0C\u65E0\u6CD5\u5728 host \u5185\u521B\u5EFA\u753B\u5E03\uFF0C\u8FD4\u56DE null");
        return null;
      }
      const made = doc.createElement("canvas");
      made.style.width = "100%";
      made.style.height = "100%";
      made.style.display = "block";
      made.style.touchAction = "none";
      if (typeof host.appendChild === "function") host.appendChild(made);
      canvas = made;
      createdCanvas = true;
    }
    const el = canvas;
    let ctx = null;
    try {
      ctx = typeof el.getContext === "function" ? el.getContext("2d") : null;
    } catch (err) {
      console.warn('[PetUI canvas-viewport] getContext("2d") \u629B\u9519\uFF0C\u89C6\u53E3\u4E0D\u53EF\u521B\u5EFA\uFF0C\u8FD4\u56DE null:', err);
      return null;
    }
    if (ctx === null || ctx === void 0) {
      console.warn("[PetUI canvas-viewport] \u62FF\u4E0D\u5230 2D \u4E0A\u4E0B\u6587\uFF08\u6D4B\u8BD5\u73AF\u5883\u8BF7\u6CE8\u5165 ctx \u66FF\u8EAB\uFF09\uFF0C\u8FD4\u56DE null");
      return null;
    }
    const disposer = createDisposer(globalThis);
    const zoomRange = normalizeZoomRange(opts.zoomRange);
    const anchor = opts.anchor === "center" ? "center" : opts.anchor === "cursor" ? "cursor" : DEFAULT_ANCHOR;
    const fitBox = opts.fitTo === void 0 || opts.fitTo === null ? null : opts.fitTo;
    const drawFn = typeof opts.draw === "function" ? opts.draw : defaultDraw;
    const onPick = typeof opts.onPick === "function" ? opts.onPick : null;
    const cam = { scale: 1, tx: 0, ty: 0 };
    let destroyed = false;
    let framePending = false;
    let needsRender = true;
    let managedBacking = false;
    let pendingFit = fitBox !== null;
    let pendingCenter = fitBox === null;
    let drawErrorReported = false;
    const colorCache = /* @__PURE__ */ new Map();
    let themeUnsub = null;
    function cssSize() {
      let width = 0;
      let height = 0;
      if (typeof el.getBoundingClientRect === "function") {
        const rect = el.getBoundingClientRect();
        width = readNumber(rect === null || rect === void 0 ? NaN : rect.width, 0);
        height = readNumber(rect === null || rect === void 0 ? NaN : rect.height, 0);
      }
      if (width <= 0 || height <= 0) {
        const cw = readNumber(el.clientWidth, 0);
        const ch = readNumber(el.clientHeight, 0);
        if (cw > 0 && ch > 0) {
          width = cw;
          height = ch;
        }
      }
      if (width <= 0 || height <= 0) {
        const aw = readNumber(el.width, 0);
        const ah = readNumber(el.height, 0);
        if (aw > 0 && ah > 0) {
          width = aw;
          height = ah;
        }
      }
      return { width: Math.max(0, width), height: Math.max(0, height) };
    }
    __name(cssSize, "cssSize");
    function measuredDpr() {
      const size = cssSize();
      const backingW = readNumber(el.width, 0);
      const backingH = readNumber(el.height, 0);
      const candidates = [];
      if (size.width > 0 && backingW > 0) candidates.push(backingW / size.width);
      if (size.height > 0 && backingH > 0) candidates.push(backingH / size.height);
      const usable = candidates.filter((v) => Number.isFinite(v) && v > 0);
      if (usable.length === 0) return null;
      return Math.max.apply(null, usable);
    }
    __name(measuredDpr, "measuredDpr");
    function environmentDpr() {
      const raw = globalThis.devicePixelRatio;
      const value = readNumber(raw, 1);
      return value > 0 ? value : 1;
    }
    __name(environmentDpr, "environmentDpr");
    function getDpr() {
      const measured = measuredDpr();
      return measured !== null ? measured : environmentDpr();
    }
    __name(getDpr, "getDpr");
    function resolveRenderDpr() {
      const env = environmentDpr();
      if (!managedBacking) {
        managedBacking = true;
        const measured = measuredDpr();
        if (measured !== null && measured > env) return measured;
      }
      return env;
    }
    __name(resolveRenderDpr, "resolveRenderDpr");
    function toWorld(screenX, screenY) {
      const scale = cam.scale > 0 ? cam.scale : 1;
      return { x: (screenX - cam.tx) / scale, y: (screenY - cam.ty) / scale };
    }
    __name(toWorld, "toWorld");
    function toScreen(worldX, worldY) {
      return { x: worldX * cam.scale + cam.tx, y: worldY * cam.scale + cam.ty };
    }
    __name(toScreen, "toScreen");
    function applyTransform(g, view) {
      const k = view.dpr * view.scale;
      g.setTransform(k, 0, 0, k, view.dpr * view.tx, view.dpr * view.ty);
    }
    __name(applyTransform, "applyTransform");
    function makeView(size, dpr) {
      const scale = cam.scale;
      const tx = cam.tx;
      const ty = cam.ty;
      const safe = scale > 0 ? scale : 1;
      const snapshotToWorld = /* @__PURE__ */ __name((screenX, screenY) => ({
        x: (screenX - tx) / safe,
        y: (screenY - ty) / safe
      }), "snapshotToWorld");
      const snapshotToScreen = /* @__PURE__ */ __name((worldX, worldY) => ({
        x: worldX * scale + tx,
        y: worldY * scale + ty
      }), "snapshotToScreen");
      return Object.freeze({
        scale,
        tx,
        ty,
        dpr,
        width: size.width,
        height: size.height,
        anchor,
        toWorld: snapshotToWorld,
        toScreen: snapshotToScreen
      });
    }
    __name(makeView, "makeView");
    function clampScale(value) {
      if (!Number.isFinite(value) || value <= 0) return zoomRange[0];
      if (value < zoomRange[0]) return zoomRange[0];
      if (value > zoomRange[1]) return zoomRange[1];
      return value;
    }
    __name(clampScale, "clampScale");
    function zoomAt(screenX, screenY, factor) {
      const size = cssSize();
      const anchorX = anchor === "center" && size.width > 0 ? size.width / 2 : screenX;
      const anchorY = anchor === "center" && size.height > 0 ? size.height / 2 : screenY;
      const before = toWorld(anchorX, anchorY);
      const next = clampScale(cam.scale * (Number.isFinite(factor) && factor > 0 ? factor : 1));
      cam.scale = next;
      cam.tx = anchorX - before.x * next;
      cam.ty = anchorY - before.y * next;
      return next;
    }
    __name(zoomAt, "zoomAt");
    function panBy(dx, dy) {
      cam.tx += readNumber(dx, 0);
      cam.ty += readNumber(dy, 0);
    }
    __name(panBy, "panBy");
    function fit(box) {
      const target = box === void 0 ? fitBox : box;
      if (target === null || target === void 0) {
        resetToUnit();
        return;
      }
      const width = readNumber(target.width, 0);
      const height = readNumber(target.height, 0);
      const size = cssSize();
      const sx = width > 0 && size.width > 0 ? size.width / width : 1;
      const sy = height > 0 && size.height > 0 ? size.height / height : 1;
      cam.scale = clampScale(Math.min(sx, sy));
      const cx = readNumber(target.x, 0) + width / 2;
      const cy = readNumber(target.y, 0) + height / 2;
      cam.tx = size.width / 2 - cx * cam.scale;
      cam.ty = size.height / 2 - cy * cam.scale;
    }
    __name(fit, "fit");
    function resetToUnit() {
      const size = cssSize();
      cam.scale = clampScale(1);
      cam.tx = size.width / 2;
      cam.ty = size.height / 2;
    }
    __name(resetToUnit, "resetToUnit");
    function reset() {
      if (fitBox !== null) fit(fitBox);
      else resetToUnit();
      invalidate();
    }
    __name(reset, "reset");
    function tokenColor(tokenExpression2) {
      const cached = colorCache.get(tokenExpression2);
      if (cached !== void 0) return cached;
      let value = null;
      try {
        const resolved = resolveTokenValue(el, tokenExpression2);
        value = resolved.value;
      } catch (err) {
        console.warn(
          "[PetUI canvas-viewport] \u4EE4\u724C\u89E3\u6790\u5931\u8D25\uFF08\u672C\u5E27\u8DF3\u8FC7\u8BE5\u89C6\u89C9\u5143\u7D20\uFF09\uFF1B\u7F3A\u4EE4\u724C\u8BF7\u8D70 --color-n-<hex> \u51FA\u53E3\u6216\u56DE\u62A5 m01\uFF08\u4E0D\u5F97\u81EA\u9020\u4EE4\u724C\u540D\uFF09:",
          tokenExpression2,
          err
        );
        value = null;
      }
      colorCache.set(tokenExpression2, value);
      return value;
    }
    __name(tokenColor, "tokenColor");
    function tokenLength(tokenExpression2, fallback) {
      const raw = tokenColor(tokenExpression2);
      if (raw === null) return fallback;
      const parsed = parseFloat(raw);
      return Number.isFinite(parsed) && parsed > 0 ? parsed : fallback;
    }
    __name(tokenLength, "tokenLength");
    function defaultDraw(g, view) {
      const gridColor = tokenColor(CANVAS_TOKENS2.grid);
      const axisColor = tokenColor(CANVAS_TOKENS2.axis);
      const surfaceColor = tokenColor(CANVAS_TOKENS2.surface);
      const textColor = tokenColor(CANVAS_TOKENS2.text);
      const series1 = tokenColor(CANVAS_TOKENS2.series1);
      const series2 = tokenColor(CANVAS_TOKENS2.series2);
      const selection = tokenColor(CANVAS_TOKENS2.selection);
      const thin = tokenLength(CANVAS_TOKENS2.lineWidth, 1);
      const heavy = tokenLength(CANVAS_TOKENS2.lineWidthHeavy, 2);
      const fontSize = tokenLength(CANVAS_TOKENS2.fontSize, 12);
      const topLeft = view.toWorld(0, 0);
      const bottomRight = view.toWorld(view.width, view.height);
      if (fitBox !== null && surfaceColor !== null) {
        g.fillStyle = surfaceColor;
        g.fillRect(fitBox.x, fitBox.y, fitBox.width, fitBox.height);
      }
      if (gridColor !== null && bottomRight.x > topLeft.x && bottomRight.y > topLeft.y) {
        const step = gridStepFor(view.scale);
        g.strokeStyle = gridColor;
        g.lineWidth = thin;
        g.beginPath();
        const gx0 = Math.floor(topLeft.x / step) * step;
        const gx1 = Math.ceil(bottomRight.x / step) * step;
        for (let x = gx0; x <= gx1; x += step) {
          g.moveTo(x, topLeft.y);
          g.lineTo(x, bottomRight.y);
        }
        const gy0 = Math.floor(topLeft.y / step) * step;
        const gy1 = Math.ceil(bottomRight.y / step) * step;
        for (let y = gy0; y <= gy1; y += step) {
          g.moveTo(topLeft.x, y);
          g.lineTo(bottomRight.x, y);
        }
        g.stroke();
      }
      if (fitBox !== null && fitBox.width > 0 && fitBox.height > 0) {
        const midY = fitBox.y + fitBox.height / 2;
        const amplitude = fitBox.height / 2 - fitBox.height / 16;
        const samples = 64;
        const drawWave = /* @__PURE__ */ __name((color, phase, freq) => {
          if (color === null) return;
          g.strokeStyle = color;
          g.lineWidth = heavy;
          g.beginPath();
          for (let i = 0; i <= samples; i += 1) {
            const t = i / samples;
            const x = fitBox.x + t * fitBox.width;
            const y = midY - Math.sin(t * Math.PI * 2 * freq + phase) * amplitude;
            if (i === 0) g.moveTo(x, y);
            else g.lineTo(x, y);
          }
          g.stroke();
        }, "drawWave");
        drawWave(series1, 0, 1.5);
        drawWave(series2, Math.PI / 2, 2.5);
        if (axisColor !== null) {
          g.strokeStyle = axisColor;
          g.lineWidth = thin;
          if (typeof g.setLineDash === "function") g.setLineDash([]);
          g.strokeRect(fitBox.x, fitBox.y, fitBox.width, fitBox.height);
        }
      }
      if (axisColor !== null) {
        g.strokeStyle = axisColor;
        g.lineWidth = heavy;
        g.beginPath();
        g.moveTo(topLeft.x, 0);
        g.lineTo(bottomRight.x, 0);
        g.moveTo(0, topLeft.y);
        g.lineTo(0, bottomRight.y);
        g.stroke();
      }
      if (selection !== null) {
        g.fillStyle = selection;
        g.beginPath();
        g.arc(0, 0, heavy * 2, 0, Math.PI * 2);
        g.fill();
      }
      if (textColor !== null) {
        g.fillStyle = textColor;
        g.font = fontSize + " sans-serif";
        g.fillText("x=" + Math.round(topLeft.x) + " y=" + Math.round(topLeft.y), topLeft.x, topLeft.y + fontSize);
        g.fillText("scale=" + Math.round(view.scale * 100) / 100, topLeft.x, topLeft.y + fontSize * 2);
      }
    }
    __name(defaultDraw, "defaultDraw");
    function render() {
      if (destroyed) return;
      const g = ctx;
      if (g === null) return;
      const dpr = resolveRenderDpr();
      const size = cssSize();
      if (size.width > 0 && size.height > 0) {
        const targetW = Math.max(1, Math.round(size.width * dpr));
        const targetH = Math.max(1, Math.round(size.height * dpr));
        if (el.width !== targetW) el.width = targetW;
        if (el.height !== targetH) el.height = targetH;
      }
      if (pendingFit && size.width > 0 && size.height > 0) {
        pendingFit = false;
        pendingCenter = false;
        fit(fitBox);
      } else if (pendingCenter && size.width > 0 && size.height > 0) {
        pendingCenter = false;
        resetToUnit();
      }
      const view = makeView(size, getDpr());
      g.setTransform(1, 0, 0, 1, 0, 0);
      g.clearRect(0, 0, el.width, el.height);
      const background = tokenColor(CANVAS_TOKENS2.background);
      if (background !== null) {
        g.fillStyle = background;
        g.fillRect(0, 0, el.width, el.height);
      }
      applyTransform(g, view);
      try {
        drawFn(g, view);
      } catch (err) {
        if (!drawErrorReported) {
          drawErrorReported = true;
          console.warn("[PetUI canvas-viewport] draw \u56DE\u8C03\u629B\u9519\uFF0C\u5DF2\u9694\u79BB\uFF08\u53EA\u62A5\u4E00\u6B21\uFF09:", err);
        }
      }
      g.setTransform(1, 0, 0, 1, 0, 0);
    }
    __name(render, "render");
    function invalidate() {
      if (destroyed) return;
      needsRender = true;
      if (framePending) return;
      framePending = true;
      disposer.raf(() => {
        framePending = false;
        if (destroyed || !needsRender) return;
        needsRender = false;
        render();
      });
    }
    __name(invalidate, "invalidate");
    let drag = null;
    function pointFromEvent(ev) {
      const e = ev;
      const ox = readNumber(e.offsetX, NaN);
      const oy = readNumber(e.offsetY, NaN);
      if (Number.isFinite(ox) && Number.isFinite(oy) && (ox !== 0 || oy !== 0)) return { x: ox, y: oy };
      const rect = typeof el.getBoundingClientRect === "function" ? el.getBoundingClientRect() : null;
      const left = rect === null || rect === void 0 ? 0 : readNumber(rect.left, 0);
      const top = rect === null || rect === void 0 ? 0 : readNumber(rect.top, 0);
      return { x: readNumber(e.clientX, 0) - left, y: readNumber(e.clientY, 0) - top };
    }
    __name(pointFromEvent, "pointFromEvent");
    function emitPick(ev, button) {
      if (onPick === null || destroyed) return;
      const p = pointFromEvent(ev);
      const pick = Object.freeze({
        x: p.x,
        y: p.y,
        world: toWorld(p.x, p.y),
        button,
        scale: cam.scale,
        dpr: getDpr()
      });
      try {
        onPick(pick);
      } catch (err) {
        console.warn("[PetUI canvas-viewport] onPick \u56DE\u8C03\u629B\u9519\uFF0C\u5DF2\u9694\u79BB:", err);
      }
    }
    __name(emitPick, "emitPick");
    function onPointerDown(ev) {
      if (destroyed) return;
      const e = ev;
      const button = readNumber(e.button, 0);
      if (button !== 0) return;
      const p = pointFromEvent(ev);
      const id = readNumber(e.pointerId, -1);
      drag = { id, x: p.x, y: p.y, moved: false, button };
      if (id >= 0 && typeof el.setPointerCapture === "function") {
        try {
          el.setPointerCapture(id);
        } catch (err) {
          console.warn("[PetUI canvas-viewport] setPointerCapture \u5931\u8D25\uFF08\u62D6\u52A8\u4ECD\u53EF\u7528\uFF09:", err);
        }
      }
    }
    __name(onPointerDown, "onPointerDown");
    function onPointerMove(ev) {
      if (drag === null || destroyed) return;
      const p = pointFromEvent(ev);
      const dx = p.x - drag.x;
      const dy = p.y - drag.y;
      if (!drag.moved && Math.abs(dx) + Math.abs(dy) < DRAG_THRESHOLD) return;
      drag.moved = true;
      drag.x = p.x;
      drag.y = p.y;
      panBy(dx, dy);
      invalidate();
    }
    __name(onPointerMove, "onPointerMove");
    function releaseCapture(id) {
      if (id < 0 || typeof el.releasePointerCapture !== "function") return;
      try {
        el.releasePointerCapture(id);
      } catch (err) {
        console.warn("[PetUI canvas-viewport] releasePointerCapture \u5931\u8D25:", err);
      }
    }
    __name(releaseCapture, "releaseCapture");
    function onPointerUp(ev) {
      if (drag === null) return;
      const current = drag;
      drag = null;
      releaseCapture(current.id);
      if (!current.moved) emitPick(ev, current.button);
    }
    __name(onPointerUp, "onPointerUp");
    function onPointerCancel() {
      if (drag === null) return;
      const current = drag;
      drag = null;
      releaseCapture(current.id);
    }
    __name(onPointerCancel, "onPointerCancel");
    function onWheel(ev) {
      if (destroyed) return;
      const e = ev;
      if (typeof e.preventDefault === "function" && e.cancelable !== false) e.preventDefault();
      const delta = readNumber(e.deltaY, 0);
      if (delta === 0) return;
      const p = pointFromEvent(ev);
      zoomAt(p.x, p.y, Math.pow(2, -delta / 400));
      invalidate();
    }
    __name(onWheel, "onWheel");
    const usePointerEvents = typeof globalThis.PointerEvent === "function";
    if (usePointerEvents) {
      disposer.addEventListener(el, "pointerdown", onPointerDown);
      disposer.addEventListener(el, "pointermove", onPointerMove);
      disposer.addEventListener(el, "pointerup", onPointerUp);
      disposer.addEventListener(el, "pointercancel", onPointerCancel);
      disposer.addEventListener(el, "lostpointercapture", onPointerCancel);
    } else {
      disposer.addEventListener(el, "mousedown", onPointerDown);
      disposer.addEventListener(el, "mousemove", onPointerMove);
      disposer.addEventListener(el, "mouseup", onPointerUp);
      disposer.addEventListener(el, "mouseleave", onPointerCancel);
    }
    disposer.addEventListener(el, "wheel", onWheel, { passive: false });
    const globalObject = globalThis;
    let observerAttached = false;
    if (typeof globalObject.ResizeObserver === "function") {
      try {
        const ro = new globalObject.ResizeObserver(() => {
          invalidate();
        });
        ro.observe(el);
        disposer.observe(ro);
        observerAttached = true;
      } catch (err) {
        console.warn("[PetUI canvas-viewport] ResizeObserver \u5EFA\u7ACB\u5931\u8D25\uFF0C\u9000\u5316\u5230 window.resize:", err);
      }
    }
    if (!observerAttached) {
      const win = typeof window === "undefined" ? null : window;
      if (win !== null && typeof win.addEventListener === "function") {
        disposer.addEventListener(win, "resize", () => invalidate());
      } else {
        console.warn("[PetUI canvas-viewport] \u65E0 ResizeObserver \u4E5F\u65E0 window\uFF1A\u5BBF\u4E3B\u5C3A\u5BF8\u53D8\u5316\u4E0D\u4F1A\u81EA\u52A8\u91CD\u7ED8\uFF08\u53EF\u624B\u52A8 invalidate\uFF09");
      }
    }
    themeUnsub = onThemeChange(() => {
      colorCache.clear();
      invalidate();
    });
    disposer.add(() => {
      if (themeUnsub !== null) {
        const unsub = themeUnsub;
        themeUnsub = null;
        unsub();
      }
    });
    if (fitBox !== null) fit(fitBox);
    else resetToUnit();
    invalidate();
    function destroy() {
      if (destroyed) return;
      destroyed = true;
      drag = null;
      colorCache.clear();
      if (themeUnsub !== null) {
        const unsub = themeUnsub;
        themeUnsub = null;
        try {
          unsub();
        } catch (err) {
          console.warn("[PetUI canvas-viewport] \u4E3B\u9898\u9000\u8BA2\u5931\u8D25:", err);
        }
      }
      try {
        disposer.dispose();
      } catch (err) {
        console.error("[PetUI canvas-viewport] destroy() \u671F\u95F4\u6709\u91CA\u653E\u52A8\u4F5C\u5931\u8D25\uFF08\u5DF2\u7531 disposer \u9010\u4E2A\u4E0A\u62A5\uFF09:", err);
      }
      needsRender = false;
      framePending = false;
      ctx = null;
      if (createdCanvas && typeof el.parentNode !== "undefined" && el.parentNode !== null) {
        el.parentNode.removeChild(el);
      }
    }
    __name(destroy, "destroy");
    const api = {
      // 只读 getter（一律 `Object.defineProperty`，见下）占位：`canvas` / `host` / `view` /
      // `scale` / `dpr` / `destroyed` 会在后面用 defineProperty 覆盖
      canvas: el,
      host,
      view: makeView(cssSize(), getDpr()),
      scale: cam.scale,
      dpr: getDpr(),
      destroyed: false,
      toWorld,
      toScreen,
      getDpr,
      getScale: /* @__PURE__ */ __name(() => cam.scale, "getScale"),
      getView: /* @__PURE__ */ __name(() => makeView(cssSize(), getDpr()), "getView"),
      zoomAt: /* @__PURE__ */ __name((sx, sy, factor) => {
        const next = zoomAt(sx, sy, factor);
        invalidate();
        return next;
      }, "zoomAt"),
      panBy: /* @__PURE__ */ __name((dx, dy) => {
        panBy(dx, dy);
        invalidate();
      }, "panBy"),
      fit: /* @__PURE__ */ __name((box) => {
        fit(box);
        invalidate();
      }, "fit"),
      reset,
      invalidate,
      destroy
    };
    Object.defineProperty(api, "canvas", { get: /* @__PURE__ */ __name(() => el, "get"), enumerable: true, configurable: true });
    Object.defineProperty(api, "host", { get: /* @__PURE__ */ __name(() => host, "get"), enumerable: true, configurable: true });
    Object.defineProperty(api, "view", { get: /* @__PURE__ */ __name(() => makeView(cssSize(), getDpr()), "get"), enumerable: true, configurable: true });
    Object.defineProperty(api, "scale", { get: /* @__PURE__ */ __name(() => cam.scale, "get"), enumerable: true, configurable: true });
    Object.defineProperty(api, "dpr", { get: /* @__PURE__ */ __name(() => getDpr(), "get"), enumerable: true, configurable: true });
    Object.defineProperty(api, "destroyed", { get: /* @__PURE__ */ __name(() => destroyed, "get"), enumerable: true, configurable: true });
    return api;
  }
  __name(createCanvasViewport, "createCanvasViewport");

  // src/components/mesh-viewport-3d.ts
  var _MeshViewport3DDisposedError = class _MeshViewport3DDisposedError extends Error {
    constructor(method) {
      super(`[PetUI mesh-viewport-3d] ${method}() \u5728 destroy() \u4E4B\u540E\u88AB\u8C03\u7528\uFF1B\u8BF7\u91CD\u5EFA\u5B9E\u4F8B`);
      this.name = "DisposedError";
      this.method = method;
    }
  };
  __name(_MeshViewport3DDisposedError, "MeshViewport3DDisposedError");
  var MeshViewport3DDisposedError = _MeshViewport3DDisposedError;
  var _MeshViewport3DCanvasError = class _MeshViewport3DCanvasError extends Error {
    constructor(reason) {
      super(`[PetUI mesh-viewport-3d] \u753B\u5E03\u4E0D\u53EF\u7528\uFF08${reason}\uFF09\uFF1B\u8BF7\u4F20\u5165\u771F\u5B9E canvas\uFF08\u8282\u70B9\u6D4B\u8BD5\u9700\u6CE8\u5165 ctx \u66FF\u8EAB\uFF09`);
      this.name = "CanvasUnavailableError";
    }
  };
  __name(_MeshViewport3DCanvasError, "MeshViewport3DCanvasError");
  var MeshViewport3DCanvasError = _MeshViewport3DCanvasError;
  var _MeshViewport3DGeometryError = class _MeshViewport3DGeometryError extends Error {
    constructor(reason) {
      super(`[PetUI mesh-viewport-3d] \u51E0\u4F55\u6570\u636E\u975E\u6CD5\uFF1A${reason}`);
      this.name = "MeshGeometryError";
    }
  };
  __name(_MeshViewport3DGeometryError, "MeshViewport3DGeometryError");
  var MeshViewport3DGeometryError = _MeshViewport3DGeometryError;
  var _MeshBoneRangeError = class _MeshBoneRangeError extends RangeError {
    constructor(index, boneCount) {
      super(`[PetUI mesh-viewport-3d] highlightBone(${index}) \u8D8A\u754C\uFF1A\u5F53\u524D\u51E0\u4F55\u6709 ${boneCount} \u6839\u9AA8\u9ABC\u63CF\u8FF0`);
      this.name = "MeshBoneRangeError";
      this.index = index;
      this.boneCount = boneCount;
    }
  };
  __name(_MeshBoneRangeError, "MeshBoneRangeError");
  var MeshBoneRangeError = _MeshBoneRangeError;
  function quatFromAxisAngle(axis, angle) {
    const half = angle / 2;
    const s = Math.sin(half);
    const len = Math.hypot(axis[0], axis[1], axis[2]) || 1;
    return [axis[0] / len * s, axis[1] / len * s, axis[2] / len * s, Math.cos(half)];
  }
  __name(quatFromAxisAngle, "quatFromAxisAngle");
  function quatMul(a, b) {
    const [ax, ay, az, aw] = a;
    const [bx, by, bz, bw] = b;
    return [
      aw * bx + ax * bw + ay * bz - az * by,
      aw * by - ax * bz + ay * bw + az * bx,
      aw * bz + ax * by - ay * bx + az * bw,
      aw * bw - ax * bx - ay * by - az * bz
    ];
  }
  __name(quatMul, "quatMul");
  function quatNormalize(q) {
    const n = Math.hypot(q[0], q[1], q[2], q[3]) || 1;
    return [q[0] / n, q[1] / n, q[2] / n, q[3] / n];
  }
  __name(quatNormalize, "quatNormalize");
  function quatFromYawPitch(yawAngle, pitchAngle) {
    return quatNormalize(quatMul(
      quatFromAxisAngle([1, 0, 0], pitchAngle),
      quatFromAxisAngle([0, 1, 0], -yawAngle)
    ));
  }
  __name(quatFromYawPitch, "quatFromYawPitch");
  function quatRotationMatrix(q) {
    const [x, y, z, w] = quatNormalize(q);
    return [
      1 - 2 * (y * y + z * z),
      2 * (x * y - w * z),
      2 * (x * z + w * y),
      2 * (x * y + w * z),
      1 - 2 * (x * x + z * z),
      2 * (y * z - w * x),
      2 * (x * z - w * y),
      2 * (y * z + w * x),
      1 - 2 * (x * x + y * y)
    ];
  }
  __name(quatRotationMatrix, "quatRotationMatrix");
  var MESH_TOKENS = Object.freeze({
    background: "var(--canvas-background)",
    grid: "var(--canvas-grid)",
    axis: "var(--canvas-axis)",
    text: "var(--canvas-text)",
    selection: "var(--canvas-selection)",
    series1: "var(--canvas-series-1)",
    series2: "var(--canvas-series-2)",
    lineWidth: "var(--space-1)",
    lineWidthHeavy: "var(--space-2)",
    fontSize: "var(--font-size-12)",
    previewSize: "var(--space-64)"
  });
  var MESH_PIXEL_BUDGET_DEFAULT = 3e6;
  var MESH_CAMERA_DEFAULTS = Object.freeze({
    /** 绕 Y 轴偏航（弧度；30° ⇒ 能同时看到两个侧面，"立体"的来由） */
    yaw: Math.PI / 6,
    /** 绕 X 轴俯仰（弧度；-20° ⇒ 略微俯视） */
    pitch: -Math.PI / 9,
    /** 竖直视场角（弧度；45°） */
    fovY: Math.PI / 4,
    /** 自动取景留边系数（> 1 ⇒ 更远 ⇒ 四周留白） */
    fitMargin: 1.15,
    /** 缩放上下界（越界**钳制**，不抛） */
    zoomMin: 0.25,
    zoomMax: 4,
    /** 轨道旋转灵敏度（弧度 / css px） */
    orbitSpeed: 0.01,
    /** 近平面系数（`near = radius * nearFactor`） */
    nearFactor: 0.05,
    /** 俯仰钳制（避免翻转） */
    pitchLimit: Math.PI / 2 - 0.05,
    /** 漫游移动速度（**几何半径 / 秒**；逐帧按时间积分，不再"每次按键跳一步"） */
    roamSpeed: 1,
    /** 漫游鼠标视角灵敏度（弧度 / 像素位移；游戏式：鼠标右移=向右转、下移=向下看） */
    roamLookSpeed: 45e-4,
    /** 漫游按住 Shift 的加速倍数 */
    roamRunFactor: 3,
    /** 漫游升降速度（**几何半径 / 秒**；Q 降 / E 升） */
    roamLiftSpeed: 0.8
  });
  var LIGHT_DIR = [0.42, 0.78, 0.46];
  var AMBIENT = 0.5;
  var DIFFUSE = 0.5;
  var BACK_FACE_FACTOR = 0.6;
  var BONE_ALPHA = 0.55;
  var BONE_ALPHA_ON_TOP = 1;
  var MESH_PIXEL_STRIDE_MAX = 6;
  var AABB_ALPHA = 0.5;
  var FALLBACK_VIEW_WIDTH = 320;
  var FALLBACK_VIEW_HEIGHT = 240;
  var MIN_RADIUS = 0.5;
  function readNumber2(value, fallback) {
    const n = typeof value === "number" ? value : Number(value);
    return Number.isFinite(n) ? n : fallback;
  }
  __name(readNumber2, "readNumber");
  function readVec3(input) {
    if (input === null || input === void 0) return null;
    if (typeof input.length === "number") {
      const arr = input;
      if (arr.length < 3) return null;
      const x2 = readNumber2(arr[0], NaN);
      const y2 = readNumber2(arr[1], NaN);
      const z2 = readNumber2(arr[2], NaN);
      return Number.isFinite(x2) && Number.isFinite(y2) && Number.isFinite(z2) ? [x2, y2, z2] : null;
    }
    const boxed = input;
    const x = readNumber2(boxed.x, NaN);
    const y = readNumber2(boxed.y, NaN);
    const z = readNumber2(boxed.z, NaN);
    return Number.isFinite(x) && Number.isFinite(y) && Number.isFinite(z) ? [x, y, z] : null;
  }
  __name(readVec3, "readVec3");
  function toFiniteArray(input, label) {
    if (input === null || input === void 0) return new Float64Array(0);
    const length = readNumber2(input.length, -1);
    if (!Number.isInteger(length) || length < 0) {
      throw new MeshViewport3DGeometryError(`${label} \u9700\u8981 ArrayLike<number>\uFF08\u8BFB\u4E0D\u5230\u5408\u6CD5 length\uFF09`);
    }
    const out = new Float64Array(length);
    for (let i = 0; i < length; i += 1) {
      const n = Number(input[i]);
      if (!Number.isFinite(n)) throw new MeshViewport3DGeometryError(`${label}[${i}] \u4E0D\u662F\u6709\u9650\u6570\uFF08${String(input[i])}\uFF09`);
      out[i] = n;
    }
    return out;
  }
  __name(toFiniteArray, "toFiniteArray");
  function aabbFromPositions(positions) {
    if (positions.length < 3) return Object.freeze({ min: [-1, -1, -1], max: [1, 1, 1] });
    let minX = Infinity;
    let minY = Infinity;
    let minZ = Infinity;
    let maxX = -Infinity;
    let maxY = -Infinity;
    let maxZ = -Infinity;
    for (let i = 0; i + 2 < positions.length; i += 3) {
      minX = Math.min(minX, positions[i]);
      minY = Math.min(minY, positions[i + 1]);
      minZ = Math.min(minZ, positions[i + 2]);
      maxX = Math.max(maxX, positions[i]);
      maxY = Math.max(maxY, positions[i + 1]);
      maxZ = Math.max(maxZ, positions[i + 2]);
    }
    return Object.freeze({ min: [minX, minY, minZ], max: [maxX, maxY, maxZ] });
  }
  __name(aabbFromPositions, "aabbFromPositions");
  function normalizeAabb(input, positions) {
    if (input === null || input === void 0) return aabbFromPositions(positions);
    const record = input;
    let min = null;
    let max = null;
    if (record.min !== void 0 && record.max !== void 0) {
      min = readVec3(record.min);
      max = readVec3(record.max);
    } else if (record.center !== void 0 && record.size !== void 0) {
      const center = readVec3(record.center);
      const size = readVec3(record.size);
      if (center !== null && size !== null) {
        min = [center[0] - size[0] / 2, center[1] - size[1] / 2, center[2] - size[2] / 2];
        max = [center[0] + size[0] / 2, center[1] + size[1] / 2, center[2] + size[2] / 2];
      }
    } else if (record.width !== void 0 || record.height !== void 0 || record.depth !== void 0) {
      const x = readNumber2(record.x, NaN);
      const y = readNumber2(record.y, NaN);
      const z = readNumber2(record.z, NaN);
      const w = readNumber2(record.width, NaN);
      const h2 = readNumber2(record.height, NaN);
      const d = readNumber2(record.depth, NaN);
      if (Number.isFinite(x) && Number.isFinite(y) && Number.isFinite(z) && Number.isFinite(w) && Number.isFinite(h2) && Number.isFinite(d)) {
        min = [x, y, z];
        max = [x + w, y + h2, z + d];
      }
    }
    if (min === null || max === null) return aabbFromPositions(positions);
    const lo = [Math.min(min[0], max[0]), Math.min(min[1], max[1]), Math.min(min[2], max[2])];
    const hi = [Math.max(min[0], max[0]), Math.max(min[1], max[1]), Math.max(min[2], max[2])];
    return Object.freeze({ min: lo, max: hi });
  }
  __name(normalizeAabb, "normalizeAabb");
  function normalizeBones(input, vertexCount) {
    if (input === null || input === void 0) return Object.freeze([]);
    if (typeof input.length !== "number") {
      throw new MeshViewport3DGeometryError("bones \u9700\u8981\u6570\u7EC4");
    }
    const out = [];
    for (let i = 0; i < input.length; i += 1) {
      const raw = input[i] === null || input[i] === void 0 ? {} : input[i];
      const source = raw.joints !== void 0 && raw.joints !== null ? raw.joints : raw.from !== void 0 || raw.to !== void 0 ? [raw.from, raw.to] : [];
      const joints = [];
      for (const j of source) {
        const n = Math.trunc(readNumber2(j, NaN));
        if (Number.isFinite(n) && n >= 0 && n < vertexCount) joints.push(n);
      }
      const name = typeof raw.name === "string" && raw.name.length > 0 ? raw.name : `bone-${i}`;
      const parentRaw = raw.parent === void 0 || raw.parent === null ? null : Math.trunc(readNumber2(raw.parent, NaN));
      out.push(Object.freeze({ index: i, name, joints: Object.freeze(joints), parent: Number.isFinite(parentRaw) ? parentRaw : null }));
    }
    return Object.freeze(out);
  }
  __name(normalizeBones, "normalizeBones");
  function normalizeGeometry(input) {
    if (input === null || input === void 0 || typeof input !== "object") {
      throw new MeshViewport3DGeometryError("geometry \u5FC5\u586B\uFF08\u81F3\u5C11\u8981\u6709 positions\uFF09");
    }
    if (input.positions === null || input.positions === void 0) {
      throw new MeshViewport3DGeometryError("positions \u5FC5\u586B");
    }
    const positions = toFiniteArray(input.positions, "positions");
    if (positions.length % 3 !== 0) {
      throw new MeshViewport3DGeometryError(`positions \u957F\u5EA6 ${positions.length} \u4E0D\u662F 3 \u7684\u500D\u6570`);
    }
    const vertexCount = positions.length / 3;
    const rawIndices = toFiniteArray(input.indices ?? null, "indices");
    if (rawIndices.length % 3 !== 0) {
      throw new MeshViewport3DGeometryError(`indices \u957F\u5EA6 ${rawIndices.length} \u4E0D\u662F 3 \u7684\u500D\u6570\uFF08\u672C\u89C6\u53E3\u6309\u4E09\u89D2\u5F62\u7D22\u5F15\u89E3\u91CA\uFF09`);
    }
    const indices = new Uint32Array(rawIndices.length);
    for (let i = 0; i < rawIndices.length; i += 1) {
      const idx = Math.trunc(rawIndices[i]);
      if (idx < 0 || idx >= vertexCount) {
        throw new MeshViewport3DGeometryError(`indices[${i}] = ${idx} \u8D8A\u754C\uFF08\u9876\u70B9\u6570 ${vertexCount}\uFF09`);
      }
      indices[i] = idx;
    }
    const rawUvs = toFiniteArray(input.uvs ?? null, "uvs");
    if (rawUvs.length !== 0 && rawUvs.length !== vertexCount * 2) {
      throw new MeshViewport3DGeometryError(`uvs \u957F\u5EA6 ${rawUvs.length} \u5E94\u4E3A\u9876\u70B9\u6570 \xD7 2 = ${vertexCount * 2}`);
    }
    const uvs = rawUvs.length === 0 ? null : rawUvs;
    const aabb = normalizeAabb(input.aabb ?? null, positions);
    const bones = normalizeBones(input.bones ?? null, vertexCount);
    const declared = input.counts ?? {};
    const counts = Object.freeze({
      vertices: vertexCount > 0 ? vertexCount : Math.max(0, Math.trunc(readNumber2(declared.vertices, 0))),
      triangles: indices.length > 0 ? indices.length / 3 : Math.max(0, Math.trunc(readNumber2(declared.triangles, 0))),
      bones: bones.length > 0 ? bones.length : Math.max(0, Math.trunc(readNumber2(declared.bones, 0)))
    });
    return Object.freeze({ positions, indices, uvs, aabb, counts, bones });
  }
  __name(normalizeGeometry, "normalizeGeometry");
  function normalizeTexture(input) {
    if (input === null || input === void 0) return null;
    const width = Math.max(0, Math.trunc(readNumber2(input.width, 0)));
    const height = Math.max(0, Math.trunc(readNumber2(input.height, 0)));
    const need = width * height * 4;
    let pixels = null;
    if (input.pixels !== null && input.pixels !== void 0 && need > 0) {
      const raw = toFiniteArray(input.pixels, "texture.pixels");
      if (raw.length >= need) {
        let maxV = 0;
        for (let i = 0; i < need; i += 1) {
          const v = raw[i];
          if (v > maxV) maxV = v;
        }
        if (maxV > 1.0000001) {
          const scale = 1 / 255;
          for (let i = 0; i < need; i += 1) raw[i] = raw[i] < 0 ? 0 : raw[i] * scale;
        } else {
          for (let i = 0; i < need; i += 1) if (raw[i] < 0) raw[i] = 0;
        }
        pixels = raw;
      }
    }
    const image = input.image === void 0 ? null : input.image;
    return Object.freeze({
      width,
      height,
      hasPixels: pixels !== null,
      hasImage: image !== null && image !== void 0,
      sampled: false,
      pixels,
      image
    });
  }
  __name(normalizeTexture, "normalizeTexture");
  function projectMeshPoint(point, camera) {
    const cx = point[0] - camera.center[0];
    const cy = point[1] - camera.center[1];
    const cz = point[2] - camera.center[2];
    const m = camera.rot && camera.rot.length === 9 ? camera.rot : quatRotationMatrix(camera.quat ?? quatFromYawPitch(camera.yaw, camera.pitch));
    const x1 = m[0] * cx + m[1] * cy + m[2] * cz;
    const y2 = m[3] * cx + m[4] * cy + m[5] * cz;
    const z2 = m[6] * cx + m[7] * cy + m[8] * cz;
    const depth = z2 + camera.distance;
    const visible = depth > camera.near;
    const safeDepth = visible ? depth : camera.near;
    return Object.freeze({
      x: camera.width / 2 + x1 * camera.focal / safeDepth,
      y: camera.height / 2 - y2 * camera.focal / safeDepth,
      depth,
      visible
    });
  }
  __name(projectMeshPoint, "projectMeshPoint");
  function normalizeVec3(v) {
    const len = Math.sqrt(v[0] * v[0] + v[1] * v[1] + v[2] * v[2]);
    if (!(len > 0)) return null;
    return [v[0] / len, v[1] / len, v[2] / len];
  }
  __name(normalizeVec3, "normalizeVec3");
  function faceNormal(a, b, c) {
    const ux = b[0] - a[0];
    const uy = b[1] - a[1];
    const uz = b[2] - a[2];
    const vx = c[0] - a[0];
    const vy = c[1] - a[1];
    const vz = c[2] - a[2];
    return normalizeVec3([uy * vz - uz * vy, uz * vx - ux * vz, ux * vy - uy * vx]);
  }
  __name(faceNormal, "faceNormal");
  function createMeshViewport3D(options) {
    const opts = options === null || options === void 0 ? {} : options;
    const canvasInput = opts.canvas === void 0 || opts.canvas === null ? null : opts.canvas;
    if (canvasInput === null) throw new MeshViewport3DCanvasError("\u672A\u4F20\u5165 canvas");
    const el = canvasInput;
    let ctx = null;
    try {
      ctx = typeof el.getContext === "function" ? el.getContext("2d") : null;
    } catch (err) {
      throw new MeshViewport3DCanvasError(`getContext("2d") \u629B\u9519\uFF1A${String(err)}`);
    }
    if (ctx === null || ctx === void 0) throw new MeshViewport3DCanvasError('getContext("2d") \u8FD4\u56DE null');
    const disposer = createDisposer(globalThis);
    let geometry = normalizeGeometry(opts.geometry);
    let texture = normalizeTexture(opts.texture ?? null);
    let highlighted = null;
    let yaw = MESH_CAMERA_DEFAULTS.yaw;
    let pitch = MESH_CAMERA_DEFAULTS.pitch;
    let quat = quatFromYawPitch(yaw, pitch);
    let viewMode = opts.viewMode === "roam" ? "roam" : "orbit";
    let roamPosition = null;
    const roamSpeed = Math.max(0.01, readNumber2(opts.roamSpeed, MESH_CAMERA_DEFAULTS.roamSpeed));
    let zoom = 1;
    const fitMargin = Math.max(1.01, readNumber2(opts.fitMargin, MESH_CAMERA_DEFAULTS.fitMargin));
    const drawEdges = opts.edges !== false;
    const drawFaces = opts.fill !== false;
    const bonesOnTop = opts.bonesOnTop !== false;
    const pixelBudget = Math.max(0, Math.trunc(readNumber2(opts.pixelBudget, MESH_PIXEL_BUDGET_DEFAULT)));
    let destroyed = false;
    let lastFrame = null;
    let renderCount = 0;
    let framePending = false;
    let needsRender = false;
    let lastCss = { width: 0, height: 0 };
    let textureWarnReported = false;
    let overlayErrorReported = false;
    let texWrites = 0;
    const overlayInput = opts.overlay === void 0 ? null : opts.overlay;
    const overlayOpts = typeof overlayInput === "function" ? { draw: overlayInput } : overlayInput === null ? {} : overlayInput;
    const overlayDraw = typeof overlayOpts.draw === "function" ? overlayOpts.draw : null;
    const overlayAxes = overlayOpts.axes !== false;
    const overlayBoneLabels = overlayOpts.boneLabels !== false;
    const overlayLabel = typeof overlayOpts.label === "string" && overlayOpts.label.length > 0 ? overlayOpts.label : null;
    if (el.style !== void 0 && el.style !== null) el.style.touchAction = "none";
    const tokenCache = /* @__PURE__ */ new Map();
    function tokenValue(tokenExpression2) {
      const cached = tokenCache.get(tokenExpression2);
      if (cached !== void 0) return cached;
      let value = null;
      try {
        value = resolveTokenValue(el, tokenExpression2).value;
      } catch (err) {
        console.warn(
          "[PetUI mesh-viewport-3d] \u4EE4\u724C\u89E3\u6790\u5931\u8D25\uFF08\u672C\u5E27\u8DF3\u8FC7\u8BE5\u89C6\u89C9\u5143\u7D20\uFF09\uFF1B\u7F3A\u4EE4\u724C\u8BF7\u8D70 --color-n-<hex> \u51FA\u53E3\u6216\u56DE\u62A5 m01\uFF08\u4E0D\u5F97\u81EA\u9020\u4EE4\u724C\u540D\uFF09:",
          tokenExpression2,
          err
        );
        value = null;
      }
      tokenCache.set(tokenExpression2, value);
      return value;
    }
    __name(tokenValue, "tokenValue");
    function tokenLength(tokenExpression2, fallback) {
      const raw = tokenValue(tokenExpression2);
      if (raw === null) return fallback;
      const parsed = parseFloat(raw);
      return Number.isFinite(parsed) && parsed > 0 ? parsed : fallback;
    }
    __name(tokenLength, "tokenLength");
    function deviceDpr() {
      const raw = readNumber2(globalThis.devicePixelRatio, 1);
      const value = raw > 0 ? raw : 1;
      return Math.min(value, 2);
    }
    __name(deviceDpr, "deviceDpr");
    function cssSize() {
      let width = 0;
      let height = 0;
      if (typeof el.getBoundingClientRect === "function") {
        const rect = el.getBoundingClientRect();
        width = readNumber2(rect === null || rect === void 0 ? NaN : rect.width, 0);
        height = readNumber2(rect === null || rect === void 0 ? NaN : rect.height, 0);
      }
      if (width <= 0 || height <= 0) {
        const cw = readNumber2(el.clientWidth, 0);
        const ch = readNumber2(el.clientHeight, 0);
        if (cw > 0 && ch > 0) {
          width = cw;
          height = ch;
        }
      }
      if (width <= 0 || height <= 0) {
        const dpr = deviceDpr();
        const bw = readNumber2(el.width, 0);
        const bh = readNumber2(el.height, 0);
        if (bw > 0 && bh > 0) {
          width = bw / dpr;
          height = bh / dpr;
        }
      }
      if (width <= 0 || height <= 0) {
        width = lastCss.width > 0 ? lastCss.width : FALLBACK_VIEW_WIDTH;
        height = lastCss.height > 0 ? lastCss.height : FALLBACK_VIEW_HEIGHT;
      }
      lastCss = { width, height };
      return { width, height };
    }
    __name(cssSize, "cssSize");
    function makeCamera(size, dpr) {
      const min = geometry.aabb.min;
      const max = geometry.aabb.max;
      const autoCenter = [(min[0] + max[0]) / 2, (min[1] + max[1]) / 2, (min[2] + max[2]) / 2];
      const radius = Math.max(
        MIN_RADIUS,
        Math.hypot(max[0] - min[0], max[1] - min[1], max[2] - min[2]) / 2
      );
      const distance = radius / Math.sin(MESH_CAMERA_DEFAULTS.fovY / 2) * fitMargin;
      let center = autoCenter;
      if (viewMode === "roam" && roamPosition !== null) {
        const rm = quatRotationMatrix(quat);
        center = [
          roamPosition[0] + rm[6] * distance,
          roamPosition[1] + rm[7] * distance,
          roamPosition[2] + rm[8] * distance
        ];
      }
      const focal = Math.min(size.width, size.height) / 2 / Math.tan(MESH_CAMERA_DEFAULTS.fovY / 2) * zoom;
      return Object.freeze({
        center,
        radius,
        distance,
        near: radius * MESH_CAMERA_DEFAULTS.nearFactor,
        yaw,
        pitch,
        quat,
        rot: quatRotationMatrix(quat),
        zoom,
        focal,
        width: size.width,
        height: size.height,
        dpr
      });
    }
    __name(makeCamera, "makeCamera");
    function sampleTextureShade(a, b, c) {
      const tex = texture;
      if (tex === null || !tex.hasPixels || tex.pixels === null) return null;
      const uvs = geometry.uvs;
      if (uvs === null) return null;
      const width = tex.width;
      const height = tex.height;
      if (width <= 0 || height <= 0) return null;
      let sum = 0;
      for (const vertex of [a, b, c]) {
        const u = uvs[vertex * 2];
        const v = uvs[vertex * 2 + 1];
        const px3 = Math.min(width - 1, Math.max(0, Math.round(u * (width - 1))));
        const py = Math.min(height - 1, Math.max(0, Math.round(v * (height - 1))));
        const offset = (py * width + px3) * 4;
        const r = readNumber2(tex.pixels[offset], 0);
        const g = readNumber2(tex.pixels[offset + 1], 0);
        const bl = readNumber2(tex.pixels[offset + 2], 0);
        sum += 0.2126 * r + 0.7152 * g + 0.0722 * bl;
      }
      return Math.min(1, Math.max(0, sum / 3));
    }
    __name(sampleTextureShade, "sampleTextureShade");
    function screenFront(p0, p1, p2) {
      return (p1.x - p0.x) * (p2.y - p0.y) - (p1.y - p0.y) * (p2.x - p0.x) > 0;
    }
    __name(screenFront, "screenFront");
    function rasterizeTexturedTriangle(g, tri, pa, pb, pc, projectedAll, stride) {
      const tex = texture;
      if (tex === null || !tex.hasPixels || tex.pixels === null) return 0;
      const uvs = geometry.uvs;
      if (uvs === null) return 0;
      const tw = tex.width;
      const th = tex.height;
      if (tw <= 0 || th <= 0) return 0;
      const positions = geometry.positions;
      const scene = el;
      const bw = readNumber2(scene.width, 0);
      const bh = readNumber2(scene.height, 0);
      const cssW = lastCss.width > 0 ? lastCss.width : bw;
      const cssH = lastCss.height > 0 ? lastCss.height : bh;
      if (!(cssW > 0) || !(bh > 0) || !(bw > 0) || !(cssH > 0)) return 0;
      const sx = bw / cssW;
      const sy = bh / cssH;
      const x0 = Math.max(0, Math.min(pa.x, pb.x, pc.x));
      const x1 = Math.min(cssW, Math.max(pa.x, pb.x, pc.x));
      const y0 = Math.max(0, Math.min(pa.y, pb.y, pc.y));
      const y1 = Math.min(cssH, Math.max(pa.y, pb.y, pc.y));
      if (!(x1 > x0) || !(y1 > y0)) return 0;
      const x0d = Math.floor(x0 * sx);
      const x1d = Math.ceil(x1 * sx);
      const y0d = Math.floor(y0 * sy);
      const y1d = Math.ceil(y1 * sy);
      const step = Math.max(1, Math.trunc(stride));
      const px0 = Math.min(bw - 1, Math.max(0, x0d));
      const py0 = Math.min(bh - 1, Math.max(0, y0d));
      const cw = Math.min(bw - px0, x1d - px0);
      const chh = Math.min(bh - py0, y1d - py0);
      if (!(cw > 0) || !(chh > 0)) return 0;
      const ctxAny = g;
      if (typeof ctxAny.getImageData !== "function" || typeof ctxAny.putImageData !== "function") return 0;
      let img = null;
      try {
        img = ctxAny.getImageData(px0, py0, cw, chh);
      } catch {
        return 0;
      }
      if (img === null || img.data === void 0) return 0;
      const den = (pb.y - pc.y) * (pa.x - pc.x) + (pc.x - pb.x) * (pa.y - pc.y);
      if (!(Math.abs(den) > 1e-12)) return 0;
      const invDen = 1 / den;
      if (tri.alpha <= 0) return 0;
      const acc = Math.min(1, tri.alpha);
      const back = tri.front ? 1 : BACK_FACE_FACTOR;
      const u0 = uvs[tri.a * 2];
      const v0 = uvs[tri.a * 2 + 1];
      const u1 = uvs[tri.b * 2];
      const v1 = uvs[tri.b * 2 + 1];
      const u2 = uvs[tri.c * 2];
      const v2 = uvs[tri.c * 2 + 1];
      const w0 = positions[tri.a * 3 + 1];
      const w1 = positions[tri.b * 3 + 1];
      const w2 = positions[tri.c * 3 + 1];
      const n0 = positions[tri.a * 3];
      const n1 = positions[tri.b * 3];
      const n2 = positions[tri.c * 3];
      const d0 = projectedAll[tri.a].depth;
      const d1 = projectedAll[tri.b].depth;
      const d2 = projectedAll[tri.c].depth;
      const data = img.data;
      let painted = 0;
      for (let py = py0; py < py0 + chh; py += step) {
        const cy = py / sy;
        for (let px3 = px0; px3 < px0 + cw; px3 += step) {
          const cx = px3 / sx;
          const l0 = ((pb.y - pc.y) * (cx - pc.x) + (pc.x - pb.x) * (cy - pc.y)) * invDen;
          if (l0 < 0) continue;
          const l1 = ((pc.y - pa.y) * (cx - pc.x) + (pa.x - pc.x) * (cy - pc.y)) * invDen;
          if (l1 < 0) continue;
          const l2 = 1 - l0 - l1;
          if (l2 < 0) continue;
          const b0 = l0 / (d0 > 1e-6 ? d0 : 1e-6);
          const b1 = l1 / (d1 > 1e-6 ? d1 : 1e-6);
          const b2 = l2 / (d2 > 1e-6 ? d2 : 1e-6);
          const bs = b0 + b1 + b2;
          const k0 = bs > 0 ? b0 / bs : l0;
          const k1 = bs > 0 ? b1 / bs : l1;
          const k2 = bs > 0 ? b2 / bs : l2;
          const uu = u0 * k0 + u1 * k1 + u2 * k2;
          const vv = v0 * k0 + v1 * k1 + v2 * k2;
          const tx = Math.min(tw - 1, Math.max(0, uu * (tw - 1)));
          const ty = Math.min(th - 1, Math.max(0, vv * (th - 1)));
          const tx0 = Math.floor(tx);
          const ty0 = Math.floor(ty);
          const tx1 = Math.min(tw - 1, tx0 + 1);
          const ty1 = Math.min(th - 1, ty0 + 1);
          const fx = tx - tx0;
          const fy = ty - ty0;
          const o00 = (ty0 * tw + tx0) * 4;
          const o10 = (ty0 * tw + tx1) * 4;
          const o01 = (ty1 * tw + tx0) * 4;
          const o11 = (ty1 * tw + tx1) * 4;
          const rT = ((tex.pixels[o00] ?? 0) * (1 - fx) + (tex.pixels[o10] ?? 0) * fx) * (1 - fy) + ((tex.pixels[o01] ?? 0) * (1 - fx) + (tex.pixels[o11] ?? 0) * fx) * fy;
          const gT = ((tex.pixels[o00 + 1] ?? 0) * (1 - fx) + (tex.pixels[o10 + 1] ?? 0) * fx) * (1 - fy) + ((tex.pixels[o01 + 1] ?? 0) * (1 - fx) + (tex.pixels[o11 + 1] ?? 0) * fx) * fy;
          const bT = ((tex.pixels[o00 + 2] ?? 0) * (1 - fx) + (tex.pixels[o10 + 2] ?? 0) * fx) * (1 - fy) + ((tex.pixels[o01 + 2] ?? 0) * (1 - fx) + (tex.pixels[o11 + 2] ?? 0) * fx) * fy;
          const nrm = perPixelNormal(
            [n0, w0, positions[tri.a * 3 + 2]],
            [n1, w1, positions[tri.b * 3 + 2]],
            [n2, w2, positions[tri.c * 3 + 2]],
            [u0, v0],
            [u1, v1],
            [u2, v2]
          );
          const lambert = nrm === null ? tri.alpha > 0 ? Math.min(1, tri.alpha) : 0 : Math.max(0, nrm[0] * LIGHT_DIR[0] + nrm[1] * LIGHT_DIR[1] + nrm[2] * LIGHT_DIR[2]);
          const lit = AMBIENT + DIFFUSE * lambert;
          const shade = Math.min(1, Math.max(0, lit)) * back;
          const off = ((py - py0) * cw + (px3 - px0)) * 4;
          data[off] = Math.round(rT * shade * 255);
          data[off + 1] = Math.round(gT * shade * 255);
          data[off + 2] = Math.round(bT * shade * 255);
          data[off + 3] = Math.round(acc * 255);
          painted += 1;
        }
      }
      try {
        const live = ctxAny.getImageData?.(px0, py0, cw, chh);
        if (live && live.data) {
          for (let i = 3; i < img.data.length; i += 4) {
            if ((img.data[i] ?? 0) <= 0) continue;
            live.data[i - 3] = img.data[i - 3] ?? 0;
            live.data[i - 2] = img.data[i - 2] ?? 0;
            live.data[i - 1] = img.data[i - 1] ?? 0;
            live.data[i] = 255;
          }
          ctxAny.putImageData?.(live, px0, py0);
        }
      } catch {
      }
      texWrites += 1;
      return painted;
    }
    __name(rasterizeTexturedTriangle, "rasterizeTexturedTriangle");
    function perPixelNormal(p0, p1, p2, uv0, uv1, uv2) {
      const du1 = uv1[0] - uv0[0];
      const dv1 = uv1[1] - uv0[1];
      const du2 = uv2[0] - uv0[0];
      const dv2 = uv2[1] - uv0[1];
      const det = du1 * dv2 - du2 * dv1;
      if (!(Math.abs(det) > 1e-12)) return null;
      const inv = 1 / det;
      const t1 = [(p1[0] - p0[0]) * inv, (p1[1] - p0[1]) * inv, (p1[2] - p0[2]) * inv];
      const t2 = [(p2[0] - p0[0]) * inv, (p2[1] - p0[1]) * inv, (p2[2] - p0[2]) * inv];
      const e1 = [t1[0] * dv2 - t2[0] * dv1, t1[1] * dv2 - t2[1] * dv1, t1[2] * dv2 - t2[2] * dv1];
      const e2 = [t2[0] * du1 - t1[0] * du2, t2[1] * du1 - t1[1] * du2, t2[2] * du1 - t1[2] * du2];
      return normalizeVec3([e1[1] * e2[2] - e1[2] * e2[1], e1[2] * e2[0] - e1[0] * e2[2], e1[0] * e2[1] - e1[1] * e2[0]]);
    }
    __name(perPixelNormal, "perPixelNormal");
    function paintBoneLayer(g, boneFrames, color, lineW, alpha2, dotRadius) {
      if (color === null) return;
      g.strokeStyle = color;
      g.fillStyle = color;
      g.lineWidth = lineW;
      g.globalAlpha = alpha2;
      for (const bf of boneFrames) {
        const pts = bf.points;
        if (pts.length === 0) continue;
        if (pts.length >= 2) {
          g.beginPath();
          g.moveTo(pts[0].x, pts[0].y);
          for (let i = 1; i < pts.length; i += 1) g.lineTo(pts[i].x, pts[i].y);
          g.stroke();
        }
        if (dotRadius > 0) {
          for (const p of pts) {
            g.beginPath();
            g.arc(p.x, p.y, dotRadius, 0, Math.PI * 2);
            g.fill();
          }
        }
      }
      g.globalAlpha = 1;
    }
    __name(paintBoneLayer, "paintBoneLayer");
    function render() {
      if (destroyed) throw new MeshViewport3DDisposedError("render");
      const g = ctx;
      if (g === null) return;
      const dpr = deviceDpr();
      const size = cssSize();
      if (size.width > 0 && size.height > 0) {
        const targetW = Math.max(1, Math.round(size.width * dpr));
        const targetH = Math.max(1, Math.round(size.height * dpr));
        if (el.width !== targetW) el.width = targetW;
        if (el.height !== targetH) el.height = targetH;
      }
      texWrites = 0;
      const camera = makeCamera(size, dpr);
      const positions = geometry.positions;
      const projected = [];
      for (let i = 0; i + 2 < positions.length; i += 3) {
        projected.push(projectMeshPoint([positions[i], positions[i + 1], positions[i + 2]], camera));
      }
      const background = tokenValue(MESH_TOKENS.background);
      const gridColor = tokenValue(MESH_TOKENS.grid);
      const axisColor = tokenValue(MESH_TOKENS.axis);
      const textColor = tokenValue(MESH_TOKENS.text);
      const selectionColor = tokenValue(MESH_TOKENS.selection);
      const series1 = tokenValue(MESH_TOKENS.series1);
      const series2 = tokenValue(MESH_TOKENS.series2);
      const thin = tokenLength(MESH_TOKENS.lineWidth, 1);
      const heavy = tokenLength(MESH_TOKENS.lineWidthHeavy, 2);
      const fontSize = tokenLength(MESH_TOKENS.fontSize, 12);
      g.setTransform(1, 0, 0, 1, 0, 0);
      g.clearRect(0, 0, el.width, el.height);
      if (background !== null) {
        g.fillStyle = background;
        g.fillRect(0, 0, el.width, el.height);
      }
      g.setTransform(dpr, 0, 0, dpr, 0, 0);
      g.imageSmoothingEnabled = true;
      const indices = geometry.indices;
      const order = [];
      const tex = texture;
      const canPixelTex = drawFaces && tex !== null && tex.hasPixels && tex.pixels !== null && geometry.uvs !== null && tex.width > 0 && tex.height > 0;
      let areaSum = 0;
      let texModeCount = { pixel: 0, flat: 0, none: 0 };
      if (drawFaces || drawEdges) {
        for (let t = 0; t + 2 < indices.length; t += 3) {
          const a = indices[t];
          const b = indices[t + 1];
          const c = indices[t + 2];
          const pa = projected[a];
          const pb = projected[b];
          const pc = projected[c];
          const depth = (pa.depth + pb.depth + pc.depth) / 3;
          const front = screenFront(pa, pb, pc);
          const normal = drawFaces ? faceNormal(
            [positions[a * 3], positions[a * 3 + 1], positions[a * 3 + 2]],
            [positions[b * 3], positions[b * 3 + 1], positions[b * 3 + 2]],
            [positions[c * 3], positions[c * 3 + 1], positions[c * 3 + 2]]
          ) : null;
          const lambert = normal === null ? 0 : Math.max(0, normal[0] * LIGHT_DIR[0] + normal[1] * LIGHT_DIR[1] + normal[2] * LIGHT_DIR[2]);
          const shade = AMBIENT + DIFFUSE * lambert;
          const textureShade = drawFaces ? sampleTextureShade(a, b, c) : null;
          const lit = textureShade === null ? shade : shade * (0.45 + 0.55 * textureShade);
          order.push(
            Object.freeze({
              a,
              b,
              c,
              depth,
              front,
              alpha: front ? lit : lit * BACK_FACE_FACTOR,
              textureShade,
              render: drawFaces ? "flat" : "none"
            })
          );
          if (canPixelTex && front) {
            const bx0 = Math.max(0, Math.min(pa.x, pb.x, pc.x));
            const bx1 = Math.min(size.width, Math.max(pa.x, pb.x, pc.x));
            const by0 = Math.max(0, Math.min(pa.y, pb.y, pc.y));
            const by1 = Math.min(size.height, Math.max(pa.y, pb.y, pc.y));
            if (bx1 > bx0 && by1 > by0) areaSum += (bx1 - bx0) * (by1 - by0);
          }
        }
      }
      let texStride = 1;
      let pixelTexActive = canPixelTex;
      if (canPixelTex && areaSum > 0) {
        if (pixelBudget <= 0) {
          texStride = 1;
        } else if (areaSum > pixelBudget) {
          texStride = Math.min(MESH_PIXEL_STRIDE_MAX, Math.ceil(Math.sqrt(areaSum / pixelBudget)));
          if (areaSum / (texStride * texStride) > pixelBudget) pixelTexActive = false;
        }
      }
      order.sort((l, r) => r.depth - l.depth || l.a - r.a || l.b - r.b || l.c - r.c);
      for (let oi = 0; oi < order.length; oi += 1) {
        const tri = order[oi];
        const pa = projected[tri.a];
        const pb = projected[tri.b];
        const pc = projected[tri.c];
        if (drawFaces || drawEdges) {
          const fillColor = tri.front ? series1 : series2;
          if (pixelTexActive && fillColor !== null) {
            const painted = rasterizeTexturedTriangle(g, tri, pa, pb, pc, projected, texStride);
            if (painted > 0) {
              texModeCount.pixel += 1;
              order[oi] = Object.freeze({ ...tri, render: "pixel" });
              continue;
            }
          }
          if (fillColor !== null) {
            g.globalAlpha = Math.min(1, Math.max(0, tri.alpha));
            g.fillStyle = fillColor;
            g.beginPath();
            g.moveTo(pa.x, pa.y);
            g.lineTo(pb.x, pb.y);
            g.lineTo(pc.x, pc.y);
            g.closePath();
            g.fill();
            texModeCount.flat += 1;
          }
        }
        if (drawEdges) {
          g.globalAlpha = 1;
          if (axisColor !== null) {
            g.strokeStyle = axisColor;
            g.lineWidth = thin;
            g.beginPath();
            g.moveTo(pa.x, pa.y);
            g.lineTo(pb.x, pb.y);
            g.lineTo(pc.x, pc.y);
            g.closePath();
            g.stroke();
          }
        }
      }
      g.globalAlpha = 1;
      const boneFrames = [];
      const bones = geometry.bones;
      for (const bone of bones) {
        const points = bone.joints.map((j) => projected[j]);
        const isHighlighted = highlighted === bone.index;
        boneFrames.push(
          Object.freeze({ index: bone.index, name: bone.name, highlighted: isHighlighted, points: Object.freeze(points) })
        );
      }
      const paintSkeleton = /* @__PURE__ */ __name(() => {
        paintBoneLayer(g, boneFrames, axisColor, bonesOnTop ? heavy : thin, bonesOnTop ? BONE_ALPHA_ON_TOP : BONE_ALPHA, 0);
        for (const boneFrame of boneFrames) {
          if (!boneFrame.highlighted) continue;
          const points = boneFrame.points;
          if (selectionColor !== null && points.length >= 1) {
            g.globalAlpha = 1;
            g.strokeStyle = selectionColor;
            g.fillStyle = selectionColor;
            g.lineWidth = heavy;
            g.beginPath();
            g.moveTo(points[0].x, points[0].y);
            for (let i = 1; i < points.length; i += 1) g.lineTo(points[i].x, points[i].y);
            g.stroke();
            for (const p of points) {
              g.beginPath();
              g.arc(p.x, p.y, heavy * 2, 0, Math.PI * 2);
              g.fill();
            }
          }
        }
        g.globalAlpha = 1;
      }, "paintSkeleton");
      if (!bonesOnTop) paintSkeleton();
      if (gridColor !== null) {
        const lo = geometry.aabb.min;
        const hi = geometry.aabb.max;
        const corners = [];
        for (let i = 0; i < 8; i += 1) {
          const x = (i & 1) === 0 ? lo[0] : hi[0];
          const y = (i & 2) === 0 ? lo[1] : hi[1];
          const z = (i & 4) === 0 ? lo[2] : hi[2];
          corners.push(projectMeshPoint([x, y, z], camera));
        }
        const edges = [
          [0, 1],
          [2, 3],
          [4, 5],
          [6, 7],
          [0, 2],
          [1, 3],
          [4, 6],
          [5, 7],
          [0, 4],
          [1, 5],
          [2, 6],
          [3, 7]
        ];
        g.globalAlpha = AABB_ALPHA;
        g.strokeStyle = gridColor;
        g.lineWidth = thin;
        g.beginPath();
        for (const [i, j] of edges) {
          g.moveTo(corners[i].x, corners[i].y);
          g.lineTo(corners[j].x, corners[j].y);
        }
        g.stroke();
        g.globalAlpha = 1;
      }
      if (bonesOnTop) paintSkeleton();
      const labels = [];
      if (textColor !== null) g.fillStyle = textColor;
      if (fontSize > 0) g.font = `${fontSize} sans-serif`;
      if (overlayAxes) {
        const min = geometry.aabb.min;
        const max = geometry.aabb.max;
        const center = [(min[0] + max[0]) / 2, (min[1] + max[1]) / 2, (min[2] + max[2]) / 2];
        const span = Math.max(camera.radius, MIN_RADIUS);
        const axes = [
          { text: "X", tip: [center[0] + span, center[1], center[2]] },
          { text: "Y", tip: [center[0], center[1] + span, center[2]] },
          { text: "Z", tip: [center[0], center[1], center[2] + span] }
        ];
        const origin = projectMeshPoint(center, camera);
        for (const axis of axes) {
          const tip = projectMeshPoint(axis.tip, camera);
          if (axisColor !== null) {
            g.globalAlpha = BONE_ALPHA;
            g.strokeStyle = axisColor;
            g.lineWidth = thin;
            g.beginPath();
            g.moveTo(origin.x, origin.y);
            g.lineTo(tip.x, tip.y);
            g.stroke();
            g.globalAlpha = 1;
          }
          if (textColor !== null && fontSize > 0) {
            g.fillStyle = textColor;
            g.fillText(axis.text, tip.x + heavy, tip.y);
          }
          labels.push(Object.freeze({ text: axis.text, x: tip.x, y: tip.y, kind: "axis" }));
        }
      }
      if (overlayBoneLabels) {
        for (const boneFrame of boneFrames) {
          const first = boneFrame.points[0];
          if (first === void 0) continue;
          const text = boneFrame.highlighted ? `\u25B6 ${boneFrame.name}` : boneFrame.name;
          if (textColor !== null && fontSize > 0) {
            g.fillStyle = textColor;
            g.fillText(text, first.x, first.y);
          }
          labels.push(Object.freeze({ text, x: first.x, y: first.y, kind: "bone" }));
        }
      }
      if (overlayLabel !== null && textColor !== null && fontSize > 0) {
        g.fillStyle = textColor;
        g.fillText(overlayLabel, heavy, fontSize + heavy);
        labels.push(Object.freeze({ text: overlayLabel, x: heavy, y: fontSize + heavy, kind: "caption" }));
      }
      const info = Object.freeze({
        width: size.width,
        height: size.height,
        dpr,
        camera,
        geometry,
        projected: Object.freeze(projected),
        highlightedBone: highlighted,
        counts: geometry.counts
      });
      if (overlayDraw !== null) {
        try {
          overlayDraw(g, info);
        } catch (err) {
          if (!overlayErrorReported) {
            overlayErrorReported = true;
            console.warn("[PetUI mesh-viewport-3d] overlay.draw \u56DE\u8C03\u629B\u9519\uFF0C\u5DF2\u9694\u79BB\uFF08\u53EA\u62A5\u4E00\u6B21\uFF09:", err);
          }
        }
      }
      let textureInfo = null;
      if (texture !== null) {
        const preview2 = tokenLength(MESH_TOKENS.previewSize, 64);
        const canPreview = texture.hasImage && typeof g.drawImage === "function";
        if (canPreview) {
          try {
            g.globalAlpha = 1;
            g.drawImage?.(texture.image, heavy, heavy, preview2, preview2);
          } catch (err) {
            if (!textureWarnReported) {
              textureWarnReported = true;
              console.warn("[PetUI mesh-viewport-3d] \u7EB9\u7406\u9884\u89C8 drawImage \u5931\u8D25\uFF0C\u5DF2\u8DF3\u8FC7:", err);
            }
          }
        }
        textureInfo = Object.freeze({
          width: texture.width,
          height: texture.height,
          hasPixels: texture.hasPixels,
          hasImage: texture.hasImage,
          // `sampled` = 本帧真的有三角形走了**逐像素**采样（旧语义是"有像素且有 uv"，会把
          // "有贴图但没画上一个像素"也报成 true ⇒ 宿主据此判断"贴图已生效"会误判）
          sampled: texModeCount.pixel > 0,
          mode: texModeCount.pixel > 0 ? "pixel" : texture.hasPixels && geometry.uvs !== null ? "flat" : "none",
          stride: texStride
        });
      }
      g.globalAlpha = 1;
      g.setTransform(1, 0, 0, 1, 0, 0);
      renderCount += 1;
      lastFrame = Object.freeze({
        width: size.width,
        height: size.height,
        dpr,
        renderCount,
        camera,
        projected: info.projected,
        triangles: Object.freeze(order),
        bones: Object.freeze(boneFrames),
        highlightedBone: highlighted,
        texture: textureInfo,
        labels: Object.freeze(labels),
        counts: geometry.counts
      });
    }
    __name(render, "render");
    function invalidate() {
      if (destroyed) return;
      needsRender = true;
      if (framePending) return;
      framePending = true;
      disposer.raf(() => {
        framePending = false;
        if (destroyed || !needsRender) return;
        needsRender = false;
        render();
      });
    }
    __name(invalidate, "invalidate");
    function setGeometry(input) {
      if (destroyed) throw new MeshViewport3DDisposedError("setGeometry");
      geometry = normalizeGeometry(input);
      highlighted = null;
      yaw = MESH_CAMERA_DEFAULTS.yaw;
      pitch = MESH_CAMERA_DEFAULTS.pitch;
      quat = quatFromYawPitch(yaw, pitch);
      roamPosition = null;
      zoom = 1;
      lastFrame = null;
      render();
    }
    __name(setGeometry, "setGeometry");
    function setTexture(input) {
      if (destroyed) throw new MeshViewport3DDisposedError("setTexture");
      texture = normalizeTexture(input);
      textureWarnReported = false;
      render();
    }
    __name(setTexture, "setTexture");
    function highlightBone(index) {
      if (destroyed) throw new MeshViewport3DDisposedError("highlightBone");
      if (index === null || index === void 0) {
        highlighted = null;
        render();
        return;
      }
      const n = Math.trunc(readNumber2(index, NaN));
      if (!Number.isFinite(n) || n < 0 || n >= geometry.bones.length) throw new MeshBoneRangeError(n, geometry.bones.length);
      highlighted = n;
      render();
    }
    __name(highlightBone, "highlightBone");
    function resetView() {
      if (destroyed) throw new MeshViewport3DDisposedError("resetView");
      yaw = MESH_CAMERA_DEFAULTS.yaw;
      pitch = MESH_CAMERA_DEFAULTS.pitch;
      quat = quatFromYawPitch(yaw, pitch);
      if (viewMode === "roam") roamPosition = null;
      zoom = 1;
      render();
    }
    __name(resetView, "resetView");
    function standPoint() {
      if (viewMode === "roam" && roamPosition !== null) return roamPosition;
      const min = geometry.aabb.min;
      const max = geometry.aabb.max;
      return [(min[0] + max[0]) / 2, (min[1] + max[1]) / 2, (min[2] + max[2]) / 2];
    }
    __name(standPoint, "standPoint");
    const heldDirs = /* @__PURE__ */ new Set();
    let moveLoopPending = false;
    let lastMoveTs = 0;
    const TAP_SECONDS = 1 / 60;
    function nowMs() {
      const perf = globalThis.performance;
      return perf !== void 0 && typeof perf.now === "function" ? perf.now() : Date.now();
    }
    __name(nowMs, "nowMs");
    const clockNow = typeof opts.now === "function" ? opts.now : nowMs;
    function roamMoveBy(dtSeconds, run) {
      if (destroyed || heldDirs.size === 0) return;
      const m = quatRotationMatrix(quat);
      const fx = m[6], fy = m[7], fz = m[8];
      const rx = m[0], ry = m[1], rz = m[2];
      let dx = 0, dy = 0, dz = 0, lift = 0;
      if (heldDirs.has("forward")) {
        dx += fx;
        dy += fy;
        dz += fz;
      }
      if (heldDirs.has("back")) {
        dx -= fx;
        dy -= fy;
        dz -= fz;
      }
      if (heldDirs.has("right")) {
        dx += rx;
        dy += ry;
        dz += rz;
      }
      if (heldDirs.has("left")) {
        dx -= rx;
        dy -= ry;
        dz -= rz;
      }
      if (heldDirs.has("up")) lift += 1;
      if (heldDirs.has("down")) lift -= 1;
      const scale = Math.max(MIN_RADIUS, geometry.aabb.max[0] - geometry.aabb.min[0]);
      const mult = run ? MESH_CAMERA_DEFAULTS.roamRunFactor : 1;
      const len = Math.hypot(dx, dy, dz);
      if (len > 0) {
        const v = roamSpeed * scale * mult * dtSeconds / len;
        dx *= v;
        dy *= v;
        dz *= v;
      }
      const liftDelta = lift === 0 ? 0 : MESH_CAMERA_DEFAULTS.roamLiftSpeed * scale * mult * dtSeconds * lift;
      if (len === 0 && liftDelta === 0) return;
      const base = standPoint();
      roamPosition = [base[0] + dx, base[1] + dy + liftDelta, base[2] + dz];
      invalidate();
    }
    __name(roamMoveBy, "roamMoveBy");
    function ensureMoveLoop() {
      if (destroyed || moveLoopPending) return;
      moveLoopPending = true;
      lastMoveTs = clockNow();
      disposer.raf(() => {
        moveLoopPending = false;
        if (destroyed) return;
        const now = clockNow();
        const dt = Math.min(0.1, Math.max(0, (now - lastMoveTs) / 1e3));
        lastMoveTs = now;
        if (heldDirs.size === 0) return;
        roamMoveBy(dt, shiftHeld);
        ensureMoveLoop();
      });
    }
    __name(ensureMoveLoop, "ensureMoveLoop");
    let shiftHeld = false;
    const ROAM_KEYS = {
      w: "forward",
      W: "forward",
      ArrowUp: "forward",
      s: "back",
      S: "back",
      ArrowDown: "back",
      a: "left",
      A: "left",
      ArrowLeft: "left",
      d: "right",
      D: "right",
      ArrowRight: "right",
      // 升降（自由相机惯例：Q 降 / E 升；PageUp/PageDown 亦可）
      e: "up",
      E: "up",
      PageUp: "up",
      q: "down",
      Q: "down",
      PageDown: "down"
    };
    function onRoamKeyDown(ev) {
      if (destroyed || viewMode !== "roam") return;
      const ke = ev;
      const dir = ROAM_KEYS[ke.key];
      if (dir === void 0) return;
      if (ke.shiftKey === true) shiftHeld = true;
      if (typeof ke.preventDefault === "function") ke.preventDefault();
      if (heldDirs.has(dir)) return;
      heldDirs.add(dir);
      roamMoveBy(TAP_SECONDS, shiftHeld);
      ensureMoveLoop();
    }
    __name(onRoamKeyDown, "onRoamKeyDown");
    function onRoamKeyUp(ev) {
      if (destroyed) return;
      const ke = ev;
      const dir = ROAM_KEYS[ke.key];
      if (dir !== void 0) heldDirs.delete(dir);
      if (ke.shiftKey !== true) shiftHeld = false;
    }
    __name(onRoamKeyUp, "onRoamKeyUp");
    function clearHeldDirs() {
      heldDirs.clear();
      shiftHeld = false;
    }
    __name(clearHeldDirs, "clearHeldDirs");
    function setViewMode(mode) {
      if (destroyed) throw new MeshViewport3DDisposedError("setViewMode");
      if (mode !== "orbit" && mode !== "roam") return;
      viewMode = mode;
      if (mode === "roam") {
        if (typeof el.tabIndex === "number") {
          el.tabIndex = 0;
        }
      } else {
        roamPosition = null;
      }
      invalidate();
    }
    __name(setViewMode, "setViewMode");
    function setPosition(point) {
      if (destroyed) throw new MeshViewport3DDisposedError("setPosition");
      roamPosition = point === null ? null : [point[0], point[1], point[2]];
      invalidate();
    }
    __name(setPosition, "setPosition");
    function destroy() {
      if (destroyed) return;
      destroyed = true;
      tokenCache.clear();
      try {
        disposer.dispose();
      } catch (err) {
        console.error("[PetUI mesh-viewport-3d] destroy() \u671F\u95F4\u6709\u91CA\u653E\u52A8\u4F5C\u5931\u8D25\uFF08\u5DF2\u7531 disposer \u9010\u4E2A\u4E0A\u62A5\uFF09:", err);
      }
      needsRender = false;
      framePending = false;
      heldDirs.clear();
      shiftHeld = false;
      ctx = null;
    }
    __name(destroy, "destroy");
    function wheelDelta(ev) {
      return readNumber2(ev.deltaY, 0);
    }
    __name(wheelDelta, "wheelDelta");
    function onWheel(ev) {
      if (destroyed) return;
      const delta = wheelDelta(ev);
      if (delta === 0) return;
      if (typeof ev.preventDefault === "function" && ev.cancelable !== false) {
        ev.preventDefault();
      }
      const next = zoom * Math.pow(2, -delta / 400);
      zoom = Math.min(MESH_CAMERA_DEFAULTS.zoomMax, Math.max(MESH_CAMERA_DEFAULTS.zoomMin, next));
      invalidate();
    }
    __name(onWheel, "onWheel");
    let drag = null;
    function eventPoint(ev) {
      return { x: readNumber2(ev.clientX, 0), y: readNumber2(ev.clientY, 0) };
    }
    __name(eventPoint, "eventPoint");
    function onPointerDown(ev) {
      if (destroyed) return;
      if (viewMode === "roam" && !isPointerLocked()) requestRoamLock();
      drag = eventPoint(ev);
    }
    __name(onPointerDown, "onPointerDown");
    function isPointerLocked() {
      const d = globalThis.document;
      return d !== void 0 && d !== null && d.pointerLockElement === el;
    }
    __name(isPointerLocked, "isPointerLocked");
    function requestRoamLock() {
      const anyEl = el;
      if (typeof anyEl.requestPointerLock !== "function") return;
      try {
        anyEl.requestPointerLock();
      } catch (err) {
        console.debug("[PetUI mesh-viewport-3d] \u6307\u9488\u9501\u5B9A\u88AB\u62D2\uFF08\u9000\u56DE\u6309\u4F4F\u62D6\u52A8\uFF09:", err);
      }
    }
    __name(requestRoamLock, "requestRoamLock");
    function applyLookDelta(dYaw, dPitch) {
      const lo = -MESH_CAMERA_DEFAULTS.pitchLimit;
      const hi = MESH_CAMERA_DEFAULTS.pitchLimit;
      const nextYaw = yaw + dYaw;
      const nextPitch = Math.max(lo, Math.min(hi, pitch + dPitch));
      if (nextYaw === yaw && nextPitch === pitch) return;
      yaw = nextYaw;
      pitch = nextPitch;
      quat = quatFromYawPitch(yaw, pitch);
    }
    __name(applyLookDelta, "applyLookDelta");
    function look(dx, dy) {
      if (destroyed) throw new MeshViewport3DDisposedError("look");
      const s = MESH_CAMERA_DEFAULTS.roamLookSpeed;
      applyLookDelta(dx * s, -dy * s);
      invalidate();
    }
    __name(look, "look");
    function onPointerMove(ev) {
      if (destroyed) return;
      if (viewMode === "roam" && isPointerLocked()) {
        const me = ev;
        look(readNumber2(me.movementX, 0), readNumber2(me.movementY, 0));
        return;
      }
      if (drag === null) return;
      const p = eventPoint(ev);
      applyLookDelta(
        (p.x - drag.x) * MESH_CAMERA_DEFAULTS.orbitSpeed,
        (p.y - drag.y) * MESH_CAMERA_DEFAULTS.orbitSpeed
      );
      drag = p;
      invalidate();
    }
    __name(onPointerMove, "onPointerMove");
    function onPointerUp() {
      drag = null;
    }
    __name(onPointerUp, "onPointerUp");
    const usePointerEvents = typeof globalThis.PointerEvent === "function";
    if (usePointerEvents) {
      disposer.addEventListener(el, "pointerdown", onPointerDown);
      disposer.addEventListener(el, "pointermove", onPointerMove);
      disposer.addEventListener(el, "pointerup", onPointerUp);
      disposer.addEventListener(el, "pointercancel", onPointerUp);
    } else {
      disposer.addEventListener(el, "mousedown", onPointerDown);
      disposer.addEventListener(el, "mousemove", onPointerMove);
      disposer.addEventListener(el, "mouseup", onPointerUp);
      disposer.addEventListener(el, "mouseleave", onPointerUp);
    }
    disposer.addEventListener(el, "keydown", onRoamKeyDown);
    disposer.addEventListener(el, "keyup", onRoamKeyUp);
    {
      const winForBlur = typeof document !== "undefined" && document.defaultView ? document.defaultView : null;
      if (winForBlur !== null && typeof winForBlur.addEventListener === "function") {
        disposer.addEventListener(winForBlur, "blur", clearHeldDirs);
      }
    }
    disposer.addEventListener(el, "wheel", onWheel, { passive: false });
    const globalObject = globalThis;
    let observerAttached = false;
    if (typeof globalObject.ResizeObserver === "function") {
      try {
        const ro = new globalObject.ResizeObserver(() => invalidate());
        ro.observe(el);
        disposer.observe(ro);
        observerAttached = true;
      } catch (err) {
        console.warn("[PetUI mesh-viewport-3d] ResizeObserver \u5EFA\u7ACB\u5931\u8D25\uFF0C\u9000\u5316\u5230 window.resize:", err);
      }
    }
    if (!observerAttached) {
      const win = typeof window === "undefined" ? null : window;
      if (win !== null && typeof win.addEventListener === "function") {
        disposer.addEventListener(win, "resize", () => invalidate());
      } else {
        console.warn("[PetUI mesh-viewport-3d] \u65E0 ResizeObserver \u4E5F\u65E0 window\uFF1A\u5BBF\u4E3B\u5C3A\u5BF8\u53D8\u5316\u4E0D\u4F1A\u81EA\u52A8\u91CD\u7ED8\uFF08\u53EF\u624B\u52A8 render\uFF09");
      }
    }
    const themeUnsub = onThemeChange(() => {
      tokenCache.clear();
      invalidate();
    });
    disposer.add(() => {
      try {
        themeUnsub();
      } catch (err) {
        console.warn("[PetUI mesh-viewport-3d] \u4E3B\u9898\u9000\u8BA2\u5931\u8D25:", err);
      }
    });
    render();
    const target = el;
    const defineGetter3 = /* @__PURE__ */ __name((name, get) => {
      Object.defineProperty(target, name, { get, enumerable: true, configurable: true });
    }, "defineGetter");
    const defineMethod = /* @__PURE__ */ __name((name, fn) => {
      Object.defineProperty(target, name, { value: fn, enumerable: true, writable: false, configurable: false });
    }, "defineMethod");
    defineGetter3("geometry", () => geometry);
    defineGetter3("camera", () => lastFrame === null ? makeCamera(cssSize(), deviceDpr()) : lastFrame.camera);
    defineGetter3("frame", () => lastFrame);
    defineGetter3("highlightedBone", () => highlighted);
    defineGetter3("texture", () => lastFrame === null || lastFrame.texture === null ? null : lastFrame.texture);
    defineGetter3("destroyed", () => destroyed);
    defineGetter3("viewMode", () => viewMode);
    defineGetter3("roamPosition", () => roamPosition === null ? null : [...roamPosition]);
    defineMethod("setViewMode", setViewMode);
    defineMethod("setPosition", setPosition);
    defineMethod("look", look);
    defineMethod("setGeometry", setGeometry);
    defineMethod("setTexture", setTexture);
    defineMethod("render", render);
    defineMethod("highlightBone", highlightBone);
    defineMethod("resetView", resetView);
    defineMethod("destroy", destroy);
    return target;
  }
  __name(createMeshViewport3D, "createMeshViewport3D");

  // src/components/webgl-viewport.ts
  var TOKENS = {
    clear: "--canvas-background",
    fallbackSurface: "--canvas-surface",
    fallbackText: "--canvas-text",
    shade: "--alpha-shade",
    space: "--space-3",
    fontSize: "--font-size-12"
  };
  var FALLBACK_TEXT = {
    "webgl-unavailable": "WebGL \u4E0D\u53EF\u7528\uFF1A\u5F53\u524D\u73AF\u5883\u672A\u80FD\u53D6\u5F97 webgl \u4E0A\u4E0B\u6587\uFF0C\u89C6\u53E3\u5DF2\u964D\u7EA7\uFF08\u4E0D\u6E32\u67D3\uFF09\u3002",
    "context-lost": "WebGL \u4E0A\u4E0B\u6587\u4E22\u5931\uFF1AGPU \u4E0A\u4E0B\u6587\u88AB\u56DE\u6536\uFF0C\u89C6\u53E3\u5DF2\u964D\u7EA7\uFF0C\u7B49\u5F85\u81EA\u52A8\u6062\u590D\u3002",
    "shader-compile-failed": "WebGL \u7740\u8272\u5668\u7F16\u8BD1\u5931\u8D25\uFF1A\u89C6\u53E3\u5DF2\u964D\u7EA7\uFF08\u4E0D\u6E32\u67D3\uFF09\u3002",
    "program-link-failed": "WebGL \u7A0B\u5E8F\u94FE\u63A5\u5931\u8D25\uFF1A\u89C6\u53E3\u5DF2\u964D\u7EA7\uFF08\u4E0D\u6E32\u67D3\uFF09\u3002",
    "gl-error": "WebGL \u8FD0\u884C\u671F\u9519\u8BEF\uFF1A\u89C6\u53E3\u5DF2\u964D\u7EA7\uFF08\u4E0D\u6E32\u67D3\uFF09\u3002"
  };
  function mat4() {
    const m = new Float32Array(16);
    m[0] = m[5] = m[10] = m[15] = 1;
    return m;
  }
  __name(mat4, "mat4");
  function perspective(out, fovyRad, aspect, near, far) {
    const f = 1 / Math.tan(fovyRad / 2);
    out.fill(0);
    out[0] = f / aspect;
    out[5] = f;
    out[10] = (far + near) / (near - far);
    out[11] = -1;
    out[14] = 2 * far * near / (near - far);
    return out;
  }
  __name(perspective, "perspective");
  function lookAt(out, eye, center, up) {
    let zx = eye[0] - center[0];
    let zy = eye[1] - center[1];
    let zz = eye[2] - center[2];
    let len = Math.hypot(zx, zy, zz) || 1;
    zx /= len;
    zy /= len;
    zz /= len;
    let xx = up[1] * zz - up[2] * zy;
    let xy = up[2] * zx - up[0] * zz;
    let xz = up[0] * zy - up[1] * zx;
    len = Math.hypot(xx, xy, xz) || 1;
    xx /= len;
    xy /= len;
    xz /= len;
    const yx = zy * xz - zz * xy;
    const yy = zz * xx - zx * xz;
    const yz = zx * xy - zy * xx;
    out[0] = xx;
    out[1] = yx;
    out[2] = zx;
    out[3] = 0;
    out[4] = xy;
    out[5] = yy;
    out[6] = zy;
    out[7] = 0;
    out[8] = xz;
    out[9] = yz;
    out[10] = zz;
    out[11] = 0;
    out[12] = -(xx * eye[0] + xy * eye[1] + xz * eye[2]);
    out[13] = -(yx * eye[0] + yy * eye[1] + yz * eye[2]);
    out[14] = -(zx * eye[0] + zy * eye[1] + zz * eye[2]);
    out[15] = 1;
    return out;
  }
  __name(lookAt, "lookAt");
  var VERT_SRC = [
    "attribute vec3 aPosition;",
    "attribute vec3 aColor;",
    "attribute vec3 aNormal;",
    "uniform mat4 uProjection;",
    "uniform mat4 uView;",
    "uniform float uVScale;",
    "uniform vec3 uLightDir;",
    "uniform float uAmbient;",
    "varying vec3 vColor;",
    "varying float vLight;",
    "void main() {",
    "  vec3 p = aPosition * uVScale;",
    "  vec3 n = normalize(aNormal);",
    "  float d = max(dot(n, normalize(uLightDir)), 0.0);",
    "  vLight = mix(uAmbient, 1.0, d);",
    "  vColor = aColor;",
    "  gl_Position = uProjection * uView * vec4(p, 1.0);",
    "}"
  ].join("\n");
  var FRAG_SRC = [
    "precision mediump float;",
    "varying vec3 vColor;",
    "varying float vLight;",
    "void main() {",
    "  gl_FragColor = vec4(vColor * vLight, 1.0);",
    "}"
  ].join("\n");
  function defineGetter(target, name, get) {
    Object.defineProperty(target, name, { get, enumerable: true, configurable: true });
  }
  __name(defineGetter, "defineGetter");
  function toFloat32(src) {
    if (!src) return new Float32Array(0);
    if (src instanceof Float32Array) return src;
    return Float32Array.from(src);
  }
  __name(toFloat32, "toFloat32");
  function normalizeColors(src, vertexCount) {
    const raw = toFloat32(src);
    if (raw.length === 0) {
      const fill = new Float32Array(vertexCount * 3);
      fill.fill(1);
      return fill;
    }
    let max = 0;
    for (let i = 0; i < raw.length; i += 1) if (raw[i] > max) max = raw[i];
    if (max <= 1.5) return raw;
    const out = new Float32Array(raw.length);
    for (let i = 0; i < raw.length; i += 1) out[i] = raw[i] / 255;
    return out;
  }
  __name(normalizeColors, "normalizeColors");
  function normalizeNormals(src, vertexCount) {
    const raw = toFloat32(src);
    if (raw.length > 0) return raw;
    const fill = new Float32Array(vertexCount * 3);
    for (let i = 1; i < fill.length; i += 3) fill[i] = 1;
    return fill;
  }
  __name(normalizeNormals, "normalizeNormals");
  function toIndexArray(src, preferUint32) {
    if (!src || src.length === 0) return new Uint16Array(0);
    if (src instanceof Uint16Array || src instanceof Uint32Array || src instanceof Uint8Array) return src;
    return preferUint32 ? Uint32Array.from(src) : Uint16Array.from(src);
  }
  __name(toIndexArray, "toIndexArray");
  function maxOf(src) {
    let m = 0;
    for (let i = 0; i < src.length; i += 1) if (src[i] > m) m = src[i];
    return m;
  }
  __name(maxOf, "maxOf");
  var HEX_COLOR_RE = /^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$/;
  var RGB_COLOR_RE = /^rgba?\(([^)]+)\)$/i;
  function parseColor(value) {
    if (!value) return null;
    const hex = HEX_COLOR_RE.exec(value);
    if (hex) {
      let h2 = hex[1];
      if (h2.length === 3) h2 = h2[0] + h2[0] + h2[1] + h2[1] + h2[2] + h2[2];
      return [
        parseInt(h2.slice(0, 2), 16) / 255,
        parseInt(h2.slice(2, 4), 16) / 255,
        parseInt(h2.slice(4, 6), 16) / 255,
        h2.length === 8 ? parseInt(h2.slice(6, 8), 16) / 255 : 1
      ];
    }
    const rgb = RGB_COLOR_RE.exec(value);
    if (!rgb) return null;
    const parts = rgb[1].split(/[\s,/]+/).filter((s) => s.length > 0);
    if (parts.length < 3) return null;
    const chan = /* @__PURE__ */ __name((s) => s.endsWith("%") ? parseFloat(s) / 100 : parseFloat(s) / 255, "chan");
    const a = parts.length >= 4 ? parseFloat(parts[3]) : 1;
    const out = [chan(parts[0]), chan(parts[1]), chan(parts[2]), a];
    for (let i = 0; i < out.length; i += 1) if (!isFinite(out[i])) return null;
    return out;
  }
  __name(parseColor, "parseColor");
  function createWebGLViewport(options) {
    const { canvas } = options;
    const disposer = createDisposer();
    const root = document.createElement("div");
    root.className = "ui-webgl-viewport";
    root.setAttribute("data-webgl-viewport", "");
    root.style.position = "relative";
    root.style.display = "block";
    root.style.background = `var(${TOKENS.clear})`;
    root.style.contain = "layout paint";
    const overlay = document.createElement("div");
    overlay.className = "ui-webgl-viewport__fallback";
    overlay.setAttribute("data-fallback", "");
    overlay.hidden = true;
    overlay.style.position = "absolute";
    overlay.style.inset = "0";
    overlay.style.display = "none";
    overlay.style.alignItems = "center";
    overlay.style.justifyContent = "center";
    overlay.style.textAlign = "center";
    overlay.style.padding = `var(${TOKENS.space})`;
    overlay.style.background = `var(${TOKENS.fallbackSurface})`;
    overlay.style.color = `var(${TOKENS.fallbackText})`;
    overlay.style.fontSize = `var(${TOKENS.fontSize})`;
    canvas.style.display = "block";
    canvas.style.width = "100%";
    canvas.style.height = "100%";
    root.appendChild(canvas);
    root.appendChild(overlay);
    let mesh = options.mesh;
    let params = { ...options.params };
    let gl = null;
    let program = null;
    let shaders = [];
    let buffers = [];
    let geom = { pos: null, nrm: null, col: null, tri: null, line: null };
    let attribs = {};
    let uniforms = {};
    let indexInfo = { tris: 0, lines: 0, triType: 0, lineType: 0 };
    let fallbackReason = null;
    let destroyed = false;
    let drawCalls = 0;
    let listeners = 0;
    const missingTokens = [];
    const unresolvedColors = [];
    let clearColor = null;
    let ambient = 1;
    const proj = mat4();
    const view = mat4();
    const lightDir = new Float32Array([0.35, 0.65, 0.68]);
    const camera = {
      eye: [0, 0, 3],
      target: [0, 0, 0],
      up: [0, 1, 0],
      fovDeg: 45,
      near: 0.05,
      far: 100
    };
    function readTokenValue(token) {
      let value = "";
      try {
        value = resolveTokenValue(root, `var(${token})`).value;
      } catch {
        value = "";
      }
      const trimmed = String(value === null || value === void 0 ? "" : value).trim();
      if (!trimmed || trimmed.indexOf("var(") === 0) {
        if (missingTokens.indexOf(token) === -1) missingTokens.push(token);
        return "";
      }
      return trimmed;
    }
    __name(readTokenValue, "readTokenValue");
    function syncTokens() {
      const rawColor = readTokenValue(TOKENS.clear);
      clearColor = rawColor ? parseColor(rawColor) : null;
      if (rawColor && !clearColor && unresolvedColors.indexOf(rawColor) === -1) {
        unresolvedColors.push(rawColor);
      }
      const shade = parseColor(readTokenValue(TOKENS.shade));
      ambient = shade === null ? 1 : Math.min(1, Math.max(0, 1 - shade[3]));
      root.setAttribute("data-missing-tokens", missingTokens.join(" "));
    }
    __name(syncTokens, "syncTokens");
    function setFallback(reason, detail) {
      fallbackReason = reason;
      root.setAttribute("data-fallback", reason);
      overlay.hidden = false;
      overlay.style.display = "flex";
      overlay.setAttribute("data-fallback-reason", reason);
      const base = FALLBACK_TEXT[reason] || FALLBACK_TEXT["gl-error"];
      overlay.textContent = detail ? `${base} (${detail})` : base;
    }
    __name(setFallback, "setFallback");
    function clearFallback() {
      fallbackReason = null;
      root.setAttribute("data-fallback", "");
      overlay.hidden = true;
      overlay.style.display = "none";
      overlay.textContent = "";
      overlay.removeAttribute("data-fallback-reason");
    }
    __name(clearFallback, "clearFallback");
    function compile2(type, src) {
      if (!gl) return null;
      const sh = gl.createShader(type);
      if (!sh) {
        setFallback("shader-compile-failed", "createShader-null");
        return null;
      }
      gl.shaderSource(sh, src);
      gl.compileShader(sh);
      if (!gl.getShaderParameter(sh, gl.COMPILE_STATUS)) {
        const log = gl.getShaderInfoLog ? gl.getShaderInfoLog(sh) : "";
        gl.deleteShader(sh);
        setFallback("shader-compile-failed", log ? String(log).slice(0, 120) : void 0);
        return null;
      }
      return sh;
    }
    __name(compile2, "compile");
    function releaseGL() {
      if (!gl) return;
      for (let i = 0; i < buffers.length; i += 1) gl.deleteBuffer(buffers[i]);
      buffers = [];
      geom = { pos: null, nrm: null, col: null, tri: null, line: null };
      if (program) {
        gl.deleteProgram(program);
        program = null;
      }
      for (let i = 0; i < shaders.length; i += 1) gl.deleteShader(shaders[i]);
      shaders = [];
      attribs = {};
      uniforms = {};
    }
    __name(releaseGL, "releaseGL");
    function buildProgram() {
      if (!gl) return false;
      const vs = compile2(gl.VERTEX_SHADER, VERT_SRC);
      const fs = compile2(gl.FRAGMENT_SHADER, FRAG_SRC);
      if (!vs || !fs) return false;
      shaders = [vs, fs];
      const prog = gl.createProgram();
      if (!prog) {
        setFallback("program-link-failed", "createProgram-null");
        return false;
      }
      gl.attachShader(prog, vs);
      gl.attachShader(prog, fs);
      gl.linkProgram(prog);
      if (!gl.getProgramParameter(prog, gl.LINK_STATUS)) {
        const log = gl.getProgramInfoLog ? gl.getProgramInfoLog(prog) : "";
        gl.deleteProgram(prog);
        setFallback("program-link-failed", log ? String(log).slice(0, 120) : void 0);
        return false;
      }
      program = prog;
      attribs = {
        aPosition: gl.getAttribLocation(prog, "aPosition"),
        aColor: gl.getAttribLocation(prog, "aColor"),
        aNormal: gl.getAttribLocation(prog, "aNormal")
      };
      uniforms = {
        uProjection: gl.getUniformLocation(prog, "uProjection"),
        uView: gl.getUniformLocation(prog, "uView"),
        uVScale: gl.getUniformLocation(prog, "uVScale"),
        uLightDir: gl.getUniformLocation(prog, "uLightDir"),
        uAmbient: gl.getUniformLocation(prog, "uAmbient")
      };
      return true;
    }
    __name(buildProgram, "buildProgram");
    function uploadArrayBuffer(data) {
      if (!gl) return null;
      const buf = gl.createBuffer();
      if (!buf) return null;
      buffers.push(buf);
      gl.bindBuffer(gl.ARRAY_BUFFER, buf);
      gl.bufferData(gl.ARRAY_BUFFER, data, gl.STATIC_DRAW);
      return buf;
    }
    __name(uploadArrayBuffer, "uploadArrayBuffer");
    function uploadIndexBuffer(arr) {
      if (!gl) return null;
      const buf = gl.createBuffer();
      if (!buf) return null;
      buffers.push(buf);
      gl.bindBuffer(gl.ELEMENT_ARRAY_BUFFER, buf);
      gl.bufferData(gl.ELEMENT_ARRAY_BUFFER, arr, gl.STATIC_DRAW);
      return buf;
    }
    __name(uploadIndexBuffer, "uploadIndexBuffer");
    function uploadMesh() {
      if (!gl || !program) return false;
      const vertexCount = Math.floor(mesh.pos.length / 3);
      if (vertexCount === 0) {
        indexInfo = { tris: 0, lines: 0, triType: 0, lineType: 0 };
        root.setAttribute("data-empty-mesh", "");
        return false;
      }
      root.removeAttribute("data-empty-mesh");
      const posBuf = uploadArrayBuffer(toFloat32(mesh.pos));
      const nrmBuf = uploadArrayBuffer(normalizeNormals(mesh.nrm, vertexCount));
      const colBuf = uploadArrayBuffer(normalizeColors(mesh.col, vertexCount));
      if (!posBuf || !nrmBuf || !colBuf) {
        setFallback("gl-error", "array-buffer-alloc-failed");
        return false;
      }
      geom.pos = posBuf;
      geom.nrm = nrmBuf;
      geom.col = colBuf;
      let triArr = new Uint16Array(0);
      let lineArr = new Uint16Array(0);
      if (mesh.idx && mesh.idx.length > 0) {
        triArr = toIndexArray(mesh.idx, maxOf(mesh.idx) > 65535);
        geom.tri = uploadIndexBuffer(triArr);
        if (!geom.tri) {
          setFallback("gl-error", "index-buffer-alloc-failed");
          return false;
        }
      }
      if (mesh.lineIdx && mesh.lineIdx.length > 0) {
        lineArr = toIndexArray(mesh.lineIdx, maxOf(mesh.lineIdx) > 65535);
        geom.line = uploadIndexBuffer(lineArr);
        if (!geom.line) {
          setFallback("gl-error", "line-buffer-alloc-failed");
          return false;
        }
      }
      indexInfo = {
        tris: triArr.length,
        lines: lineArr.length,
        triType: triArr.length > 0 ? indexTypeOf(triArr) : 0,
        lineType: lineArr.length > 0 ? indexTypeOf(lineArr) : 0
      };
      return true;
    }
    __name(uploadMesh, "uploadMesh");
    function indexTypeOf(arr) {
      if (!gl) return 0;
      if (arr instanceof Uint32Array) return gl.UNSIGNED_INT;
      if (arr instanceof Uint8Array) return gl.UNSIGNED_BYTE;
      return gl.UNSIGNED_SHORT;
    }
    __name(indexTypeOf, "indexTypeOf");
    function bindAttribs() {
      if (!gl) return;
      const slots = [
        ["aPosition", geom.pos, 3],
        ["aColor", geom.col, 3],
        ["aNormal", geom.nrm, 3]
      ];
      for (let i = 0; i < slots.length; i += 1) {
        const name = slots[i][0];
        const buf = slots[i][1];
        const size = slots[i][2];
        const loc = attribs[name];
        if (buf === null || loc === void 0 || loc < 0) continue;
        gl.bindBuffer(gl.ARRAY_BUFFER, buf);
        gl.enableVertexAttribArray(loc);
        gl.vertexAttribPointer(loc, size, gl.FLOAT, false, 0, 0);
      }
    }
    __name(bindAttribs, "bindAttribs");
    function acquireContext() {
      if (options.gl) {
        gl = options.gl;
      } else if (typeof options.glFactory === "function") {
        gl = options.glFactory(canvas) || null;
      } else if (typeof canvas.getContext === "function") {
        try {
          gl = canvas.getContext("webgl", options.glAttributes || void 0) || null;
        } catch {
          gl = null;
        }
      }
      if (!gl) {
        setFallback("webgl-unavailable");
        return false;
      }
      if (typeof gl.isContextLost === "function" && gl.isContextLost()) {
        setFallback("context-lost");
        return false;
      }
      return true;
    }
    __name(acquireContext, "acquireContext");
    function initGL() {
      if (!acquireContext() || !gl) return false;
      clearFallback();
      if (!buildProgram()) return false;
      uploadMesh();
      gl.viewport(0, 0, canvas.width || 1, canvas.height || 1);
      return true;
    }
    __name(initGL, "initGL");
    function listen(target, type, handler) {
      const fn = handler;
      disposer.addEventListener(target, type, fn);
      listeners += 1;
    }
    __name(listen, "listen");
    listen(canvas, "webglcontextlost", (event) => {
      const ev = event;
      if (typeof ev.preventDefault === "function") ev.preventDefault();
      releaseGL();
      setFallback("context-lost");
    });
    listen(canvas, "webglcontextrestored", () => {
      if (destroyed) return;
      releaseGL();
      gl = null;
      if (initGL()) render();
    });
    listen(window, "resize", () => {
      if (destroyed || !gl || fallbackReason !== null) return;
      render();
    });
    function currentVScale() {
      const raw = params && typeof params === "object" ? params.vscale : void 0;
      const n = typeof raw === "number" ? raw : Number(raw);
      return isFinite(n) ? n : 1;
    }
    __name(currentVScale, "currentVScale");
    function render() {
      if (destroyed || !gl || !program) return;
      syncTokens();
      if (clearColor) gl.clearColor(clearColor[0], clearColor[1], clearColor[2], clearColor[3]);
      gl.viewport(0, 0, canvas.width || 1, canvas.height || 1);
      gl.clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT);
      gl.enable(gl.DEPTH_TEST);
      const vscale = currentVScale();
      perspective(proj, camera.fovDeg * Math.PI / 180, (canvas.width || 1) / (canvas.height || 1), camera.near, camera.far);
      lookAt(view, camera.eye, camera.target, camera.up);
      gl.useProgram(program);
      bindAttribs();
      if (uniforms.uProjection) gl.uniformMatrix4fv(uniforms.uProjection, false, proj);
      if (uniforms.uView) gl.uniformMatrix4fv(uniforms.uView, false, view);
      if (uniforms.uVScale) gl.uniform1f(uniforms.uVScale, vscale);
      if (uniforms.uLightDir) gl.uniform3fv(uniforms.uLightDir, lightDir);
      if (uniforms.uAmbient) gl.uniform1f(uniforms.uAmbient, ambient);
      drawCalls = 0;
      if (geom.tri && indexInfo.tris > 0) {
        gl.bindBuffer(gl.ELEMENT_ARRAY_BUFFER, geom.tri);
        gl.drawElements(gl.TRIANGLES, indexInfo.tris, indexInfo.triType, 0);
        drawCalls += 1;
      }
      if (geom.line && indexInfo.lines > 0) {
        gl.bindBuffer(gl.ELEMENT_ARRAY_BUFFER, geom.line);
        gl.drawElements(gl.LINES, indexInfo.lines, indexInfo.lineType, 0);
        drawCalls += 1;
      }
      root.setAttribute("data-draw-calls", String(drawCalls));
      root.setAttribute("data-vscale", String(vscale));
    }
    __name(render, "render");
    function setMesh(next) {
      if (destroyed || !next || !next.pos) return;
      mesh = next;
      if (gl && program && fallbackReason === null) {
        for (let i = 0; i < buffers.length; i += 1) gl.deleteBuffer(buffers[i]);
        buffers = [];
        geom = { pos: null, nrm: null, col: null, tri: null, line: null };
        uploadMesh();
      }
    }
    __name(setMesh, "setMesh");
    function setCamera(next) {
      if (destroyed || !next) return;
      const vec = /* @__PURE__ */ __name((src) => src ? [Number(src[0]) || 0, Number(src[1]) || 0, Number(src[2]) || 0] : null, "vec");
      const eye = vec(next.eye);
      const target = vec(next.target);
      const up = vec(next.up);
      if (eye) camera.eye = eye;
      if (target) camera.target = target;
      if (up) camera.up = up;
      if (typeof next.fovDeg === "number") camera.fovDeg = next.fovDeg;
      if (typeof next.near === "number") camera.near = next.near;
      if (typeof next.far === "number") camera.far = next.far;
    }
    __name(setCamera, "setCamera");
    function setOptions(next) {
      if (destroyed || !next) return;
      params = { ...params, ...next };
      if (Object.prototype.hasOwnProperty.call(next, "vscale")) {
        root.setAttribute("data-vscale", String(currentVScale()));
      }
    }
    __name(setOptions, "setOptions");
    function destroy() {
      if (destroyed) return;
      destroyed = true;
      disposer.dispose();
      listeners = 0;
      releaseGL();
      gl = null;
      if (root.parentNode) root.parentNode.removeChild(root);
    }
    __name(destroy, "destroy");
    defineGetter(root, "gl", () => gl);
    defineGetter(root, "fallbackReason", () => fallbackReason);
    defineGetter(root, "fallbackMessage", () => fallbackReason ? FALLBACK_TEXT[fallbackReason] || "" : "");
    defineGetter(root, "drawStats", () => ({
      drawCalls,
      triangles: indexInfo.tris / 3,
      lines: indexInfo.lines / 2,
      vscale: currentVScale()
    }));
    defineGetter(root, "missingTokens", () => missingTokens.slice());
    defineGetter(root, "unresolvedColors", () => unresolvedColors.slice());
    defineGetter(root, "listenerCount", () => listeners);
    defineGetter(root, "destroyed", () => destroyed);
    const viewport = root;
    viewport.setMesh = setMesh;
    viewport.setCamera = setCamera;
    viewport.setOptions = setOptions;
    viewport.render = render;
    viewport.destroy = destroy;
    syncTokens();
    if (initGL()) render();
    return viewport;
  }
  __name(createWebGLViewport, "createWebGLViewport");

  // src/components/mini-chart.ts
  var CANVAS_TOKENS3 = Object.freeze({
    background: "var(--canvas-background)",
    surface: "var(--canvas-surface)",
    grid: "var(--canvas-grid)",
    axis: "var(--canvas-axis)",
    text: "var(--canvas-text)",
    selection: "var(--canvas-selection)",
    series1: "var(--canvas-series-1)",
    series2: "var(--canvas-series-2)",
    lineWidth: "var(--space-1)",
    lineWidthHeavy: "var(--space-2)",
    fontSize: "var(--font-size-12)"
  });
  var DEFAULT_SERIES_TOKENS = Object.freeze(["--canvas-series-1", "--canvas-series-2"]);
  var MINI_CHART_SERIES_TOKEN_COUNT = DEFAULT_SERIES_TOKENS.length;
  var HEX_SHAPE_RE = /^#[0-9a-fA-F]{3,8}$/;
  function isSeriesObject(v) {
    return typeof v === "object" && v !== null && !Array.isArray(v) && "values" in v;
  }
  __name(isSeriesObject, "isSeriesObject");
  function toFinite(v) {
    return typeof v === "number" && Number.isFinite(v) ? v : 0;
  }
  __name(toFinite, "toFinite");
  function readNumber3(v, fallback) {
    return typeof v === "number" && Number.isFinite(v) ? v : fallback;
  }
  __name(readNumber3, "readNumber");
  function defineGetter2(target, key, get) {
    Object.defineProperty(target, key, { get, enumerable: true, configurable: false });
  }
  __name(defineGetter2, "defineGetter");
  function computedStyleValue(el, prop) {
    const view = el.ownerDocument?.defaultView ?? null;
    const gcs = typeof globalThis.getComputedStyle === "function" ? globalThis.getComputedStyle : view?.getComputedStyle;
    if (typeof gcs !== "function") return "";
    const cs = gcs(el);
    const raw = cs ? cs[prop] : void 0;
    return typeof raw === "string" ? raw.trim() : "";
  }
  __name(computedStyleValue, "computedStyleValue");
  function createMiniChart(options) {
    const canvas = options.canvas;
    const doc = canvas.ownerDocument ?? globalThis.document ?? null;
    if (doc === null) {
      throw new Error("[PetUI mini-chart] \u9700\u8981 canvas \u5C5E\u4E8E\u4E00\u4E2A Document\uFF08ownerDocument \u7F3A\u5931\u4E14\u65E0\u5168\u5C40 document\uFF09");
    }
    const disposer = createDisposer();
    const requestedKind = options.kind === "bar" ? "bar" : "line";
    if (options.kind !== "line" && options.kind !== "bar") {
      console.warn(`[PetUI mini-chart] \u672A\u77E5 kind=${String(options.kind)}\uFF0C\u5DF2\u56DE\u843D 'line'`);
    }
    const host = doc.createElement("div");
    host.className = "pui-mini-chart";
    host.setAttribute("data-kind", requestedKind);
    const body = doc.createElement("div");
    body.className = "pui-mini-chart__canvas";
    body.appendChild(canvas);
    host.appendChild(body);
    const state = {
      ctx: typeof canvas.getContext === "function" ? canvas.getContext("2d") : null,
      kind: requestedKind,
      series: options.series ?? [],
      axis: options.axis ?? {},
      labels: options.labels ?? [],
      palette: (options.palette ?? []).slice(),
      paletteTokens: [],
      dataState: "no-data",
      listeners: 0,
      destroyed: false
    };
    if (state.ctx === null) {
      console.warn('[PetUI mini-chart] getContext("2d") \u8FD4\u56DE null\uFF1A\u6E32\u67D3\u4E3A\u7A7A\u64CD\u4F5C\uFF08\u672C\u5E27\u8DF3\u8FC7\u7ED8\u5236\uFF09');
    }
    const element = host;
    function tokenValue(tokenExpression2) {
      try {
        const resolved = resolveTokenValue(element, tokenExpression2);
        const value = resolved.value.trim();
        if (value.length === 0 || value.startsWith("var(")) return null;
        return value;
      } catch (err) {
        console.warn(
          `[PetUI mini-chart] \u4EE4\u724C\u89E3\u6790\u5931\u8D25\uFF08\u672C\u5E27\u8DF3\u8FC7\u8BE5\u89C6\u89C9\u5143\u7D20\uFF09\uFF1A${tokenExpression2}\uFF1B\u7F3A\u4EE4\u724C\u8BF7\u8D70 --color-n-<hex> \u51FA\u53E3\u6216\u56DE\u62A5 m01\uFF08\u4E0D\u5F97\u81EA\u9020\u4EE4\u724C\u540D\uFF09`,
          err
        );
        return null;
      }
    }
    __name(tokenValue, "tokenValue");
    function tokenLength(tokenExpression2, fallback) {
      const raw = tokenValue(tokenExpression2);
      if (raw === null) return fallback;
      const parsed = parseFloat(raw);
      return Number.isFinite(parsed) && parsed > 0 ? parsed : fallback;
    }
    __name(tokenLength, "tokenLength");
    function colorFrom(input) {
      if (input.startsWith("--")) return tokenValue(`var(${input})`);
      if (HEX_SHAPE_RE.test(input)) {
        const exitName = `--color-n-${input.slice(1).toLowerCase()}`;
        if (isRegistered(exitName)) return tokenValue(`var(${exitName})`);
        console.warn(
          `[PetUI mini-chart] \u53D6\u8272\u5B57\u9762\u503C\u7F3A\u5C11 --color-n-<hex> \u51FA\u53E3\uFF1A${input}\uFF08\u56DE\u62A5 m01 \u8FFD\u52A0\u767B\u8BB0\uFF09`
        );
        return null;
      }
      console.warn(`[PetUI mini-chart] \u4E0D\u63A5\u53D7\u7684\u53D6\u8272\u5165\u53C2\uFF08\u65E2\u975E\u4EE4\u724C\u540D\u4E5F\u975E\u5341\u516D\u8FDB\u5236\uFF09\uFF1A${String(input)}`);
      return null;
    }
    __name(colorFrom, "colorFrom");
    function defaultPaletteToken(index) {
      return DEFAULT_SERIES_TOKENS[index % DEFAULT_SERIES_TOKENS.length];
    }
    __name(defaultPaletteToken, "defaultPaletteToken");
    function resolveSeriesColor(series, index) {
      if (series.color !== null) {
        const explicit = colorFrom(series.color);
        if (explicit !== null) return explicit;
      }
      const paletteEntry = state.palette[index];
      if (paletteEntry !== void 0) {
        const fromPalette = colorFrom(paletteEntry);
        if (fromPalette !== null) return fromPalette;
      }
      return tokenValue(`var(${defaultPaletteToken(index)})`);
    }
    __name(resolveSeriesColor, "resolveSeriesColor");
    function normalizeSeries() {
      return state.series.map((entry, index) => {
        if (isSeriesObject(entry)) {
          return {
            name: entry.name ?? `series-${index + 1}`,
            values: (entry.values ?? []).map(toFinite),
            color: entry.color ?? null
          };
        }
        return { name: `series-${index + 1}`, values: (entry ?? []).map(toFinite), color: null };
      });
    }
    __name(normalizeSeries, "normalizeSeries");
    function computeDomain(all) {
      let min = readNumber3(state.axis.min, Number.NaN);
      let max = readNumber3(state.axis.max, Number.NaN);
      if (!Number.isFinite(min) || !Number.isFinite(max)) {
        for (const v of all) {
          if (!Number.isFinite(v)) continue;
          if (!Number.isFinite(min) || v < min) min = v;
          if (!Number.isFinite(max) || v > max) max = v;
        }
      }
      if (!Number.isFinite(min)) min = 0;
      if (!Number.isFinite(max)) max = min;
      if (min === max) {
        const pad3 = min !== 0 ? Math.abs(min) * 0.5 : 1;
        min -= pad3;
        max += pad3;
      }
      return { min, max };
    }
    __name(computeDomain, "computeDomain");
    function prepareFrame() {
      const ctx = state.ctx;
      if (ctx === null) return null;
      const cssWidth = readNumber3(canvas.clientWidth, 0);
      const cssHeight = readNumber3(canvas.clientHeight, 0);
      const dpr = Math.min(readNumber3(globalThis.devicePixelRatio, 1) || 1, 2);
      if (cssWidth > 0 && cssHeight > 0) {
        const backingW = Math.round(cssWidth * dpr);
        const backingH = Math.round(cssHeight * dpr);
        if (canvas.width !== backingW) canvas.width = backingW;
        if (canvas.height !== backingH) canvas.height = backingH;
        if (typeof ctx.setTransform === "function") ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
        return { ctx, width: cssWidth, height: cssHeight };
      }
      const width = readNumber3(canvas.width, 0);
      const height = readNumber3(canvas.height, 0);
      if (width <= 0 || height <= 0) return null;
      if (typeof ctx.setTransform === "function") ctx.setTransform(1, 0, 0, 1, 0, 0);
      return { ctx, width, height };
    }
    __name(prepareFrame, "prepareFrame");
    function applyFont(ctx) {
      const size = tokenLength(CANVAS_TOKENS3.fontSize, 0);
      const family = computedStyleValue(element, "fontFamily");
      if (size > 0 && family.length > 0) {
        ctx.font = `${size}px ${family}`;
        return;
      }
      const inherited = computedStyleValue(element, "font");
      if (inherited.length > 0) ctx.font = inherited;
    }
    __name(applyFont, "applyFont");
    function strokePolyline(ctx, color, width, points) {
      ctx.strokeStyle = color;
      ctx.lineWidth = width;
      ctx.beginPath();
      points.forEach(([x, y], i) => {
        if (i === 0) ctx.moveTo(x, y);
        else ctx.lineTo(x, y);
      });
      ctx.stroke();
    }
    __name(strokePolyline, "strokePolyline");
    function drawNoData(ctx, width, height) {
      applyFont(ctx);
      ctx.textAlign = "center";
      ctx.textBaseline = "middle";
      const color = tokenValue(CANVAS_TOKENS3.text);
      if (color !== null) ctx.fillStyle = color;
      ctx.fillText("\u65E0\u6570\u636E", width / 2, height / 2);
    }
    __name(drawNoData, "drawNoData");
    function drawBaseline(ctx, padX, plotW, baseY) {
      const color = tokenValue(CANVAS_TOKENS3.axis);
      if (color === null) return;
      const width = tokenLength(CANVAS_TOKENS3.lineWidth, 1);
      ctx.strokeStyle = color;
      ctx.lineWidth = width;
      ctx.beginPath();
      ctx.moveTo(padX, baseY);
      ctx.lineTo(padX + plotW, baseY);
      ctx.stroke();
    }
    __name(drawBaseline, "drawBaseline");
    function drawGrid(ctx, padX, padY, plotW, plotH) {
      const color = tokenValue(CANVAS_TOKENS3.grid);
      if (color === null) return;
      const width = tokenLength(CANVAS_TOKENS3.lineWidth, 1);
      ctx.strokeStyle = color;
      ctx.lineWidth = width;
      ctx.beginPath();
      const midY = padY + plotH / 2;
      ctx.moveTo(padX, midY);
      ctx.lineTo(padX + plotW, midY);
      ctx.stroke();
    }
    __name(drawGrid, "drawGrid");
    function render() {
      if (state.destroyed) return;
      const frame = prepareFrame();
      if (frame === null) {
        state.dataState = "no-data";
        state.paletteTokens = [];
        return;
      }
      const { ctx, width, height } = frame;
      ctx.clearRect(0, 0, width, height);
      const series = normalizeSeries();
      const pointCount = series.reduce((n, s) => Math.max(n, s.values.length), 0);
      if (series.length === 0 || pointCount === 0) {
        state.dataState = "no-data";
        state.paletteTokens = [];
        drawNoData(ctx, width, height);
        return;
      }
      state.dataState = "ready";
      state.paletteTokens = series.map((s, i) => {
        if (s.color !== null && s.color.startsWith("--")) return s.color;
        const entry = state.palette[i];
        if (entry !== void 0 && entry.startsWith("--")) return entry;
        return defaultPaletteToken(i);
      });
      const all = series.flatMap((s) => s.values);
      const { min, max } = computeDomain(all);
      const span = max - min || 1;
      const padX = width * 0.06;
      const padY = height * 0.08;
      const plotW = Math.max(width - padX * 2, 1);
      const plotH = Math.max(height - padY * 2, 1);
      const baseY = padY + plotH;
      const xAt = /* @__PURE__ */ __name((i) => pointCount <= 1 ? padX + plotW / 2 : padX + plotW * i / (pointCount - 1), "xAt");
      const yAt = /* @__PURE__ */ __name((v) => padY + plotH * (1 - (v - min) / span), "yAt");
      drawGrid(ctx, padX, padY, plotW, plotH);
      if (state.axis.showBaseline !== false) drawBaseline(ctx, padX, plotW, baseY);
      if (state.kind === "bar") {
        const groups = Math.max(series.length, 1);
        const slot = plotW / pointCount;
        const barW = Math.max(slot / (groups + 0.6), 1);
        series.forEach((s, idx) => {
          const color = resolveSeriesColor(s, idx);
          if (color === null) return;
          ctx.fillStyle = color;
          s.values.forEach((v, i) => {
            const center = pointCount <= 1 ? padX + plotW / 2 : padX + slot * i + slot / 2;
            const x = center - barW * groups / 2 + barW * idx;
            const y = yAt(v);
            ctx.fillRect(x, y, barW, Math.max(baseY - y, 0));
          });
        });
      } else {
        applyFont(ctx);
        const width0 = tokenLength(CANVAS_TOKENS3.lineWidthHeavy, tokenLength(CANVAS_TOKENS3.lineWidth, 1));
        series.forEach((s, idx) => {
          const color = resolveSeriesColor(s, idx);
          if (color === null) return;
          const points = s.values.map((v, i) => [xAt(i), yAt(v)]);
          strokePolyline(ctx, color, width0, points);
        });
      }
      if (state.labels.length > 0 && state.axis.showLabels !== false) {
        applyFont(ctx);
        ctx.textAlign = "center";
        ctx.textBaseline = "top";
        const color = tokenValue(CANVAS_TOKENS3.text);
        if (color === null) return;
        ctx.fillStyle = color;
        state.labels.slice(0, pointCount).forEach((label, i) => {
          ctx.fillText(String(label), xAt(i), baseY + 2);
        });
      }
    }
    __name(render, "render");
    function destroy() {
      if (state.destroyed) return;
      state.destroyed = true;
      state.listeners = 0;
      disposer.dispose();
      if (state.ctx !== null) {
        state.ctx.clearRect(0, 0, readNumber3(canvas.width, 0), readNumber3(canvas.height, 0));
      }
      state.ctx = null;
      state.series = [];
      state.labels = [];
      state.palette = [];
      state.paletteTokens = [];
      state.dataState = "no-data";
      if (typeof body.removeChild === "function" && canvas.parentNode === body) body.removeChild(canvas);
      if (typeof host.remove === "function") host.remove();
    }
    __name(destroy, "destroy");
    const win = doc?.defaultView ?? null;
    if (win !== null && typeof win.addEventListener === "function") {
      disposer.addEventListener(win, "resize", () => {
        if (!state.destroyed) render();
      });
      state.listeners += 1;
    }
    defineGetter2(element, "kind", () => state.kind);
    defineGetter2(element, "series", () => state.series);
    defineGetter2(element, "axis", () => state.axis);
    defineGetter2(element, "labels", () => state.labels);
    defineGetter2(element, "palette", () => state.palette);
    defineGetter2(element, "paletteTokens", () => state.paletteTokens);
    defineGetter2(element, "dataState", () => state.dataState);
    defineGetter2(element, "destroyed", () => state.destroyed);
    defineGetter2(element, "listenerCount", () => state.destroyed ? 0 : state.listeners);
    Object.defineProperty(element, "render", { value: render, enumerable: false, configurable: false });
    Object.defineProperty(element, "destroy", { value: destroy, enumerable: false, configurable: false });
    render();
    const raf = globalThis.requestAnimationFrame;
    if (typeof raf === "function") {
      const id = raf(() => {
        if (!state.destroyed) render();
      });
      const caf = globalThis.cancelAnimationFrame;
      if (typeof caf === "function") disposer.add(() => caf(id));
    }
    const RO = globalThis.ResizeObserver;
    if (typeof RO === "function") {
      const ro = new RO(() => {
        if (!state.destroyed) render();
      });
      ro.observe(element);
      disposer.add(() => ro.disconnect());
    }
    return element;
  }
  __name(createMiniChart, "createMiniChart");

  // src/components/media-timeline.ts
  var DEFAULT_FRAMES = 120;
  var DEFAULT_FPS = 24;
  var DEFAULT_SPEED = 1;
  var SPEED_PRESETS = [0.25, 0.5, 1, 2, 4];
  function tokVar6(name) {
    return "var(" + name + ")";
  }
  __name(tokVar6, "tokVar");
  function defaultClock2() {
    const g = globalThis;
    if (typeof g.setTimeout !== "function" || typeof g.clearTimeout !== "function") {
      throw new Error(
        "TimelineClockUnavailableError: \u5F53\u524D\u5BBF\u4E3B\u6CA1\u6709 setTimeout/clearTimeout\uFF0C\u65E0\u6CD5\u5EFA\u7ACB\u5B9A\u65F6\u5668\uFF1B\u8BF7\u5728 clock \u6CE8\u5165\u65F6\u949F\u5B9E\u73B0"
      );
    }
    const readNow = g.Date && typeof g.Date.now === "function" ? () => g.Date.now() : () => 0;
    return {
      now: readNow,
      setTimer: /* @__PURE__ */ __name((fn, ms) => g.setTimeout(fn, ms), "setTimer"),
      clearTimer: /* @__PURE__ */ __name((handle) => {
        if (handle !== void 0 && handle !== null) g.clearTimeout(handle);
      }, "clearTimer")
    };
  }
  __name(defaultClock2, "defaultClock");
  function toPositive(value, fallback) {
    return typeof value === "number" && Number.isFinite(value) && value > 0 ? value : fallback;
  }
  __name(toPositive, "toPositive");
  function toFrameCount(value) {
    if (typeof value !== "number" || !Number.isFinite(value)) return DEFAULT_FRAMES;
    const floored = Math.floor(value);
    return floored >= 1 ? floored : 1;
  }
  __name(toFrameCount, "toFrameCount");
  function clampFrame(value, frames) {
    if (typeof value !== "number" || !Number.isFinite(value)) return 0;
    const rounded = Math.round(value);
    if (rounded < 0) return 0;
    const last = frames - 1;
    return rounded > last ? last : rounded;
  }
  __name(clampFrame, "clampFrame");
  function pad2(value, width) {
    return String(value).padStart(width, "0");
  }
  __name(pad2, "pad");
  function formatTimecode(frame, fps) {
    const totalMs = Math.round(frame / fps * 1e3);
    const ms = totalMs % 1e3;
    const totalSeconds = Math.floor(totalMs / 1e3);
    const seconds = totalSeconds % 60;
    const minutes = Math.floor(totalSeconds / 60);
    return pad2(minutes, 2) + ":" + pad2(seconds, 2) + "." + pad2(ms, 3);
  }
  __name(formatTimecode, "formatTimecode");
  function createMediaTimeline(options = {}) {
    const doc = document;
    const disposer = createDisposer();
    const clock = options.clock ?? defaultClock2();
    let frames = toFrameCount(options.frames);
    let fps = toPositive(options.fps, DEFAULT_FPS);
    let speed = toPositive(options.speed, DEFAULT_SPEED);
    let current = clampFrame(options.current, frames);
    const loop = options.loop === true;
    let playing = false;
    let state = "idle";
    let destroyed = false;
    let handle = null;
    let liveResources = 0;
    const onSeek = typeof options.onSeek === "function" ? options.onSeek : null;
    const onPlayStateChange = typeof options.onPlayStateChange === "function" ? options.onPlayStateChange : null;
    const el = doc.createElement("div");
    el.className = "petui-media-timeline";
    el.setAttribute("role", "group");
    el.setAttribute("aria-label", options.label ?? "Media timeline");
    el.style.cssText = [
      "display:flex",
      "flex-direction:column",
      "gap:" + tokVar6("--space-2"),
      "padding:" + tokVar6("--space-4") + " " + tokVar6("--space-8"),
      "background:" + tokVar6("--color-bg"),
      "color:" + tokVar6("--color-text"),
      "border:" + tokVar6("--space-1") + " solid " + tokVar6("--color-border"),
      "border-radius:" + tokVar6("--border-radius-sm"),
      "font-size:" + tokVar6("--font-size-12")
    ].join(";");
    const bar = doc.createElement("div");
    bar.className = "petui-media-timeline-bar";
    bar.style.cssText = ["display:flex", "align-items:center", "gap:" + tokVar6("--space-4")].join(";");
    const playBtn = doc.createElement("button");
    playBtn.type = "button";
    playBtn.className = "petui-media-timeline-play";
    playBtn.style.cssText = [
      "background:" + tokVar6("--color-bg-light"),
      "color:" + tokVar6("--color-text"),
      "border:" + tokVar6("--space-1") + " solid " + tokVar6("--color-border"),
      "border-radius:" + tokVar6("--border-radius-sm"),
      "padding:" + tokVar6("--space-2") + " " + tokVar6("--space-8"),
      "font-size:" + tokVar6("--font-size-12"),
      "cursor:pointer"
    ].join(";");
    const scrub = doc.createElement("input");
    scrub.type = "range";
    scrub.className = "petui-media-timeline-scrub";
    scrub.min = "0";
    scrub.max = String(frames - 1);
    scrub.step = "1";
    scrub.value = String(current);
    scrub.setAttribute("aria-label", "Frame");
    scrub.style.cssText = [
      "flex:1 1 auto",
      "min-width:" + tokVar6("--space-300"),
      "accent-color:" + tokVar6("--color-primary")
    ].join(";");
    const speedSel = doc.createElement("select");
    speedSel.className = "petui-media-timeline-speed";
    speedSel.setAttribute("aria-label", "Speed");
    speedSel.style.cssText = [
      "background:" + tokVar6("--color-bg-light"),
      "color:" + tokVar6("--color-text"),
      "border:" + tokVar6("--space-1") + " solid " + tokVar6("--color-border"),
      "border-radius:" + tokVar6("--border-radius-sm"),
      "padding:" + tokVar6("--space-2") + " " + tokVar6("--space-8"),
      "font-size:" + tokVar6("--font-size-12")
    ].join(";");
    for (const preset of SPEED_PRESETS) {
      const opt = doc.createElement("option");
      opt.value = String(preset);
      opt.textContent = String(preset) + "\xD7";
      speedSel.appendChild(opt);
    }
    const readout = doc.createElement("div");
    readout.className = "petui-media-timeline-readout";
    readout.style.cssText = [
      "display:flex",
      "justify-content:space-between",
      "gap:" + tokVar6("--space-4"),
      "font-size:" + tokVar6("--font-size-12")
    ].join(";");
    const frameText = doc.createElement("span");
    frameText.className = "petui-media-timeline-frame";
    const timeText = doc.createElement("span");
    timeText.className = "petui-media-timeline-timecode";
    readout.appendChild(frameText);
    readout.appendChild(timeText);
    bar.appendChild(playBtn);
    bar.appendChild(scrub);
    bar.appendChild(speedSel);
    el.appendChild(bar);
    el.appendChild(readout);
    function bindListener(target, type, fn) {
      disposer.addEventListener(target, type, fn);
      liveResources += 1;
    }
    __name(bindListener, "bindListener");
    function registerCleanup(fn) {
      disposer.add(fn);
      liveResources += 1;
    }
    __name(registerCleanup, "registerCleanup");
    function render() {
      playBtn.textContent = playing ? "\u275A\u275A" : "\u25B6";
      playBtn.setAttribute("aria-label", playing ? "Pause" : "Play");
      playBtn.setAttribute("aria-pressed", playing ? "true" : "false");
      el.setAttribute("data-state", state);
      el.setAttribute("data-frame", String(current));
      frameText.textContent = String(current + 1) + " / " + String(frames);
      timeText.textContent = formatTimecode(current, fps);
      frameText.style.color = tokVar6(state === "ended" ? "--color-warning" : "--color-text-muted");
      timeText.style.color = tokVar6("--color-text-muted");
      const value = String(current);
      if (scrub.value !== value) scrub.value = value;
      const preset = String(speed);
      if (speedSel.value !== preset) speedSel.value = preset;
    }
    __name(render, "render");
    function emitFrame() {
      if (onSeek !== null) onSeek(current);
    }
    __name(emitFrame, "emitFrame");
    function setState(next) {
      if (state === next) return;
      state = next;
      render();
    }
    __name(setState, "setState");
    function setPlaying(next) {
      const flipped = playing !== next;
      playing = next;
      if (flipped && onPlayStateChange !== null) onPlayStateChange(playing);
    }
    __name(setPlaying, "setPlaying");
    function clearTimer() {
      if (handle === null) return;
      clock.clearTimer(handle);
      handle = null;
    }
    __name(clearTimer, "clearTimer");
    function frameDelay() {
      return 1e3 / (fps * speed);
    }
    __name(frameDelay, "frameDelay");
    function schedule() {
      clearTimer();
      handle = clock.setTimer(onTick, frameDelay());
    }
    __name(schedule, "schedule");
    function onTick() {
      handle = null;
      if (destroyed || !playing) return;
      advance();
      if (playing && !destroyed) schedule();
    }
    __name(onTick, "onTick");
    function goToFrame(frame) {
      current = frame;
      render();
      emitFrame();
    }
    __name(goToFrame, "goToFrame");
    function advance() {
      if (current < frames - 1) {
        goToFrame(current + 1);
        return;
      }
      if (loop) {
        goToFrame(0);
        return;
      }
      setPlaying(false);
      setState("ended");
    }
    __name(advance, "advance");
    function play() {
      if (destroyed || playing) return;
      if (!loop && current >= frames - 1) goToFrame(0);
      setPlaying(true);
      setState("playing");
      render();
      schedule();
    }
    __name(play, "play");
    function pause() {
      if (destroyed || !playing) return;
      clearTimer();
      setPlaying(false);
      setState("paused");
      render();
    }
    __name(pause, "pause");
    function seek(frame) {
      if (destroyed) return;
      const next = clampFrame(frame, frames);
      current = next;
      if (state === "ended" && current < frames - 1) state = "paused";
      render();
      emitFrame();
    }
    __name(seek, "seek");
    function setSpeed(next) {
      if (typeof next !== "number" || !Number.isFinite(next) || next <= 0) {
        throw new RangeError("media-timeline: speed \u5FC5\u987B\u662F\u6709\u9650\u6B63\u6570\uFF0C\u6536\u5230 " + String(next));
      }
      if (destroyed || next === speed) return;
      speed = next;
      render();
      if (playing) schedule();
    }
    __name(setSpeed, "setSpeed");
    function onPlayClick() {
      if (playing) pause();
      else play();
    }
    __name(onPlayClick, "onPlayClick");
    function onScrubInput() {
      seek(Number(scrub.value));
    }
    __name(onScrubInput, "onScrubInput");
    function onSpeedChange() {
      const parsed = Number(speedSel.value);
      if (Number.isFinite(parsed) && parsed > 0) setSpeed(parsed);
    }
    __name(onSpeedChange, "onSpeedChange");
    function destroy() {
      if (destroyed) return;
      destroyed = true;
      clearTimer();
      setPlaying(false);
      state = "idle";
      disposer.dispose();
      liveResources = 0;
    }
    __name(destroy, "destroy");
    bindListener(playBtn, "click", onPlayClick);
    bindListener(scrub, "input", onScrubInput);
    bindListener(speedSel, "change", onSpeedChange);
    registerCleanup(clearTimer);
    const getters = [
      ["frames", () => frames],
      ["fps", () => fps],
      ["current", () => current],
      ["loop", () => loop],
      ["speed", () => speed],
      ["playing", () => playing],
      ["state", () => state],
      ["destroyed", () => destroyed],
      ["listenerCount", () => liveResources]
    ];
    for (const [name, read] of getters) {
      Object.defineProperty(el, name, { get: read, enumerable: true, configurable: true });
    }
    const methods = [
      ["play", play],
      ["pause", pause],
      ["seek", seek],
      ["setSpeed", setSpeed],
      ["destroy", destroy]
    ];
    for (const [name, fn] of methods) {
      Object.defineProperty(el, name, { value: fn, enumerable: false, configurable: true, writable: false });
    }
    render();
    return el;
  }
  __name(createMediaTimeline, "createMediaTimeline");

  // src/components/hex-viewer.ts
  var MAX_WIDTH = 4096;
  var MAX_INDEX = Number.MAX_SAFE_INTEGER;
  var HEX_DIGITS_PER_BYTE = 2;
  var HEX_GAP = 1;
  var ASCII_PRINTABLE_MIN = 32;
  var ASCII_PRINTABLE_MAX = 126;
  var HEX_VIEWER_MAX_ROWS = 512;
  var HEX_VIEWER_EMPTY_TEXT = "\u7A7A\u7A97\u53E3\uFF1A\u8BE5\u7A97\u53E3\u6CA1\u6709\u53EF\u663E\u793A\u7684\u5B57\u8282\uFF08\u53EF\u8BF7\u6C42\u65B0\u7A97\u53E3\uFF09";
  var HEX_VIEWER_ASCII_PLACEHOLDER = ".";
  var _HexViewerOptionsError = class _HexViewerOptionsError extends Error {
    constructor(message) {
      super("[m32 hex-viewer] " + message);
      this.name = "HexViewerOptionsError";
    }
  };
  __name(_HexViewerOptionsError, "HexViewerOptionsError");
  var HexViewerOptionsError = _HexViewerOptionsError;
  var _HexViewerDisposedError = class _HexViewerDisposedError extends Error {
    constructor() {
      super("[m32 hex-viewer] \u5B9E\u4F8B\u5DF2 destroy()\uFF0C\u4E0D\u80FD\u518D\u8C03\u7528 setWindow()");
      this.name = "HexViewerDisposedError";
    }
  };
  __name(_HexViewerDisposedError, "HexViewerDisposedError");
  var HexViewerDisposedError = _HexViewerDisposedError;
  function offsetColumnDigits(offset, length) {
    const last = length > 0 ? offset + length - 1 : offset;
    const digits = Math.max(0, Math.floor(last)).toString(16).length;
    return Math.max(1, digits);
  }
  __name(offsetColumnDigits, "offsetColumnDigits");
  function asciiColumn(bytes) {
    let out = "";
    for (let i = 0; i < bytes.length; i++) {
      const b = bytes[i];
      out += b >= ASCII_PRINTABLE_MIN && b <= ASCII_PRINTABLE_MAX ? String.fromCharCode(b) : HEX_VIEWER_ASCII_PLACEHOLDER;
    }
    return out;
  }
  __name(asciiColumn, "asciiColumn");
  function byteColumn(byteIndex) {
    return byteIndex * (HEX_DIGITS_PER_BYTE + HEX_GAP);
  }
  __name(byteColumn, "byteColumn");
  function requireIndex(name, value, min, max) {
    if (typeof value !== "number" || !Number.isInteger(value) || value < min || value > max) {
      throw new HexViewerOptionsError(
        name + " \u5FC5\u987B\u662F [" + String(min) + ", " + String(max) + "] \u5185\u7684\u6574\u6570\uFF0C\u6536\u5230 " + String(value)
      );
    }
    return value;
  }
  __name(requireIndex, "requireIndex");
  function requireWidth(value) {
    if (typeof value !== "number" || !Number.isInteger(value) || value < 1 || value > MAX_WIDTH) {
      throw new HexViewerOptionsError(
        "width \u5FC5\u586B\u4E14\u5FC5\u987B\u662F [1, " + String(MAX_WIDTH) + "] \u5185\u7684\u6574\u6570\uFF08\u6BCF\u884C\u5B57\u8282\u6570\u4E0D\u5F97\u6709\u9690\u5F0F\u9ED8\u8BA4\u503C\uFF09\uFF0C\u6536\u5230 " + String(value)
      );
    }
    return value;
  }
  __name(requireWidth, "requireWidth");
  function tokVar7(name) {
    return "var(" + name + ")";
  }
  __name(tokVar7, "tokVar");
  function createHexViewer(options) {
    if (!options || typeof options !== "object") {
      throw new HexViewerOptionsError("\u5FC5\u987B\u4F20\u5165\u9009\u9879\u5BF9\u8C61\uFF08\u542B bytes \u6216 rows\uFF0C\u4EE5\u53CA\u5FC5\u586B\u7684 width\uFF09");
    }
    const doc = document;
    const width = requireWidth(options.width);
    const hasBytes = options.bytes !== void 0 && options.bytes !== null;
    const hasRows = options.rows !== void 0 && options.rows !== null;
    if (hasBytes === hasRows) {
      throw new HexViewerOptionsError("bytes \u4E0E rows \u5FC5\u987B\u4E8C\u9009\u4E00\uFF08\u90FD\u4E0D\u7ED9\u6216\u540C\u65F6\u7ED9\u90FD\u65E0\u6CD5\u786E\u5B9A\u6570\u636E\u6E90\uFF09");
    }
    const maxRows = options.maxRows === void 0 ? HEX_VIEWER_MAX_ROWS : requireIndex("maxRows", options.maxRows, 1, 1e6);
    let data = null;
    let dataBase = 0;
    let sparse = null;
    let providedCount = 0;
    let rangeStart = 0;
    let rangeEnd = 0;
    let initialOffset = 0;
    if (hasBytes) {
      const input = options.bytes;
      let view;
      if (Array.isArray(input)) {
        view = new Uint8Array(input.length);
        for (let i = 0; i < input.length; i++) {
          const v = input[i];
          if (typeof v !== "number" || !Number.isInteger(v) || v < 0 || v > 255) {
            throw new HexViewerOptionsError("bytes[" + String(i) + "] \u5FC5\u987B\u662F 0..255 \u7684\u6574\u6570\uFF0C\u6536\u5230 " + String(v));
          }
          view[i] = v;
        }
      } else if (input instanceof ArrayBuffer) {
        view = new Uint8Array(input);
      } else if (ArrayBuffer.isView(input)) {
        view = new Uint8Array(input.buffer, input.byteOffset, input.byteLength);
      } else {
        throw new HexViewerOptionsError("bytes \u5FC5\u987B\u662F Uint8Array / ArrayBuffer / 0..255 \u6574\u6570\u6570\u7EC4");
      }
      data = view;
      dataBase = options.offset === void 0 ? 0 : requireIndex("offset", options.offset, 0, MAX_INDEX);
      providedCount = view.length;
      rangeStart = dataBase;
      rangeEnd = dataBase + view.length;
      initialOffset = dataBase;
    } else {
      const rows = options.rows;
      if (!Array.isArray(rows)) {
        throw new HexViewerOptionsError("rows \u5FC5\u987B\u662F\u6570\u7EC4\uFF08\u5143\u7D20\u4E3A\u5B57\u8282\u6570\u7EC4\u6216 { offset, bytes }\uFF09");
      }
      const base = options.offset === void 0 ? 0 : requireIndex("offset", options.offset, 0, MAX_INDEX);
      const map = /* @__PURE__ */ new Map();
      let min = Number.POSITIVE_INFINITY;
      let max = -1;
      rows.forEach((row, index) => {
        const descriptor = !Array.isArray(row) && row !== null && typeof row === "object";
        const rowOffset = descriptor ? requireIndex("rows[" + String(index) + "].offset", row.offset, 0, MAX_INDEX) : base + index * width;
        const rowBytes = descriptor ? row.bytes : row;
        if (!Array.isArray(rowBytes)) {
          throw new HexViewerOptionsError("rows[" + String(index) + "] \u7684 bytes \u5FC5\u987B\u662F\u6570\u7EC4");
        }
        for (let i = 0; i < rowBytes.length; i++) {
          const v = rowBytes[i];
          if (typeof v !== "number" || !Number.isInteger(v) || v < 0 || v > 255) {
            throw new HexViewerOptionsError(
              "rows[" + String(index) + "].bytes[" + String(i) + "] \u5FC5\u987B\u662F 0..255 \u7684\u6574\u6570\uFF0C\u6536\u5230 " + String(v)
            );
          }
          const addr = rowOffset + i;
          if (!map.has(addr)) {
            map.set(addr, v);
            providedCount++;
            if (addr < min) min = addr;
            if (addr > max) max = addr;
          }
        }
      });
      sparse = map;
      rangeStart = min === Number.POSITIVE_INFINITY ? base : min;
      rangeEnd = max < 0 ? rangeStart : max + 1;
      initialOffset = base;
    }
    function byteAt(addr) {
      if (data) {
        const i = addr - dataBase;
        return i >= 0 && i < data.length ? data[i] : void 0;
      }
      return sparse ? sparse.get(addr) : void 0;
    }
    __name(byteAt, "byteAt");
    function contiguousSlice(start, end) {
      if (!data) return null;
      const s = start - dataBase;
      const e = end - dataBase;
      if (s < 0 || e > data.length || e < s) return null;
      return Array.prototype.slice.call(data, s, e);
    }
    __name(contiguousSlice, "contiguousSlice");
    const span = options.length === void 0 ? rangeEnd - rangeStart : requireIndex("length", options.length, 0, MAX_INDEX);
    let current = { offset: initialOffset, length: span, width };
    let readings = { rowCount: 0, renderedRows: 0, truncated: false, empty: true, covered: false, offsetDigits: 1 };
    let destroyed = false;
    let lastRequestKey = null;
    const disposer = createDisposer();
    const el = doc.createElement("div");
    el.className = "petui-hex-viewer";
    el.style.cssText = [
      "display:flex",
      "flex-direction:column",
      "background:" + tokVar7("--color-bg"),
      "color:" + tokVar7("--color-text"),
      "border:" + tokVar7("--space-1") + " solid " + tokVar7("--color-border"),
      "border-radius:" + tokVar7("--border-radius-sm"),
      "font-size:" + tokVar7("--font-size-12"),
      "overflow:hidden"
    ].join(";");
    el.setAttribute("role", "group");
    el.setAttribute("tabindex", "0");
    el.setAttribute("aria-label", options.label === void 0 ? "hex viewer" : options.label);
    const head = doc.createElement("div");
    head.className = "petui-hex-viewer-head";
    head.style.cssText = [
      "display:flex",
      "align-items:center",
      "justify-content:space-between",
      "gap:" + tokVar7("--space-8"),
      "padding:" + tokVar7("--space-4") + " " + tokVar7("--space-8"),
      "background:" + tokVar7("--color-bg-light"),
      "border-bottom:" + tokVar7("--space-1") + " solid " + tokVar7("--color-border"),
      "color:" + tokVar7("--color-text-muted")
    ].join(";");
    const statusEl = doc.createElement("span");
    statusEl.className = "petui-hex-viewer-status";
    const truncatedEl = doc.createElement("span");
    truncatedEl.className = "petui-hex-viewer-truncated";
    truncatedEl.setAttribute("data-petui-truncated", "hex-viewer");
    truncatedEl.style.cssText = [
      "color:" + tokVar7("--color-warning"),
      "display:none"
    ].join(";");
    head.appendChild(statusEl);
    head.appendChild(truncatedEl);
    const body = doc.createElement("div");
    body.className = "petui-hex-viewer-body";
    body.style.cssText = [
      "display:flex",
      "gap:" + tokVar7("--space-16"),
      "padding:" + tokVar7("--space-4") + " " + tokVar7("--space-8"),
      "overflow:auto",
      "white-space:pre",
      "font-family:monospace",
      "line-height:" + tokVar7("--row-height-compact")
    ].join(";");
    const offsetCol = doc.createElement("div");
    offsetCol.className = "petui-hex-viewer-offset";
    offsetCol.setAttribute("data-column", "offset");
    offsetCol.style.color = tokVar7("--color-text-muted");
    const hexCol = doc.createElement("div");
    hexCol.className = "petui-hex-viewer-hex";
    hexCol.setAttribute("data-column", "hex");
    hexCol.style.color = tokVar7("--color-text");
    const asciiCol = doc.createElement("div");
    asciiCol.className = "petui-hex-viewer-ascii";
    asciiCol.setAttribute("data-column", "ascii");
    asciiCol.style.color = tokVar7("--color-text-secondary");
    const emptyEl = doc.createElement("div");
    emptyEl.className = "petui-hex-viewer-empty";
    emptyEl.setAttribute("data-petui-empty", "hex-viewer");
    emptyEl.style.cssText = [
      "flex:1",
      "padding:" + tokVar7("--space-16"),
      "text-align:center",
      "color:" + tokVar7("--color-text-muted"),
      "display:none"
    ].join(";");
    body.appendChild(offsetCol);
    body.appendChild(hexCol);
    body.appendChild(asciiCol);
    body.appendChild(emptyEl);
    el.appendChild(head);
    el.appendChild(body);
    function showColumns(show) {
      const display = show ? "" : "none";
      emptyEl.style.display = show ? "none" : "";
      offsetCol.style.display = display;
      hexCol.style.display = display;
      asciiCol.style.display = display;
    }
    __name(showColumns, "showColumns");
    function rowText(rowStart, rowWidth) {
      const rowEnd = rowStart + rowWidth;
      let hex = "";
      let ascii = "";
      let cursor = rowStart;
      while (cursor < rowEnd) {
        if (byteAt(cursor) === void 0) {
          cursor++;
          continue;
        }
        const index = cursor - rowStart;
        const run = [];
        while (cursor < rowEnd) {
          const v = byteAt(cursor);
          if (v === void 0) break;
          run.push(v);
          cursor++;
        }
        hex = hex.padEnd(byteColumn(index), " ") + formatHexRow(run, { bytesPerLine: rowWidth, gap: HEX_GAP });
        ascii = ascii.padEnd(index, " ") + asciiColumn(run);
      }
      return {
        hex: hex.padEnd(hexColumnWidth(rowWidth, HEX_GAP), " "),
        ascii: ascii.padEnd(rowWidth, " ")
      };
    }
    __name(rowText, "rowText");
    function render() {
      const offset = current.offset;
      const length = current.length;
      const digits = offsetColumnDigits(offset, length);
      const totalRows = length > 0 ? Math.ceil(length / current.width) : 0;
      const renderRows = Math.min(totalRows, maxRows);
      const presentStart = Math.max(offset, rangeStart);
      const presentEnd = Math.min(offset + length, rangeEnd);
      const covered = offset >= rangeStart && offset + length <= rangeEnd;
      const isEmpty = length <= 0 || presentEnd <= presentStart || providedCount === 0;
      statusEl.textContent = "offset=0x" + offset.toString(16).padStart(digits, "0") + " length=" + String(length) + " width=" + String(current.width) + " rows=" + String(isEmpty ? 0 : renderRows) + "/" + String(totalRows);
      if (isEmpty) {
        emptyEl.textContent = HEX_VIEWER_EMPTY_TEXT + "\uFF08offset=0x" + offset.toString(16).padStart(digits, "0") + " length=" + String(length) + " width=" + String(current.width) + "\uFF09";
        showColumns(false);
        truncatedEl.textContent = "";
        truncatedEl.style.display = "none";
        readings = { rowCount: totalRows, renderedRows: 0, truncated: false, empty: true, covered, offsetDigits: digits };
        return;
      }
      let fastHex = null;
      if (presentStart === offset) {
        const limit = Math.min(presentEnd, offset + renderRows * current.width);
        const buf = contiguousSlice(presentStart, limit);
        if (buf) fastHex = formatHexRows(buf, { bytesPerLine: current.width, gap: HEX_GAP, pad: true });
      }
      const offsetLines = new Array(renderRows);
      const hexLines = new Array(renderRows);
      const asciiLines = new Array(renderRows);
      for (let i = 0; i < renderRows; i++) {
        const rowStart = offset + i * current.width;
        offsetLines[i] = rowStart.toString(16).padStart(digits, "0");
        const row = rowText(rowStart, current.width);
        hexLines[i] = fastHex && i < fastHex.length ? fastHex[i] : row.hex;
        asciiLines[i] = row.ascii;
      }
      offsetCol.textContent = offsetLines.join("\n");
      hexCol.textContent = hexLines.join("\n");
      asciiCol.textContent = asciiLines.join("\n");
      showColumns(true);
      const truncated = renderRows < totalRows;
      if (truncated) {
        truncatedEl.textContent = "\u884C\u6570\u4E0A\u9650 maxRows=" + String(maxRows) + "\uFF1A\u4EC5\u6E32\u67D3\u524D " + String(renderRows) + " \u884C\uFF08\u5171 " + String(totalRows) + " \u884C\uFF09";
        truncatedEl.style.display = "";
      } else {
        truncatedEl.textContent = "";
        truncatedEl.style.display = "none";
      }
      readings = { rowCount: totalRows, renderedRows: renderRows, truncated, empty: false, covered, offsetDigits: digits };
    }
    __name(render, "render");
    function maybeRequestWindow() {
      if (readings.covered) {
        lastRequestKey = null;
        return;
      }
      const key = String(current.offset) + ":" + String(current.length);
      if (key === lastRequestKey) return;
      lastRequestKey = key;
      if (typeof options.onRequestWindow !== "function") return;
      try {
        options.onRequestWindow(current.offset, current.length);
      } catch (err) {
        console.error("[m32 hex-viewer] onRequestWindow \u56DE\u8C03\u629B\u9519:", err);
      }
    }
    __name(maybeRequestWindow, "maybeRequestWindow");
    function assertAlive() {
      if (destroyed) throw new HexViewerDisposedError();
    }
    __name(assertAlive, "assertAlive");
    function setWindow(offset, length, nextWidth) {
      assertAlive();
      const nextOffset = requireIndex("setWindow(offset)", offset, 0, MAX_INDEX);
      const nextLength = requireIndex("setWindow(length)", length, 0, MAX_INDEX);
      const w = nextWidth === void 0 ? current.width : requireWidth(nextWidth);
      current = { offset: nextOffset, length: nextLength, width: w };
      render();
      maybeRequestWindow();
    }
    __name(setWindow, "setWindow");
    function destroy() {
      if (destroyed) return;
      destroyed = true;
      try {
        disposer.dispose();
      } catch (err) {
        console.error("[m32 hex-viewer] \u91CA\u653E\u76D1\u542C\u5668\u65F6\u629B\u9519:", err);
      }
      while (el.firstChild) el.removeChild(el.firstChild);
      const parent = el.parentNode;
      if (parent && typeof parent.removeChild === "function") parent.removeChild(el);
    }
    __name(destroy, "destroy");
    const onKeyDown = /* @__PURE__ */ __name((event) => {
      const e = event;
      const pageRows = readings.renderedRows > 0 ? readings.renderedRows : 1;
      const page = current.width * pageRows;
      let next = null;
      switch (e.key) {
        case "ArrowDown":
          next = current.offset + current.width;
          break;
        case "ArrowUp":
          next = Math.max(0, current.offset - current.width);
          break;
        case "PageDown":
          next = current.offset + page;
          break;
        case "PageUp":
          next = Math.max(0, current.offset - page);
          break;
        case "Home":
          next = 0;
          break;
        case "End":
          next = Math.max(0, rangeEnd - current.length);
          break;
        default:
          return;
      }
      if (typeof e.preventDefault === "function") e.preventDefault();
      setWindow(next, current.length);
    }, "onKeyDown");
    disposer.addEventListener(el, "keydown", onKeyDown);
    Object.assign(el, { setWindow, destroy });
    const getters = {
      viewWindow: /* @__PURE__ */ __name(() => ({ offset: current.offset, length: current.length, width: current.width }), "viewWindow"),
      providedRange: /* @__PURE__ */ __name(() => ({ start: rangeStart, end: rangeEnd }), "providedRange"),
      providedBytes: /* @__PURE__ */ __name(() => providedCount, "providedBytes"),
      rowCount: /* @__PURE__ */ __name(() => readings.rowCount, "rowCount"),
      renderedRows: /* @__PURE__ */ __name(() => readings.renderedRows, "renderedRows"),
      maxRows: /* @__PURE__ */ __name(() => maxRows, "maxRows"),
      truncated: /* @__PURE__ */ __name(() => readings.truncated, "truncated"),
      empty: /* @__PURE__ */ __name(() => readings.empty, "empty"),
      covered: /* @__PURE__ */ __name(() => readings.covered, "covered"),
      offsetDigits: /* @__PURE__ */ __name(() => readings.offsetDigits, "offsetDigits"),
      destroyed: /* @__PURE__ */ __name(() => destroyed, "destroyed")
    };
    for (const name of Object.keys(getters)) {
      Object.defineProperty(el, name, { configurable: true, enumerable: true, get: getters[name] });
    }
    render();
    maybeRequestWindow();
    return el;
  }
  __name(createHexViewer, "createHexViewer");

  // src/components/hex-patcher.ts
  var HEX_PATCH_CODES = {
    /** 含非 16 进制字符（或 `0x` 前缀 —— 本组件只收裸字节串） */
    BadHexChar: "BadHexChar",
    /** 16 进制位数为奇数 ⇒ 有半个字节，无法成字节 */
    OddHexDigits: "OddHexDigits",
    /** 字节数与 `size` 不符（**不截断、不补零**） */
    SizeMismatch: "SizeMismatch",
    /** 宿主校验器返回了非空原因（**原样透出**） */
    HostRejected: "HostRejected",
    /** 宿主校验器抛错（可观察降级，字段不卡死） */
    ValidatorError: "ValidatorError"
  };
  var ROOT_CLASS5 = "petui-hex-patcher";
  var META_CLASS = "petui-hex-patcher__meta";
  var INPUT_CLASS2 = "petui-hex-patcher__input";
  var STATUS_CLASS = "petui-hex-patcher__status";
  var BYTES_CLASS = "petui-hex-patcher__bytes";
  var APPLY_CLASS2 = "petui-hex-patcher__apply";
  var LAST_CLASS = "petui-hex-patcher__last";
  var STYLE_ID3 = "petui-hex-patcher-styles";
  var STYLE_RULES2 = [
    "." + ROOT_CLASS5 + "{display:flex;flex-direction:column;gap:var(--space-6)}",
    "." + META_CLASS + "{margin:0;font-size:var(--font-size-12);color:var(--color-text-secondary)}",
    "." + INPUT_CLASS2 + "{padding:var(--space-6) var(--space-8);border-style:solid;border-width:var(--space-1);border-color:var(--color-border);border-radius:var(--border-radius-md);font-size:var(--font-size-13);color:var(--color-text);background-color:var(--color-bg);outline:none}",
    "." + INPUT_CLASS2 + '[aria-invalid="true"]{border-color:var(--color-danger)}',
    "." + STATUS_CLASS + "{margin:0;font-size:var(--font-size-12);color:var(--color-text-muted)}",
    "." + STATUS_CLASS + '[data-state="error"]{color:var(--color-danger)}',
    "." + STATUS_CLASS + '[data-state="ok"]{color:var(--color-success)}',
    "." + BYTES_CLASS + "{margin:0;font-size:var(--font-size-12);color:var(--color-text-secondary)}",
    "." + APPLY_CLASS2 + "{align-self:flex-start;padding:var(--space-6) var(--space-12);border-style:solid;border-width:var(--space-1);border-color:var(--color-primary);border-radius:var(--border-radius-md);font-size:var(--font-size-13);color:var(--color-bg);background-color:var(--color-primary)}",
    "." + APPLY_CLASS2 + "[disabled]{color:var(--color-text-muted);background-color:var(--color-bg-light);border-color:var(--color-border)}",
    "." + LAST_CLASS + "{margin:0;font-size:var(--font-size-12);color:var(--color-text-muted)}"
  ];
  var autoIdSeq = 0;
  function toRaw3(value) {
    if (value === null || value === void 0) return "";
    return String(value);
  }
  __name(toRaw3, "toRaw");
  function isNonNegativeInt(value) {
    return typeof value === "number" && Number.isFinite(value) && Number.isInteger(value) && value >= 0;
  }
  __name(isNonNegativeInt, "isNonNegativeInt");
  function toHexPair(byte) {
    const text = byte.toString(16);
    return text.length === 1 ? "0" + text : text;
  }
  __name(toHexPair, "toHexPair");
  function diagnoseHexPatch(hex, size, validate) {
    const raw = toRaw3(hex);
    const compact = raw.replace(/\s+/g, "");
    const bad = compact.match(/[^0-9a-fA-F]/);
    if (bad !== null) {
      return {
        ok: false,
        code: HEX_PATCH_CODES.BadHexChar,
        reason: "\u542B\u975E 16 \u8FDB\u5236\u5B57\u7B26\u300C" + bad[0] + "\u300D\uFF08\u53EA\u63A5\u53D7 0-9 a-f A-F\uFF0C\u7A7A\u683C\u5916\u53EF\u7559\u7A7A\u767D\uFF09",
        bytes: [],
        byteLength: 0
      };
    }
    if (compact.length % 2 !== 0) {
      return {
        ok: false,
        code: HEX_PATCH_CODES.OddHexDigits,
        reason: "16 \u8FDB\u5236\u4F4D\u6570\u4E3A\u5947\u6570\uFF08" + String(compact.length) + " \u4F4D\uFF09\u2014\u2014 \u6709\u534A\u4E2A\u5B57\u8282\uFF0C\u65E0\u6CD5\u6210\u5B57\u8282",
        bytes: [],
        byteLength: 0
      };
    }
    const bytes = [];
    for (let i = 0; i < compact.length; i += 2) {
      bytes.push(parseInt(compact.slice(i, i + 2), 16));
    }
    if (bytes.length !== size) {
      return {
        ok: false,
        code: HEX_PATCH_CODES.SizeMismatch,
        reason: "\u5B57\u8282\u6570 " + String(bytes.length) + " \u4E0E size " + String(size) + " \u4E0D\u7B26\uFF08\u4E0D\u622A\u65AD\u3001\u4E0D\u8865\u96F6\uFF09\u2014\u2014 \u8BF7\u7ED9\u51FA\u6070\u597D " + String(size) + " \u5B57\u8282",
        bytes,
        byteLength: bytes.length
      };
    }
    if (validate !== void 0 && validate !== null) {
      if (typeof validate !== "function") {
        throw new TypeError("[PetUI hex-patcher] validate \u5FC5\u987B\u662F\u51FD\u6570");
      }
      let verdict;
      try {
        verdict = validate(bytes.slice());
      } catch (err) {
        console.error("[PetUI hex-patcher] validate \u629B\u9519\uFF0C\u5DF2\u964D\u7EA7\u4E3A ValidatorError:", err);
        return {
          ok: false,
          code: HEX_PATCH_CODES.ValidatorError,
          reason: "\u6821\u9A8C\u5668\u5F02\u5E38",
          bytes,
          byteLength: bytes.length
        };
      }
      if (verdict !== null && verdict !== void 0 && verdict !== false && String(verdict).trim() !== "") {
        return {
          ok: false,
          code: HEX_PATCH_CODES.HostRejected,
          reason: String(verdict),
          // **原样透出**：不 trim、不包裹、不改写
          bytes,
          byteLength: bytes.length
        };
      }
    }
    return { ok: true, code: "", reason: "", bytes, byteLength: bytes.length };
  }
  __name(diagnoseHexPatch, "diagnoseHexPatch");
  function injectStyles(doc) {
    if (typeof doc.getElementById === "function" && doc.getElementById(STYLE_ID3) !== null) return;
    const style = doc.createElement("style");
    style.id = STYLE_ID3;
    style.textContent = STYLE_RULES2.join("\n");
    const head = doc.head || doc.documentElement;
    if (head && typeof head.appendChild === "function") head.appendChild(style);
  }
  __name(injectStyles, "injectStyles");
  function createHexPatcher(options) {
    if (options === null || options === void 0) {
      throw new TypeError("[PetUI hex-patcher] \u9700\u8981\u4E00\u4E2A options \u5BF9\u8C61");
    }
    if (!isNonNegativeInt(options.size)) {
      throw new RangeError("[PetUI hex-patcher] size \u5FC5\u987B\u662F\u975E\u8D1F\u6574\u6570");
    }
    if (!isNonNegativeInt(options.offset)) {
      throw new RangeError("[PetUI hex-patcher] offset \u5FC5\u987B\u662F\u975E\u8D1F\u6574\u6570");
    }
    if (typeof options.onPatch !== "function") {
      throw new TypeError("[PetUI hex-patcher] onPatch \u5FC5\u987B\u662F\u51FD\u6570\uFF08\u771F\u6B63\u7684\u5199\u5165\u7531\u5BBF\u4E3B\u6267\u884C\uFF09");
    }
    if (options.validate !== void 0 && options.validate !== null && typeof options.validate !== "function") {
      throw new TypeError("[PetUI hex-patcher] validate \u5FC5\u987B\u662F\u51FD\u6570");
    }
    const doc = document;
    injectStyles(doc);
    const disposer = createDisposer();
    const offset = options.offset;
    const size = options.size;
    let raw = toRaw3(options.hex);
    let diagnosis = diagnoseHexPatch(raw, size, options.validate);
    let lastPatch = null;
    const root = doc.createElement("div");
    root.className = ROOT_CLASS5;
    const metaEl = doc.createElement("p");
    metaEl.className = META_CLASS;
    metaEl.textContent = "\u504F\u79FB offset=" + String(offset) + "\uFF080x" + offset.toString(16) + "\uFF09\xB7 \u671F\u671B " + String(size) + " \u5B57\u8282";
    root.appendChild(metaEl);
    const inputEl = doc.createElement("input");
    inputEl.type = "text";
    inputEl.className = INPUT_CLASS2;
    const idSeq = String(++autoIdSeq);
    const statusId = "petui-hex-patcher-status-" + idSeq;
    inputEl.setAttribute("aria-label", "\u5341\u516D\u8FDB\u5236\u5B57\u8282\u4E32\uFF08\u53EF\u542B\u7A7A\u683C\uFF09");
    inputEl.setAttribute("aria-describedby", statusId);
    inputEl.value = raw;
    root.appendChild(inputEl);
    const statusEl = doc.createElement("p");
    statusEl.className = STATUS_CLASS;
    statusEl.id = statusId;
    statusEl.setAttribute("aria-live", "polite");
    root.appendChild(statusEl);
    const bytesEl = doc.createElement("p");
    bytesEl.className = BYTES_CLASS;
    root.appendChild(bytesEl);
    const applyBtn = doc.createElement("button");
    applyBtn.type = "button";
    applyBtn.className = APPLY_CLASS2;
    applyBtn.textContent = "\u63D0\u4EA4\u8865\u4E01";
    root.appendChild(applyBtn);
    const lastEl = doc.createElement("p");
    lastEl.className = LAST_CLASS;
    root.appendChild(lastEl);
    const render = /* @__PURE__ */ __name(() => {
      root.setAttribute("data-state", diagnosis.ok ? "ok" : "error");
      root.setAttribute("data-code", diagnosis.code);
      inputEl.setAttribute("aria-invalid", diagnosis.ok ? "false" : "true");
      applyBtn.disabled = !diagnosis.ok;
      statusEl.setAttribute("data-state", diagnosis.ok ? "ok" : "error");
      statusEl.textContent = diagnosis.ok ? "\u5408\u6CD5\uFF1A\u53EF\u63D0\u4EA4 " + String(diagnosis.byteLength) + " \u5B57\u8282" : diagnosis.code + "\uFF1A" + diagnosis.reason;
      bytesEl.textContent = "\u89E3\u6790\uFF1A" + String(diagnosis.byteLength) + "/" + String(size) + " \u5B57\u8282" + (diagnosis.bytes.length > 0 ? " \xB7 " + diagnosis.bytes.map(toHexPair).join(" ") : "");
      if (lastPatch === null) {
        lastEl.setAttribute("data-patched", "none");
        lastEl.textContent = "\u5C1A\u672A\u63D0\u4EA4\uFF08\u771F\u6B63\u7684\u5199\u5165\u7531\u5BBF\u4E3B\u5728 onPatch \u91CC\u505A\uFF09";
      } else {
        lastEl.setAttribute("data-patched", "yes");
        lastEl.setAttribute("data-patch-offset", String(lastPatch.offset));
        lastEl.textContent = "\u4E0A\u6B21\u6D3E\u53D1\u7ED9\u5BBF\u4E3B\uFF1Aoffset=" + String(lastPatch.offset) + " (0x" + lastPatch.offset.toString(16) + ") \xB7 bytes=[" + lastPatch.bytes.join(",") + "]\uFF08" + String(lastPatch.bytes.length) + " \u5B57\u8282\uFF09";
      }
    }, "render");
    const refresh = /* @__PURE__ */ __name(() => {
      diagnosis = diagnoseHexPatch(raw, size, options.validate);
      render();
    }, "refresh");
    const onInput = /* @__PURE__ */ __name(() => {
      raw = toRaw3(inputEl.value);
      refresh();
    }, "onInput");
    const onSubmit = /* @__PURE__ */ __name(() => {
      if (disposer.disposed) {
        console.warn("[PetUI hex-patcher] destroy() \u4E4B\u540E\u63D0\u4EA4\u88AB\u5FFD\u7565");
        return;
      }
      if (!diagnosis.ok) {
        console.warn("[PetUI hex-patcher] \u975E\u6CD5\u503C\u4E0D\u53EF\u63D0\u4EA4\uFF1A" + diagnosis.code);
        return;
      }
      const bytes = diagnosis.bytes.slice();
      try {
        options.onPatch(offset, bytes);
      } catch (err) {
        console.error("[PetUI hex-patcher] onPatch\uFF08\u5BBF\u4E3B\u5199\u5165\uFF09\u629B\u9519:", err);
        lastEl.setAttribute("data-patched", "failed");
        lastEl.textContent = "\u5BBF\u4E3B\u5199\u5165\u5931\u8D25\uFF1A" + toRaw3(err?.message ?? err);
        return;
      }
      lastPatch = { offset, bytes };
      render();
    }, "onSubmit");
    disposer.addEventListener(inputEl, "input", onInput);
    disposer.addEventListener(applyBtn, "click", onSubmit);
    disposer.add(() => {
      const parent = root.parentNode;
      if (parent && typeof parent.removeChild === "function") parent.removeChild(root);
    });
    render();
    Object.defineProperties(root, {
      setValue: {
        value: /* @__PURE__ */ __name((hex) => {
          if (disposer.disposed) {
            console.warn("[PetUI hex-patcher] destroy() \u4E4B\u540E setValue \u88AB\u5FFD\u7565");
            return;
          }
          raw = toRaw3(hex);
          inputEl.value = raw;
          refresh();
        }, "value"),
        writable: true,
        configurable: true
      },
      getValue: {
        value: /* @__PURE__ */ __name(() => raw, "value"),
        writable: true,
        configurable: true
      },
      isValid: {
        value: /* @__PURE__ */ __name(() => !disposer.disposed && diagnosis.ok, "value"),
        writable: true,
        configurable: true
      },
      destroy: {
        value: /* @__PURE__ */ __name(() => {
          if (disposer.disposed) return;
          disposer.dispose();
        }, "value"),
        writable: true,
        configurable: true
      }
    });
    return root;
  }
  __name(createHexPatcher, "createHexPatcher");

  // src/components/icon-sprite.ts
  var SVG_NS2 = "http://www.w3.org/2000/svg";
  var SPRITE_HOST_ID = "petui-icon-sprite";
  var ICON_CLASS = "petui-icon";
  var ICON_SVG_CLASS = "petui-icon-svg";
  var ICON_SYMBOL_PREFIX = "petui-icon-";
  var ICON_FALLBACK_NAME = "unknown";
  var DEFAULT_ICON_SIZE_TOKEN = "--space-16";
  var FALLBACK_LABEL_SUFFIX = "\uFF08\u672A\u77E5\u56FE\u6807\uFF09";
  var ICON_VIEW_BOX = "0 0 24 24";
  var MAX_SPRITE_LENGTH = 8 * 1024;
  var MAX_ATTR_LENGTH = 512;
  var MAX_PATH_LENGTH = 4 * 1024;
  var _IconSizeError = class _IconSizeError extends Error {
    constructor(size) {
      super(`[PetUI icon-sprite] \u5C3A\u5BF8\u5FC5\u987B\u662F\u6570\u5B57\u6216\u4EE4\u724C\u540D\uFF08\u5982 '--space-16' / 'var(--space-16)'\uFF09\uFF0C\u6536\u5230\u300C${size}\u300D`);
      this.name = "IconSizeError";
      this.size = size;
    }
  };
  __name(_IconSizeError, "IconSizeError");
  var IconSizeError = _IconSizeError;
  var _IconSpriteContextError = class _IconSpriteContextError extends Error {
    constructor(action) {
      super(`[PetUI icon-sprite] \u7F3A\u5C11 document / createElementNS\uFF0C\u65E0\u6CD5\u6267\u884C ${action}`);
      this.name = "IconSpriteContextError";
      this.action = action;
    }
  };
  __name(_IconSpriteContextError, "IconSpriteContextError");
  var IconSpriteContextError = _IconSpriteContextError;
  var _IconSpriteDisposedError = class _IconSpriteDisposedError extends Error {
    constructor(action) {
      super(`[PetUI icon-sprite] \u5DF2 destroy()\uFF0C\u4E0D\u80FD\u518D\u8C03\u7528 ${action}()`);
      this.name = "IconSpriteDisposedError";
      this.action = action;
    }
  };
  __name(_IconSpriteDisposedError, "IconSpriteDisposedError");
  var IconSpriteDisposedError = _IconSpriteDisposedError;
  var FALLBACK_GLYPH = [
    {
      tag: "path",
      attrs: [
        ["d", "M12 2.6 L21.6 20.4 L2.4 20.4 Z"],
        ["fill", "none"],
        ["stroke", "currentColor"],
        ["stroke-width", "1.8"],
        ["stroke-linejoin", "round"]
      ]
    },
    {
      tag: "path",
      attrs: [
        ["d", "M12 9.2 L12 14.6"],
        ["fill", "none"],
        ["stroke", "currentColor"],
        ["stroke-width", "1.8"],
        ["stroke-linecap", "round"]
      ]
    },
    {
      tag: "circle",
      attrs: [
        ["cx", "12"],
        ["cy", "17.4"],
        ["r", "1"],
        ["fill", "currentColor"]
      ]
    }
  ];
  var SVG_ELEMENTS = /* @__PURE__ */ new Map([
    ["svg", "svg"],
    ["defs", "defs"],
    ["symbol", "symbol"],
    ["g", "g"],
    ["use", "use"],
    ["path", "path"],
    ["circle", "circle"],
    ["ellipse", "ellipse"],
    ["rect", "rect"],
    ["line", "line"],
    ["polyline", "polyline"],
    ["polygon", "polygon"]
  ]);
  var SVG_ATTRS = /* @__PURE__ */ new Map([
    ["id", "id"],
    ["viewbox", "viewBox"],
    ["preserveaspectratio", "preserveAspectRatio"],
    ["d", "d"],
    ["points", "points"],
    ["transform", "transform"],
    ["x", "x"],
    ["y", "y"],
    ["width", "width"],
    ["height", "height"],
    ["cx", "cx"],
    ["cy", "cy"],
    ["r", "r"],
    ["rx", "rx"],
    ["ry", "ry"],
    ["x1", "x1"],
    ["y1", "y1"],
    ["x2", "x2"],
    ["y2", "y2"],
    ["fill", "fill"],
    ["stroke", "stroke"],
    ["stroke-width", "stroke-width"],
    ["stroke-linecap", "stroke-linecap"],
    ["stroke-linejoin", "stroke-linejoin"],
    ["fill-rule", "fill-rule"],
    ["clip-rule", "clip-rule"],
    ["opacity", "opacity"],
    ["fill-opacity", "fill-opacity"],
    ["stroke-opacity", "stroke-opacity"],
    ["href", "href"]
  ]);
  var ATTR_KINDS = /* @__PURE__ */ new Map([
    ["id", "id"],
    ["viewbox", "viewBox"],
    ["href", "ref"],
    ["d", "path"],
    ["points", "points"],
    ["fill", "paint"],
    ["stroke", "paint"],
    ["transform", "transform"],
    ["preserveaspectratio", "aspect"],
    ["x", "number"],
    ["y", "number"],
    ["width", "number"],
    ["height", "number"],
    ["cx", "number"],
    ["cy", "number"],
    ["r", "number"],
    ["rx", "number"],
    ["ry", "number"],
    ["x1", "number"],
    ["y1", "number"],
    ["x2", "number"],
    ["y2", "number"],
    ["stroke-width", "number"],
    ["opacity", "number"],
    ["fill-opacity", "number"],
    ["stroke-opacity", "number"]
  ]);
  var ID_PATTERN = /^[A-Za-z][A-Za-z0-9_-]{0,63}$/;
  var REF_PATTERN = /^#[A-Za-z][A-Za-z0-9_-]{0,63}$/;
  var NUMBER_PATTERN = /^-?\d+(\.\d+)?%?$/;
  var VIEW_BOX_PATTERN = /^-?\d+(\.\d+)?([\s,]+-?\d+(\.\d+)?){3}$/;
  var POINTS_PATTERN = /^-?\d+(\.\d+)?([\s,]+-?\d+(\.\d+)?)*$/;
  var PATH_PATTERN = /^[MmLlHhVvCcSsQqTtAaZz0-9eE.,\s+-]+$/;
  var ASPECT_PATTERN = /^[A-Za-z]+(\s+[A-Za-z]+)*$/;
  var TRANSFORM_PATTERN = /^[A-Za-z]+\([^()]*\)(\s*[A-Za-z]+\([^()]*\))*$/;
  function judgeSpriteAttribute(lowerName, rawValue) {
    const canonical = SVG_ATTRS.get(lowerName);
    if (canonical === void 0) return null;
    const value = rawValue.trim();
    if (value.length === 0 || value.length > MAX_ATTR_LENGTH) return null;
    const kind = ATTR_KINDS.get(lowerName);
    switch (kind) {
      case "id":
        return ID_PATTERN.test(value) ? [canonical, value] : null;
      case "ref":
        return REF_PATTERN.test(value) ? [canonical, value] : null;
      case "paint":
        if (value === "none" || value === "currentColor") return [canonical, value];
        if (value.indexOf("var(") === 0 && isSafeColor(value)) return [canonical, value];
        return null;
      case "number":
        return NUMBER_PATTERN.test(value) ? [canonical, value] : null;
      case "viewBox":
        return VIEW_BOX_PATTERN.test(value) ? [canonical, value] : null;
      case "points":
        return POINTS_PATTERN.test(value) ? [canonical, value] : null;
      case "path":
        if (value.length > MAX_PATH_LENGTH) return null;
        return PATH_PATTERN.test(value) ? [canonical, value] : null;
      case "aspect":
        return ASPECT_PATTERN.test(value) ? [canonical, value] : null;
      case "transform":
        return TRANSFORM_PATTERN.test(value) ? [canonical, value] : null;
      default:
        return null;
    }
  }
  __name(judgeSpriteAttribute, "judgeSpriteAttribute");
  var NAME_HEAD = /[A-Za-z]/;
  var LIVE_CHAR = /[A-Za-z0-9]/;
  var TAG_NAME = /^[a-z][a-z0-9]*$/;
  var ATTR_BREAK = /[\s=>/]/;
  function scanSprite(sprite) {
    const src = String(sprite ?? "");
    const dropped = [];
    if (src.length === 0) return { ok: false, reason: "empty", dropped };
    if (src.length > MAX_SPRITE_LENGTH) return { ok: false, reason: "too-long", dropped };
    const stack = [];
    let root = null;
    let dropDepth = 0;
    let i = 0;
    const n = src.length;
    while (i < n) {
      const lt = src.indexOf("<", i);
      if (lt < 0) {
        if (dropDepth === 0 && src.slice(i).trim().length > 0) dropped.push("#text");
        break;
      }
      if (dropDepth === 0 && src.slice(i, lt).trim().length > 0) dropped.push("#text");
      if (src.startsWith("<!--", lt)) {
        const end = src.indexOf("-->", lt + 4);
        if (end < 0) return { ok: false, reason: "syntax", dropped };
        i = end + 3;
        continue;
      }
      if (src.startsWith("<?", lt)) {
        const end = src.indexOf("?>", lt + 2);
        if (end < 0) return { ok: false, reason: "syntax", dropped };
        i = end + 2;
        continue;
      }
      if (src.startsWith("<!", lt)) {
        const end = src.indexOf(">", lt + 2);
        if (end < 0) return { ok: false, reason: "syntax", dropped };
        i = end + 1;
        continue;
      }
      if (src.startsWith("</", lt)) {
        const end = src.indexOf(">", lt + 2);
        if (end < 0) return { ok: false, reason: "syntax", dropped };
        const name = src.slice(lt + 2, end).trim().toLowerCase();
        if (!TAG_NAME.test(name)) return { ok: false, reason: "syntax", dropped };
        if (dropDepth > 0) {
          dropDepth -= 1;
        } else {
          const open = stack.pop();
          if (open === void 0 || open.tag !== name) return { ok: false, reason: "syntax", dropped };
        }
        i = end + 1;
        continue;
      }
      let p = lt + 1;
      if (p >= n || !NAME_HEAD.test(src[p])) return { ok: false, reason: "syntax", dropped };
      let nameEnd = p;
      while (nameEnd < n && LIVE_CHAR.test(src[nameEnd])) nameEnd += 1;
      const tagLower = src.slice(p, nameEnd).toLowerCase();
      const rawAttrs = [];
      let selfClosing = false;
      let closed = false;
      p = nameEnd;
      while (p < n) {
        while (p < n && /\s/.test(src[p])) p += 1;
        if (p >= n) break;
        if (src[p] === ">") {
          p += 1;
          closed = true;
          break;
        }
        if (src[p] === "/" && src[p + 1] === ">") {
          p += 2;
          closed = true;
          selfClosing = true;
          break;
        }
        let attrEnd = p;
        while (attrEnd < n && !ATTR_BREAK.test(src[attrEnd])) attrEnd += 1;
        const attrName = src.slice(p, attrEnd).toLowerCase();
        if (attrName.length === 0) return { ok: false, reason: "syntax", dropped };
        p = attrEnd;
        while (p < n && /\s/.test(src[p])) p += 1;
        if (src[p] !== "=") return { ok: false, reason: "syntax", dropped };
        p += 1;
        while (p < n && /\s/.test(src[p])) p += 1;
        const quote = src[p];
        if (quote !== '"' && quote !== "'") return { ok: false, reason: "syntax", dropped };
        const valueEnd = src.indexOf(quote, p + 1);
        if (valueEnd < 0) return { ok: false, reason: "syntax", dropped };
        const rawValue = src.slice(p + 1, valueEnd);
        if (rawValue.indexOf("<") >= 0) return { ok: false, reason: "syntax", dropped };
        rawAttrs.push([attrName, rawValue]);
        p = valueEnd + 1;
      }
      if (!closed) return { ok: false, reason: "syntax", dropped };
      i = p;
      if (dropDepth > 0) {
        if (!selfClosing) dropDepth += 1;
        continue;
      }
      const canonicalTag = SVG_ELEMENTS.get(tagLower);
      if (canonicalTag === void 0) {
        if (dropped.length < 16) dropped.push(tagLower);
        if (!selfClosing) dropDepth = 1;
        continue;
      }
      const node = { tag: canonicalTag, attrs: [], children: [] };
      for (const pair of rawAttrs) {
        const accepted = judgeSpriteAttribute(pair[0], pair[1]);
        if (accepted !== null) node.attrs.push(accepted);
        else if (dropped.length < 16) dropped.push(tagLower + "@" + pair[0]);
      }
      if (stack.length === 0) {
        if (root !== null) return { ok: false, reason: "syntax", dropped };
        root = node;
      } else {
        stack[stack.length - 1].children.push(node);
      }
      if (!selfClosing) stack.push(node);
    }
    if (dropDepth > 0 || stack.length > 0) return { ok: false, reason: "syntax", dropped };
    if (root === null || root.tag !== "svg") return { ok: false, reason: "root", dropped };
    return { ok: true, root, dropped };
  }
  __name(scanSprite, "scanSprite");
  function buildSpriteNode(scan, doc) {
    const el = doc.createElementNS(SVG_NS2, scan.tag);
    for (const pair of scan.attrs) el.setAttribute(pair[0], pair[1]);
    for (const child of scan.children) el.appendChild(buildSpriteNode(child, doc));
    return el;
  }
  __name(buildSpriteNode, "buildSpriteNode");
  function countSymbols(node) {
    let total = 0;
    const kids = node.childNodes;
    for (let i = 0; i < kids.length; i += 1) {
      const child = kids[i];
      if (child.nodeType !== 1) continue;
      const el = child;
      if (String(el.tagName).toLowerCase() === "symbol") total += 1;
      total += countSymbols(el);
    }
    return total;
  }
  __name(countSymbols, "countSymbols");
  function resolveDocument3(preferred) {
    if (preferred !== null && preferred !== void 0) return preferred;
    if (typeof document !== "undefined" && document !== null) return document;
    return null;
  }
  __name(resolveDocument3, "resolveDocument");
  function ensureSpriteHost(doc) {
    if (typeof doc.getElementById !== "function") return null;
    const existing = doc.getElementById(SPRITE_HOST_ID);
    if (existing !== null && existing !== void 0) return existing;
    if (typeof doc.createElementNS !== "function") return null;
    const host = doc.createElementNS(SVG_NS2, "svg");
    host.setAttribute("id", SPRITE_HOST_ID);
    host.setAttribute("aria-hidden", "true");
    host.setAttribute("focusable", "false");
    host.setAttribute("data-icon-sprite", "host");
    host.style.setProperty("position", "absolute");
    host.style.setProperty("width", "0");
    host.style.setProperty("height", "0");
    host.style.setProperty("overflow", "hidden");
    const parent = doc.body ?? doc.documentElement ?? null;
    if (parent === null) return null;
    parent.appendChild(host);
    return host;
  }
  __name(ensureSpriteHost, "ensureSpriteHost");
  function k8LeftoverText(doc, sprite) {
    if (typeof doc.createElement !== "function") return "";
    const staging = doc.createElement("div");
    setSafeHTML(staging, sprite);
    return String(staging.textContent ?? "").trim();
  }
  __name(k8LeftoverText, "k8LeftoverText");
  function injectSprite(sprite, doc) {
    const target = resolveDocument3(doc ?? null);
    const text = String(sprite ?? "");
    if (target === null) {
      console.warn("[PetUI icon-sprite] \u65E0 document\uFF08SSR / \u65E0 DOM \u73AF\u5883\uFF09\uFF0Csprite \u672A\u6CE8\u5165\uFF1B\u56FE\u6807\u5C06\u4F7F\u7528\u5185\u5EFA\u964D\u7EA7\u5B57\u5F62");
      return { injected: false, added: 0, total: 0, reason: "no-document", dropped: [] };
    }
    if (typeof target.createElementNS !== "function" || typeof target.getElementById !== "function") {
      console.warn("[PetUI icon-sprite] \u5BBF\u4E3B\u7F3A\u5C11 createElementNS / getElementById\uFF0Csprite \u672A\u6CE8\u5165");
      return { injected: false, added: 0, total: 0, reason: "no-namespace-api", dropped: [] };
    }
    const leftover = k8LeftoverText(target, text);
    const scan = scanSprite(text);
    if (!scan.ok) {
      console.warn(
        `[PetUI icon-sprite] sprite \u88AB\u62D2\uFF08reason=${scan.reason}\uFF09\uFF1A\u53EA\u63A5\u53D7\u4E25\u683C XML \u7684\u300Csvg\u300D+\u300Csymbol id=\u2026\u300D\u56FE\u5143\u96C6\u5408`
      );
      return { injected: false, added: 0, total: 0, reason: scan.reason, dropped: scan.dropped };
    }
    if (scan.dropped.length > 0) {
      console.warn(`[PetUI icon-sprite] sprite \u5185\u975E\u767D\u540D\u5355\u5185\u5BB9\u5DF2\u4E22\u5F03\uFF1A${scan.dropped.join(" / ")}`);
    }
    if (leftover.length > 0) {
      console.warn(
        `[PetUI icon-sprite] setSafeHTML \u628A sprite \u5185\u5BB9\u63D0\u5347\u4E3A\u6587\u672C\uFF08K8 \u767D\u540D\u5355\u4E0D\u542B SVG \u547D\u540D\u7A7A\u95F4\uFF0C\u5DF2\u767B\u8BB0\u4E3A\u672A\u51B3\u9879\uFF09\uFF1A${leftover.slice(0, 80)}`
      );
    }
    const host = ensureSpriteHost(target);
    if (host === null) {
      console.warn("[PetUI icon-sprite] \u65E0\u6CD5\u521B\u5EFA sprite \u5BBF\u4E3B\uFF08\u7F3A body / documentElement\uFF09\uFF0Csprite \u672A\u6CE8\u5165");
      return { injected: false, added: 0, total: 0, reason: "no-host", dropped: scan.dropped };
    }
    let added = 0;
    for (const child of scan.root.children) {
      if (child.tag !== "symbol" && child.tag !== "defs") continue;
      if (child.tag === "symbol") {
        const idAttr = child.attrs.filter((pair) => pair[0] === "id")[0];
        if (idAttr === void 0) continue;
        if (target.getElementById(idAttr[1]) !== null) continue;
      }
      host.appendChild(buildSpriteNode(child, target));
      added += 1;
    }
    const total = countSymbols(host);
    return {
      injected: added > 0,
      added,
      total,
      reason: added > 0 ? "added" : "duplicate",
      dropped: scan.dropped
    };
  }
  __name(injectSprite, "injectSprite");
  function iconSymbolId(name) {
    return ICON_SYMBOL_PREFIX + String(name ?? "");
  }
  __name(iconSymbolId, "iconSymbolId");
  function normalizeIconName(raw) {
    const name = String(raw ?? "").trim();
    return ID_PATTERN.test(name) ? name : null;
  }
  __name(normalizeIconName, "normalizeIconName");
  function tokenExpression(token) {
    return "var(" + token + ")";
  }
  __name(tokenExpression, "tokenExpression");
  function normalizeSizeToken(size) {
    const raw = String(size).trim();
    if (/^\d+$/.test(raw)) return "--space-" + raw;
    if (/^--[a-z0-9-]+$/.test(raw)) return raw;
    if (/^var\(--[a-z0-9-]+\)$/.test(raw)) return raw.slice(4, raw.length - 1);
    throw new IconSizeError(raw);
  }
  __name(normalizeSizeToken, "normalizeSizeToken");
  function resolveSize2(size) {
    let token = size === void 0 || size === null ? DEFAULT_ICON_SIZE_TOKEN : normalizeSizeToken(size);
    if (!isRegistered(token)) {
      console.warn(`[PetUI icon-sprite] \u5C3A\u5BF8\u4EE4\u724C ${token} \u672A\u5728\u4EE4\u724C\u6E90\u767B\u8BB0\uFF08K1\uFF09\uFF0C\u9000\u56DE\u9ED8\u8BA4 ${DEFAULT_ICON_SIZE_TOKEN}`);
      token = DEFAULT_ICON_SIZE_TOKEN;
    }
    return { token, expression: tokenExpression(token) };
  }
  __name(resolveSize2, "resolveSize");
  var ICON_STATE = /* @__PURE__ */ new WeakMap();
  function clearChildren3(el) {
    if (typeof el.replaceChildren === "function") {
      el.replaceChildren();
      return;
    }
    while (el.firstChild !== null) el.removeChild(el.firstChild);
  }
  __name(clearChildren3, "clearChildren");
  function createSvgNode(doc, tag) {
    if (typeof doc.createElementNS !== "function") throw new IconSpriteContextError(`createElementNS(${tag})`);
    return doc.createElementNS(SVG_NS2, tag);
  }
  __name(createSvgNode, "createSvgNode");
  function renderFallbackGlyph(svg, doc) {
    if (doc === null || typeof doc.createElementNS !== "function") {
      svg.setAttribute("data-icon-glyph", "unavailable");
      return;
    }
    for (const spec of FALLBACK_GLYPH) {
      const node = doc.createElementNS(SVG_NS2, spec.tag);
      for (const pair of spec.attrs) node.setAttribute(pair[0], pair[1]);
      svg.appendChild(node);
    }
    svg.setAttribute("data-icon-glyph", "inline-fallback");
  }
  __name(renderFallbackGlyph, "renderFallbackGlyph");
  function fallbackLabel(label, requested) {
    const base = label !== null && label.length > 0 ? label : requested.length > 0 ? requested : "\uFF08\u7A7A\u540D\uFF09";
    return base + FALLBACK_LABEL_SUFFIX;
  }
  __name(fallbackLabel, "fallbackLabel");
  function renderFallback(el, state, kind) {
    const svg = state.svg;
    if (svg === null) return;
    clearChildren3(svg);
    renderFallbackGlyph(svg, state.doc);
    state.effective = ICON_FALLBACK_NAME;
    state.fallback = kind;
    el.setAttribute("data-icon-name", ICON_FALLBACK_NAME);
    el.setAttribute("data-icon-fallback", kind);
    el.removeAttribute("data-icon-symbol");
    el.setAttribute("aria-label", fallbackLabel(state.label, state.requested));
  }
  __name(renderFallback, "renderFallback");
  function renderIcon(el, state) {
    const requested = String(state.requested ?? "").trim();
    const safe = normalizeIconName(requested);
    let symbol = null;
    if (safe !== null && state.doc !== null && typeof state.doc.getElementById === "function") {
      const found = state.doc.getElementById(iconSymbolId(safe));
      if (found !== null && found !== void 0 && String(found.tagName).toLowerCase() === "symbol") symbol = found;
    }
    if (safe === null || symbol === null || state.doc === null || state.svg === null) {
      renderFallback(el, state, "unknown");
      el.setAttribute("data-icon-request", requested);
      return false;
    }
    clearChildren3(state.svg);
    const useEl = createSvgNode(state.doc, "use");
    useEl.setAttribute("href", "#" + iconSymbolId(safe));
    useEl.setAttribute("data-icon-use", safe);
    state.svg.appendChild(useEl);
    state.effective = safe;
    state.fallback = null;
    el.setAttribute("data-icon-name", safe);
    el.setAttribute("data-icon-symbol", iconSymbolId(safe));
    el.removeAttribute("data-icon-fallback");
    el.removeAttribute("data-icon-request");
    el.setAttribute("aria-label", state.label !== null && state.label.length > 0 ? state.label : safe);
    return true;
  }
  __name(renderIcon, "renderIcon");
  function onUseError(el) {
    const state = ICON_STATE.get(el);
    if (state === void 0 || state.disposed) return;
    renderFallback(el, state, "sprite");
  }
  __name(onUseError, "onUseError");
  function defineReadonly(el, key, get) {
    Object.defineProperty(el, key, { get, enumerable: true, configurable: false });
  }
  __name(defineReadonly, "defineReadonly");
  function defineIconApi(el, state, disposer) {
    el.setIcon = (name) => {
      setIcon(el, name);
    };
    el.destroy = () => {
      destroyIcon(el);
    };
    defineReadonly(el, "name", () => state.effective);
    defineReadonly(el, "size", () => state.sizeExpression);
    defineReadonly(el, "sizeToken", () => state.sizeToken);
    defineReadonly(
      el,
      "symbolId",
      () => state.fallback === null ? iconSymbolId(state.effective) : null
    );
    defineReadonly(el, "fallback", () => state.fallback);
    defineReadonly(el, "disposed", () => state.disposed);
    disposer.addEventListener(el, "error", () => onUseError(el), { capture: true });
  }
  __name(defineIconApi, "defineIconApi");
  function destroyIcon(el) {
    const state = ICON_STATE.get(el);
    if (state === void 0 || state.disposed) return;
    state.disposed = true;
    const disposer = state.disposer;
    if (disposer !== null) {
      try {
        disposer.dispose();
      } catch (err) {
        console.error("[PetUI icon-sprite] destroy() \u7684\u91CA\u653E\u52A8\u4F5C\u629B\u9519\uFF08\u5DF2\u4E0A\u62A5\uFF1B\u8282\u70B9\u4ECD\u4F1A\u81EA DOM \u6458\u9664\uFF09:", err);
      }
    }
    const parent = el.parentNode;
    if (parent !== null) parent.removeChild(el);
  }
  __name(destroyIcon, "destroyIcon");
  function initIcon(node, doc, size, label) {
    const el = node;
    const prior = ICON_STATE.get(el);
    if (prior !== void 0) return el;
    const svg = createSvgNode(doc, "svg");
    svg.setAttribute("class", ICON_SVG_CLASS);
    svg.setAttribute("viewBox", ICON_VIEW_BOX);
    svg.setAttribute("width", "100%");
    svg.setAttribute("height", "100%");
    svg.setAttribute("fill", "currentColor");
    svg.setAttribute("aria-hidden", "true");
    svg.setAttribute("focusable", "false");
    const state = {
      doc,
      sizeToken: size.token,
      sizeExpression: size.expression,
      label,
      svg,
      disposer: createDisposer(),
      requested: "",
      effective: ICON_FALLBACK_NAME,
      fallback: "unknown",
      disposed: false
    };
    ICON_STATE.set(el, state);
    el.setAttribute("class", ICON_CLASS);
    el.setAttribute("role", "img");
    el.setAttribute("data-icon-size", size.token);
    if (el.style !== void 0 && typeof el.style.setProperty === "function") {
      el.style.setProperty("width", size.expression);
      el.style.setProperty("height", size.expression);
      el.style.setProperty("display", "inline-block");
      el.style.setProperty("vertical-align", "middle");
    }
    el.appendChild(svg);
    const disposer = state.disposer;
    if (disposer !== null) defineIconApi(el, state, disposer);
    return el;
  }
  __name(initIcon, "initIcon");
  function createIcon(options = {}) {
    const doc = resolveDocument3(options.doc ?? null);
    if (doc === null) throw new IconSpriteContextError("createIcon");
    const size = resolveSize2(options.size);
    const el = initIcon(doc.createElement("span"), doc, size, options.label ?? null);
    setIcon(el, options.name ?? ICON_FALLBACK_NAME);
    return el;
  }
  __name(createIcon, "createIcon");
  function setIcon(node, name) {
    const el = node;
    let state = ICON_STATE.get(el);
    if (state === void 0) {
      const doc = resolveDocument3(el.ownerDocument ?? null);
      if (doc === null) throw new IconSpriteContextError("setIcon");
      initIcon(el, doc, resolveSize2(void 0), null);
      state = ICON_STATE.get(el);
      if (state === void 0) throw new IconSpriteContextError("setIcon");
    }
    if (state.disposed) throw new IconSpriteDisposedError("setIcon");
    state.requested = String(name ?? "").trim();
    return renderIcon(el, state);
  }
  __name(setIcon, "setIcon");

  // src/lib/dom-clear.ts
  function clearChildren4(host) {
    while (host.firstChild) host.removeChild(host.firstChild);
  }
  __name(clearChildren4, "clearChildren");

  // src/components/panel-dom.ts
  var PANEL_CLASS = "petui-panel";
  var PANEL_FILL_CLASS = "petui-panel--fill";
  var PANEL_TITLE_CLASS = "petui-panel__title";
  var PANEL_HINT_CLASS = "petui-panel__hint";
  var PANEL_BODY_CLASS = "petui-panel__body";
  var PANEL_ACTIONS_CLASS = "petui-panel__actions";
  function panelRootStyle(fixed) {
    return [
      "display:flex",
      "flex-direction:column",
      "min-height:0",
      "background:var(--color-bg-light)",
      "border:var(--space-1) solid var(--color-border)",
      "border-radius:var(--border-radius-md)",
      "color:var(--color-text)",
      "font-size:var(--font-size-14)",
      fixed ? "flex-shrink:0" : "flex:1 1 auto"
    ].join(";");
  }
  __name(panelRootStyle, "panelRootStyle");
  function setTitleContent(host, title) {
    clearChildren4(host);
    if (title === null || title === void 0) return;
    if (typeof title === "string") {
      host.textContent = title;
      return;
    }
    host.appendChild(title);
  }
  __name(setTitleContent, "setTitleContent");
  function buildHead(o) {
    const head = document.createElement("div");
    head.className = PANEL_TITLE_CLASS;
    head.style.cssText = [
      "display:flex",
      "align-items:center",
      "gap:var(--space-8)",
      "padding:var(--space-8) var(--space-12)",
      "border-bottom:var(--space-1) solid var(--color-border)",
      "color:var(--color-text)",
      "font-size:var(--font-size-14)"
    ].join(";");
    const titleEl = document.createElement("span");
    titleEl.style.cssText = "flex:1 1 auto;min-width:0";
    if (o.titleClass) titleEl.className = o.titleClass;
    setTitleContent(titleEl, o.title);
    head.appendChild(titleEl);
    if (o.hint !== null && o.hint !== void 0) head.appendChild(buildHint(o.hint, o.titleClass));
    for (const extra of o.titleExtra || []) if (extra) head.appendChild(extra);
    return { head, titleEl };
  }
  __name(buildHead, "buildHead");
  function buildHint(hint, titleClass) {
    if (typeof hint !== "string") return hint;
    const span = document.createElement("span");
    span.className = PANEL_HINT_CLASS + (titleClass ? " " + titleClass : "");
    span.style.cssText = "color:var(--color-text-secondary);font-size:var(--font-size-12)";
    span.textContent = hint;
    return span;
  }
  __name(buildHint, "buildHint");
  function buildBody(o) {
    const want = o.bodyMode === "if-content" ? !!(o.body && o.body.length) : true;
    if (!want) return null;
    const body = document.createElement("div");
    const cls = [PANEL_BODY_CLASS];
    if (o.bodyFill) cls.push("petui-panel__body--fill");
    if (o.bodyScroll) cls.push("petui-panel__body--scroll");
    if (o.bodyRow) cls.push("petui-panel__body--row");
    if (o.bodyClass) cls.push(o.bodyClass);
    body.className = cls.join(" ");
    const css = ["padding:var(--space-8)", "min-height:0"];
    if (o.bodyFill) css.push("flex:1 1 auto");
    if (o.bodyFlex) css.push("display:flex", "flex-direction:column");
    if (o.bodyScroll) css.push("overflow:auto");
    if (o.bodyRow) css.push("display:flex", "flex-direction:row", "gap:var(--space-8)", "flex-wrap:wrap");
    body.style.cssText = css.join(";");
    for (const node of o.body || []) if (node) body.appendChild(node);
    return body;
  }
  __name(buildBody, "buildBody");
  function buildActions() {
    const actions = document.createElement("div");
    actions.className = PANEL_ACTIONS_CLASS;
    actions.style.cssText = [
      "display:flex",
      "align-items:center",
      "gap:var(--space-8)",
      "flex-wrap:wrap",
      "padding:var(--space-8) var(--space-12)",
      "border-top:var(--space-1) solid var(--color-border)"
    ].join(";");
    return actions;
  }
  __name(buildActions, "buildActions");
  function appendNodes(host, nodes) {
    for (const node of nodes || []) if (node) host.appendChild(node);
  }
  __name(appendNodes, "appendNodes");

  // src/components/panel.ts
  function createPanel(options = {}) {
    const o = options;
    const el = document.createElement(o.tag || "section");
    el.className = [PANEL_CLASS, o.fill ? PANEL_FILL_CLASS : "", o.className || ""].filter(Boolean).join(" ");
    el.style.cssText = panelRootStyle(o.fixed === true);
    const built = o.bare === true ? null : buildHead(o);
    const body = buildBody(o);
    const actions = buildActions();
    if (built) el.appendChild(built.head);
    if (body) el.appendChild(body);
    if (o.actions && o.actions.length) {
      appendNodes(actions, o.actions);
      el.appendChild(actions);
    }
    return Object.assign(el, {
      head: built ? built.head : null,
      body,
      actions,
      appendNodes(nodes) {
        if (!body || !body.parentNode) return null;
        appendNodes(body, nodes);
        return body;
      },
      setActions(nodes) {
        while (actions.firstChild) actions.removeChild(actions.firstChild);
        appendNodes(actions, nodes);
        if (actions.childNodes.length && !actions.parentNode) el.appendChild(actions);
        return actions;
      },
      setTitle(title) {
        if (built) setTitleContent(built.titleEl, title);
      },
      destroy() {
        while (el.firstChild) el.removeChild(el.firstChild);
      }
    });
  }
  __name(createPanel, "createPanel");

  // src/components/row.ts
  var ALIGN = {
    start: "flex-start",
    center: "center",
    end: "flex-end",
    stretch: "stretch",
    baseline: "baseline"
  };
  var JUSTIFY = {
    start: "flex-start",
    center: "center",
    end: "flex-end",
    between: "space-between",
    around: "space-around"
  };
  function buildFlex(nodes, opts, direction) {
    const o = opts || {};
    const el = document.createElement(o.tag || "div");
    el.className = [direction === "row" ? "petui-row" : "petui-stack", o.extraClass || ""].filter(Boolean).join(" ");
    const css = ["display:flex", "flex-direction:" + direction, "min-width:0", "min-height:0"];
    if (direction === "row") css.push("align-items:" + ALIGN[o.align || "center"]);
    else css.push("align-items:" + ALIGN[o.align || "stretch"]);
    css.push("justify-content:" + JUSTIFY[o.justify || "start"]);
    if (o.gap !== null) css.push("gap:" + (o.gap || "var(--space-8)"));
    if (o.wrap) css.push("flex-wrap:wrap");
    el.style.cssText = css.join(";");
    for (const node of nodes || []) if (node) el.appendChild(node);
    return el;
  }
  __name(buildFlex, "buildFlex");
  function createRow(nodes = [], opts = {}) {
    return buildFlex(nodes, opts, "row");
  }
  __name(createRow, "createRow");
  function createStack(nodes = [], opts = {}) {
    return buildFlex(nodes, opts, "column");
  }
  __name(createStack, "createStack");

  // src/lib/edit-session.ts
  function deepSnapshot(value, seen) {
    if (value === null || typeof value !== "object") return value;
    const source = value;
    const cached = seen.get(source);
    if (cached !== void 0) return cached;
    if (source instanceof Date) return new Date(source.getTime());
    if (source instanceof RegExp) return new RegExp(source.source, source.flags);
    if (Array.isArray(source)) {
      const out2 = [];
      seen.set(source, out2);
      for (const item of source) out2.push(deepSnapshot(item, seen));
      return Object.freeze(out2);
    }
    if (source instanceof Map) {
      const out2 = /* @__PURE__ */ new Map();
      seen.set(source, out2);
      for (const [k, v] of source) out2.set(deepSnapshot(k, seen), deepSnapshot(v, seen));
      return out2;
    }
    if (source instanceof Set) {
      const out2 = /* @__PURE__ */ new Set();
      seen.set(source, out2);
      for (const item of source) out2.add(deepSnapshot(item, seen));
      return out2;
    }
    const out = {};
    seen.set(source, out);
    const record = source;
    for (const key of Object.keys(record)) out[key] = deepSnapshot(record[key], seen);
    return Object.freeze(out);
  }
  __name(deepSnapshot, "deepSnapshot");
  function snapshot(value) {
    return deepSnapshot(value, /* @__PURE__ */ new WeakMap());
  }
  __name(snapshot, "snapshot");
  function freezeKey(rowKey, colKey) {
    return Object.freeze([rowKey, colKey]);
  }
  __name(freezeKey, "freezeKey");
  function rejection(code, reason, remedy) {
    return Object.freeze({
      ok: false,
      discarded: false,
      code,
      reason,
      remedy: Object.freeze(remedy)
    });
  }
  __name(rejection, "rejection");
  function submitRejection(code, reason, remedy, error) {
    return Object.freeze({
      ok: false,
      code,
      reason,
      remedy: Object.freeze(remedy),
      error
    });
  }
  __name(submitRejection, "submitRejection");
  function defaultDiscardRemedy() {
    return {
      code: "save-then-discard",
      hint: "\u5148 submit() \u4FDD\u5B58\u6539\u52A8\uFF0C\u6216\u4FEE\u590D\u5B88\u536B\u5224\u5B9A\u6761\u4EF6\u540E\u91CD\u8BD5 discard()\uFF1B\u672C\u6A21\u5757\u4E0D\u63D0\u4F9B\u9759\u9ED8\u4E22\u5F03\u3002"
    };
  }
  __name(defaultDiscardRemedy, "defaultDiscardRemedy");
  function normalizeRemedy(raw) {
    if (typeof raw === "string" && raw.length > 0) {
      return { code: raw, hint: `\u6309\u51FA\u8DEF\u201C${raw}\u201D\u5904\u7406\u540E\u91CD\u8BD5 discard()\u3002` };
    }
    if (raw && typeof raw === "object" && typeof raw.code === "string" && raw.code.length > 0) {
      return {
        code: raw.code,
        hint: typeof raw.hint === "string" && raw.hint.length > 0 ? raw.hint : `\u6309\u51FA\u8DEF\u201C${raw.code}\u201D\u5904\u7406\u540E\u91CD\u8BD5 discard()\u3002`
      };
    }
    return defaultDiscardRemedy();
  }
  __name(normalizeRemedy, "normalizeRemedy");
  function errorMessage(error) {
    if (error instanceof Error && typeof error.message === "string" && error.message.length > 0) {
      return error.message;
    }
    return String(error);
  }
  __name(errorMessage, "errorMessage");
  function toChange(entry) {
    return Object.freeze({
      rowKey: entry.rowKey,
      colKey: entry.colKey,
      key: freezeKey(entry.rowKey, entry.colKey),
      value: snapshot(entry.value),
      previous: entry.hadPrevious ? snapshot(entry.previous) : void 0,
      hadPrevious: entry.hadPrevious
    });
  }
  __name(toChange, "toChange");
  function createEditSession(options = {}) {
    const { onSubmit, onDirtyChange, onDiscardGuard } = options;
    const pending = /* @__PURE__ */ new Map();
    const committed = /* @__PURE__ */ new Map();
    let revision = 0;
    let submitting = false;
    function pendingRow(rowKey) {
      return pending.get(rowKey);
    }
    __name(pendingRow, "pendingRow");
    function committedValue(rowKey, colKey) {
      const row = committed.get(rowKey);
      if (!row || !row.has(colKey)) return { found: false, value: void 0 };
      return { found: true, value: row.get(colKey) };
    }
    __name(committedValue, "committedValue");
    function collectKeys() {
      const keys = [];
      for (const [rowKey, cols] of pending) {
        for (const colKey of cols.keys()) keys.push(freezeKey(rowKey, colKey));
      }
      return keys;
    }
    __name(collectKeys, "collectKeys");
    function collectChanges() {
      const changes = [];
      for (const cols of pending.values()) {
        for (const entry of cols.values()) changes.push(toChange(entry));
      }
      return changes;
    }
    __name(collectChanges, "collectChanges");
    function notifyDirtyChange() {
      if (!onDirtyChange) return;
      onDirtyChange(Object.freeze(collectKeys()), session);
    }
    __name(notifyDirtyChange, "notifyDirtyChange");
    function selectChanges(target) {
      if (target === void 0) return collectChanges();
      const selected = [];
      const seen = /* @__PURE__ */ new Set();
      for (const key of target) {
        const rowKey = String(key[0]);
        const colKey = String(key[1]);
        const token = `${rowKey}\0${colKey}`;
        if (seen.has(token)) continue;
        seen.add(token);
        const entry = pendingRow(rowKey)?.get(colKey);
        if (entry) selected.push(toChange(entry));
      }
      return selected;
    }
    __name(selectChanges, "selectChanges");
    function applyDiscard(changes) {
      for (const change of changes) {
        const row = pendingRow(change.rowKey);
        if (!row) continue;
        row.delete(change.colKey);
        if (row.size === 0) pending.delete(change.rowKey);
      }
      if (changes.length > 0) notifyDirtyChange();
      return Object.freeze({
        ok: true,
        discarded: true,
        keys: Object.freeze(changes.map((c) => c.key)),
        changes: Object.freeze(changes.slice())
      });
    }
    __name(applyDiscard, "applyDiscard");
    async function runGuard(origin, changes) {
      if (!onDiscardGuard) return Object.freeze({ ok: true });
      const request = Object.freeze({
        origin,
        keys: Object.freeze(changes.map((c) => c.key)),
        changes: Object.freeze(changes.slice())
      });
      let raw;
      try {
        raw = await onDiscardGuard(request, session);
      } catch (error) {
        return rejection(
          "guard-error",
          `\u4E22\u5F03\u5B88\u536B\u629B\u9519\uFF0C\u5DF2\u6309\u62D2\u7EDD\u5904\u7406\uFF1A${errorMessage(error)}`,
          {
            code: "fix-guard-error",
            hint: "\u4FEE\u590D onDiscardGuard \u7684\u5F02\u5E38\uFF08\u6216\u6539\u4E3A\u8FD4\u56DE DiscardDecision\uFF09\u540E\u91CD\u8BD5 discard()\u3002"
          }
        );
      }
      if (raw === void 0 || raw === null || raw === true) return Object.freeze({ ok: true });
      if (raw === false) {
        return rejection(
          "guard-denied",
          `\u4E22\u5F03\u5B88\u536B\u62D2\u7EDD\u4E86\u672C\u6B21\u4E22\u5F03\uFF08origin=${origin}\uFF0C\u6D89\u53CA ${changes.length} \u9879\u810F\u503C\uFF09\u3002`,
          defaultDiscardRemedy()
        );
      }
      if (typeof raw === "object") {
        if (raw.ok === true) return Object.freeze({ ok: true });
        const code = typeof raw.code === "string" && raw.code.length > 0 ? raw.code : "guard-denied";
        const reason = typeof raw.reason === "string" && raw.reason.length > 0 ? raw.reason : `\u4E22\u5F03\u5B88\u536B\u62D2\u7EDD\u4E86\u672C\u6B21\u4E22\u5F03\uFF08origin=${origin}\uFF09\u4F46\u672A\u7ED9\u51FA\u539F\u56E0\u3002`;
        return rejection(code, reason, normalizeRemedy(raw.remedy));
      }
      return rejection(
        "guard-contract-violation",
        `\u4E22\u5F03\u5B88\u536B\u8FD4\u56DE\u503C\u975E\u6CD5\uFF08${typeof raw}\uFF09\uFF0C\u6309\u62D2\u7EDD\u5904\u7406\uFF1B\u5B88\u536B\u5FC5\u987B\u8FD4\u56DE true/false \u6216 { ok, code?, reason?, remedy? }\u3002`,
        {
          code: "fix-guard-return",
          hint: "\u4FEE\u6B63 onDiscardGuard \u7684\u8FD4\u56DE\u503C\u4E3A DiscardDecision \u540E\u91CD\u8BD5 discard()\u3002"
        }
      );
    }
    __name(runGuard, "runGuard");
    const session = {
      setValue(rowKey, colKey, value) {
        const rk = String(rowKey);
        const ck = String(colKey);
        if (rk.length === 0 || ck.length === 0) {
          throw new Error("[edit-session] \u7F16\u8F91\u952E\u4E0D\u80FD\u4E3A\u7A7A\uFF1ArowKey \u4E0E colKey \u90FD\u5FC5\u987B\u662F\u975E\u7A7A\u5B57\u7B26\u4E32\u3002");
        }
        let row = pending.get(rk);
        if (!row) {
          row = /* @__PURE__ */ new Map();
          pending.set(rk, row);
        }
        const existed = row.has(ck);
        const base = committedValue(rk, ck);
        const entry = {
          rowKey: rk,
          colKey: ck,
          // 登记时即快照：调用方之后改自己那个对象，不影响会话内部状态。
          value: snapshot(value),
          previous: base.found ? snapshot(base.value) : void 0,
          hadPrevious: base.found,
          revision: revision += 1
        };
        row.set(ck, entry);
        if (!existed) notifyDirtyChange();
        return toChange(entry);
      },
      markDirty(rowKey, colKey, value) {
        return session.setValue(rowKey, colKey, value);
      },
      has(rowKey, colKey) {
        return pendingRow(String(rowKey))?.has(String(colKey)) === true;
      },
      getValue(rowKey, colKey) {
        const rk = String(rowKey);
        const ck = String(colKey);
        const entry = pendingRow(rk)?.get(ck);
        if (entry) return snapshot(entry.value);
        const base = committedValue(rk, ck);
        return base.found ? snapshot(base.value) : void 0;
      },
      getBaselineValue(rowKey, colKey) {
        const base = committedValue(String(rowKey), String(colKey));
        return base.found ? snapshot(base.value) : void 0;
      },
      isDirty() {
        return pending.size > 0;
      },
      dirtyCount() {
        let count = 0;
        for (const row of pending.values()) count += row.size;
        return count;
      },
      getDirtyKeys() {
        return Object.freeze(collectKeys());
      },
      getChanges() {
        return Object.freeze(collectChanges());
      },
      getChangesByRow() {
        const out = {};
        for (const [rowKey, cols] of pending) {
          const rowOut = {};
          for (const [colKey, entry] of cols) rowOut[colKey] = snapshot(entry.value);
          out[rowKey] = Object.freeze(rowOut);
        }
        return Object.freeze(out);
      },
      async submit() {
        if (submitting) {
          return submitRejection(
            "submit-in-flight",
            "\u4E0A\u4E00\u6B21\u63D0\u4EA4\u5C1A\u672A\u7ED3\u675F\uFF0C\u672C\u6B21\u63D0\u4EA4\u672A\u6267\u884C\u3002",
            { code: "await-pending-submit", hint: "\u7B49\u5F85\u4E0A\u4E00\u6B21 submit() \u7684\u7ED3\u679C\u540E\u518D\u51B3\u5B9A\u662F\u5426\u91CD\u63D0\u3002" }
          );
        }
        const cutoff = revision;
        const changes = collectChanges();
        if (changes.length === 0) {
          return Object.freeze({
            ok: true,
            submitted: Object.freeze([]),
            stillDirty: Object.freeze([])
          });
        }
        submitting = true;
        try {
          if (onSubmit) await onSubmit(Object.freeze(changes.slice()), session);
        } catch (error) {
          submitting = false;
          return submitRejection(
            "submit-rejected",
            `\u63D0\u4EA4\u901A\u9053\u62D2\u7EDD\u4E86\u672C\u6B21\u63D0\u4EA4\uFF0C\u810F\u503C\u5DF2\u4FDD\u7559\uFF1A${errorMessage(error)}`,
            { code: "fix-and-retry-submit", hint: "\u4FEE\u6B63 onSubmit \u7684\u5931\u8D25\u539F\u56E0\u540E\u91CD\u8BD5 submit()\uFF1B\u810F\u503C\u672A\u4E22\u5931\u3002" },
            error
          );
        }
        submitting = false;
        const stillDirtyKeys = [];
        for (const change of changes) {
          let row = committed.get(change.rowKey);
          if (!row) {
            row = /* @__PURE__ */ new Map();
            committed.set(change.rowKey, row);
          }
          row.set(change.colKey, snapshot(change.value));
          const live = pendingRow(change.rowKey)?.get(change.colKey);
          if (!live) continue;
          if (live.revision <= cutoff) {
            const pendingCols = pendingRow(change.rowKey);
            pendingCols?.delete(change.colKey);
            if (pendingCols && pendingCols.size === 0) pending.delete(change.rowKey);
          } else {
            live.previous = snapshot(change.value);
            live.hadPrevious = true;
            stillDirtyKeys.push(freezeKey(change.rowKey, change.colKey));
          }
        }
        notifyDirtyChange();
        return Object.freeze({
          ok: true,
          submitted: Object.freeze(changes.slice()),
          stillDirty: Object.freeze(stillDirtyKeys)
        });
      },
      async previewDiscard(keys) {
        const changes = selectChanges(keys);
        if (changes.length === 0) {
          return Object.freeze({
            ok: true,
            discarded: true,
            keys: Object.freeze([]),
            changes: Object.freeze([])
          });
        }
        const decision = await runGuard("preview-discard", changes);
        if (!decision.ok) return decision;
        return Object.freeze({
          ok: true,
          discarded: true,
          keys: Object.freeze(changes.map((c) => c.key)),
          changes: Object.freeze(changes.slice())
        });
      },
      async discard(keys) {
        const changes = selectChanges(keys);
        if (changes.length === 0) {
          return Object.freeze({
            ok: true,
            discarded: true,
            keys: Object.freeze([]),
            changes: Object.freeze([])
          });
        }
        const decision = await runGuard("discard", changes);
        if (!decision.ok) return decision;
        return applyDiscard(changes);
      }
    };
    return Object.freeze(session);
  }
  __name(createEditSession, "createEditSession");

  // src/runtime.ts
  PetUIExpand.reactive = reactive;
  PetUIExpand.ref = ref;
  PetUIExpand.computed = computed;
  PetUIExpand.effect = effect;
  PetUIExpand.compile = compile;
  PetUIExpand.h = h;
  PetUIExpand.patch = patch;
  PetUIExpand.createApp = createApp;
  PetUIExpand.defineComponent = defineComponent;
  PetUIExpand.createComponent = createComponent;
  PetUIExpand.mountComponent = mountComponent;
  PetUIExpand.MovableControl = MovableControl;
  PetUIExpand.SplitPane = SplitPane;
  PetUIExpand.DockLayout = DockLayout;
  PetUIExpand.createButton = createButton;
  PetUIExpand.createSwitch = createSwitch;
  PetUIExpand.createProgress = createProgress;
  PetUIExpand.createInput = createInput;
  PetUIExpand.createModal = createModal;
  PetUIExpand.createCheckbox = createCheckbox;
  PetUIExpand.createRadioGroup = createRadioGroup;
  PetUIExpand.createSelect = createSelect;
  PetUIExpand.createSlider = createSlider;
  PetUIExpand.createSpinner = createSpinner;
  PetUIExpand.createBadge = createBadge;
  PetUIExpand.createTag = createTag;
  PetUIExpand.createAlert = createAlert;
  PetUIExpand.createAvatar = createAvatar;
  PetUIExpand.createDivider = createDivider;
  PetUIExpand.createCode = createCode;
  PetUIExpand.createNavbar = createNavbar;
  PetUIExpand.createTabs = createTabs;
  PetUIExpand.createBreadcrumb = createBreadcrumb;
  PetUIExpand.createPagination = createPagination;
  PetUIExpand.createSteps = createSteps;
  PetUIExpand.createToast = createToast;
  PetUIExpand.createTooltip = createTooltip;
  PetUIExpand.createDropdown = createDropdown;
  PetUIExpand.createPopconfirm = createPopconfirm;
  PetUIExpand.createRate = createRate;
  PetUIExpand.createCard = createCard;
  PetUIExpand.createAccordion = createAccordion;
  PetUIExpand.createTimeline = createTimeline;
  PetUIExpand.createTable = createTable;
  PetUIExpand.createFrostedGlass = createFrostedGlass;
  PetUIExpand.createGrid = createGrid;
  PetUIExpand.createNineGridResizer = createNineGridResizer;
  PetUIExpand.createSplitPane = createSplitPane;
  PetUIExpand.createStatusPill = createStatusPill;
  PetUIExpand.createLogConsole = createLogConsole;
  PetUIExpand.createResultBox = createResultBox;
  PetUIExpand.createWriteReport = createWriteReport;
  PetUIExpand.createDataTable = createDataTable;
  PetUIExpand.createTreeView = createTreeView;
  PetUIExpand.createDescriptionList = createDescriptionList;
  PetUIExpand.createVirtualList = createVirtualList;
  PetUIExpand.createSelectableList = createSelectableList;
  PetUIExpand.createFilterableListbox = createFilterableListbox;
  PetUIExpand.createFormRow = createFormRow;
  PetUIExpand.createValidatedField = createValidatedField;
  PetUIExpand.createPropertyGrid = createPropertyGrid;
  PetUIExpand.createEditSession = createEditSession;
  PetUIExpand.createDangerousMode = createDangerousMode;
  PetUIExpand.createFilePicker = createFilePicker;
  PetUIExpand.createCanvasViewport = createCanvasViewport;
  PetUIExpand.createMeshViewport3D = createMeshViewport3D;
  PetUIExpand.createWebGLViewport = createWebGLViewport;
  PetUIExpand.createMiniChart = createMiniChart;
  PetUIExpand.createMediaTimeline = createMediaTimeline;
  PetUIExpand.createHexViewer = createHexViewer;
  PetUIExpand.createHexPatcher = createHexPatcher;
  PetUIExpand.createIcon = createIcon;
  PetUIExpand.injectSprite = injectSprite;
  PetUIExpand.setIcon = setIcon;
  PetUIExpand.createPanel = createPanel;
  PetUIExpand.createRow = createRow;
  PetUIExpand.createStack = createStack;
  PetUIExpand.themes = themes;
  themes.apply("default");
  PetUIExpand.createStateBox = createStateBox;
})();
