/**
 * UpdateBanner — renders one of:
 *   - null (no update, supported) → hidden
 *   - yellow "update available" with Download + See what's new
 *   - red "update required" (IsSupported=false) with Download
 *   - red modal (MandatoryUpdate=true) blocking progress
 *
 * Consumes an updater.Status from the Go side.
 */
import { renderReleaseNotesModal } from './releaseNotesModal.js';
import { BrowserOpenURL } from '../../wailsjs/runtime/runtime.js';

export function renderUpdateBanner(status, opts = {}) {
  if (!status || !status.OK) return null;

  if (status.MandatoryUpdate && status.UpdateAvailable) {
    return renderModal(status, opts);
  }

  if (!status.IsSupported) {
    return renderBar({
      level: 'error',
      message: `Update required — this version can no longer upload reports. Please install ${status.Latest}.`,
      status,
      opts,
      includeReleaseNotes: false,
    });
  }

  if (status.UpdateAvailable) {
    return renderBar({
      level: 'info',
      message: `Version ${status.Latest} available.`,
      status,
      opts,
      includeReleaseNotes: true,
    });
  }

  return null;
}

function renderBar({ level, message, status, opts, includeReleaseNotes }) {
  const bar = document.createElement('div');
  bar.className = `update-banner update-banner-${level}`;
  bar.setAttribute('role', 'status');

  const text = document.createElement('span');
  text.textContent = message;
  bar.appendChild(text);

  const actions = document.createElement('span');
  actions.className = 'update-banner-actions';

  if (includeReleaseNotes && status.ReleaseNotesHTML) {
    const notes = document.createElement('button');
    notes.type = 'button';
    notes.className = 'update-banner-link';
    notes.textContent = ‘See what’s new’;
    notes.addEventListener('click', () => {
      document.body.appendChild(renderReleaseNotesModal(status.ReleaseNotesHTML));
    });
    actions.appendChild(notes);
  }

  actions.appendChild(buildUpdateControl(status, opts));

  bar.appendChild(actions);
  return bar;
}

function renderModal(status, opts = {}) {
  const modal = document.createElement('div');
  modal.className = 'update-banner-modal';
  modal.setAttribute('role', 'dialog');
  modal.setAttribute('aria-modal', 'true');

  const inner = document.createElement('div');
  inner.className = 'update-banner-modal-inner';

  // Header — fixed, never scrolls.
  const header = document.createElement('div');
  header.className = 'update-banner-modal-header';
  const h = document.createElement('h2');
  h.textContent = 'Update required';
  const p = document.createElement('p');
  p.textContent = `Version ${status.Latest} is required to continue.`;
  header.appendChild(h);
  header.appendChild(p);
  inner.appendChild(header);

  // Body — scrollable. Release notes go here.
  const body = document.createElement('div');
  body.className = 'update-banner-modal-body';
  if (status.ReleaseNotesHTML) {
    body.innerHTML = status.ReleaseNotesHTML; // server-sanitised
  } else {
    const placeholder = document.createElement('p');
    placeholder.textContent = 'No release notes available.';
    body.appendChild(placeholder);
  }
  inner.appendChild(body);

  // Footer — fixed, always shows update control.
  const footer = document.createElement('div');
  footer.className = 'update-banner-modal-footer';
  footer.appendChild(buildUpdateControl(status, opts));
  inner.appendChild(footer);

  modal.appendChild(inner);
  return modal;
}

// buildUpdateControl returns the browser-download button when self-update is
// unavailable, otherwise the in-app Install Update control with progress.
function buildUpdateControl(status, opts) {
  const canSelf = !!opts.canSelfUpdate;
  if (!canSelf) {
    const dl = document.createElement('button');
    dl.type = 'button';
    dl.className = 'update-banner-download';
    dl.textContent = 'Download';
    dl.addEventListener('click', () => {
      const url = status.DownloadURL || '';
      if (/^https?:\/\//i.test(url)) BrowserOpenURL(url);
    });
    return dl;
  }

  const wrap = document.createElement('span');
  wrap.className = 'update-banner-install';

  const btn = document.createElement('button');
  btn.type = 'button';
  btn.className = 'update-banner-download';
  btn.textContent = 'Install Update';

  const progress = document.createElement('span');
  progress.className = 'update-banner-progress';

  let phase = 'idle'; // idle → downloading → ready → installing

  const fns = opts.updateFns || {};

  btn.addEventListener('click', async () => {
    if (phase === 'idle') {
      phase = 'downloading';
      btn.disabled = true;
      progress.textContent = 'Downloading… 0%';
      try {
        await fns.download();
      } catch (err) {
        phase = 'idle';
        btn.disabled = false;
        progress.textContent = `Error: ${err?.message ?? err}`;
      }
      return;
    }
    if (phase === 'ready') {
      phase = 'installing';
      btn.disabled = true;
      progress.textContent = 'Quitting to install…';
      try {
        await fns.apply();
      } catch (err) {
        phase = 'ready';
        btn.disabled = false;
        progress.textContent = `Error: ${err?.message ?? err}`;
      }
    }
  });

  // Event hooks driven by main.js.
  fns.onProgress = (done, total) => {
    if (phase !== 'downloading') return;
    const pct = total > 0 ? Math.floor((done / total) * 100) : 0;
    progress.textContent = `Downloading… ${pct}%`;
  };
  fns.onReady = () => {
    phase = 'ready';
    btn.disabled = false;
    btn.textContent = 'Update now';
    progress.textContent = 'Verified — ready to install.';
  };
  fns.onError = (msg) => {
    phase = 'idle';
    btn.disabled = false;
    btn.textContent = 'Install Update';
    progress.textContent = `Error: ${msg}`;
  };
  opts.updateFns = fns;

  wrap.appendChild(btn);
  wrap.appendChild(progress);
  return wrap;
}
