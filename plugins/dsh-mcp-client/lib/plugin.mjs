/**
 * dsh-mcp-client：对 @deepseek-ai/dsh-mcp-client 的薄包装。
 * 目的只有一个——bundle patch 不写 config；启动参数按固定运行目录解析。
 *
 * 配置优先级：
 *   1. 行配置（测试/临时覆盖）
 *   2. ~/.dsh-neko/Neko/data/mcp-astrbox.json
 *   3. 默认值（~/.dsh-neko/Neko/bin/dsh-mcp）
 */
import { existsSync, readFileSync } from 'node:fs'
import { homedir } from 'node:os'
import { join } from 'node:path'
import * as upstream from '@deepseek-ai/dsh-mcp-client'

export const name = upstream.name
export const inject = upstream.inject
export const Config = upstream.Config

export const NEKO_HOME = join(homedir(), '.dsh-neko', 'Neko')
export const CONFIG_PATH = join(NEKO_HOME, 'data', 'mcp-astrbox.json')

function defaultConfig() {
  return {
    serverName: 'astrbox',
    transport: 'stdio',
    command: join(NEKO_HOME, 'bin', 'dsh-mcp'),
    args: [
      '-data-dir', join(NEKO_HOME, 'data'),
      '-scripts-dir', join(NEKO_HOME, 'scripts'),
      '-config', join(NEKO_HOME, 'config.json'),
    ],
    cwd: NEKO_HOME,
    toolCallTimeoutMs: 600000,
  }
}

export function resolveConfig(raw) {
  if (raw && typeof raw === 'object' && Object.keys(raw).length > 0) return raw
  if (existsSync(CONFIG_PATH)) {
    try {
      return JSON.parse(readFileSync(CONFIG_PATH, 'utf8'))
    } catch (error) {
      throw new Error(`dsh-mcp-client: 配置不是合法 JSON: ${CONFIG_PATH}: ${error.message}`)
    }
  }
  return defaultConfig()
}

export async function apply(ctx, rawConfig) {
  return upstream.apply(ctx, resolveConfig(rawConfig))
}
