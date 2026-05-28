import { SendReport, LastUploadInfo } from '../../wailsjs/go/wizard/App.js';

export function renderResult(root, { navigate, state }) {
  const sendState = state.sendState || 'failed';
  if (sendState === 'sent') {
    renderSent(root, state, navigate);
  } else {
    renderFailed(root, state, navigate);
  }
}

function renderSent(root, state, navigate) {
  const c = state.completion || {};
  const info = state.uploadInfo || {};
  root.innerHTML = `
    <h1>Sent — thanks</h1>
    <p>IT will flag anything that needs attention. You can close this window.</p>
    <details>
      <summary>Show details</summary>
      <pre id="detail"></pre>
    </details>
    <div class="actions">
      <button class="primary" id="done">Close</button>
    </div>
  `;
  root.querySelector('#detail').textContent = JSON.stringify({
    report_id:   info.report_id,
    received_at: info.received_at,
    local_path:  c.local_path,
  }, null, 2);
  root.querySelector('#done').addEventListener('click', () => navigate('#/done'));
}

function renderFailed(root, state, navigate) {
  const c = state.completion || {};
  root.innerHTML = `
    <h1>Couldn't reach Sunrise</h1>
    <p>Your report is saved here:</p>
    <pre>${escapeHTML(c.local_path || '')}</pre>
    <p>You can try again, or email the file to <strong>${escapeHTML(c.it_email || '')}</strong>.</p>
    <p class="field-error">${escapeHTML(state.sendError || '')}</p>
    <div class="actions">
      <button class="secondary" id="close">Close</button>
      <button class="primary" id="retry">Try again</button>
    </div>
  `;

  root.querySelector('#close').addEventListener('click', () => navigate('#/done'));

  const retry = root.querySelector('#retry');
  retry.addEventListener('click', async () => {
    retry.disabled = true;
    retry.textContent = 'Sending…';
    try {
      await SendReport();
      const info = await LastUploadInfo();
      state.sendState = 'sent';
      state.uploadInfo = info;
      navigate('#/result');
    } catch (e) {
      retry.disabled = false;
      retry.textContent = 'Try again';
      state.sendError = String(e);
      renderFailed(root, state, navigate);
    }
  });
}

function escapeHTML(s) {
  return String(s || '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
}
