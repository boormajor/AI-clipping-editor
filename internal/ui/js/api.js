// Thin fetch wrapper for the local JSON API.
// Every error becomes an Error with a readable message; a 401 flips the app
// into "session not linked" mode so the user gets one clear banner instead of
// a stream of failing calls.

export class ApiError extends Error {
  constructor(message, status, code) {
    super(message);
    this.status = status;
    this.code = code;
  }
}

const listeners = new Set();

/** Subscribe to API-level events: 'unauthorized' | 'offline' | 'online'. */
export function onApiEvent(fn) {
  listeners.add(fn);
  return () => listeners.delete(fn);
}

function emit(event) {
  for (const fn of listeners) fn(event);
}

let offline = false;

/**
 * Call an API route. GET when body is undefined, POST otherwise.
 * @param {string} path  route without the /api/ prefix
 * @param {object=} body JSON body for POST
 * @param {{signal?: AbortSignal}=} options
 */
export async function api(path, body, options = {}) {
  const init = {method: body === undefined ? 'GET' : 'POST', headers: {Accept: 'application/json'}, signal: options.signal};
  if (body !== undefined) {
    init.headers['Content-Type'] = 'application/json';
    init.body = JSON.stringify(body);
  }
  let res;
  try {
    res = await fetch('/api/' + path, init);
  } catch (err) {
    if (err.name === 'AbortError') throw err;
    if (!offline) {
      offline = true;
      emit('offline');
    }
    throw new ApiError('Cannot reach Podcast AI Studio. Is the console window still open?', 0);
  }
  if (offline) {
    offline = false;
    emit('online');
  }
  let data = null;
  const text = await res.text();
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      throw new ApiError(`Unexpected server response (${res.status})`, res.status);
    }
  }
  if (!res.ok) {
    if (res.status === 401) emit('unauthorized');
    throw new ApiError(data?.error || `Server error ${res.status}`, res.status, data?.code);
  }
  return data;
}
