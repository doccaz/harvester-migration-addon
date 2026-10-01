// Wraps fetch for the backend API: serves from a sub-path (service proxy) and,
// when the backend runs with USER_AUTH=token, attaches the user's own token.
//
// The backend answers 401 when it needs a token. We then mint a short-lived
// Rancher API token with the user's browser session (same origin as Rancher when
// served through the proxy) and retry once. In USER_AUTH=serviceaccount mode the
// backend never answers 401, so this code stays idle.

export const TOKEN_HEADER = 'X-Migration-Token';
const STORAGE_KEY = 'migration-ui-token';
// Rancher token TTL is in milliseconds. NOTE: not yet verified against the lab;
// see docs/identity.md ("Token minting").
export const TOKEN_TTL_MS = 60 * 60 * 1000;
const EXPIRY_MARGIN_MS = 60 * 1000;
// After a failed attempt to obtain a working token, do not try again for this
// long. The page polls every few seconds, and every attempt creates a token in
// Rancher, so retrying on each poll would flood it.
export const MINT_BACKOFF_MS = 5 * 60 * 1000;

const isApiCall = (input) => typeof input === 'string' && input.startsWith('/api/');

function readCookie(doc, name) {
  const match = (doc.cookie || '').split('; ').find((c) => c.startsWith(`${name}=`));
  return match ? decodeURIComponent(match.slice(name.length + 1)) : '';
}

export function createApiFetch(originalFetch, { apiBase = '', storage, doc = document, now = Date.now } = {}) {
  let minting = null;

  const load = () => {
    try {
      const raw = storage && storage.getItem(STORAGE_KEY);
      if (!raw) return null;
      const { token, expiresAt } = JSON.parse(raw);
      return token && expiresAt - EXPIRY_MARGIN_MS > now() ? token : null;
    } catch (e) {
      return null;
    }
  };
  const save = (token) => {
    try {
      if (storage) storage.setItem(STORAGE_KEY, JSON.stringify({ token, expiresAt: now() + TOKEN_TTL_MS }));
    } catch (e) { /* storage unavailable: token just isn't cached */ }
  };
  const clear = () => {
    try { if (storage) storage.removeItem(STORAGE_KEY); } catch (e) { /* ignore */ }
  };

  const csrfHeaders = () => ({ 'Content-Type': 'application/json', 'X-Api-Csrf': readCookie(doc, 'CSRF') });

  // Best effort: do not leave a token behind that we cannot use.
  const discard = (id) => {
    if (!id) return;
    originalFetch(`/v3/tokens/${encodeURIComponent(id)}`, {
      method: 'DELETE', credentials: 'same-origin', headers: csrfHeaders(),
    }).catch(() => {});
  };

  const mint = () => {
    if (!minting) {
      minting = originalFetch('/v3/tokens', {
        method: 'POST',
        credentials: 'same-origin',
        headers: csrfHeaders(),
        body: JSON.stringify({ type: 'token', description: 'Harvester migration UI', ttl: TOKEN_TTL_MS }),
      })
        .then((r) => (r.ok ? r.json() : Promise.reject(new Error(`token request failed: HTTP ${r.status}`))))
        .then((body) => {
          if (!body || !body.token) {
            discard(body && body.id);
            // Field names only: never log values.
            throw new Error(`token response had no usable token (fields: ${Object.keys(body || {}).join(', ')})`);
          }
          save(body.token);
          return body.token;
        })
        .finally(() => { minting = null; });
    }
    return minting;
  };

  const send = (input, init, token) => {
    const headers = new Headers((init && init.headers) || {});
    if (token) headers.set(TOKEN_HEADER, token);
    return originalFetch(apiBase + input, { ...init, headers });
  };

  let blockedUntil = 0;
  const giveUp = (res, reason) => {
    blockedUntil = now() + MINT_BACKOFF_MS;
    // eslint-disable-next-line no-console
    console.warn(`[migration-ui] not authenticated: ${reason}. Not retrying for ${MINT_BACKOFF_MS / 60000} min.`);
    return res;
  };

  return async (input, init) => {
    if (!isApiCall(input)) return originalFetch(input, init);
    const res = await send(input, init, load());
    if (res.status !== 401) return res;
    clear();
    if (now() < blockedUntil) return res;
    let token;
    try {
      token = await mint();
    } catch (e) {
      return giveUp(res, e.message);
    }
    const retry = await send(input, init, token);
    if (retry.status === 401) {
      clear();
      return giveUp(retry, 'the backend rejected a freshly minted token');
    }
    return retry;
  };
}
