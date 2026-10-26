import {api} from '../api.js';
import {applyStatus, get, set, subscribe} from '../store.js';
import {$, busy, clear, fmtTime, h, notify, setError} from '../ui.js';
import {PROVIDER_LABELS} from '../options.js';
import {providerConfig, requireOnlineProvider} from './shared.js';
import {navigate} from '../router.js';
import {flushEditor} from './editor.js';

export function initClips() {
  subscribe(['project', 'selectedID'], renderList);
  subscribe('provider', ({provider}) => {
    $('providerPill').textContent = 'AI: ' + (PROVIDER_LABELS[provider.provider] || provider.provider);
    $('providerPill').className = 'pill ' + (provider.provider === 'heuristic' ? '' : 'ok');
  });
  subscribe('options', ({options}) => {
    if (!options) return;
    $('clipMin').min = $('clipMax').min = options.clipLimits.minSeconds;
    $('clipMin').max = $('clipMax').max = options.clipLimits.maxSeconds;
    $('clipCount').max = options.clipLimits.maxCount;
  });

  $('analyzeBtn').addEventListener('click', () => busy($('analyzeBtn'), analyze));
  $('reviewAllBtn').addEventListener('click', () => busy($('reviewAllBtn'), reviewAll));
  $('renderAllBtn').addEventListener('click', () => busy($('renderAllBtn'), renderAll));

  // Keyboard navigation of the clip list.
  $('clipList').addEventListener('keydown', (e) => {
    const clips = get().project.clips;
    if (!clips.length) return;
    const idx = clips.findIndex((c) => c.id === get().selectedID);
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault();
      const next = clips[Math.max(0, Math.min(clips.length - 1, idx + (e.key === 'ArrowDown' ? 1 : -1)))];
      selectClip(next.id);
    } else if (e.key === 'Home') {
      selectClip(clips[0].id);
    } else if (e.key === 'End') {
      selectClip(clips[clips.length - 1].id);
    }
  });
}

export async function selectClip(id) {
  if (id === get().selectedID) return;
  await flushEditor();
  set({selectedID: id});
  const el = document.querySelector(`.clip[data-id="${CSS.escape(id)}"]`);
  el?.scrollIntoView({block: 'nearest'});
}

function renderList({project, selectedID}) {
  const list = $('clipList');
  const clips = project.clips || [];
  $('clipBadge').textContent = clips.length;
  $('clipBadge').classList.toggle('hide', !clips.length);
  clear(list);
  if (!project.videoPath) {
    list.append(h('div', {class: 'empty'}, h('h3', {}, 'No episode yet'), h('p', {}, 'Import a recording to get started.'), h('button', {class: 'btn sm', type: 'button', style: {marginTop: '12px'}, onclick: () => navigate('import')}, 'Go to Import')));
    return;
  }
  if (!clips.length) {
    list.append(h('div', {class: 'empty'}, h('h3', {}, 'Your best moments will appear here'), h('p', {}, `${project.cues.length} transcript cues ready. Click “Find best moments”.`)));
    return;
  }
  clips.forEach((c, i) => {
    const flag = reviewFlag(c);
    const card = h('div', {
      class: 'clip',
      role: 'option',
      'aria-selected': String(c.id === selectedID),
      tabindex: c.id === selectedID ? '0' : '-1',
      dataset: {id: c.id},
      onclick: () => selectClip(c.id),
      onkeydown: (e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          selectClip(c.id);
        }
      },
    },
    h('span', {class: 'idx'}, String(i + 1).padStart(2, '0')),
    h('div', {class: 'grow'}, h('div', {class: 'title', title: c.title}, c.title || 'Untitled clip'), h('div', {class: 'meta'}, `${fmtTime(c.start)} – ${fmtTime(c.end)} · ${Math.round(c.end - c.start)}s${c.words?.length ? ' · word-timed' : ''}`)),
    h('span', {class: 'score', title: 'Heuristic score'}, `${Math.round(c.score)}`),
    flag ? h('span', {class: 'flag ' + flag.kind}, flag.text) : null,
    );
    list.append(card);
  });
}

function reviewFlag(c) {
  if (c.selected === false) return {kind: 'off', text: 'Excluded from “Render all”'};
  if (c.reviewed) return c.reviewOK ? {kind: 'ok', text: 'AI context check passed'} : {kind: 'warn', text: c.reviewSummary ? 'Needs attention: ' + c.reviewSummary : 'Needs attention'};
  return {kind: '', text: c.reason || ''};
}

function validateFind() {
  const count = Number($('clipCount').value);
  const min = Number($('clipMin').value);
  const max = Number($('clipMax').value);
  const limits = get().options?.clipLimits || {minSeconds: 10, maxSeconds: 240, maxCount: 30};
  let msg = '';
  if (!(count >= 1 && count <= limits.maxCount)) msg = `Clip count must be between 1 and ${limits.maxCount}.`;
  else if (!(min >= limits.minSeconds && max <= limits.maxSeconds)) msg = `Clip length must be between ${limits.minSeconds} and ${limits.maxSeconds} seconds.`;
  else if (max < min) msg = 'Maximum length must be at least the minimum.';
  setError(null, $('findErr'), msg);
  return msg ? null : {count, min, max};
}

async function analyze() {
  if (!get().project.videoPath) {
    notify.info('Import an episode first.');
    navigate('import');
    return;
  }
  const params = validateFind();
  if (!params) return;
  await flushEditor();
  const provider = providerConfig();
  const res = await api('analyze', {...params, provider});
  set({selectedID: ''});
  applyStatus(res);
  const n = res.project.clips.length;
  notify.ok(`Found ${n} candidate clip${n === 1 ? '' : 's'}.`);
  if (provider.provider !== 'heuristic') {
    $('reviewStatus').textContent = 'Tip: “Review all with AI” checks each clip stands alone for a new viewer.';
  } else {
    $('reviewStatus').textContent = 'Ranked by the local heuristic. Connect an AI provider for editorial selection.';
  }
}

async function reviewAll() {
  const clips = get().project.clips;
  if (!clips.length) {
    notify.info('Find clips first.');
    return;
  }
  if (!requireOnlineProvider()) return;
  await flushEditor();
  const label = $('reviewStatus');
  let passed = 0;
  for (let i = 0; i < clips.length; i++) {
    label.textContent = `AI editorial review ${i + 1} / ${clips.length} · ${clips[i].title}`;
    try {
      const r = await api('clip/review', {id: clips[i].id, provider: providerConfig()});
      if (r.contextOK) passed++;
    } catch (err) {
      notify.err(`${clips[i].title}: ${err.message}`);
    }
    applyStatus(await api('status'));
  }
  label.textContent = `Review complete: ${passed}/${clips.length} clips passed the context check. Captions and framing are not verified by this step.`;
}

async function renderAll() {
  const list = get().project.clips.filter((c) => c.selected !== false);
  if (!list.length) {
    notify.info('No clips to render.');
    return;
  }
  await flushEditor();
  let queued = 0;
  for (const c of list) {
    try {
      await api('render', {id: c.id});
      queued++;
    } catch (err) {
      notify.err(`${c.title}: ${err.message}`);
    }
  }
  notify.ok(`Queued ${queued} render${queued === 1 ? '' : 's'}. Renders run one at a time.`);
  set({jobsDirty: Date.now()});
  navigate('queue');
}
