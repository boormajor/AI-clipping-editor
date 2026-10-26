// Small DOM helpers: element lookup, node building, toasts, dialogs, busy
// buttons and formatting. No HTML strings are built from data; text always
// goes through textContent.

export const $ = (id) => {
  const el = document.getElementById(id);
  if (!el) throw new Error(`Missing element #${id}`);
  return el;
};

/** Build an element: h('div', {class: 'x', onclick}, child, 'text', ...). */
export function h(tag, attrs = {}, ...children) {
  const el = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs || {})) {
    if (v === undefined || v === null || v === false) continue;
    if (k === 'class') el.className = v;
    else if (k === 'style' && typeof v === 'object') Object.assign(el.style, v);
    else if (k.startsWith('on') && typeof v === 'function') el.addEventListener(k.slice(2), v);
    else if (k === 'dataset') Object.assign(el.dataset, v);
    else if (v === true) el.setAttribute(k, '');
    else el.setAttribute(k, v);
  }
  for (const c of children.flat()) {
    if (c === null || c === undefined || c === false) continue;
    el.append(c instanceof Node ? c : document.createTextNode(String(c)));
  }
  return el;
}

export function clear(el) {
  while (el.firstChild) el.removeChild(el.firstChild);
  return el;
}

/** Format seconds as m:ss or h:mm:ss. */
export function fmtTime(s, decimals = 0) {
  if (!Number.isFinite(s) || s < 0) s = 0;
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const sec = s % 60;
  const secStr = decimals ? sec.toFixed(decimals).padStart(3 + decimals, '0') : String(Math.floor(sec)).padStart(2, '0');
  return h ? `${h}:${String(m).padStart(2, '0')}:${secStr}` : `${m}:${secStr}`;
}

export function round2(n) {
  return Math.round(n * 100) / 100;
}

export function clamp(n, min, max) {
  return Math.max(min, Math.min(max, n));
}

// ---------- Toasts ----------
const toastRoot = () => document.getElementById('toasts');

/**
 * Show a toast. Errors stay until dismissed; everything else fades after 5s.
 * Returns a function that removes the toast early.
 */
export function toast(message, kind = 'info') {
  const el = h('div', {class: `toast ${kind}`, role: kind === 'error' ? 'alert' : 'status'}, h('span', {}, message), h('button', {type: 'button', 'aria-label': 'Dismiss', onclick: () => el.remove()}, '✕'));
  toastRoot().append(el);
  const root = toastRoot();
  while (root.children.length > 5) root.firstChild.remove();
  let timer = null;
  if (kind !== 'error') timer = setTimeout(() => el.remove(), 5000);
  return () => {
    clearTimeout(timer);
    el.remove();
  };
}

export const notify = {
  info: (m) => toast(m, 'info'),
  ok: (m) => toast(m, 'success'),
  err: (m) => toast(m instanceof Error ? m.message : String(m), 'error'),
};

// ---------- Busy buttons ----------
/** Run fn while the button is disabled and shows its spinner. */
export async function busy(btn, fn) {
  if (btn.getAttribute('aria-busy') === 'true') return undefined;
  btn.setAttribute('aria-busy', 'true');
  btn.disabled = true;
  try {
    return await fn();
  } catch (err) {
    if (err?.name !== 'AbortError') notify.err(err);
    return undefined;
  } finally {
    btn.removeAttribute('aria-busy');
    btn.disabled = false;
  }
}

// ---------- Dialogs ----------
/** In-app confirm dialog. Resolves true/false. */
export function confirm(title, body, {yes = 'Confirm', danger = false} = {}) {
  return new Promise((resolve) => {
    const dlg = $('confirmDialog');
    $('confirmTitle').textContent = title;
    $('confirmBody').textContent = body;
    const yesBtn = $('confirmYes');
    yesBtn.textContent = yes;
    yesBtn.className = danger ? 'btn danger' : 'btn primary';
    const done = (v) => {
      dlg.close();
      resolve(v);
    };
    yesBtn.onclick = () => done(true);
    $('confirmNo').onclick = () => done(false);
    dlg.oncancel = (e) => {
      e.preventDefault();
      done(false);
    };
    dlg.showModal();
    yesBtn.focus();
  });
}

// ---------- Field errors ----------
export function setError(input, errEl, message) {
  if (errEl) errEl.textContent = message || '';
  if (input) {
    if (message) input.setAttribute('aria-invalid', 'true');
    else input.removeAttribute('aria-invalid');
  }
  return !message;
}

/** Debounce helper. */
export function debounce(fn, ms) {
  let t = null;
  const wrapped = (...args) => {
    clearTimeout(t);
    t = setTimeout(() => fn(...args), ms);
  };
  wrapped.flush = () => {
    clearTimeout(t);
  };
  return wrapped;
}

export function fillSelect(select, items, labelFor = (v) => v, current) {
  clear(select);
  for (const item of items) {
    const value = typeof item === 'object' ? item.value : item;
    const label = typeof item === 'object' ? item.label : labelFor(item);
    select.append(h('option', {value}, label));
  }
  if (current !== undefined) select.value = String(current);
}

export function fileName(path) {
  return String(path || '').split(/[\\/]/).pop();
}
