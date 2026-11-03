// verifyComponent.mjs —— 逐元件契约验收器：读 design 真源，机检 source / CSS / 事件 / 插槽 / 状态 / 实例结构。
// 用法: node verifyComponent.mjs <元件名>；退出码 0=全绿 1=违规 2=用法或 IO 错误。

import fs from "node:fs";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { createDocument } from "./fakeDom.mjs";

const HERE = path.dirname(fileURLToPath(import.meta.url));
const PROJECT_ROOT = path.resolve(HERE, "../../../../..");
const PLAN_DIR = path.join(PROJECT_ROOT, "design");
const PSEUDO_STATES = new Set(["hover"]);
const HEX_RE = /#[0-9a-fA-F]{3,8}\b/;
const RGB_RE = /\b(?:rgba?|hsla?)\s*\(([^()]*)\)/i;
const LEN_RE = /(?<![\w.])(-?(?:0|[1-9]\d*)(?:\.\d+)?)(px|rem|em)\b/;
const EASING_RE = /\bcubic-bezier\s*\(/i;
const VAR_RE = /var\(\s*--([A-Za-z][A-Za-z0-9-]*)\s*\)/g;
const ZERO_LENGTHS = new Set(["0", "-0", "0.0", "-0.0"]);

const findings = [];
let checks = 0;

/** 记录一项检查：通过计数，失败进违规表。 */
function check(passed, code, message) {
  checks += 1;
  if (passed) return true;
  findings.push(`${code}\t${message}`);
  return false;
}

/** 记录一条违规（不参与计数）。 */
function fail(code, message) {
  findings.push(`${code}\t${message}`);
}

const readJson = (file) => {
  try {
    return JSON.parse(fs.readFileSync(file, "utf8"));
  } catch (error) {
    console.error(`verifyComponent: 读取失败 ${file}: ${error.message}`);
    process.exit(2);
  }
};

const tokenToVar = (tokenName) => "--" + tokenName.split(".").join("-");
const varToToken = (varName) => varName.split("-").join(".");

const factoryNameFor = (componentName) =>
  "create" + componentName.split(".").map((part) => part.charAt(0).toUpperCase() + part.slice(1)).join("");

const contractEventNames = (events = {}) =>
  Object.keys(events).map((key) => key.charAt(2).toLowerCase() + key.slice(3));

function buildSampleProps(propsSpec = {}) {
  const props = {};
  for (const [name, def] of Object.entries(propsSpec)) {
    if (def.type === "enum") props[name] = def.default ?? def.values?.[0];
    else if (def.type === "boolean") props[name] = def.default ?? false;
    else if (def.type === "array") props[name] = def.default ?? [];
    else if (def.type === "number") props[name] = def.default ?? 0;
    else if (def.type === "string") props[name] = def.default ?? `sample-${name}`;
    else props[name] = def.default;
  }
  return props;
}

function buildComputedValues(tokensDoc) {
  const values = {};
  for (const [name, def] of Object.entries(tokensDoc.tokens ?? {})) {
    if (def.tier !== "semantic" || def.value === undefined) continue;
    values[tokenToVar(name)] = String(def.value);
  }
  return values;
}

function needsContentElement(compositionDoc, componentName) {
  const nodes = compositionDoc.nodes ?? {};
  for (const node of Object.values(nodes)) {
    if (node.component !== componentName) continue;
    for (const childId of node.children ?? []) {
      if (nodes[childId] && !nodes[childId].slot) return true;
    }
  }
  return false;
}

function checkCss(cssText, entry, tokenIndex) {
  const stripped = cssText.replace(/\/\*[\s\S]*?\*\//g, " ");
  for (const state of entry.states ?? []) {
    if (state === "default") continue;
    if (PSEUDO_STATES.has(state)) {
      check(stripped.includes(":hover"), "CSS-STATE", `缺少 ${state} 状态对应的 :hover 规则`);
      continue;
    }
    check(stripped.includes(`[data-state="${state}"]`), "CSS-STATE", `缺少 [data-state="${state}"] 规则`);
  }
  check(/\.[A-Za-z][A-Za-z0-9_-]*/.test(stripped), "CSS-CLASS", "CSS 没有任何类选择器");
  const usedTokens = new Set();
  stripped.split("\n").forEach((line, index) => {
    for (const match of line.matchAll(VAR_RE)) {
      const tokenName = varToToken(match[1]);
      const def = tokenIndex[tokenName];
      if (!def) {
        fail("CSS-VAR", `第 ${index + 1} 行 var(--${match[1]}) 不是已登记令牌（不得自造）`);
      } else if (def.tier !== "semantic") {
        fail("CSS-PRIMITIVE", `第 ${index + 1} 行 var(--${match[1]}) 指向 primitive 层（元件层禁止直连）`);
      } else {
        usedTokens.add(tokenName);
      }
    }
    if (HEX_RE.test(line)) fail("CSS-HARDCODED", `第 ${index + 1} 行出现硬编码颜色`);
    if (RGB_RE.test(line)) fail("CSS-HARDCODED", `第 ${index + 1} 行出现硬编码颜色函数`);
    const length = LEN_RE.exec(line);
    if (length && !ZERO_LENGTHS.has(length[1])) fail("CSS-HARDCODED", `第 ${index + 1} 行出现硬编码长度 ${length[0]}`);
    if (EASING_RE.test(line)) fail("CSS-HARDCODED", `第 ${index + 1} 行出现硬编码缓动曲线`);
  });
  for (const [prop, tokenName] of Object.entries(entry.style ?? {})) {
    check(usedTokens.has(tokenName), "CSS-TOKEN", `style.${prop} 的令牌 ${tokenName} 未在 CSS 中以 var(${tokenToVar(tokenName)}) 使用`);
  }
}

function checkJsStatic(jsText, entry, factoryName) {
  const head = jsText.split("\n").slice(0, 8).join("\n");
  check(/^\s*(\/\/|\*|\/\*)/m.test(head), "JS-HEADER", "缺文件头注释（模块头必填）");
  check(new RegExp(`export\\s+function\\s+${factoryName}\\s*\\(`).test(jsText), "JS-FACTORY",
    `缺导出工厂函数 ${factoryName}`);
  for (const name of contractEventNames(entry.events)) {
    check(new RegExp(`["']${name}["']`).test(jsText), "JS-EVENT", `契约事件 ${name} 未在源码中派发`);
  }
  for (const slotName of Object.keys(entry.slots ?? {})) {
    check(jsText.includes(`"${slotName}"`) || jsText.includes(`'${slotName}'`), "JS-SLOT",
      `契约插槽 ${slotName} 未在源码中声明`);
  }
}

async function checkInstance(componentName, entry, props, documentRef, sourcePath, cssText, needsContent) {
  let moduleRef;
  try {
    moduleRef = await import(pathToFileURL(sourcePath).href);
  } catch (error) {
    fail("JS-IMPORT", `ES module 导入失败: ${error.message}`);
    return {};
  }
  const factoryName = factoryNameFor(componentName);
  if (!check(typeof moduleRef[factoryName] === "function", "JS-FACTORY", `未导出 ${factoryName}`)) return {};

  let instance;
  try {
    instance = moduleRef[factoryName](props, { document: documentRef, window: documentRef.defaultView });
  } catch (error) {
    fail("INSTANCE", `实例化抛错: ${error.message}`);
    return {};
  }
  const root = instance?.element;
  if (!check(root?.nodeType === 1, "INSTANCE", "实例缺少 element 根节点")) return instance;

  const rootClass = String(root.className).split(" ").find((name) => name.startsWith("nek-")) ?? "";
  check(rootClass !== "", "INSTANCE", "根节点缺 nek- 前缀类名");
  check(rootClass !== "" && cssText.includes(`.${rootClass}`), "INSTANCE", `根类名 .${rootClass} 未在 CSS 中出现`);
  check((entry.states ?? []).includes(root.getAttribute("data-state")), "INSTANCE",
    `初始 data-state=${root.getAttribute("data-state")} 不在契约 states`);

  for (const slotName of Object.keys(entry.slots ?? {})) {
    const hasSlot = root.children.some((child) => child.getAttribute("data-slot") === slotName);
    check(hasSlot, "INSTANCE", `缺少 data-slot="${slotName}" 的直系插槽容器`);
    const fillSlot = instance.slots?.[slotName];
    check(typeof fillSlot === "function", "INSTANCE", `slots.${slotName} 不是填充函数`);
    if (typeof fillSlot === "function") {
      const probe = documentRef.createElement("div");
      try {
        fillSlot(probe);
        check(probe.parentNode !== null, "INSTANCE", `slots.${slotName}(node) 未把节点挂进插槽`);
      } catch (error) {
        fail("INSTANCE", `slots.${slotName}(node) 抛错: ${error.message}`);
      }
    }
  }
  if (needsContent) {
    const content = instance.content;
    check(content?.nodeType === 1, "INSTANCE", "组合树有非插槽子节点，但实例没有 content 容器");
    if (content?.nodeType === 1) {
      const probe = documentRef.createElement("div");
      content.append(probe);
      check(probe.parentNode === content, "INSTANCE", "content 容器无法挂载子元件");
    }
  }
  check(typeof instance.update === "function", "INSTANCE", "缺少 update(props)");
  check(typeof instance.destroy === "function", "INSTANCE", "缺少 destroy()");
  check(typeof instance.on === "function", "INSTANCE", "缺少 on(type, handler)");

  const eventName = contractEventNames(entry.events)[0];
  if (eventName && typeof instance.on === "function") {
    let calls = 0;
    instance.on(eventName, () => { calls += 1; });
    root.dispatchEvent(new documentRef.defaultView.CustomEvent(eventName, { detail: {} }));
    check(calls === 1, "INSTANCE", `on(${eventName}) 未收到事件`);
  }
  try {
    instance.update?.(props);
    check(true, "INSTANCE", "update 抛错");
  } catch (error) {
    check(false, "INSTANCE", `update(${JSON.stringify(props)}) 抛错: ${error.message}`);
  }
  try {
    instance.destroy?.();
    check(true, "INSTANCE", "destroy 抛错");
  } catch (error) {
    check(false, "INSTANCE", `destroy() 抛错: ${error.message}`);
  }
  return instance;
}

async function main(argv) {
  const componentName = argv[2];
  if (!componentName) {
    console.error("用法: node verifyComponent.mjs <元件名>");
    return 2;
  }
  const componentsDoc = readJson(path.join(PLAN_DIR, "components.json"));
  const tokensDoc = readJson(path.join(PLAN_DIR, "tokens.json"));
  const compositionDoc = readJson(path.join(PLAN_DIR, "composition.json"));
  const entry = componentsDoc.components?.[componentName];
  if (!entry) {
    console.error(`verifyComponent: components.json 未登记元件 ${componentName}`);
    return 2;
  }

  const sourcePath = path.resolve(PROJECT_ROOT, entry.source);
  const sourceDir = path.dirname(sourcePath);
  const baseName = path.basename(sourcePath, ".js");
  const cssPath = path.join(sourceDir, `${baseName}.css`);
  const verifyPath = path.join(sourceDir, "verify.sh");
  check(fs.existsSync(sourcePath), "FILES", `source 不存在: ${entry.source}`);
  check(fs.existsSync(cssPath), "FILES", `缺元件 CSS: ${baseName}.css`);
  check(fs.existsSync(verifyPath), "FILES", "缺 verify.sh");
  check(entry.verify === `bash internal/monitor/web/components/${path.basename(sourceDir)}/verify.sh`, "FILES",
    `契约 verify 路径与目录不一致: ${entry.verify}`);

  const jsText = fs.existsSync(sourcePath) ? fs.readFileSync(sourcePath, "utf8") : "";
  const cssText = fs.existsSync(cssPath) ? fs.readFileSync(cssPath, "utf8") : "";
  checkJsStatic(jsText, entry, factoryNameFor(componentName));
  checkCss(cssText, entry, tokensDoc.tokens ?? {});

  const documentRef = createDocument(buildComputedValues(tokensDoc));
  const props = buildSampleProps(entry.props);
  const needsContent = needsContentElement(compositionDoc, componentName);
  await checkInstance(componentName, entry, props, documentRef, sourcePath, cssText, needsContent);

  if (findings.length > 0) {
    console.log(`FAIL ${componentName}（${findings.length} 违规 / ${checks} 检查）`);
    for (const item of findings) console.log(`  ${item}`);
    return 1;
  }
  console.log(`OK ${componentName}: ${checks} 项检查通过（source / CSS / 事件 / 插槽 / 状态 / 实例）`);
  return 0;
}

process.exitCode = await main(process.argv);
