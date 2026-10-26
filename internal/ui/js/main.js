import {api, onApiEvent} from './api.js';
import {applyStatus, get, set, subscribe} from './store.js';
import {$, busy, confirm, notify} from './ui.js';
import {initRouter, navigate} from './router.js';
import {initImport} from './views/import.js';
import {initClips, selectClip} from './views/clips.js';
import {flushEditor, initEditor, setInOutFromPlayhead} from './views/editor.js';
import {initPreview, nudge, togglePlay} from './views/preview.js';
import {initSettings, saveSettingsNow} from './views/settings.js';
import {initConnections} from './views/connections.js';
import {initQueue} from './views/queue.js';
import {pickPath} from './views/shared.js';

function renderTopbar({project, tools, version}) {
  $('projectName').textContent = project.videoPath ? `${project.name} · ${Math.round(project.duration / 60)} min · ${project.cues.length} cues${project.words?.length ? ' · word-timed' : ''}` : 'No episode imported';
  $('version').textContent = version ? `v${version}` : '';
  const missing = [];
  if (!tools.ffmpeg || !tools.ffprobe) missing.push('FFmpeg');
  if (!tools.remotion) missing.push('Remotion');
  const pill = $('toolPill');
  if (!missing.length) {
    pill.textContent = 'FFmpeg + Remotion ready';
    pill.className = 'pill ok';
    pill.title = 'Renders use Remotion';
  } else if (missing.includes('FFmpeg')) {
    pill.textContent = 'FFmpeg missing';
    pill.className = 'pill bad';
    pill.title = 'Run Install-and-Run.bat to install FFmpeg';
  } else {
    pill.textContent = 'Remotion not installed';
    pill.className = 'pill warn';
    pill.title = 'Renders will use the FFmpeg fallback until Remotion is installed (run Install-and-Run.bat)';
  }
}

function initShortcuts() {
  document.addEventListener('keydown', (e) => {
    const target = e.target;
    const typing = target instanceof HTMLElement && (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.tagName === 'SELECT' || target.isContentEditable);
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 's') {
      e.preventDefault();
      $('saveProjectBtn').click();
      return;
    }
    if (typing || get().route !== 'clips') return;
    switch (e.key) {
      case ' ':
        e.preventDefault();
        togglePlay();
        break;
      case 'i':
      case 'I':
        setInOutFromPlayhead('start');
        break;
      case 'o':
      case 'O':
        setInOutFromPlayhead('end');
        break;
      case 'j':
      case 'J':
        nudge(-5);
        break;
      case 'l':
      case 'L':
        nudge(5);
        break;
      case 'k':
      case 'K':
        togglePlay();
        break;
      case 'ArrowLeft':
        nudge(e.shiftKey ? -1 : -0.1);
        break;
      case 'ArrowRight':
        nudge(e.shiftKey ? 1 : 0.1);
        break;
      default:
        return;
    }
  });
}

async function boot() {
  onApiEvent((event) => {
    if (event === 'unauthorized') {
      set({linked: false});
      $('sessionBanner').classList.remove('hide');
    } else if (event === 'offline') {
      notify.err('Lost contact with Podcast AI Studio. Keep the console window open and reload once it is back.');
    } else if (event === 'online') {
      notify.ok('Reconnected.');
    }
  });

  subscribe(['project', 'tools', 'version'], renderTopbar);
  initRouter('import');
  initImport();
  initClips();
  initEditor();
  initPreview();
  initSettings();
  initConnections();
  initQueue();
  initShortcuts();

  $('saveProjectBtn').addEventListener('click', () => busy($('saveProjectBtn'), async () => {
    await flushEditor();
    await saveSettingsNow();
    const r = await api('project/save', {});
    notify.ok(`Project saved to ${r.path}`);
  }));
  $('openProjectBtn').addEventListener('click', () => busy($('openProjectBtn'), async () => {
    const path = await pickPath('project');
    if (!path) return;
    await flushEditor();
    set({selectedID: ''});
    applyStatus(await api('project/open', {path}));
    notify.ok('Project opened.');
    navigate('clips');
  }));
  $('newProjectBtn').addEventListener('click', () => busy($('newProjectBtn'), async () => {
    if (!(await confirm('Start a new project?', 'The current episode and clips are cleared from the studio. Saved project files are not deleted.', {yes: 'New project', danger: true}))) return;
    set({selectedID: ''});
    applyStatus(await api('project/new', {}));
    navigate('import');
  }));
  $('quitBtn').addEventListener('click', () => busy($('quitBtn'), async () => {
    if (!(await confirm('Quit Podcast AI Studio?', 'Running renders are cancelled. Unsaved edits are kept in the studio and restored next launch.', {yes: 'Quit'}))) return;
    await flushEditor();
    await api('quit', {});
    document.body.innerHTML = '<p style="padding:40px;font:16px system-ui;color:#aab4c6">Podcast AI Studio has stopped. You can close this window.</p>';
  }));

  try {
    const [status, options] = await Promise.all([api('status'), api('options')]);
    set({options});
    applyStatus(status);
    if (status.project?.videoPath && !location.hash) navigate('clips');
    if (get().project.clips.length) selectClip(get().project.clips[0].id);
  } catch (err) {
    if (err.status !== 401) notify.err(err);
  }
}

boot();
