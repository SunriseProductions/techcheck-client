import { SavedIdentity, StartRun, ValidateIdentity } from '../../wailsjs/go/wizard/App.js';

export function renderIdentify(root, { navigate, state }) {
  root.innerHTML = `
    <h1>Who are you?</h1>
    <p>We use this to identify your report.</p>
    <label for="name">Full name</label>
    <input type="text" id="name" value="${escapeAttr(state.fullName)}" />
    <label for="email">Email</label>
    <input type="email" id="email" value="${escapeAttr(state.email)}" />
    <p class="hint">Close streaming, large downloads, and video calls before you continue — they'll skew the throughput numbers.</p>
    <div id="err" class="field-error" hidden></div>
    <div class="actions">
      <button class="secondary" id="back">Back</button>
      <button class="primary" id="next">Next</button>
    </div>
  `;

  // Pre-fill from saved identity if this screen is being rendered fresh.
  if (!state.fullName && !state.email && SavedIdentity) {
    SavedIdentity().then((saved) => {
      if (saved && (saved.full_name || saved.email)) {
        const nameEl = root.querySelector('#name');
        const emailEl = root.querySelector('#email');
        if (nameEl && !nameEl.value) nameEl.value = saved.full_name || '';
        if (emailEl && !emailEl.value) emailEl.value = saved.email || '';
      }
    }).catch(() => {});
  }

  const err = root.querySelector('#err');
  root.querySelector('#back').addEventListener('click', () => navigate('#/consent'));
  root.querySelector('#next').addEventListener('click', async () => {
    const name = root.querySelector('#name').value.trim();
    const email = root.querySelector('#email').value.trim();
    try {
      await ValidateIdentity(name, email);
    } catch (e) {
      err.textContent = String(e);
      err.hidden = false;
      return;
    }
    state.fullName = name;
    state.email = email;
    state.progress = new Map();
    state.completion = null;
    try {
      await StartRun(name, email);
    } catch (e) {
      err.textContent = 'Failed to start: ' + String(e);
      err.hidden = false;
      return;
    }
    navigate('#/running');
  });
}

function escapeAttr(s) {
  return String(s || '').replace(/"/g, '&quot;');
}
