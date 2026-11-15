import assert from 'node:assert/strict';
import { execFileSync, spawn, spawnSync } from 'node:child_process';
import { existsSync, mkdtempSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { test } from 'node:test';
import { once } from 'node:events';
import { fileURLToPath } from 'node:url';

const repoRoot = fileURLToPath(new URL('..', import.meta.url));

test('runtime_write_pid_record serializes every argv element', () => {
  const dir = mkdtempSync(join(tmpdir(), 'o11y-pid-'));
  const pidfile = join(dir, 'server.pid');
  const argv = ['/abs/bin', '--addr', '127.0.0.1:18080', 'svc\nname', 'tab\there', 'quote"slash\\', 'ünïcode', ''];
  try {
    execFileSync('bash', ['-c', 'source scripts/runtime.sh; runtime_write_pid_record "$@"', 'bash', pidfile, '4321', '987', ...argv], {
      cwd: repoRoot,
    });
    const record = JSON.parse(readFileSync(pidfile, 'utf8'));
    assert.equal(record.pid, 4321);
    assert.equal(record.start_ticks, 987);
    assert.deepEqual(record.argv, argv);
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});

test('runtime_write_pid_record accepts null start ticks', () => {
  const dir = mkdtempSync(join(tmpdir(), 'o11y-pid-'));
  const pidfile = join(dir, 'server.pid');
  try {
    execFileSync('bash', ['-c', 'source scripts/runtime.sh; runtime_write_pid_record "$@"', 'bash', pidfile, '1', 'null', '/bin/x'], {
      cwd: repoRoot,
    });
    const record = JSON.parse(readFileSync(pidfile, 'utf8'));
    assert.equal(record.start_ticks, null);
    assert.deepEqual(record.argv, ['/bin/x']);
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});

test('runtime_record_pid verifies exact Linux argv', async (t) => {
  if (process.platform !== 'linux' || !existsSync('/proc/self/cmdline') ||
      spawnSync('python3', ['-c', 'pass']).status !== 0) {
    t.skip('Linux /proc and python3 are required');
    return;
  }
  const dir = mkdtempSync(join(tmpdir(), 'o11y-pid-'));
  const pidfile = join(dir, 'server.pid');
  const wrongPidfile = join(dir, 'wrong.pid');
  const argv = ['python3', '-c', 'import time; time.sleep(60)', 'a\nb', 'tab\there', 'quote"', 'back\\slash', 'ünïcode', ''];
  const child = spawn(argv[0], argv.slice(1), { stdio: 'ignore' });
  const exited = once(child, 'exit');
  try {
    const record = (file, args) => execFileSync('bash', [
      '-c', 'source scripts/runtime.sh; runtime_record_pid "$@"',
      'bash', file, String(child.pid), ...args,
    ], { cwd: repoRoot, stdio: 'pipe' });
    record(pidfile, argv);
    const actual = readFileSync(`/proc/${child.pid}/cmdline`).toString('utf8').split('\0');
    assert.equal(actual.pop(), '');
    const saved = JSON.parse(readFileSync(pidfile, 'utf8'));
    assert.equal(saved.pid, child.pid);
    assert.ok(Number.isInteger(saved.start_ticks));
    assert.deepEqual(saved.argv, actual);
    assert.deepEqual(actual, argv);

    const wrong = [...argv.slice(0, 3), 'a', 'b', ...argv.slice(4)];
    assert.throws(() => record(wrongPidfile, wrong));
    assert.equal(existsSync(wrongPidfile), false);
    assert.throws(() => record(wrongPidfile, argv.slice(0, -1)));
    assert.equal(existsSync(wrongPidfile), false);
  } finally {
    child.kill('SIGTERM');
    await exited;
    rmSync(dir, { recursive: true, force: true });
  }
});
