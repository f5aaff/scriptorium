import { describe, expect, it } from 'vitest';
import { parseSearch, searchParams } from './librarySearch';

describe('catalogue search parameters', () => {
  it.each([
    [' author: Ursula Le Guin ', 'Author', 'Ursula Le Guin'],
    ['TYPE: Book', 'DocType', 'Book'],
    ['filetype: .pdf', 'FileType', '.pdf'],
    ['dewey: 800', 'DeweyDecimal', '800']
  ])('preserves %s on subsequent pages', (input, key, value) => {
    const criteria = parseSearch(input);
    for (const page of [1, 2, 3]) {
      expect(Object.fromEntries(searchParams(criteria, page))).toEqual({
        page: String(page), limit: '20', key, value
      });
    }
  });

  it('preserves free-text searches on subsequent pages', () => {
    const criteria = parseSearch('  architecture & design  ');
    expect(searchParams(criteria, 2).get('q')).toBe('architecture & design');
    expect(new URLSearchParams(searchParams(criteria, 2).toString()).get('q'))
      .toBe('architecture & design');
  });

  it('leaves unknown prefixes as searchable text', () => {
    expect(parseSearch('title: Dune')).toEqual({ query: 'title: Dune' });
  });

  it('clears filters for a blank search', () => {
    expect(Object.fromEntries(searchParams(parseSearch('  '), 1)))
      .toEqual({ page: '1', limit: '20' });
  });
});
