import { expect, it } from 'vitest';
import { boundedPdfScale, MAX_PDF_CANVAS_PIXELS } from './pdfRenderer';

it('keeps normal PDF pages at the requested zoom', () => {
  expect(boundedPdfScale(612, 792, 1.5)).toBe(1.5);
});

it('caps large pages by area and maximum dimension', () => {
  for (const [width, height] of [[10000, 10000], [100000, 10], [10, 100000]]) {
    const scale = boundedPdfScale(width, height, 3);
    expect(width * height * scale * scale).toBeLessThanOrEqual(MAX_PDF_CANVAS_PIXELS + .001);
    expect(Math.max(width, height) * scale).toBeLessThanOrEqual(8192);
  }
});

it.each([[0, 100, 1], [100, -1, 1], [NaN, 100, 1], [100, Infinity, 1], [100, 100, 0], [1e308, 1e308, 1]])(
  'rejects invalid or overflowing page dimensions', (width, height, scale) => {
    expect(() => boundedPdfScale(width, height, scale)).toThrow();
  }
);
