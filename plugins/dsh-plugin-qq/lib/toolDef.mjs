/**
 * 零依赖工具定义助手：手工构造与宿主 `defineTool` 同形的工具对象。
 *
 * 为什么不 import `@deepseek-ai/dsh-tools`：本插件的定位是「装到任意 profile 都能跑、
 * 不带自己的依赖树」。宿主对工具定义的硬要求只有三件事——`name`、`parameters`（受支持
 * JSON Schema 子集）、`output: { schema, render }`：见 dsh-tools 的 `ToolRegistry.register`。
 * 这里按同一形状手工产出，并用工具自测离线复验（见 lib/test/testDiary.mjs）。
 */

/** 所有工具共用的输出声明：模型看 render 文本，value 只承载 ok/text。 */
export const TOOL_OUTPUT_SCHEMA = {
  type: "object",
  properties: {
    ok: { type: "boolean" },
    text: { type: "string" },
  },
  required: ["ok", "text"],
  additionalProperties: false,
};

/**
 * 造一个工具定义。
 * @param {{name:string, description:string, parameters:object,
 *   run:(args:object, exec:object)=>Promise<string>|string}} spec 工具规格
 * @returns {object} 可传给 `ctx.tools.register` 的定义
 */
export function makeTool(spec) {
  return {
    name: spec.name,
    description: spec.description,
    parameters: spec.parameters,
    output: {
      schema: TOOL_OUTPUT_SCHEMA,
      /** 把规范化结果投影成模型可见文本。 */
      render(_args, value) {
        const text = typeof value?.text === "string" ? value.text : "";
        return [{ type: "text", text: value?.ok === false ? `失败：${text}` : text }];
      },
    },
    /**
     * 执行一次调用；失败不抛，返回 ok=false 让模型看到原因并自行决定下一步。
     * @param {object} args 模型给的结构化参数
     * @param {object} exec 宿主执行上下文（exec.agent / exec.signal）
     */
    async execute(args, exec) {
      try {
        const text = await spec.run(args || {}, exec || {});
        return { ok: true, text: String(text ?? "") };
      } catch (error) {
        return { ok: false, text: error instanceof Error ? error.message : String(error) };
      }
    },
  };
}
