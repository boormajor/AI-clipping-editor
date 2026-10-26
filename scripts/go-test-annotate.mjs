#!/usr/bin/env node
// Reads `go test -json` from stdin, prints a readable log and emits GitHub
// Actions error annotations for failed tests so failures are visible in the
// checks UI and API without opening the raw log.
import {createInterface} from 'node:readline';

const output = new Map(); // test key -> lines
let failed = 0;
const rl = createInterface({input: process.stdin});
rl.on('line', (line) => {
  let ev;
  try {
    ev = JSON.parse(line);
  } catch {
    process.stdout.write(line + '\n');
    return;
  }
  const key = `${ev.Package || ''}/${ev.Test || ''}`;
  if (ev.Action === 'output') {
    process.stdout.write(ev.Output || '');
    if (ev.Test) {
      const arr = output.get(key) || [];
      if (arr.length < 40) arr.push((ev.Output || '').trimEnd());
      output.set(key, arr);
    }
  } else if (ev.Action === 'fail' && ev.Test) {
    failed++;
    const text = (output.get(key) || []).filter((l) => l && !l.startsWith('=== ')).join(' | ').replace(/[\r\n]+/g, ' ').slice(0, 900);
    console.log(`::error title=${ev.Test} (${ev.Package})::${text}`);
  }
});
rl.on('close', () => {
  if (failed) {
    console.log(`::error::${failed} test(s) failed`);
    process.exit(1);
  }
});
