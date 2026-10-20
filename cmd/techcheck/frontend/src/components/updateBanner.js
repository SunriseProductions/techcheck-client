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
      message: `Version ${status.Latest} is available.`,
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
  text.className = 'update-banner-message';
  text.textContent = message;
  bar.appendChild(text);

  const actions = document.createElement('span');
  actions.className = 'update-banner-actions';

  if (includeReleaseNotes && status.ReleaseNotesHTML) {
    const notes = document.createElement('button');
    notes.type = 'button';
    notes.className = 'update-banner-link';
    notes.textContent = 'See what’s new';
    notes.addEventListener('click', () => {
      document.body.appendChild(renderReleaseNotesModal(status.ReleaseNotesHTML));
    });
    actions.appendChild(notes);
  }

  actions.appendChild(buildUpdateControl(status, opts, { bar, message: text }));

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

const BUTTON_LABELS = {
  idle: 'Download update',
  downloading: 'Downloading…',
  ready: 'Restart to update',
  installing: 'Restarting…',
  error: 'Try again',
};

// buildUpdateControl returns the browser-download button when self-update is
// unavailable, otherwise the in-app download → restart control.
//
// In the bar, `ui.bar` and `ui.message` are passed in: each phase rewrites the
// bar's own message, sets data-phase (which drives the colour), and fills a
// progress track along the bar's bottom edge. In the modal there is no bar, so
// the message sits next to the button instead.
function buildUpdateControl(status, opts, ui = {}) {
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
  wrap.appendChild(btn);

  let message = ui.message;
  if (!message) {
    message = document.createElement('span');
    message.className = 'update-banner-progress';
    wrap.prepend(message);
  }

  let fill = null;
  if (ui.bar) {
    const track = document.createElement('div');
    track.className = 'update-banner-track';
    fill = document.createElement('div');
    fill.className = 'update-banner-fill';
    track.appendChild(fill);
    ui.bar.appendChild(track);
  }

  const version = status.Latest;
  const fns = opts.updateFns || {};

  // phase drives what a click does: idle → downloading → ready → installing.
  // A failed download lands in 'error' (click retries the download); a failed
  // restart goes back to 'ready' but shows the error styling.
  let phase = 'idle';
  const setPhase = (next, text, { look = next, pct = 0 } = {}) => {
    phase = next;
    if (ui.bar) ui.bar.dataset.phase = look;
    btn.disabled = next === 'downloading' || next === 'installing';
    btn.textContent = look === 'error' ? BUTTON_LABELS.error : BUTTON_LABELS[next];
    message.textContent = text;
    if (fill) fill.style.width = `${pct}%`;
  };
  setPhase('idle', message.textContent);

  btn.addEventListener('click', async () => {
    if (phase === 'idle' || phase === 'error') {
      setPhase('downloading', `Downloading ${version}…`);
      try {
        await fns.download();
      } catch (err) {
        fns.onError(err?.message ?? err);
      }
      return;
    }
    if (phase === 'ready') {
      setPhase('installing', `Restarting to install ${version}…`);
      try {
        await fns.apply();
      } catch (err) {
        setPhase('ready', `Couldn’t restart: ${err?.message ?? err}`, { look: 'error' });
      }
    }
  });

  // Event hooks driven by main.js.
  fns.onProgress = (done, total) => {
    if (phase !== 'downloading') return;
    if (total > 0) {
      const pct = Math.floor((done / total) * 100);
      setPhase('downloading', `Downloading ${version}… ${pct}%`, { pct });
    }
  };
  fns.onReady = () => {
    setPhase('ready', `✓ ${version} is ready. The app will restart to install it.`);
  };
  fns.onError = (msg) => {
    setPhase('error', `Download failed: ${msg}`);
  };
  opts.updateFns = fns;

  return wrap;
}
