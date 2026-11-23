import workerURL from 'pdfjs-dist/legacy/build/pdf.worker.min.mjs?url';
import type { PDFDocumentLoadingTask, PDFDocumentProxy } from 'pdfjs-dist';

export const MAX_PDF_CANVAS_PIXELS = 16 * 1024 * 1024;
const MAX_CANVAS_SIDE = 8192;
const MAX_PAGE_TEXT_CHARACTERS = 1024 * 1024;

/** PdfDocument exposes only canvas rendering and inert text, not PDF actions/scripts. */
export interface PdfDocument {
  pageCount: number;
  renderPage(canvas: HTMLCanvasElement, page: number, scale: number, signal: AbortSignal): Promise<number>;
  pageText(page: number, signal: AbortSignal): Promise<{ text: string; truncated: boolean }>;
  destroy(): Promise<void>;
}

/** boundedPdfScale limits both pixel area and the longest canvas dimension. */
export function boundedPdfScale(width: number, height: number, requested: number): number {
  if (![width, height, requested].every(value => Number.isFinite(value) && value > 0)) {
    throw new Error('The PDF page has invalid dimensions.');
  }
  const scale = Math.min(requested, Math.sqrt(MAX_PDF_CANVAS_PIXELS / (width * height)), MAX_CANVAS_SIDE / width, MAX_CANVAS_SIDE / height);
  if (!Number.isFinite(scale) || scale <= 0) throw new Error('The PDF page is too large to render.');
  return scale;
}

function checkCancelled(signal: AbortSignal) {
  if (signal.aborted) throw new DOMException('PDF viewer closed', 'AbortError');
}

/** openPdf loads local bytes into a locally bundled PDF.js worker. No URL from
 * the PDF is navigated, no scripting manager is instantiated, and XFA is disabled.
 * Parsing is limited to 30 seconds; destruction terminates the document worker.
 */
export async function openPdf(data: ArrayBuffer, signal: AbortSignal): Promise<PdfDocument> {
  checkCancelled(signal);
  const pdfjs = await import('pdfjs-dist/legacy/build/pdf.mjs');
  checkCancelled(signal);
  pdfjs.GlobalWorkerOptions.workerSrc = workerURL;
  const assets = new URL(`${import.meta.env.BASE_URL}pdfjs/`, document.baseURI).href;
  const worker = pdfjs.PDFWorker.create({ name: 'Scriptorium reader' });
  let task: PDFDocumentLoadingTask;
  try {
    task = pdfjs.getDocument({
      worker, data: new Uint8Array(data),
      cMapUrl: `${assets}cmaps/`, cMapPacked: true,
      standardFontDataUrl: `${assets}standard_fonts/`,
      wasmUrl: `${assets}wasm/`, iccUrl: `${assets}iccs/`,
      useWorkerFetch: true, useSystemFonts: false, enableXfa: false, disableWorkerRendering: true,
      maxImageSize: -1, canvasMaxAreaInBytes: MAX_PDF_CANVAS_PIXELS * 4,
      stopAtErrors: true
    });
  } catch (error) {
    worker.destroy();
    throw error;
  }
  let destruction: Promise<void> | undefined;
  let timer: ReturnType<typeof setTimeout> | undefined;
  let rejectLoading!: (reason: Error) => void;
  const interrupted = new Promise<never>((_, reject) => { rejectLoading = reject; });
  function destroy(): Promise<void> {
    if (!destruction) {
      clearTimeout(timer);
      signal.removeEventListener('abort', abort);
      // A stuck parser may never acknowledge the graceful transport shutdown.
      // Own the worker explicitly so close still terminates it after a short grace.
      destruction = Promise.resolve().then(async () => {
        let grace: ReturnType<typeof setTimeout> | undefined;
        try {
          await Promise.race([task.destroy(), new Promise<void>(resolve => { grace = setTimeout(resolve, 250); })]);
        } finally {
          clearTimeout(grace);
          worker.destroy();
        }
      });
    }
    return destruction;
  }
  function abort() {
    rejectLoading(new DOMException('PDF viewer closed', 'AbortError'));
    void destroy().catch(() => {});
  }
  timer = setTimeout(() => {
    rejectLoading(new Error('PDF loading timed out. Download the original instead.'));
    void destroy().catch(() => {});
  }, 30000);
  signal.addEventListener('abort', abort, { once: true });
  if (signal.aborted) abort();
  let pdf: PDFDocumentProxy;
  try {
    pdf = await Promise.race([task.promise, interrupted]);
    checkCancelled(signal);
    clearTimeout(timer);
  } catch (failure) {
    await destroy().catch(() => {});
    throw failure;
  }

  return {
    pageCount: pdf.numPages,
    destroy,
    async renderPage(canvas, number, requestedScale, pageSignal) {
      checkCancelled(pageSignal);
      const page = await pdf.getPage(number);
      checkCancelled(pageSignal);
      const original = page.getViewport({ scale: 1 });
      const scale = boundedPdfScale(original.width, original.height, requestedScale);
      const viewport = page.getViewport({ scale });
      canvas.width = Math.max(1, Math.floor(viewport.width));
      canvas.height = Math.max(1, Math.floor(viewport.height));
      const render = page.render({ canvas, viewport, annotationMode: pdfjs.AnnotationMode.DISABLE });
      const cancel = () => render.cancel();
      pageSignal.addEventListener('abort', cancel, { once: true });
      if (pageSignal.aborted) cancel();
      try {
        await render.promise;
        return scale;
      } finally {
        pageSignal.removeEventListener('abort', cancel);
        page.cleanup();
      }
    },
    async pageText(number, pageSignal) {
      checkCancelled(pageSignal);
      const page = await pdf.getPage(number);
      checkCancelled(pageSignal);
      const reader = page.streamTextContent().getReader();
      const cancel = () => { void reader.cancel().catch(() => {}); };
      pageSignal.addEventListener('abort', cancel, { once: true });
      let text = '';
      let truncated = false;
      try {
        while (true) {
          checkCancelled(pageSignal);
          const { value, done } = await reader.read();
          checkCancelled(pageSignal);
          if (done) break;
          for (const item of value.items) {
            if (!('str' in item)) continue;
            const part = item.str + (item.hasEOL ? '\n' : ' ');
            const remaining = MAX_PAGE_TEXT_CHARACTERS - text.length;
            if (part.length > remaining) {
              await reader.cancel();
              throw new Error('Selectable page text exceeds the text-panel limit; the rendered PDF page is not truncated.');
            }
            text += part;
          }
        }
        return { text, truncated };
      } finally {
        pageSignal.removeEventListener('abort', cancel);
        if (pageSignal.aborted) await reader.cancel().catch(() => {});
        reader.releaseLock();
        page.cleanup();
      }
    }
  };
}
