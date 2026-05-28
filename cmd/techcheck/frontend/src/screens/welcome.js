export function renderWelcome(root, { navigate }) {
  root.innerHTML = `
    <img src="assets/sunrise-logo.png" alt="Sunrise Animation Studios" class="brand-logo" />
    <h1>Welcome to Tech Check</h1>
    <p>Tech Check confirms whether your machine and network are ready for remote work via Sunrise.</p>
    <p>It takes about 3 minutes. You'll be asked to accept a short privacy notice, then the tool will run a series of tests and upload the result to IT.</p>
    <div class="actions">
      <button class="primary" id="next">Next</button>
    </div>
  `;
  root.querySelector('#next').addEventListener('click', () => navigate('#/consent'));
}
