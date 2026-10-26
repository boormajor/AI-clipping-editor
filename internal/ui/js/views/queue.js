import {api} from '../api.js';
import {applyStatus, get, set, subscribe} from '../store.js';
import {$, busy, clear, fileName, h, notify} from '../ui.js';

// The queue polls while any job is active, on every page, so finished
// renders and imports are noticed wherever the user is.

let timer = null;
let failures = 0;
const announced = new Set();

export function initQueue() {
  subscribe('jobs', ({jobs, route}) => {
    const active = jobs.filter((j) => j.status === 'queued' || j.status === 'running').length;
    $('queueBadge').textContent = active;
    $('queueBadge').classList.toggle('hide', !active);
    if (route === 'queue') renderJobs(jobs);
    announce(jobs);
    schedule(active > 0);
  });
  subscribe('route', ({route, jobs}) => {
    if (route === 'queue') renderJobs(jobs);
  });
  subscribe('jobsDirty', () => refreshJobs());
  $('refreshJobsBtn').addEventListener('click', () => busy($('refreshJobsBtn'), refreshJobs));
  $('videoDialogClose').addEventListener('click', () => {
    $('videoDialogPlayer').pause();
    $('videoDialog').close();
  });
  refreshJobs();
}

function schedule(active) {
  clearTimeout(timer);
  const delay = active ? 1500 : 8000;
  timer = setTimeout(refreshJobs, Math.min(60000, delay * (failures + 1)));
}

export async function refreshJobs() {
  try {
    const jobs = await api('jobs');
    failures = 0;
    set({jobs});
  } catch {
    failures = Math.min(failures + 1, 8);
    schedule(false);
  }
}

function announce(jobs) {
  for (const j of jobs) {
    if (j.status === 'queued' || j.status === 'running' || announced.has(j.id)) continue;
    announced.add(j.id);
    if (!j.finished) continue;
    const age = Date.now() - new Date(j.finished).getTime();
    if (age > 60000) continue; // old jobs loaded from disk
    if (j.kind === 'import' && j.status === 'complete' && get().route !== 'import') {
      api('status').then(applyStatus).catch(() => {});
    }
    if (j.kind === 'render') {
      if (j.status === 'complete') {
        notify.ok(`Rendered: ${fileName(j.output)}`);
        api('status').then(applyStatus).catch(() => {});
      } else if (j.status === 'failed') notify.err(`Render failed: ${j.error}`);
    }
  }
}

function renderJobs(jobs) {
  const root = clear($('jobList'));
  if (!jobs.length) {
    root.append(h('div', {class: 'empty'}, 'No jobs yet. Renders and imports appear here.'));
    return;
  }
  const titles = new Map(get().project.clips.map((c) => [c.id, c.title]));
  for (const j of jobs) {
    const active = j.status === 'queued' || j.status === 'running';
    const title = j.kind === 'import' ? `Import · ${j.title || ''}` : titles.get(j.clipId) || j.title || j.clipId;
    const sub = active ? `${j.stage || j.status}${j.progress ? ` · ${j.progress}%` : ''}` : j.status === 'complete' ? j.output || 'Done' : j.error || j.status;
    const left = h('div', {class: 'grow'}, h('div', {class: 'title', title}, title), h('div', {class: 'sub', title: sub}, sub));
    if (active) left.append(h('div', {class: 'progress' + (j.progress ? '' : ' indeterminate')}, h('i', {style: {width: `${j.progress || 0}%`}})));
    const right = h('div', {class: 'right'});
    if (j.renderer === 'ffmpeg-fallback') right.append(h('span', {class: 'tag', title: 'Remotion was unavailable; the FFmpeg fallback rendered this clip'}, 'FFmpeg'));
    right.append(h('span', {class: 'tag ' + j.status}, j.status));
    if (active) {
      const cancel = h('button', {class: 'btn sm danger', type: 'button'}, 'Cancel');
      cancel.addEventListener('click', () => busy(cancel, async () => {
        await api('jobs/cancel', {id: j.id});
        await refreshJobs();
      }));
      right.append(cancel);
    }
    if (j.status === 'complete' && j.kind === 'render') {
      right.append(h('button', {class: 'btn sm', type: 'button', onclick: () => openVideo(j)}, 'Play'));
      const reveal = h('button', {class: 'btn sm', type: 'button'}, 'Show in folder');
      reveal.addEventListener('click', () => busy(reveal, () => api('reveal', {jobId: j.id})));
      right.append(reveal);
    }
    const row = h('div', {class: 'job'}, left, right);
    if (j.log && (j.status === 'failed' || j.renderer === 'ffmpeg-fallback')) {
      const details = h('details', {style: {gridColumn: '1 / -1'}}, h('summary', {class: 'small muted', style: {cursor: 'pointer'}}, 'Show log'), h('pre', {class: 'log'}, j.log));
      row.append(details);
    }
    root.append(row);
  }
}

function openVideo(j) {
  $('videoDialogTitle').textContent = fileName(j.output);
  const v = $('videoDialogPlayer');
  v.src = '/api/media?id=' + encodeURIComponent(j.id);
  $('videoDialog').showModal();
  v.play().catch(() => {});
}
