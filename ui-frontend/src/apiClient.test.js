import { createApiFetch, TOKEN_HEADER } from './apiClient';

const memStorage = () => {
  const m = {};
  return { getItem: (k) => (k in m ? m[k] : null), setItem: (k, v) => { m[k] = v; }, removeItem: (k) => { delete m[k]; } };
};
const res = (status, body) => ({ status, ok: status >= 200 && status < 300, json: async () => body });
const doc = { cookie: 'CSRF=abc123; other=1' };

test('rewrites /api paths and leaves other URLs alone', async () => {
  const f = jest.fn().mockResolvedValue(res(200, {}));
  const api = createApiFetch(f, { apiBase: '/proxy', storage: memStorage(), doc });
  await api('/api/v1/plans');
  await api('/static/x.js');
  expect(f.mock.calls[0][0]).toBe('/proxy/api/v1/plans');
  expect(f.mock.calls[1][0]).toBe('/static/x.js');
});

test('on 401 mints a token with the CSRF header, retries once, and caches it', async () => {
  const calls = [];
  const fetchFn = jest.fn((url, init) => {
    calls.push([url, init]);
    if (url === '/v3/tokens') return Promise.resolve(res(201, { token: 'token-1:secret' }));
    const has = init.headers[TOKEN_HEADER];
    return Promise.resolve(has ? res(200, {}) : res(401, {}));
  });
  const api = createApiFetch(fetchFn, { apiBase: '', storage: memStorage(), doc });
  const r1 = await api('/api/v1/plans');
  expect(r1.status).toBe(200);
  const mintCall = calls.find((c) => c[0] === '/v3/tokens');
  expect(mintCall[1].headers['X-Api-Csrf']).toBe('abc123');
  expect(mintCall[1].credentials).toBe('same-origin');
  const before = calls.length;
  await api('/api/v1/plans'); // cached: no new mint, one request
  expect(calls.length).toBe(before + 1);
  expect(calls.filter((c) => c[0] === '/v3/tokens')).toHaveLength(1);
});

test('concurrent 401s share one token request', async () => {
  let mints = 0;
  const fetchFn = jest.fn((url, init) => {
    if (url === '/v3/tokens') { mints += 1; return Promise.resolve(res(201, { token: 't' })); }
    return Promise.resolve(init.headers[TOKEN_HEADER] ? res(200, {}) : res(401, {}));
  });
  const api = createApiFetch(fetchFn, { storage: memStorage(), doc });
  await Promise.all([api('/api/v1/a'), api('/api/v1/b'), api('/api/v1/c')]);
  expect(mints).toBe(1);
});

test('surfaces the 401 when a token cannot be minted, and stops retrying', async () => {
  const fetchFn = jest.fn((url) => Promise.resolve(url === '/v3/tokens' ? res(404, {}) : res(401, {})));
  const api = createApiFetch(fetchFn, { storage: memStorage(), doc });
  expect((await api('/api/v1/plans')).status).toBe(401);
});

test('expired cached tokens are not sent', async () => {
  const storage = memStorage();
  storage.setItem('migration-ui-token', JSON.stringify({ token: 'old', expiresAt: 1000 }));
  const fetchFn = jest.fn().mockResolvedValue(res(200, {}));
  const api = createApiFetch(fetchFn, { storage, doc, now: () => 5000000 });
  await api('/api/v1/plans');
  expect(fetchFn.mock.calls[0][1].headers[TOKEN_HEADER]).toBeUndefined();
});

test('a 201 without a token field discards the token, warns, and backs off', async () => {
  const warn = jest.spyOn(console, 'warn').mockImplementation(() => {});
  const calls = [];
  const fetchFn = jest.fn((url, init) => {
    calls.push([url, init && init.method]);
    if (url === '/v3/tokens') return Promise.resolve(res(201, { id: 'token-abc', type: 'token' }));
    if (url.startsWith('/v3/tokens/')) return Promise.resolve(res(204, {}));
    return Promise.resolve(res(401, { error: 'missing' }));
  });
  const api = createApiFetch(fetchFn, { storage: memStorage(), doc });
  expect((await api('/api/v1/plans')).status).toBe(401);
  expect(calls).toContainEqual(['/v3/tokens/token-abc', 'DELETE']);
  expect(warn.mock.calls[0][0]).toMatch(/no usable token \(fields: id, type\)/);
  // Further polls must not mint again.
  await api('/api/v1/plans');
  await api('/api/v1/plans');
  expect(calls.filter(([u, m]) => u === '/v3/tokens' && m === 'POST')).toHaveLength(1);
  warn.mockRestore();
});

test('a freshly minted token that the backend still rejects is not minted again', async () => {
  const warn = jest.spyOn(console, 'warn').mockImplementation(() => {});
  let mints = 0;
  const fetchFn = jest.fn((url) => {
    if (url === '/v3/tokens') { mints += 1; return Promise.resolve(res(201, { id: 'x', token: 't' })); }
    return Promise.resolve(res(401, {}));
  });
  const api = createApiFetch(fetchFn, { storage: memStorage(), doc });
  await api('/api/v1/a');
  await api('/api/v1/b');
  await api('/api/v1/c');
  expect(mints).toBe(1);
  warn.mockRestore();
});

test('minting is attempted again after the backoff window', async () => {
  const warn = jest.spyOn(console, 'warn').mockImplementation(() => {});
  let t = 1000;
  let mints = 0;
  const fetchFn = jest.fn((url) => {
    if (url === '/v3/tokens') { mints += 1; return Promise.resolve(res(500, {})); }
    return Promise.resolve(res(401, {}));
  });
  const api = createApiFetch(fetchFn, { storage: memStorage(), doc, now: () => t });
  await api('/api/v1/a');
  t += 60 * 1000;
  await api('/api/v1/a');
  expect(mints).toBe(1);
  t += 5 * 60 * 1000;
  await api('/api/v1/a');
  expect(mints).toBe(2);
  warn.mockRestore();
});

test('headers are sent as a plain object and caller headers are preserved', async () => {
  const fetchFn = jest.fn((url, init) => Promise.resolve(
    url === '/v3/tokens' ? res(201, { token: 't', id: 'i', ttl: 1 }) : (init.headers[TOKEN_HEADER] ? res(200, {}) : res(401, {}))));
  jest.spyOn(console, 'info').mockImplementation(() => {});
  const api = createApiFetch(fetchFn, { storage: memStorage(), doc });
  await api('/api/v1/x', { headers: new Headers({ 'Content-Type': 'application/json' }) });
  const sent = fetchFn.mock.calls.filter(([u]) => u === '/api/v1/x').pop()[1].headers;
  expect(Object.getPrototypeOf(sent)).toBe(Object.prototype);
  expect(sent['content-type']).toBe('application/json');
  expect(sent[TOKEN_HEADER]).toBe('t');
  console.info.mockRestore();
});

test('the token stays usable when sessionStorage is unavailable', async () => {
  const broken = { getItem: () => null, setItem: () => { throw new Error('blocked'); }, removeItem: () => {} };
  let mints = 0;
  jest.spyOn(console, 'info').mockImplementation(() => {});
  const fetchFn = jest.fn((url, init) => {
    if (url === '/v3/tokens') { mints += 1; return Promise.resolve(res(201, { token: 't', id: 'i', ttl: 1 })); }
    return Promise.resolve(init.headers[TOKEN_HEADER] ? res(200, {}) : res(401, {}));
  });
  const api = createApiFetch(fetchFn, { storage: broken, doc });
  await api('/api/v1/a');
  await api('/api/v1/b');
  await api('/api/v1/c');
  expect(mints).toBe(1);
  console.info.mockRestore();
});
