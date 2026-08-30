// The async Clipboard API is unavailable in insecure contexts (plain HTTP on a
// LAN host), so fall back to a hidden-textarea execCommand copy there.
export async function copyText(text: string): Promise<void> {
  if (navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(text);
    return;
  }
  const ta = document.createElement('textarea');
  ta.value = text;
  ta.style.position = 'fixed';
  ta.style.top = '0';
  ta.style.left = '0';
  ta.style.opacity = '0';
  ta.style.pointerEvents = 'none';
  document.body.appendChild(ta);
  ta.focus();
  ta.select();
  try {
    // eslint-disable-next-line sonarjs/deprecation -- the only copy path in insecure (LAN HTTP) contexts; Clipboard API is unavailable there
    if (!document.execCommand('copy')) throw new Error('copy command rejected');
  } finally {
    ta.remove();
  }
}
