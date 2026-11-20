/** SearchCriteria describes the active catalogue query, independent of its page. */
export interface SearchCriteria {
  query?: string;
  key?: string;
  value?: string;
}

/** parseSearch interprets supported field prefixes and trims surrounding whitespace. */
export function parseSearch(input: string): SearchCriteria {
  const query = input.trim();
  const match = /^(author|type|filetype|dewey):\s*(.*)$/i.exec(query);
  if (match) {
    const keys: Record<string, string> = {
      author: 'Author', type: 'DocType', filetype: 'FileType', dewey: 'DeweyDecimal'
    };
    return { key: keys[match[1].toLowerCase()], value: match[2].trim() };
  }
  return query ? { query } : {};
}

/** searchParams applies the same criteria to every page of a search. */
export function searchParams(criteria: SearchCriteria, page: number): URLSearchParams {
  const params = new URLSearchParams({ page: String(page), limit: '20' });
  if (criteria.query) {
    params.set('q', criteria.query);
  } else if (criteria.key && criteria.value) {
    params.set('key', criteria.key);
    params.set('value', criteria.value);
  }
  return params;
}
