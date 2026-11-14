import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, writeFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { test } from 'node:test';
import { checkUIAssets } from './check-ui-assets.mjs';

test('entry point must reference existing built JS, CSS and icon files', t => {
  const dir = mkdtempSync(join(tmpdir(), 'o11y-ui-assets-'));
  t.after(() => rmSync(dir, { recursive: true, force: true }));
  mkdirSync(join(dir, 'assets'));
  writeFileSync(join(dir, 'index.html'), '<link href="/pipeline/favicon.svg"><link href="/pipeline/assets/app.css"><script src="/pipeline/assets/app.js"></script>');
  writeFileSync(join(dir, 'favicon.svg'), '<svg/>');
  writeFileSync(join(dir, 'assets/app.css'), 'body {}');
  assert.throws(() => checkUIAssets(dir, '/pipeline/'), /ENOENT/);
  writeFileSync(join(dir, 'assets/app.js'), 'export {};');
  assert.equal(checkUIAssets(dir, '/pipeline/'), 3);
  rmSync(join(dir, 'assets/app.css'));
  assert.throws(() => checkUIAssets(dir, '/pipeline/'), /ENOENT/);
});

test('placeholder, wrong-base and external builds cannot pass as a complete UI', t => {
  const dir = mkdtempSync(join(tmpdir(), 'o11y-ui-assets-'));
  t.after(() => rmSync(dir, { recursive: true, force: true }));
  mkdirSync(join(dir, 'assets'));
  writeFileSync(join(dir, 'assets/app.js'), 'export {};');
  for (const html of [
    '<p>Build the frontend</p>',
    '<script src="/other/assets/app.js"></script>',
    '<script src="https://example.invalid/app.js"></script>',
    '<script src="/pipeline/assets/app.js"></script><link href="/pipeline/%2e%2e/outside.css">',
  ]) {
    writeFileSync(join(dir, 'index.html'), html);
    assert.throws(() => checkUIAssets(dir, '/pipeline/'));
  }
});
