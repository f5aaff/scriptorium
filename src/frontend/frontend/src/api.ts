import { API_BASE_URL } from './config';

let csrfToken = '';
let sessionToken = '';

export function setCSRFToken(token: string) {
  csrfToken = token;
}

export function setSessionToken(token: string) {
  sessionToken = token;
}

export async function apiFetch(input: RequestInfo | URL, init: RequestInit = {}): Promise<Response> {
  const method = (init.method || 'GET').toUpperCase();
  const headers = new Headers(init.headers || {});
  if (sessionToken) headers.set('Authorization', `Bearer ${sessionToken}`);
  if (!sessionToken && !['GET', 'HEAD', 'OPTIONS'].includes(method) && csrfToken) {
    headers.set('X-CSRF-Token', csrfToken);
  }
  const response = await fetch(input, {
    ...init,
    headers,
    credentials: 'include'
  });
  const requestURL = typeof input === 'string' ? input : input instanceof URL ? input.toString() : input.url;
  if (response.status === 401 && !requestURL.includes('/auth/login') && !requestURL.includes('/auth/me')) {
    window.dispatchEvent(new Event('scriptorium:unauthorized'));
  }
  return response;
}

export async function checkAuthentication(): Promise<boolean> {
  try {
    const response = await apiFetch(`${API_BASE_URL}/auth/me`);
    if (!response.ok) return false;
    const result = await response.json();
    setCSRFToken(result.csrf_token || '');
    return result.authenticated === true;
  } catch {
    return false;
  }
}
