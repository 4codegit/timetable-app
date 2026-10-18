import './style.css'
import { mount } from 'svelte'
import App from './App.svelte'

function showError(msg: string) {
  let el = document.getElementById('opencode-error');
  if (!el) {
    el = document.createElement('div');
    el.id = 'opencode-error';
    el.style.cssText = 'position:fixed;left:0;right:0;bottom:0;z-index:99999;background:#7f1d1d;color:#fff;padding:8px 12px;font:12px/1.4 ui-monospace,monospace;white-space:pre-wrap;max-height:40vh;overflow:auto';
    document.body.appendChild(el);
  }
  el.textContent = 'Error: ' + msg;
}

window.addEventListener('error', (e: any) => {
  showError(e.error ? e.error.message : e.message);
});
window.addEventListener('unhandledrejection', (e: any) => {
  const r = e.reason;
  showError(r ? (r.message || String(r)) : 'unknown');
});

let app: any;
try {
  app = mount(App, { target: document.getElementById('app') as HTMLElement });
} catch (e: any) {
  showError(e && e.message ? e.message : String(e));
}

export default app
