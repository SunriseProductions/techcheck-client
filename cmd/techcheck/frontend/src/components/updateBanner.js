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

export function renderUpdateBanner(status) {
  if (!status || !status.OK) return null;

  if (status.MandatoryUpdate && status.UpdateAvailable) {
    return renderModal(status);
  }

  if (!status.IsSupported) {
    return renderBar({
      level: 'error',
      message: `Update required — this version can no longer upload reports. Please install ${status.Latest}.`,
      status,
      includeReleaseNotes: false,
    });
  }

  if (status.UpdateAvailable) {
    return renderBar({
      level: 'info',
      message: `Version ${status.Latest} available.`,
      status,
      includeReleaseNotes: true,
    });
  }

  return null;
}

function renderBar({ level, message, status, includeReleaseNotes }) {
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
    notes.textContent = 'See what\u2019s new';
    notes.addEventListener('click', () => {
      document.body.appendChild(renderReleaseNotesModal(status.ReleaseNotesHTML));
    });
    actions.appendChild(notes);
  }

  const dl = document.createElement('button');
  dl.type = 'button';
  dl.className = 'update-banner-download';
  dl.textContent = 'Download';
  dl.addEventListener('click', () => {
    const url = status.DownloadURL || '';
    if (/^https?:\/\//i.test(url)) BrowserOpenURL(url);
  });
  actions.appendChild(dl);

  bar.appendChild(actions);
  return bar;
}

function renderModal(status) {
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

  // Footer — fixed, always shows Download.
  const footer = document.createElement('div');
  footer.className = 'update-banner-modal-footer';
  const dl = document.createElement('button');
  dl.type = 'button';
  dl.className = 'update-banner-download';
  dl.textContent = 'Download';
  dl.addEventListener('click', () => {
    const url = status.DownloadURL || '';
    if (/^https?:\/\//i.test(url)) BrowserOpenURL(url);
  });
  footer.appendChild(dl);
  inner.appendChild(footer);

  modal.appendChild(inner);
  return modal;
}
