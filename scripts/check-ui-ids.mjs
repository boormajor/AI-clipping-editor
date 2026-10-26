#!/usr/bin/env node
// Verifies that every element id the front end looks up with $('…') exists in
// index.html, and that every JavaScript module parses. Catches the class of
// bug where a renamed element silently breaks a whole view.
import {readFileSync, readdirSync, statSync} from 'node:fs';
import {join, resolve} from 'node:path';
import {execFileSync} from 'node:child_process';

const root = resolve(new URL('..', import.meta.url).pathname, 'internal', 'ui');
const html = readFileSync(join(root, 'index.html'), 'utf8');
const ids = new Set([...html.matchAll(/\sid="([^"]+)"/g)].map((m) => m[1]));

function walk(dir) {
  return readdirSync(dir).flatMap((name) => {
    const p = join(dir, name);
    return statSync(p).isDirectory() ? walk(p) : p.endsWith('.js') ? [p] : [];
  });
}

let failures = 0;
for (const file of walk(join(root, 'js'))) {
  try {
    execFileSync(process.execPath, ['--check', file], {stdio: 'pipe'});
  } catch (err) {
    failures++;
    console.error(`syntax error in ${file}:\n${err.stderr}`);
  }
  const src = readFileSync(file, 'utf8');
  for (const m of src.matchAll(/\$\('([A-Za-z0-9_-]+)'\)/g)) {
    if (!ids.has(m[1])) {
      failures++;
      console.error(`${file}: references missing element #${m[1]}`);
    }
  }
}
if (failures) {
  console.error(`\n${failures} problem(s) found`);
  process.exit(1);
}
console.log(`UI check OK: ${ids.size} ids, all references resolve`);
