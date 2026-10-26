import {get, subscribe} from '../store.js';
import {$, fmtTime} from '../ui.js';
import {updatePlayhead} from './editor.js';

// Approximate preview: plays the source video cropped by the chosen pan and
// overlays captions grouped the same way the renderer groups them.

let clip = null;
let groups = [];
let srcPath = '';

export const preview = {
  update(c) {
    clip = c;
    groups = c?.words?.length ? captionGroups(c.words, Number(get().project.config.captionWords) || 3) : [];
    applyFraming();
    renderCaption();
  },
  seek(t) {
    const v = $('previewVideo');
    if (!v.src) return;
    try {
      v.currentTime = t;
    } catch {
      /* metadata not ready yet */
    }
  },
  currentTime() {
    return $('previewVideo').currentTime || 0;
  },
};

export function initPreview() {
  const v = $('previewVideo');
  subscribe('project', ({project}) => {
    const path = project.videoPath || '';
    $('previewPlaceholder').classList.toggle('hide', !!path);
    if (path !== srcPath) {
      srcPath = path;
      v.src = path ? '/api/media?v=' + encodeURIComponent(path) : '';
    }
    applyAspect(project.config);
    applyFraming();
  });
  subscribe('project', () => {
    if (clip) {
      const fresh = get().project.clips.find((c) => c.id === clip.id);
      if (fresh) preview.update({...fresh, ...clip});
    }
  });
  v.addEventListener('timeupdate', () => {
    if (clip && v.currentTime > clip.end + 0.05) {
      v.pause();
      v.currentTime = clip.start;
    }
    $('previewTime').textContent = `${fmtTime(v.currentTime)} / ${fmtTime(get().project.duration || 0)}`;
    updatePlayhead(v.currentTime);
    renderCaption();
  });
  v.addEventListener('play', () => ($('playBtn').textContent = '❚❚'));
  v.addEventListener('pause', () => ($('playBtn').textContent = '▶'));
  $('playBtn').addEventListener('click', togglePlay);
  v.addEventListener('click', togglePlay);
}

export function togglePlay() {
  const v = $('previewVideo');
  if (!v.src) return;
  if (v.paused) {
    if (clip && (v.currentTime < clip.start || v.currentTime > clip.end)) v.currentTime = clip.start;
    v.play().catch(() => {});
  } else v.pause();
}

export function nudge(seconds) {
  const v = $('previewVideo');
  if (!v.src) return;
  v.currentTime = Math.max(0, v.currentTime + seconds);
}

function applyAspect(cfg) {
  const w = cfg.width || 1080;
  const hgt = cfg.height || 1920;
  $('frame').style.aspectRatio = `${w} / ${hgt}`;
  $('frame').style.maxWidth = hgt >= w ? '280px' : '340px';
  const ratio = w / hgt;
  const label = Math.abs(ratio - 9 / 16) < 0.01 ? '9:16' : Math.abs(ratio - 4 / 5) < 0.01 ? '4:5' : Math.abs(ratio - 1) < 0.01 ? '1:1' : Math.abs(ratio - 16 / 9) < 0.01 ? '16:9' : `${w}×${hgt}`;
  $('aspectPill').textContent = label;
}

function applyFraming() {
  const v = $('previewVideo');
  const cfg = get().project.config || {};
  if (!clip || !cfg.reframe) {
    v.style.objectPosition = 'center';
    v.style.objectFit = cfg.reframe === false ? 'contain' : 'cover';
    return;
  }
  v.style.objectFit = 'cover';
  v.style.objectPosition = clip.layout === 'two-up' ? 'center' : `${Math.round((clip.pan ?? 0.5) * 100)}% center`;
}

function captionGroups(words, maxWords) {
  const out = [];
  let cur = [];
  for (const w of words) {
    if (cur.length) {
      const last = cur[cur.length - 1];
      if (w.start - last.end > 0.32 || w.end - cur[0].start > 2.35 || cur.length >= maxWords) {
        out.push(cur);
        cur = [];
      }
    }
    cur.push(w);
  }
  if (cur.length) out.push(cur);
  return out;
}

const BOXED = new Set(['boxed', 'podcast', 'highlight']);
const UPPER = new Set(['kinetic', 'bold', 'karaoke', 'boxed', 'yellow', 'highlight', 'popline', 'wordbox', 'wipe', 'bounce']);

function renderCaption() {
  const cap = $('captionPreview');
  const cfg = get().project.config || {};
  if (!clip || !cfg.captions || !clip.captions) {
    cap.textContent = '';
    return;
  }
  const v = $('previewVideo');
  const t = v.currentTime - clip.start - (cfg.captionOffset || 0);
  const style = clip.captionStyle || cfg.captionStyle || 'kinetic';
  const accent = cfg.accentColor || '#FACC15';
  const base = ['yellow', 'highlight'].includes(style) ? accent : cfg.captionColor || '#fff';
  const frameW = $('frame').clientWidth || 280;
  cap.style.fontSize = `${((cfg.captionSize || 76) / 1080) * frameW}px`;
  cap.style.fontFamily = `"${cfg.captionFont || 'Inter'}", Inter, Arial, sans-serif`;
  cap.style.top = `${(cfg.captionPosition || 0.82) * 100}%`;
  cap.style.color = base;
  cap.classList.toggle('box', BOXED.has(style));
  while (cap.firstChild) cap.removeChild(cap.firstChild);
  let tokens = [];
  if (groups.length) {
    const g = groups.find((x) => t >= x[0].start && t < x[x.length - 1].end);
    if (g) tokens = g.map((w) => ({text: w.word, active: t >= w.start && t < w.end}));
  } else {
    const abs = v.currentTime - (cfg.captionOffset || 0);
    const cue = (get().project.cues || []).find((c) => c.start <= abs && c.end >= abs);
    if (cue) tokens = cue.text.split(/\s+/).map((text) => ({text, active: false}));
  }
  const upper = UPPER.has(style);
  tokens.forEach((tok, i) => {
    const span = document.createElement('b');
    span.textContent = upper ? tok.text.toUpperCase() : tok.text;
    if (tok.active && !['editorial', 'minimal', 'bold', 'boxed', 'podcast', 'yellow', 'highlight', 'typewriter'].includes(style)) span.style.color = accent;
    cap.append(span);
    if (i < tokens.length - 1) cap.append(' ');
  });
}
