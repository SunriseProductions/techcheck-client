export function renderDone(root) {
  root.innerHTML = `
    <h1>Done</h1>
    <p>Tech Check has finished. You can close this window.</p>
    <div class="actions">
      <button class="primary" id="close">Close</button>
    </div>
  `;
  root.querySelector('#close').addEventListener('click', () => {
    if (window.runtime && window.runtime.Quit) window.runtime.Quit();
  });
}
