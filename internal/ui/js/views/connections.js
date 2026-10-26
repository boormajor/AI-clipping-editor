import {api} from '../api.js';
import {get, saveProvider} from '../store.js';
import {$, busy, clear, h, notify} from '../ui.js';
import {PROVIDER_LABELS} from '../options.js';
import {navigate} from '../router.js';

let pollTimer = null;
let stopWaiting = null;

export function initConnections() {
  const p = get().provider;
  $('provider').value = p.provider;
  $('model').value = p.model || '';
  $('provider').addEventListener('change', onProviderChange);
  onProviderChange();

  $('chatgptConnect').addEventListener('click', () => busy($('chatgptConnect'), connectChatGPT));
  $('chatgptDisconnect').addEventListener('click', () => busy($('chatgptDisconnect'), async () => {
    await api('auth/chatgpt/disconnect', {});
    $('model').value = '';
    clear($('availableModels'));
    await refreshChatGPT();
    notify.info('Local ChatGPT connection cleared. You can also revoke the app in your ChatGPT settings.');
  }));
  $('chatgptModels').addEventListener('click', () => busy($('chatgptModels'), refreshChatGPT));
  $('claudeLogin').addEventListener('click', () => busy($('claudeLogin'), async () => {
    await api('auth/claude/login', {});
    notify.info('Finish signing in inside the terminal window, then click Recheck.');
  }));
  $('claudeCheck').addEventListener('click', () => busy($('claudeCheck'), refreshClaude));
  $('applyProviderBtn').addEventListener('click', applyProvider);
}

function onProviderChange() {
  const p = $('provider').value;
  $('apiKeyField').classList.toggle('hide', p !== 'openai' && p !== 'anthropic');
  $('chatgptBox').classList.toggle('hide', p !== 'chatgpt');
  $('claudeBox').classList.toggle('hide', p !== 'claude-code');
  const placeholders = {heuristic: 'Not used', chatgpt: 'Pick from the list after connecting', 'claude-code': 'Uses your Claude Code default', openai: 'gpt-4.1-mini', anthropic: 'claude-sonnet-4-5'};
  $('model').placeholder = placeholders[p] || '';
  $('model').disabled = p === 'heuristic';
  if (p === 'chatgpt') refreshChatGPT();
  if (p === 'claude-code') refreshClaude();
}

async function refreshChatGPT() {
  try {
    const s = await api('auth/status');
    $('chatgptStatus').textContent = s.pending ? 'Waiting for you to finish signing in in the browser…' : s.connected ? `Connected as ${s.email || 'ChatGPT user'} · plan usage enabled` : s.error || 'Not connected. Sign in and approve plan usage.';
    $('chatgptDisconnect').disabled = !s.connected;
    if (s.connected) {
      const r = await api('auth/chatgpt/models');
      const list = r.models || [];
      const dl = clear($('availableModels'));
      for (const m of list) dl.append(h('option', {value: m.slug}, m.name || m.slug));
      if (list.length && !$('model').value) $('model').value = list[0].slug;
    }
  } catch (err) {
    $('chatgptStatus').textContent = err.message;
  }
}

async function connectChatGPT() {
  await api('auth/chatgpt/start', {});
  if (stopWaiting) stopWaiting();
  stopWaiting = notify.info('Complete the sign-in in the browser window that just opened.');
  clearInterval(pollTimer);
  const started = Date.now();
  pollTimer = setInterval(async () => {
    try {
      const s = await api('auth/status');
      if (s.connected || s.error || Date.now() - started > 5 * 60 * 1000) {
        clearInterval(pollTimer);
        if (stopWaiting) stopWaiting();
        await refreshChatGPT();
        if (s.connected) notify.ok('ChatGPT connected.');
        else if (s.error) notify.err(s.error);
        else notify.info('Sign-in timed out. Try again.');
      }
    } catch {
      clearInterval(pollTimer);
    }
  }, 2200);
}

async function refreshClaude() {
  try {
    const s = await api('auth/claude/status');
    $('claudeStatus').textContent = (s.connected ? 'Connected: ' : '') + s.message;
  } catch (err) {
    $('claudeStatus').textContent = err.message;
  }
}

function applyProvider() {
  const provider = $('provider').value;
  const apiKey = $('apiKey').value.trim();
  if ((provider === 'openai' || provider === 'anthropic') && !apiKey) {
    notify.err('Enter an API key for this provider.');
    $('apiKey').focus();
    return;
  }
  saveProvider({provider, model: $('model').value.trim(), apiKey});
  notify.ok(`Using ${PROVIDER_LABELS[provider] || provider} for this session.`);
  navigate('clips');
}
