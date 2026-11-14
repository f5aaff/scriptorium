const configuredAPIBaseURL = (import.meta as any).env?.VITE_API_BASE_URL as string | undefined;

// A production browser deployment serves the UI and API from one origin.
// Wails' local application origin is not the backend origin, so use localhost there.
const browserOrigin =
  typeof window !== 'undefined' &&
  (window.location.protocol === 'http:' || window.location.protocol === 'https:') &&
  !window.location.hostname.endsWith('.localhost')
    ? window.location.origin
    : undefined;

export const API_BASE_URL =
  configuredAPIBaseURL?.replace(/\/$/, '') || browserOrigin || 'http://localhost:8080';
