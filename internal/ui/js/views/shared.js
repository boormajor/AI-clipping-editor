import {api, ApiError} from '../api.js';
import {get} from '../store.js';
import {notify} from '../ui.js';

/** Open the native picker; falls back to a prompt on platforms without one. */
export async function pickPath(kind) {
  try {
    const r = await api('pick', {kind});
    return r.path || '';
  } catch (err) {
    if (err instanceof ApiError && err.code === 'no-picker') {
      const v = window.prompt(kind === 'folder' ? 'Folder path' : 'File path');
      return v ? v.trim() : '';
    }
    throw err;
  }
}

/** The provider config to send with AI requests. */
export function providerConfig() {
  return get().provider;
}

/** Ensure an online provider is configured, with a helpful message if not. */
export function requireOnlineProvider() {
  const p = get().provider;
  if (!p || p.provider === 'heuristic' || !p.provider) {
    notify.info('Choose ChatGPT, Claude or an API key under AI connections first.');
    return false;
  }
  return true;
}
