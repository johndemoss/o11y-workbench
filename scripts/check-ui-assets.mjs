#!/usr/bin/env node
// Validate the HTML entry point before embedding/distributing a Vite build.
import { readFileSync, statSync } from 'node:fs';
import { resolve, relative, isAbsolute } from 'node:path';
import { pathToFileURL } from 'node:url';

export function checkUIAssets(directory, prefix) {
  if (!prefix.startsWith('/') || !prefix.endsWith('/')) {
    throw new Error('UI prefix must start and end with /');
  }
  const root = resolve(directory);
  const html = readFileSync(resolve(root, 'index.html'), 'utf8');
  const refs = [...html.matchAll(/<(?:script|link)\b[^>]*\b(?:src|href)=["']([^"']+)["']/gi)]
    .map(match => match[1]);
  if (!refs.some(ref => ref.startsWith(prefix) && /\.js(?:[?#]|$)/.test(ref))) {
    throw new Error(`${directory}: no built JavaScript entry point for ${prefix}`);
  }
  for (const ref of refs) {
    // Published entry points must resolve entirely within this UI's bundle.
    if (!ref.startsWith(prefix) || ref.startsWith('//')) {
      throw new Error(`${directory}: asset outside ${prefix}: ${ref}`);
    }
    const path = decodeURIComponent(ref.split(/[?#]/, 1)[0].slice(prefix.length));
    const target = resolve(root, path);
    const rel = relative(root, target);
    if (rel === '..' || rel.startsWith('../') || isAbsolute(rel) || !statSync(target).isFile()) {
      throw new Error(`${directory}: invalid asset ${ref}`);
    }
  }
  return refs.length;
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  try {
    const [, , directory, prefix] = process.argv;
    if (!directory || !prefix) throw new Error('Usage: node scripts/check-ui-assets.mjs <dist> </prefix/>');
    console.log(`${directory}: ${checkUIAssets(directory, prefix)} entry-point assets verified`);
  } catch (err) {
    console.error(err.message);
    process.exitCode = 1;
  }
}
