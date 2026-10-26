#!/usr/bin/env node
/**
 * Render smoke test: generates a 4-second synthetic source with FFmpeg,
 * renders it through the PodcastClip composition and checks the output exists
 * with the expected duration. Used by CI and `npm run smoke`.
 *
 * Optional env:
 *   REMOTION_BROWSER  path to a Chrome/Chromium binary (skips the download)
 */
import {execFileSync, spawnSync} from 'node:child_process';
import {existsSync, mkdirSync, rmSync, writeFileSync} from 'node:fs';
import {dirname, join, resolve} from 'node:path';
import {fileURLToPath} from 'node:url';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const jobDir = join(root, 'public', 'jobs', 'smoke');
const outDir = join(root, 'out');
const source = join(jobDir, 'source.mp4');
const props = join(jobDir, 'props.json');
const output = join(outDir, 'smoke.mp4');

function run(cmd, args, opts = {}) {
  const res = spawnSync(cmd, args, {stdio: 'inherit', shell: process.platform === 'win32', cwd: root, ...opts});
  if (res.status !== 0) {
    console.error(`\n[smoke] ${cmd} ${args.join(' ')} exited with ${res.status}`);
    process.exit(res.status ?? 1);
  }
}

mkdirSync(jobDir, {recursive: true});
mkdirSync(outDir, {recursive: true});

run('ffmpeg', ['-hide_banner', '-loglevel', 'error', '-y', '-f', 'lavfi', '-i', 'testsrc2=s=1280x720:r=30:d=4', '-f', 'lavfi', '-i', 'sine=frequency=440:duration=4', '-c:v', 'libx264', '-pix_fmt', 'yuv420p', '-c:a', 'aac', '-shortest', source]);

writeFileSync(
  props,
  JSON.stringify({
    media: 'jobs/smoke/source.mp4',
    duration: 4,
    width: 540,
    height: 960,
    fps: 30,
    layout: 'two-up',
    captions: true,
    captionPreset: 'kinetic',
    captionFont: 'Arial',
    graphics: true,
    callout: 'SMOKE TEST',
    progressBar: true,
    emphasis: true,
    words: [
      {start: 0.2, end: 0.5, word: 'The'},
      {start: 0.5, end: 0.9, word: 'biggest'},
      {start: 0.9, end: 1.3, word: 'SEO'},
      {start: 1.3, end: 1.8, word: 'mistake'},
      {start: 1.9, end: 2.3, word: 'is'},
      {start: 2.3, end: 2.9, word: 'ignoring'},
      {start: 2.9, end: 3.6, word: 'intent.'},
    ],
  }),
);

const args = ['remotion', 'render', 'src/index.ts', 'PodcastClip', output, '--props', props, '--codec', 'h264', '--log', 'warn'];
if (process.env.REMOTION_BROWSER) args.push('--browser-executable', process.env.REMOTION_BROWSER);
run(process.platform === 'win32' ? 'npx.cmd' : 'npx', args);

if (!existsSync(output)) {
  console.error('[smoke] no output file produced');
  process.exit(1);
}
const duration = Number(execFileSync('ffprobe', ['-v', 'error', '-show_entries', 'format=duration', '-of', 'csv=p=0', output]).toString().trim());
if (!(duration > 3.8 && duration < 4.3)) {
  console.error(`[smoke] unexpected duration ${duration}s`);
  process.exit(1);
}
console.log(`[smoke] OK: ${output} (${duration.toFixed(2)}s)`);
rmSync(jobDir, {recursive: true, force: true});
