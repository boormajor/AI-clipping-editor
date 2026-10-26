// Minimal observable store. Views subscribe to the slices they render and
// re-render when `set` changes them; there is one source of truth for the
// project, options, jobs and the editor selection.

const state = {
  project: {clips: [], cues: [], words: [], config: {}},
  revision: 0,
  tools: {},
  version: '',
  stateDir: '',
  logPath: '',
  options: null,
  jobs: [],
  selectedID: '',
  provider: loadProvider(),
  route: 'import',
  linked: true,
};

const subs = new Map();

export function get() {
  return state;
}

/** Merge a partial state and notify subscribers of the changed keys. */
export function set(patch) {
  const changed = [];
  for (const [k, v] of Object.entries(patch)) {
    if (state[k] !== v) {
      state[k] = v;
      changed.push(k);
    }
  }
  if (!changed.length) return;
  const seen = new Set();
  for (const k of changed) {
    for (const fn of subs.get(k) || []) {
      if (!seen.has(fn)) {
        seen.add(fn);
        fn(state);
      }
    }
  }
}

/** Subscribe to one or more keys. Returns an unsubscribe function. */
export function subscribe(keys, fn) {
  for (const k of [].concat(keys)) {
    if (!subs.has(k)) subs.set(k, new Set());
    subs.get(k).add(fn);
  }
  return () => {
    for (const k of [].concat(keys)) subs.get(k)?.delete(fn);
  };
}

/** Apply a /api/status style payload. */
export function applyStatus(s) {
  const patch = {project: s.project, revision: s.revision, tools: s.tools || state.tools, version: s.version || state.version, stateDir: s.stateDir ?? state.stateDir, logPath: s.logPath ?? state.logPath};
  const clips = s.project?.clips || [];
  if (state.selectedID && !clips.some((c) => c.id === state.selectedID)) patch.selectedID = '';
  if (!patch.selectedID && !state.selectedID && clips.length) patch.selectedID = clips[0].id;
  set(patch);
}

export function selectedClip() {
  return state.project.clips.find((c) => c.id === state.selectedID);
}

/** Replace one clip in the project without touching the rest. */
export function replaceClip(clip) {
  const clips = state.project.clips.map((c) => (c.id === clip.id ? clip : c));
  set({project: {...state.project, clips}});
}

// The provider choice (not the API key) persists across launches.
function loadProvider() {
  try {
    const saved = JSON.parse(localStorage.getItem('podcast-ai-studio.provider') || 'null');
    if (saved && typeof saved.provider === 'string') return {provider: saved.provider, model: saved.model || '', apiKey: ''};
  } catch {
    /* ignore */
  }
  return {provider: 'heuristic', model: '', apiKey: ''};
}

export function saveProvider(p) {
  set({provider: p});
  try {
    localStorage.setItem('podcast-ai-studio.provider', JSON.stringify({provider: p.provider, model: p.model}));
  } catch {
    /* ignore */
  }
}
