import type { Rect, Size } from "./region-content";

/**
 * Canvas-sized PNG whose alpha is opaque exactly over `rects` and transparent
 * everywhere else — the mask OBS's Image Mask/Blend filter consumes, since that
 * filter takes a local file path rather than a URL.
 *
 * Filled white rather than black so the same file also works where a mask is read
 * as luminance or via a color channel.
 */
export async function renderMaskPNG(
  rects: readonly Rect[],
  canvas: Size,
): Promise<Blob> {
  const el = document.createElement("canvas");
  el.width = Math.round(canvas.w);
  el.height = Math.round(canvas.h);

  const ctx = el.getContext("2d");
  if (!ctx) throw new Error("2D canvas context unavailable");

  // Written as raw RGBA rather than fillRect so the rect edges stay hard — an
  // antialiased edge would bleed partial alpha into the mask.
  const image = ctx.createImageData(el.width, el.height);
  for (const r of rects) {
    const x0 = Math.max(0, Math.round(r.x));
    const y0 = Math.max(0, Math.round(r.y));
    const x1 = Math.min(el.width, Math.round(r.x + r.w));
    const y1 = Math.min(el.height, Math.round(r.y + r.h));
    for (let y = y0; y < y1; y++) {
      const start = (y * el.width + x0) * 4;
      image.data.fill(255, start, start + (x1 - x0) * 4);
    }
  }
  ctx.putImageData(image, 0, 0);

  return new Promise<Blob>((resolve, reject) => {
    el.toBlob((blob) => {
      if (blob) resolve(blob);
      else reject(new Error("PNG encoding failed"));
    }, "image/png");
  });
}
