import {api} from '../api.js';
import {applyStatus, get, set, subscribe} from '../store.js';
import {$, busy, fillSelect, notify, setError} from '../ui.js';
import {LANGUAGE_LABELS, SPEECH_HINTS, SPEECH_LABELS} from '../options.js';
import {navigate} from '../router.js';
import {pickPath} from './shared.js';

let activeImport = null; // job id while an import runs

export function initImport() {
  subscribe('options', ({options}) => {
    if (!options) return;
    fillSelect($('speechProvider'), options.speechProviders, (v) => SPEECH_LABELS[v] || v);
    fillSelect($('speechLanguage'), options.speechLanguages, (v) => LANGUAGE_LABELS[v] || v);
    updateHint();
  });
  subscribe('project', ({project}) => {
    if (project.videoPath && !$('videoPath').value) $('videoPath').value = project.videoPath;
    if (project.transcriptPath && !$('transcriptPath').value) $('transcriptPath').value = project.transcriptPath;
    if (project.outputDir && !$('outputDir').value) $('outputDir').value = project.outputDir;
  });
  subscribe('jobs', ({jobs}) => renderImportProgress(jobs));

  $('speechProvider').addEventListener('change', updateHint);
  $('transcriptPath').addEventListener('input', updateHint);
  document.querySelectorAll('.browse').forEach((btn) => btn.addEventListener('click', () => busy(btn, async () => {
    const path = await pickPath(btn.dataset.kind);
    if (path) {
      $(btn.dataset.target).value = path;
      $(btn.dataset.target).dispatchEvent(new Event('input'));
    }
  })));

  // Drag & drop fills in the file name. Browsers hide the full path, so the
  // user still confirms the folder; on Windows the Browse dialog is faster.
  const zone = $('dropzone');
  for (const evt of ['dragenter', 'dragover']) zone.addEventListener(evt, (e) => {
    e.preventDefault();
    zone.classList.add('over');
  });
  for (const evt of ['dragleave', 'drop']) zone.addEventListener(evt, () => zone.classList.remove('over'));
  zone.addEventListener('drop', (e) => {
    e.preventDefault();
    const file = e.dataTransfer?.files?.[0];
    if (!file) return;
    const full = file.path || file.name; // Edge app mode exposes .path; browsers only the name
    $('videoPath').value = full;
    if (!file.path) notify.info(`Dropped ${file.name}. Add the folder path before importing, or use Browse.`);
  });

  $('importBtn').addEventListener('click', () => busy($('importBtn'), startImport));
  $('importCancel').addEventListener('click', () => busy($('importCancel'), async () => {
    if (activeImport) await api('jobs/cancel', {id: activeImport});
  }));
}

function updateHint() {
  const provider = $('speechProvider').value;
  $('speechHint').textContent = SPEECH_HINTS[provider] || '';
  const hasTranscript = $('transcriptPath').value.trim() !== '';
  $('speechFields').style.opacity = hasTranscript ? '0.5' : '';
  $('speechFields').title = hasTranscript ? 'Not needed when a timed transcript is supplied' : '';
}

function validate() {
  let ok = true;
  const video = $('videoPath').value.trim();
  ok = setError($('videoPath'), $('videoPathErr'), video ? '' : 'Choose the podcast video first.') && ok;
  const needsKey = $('transcriptPath').value.trim() === '';
  const key = $('speechKey').value.trim();
  ok = setError($('speechKey'), $('speechKeyErr'), needsKey && !key ? 'Enter a speech API key, or supply a timed transcript instead.' : '') && ok;
  return ok;
}

async function startImport() {
  if (!validate()) return;
  if (get().project.clips?.length) {
    const {confirm} = await import('../ui.js');
    if (!(await confirm('Replace the current episode?', 'Importing a new episode clears the current clips. Save the project first if you want to keep them.', {yes: 'Import'}))) return;
  }
  const body = {
    videoPath: $('videoPath').value.trim(),
    transcriptPath: $('transcriptPath').value.trim(),
    outputDir: $('outputDir').value.trim(),
    name: '',
    speechProvider: $('speechProvider').value,
    speechKey: $('speechKey').value.trim(),
    speechLanguage: $('speechLanguage').value,
  };
  const r = await api('import', body);
  activeImport = r.job;
  $('importStatus').classList.remove('hide');
  $('importStage').textContent = 'Starting…';
  $('importBar').style.width = '0%';
  $('importPct').textContent = '0%';
  set({jobsDirty: Date.now()}); // nudge the poller
}

function renderImportProgress(jobs) {
  if (!activeImport) return;
  const j = jobs.find((x) => x.id === activeImport);
  if (!j) return;
  $('importStage').textContent = j.stage || j.status;
  $('importBar').style.width = `${j.progress || 0}%`;
  $('importPct').textContent = `${j.progress || 0}%`;
  if (j.status === 'queued' || j.status === 'running') return;
  activeImport = null;
  $('importStatus').classList.add('hide');
  if (j.status === 'complete') {
    $('speechKey').value = '';
    api('status').then((s) => {
      set({selectedID: ''});
      applyStatus(s);
      notify.ok('Episode imported. Find the best moments next.');
      navigate('clips');
    }).catch(notify.err);
  } else if (j.status === 'cancelled') {
    notify.info('Import cancelled.');
  } else {
    notify.err(j.error || 'Import failed');
  }
}
