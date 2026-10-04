// A fetch mock that answers from a table of routes, for tests that render the
// whole app. A route value is the JSON body, or a function (url, init) => body
// or { status, body }. Unknown API paths answer 404 and are recorded so a test
// can assert that nothing unexpected was requested.

export function installApi(routes) {
  const calls = [];
  global.fetch = jest.fn((url, init = {}) => {
    const path = String(url).split('?')[0];
    calls.push({ url: String(url), method: (init.method || 'GET').toUpperCase(), body: init.body });
    const key = Object.keys(routes).find((k) => k === path || k === `${(init.method || 'GET').toUpperCase()} ${path}`);
    if (key === undefined) {
      return Promise.resolve({ ok: false, status: 404, json: async () => ({ error: `no mock for ${path}` }), text: async () => `no mock for ${path}` });
    }
    let value = routes[key];
    if (typeof value === 'function') value = value(String(url), init);
    // A route may answer with a promise, to hold a request open.
    return Promise.resolve(value).then((resolved) => {
      const { status = 200, body = resolved } = resolved && resolved.__status ? resolved : { body: resolved };
      const text = typeof body === 'string' ? body : JSON.stringify(body);
      return {
        ok: status >= 200 && status < 300,
        status,
        headers: { get: () => (typeof body === 'string' ? 'text/plain' : 'application/json') },
        json: async () => (typeof body === 'string' ? JSON.parse(body) : body),
        text: async () => text,
      };
    });
  });
  return calls;
}

// Resolves once no new request has been made for a short while and React has
// flushed the results. The app refetches around start-up (and shows its loading
// state while it does), so a snapshot taken at the first sight of data can catch
// a flicker; waiting for quiet makes snapshots deterministic.
export async function settle(calls, { quietMs = 150, maxMs = 3000 } = {}) {
  const { act } = require('@testing-library/react');
  // Bounded by iterations, not by Date.now: a test may freeze the clock.
  const maxIterations = Math.ceil(maxMs / quietMs);
  let seen = -1;
  let stable = 0;
  for (let i = 0; i < maxIterations && stable < 2; i += 1) {
    // eslint-disable-next-line no-await-in-loop
    await act(async () => { await new Promise((r) => setTimeout(r, quietMs)); });
    if (calls.length === seen) stable += 1; else { stable = 0; seen = calls.length; }
  }
}

// Helpers for asserting on the recorded requests.
export const requests = (calls, method, urlPart) => calls.filter((c) => c.method === method && c.url.includes(urlPart));

export const reply = (status, body) => ({ __status: true, status, body });
