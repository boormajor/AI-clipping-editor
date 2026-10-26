import {api} from '../api.js';
import {applyStatus, get, replaceClip, selectedClip, set, subscribe} from '../store.js';
import {$, busy, clamp, clear, confirm, debounce, fileName, fillSelect, fmtTime, h, notify, round2, setError} from '../ui.js';
import {LAYOUT_LABELS, PRESET_LABELS, WORD_TIMED} from '../options.js';
import {pickPath, providerConfig, requireOnlineProvider} from './shared.js';
import {preview} from './preview.js';
import {navigate} from '../router.js';

// Draft of the clip being edited. Fields the user touches are collected into
// `pending` and sent as one patch; the server is the source of truth for
// everything else (score, words, shot plan).
let draft = null;
let pending = {};
let saving = null;

export function initEditor() {
  subscribe('options', ({options}) => {
    if (!options) return;
    fillSelect($('editLayout'), options.layouts, (v) => LAYOUT_LABELS[v] || v);
    fillSelect($('editCaptionStyle'), options.captionPresets.map((v) => ({value: v, label: PRESET_LABELS[v] || v})));
  });
  subscribe(['selectedID', 'project'], ({selectedID, project}) => {
    const clip = project.clips.find((c) => c.id === selectedID);
    if (!clip) {
      draft = null;
      pending = {};
      showEmpty();
      return;
    }
    if (!draft || draft.id !== clip.id) {
      load(clip);
    } else {
      // Server-side changes (AI review, re-derived words) reach the draft while
      // keeping the user's unsent edits.
      draft = {...clip, ...pendingValues()};
      renderMeta();
      drawTimeline();
    }
  });

  const markField = (id, key, read) => $(id).addEventListener('input', () => {
    if (!draft) return;
    const v = read ? read() : $(id).value;
    draft[key] = v;
    pending[key] = v;
    markDirty();
    if (key === 'start' || key === 'end') {
      if (!validateBounds()) return;
      drawTimeline();
      preview.seek(key === 'start' ? draft.start : Math.max(draft.start, draft.end - 1.5));
    }
    if (key === 'pan') {
      $('panLabel').textContent = `${Math.round(v * 100)}%`;
      preview.update(draft);
    }
    if (key === 'captions' || key === 'captionStyle') preview.update(draft);
    queueSave();
  });
  markField('editTitle', 'title');
  markField('editStart', 'start', () => Number($('editStart').value));
  markField('editEnd', 'end', () => Number($('editEnd').value));
  markField('editPan', 'pan', () => Number($('editPan').value) / 100);
  markField('editLayout', 'layout');
  markField('editCaptions', 'captions', () => $('editCaptions').checked);
  markField('editCaptionStyle', 'captionStyle');
  markField('editCallout', 'callout');
  $('editLayout').addEventListener('change', () => queueSave());

  initHandles();
  $('renderBtn').addEventListener('click', () => busy($('renderBtn'), renderSelected));
  $('aiReviewBtn').addEventListener('click', () => busy($('aiReviewBtn'), aiReview));
  $('cleanBtn').addEventListener('click', () => busy($('cleanBtn'), cleanDeadAir));
  $('transcriptBtn').addEventListener('click', toggleTranscriptEditor);
  $('metaBtn').addEventListener('click', () => busy($('metaBtn'), generateMeta));
  $('addBrollBtn').addEventListener('click', () => busy($('addBrollBtn'), addBroll));
  $('stockSearchBtn').addEventListener('click', () => busy($('stockSearchBtn'), searchStock));
  $('stockQuery').addEventListener('keydown', (e) => {
    if (e.key === 'Enter') $('stockSearchBtn').click();
  });

  window.addEventListener('beforeunload', (e) => {
    if (Object.keys(pending).length) {
      e.preventDefault();
      e.returnValue = '';
    }
  });
}

function pendingValues() {
  const out = {};
  for (const k of Object.keys(pending)) out[k] = pending[k];
  return out;
}

function showEmpty() {
  $('editorEmpty').classList.remove('hide');
  $('editorForm').classList.add('hide');
  $('renderBtn').disabled = true;
  preview.update(null);
}

function load(clip) {
  draft = {...clip};
  pending = {};
  $('editorEmpty').classList.add('hide');
  $('editorForm').classList.remove('hide');
  $('renderBtn').disabled = false;
  $('editTitle').value = clip.title || '';
  $('editStart').value = round2(clip.start);
  $('editEnd').value = round2(clip.end);
  $('editPan').value = Math.round((clip.pan ?? 0.5) * 100);
  $('panLabel').textContent = `${Math.round((clip.pan ?? 0.5) * 100)}%`;
  $('editLayout').value = clip.layout || 'focus';
  $('editCaptions').checked = !!clip.captions;
  $('editCaptionStyle').value = clip.captionStyle || get().project.config.captionStyle || 'kinetic';
  $('editCallout').value = clip.callout || '';
  $('transcriptEditor').classList.add('hide');
  clear($('transcriptEditor'));
  $('metaOutput').classList.add('hide');
  clear($('metaOutput'));
  clear($('stockResults'));
  setError($('editEnd'), $('boundsErr'), '');
  $('dirtyFlag').classList.add('hide');
  renderMeta();
  drawTimeline();
  preview.update(draft);
  preview.seek(clip.start);
}

function renderMeta() {
  if (!draft) return;
  $('editorTitle').textContent = draft.id.replace('clip-', 'Clip ');
  const notes = [];
  if (draft.words?.length) notes.push(`${draft.words.length} word timings`);
  else if (WORD_TIMED.has(draft.captionStyle)) notes.push('No word timings: this template will show whole cues');
  if (draft.shots?.length) notes.push(`AI shot plan: ${draft.shots.length} shots`);
  if (draft.reviewed) notes.push(draft.reviewOK ? 'Context check passed' : 'Context check flagged issues');
  if (draft.reviewSummary) notes.push(draft.reviewSummary);
  $('editorNote').textContent = notes.join(' · ');
  renderBroll();
}

function markDirty() {
  $('dirtyFlag').classList.remove('hide');
}

function validateBounds() {
  if (!draft) return true;
  const dur = get().project.duration || 0;
  let msg = '';
  if (!(draft.start >= 0)) msg = 'Start cannot be negative.';
  else if (!(draft.end > draft.start + 1)) msg = 'A clip must be at least one second long.';
  else if (draft.end > dur + 0.5) msg = `The episode ends at ${fmtTime(dur)}.`;
  setError($('editEnd'), $('boundsErr'), msg);
  return !msg;
}

const queueSave = debounce(() => save(), 700);

/** Send pending edits to the server. */
async function save() {
  if (!draft || !Object.keys(pending).length) return;
  if (!validateBounds()) return;
  const patch = {id: draft.id, ...pending};
  pending = {};
  saving = (async () => {
    try {
      const r = await api('clip', patch);
      if (draft && draft.id === r.clip.id) draft = {...r.clip, ...pendingValues()};
      replaceClip(r.clip);
      if (!Object.keys(pending).length) $('dirtyFlag').classList.add('hide');
    } catch (err) {
      pending = {...patch, ...pending};
      delete pending.id;
      notify.err(err);
    }
  })();
  await saving;
}

/** Flush unsent edits before switching clips or starting a job. */
export async function flushEditor() {
  queueSave.flush();
  await save();
  if (saving) await saving;
}

// ---------- Timeline ----------
function window_() {
  const dur = get().project.duration || 1;
  const first = Math.max(0, Math.min(draft.start, Number($('editStart').value) || draft.start) - 10);
  const last = Math.min(dur, Math.max(draft.end, Number($('editEnd').value) || draft.end) + 10);
  return {first, last, total: Math.max(1, last - first)};
}

function pct(n, win) {
  return clamp(((n - win.first) / win.total) * 100, 0, 100);
}

function drawTimeline() {
  if (!draft) return;
  const win = window_();
  $('tlStart').textContent = fmtTime(win.first);
  $('tlEnd').textContent = fmtTime(win.last);
  const s = pct(draft.start, win);
  const e = pct(draft.end, win);
  $('trimStart').style.left = `${s}%`;
  $('trimEnd').style.left = `${e}%`;
  $('trimSel').style.left = `${s}%`;
  $('trimSel').style.width = `${Math.max(0, e - s)}%`;
  for (const [id, v] of [['trimStart', draft.start], ['trimEnd', draft.end]]) {
    const el = $(id);
    el.setAttribute('aria-valuemin', String(Math.round(win.first)));
    el.setAttribute('aria-valuemax', String(Math.round(win.last)));
    el.setAttribute('aria-valuenow', String(Math.round(v)));
    el.setAttribute('aria-valuetext', fmtTime(v, 1));
  }
  const stripes = clear($('cueStripes'));
  for (const c of get().project.cues || []) {
    if (c.end < win.first || c.start > win.last) continue;
    stripes.append(h('div', {class: 'cue', title: c.text, style: {left: `${pct(c.start, win)}%`, width: `${Math.max(0.2, pct(c.end, win) - pct(c.start, win))}%`}}));
  }
  const lane = clear($('brollLane'));
  for (const b of draft.broll || []) {
    const bs = draft.start + b.start;
    lane.append(h('div', {class: 'block', title: fileName(b.path), style: {left: `${pct(bs, win)}%`, width: `${Math.max(0.3, pct(bs + b.duration, win) - pct(bs, win))}%`}}));
  }
  $('track').dataset.first = win.first;
  $('track').dataset.last = win.last;
}

export function updatePlayhead(t) {
  if (!draft) return;
  const win = {first: Number($('track').dataset.first) || 0, last: Number($('track').dataset.last) || 1};
  win.total = Math.max(1, win.last - win.first);
  $('playhead').style.left = `${pct(t, win)}%`;
}

function setBound(key, time) {
  if (!draft) return;
  if (key === 'start') time = clamp(time, 0, draft.end - 1);
  else time = clamp(time, draft.start + 1, (get().project.duration || time) + 0.5);
  time = round2(time);
  draft[key] = time;
  pending[key] = time;
  $(key === 'start' ? 'editStart' : 'editEnd').value = time;
  markDirty();
  validateBounds();
  drawTimeline();
  preview.seek(time);
  queueSave();
}

function initHandles() {
  for (const [id, key] of [['trimStart', 'start'], ['trimEnd', 'end']]) {
    const el = $(id);
    let raf = 0;
    el.addEventListener('pointerdown', (e) => {
      e.preventDefault();
      el.setPointerCapture(e.pointerId);
    });
    el.addEventListener('pointermove', (e) => {
      if (!el.hasPointerCapture(e.pointerId) || !draft) return;
      const track = $('track');
      const rect = track.getBoundingClientRect();
      const first = Number(track.dataset.first);
      const last = Number(track.dataset.last);
      const time = first + clamp((e.clientX - rect.left) / rect.width, 0, 1) * (last - first);
      cancelAnimationFrame(raf);
      raf = requestAnimationFrame(() => setBound(key, time));
    });
    el.addEventListener('pointerup', (e) => {
      try {
        el.releasePointerCapture(e.pointerId);
      } catch {
        /* ignore */
      }
    });
    el.addEventListener('keydown', (e) => {
      if (!draft) return;
      const step = e.shiftKey ? 1 : 0.1;
      if (e.key === 'ArrowLeft' || e.key === 'ArrowDown') {
        e.preventDefault();
        setBound(key, draft[key] - step);
      } else if (e.key === 'ArrowRight' || e.key === 'ArrowUp') {
        e.preventDefault();
        setBound(key, draft[key] + step);
      }
    });
  }
}

/** Keyboard shortcuts used by main.js: set in/out at the playhead. */
export function setInOutFromPlayhead(which) {
  if (!draft) return;
  setBound(which, preview.currentTime());
}

// ---------- Actions ----------
async function renderSelected() {
  if (!draft) return;
  await flushEditor();
  if (!validateBounds()) return;
  await api('render', {id: draft.id});
  notify.ok('Render queued.');
  set({jobsDirty: Date.now()});
  navigate('queue');
}

async function aiReview() {
  if (!draft || !requireOnlineProvider()) return;
  await flushEditor();
  $('editorNote').textContent = 'The editorial model is reviewing context and proposing framing…';
  const r = await api('clip/review', {id: draft.id, provider: providerConfig()});
  applyStatus(await api('status'));
  const issues = (r.issues || []).join(' • ');
  $('editorNote').textContent = `${r.contextOK ? 'Context OK.' : 'Missing context.'} ${issues} ${r.faceData ? '· Face metadata reviewed' : '· No face metadata available'}`;
  notify.ok('AI updated the title and framing. Review before rendering.');
}

async function cleanDeadAir() {
  if (!draft) return;
  await flushEditor();
  const r = await api('clip/clean', {id: draft.id});
  const rep = r.report || {};
  let msg = `${rep.fillers} filler word${rep.fillers === 1 ? '' : 's'} · ${rep.silences} long pause${rep.silences === 1 ? '' : 's'}`;
  if (rep.fillerWords?.length) msg += ` (${rep.fillerWords.slice(0, 8).join(', ')})`;
  if (rep.deadAir > 0.15) {
    $('editorNote').textContent = `${msg} · ${rep.deadAir.toFixed(1)}s of dead air at the edges`;
    if (await confirm('Trim dead air?', `Tighten this clip by ${rep.deadAir.toFixed(1)} seconds of silence at the start and end?`, {yes: 'Trim'})) {
      setBound('start', r.suggestedStart);
      setBound('end', r.suggestedEnd);
      await flushEditor();
      notify.ok('Clip tightened.');
    }
  } else {
    $('editorNote').textContent = `${msg} · edges already tight`;
  }
}

function toggleTranscriptEditor() {
  if (!draft) return;
  const box = $('transcriptEditor');
  if (!box.classList.contains('hide')) {
    box.classList.add('hide');
    clear(box);
    return;
  }
  const cues = (get().project.cues || []).map((cue, idx) => ({cue, idx})).filter((x) => x.cue.end > draft.start && x.cue.start < draft.end);
  if (!cues.length) {
    notify.info('No transcript cues overlap this clip.');
    return;
  }
  clear(box);
  const inputs = cues.map((x) => {
    const input = h('input', {type: 'text', value: x.cue.text, 'aria-label': `Cue at ${fmtTime(x.cue.start)}`});
    box.append(h('div', {class: 'tcue'}, h('span', {}, fmtTime(x.cue.start)), input));
    return {idx: x.idx, input};
  });
  const saveBtn = h('button', {class: 'btn primary sm', type: 'button'}, 'Save transcript');
  const cancelBtn = h('button', {class: 'btn sm', type: 'button', onclick: toggleTranscriptEditor}, 'Close');
  saveBtn.addEventListener('click', () => busy(saveBtn, async () => {
    const all = (get().project.cues || []).map((c) => ({start: c.start, end: c.end, text: c.text}));
    for (const {idx, input} of inputs) if (all[idx]) all[idx].text = input.value;
    applyStatus(await api('transcript', {cues: all}));
    toggleTranscriptEditor();
    notify.ok('Transcript updated. Re-render to see the new captions.');
  }));
  box.append(h('div', {class: 'actions', style: {marginTop: '8px'}}, saveBtn, cancelBtn));
  box.classList.remove('hide');
  inputs[0].input.focus();
}

async function generateMeta() {
  if (!draft || !requireOnlineProvider()) return;
  await flushEditor();
  const out = clear($('metaOutput'));
  out.classList.remove('hide');
  out.append(h('p', {class: 'muted small'}, 'Writing title, description and tags…'));
  const r = await api('clip/metadata', {id: draft.id, platform: 'youtube', provider: providerConfig()});
  clear(out);
  const block = (label, val) => {
    const copy = h('button', {class: 'btn sm', type: 'button'}, 'Copy');
    copy.addEventListener('click', async () => {
      try {
        await navigator.clipboard.writeText(val);
        notify.ok(`${label} copied.`);
      } catch {
        notify.err('Clipboard is unavailable; select the text and copy it manually.');
      }
    });
    return h('div', {class: 'meta-block'}, h('h4', {}, label), h('div', {class: 'val'}, val), copy);
  };
  out.append(block('Title', r.title || ''), block('Description', r.description || ''), block('Hashtags', (r.hashtags || []).map((x) => '#' + x).join(' ')), block('Tags', (r.tags || []).join(', ')));
  notify.ok('Metadata ready. Review before posting.');
}

// ---------- B-roll ----------
function renderBroll() {
  if (!draft) return;
  const list = clear($('brollList'));
  const items = draft.broll || [];
  $('brollCount').textContent = items.length ? `${items.length} overlay${items.length === 1 ? '' : 's'}` : 'none';
  if (!items.length) list.append(h('p', {class: 'muted small'}, 'No overlays yet. Add a local file or search free stock footage.'));
  items.forEach((b, i) => {
    list.append(h('div', {class: 'broll-item'}, h('span', {title: b.path}, `${fileName(b.path)} · ${b.start}s for ${b.duration}s${b.mode === 'pip' ? ' · PiP' : ''}`), h('button', {class: 'btn ghost sm', type: 'button', 'aria-label': 'Remove overlay', onclick: () => setBroll(items.filter((_, k) => k !== i))}, '✕')));
  });
}

function setBroll(items) {
  draft.broll = items;
  pending.broll = items;
  markDirty();
  renderBroll();
  drawTimeline();
  queueSave();
}

async function addBroll() {
  if (!draft) return;
  const path = $('brollPath').value.trim();
  if (!path) {
    notify.info('Choose a local video or image first.');
    return;
  }
  const start = Number($('brollStart').value);
  const duration = Number($('brollDuration').value);
  if (!(start >= 0) || !(duration > 0)) {
    notify.err('Overlay start must be ≥ 0 and the length positive.');
    return;
  }
  if (start > draft.end - draft.start) {
    notify.err('The overlay starts after the clip ends.');
    return;
  }
  setBroll([...(draft.broll || []), {path, start, duration, mode: $('brollMode').value}]);
  $('brollPath').value = '';
  await flushEditor();
}

async function searchStock() {
  if (!draft) return;
  const query = $('stockQuery').value.trim();
  const key = $('pexelsKey').value.trim();
  const out = clear($('stockResults'));
  if (!query) {
    notify.info('Type what to search for.');
    return;
  }
  const r = await api('stock/search', {query, apiKey: key});
  const videos = r.videos || [];
  if (!videos.length) {
    out.append(h('p', {class: 'muted small'}, 'No results. Try different words.'));
    return;
  }
  for (const v of videos) {
    const btn = h('button', {type: 'button', title: 'Download and add as an overlay'}, h('img', {src: v.preview, alt: '', loading: 'lazy'}), h('span', {}, v.credit || 'Pexels'));
    btn.addEventListener('click', () => busy(btn, async () => {
      const res = await api('stock/download', {url: v.url, id: v.id});
      setBroll([...(draft.broll || []), {path: res.path, start: Number($('brollStart').value) || 0, duration: Number($('brollDuration').value) || 3, mode: $('brollMode').value}]);
      await flushEditor();
      notify.ok('Stock footage added as an overlay.');
    }));
    out.append(btn);
  }
}

export {pickPath};
