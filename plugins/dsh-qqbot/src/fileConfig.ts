/**
 * 行配置为空时，从固定数据目录读取运行配置：
 *   ~/.dsh-neko/Neko/data/<name>.config.json
 *
 * 这样 bundle patch 只需声明插件加载，不再承载 appId/entity/owner 等运行参数。
 * 行配置非空时仍以行配置为准（便于测试与临时覆盖）。
 */
import { existsSync, readFileSync } from 'node:fs'
import { homedir } from 'node:os'
import { join } from 'node:path'

/** 固定运行配置路径。 */
export function fileConfigPath(name: string): string {
  return join(homedir(), '.dsh-neko', 'Neko', 'data', `${name}.config.json`)
}

/** 行配置非空则原样返回；否则读固定路径；文件不存在则返回空对象。 */
export function loadFileConfig<T extends object>(name: string, raw: T | undefined): T {
  if (raw && typeof raw === 'object' && Object.keys(raw).length > 0) return raw
  const path = fileConfigPath(name)
  if (!existsSync(path)) return (raw ?? {}) as T
  try {
    return JSON.parse(readFileSync(path, 'utf8')) as T
  } catch (error) {
    throw new Error(`im-qqbot: 配置文件不是合法 JSON: ${path}: ${(error as Error).message}`)
  }
}
