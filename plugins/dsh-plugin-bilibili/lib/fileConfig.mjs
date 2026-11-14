// 行配置为空时从 ~/.dsh-neko/Neko/data/<name>.config.json 读取运行配置。
import { existsSync, readFileSync } from 'node:fs'
import { homedir } from 'node:os'
import { join } from 'node:path'

export function fileConfigPath(name) {
  return join(homedir(), '.dsh-neko', 'Neko', 'data', `${name}.config.json`)
}

export function loadFileConfig(name, raw) {
  if (raw && typeof raw === 'object' && Object.keys(raw).length > 0) return raw
  const path = fileConfigPath(name)
  if (!existsSync(path)) return raw ?? {}
  try {
    return JSON.parse(readFileSync(path, 'utf8'))
  } catch (error) {
    throw new Error(`bilibili: 配置文件不是合法 JSON: ${path}: ${error.message}`)
  }
}
