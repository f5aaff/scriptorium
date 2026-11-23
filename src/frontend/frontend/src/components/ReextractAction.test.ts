// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import ReextractAction from './ReextractAction.svelte';
import { reextractText } from '../reextract';
import type { LibraryItem } from '../libraryTypes';

vi.mock('../reextract', () => ({ reextractText: vi.fn() }));
beforeEach(() => vi.mocked(reextractText).mockReset());
afterEach(cleanup);

const metadata: LibraryItem = {
  Uuid: 'doc-1', Title: 'User title', Author: 'User author', PublishDate: '', LastUpdated: '',
  FileType: '.pdf', DocType: 'Book', DeweyDecimal: '900', Path: 'original-key', ExtractionStatus: 'complete'
};

it('prevents duplicate submissions and emits refreshed metadata', async () => {
  let resolve!: (value: LibraryItem) => void;
  vi.mocked(reextractText).mockReturnValue(new Promise(res => { resolve = res; }));
  const { component } = render(ReextractAction, { documentID: 'doc-1' });
  const updated = vi.fn();
  component.$on('updated', updated);
  await fireEvent.click(screen.getByRole('button', { name: 'Re-extract text' }));
  const busyButton = screen.getByRole('button', { name: 'Re-extracting text…' }) as HTMLButtonElement;
  expect(busyButton.disabled).toBe(true);
  await fireEvent.click(busyButton);
  expect(reextractText).toHaveBeenCalledTimes(1);
  await act(async () => { resolve(metadata); });
  expect(updated.mock.calls[0][0].detail).toEqual(metadata);
  expect(screen.getByText('Text extraction and search index refreshed.')).toBeTruthy();
});

it('shows failures and lets the user retry', async () => {
  vi.mocked(reextractText).mockRejectedValueOnce(new Error('Extractor unavailable; previous text retained.'))
    .mockResolvedValueOnce(metadata);
  render(ReextractAction, { documentID: 'doc-1' });
  await fireEvent.click(screen.getByRole('button', { name: 'Re-extract text' }));
  expect(await screen.findByRole('alert')).toBeTruthy();
  expect(screen.getByText('Extractor unavailable; previous text retained.')).toBeTruthy();
  await fireEvent.click(screen.getByRole('button', { name: 'Re-extract text' }));
  expect(await screen.findByText('Text extraction and search index refreshed.')).toBeTruthy();
  expect(screen.queryByRole('alert')).toBeNull();
});

it('aborts work and ignores a late result after navigation', async () => {
  let resolve!: (value: LibraryItem) => void;
  vi.mocked(reextractText).mockReturnValue(new Promise(res => { resolve = res; }));
  const { component, unmount } = render(ReextractAction, { documentID: 'doc-1' });
  const updated = vi.fn();
  component.$on('updated', updated);
  await fireEvent.click(screen.getByRole('button', { name: 'Re-extract text' }));
  unmount();
  expect(vi.mocked(reextractText).mock.calls[0][1].aborted).toBe(true);
  await act(async () => { resolve(metadata); });
  expect(updated).not.toHaveBeenCalled();
});
