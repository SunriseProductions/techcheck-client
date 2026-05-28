/** Modal that renders pre-sanitised release-notes HTML. Header has a
 *  Close button; body is scrollable so long notes don't push Close out
 *  of reach. Called from the yellow "update available" banner's
 *  "See what's new" link; the mandatory-update modal uses its own
 *  renderer (it replaces Close with Download in the footer). */
export function renderReleaseNotesModal(html) {
  const overlay = document.createElement('div');
  overlay.className = 'release-notes-overlay';
  overlay.setAttribute('role', 'dialog');
  overlay.setAttribute('aria-modal', 'true');
  overlay.addEventListener('click', (e) => {
    if (e.target === overlay) overlay.remove();
  });

  const panel = document.createElement('div');
  panel.className = 'release-notes-panel';

  const header = document.createElement('div');
  header.className = 'release-notes-header';
  const title = document.createElement('h2');
  title.textContent = "What's new";
  const close = document.createElement('button');
  close.type = 'button';
  close.className = 'release-notes-close';
  close.textContent = 'Close';
  close.addEventListener('click', () => overlay.remove());
  header.appendChild(title);
  header.appendChild(close);

  const body = document.createElement('div');
  body.className = 'release-notes-body';
  body.innerHTML = html; // server-side sanitised by bleach; safe.

  panel.appendChild(header);
  panel.appendChild(body);
  overlay.appendChild(panel);
  return overlay;
}
