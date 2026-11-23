// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, expect, it } from 'vitest';
import TextReader from './TextReader.svelte';
afterEach(cleanup);

it('makes the complete text readable without losing newlines or surrogate pairs at section boundaries', async () => {
  const source = ('Paragraph with books 📚\n').repeat(7000) + 'The actual ending';
  render(TextReader, { text: source });
  const area = screen.getByRole('textbox', { name: 'Document text' }) as HTMLTextAreaElement;
  let reconstructed = '';
  while (true) {
    reconstructed += area.value;
    const next = screen.getByRole('button', { name: 'Next section' }) as HTMLButtonElement;
    if (next.disabled) break;
    await fireEvent.click(next);
  }
  expect(reconstructed).toBe(source);
  expect(area.value).toContain('The actual ending');
});

it('restores a saved structural character anchor and emits new personal positions', async () => {
  const text = 'a'.repeat(70000);
  const { component } = render(TextReader, { text, initialPosition: { kind: 'text', page: 1, locator: '65536', offset: 0, scale: 1 } });
  const positions: number[] = [];
  component.$on('position', event => positions.push(event.detail.page));
  await waitFor(() => expect((screen.getByRole('spinbutton', { name: 'Section' }) as HTMLInputElement).value).toBe('3'));
  await fireEvent.click(screen.getByRole('button', { name: 'Previous section' }));
  expect(positions).toContain(2);
});
