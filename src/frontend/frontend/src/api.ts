import { API_BASE_URL } from './config';

export interface UserPermissions {
  upload: boolean;
  edit: boolean;
  delete: boolean;
}

export interface AuthUser {
  username: string;
  admin: boolean;
  permissions: UserPermissions;
}

let csrfToken = '';
let sessionToken = '';

export function setCSRFToken(token: string) {
  csrfToken = token;
}

export function getSessionToken(): string {
  return sessionToken;
}

export function setSessionToken(token: string) {
  sessionToken = token;
}

export async function apiFetch(
  input: RequestInfo | URL,
  init: RequestInit = {}
): Promise<Response> {
  const method = (init.method || 'GET').toUpperCase();
  const headers = new Headers(init.headers || {});

  if (sessionToken) {
    headers.set('Authorization', `Bearer ${sessionToken}`);
  }

  if (!sessionToken && !['GET', 'HEAD', 'OPTIONS'].includes(method) && csrfToken) {
    headers.set('X-CSRF-Token', csrfToken);
  }

  const response = await fetch(input, {
    ...init,
    headers,
    credentials: 'include'
  });

  const requestURL =
    typeof input === 'string'
      ? input
      : input instanceof URL
        ? input.toString()
        : input.url;

  if (
    response.status === 401 &&
    !requestURL.includes('/auth/login') &&
    !requestURL.includes('/auth/me')
  ) {
    window.dispatchEvent(new Event('scriptorium:unauthorized'));
  }

  return response;
}

export async function getCurrentUser(): Promise<AuthUser | null> {
  try {
    const response = await apiFetch(`${API_BASE_URL}/auth/me`);

    if (!response.ok) {
      return null;
    }

    const result = await response.json();

    setCSRFToken(result.csrf_token || '');

    if (result.authenticated !== true || !result.user) {
      return null;
    }

    const user = result.user;
    const admin = user.admin === true;

    return {
      username: user.username,
      admin,
      permissions: {
        upload: admin || user.permissions?.upload === true,
        edit: admin || user.permissions?.edit === true,
        delete: admin || user.permissions?.delete === true
      }
    };
  } catch {
    return null;
  }
}

export async function checkAuthentication(): Promise<boolean> {
  return (await getCurrentUser()) !== null;
}
