# Scriptorium security model

## Intended threat model

Treat the HTTP API as reachable by untrusted clients, including clients coming
from the public internet. A LAN address is not an authentication boundary.
Assume users may upload malformed or deliberately hostile files, and assume
that the host's filesystem must not be exposed through request-controlled paths.

The current application is a **single-owner library**. It does not yet provide
multiple accounts, roles, sharing permissions, or per-document authorization.

## Controls currently implemented

- A first-run owner account is mandatory; the service fails closed if no
  credential file exists and bootstrap credentials are absent.
- Passwords are stored as bcrypt hashes (cost 12), not plaintext. Passwords
  must contain at least 12 Unicode characters and no more than 72 UTF-8 bytes.
- Credential files are regular files, not symlinks, and are forced to mode 0600.
  Updates use a temporary file and atomic rename.
- Login failures are rate-limited by source address (five failures per 15
  minutes). Trusted proxy addresses are opt-in and must be configured exactly.
- Sessions use cryptographically random tokens, expire after 12 hours, are
  memory-only, and are invalidated on password change or process restart.
- Browser sessions use HttpOnly, Secure-by-default, SameSite=Strict cookies and
  CSRF validation for state-changing requests.
- The Wails client can use an explicit bearer session held in memory. It is not
  written to local storage.
- All `/data` and `/file` routes require authentication. The only public
  application endpoints are login, health, and static frontend assets.
- CORS is disabled unless exact origins are configured. Wildcard origins are
  rejected by configuration validation.
- Security response headers include CSP, frame denial, MIME sniffing
  protection, restrictive permissions policy, and no-store on API responses.
- The HTTP server has bounded header/read/write timeouts and a maximum header
  size. Upload bodies are limited separately.
- File storage uses generated storage keys and validates local paths, rejecting
  traversal and symlink components.

## PDF extraction boundary

PDF text extraction invokes the optional Poppler `pdftotext` executable directly,
without a shell or request-controlled command flags. Sources are copied to private
temporary files, removed on completion or failure, and limited to 100 MiB. Captured
text is limited to 1 MiB, extraction (including staging and queueing) times out after
30 seconds, and at most two PDF subprocesses run at once. Parent cancellation or
the output limit terminates the extractor. Tool stderr is discarded rather than
returned to clients or accumulated without a bound.

These controls do **not** sandbox Poppler or impose a hard memory limit on the
child process. Malicious PDF parser inputs remain a risk. Run a maintained Poppler
version under a dedicated unprivileged account, with host/container resource
limits and filesystem/network isolation appropriate to the deployment. Do not
interpret a successful extraction as a malware scan or a guarantee that the PDF
is safe to open. OCR and PDF password handling are not implemented.

Existing-item re-extraction requires `edit` permission and accepts only a catalogue
UUID, never a client-selected server path. It reads the original through the
storage-root-confined abstraction and checks its SHA-256 digest when one exists.
Legacy records without digests are permitted, without an ingestion-identity
backfill. At most two reprocessing requests (including source staging) are active;
extra requests fail fast. Attempts have a 100 MiB source limit and a one-minute
outer deadline in addition to PDF limits. Temporary staging files are private and
removed when an attempt finishes.

Extraction/index updates commit together and use a metadata snapshot check to
reject concurrent edits or deletion. Confirmed classifications and editable
metadata are preserved. Failed attempts do not clear previously indexed text.
A successful empty extraction does clear old text. Cancellation is best-effort:
it does not undo a transaction that committed before the cancellation arrived.

## Recommended internet-facing deployment

Do not expose the Scriptorium HTTP listener directly to the internet. Keep it
bound to loopback and place a maintained TLS reverse proxy in front of it. For
example, with Caddy running on the same host:

```caddyfile
library.example.com {
    encode zstd gzip
    header Strict-Transport-Security "max-age=31536000"
    reverse_proxy 127.0.0.1:8080
}
```

Use a real DNS name and a valid certificate. The firewall should expose only
80/443 as required by the proxy and should not expose port 8080. Keep
`COOKIE_SECURE=true`. For the same-host Caddy example, configure:

```dotenv
REST_HOST=127.0.0.1
REST_PORT=8080
COOKIE_SECURE=true
CORS_ALLOWED_ORIGINS=
TRUSTED_PROXIES=127.0.0.1
```

The reverse proxy must discard client-supplied forwarding headers and set its
own. If the proxy runs in another container or host, set `TRUSTED_PROXIES` to
only its actual IP/CIDR. Never trust all addresses. Do not add a CORS origin for
a same-origin browser deployment.

For a Wails desktop client pointed at a separately hosted API, configure the
exact Wails webview origin in `CORS_ALLOWED_ORIGINS`. The desktop client uses a
12-hour bearer token in memory; do not persist it in browser storage.

## Bootstrap and account recovery

On first startup, set `AUTH_USERNAME` and `AUTH_PASSWORD` through a protected
service environment or secret manager. Do not commit `.env` files or place
credentials in shell history. Once `AUTH_FILE` exists, bootstrap environment
values are ignored. Change the initial password in Settings after signing in.

If the owner password is lost, stop Scriptorium, preserve a backup of the
credential file, remove the active authentication file, and restart with new
bootstrap credentials. This operation requires filesystem access to the host.

Back up the database, credential file, and storage directory together. Keep
backups encrypted and test restoration.

## Known gaps — do not treat this as a completed security audit

- No multi-user accounts, roles, invitations, or per-item access control.
- No MFA or external identity provider.
- Sessions are in-memory and are lost on restart; there is no persistent
  session revocation/audit history.
- Login throttling is in-process and per source address. A reverse proxy should
  also enforce request and connection limits.
- No audit log, alerting, or intrusion detection.
- No malware scanning or sandbox for uploaded files and conversion tools. Background imports are bounded by per-file and per-batch limits; server-side directory imports are restricted to configured roots and reject traversal, but filesystem race-hardening and hostile-file sandboxing still require review.
  Document converters such as Pandoc must be treated as parsers of untrusted
  input and should be isolated before public deployment.
- No formal backup/restore workflow or encrypted-at-rest storage yet.
- No automated dependency vulnerability gate or external penetration test.
- The health endpoint is intentionally unauthenticated and currently reports
  process health only.

Before public exposure, complete the gaps above that are relevant to the
intended deployment, review the full upload/conversion path, and run dependency,
static-analysis, integration, and penetration testing.
