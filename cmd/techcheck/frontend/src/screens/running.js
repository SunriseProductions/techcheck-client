import { Cancel } from '../../wailsjs/go/wizard/App.js';

export function renderRunning(root, { state }) {
  const rows = [...state.progress.entries()].map(([, v]) => renderRow(v)).join('');
  root.innerHTML = `
    <h1>Running tests</h1>
    <p>This takes 3 minutes. You can cancel at any time.</p>
    <div id="rows" class="progress-rows">${rows}</div>
    <div class="actions">
      <button class="secondary" id="cancel">Cancel</button>
    </div>
  `;
  root.querySelector('#cancel').addEventListener('click', () => {
    if (Cancel) Cancel();
  });
  // Auto-scroll to the newest row.
  const container = root.querySelector('#rows');
  if (container) container.scrollTop = container.scrollHeight;
}

function renderRow(row) {
  const { label, state, summary, err } = row;
  let klass = 'progress-row';
  let marker = '…';
  if (state === 'done') { klass += ' done'; marker = '✓'; }
  else if (state === 'failed') { klass += ' failed'; marker = '✗'; }

  const value = state === 'failed' ? err : summary;
  const valueHTML = value ? `<div class="value">${escapeHTML(value)}</div>` : '';

  return `
    <div class="${klass}">
      <div class="marker">${marker}</div>
      <div class="label">${escapeHTML(label)}</div>
      ${valueHTML}
    </div>
  `;
}

function escapeHTML(s) {
  return String(s || '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
}
