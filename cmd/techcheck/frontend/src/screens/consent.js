export async function renderConsent(root, { navigate }) {
  let isWindows = false;
  try {
    const env = await window.runtime.Environment();
    isWindows = env.platform === 'windows';
  } catch (_) {
    // Runtime unavailable — default to hiding the Windows-only note.
  }

  const windowsNote = isWindows
    ? `<p><strong>Heads up:</strong> you'll get a firewall prompt during the network tests. Click Allow, or that test fails for no real reason.</p>`
    : '';

  root.innerHTML = `
    <h1>Before we start</h1>
    <p>Tech Check looks over your computer and its connection to Sunrise, then sends the report to IT so they can flag anything that would slow you down working remotely.</p>

    <h2>What it looks at</h2>
    <p>Your machine (OS, CPU, RAM, disk, GPU, screens, audio, battery, network, security posture) and your connection to each Sunrise region (ping, throughput, jitter, packet loss, UDP 4172, public IP).</p>

    <h2>What it doesn't</h2>
    <p>It doesn't record your webcam or mic, read your files, watch your screen, or touch your passwords. It notes what's there, not what you're doing with it.</p>

    ${windowsNote}

    <div class="actions">
      <button class="secondary" id="cancel">Cancel</button>
      <button class="primary" id="accept">Start</button>
    </div>
  `;

  root.querySelector('#cancel').addEventListener('click', () => {
    if (window.runtime && window.runtime.Quit) window.runtime.Quit();
  });
  root.querySelector('#accept').addEventListener('click', () => navigate('#/identify'));
}
