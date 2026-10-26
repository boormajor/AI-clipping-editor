import {api} from '../api.js';
import {applyStatus, get, subscribe} from '../store.js';
import {$, clear, debounce, fillSelect, h, notify, setError} from '../ui.js';
import {FEATURE_GROUPS, PRESETS, PRESET_LABELS} from '../options.js';

// Settings save automatically after a short pause in typing. Nothing here is
// per clip; it is the project's defaults.

let hydrating = false;

export function initSettings() {
  subscribe('options', ({options}) => {
    if (!options) return;
    fillSelect($('captionFont'), options.captionFonts);
    fillSelect($('captionStyle'), options.captionPresets.map((v) => ({value: v, label: PRESET_LABELS[v] || v})));
    fillSelect($('renderFPS'), options.fps.map((v) => ({value: v, label: `${v} fps`})));
    sync(get().project.config);
  });
  subscribe('project', ({project}) => sync(project.config || {}));
  subscribe(['stateDir', 'logPath', 'project'], ({stateDir, logPath, project}) => {
    const out = clear($('pathsInfo'));
    const rows = [['Exports', project.outputDir || (project.videoPath ? 'PodcastAI-Exports beside the video' : '—')], ['Settings & cache', stateDir || '—'], ['Log file', logPath || '—']];
    for (const [k, v] of rows) out.append(h('div', {}, h('b', {}, k + ': '), h('span', {class: 'mono'}, v)));
  });

  drawSwitches();
  $('preset').addEventListener('change', () => {
    const p = PRESETS[$('preset').value];
    if (p) for (const [k, on] of Object.entries(p)) $('feature-' + k).checked = on;
    queueSave();
  });
  for (const el of document.querySelectorAll('#page-settings input, #page-settings select, #page-settings textarea')) {
    el.addEventListener('input', () => {
      if (hydrating) return;
      if (el.id === 'captionPosition') $('posLabel').textContent = `${el.value}% down`;
      queueSave();
    });
  }
  document.querySelectorAll('#aspectRow button').forEach((b) => b.addEventListener('click', () => {
    $('renderWidth').value = b.dataset.w;
    $('renderHeight').value = b.dataset.h;
    markAspect();
    queueSave();
  }));
}

function drawSwitches() {
  const target = clear($('switches'));
  for (const [group, items] of FEATURE_GROUPS) {
    const wrap = h('div', {style: {marginBottom: '8px'}}, h('h4', {class: 'small muted', style: {margin: '8px 0 2px', textTransform: 'uppercase', letterSpacing: '.08em'}}, group));
    for (const [key, label] of items) {
      const input = h('input', {type: 'checkbox', id: 'feature-' + key});
      input.addEventListener('change', () => {
        if (!hydrating) $('preset').value = 'custom';
      });
      wrap.append(h('label', {class: 'switch'}, h('div', {}, label), input));
    }
    target.append(wrap);
  }
}

function markAspect() {
  const w = $('renderWidth').value;
  const hgt = $('renderHeight').value;
  document.querySelectorAll('#aspectRow button').forEach((b) => b.setAttribute('aria-pressed', String(b.dataset.w === w && b.dataset.h === hgt)));
}

function sync(cfg) {
  if (!get().options) return;
  hydrating = true;
  try {
    $('preset').value = cfg.preset || 'simple';
    for (const [, items] of FEATURE_GROUPS) for (const [k] of items) $('feature-' + k).checked = !!cfg[k];
    $('captionFont').value = cfg.captionFont || 'Inter';
    $('captionSize').value = cfg.captionSize || 76;
    $('captionColor').value = (cfg.captionColor || '#ffffff').toLowerCase();
    $('accentColor').value = (cfg.accentColor || '#facc15').toLowerCase();
    $('captionWords').value = cfg.captionWords || 3;
    $('captionPosition').value = Math.round((cfg.captionPosition || 0.82) * 100);
    $('posLabel').textContent = `${Math.round((cfg.captionPosition || 0.82) * 100)}% down`;
    $('captionStyle').value = cfg.captionStyle || 'kinetic';
    $('captionOffset').value = cfg.captionOffset || 0;
    $('captionEmphasis').checked = !!cfg.captionEmphasis;
    $('instructions').value = cfg.instructions || '';
    $('renderWidth').value = cfg.width || 1080;
    $('renderHeight').value = cfg.height || 1920;
    $('renderFPS').value = String(cfg.fps || 30);
    $('gpu').checked = cfg.gpu !== false;
    markAspect();
  } finally {
    hydrating = false;
  }
}

export function formConfig() {
  const cfg = {
    ...get().project.config,
    preset: $('preset').value,
    captionFont: $('captionFont').value,
    captionSize: Number($('captionSize').value),
    captionColor: $('captionColor').value.toUpperCase(),
    accentColor: $('accentColor').value.toUpperCase(),
    captionWords: Number($('captionWords').value),
    captionPosition: Number($('captionPosition').value) / 100,
    captionStyle: $('captionStyle').value,
    captionOffset: Number($('captionOffset').value),
    captionEmphasis: $('captionEmphasis').checked,
    instructions: $('instructions').value,
    width: Number($('renderWidth').value),
    height: Number($('renderHeight').value),
    fps: Number($('renderFPS').value),
    gpu: $('gpu').checked,
  };
  for (const [, items] of FEATURE_GROUPS) for (const [k] of items) cfg[k] = $('feature-' + k).checked;
  return cfg;
}

function validate(cfg) {
  let msg = '';
  if (cfg.width % 2 || cfg.height % 2) msg = 'Width and height must be even numbers.';
  else if (cfg.width < 240 || cfg.height < 240 || cfg.width > 3840 || cfg.height > 3840) msg = 'Resolution must be between 240 and 3840 pixels.';
  else if (cfg.captionSize < 24 || cfg.captionSize > 150) msg = 'Font size must be between 24 and 150.';
  else if (cfg.captionWords < 1 || cfg.captionWords > 12) msg = 'Words on screen must be between 1 and 12.';
  else if (Math.abs(cfg.captionOffset) > 10) msg = 'Offset must be within ±10 seconds.';
  setError(null, $('settingsErr'), msg);
  return !msg;
}

const queueSave = debounce(async () => {
  const cfg = formConfig();
  if (!validate(cfg)) return;
  try {
    applyStatus(await api('config', cfg));
  } catch (err) {
    notify.err(err);
  }
}, 600);

/** Save immediately (used before renders). */
export async function saveSettingsNow() {
  queueSave.flush();
  const cfg = formConfig();
  if (!validate(cfg)) throw new Error($('settingsErr').textContent);
  applyStatus(await api('config', cfg));
}
