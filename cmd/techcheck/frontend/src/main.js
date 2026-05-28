import { renderWelcome } from './screens/welcome.js';
import { renderConsent } from './screens/consent.js';
import { renderIdentify } from './screens/identify.js';
import { renderRunning } from './screens/running.js';
import { renderResult } from './screens/result.js';
import { renderDone } from './screens/done.js';
import { renderUpdateBanner } from './components/updateBanner.js';

const routes = {
  '#/welcome':  renderWelcome,
  '#/consent':  renderConsent,
  '#/identify': renderIdentify,
  '#/running':  renderRunning,
  '#/result':   renderResult,
  '#/done':     renderDone,
};

export const state = {
  fullName: '',
  email: '',
  // Keyed by task ID (e.g. "sysinfo", "<pop>:<test>"). Values carry the
  // latest status + summary so each task renders as a single row that updates.
  progress: new Map(),
  completion: null,
};

function navigate(hash) {
  if (window.location.hash === hash) {
    renderCurrent();
  } else {
    window.location.hash = hash;
  }
}

function renderCurrent() {
  const hash = window.location.hash || '#/welcome';
  const root = document.getElementById('app');
  root.innerHTML = '';
  const render = routes[hash] || renderWelcome;
  render(root, { navigate, state });
}

window.addEventListener('hashchange', renderCurrent);
window.addEventListener('DOMContentLoaded', renderCurrent);

async function checkForUpdate() {
  try {
    const mod = await import('../wailsjs/go/wizard/App.js');
    const status = await mod.CheckForUpdate();
    const banner = renderUpdateBanner(status);
    if (!banner) return;
    const app = document.getElementById('app');
    app.parentNode.insertBefore(banner, app);
  } catch (err) {
    console.warn('[updater] CheckForUpdate unavailable:', err);
  }
}

window.addEventListener('DOMContentLoaded', checkForUpdate);

function applyProgress(data) {
  const p = data || {};
  let key, label;
  if (p.phase === 'sysinfo_start' || p.phase === 'sysinfo_done') {
    key = 'sysinfo';
    label = 'Collecting system info';
  } else if (p.phase === 'test') {
    key = `${p.pop}:${p.test}`;
    label = `${p.pop} · ${p.test}`;
  } else {
    return; // unknown phase
  }

  const existing = state.progress.get(key) || { label, state: 'start', summary: '', err: '' };
  existing.label = label;
  if (p.phase === 'sysinfo_done') {
    existing.state = 'done';
  } else if (p.state) {
    existing.state = p.state; // "start" | "done" | "failed"
    existing.summary = p.summary || '';
    existing.err = p.err || '';
  } else if (p.phase === 'sysinfo_start') {
    existing.state = 'start';
  }
  state.progress.set(key, existing);
}

if (window.runtime && window.runtime.EventsOn) {
  window.runtime.EventsOn('progress', (data) => {
    applyProgress(data);
    if (window.location.hash === '#/running') renderCurrent();
  });
  window.runtime.EventsOn('complete', (data) => {
    state.completion = data || {};
    if (state.completion.uploaded) {
      state.sendState = 'sent';
      state.uploadInfo = {
        report_id: state.completion.report_id,
        received_at: state.completion.received_at,
      };
      state.sendError = null;
    } else {
      state.sendState = 'failed';
      state.uploadInfo = null;
      state.sendError = state.completion.error || '';
    }
    navigate('#/result');
  });
}
