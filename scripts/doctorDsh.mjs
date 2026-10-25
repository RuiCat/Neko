#!/usr/bin/env node
// dsh-mcp doctor: spawn bin/dsh-mcp, complete the MCP handshake, assert the
// frozen 17-tool list, the 0600 control socket, and SIGTERM cleanup. It uses a
// throwaway data directory and never touches data/ or a DSH profile.
import { spawn, execFileSync } from 'node:child_process';
import { existsSync, mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const PROTOCOL_VERSION = '2025-06-18';
const REQUEST_TIMEOUT_MS = 20000;
const EXIT_TIMEOUT_MS = 15000;
const SOCKET_WAIT_MS = 10000;

// K6 frozen tool set (alphabetical).
const FROZEN_TOOLS = [
  'diagnostics_query', 'entity_create', 'entity_query', 'entity_snapshot',
  'entity_state_get', 'entity_state_set', 'event_log', 'inventory',
  'neko_script_apply', 'neko_script_list', 'neko_script_read',
  'neko_script_remove', 'neko_script_validate', 'neko_script_write',
  'state_machine_resolve', 'state_machine_tick', 'stats',
];

const REPO_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const BINARY = join(REPO_ROOT, 'bin', 'dsh-mcp');

const TEMPLATE_ENTITIES = JSON.stringify({
  version: 1,
  entities: {
    猫猫: {
      id: '猫猫',
      type: 'NPC',
      data: { 状态: { 情绪: '平静', 身体: '健康' }, 身体: { 体力: 80 } },
      keys: {},
    },
  },
});

// fail prints one diagnostic line and terminates the doctor run.
function fail(reason) {
  console.error(`doctor: FAIL: ${reason}`);
  process.exit(1);
}

// writeMinimalData creates the smallest data tree bootstrap.New accepts.
function writeMinimalData(dataDir) {
  const templateDir = join(dataDir, 'templates');
  mkdirSync(templateDir, { recursive: true });
  writeFileSync(
    join(templateDir, 'NPC.json'),
    '{"状态":{"情绪":"平静","身体":"健康"},"身体":{"体力":80}}',
  );
  writeFileSync(join(dataDir, 'entities.json'), TEMPLATE_ENTITIES);
}

// delay waits before the next poll.
function delay(ms) {
  return new Promise((done) => setTimeout(done, ms));
}

// waitForSocket polls until the control socket path exists.
async function waitForSocket(socketPath) {
  const deadline = Date.now() + SOCKET_WAIT_MS;
  while (Date.now() < deadline) {
    if (existsSync(socketPath)) return;
    await delay(20);
  }
  fail(`control socket 未出现: ${socketPath}`);
}

// socketMode returns the octal permission string from stat -c %a.
function socketMode(socketPath) {
  return execFileSync('stat', ['-c', '%a', socketPath], { encoding: 'utf8' }).trim();
}

// startServer spawns one dsh-mcp with fresh temp directories.
function startServer(root, stderr) {
  const dataDir = join(root, 'data');
  const scriptsDir = join(root, 'scripts');
  mkdirSync(scriptsDir, { recursive: true });
  writeMinimalData(dataDir);
  const child = spawn(
    BINARY,
    ['-data-dir', dataDir, '-scripts-dir', scriptsDir, '-config', join(root, 'config.json')],
    { cwd: REPO_ROOT, stdio: ['pipe', 'pipe', 'pipe'] },
  );
  child.stderr.on('data', (chunk) => {
    stderr.text += chunk.toString();
    if (stderr.text.length > 8000) stderr.text = stderr.text.slice(-8000);
  });
  return { child, dataDir, scriptsDir, socketPath: join(dataDir, 'dsh-control.sock') };
}

// createClient attaches a newline-delimited JSON-RPC client to the child stdio.
function createClient(child, stderr) {
  const pending = new Map();
  let nextId = 0;
  let buffer = '';
  let closed = false;
  child.stdout.on('data', (chunk) => {
    buffer += chunk.toString();
    let newline = buffer.indexOf('\n');
    while (newline >= 0) {
      const line = buffer.slice(0, newline).trim();
      buffer = buffer.slice(newline + 1);
      if (line !== '') {
        let message;
        try {
          message = JSON.parse(line);
        } catch {
          fail(`stdout 不是合法 JSON-RPC 帧: ${line.slice(0, 200)}`);
        }
        const channel = pending.get(message.id);
        if (channel) {
          pending.delete(message.id);
          channel(message);
        }
      }
      newline = buffer.indexOf('\n');
    }
  });
  child.on('exit', () => {
    closed = true;
    for (const channel of pending.values()) channel(null);
    pending.clear();
  });

  //request sends one request frame and resolves to its response.
  function request(method, params) {
    nextId += 1;
    const id = nextId;
    const frame = { jsonrpc: '2.0', id, method };
    if (params !== undefined) frame.params = params;
    return new Promise((settle) => {
      const timer = setTimeout(() => {
        pending.delete(id);
        fail(`${method} 超时（stderr=${stderr.text}）`);
      }, REQUEST_TIMEOUT_MS);
      pending.set(id, (message) => {
        clearTimeout(timer);
        if (message === null) fail(`${method} 之前 stdout 关闭（stderr=${stderr.text}）`);
        settle(message);
      });
      child.stdin.write(`${JSON.stringify(frame)}\n`);
    });
  }

  //notify sends one notification frame (no id, no response).
  function notify(method, params) {
    const frame = { jsonrpc: '2.0', method };
    if (params !== undefined) frame.params = params;
    child.stdin.write(`${JSON.stringify(frame)}\n`);
  }

  return { request, notify, isClosed: () => closed };
}

// assertTools matches the advertised names against K6, case-insensitively.
function assertTools(result) {
  if (!result || !Array.isArray(result.tools)) {
    fail(`tools/list 缺少 tools 数组: ${JSON.stringify(result)}`);
  }
  const names = result.tools.map((tool) => String(tool.name).toLowerCase()).sort();
  const expected = FROZEN_TOOLS.map((name) => name.toLowerCase()).sort();
  if (names.length !== expected.length) {
    fail(`tools/list = ${names.length} 个, 期望 ${expected.length}: ${names.join(', ')}`);
  }
  for (let index = 0; index < expected.length; index += 1) {
    if (names[index] !== expected[index]) {
      fail(`tools/list 不匹配: 第 ${index} 项 ${names[index]}, 期望 ${expected[index]}`);
    }
  }
}

// waitExit waits for the child to exit after SIGTERM.
function waitExit(child) {
  return new Promise((settle) => {
    const timer = setTimeout(() => {
      child.kill('SIGKILL');
      fail(`SIGTERM 后 ${EXIT_TIMEOUT_MS}ms 未退出`);
    }, EXIT_TIMEOUT_MS);
    child.once('exit', (code, signal) => {
      clearTimeout(timer);
      settle({ code, signal });
    });
  });
}

// waitGone polls until the socket path disappears.
async function waitGone(socketPath) {
  const deadline = Date.now() + SOCKET_WAIT_MS;
  while (Date.now() < deadline) {
    if (!existsSync(socketPath)) return;
    await delay(20);
  }
  fail(`SIGTERM 后 socket 仍存在: ${socketPath}`);
}

// run executes all doctor checks.
async function run() {
  if (!existsSync(BINARY)) fail(`缺少二进制 ${BINARY}（先运行 make build）`);
  const root = mkdtempSync(join(tmpdir(), 'dsh-mcp-doctor-'));
  const stderr = { text: '' };
  let session;
  try {
    session = startServer(root, stderr);
    const { child, socketPath } = session;
    const client = createClient(child, stderr);
    await waitForSocket(socketPath);

    const initialize = await client.request('initialize', {
      protocolVersion: PROTOCOL_VERSION,
      capabilities: {},
      clientInfo: { name: 'doctor', version: '1.0.0' },
    });
    if (initialize.error) fail(`initialize 返回错误: ${JSON.stringify(initialize.error)}`);
    const initResult = initialize.result || {};
    if (initResult.protocolVersion !== PROTOCOL_VERSION) {
      fail(`protocolVersion = ${initResult.protocolVersion}, 期望 ${PROTOCOL_VERSION}`);
    }
    client.notify('notifications/initialized');

    const listed = await client.request('tools/list', {});
    if (listed.error) fail(`tools/list 返回错误: ${JSON.stringify(listed.error)}`);
    assertTools(listed.result);

    const mode = socketMode(socketPath);
    if (mode !== '600') fail(`socket 权限 = ${mode}, 期望 600`);

    child.kill('SIGTERM');
    const exit = await waitExit(child);
    if (exit.code !== 0) fail(`SIGTERM 后退出码 = ${exit.code} (signal=${exit.signal})`);
    await waitGone(socketPath);

    console.log(
      `OK: doctor 通过（17 tools, socket 0600, SIGTERM 清理；data=${session.dataDir}）`,
    );
  } finally {
    if (session && session.child.exitCode === null) session.child.kill('SIGKILL');
    rmSync(root, { recursive: true, force: true });
  }
}

run().catch((error) => fail(error && error.stack ? error.stack : String(error)));
