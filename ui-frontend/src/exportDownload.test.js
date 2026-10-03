import { requestDownload, apiBaseFor, withApiBase, MAX_ATTEMPTS } from './exportDownload';

const res = (status, body) => ({
  status,
  ok: status >= 200 && status < 300,
  json: async () => body,
  text: async () => (typeof body === 'string' ? body : JSON.stringify(body)),
});
const exp = { namespace: 'labs', exportId: 'abc123', targetName: 'vm1', vmName: 'vm1' };
const noSleep = () => Promise.resolve();

test('asks for a ticket with POST and returns the URL', async () => {
  const f = jest.fn().mockResolvedValue(res(200, { state: 'ready', url: '/api/v1/exports/labs/abc123/download?ticket=t', fileName: 'vm1.ova' }));
  const out = await requestDownload(f, exp, { sleep: noSleep });
  expect(f).toHaveBeenCalledWith('/api/v1/exports/labs/abc123/download-ticket', { method: 'POST' });
  expect(out).toEqual({ url: '/api/v1/exports/labs/abc123/download?ticket=t', fileName: 'vm1.ova' });
});

test('keeps asking while the service is starting, and reports waiting', async () => {
  const f = jest.fn()
    .mockResolvedValueOnce(res(202, { state: 'starting' }))
    .mockResolvedValueOnce(res(202, { state: 'starting' }))
    .mockResolvedValueOnce(res(200, { url: '/api/x', fileName: 'a.ova' }));
  const waits = [];
  const out = await requestDownload(f, exp, { sleep: noSleep, onWaiting: (n) => waits.push(n) });
  expect(f).toHaveBeenCalledTimes(3);
  expect(waits).toEqual([0, 1]);
  expect(out.url).toBe('/api/x');
});

test('falls back to the target name when the response has no file name', async () => {
  const f = jest.fn().mockResolvedValue(res(200, { url: '/api/x' }));
  expect((await requestDownload(f, exp, { sleep: noSleep })).fileName).toBe('vm1.ova');
});

test('shows the backend error, not a generic one', async () => {
  const f = jest.fn().mockResolvedValue(res(403, { error: 'pods is forbidden' }));
  await expect(requestDownload(f, exp, { sleep: noSleep })).rejects.toThrow('pods is forbidden');
  const g = jest.fn().mockResolvedValue(res(409, 'not json'));
  await expect(requestDownload(g, exp, { sleep: noSleep })).rejects.toThrow('not json');
});

test('gives up after a bounded number of attempts', async () => {
  const f = jest.fn().mockResolvedValue(res(202, { state: 'starting' }));
  await expect(requestDownload(f, exp, { sleep: noSleep })).rejects.toThrow(/did not become ready/);
  expect(f).toHaveBeenCalledTimes(MAX_ATTEMPTS);
});

test('escapes path segments', async () => {
  const f = jest.fn().mockResolvedValue(res(200, { url: '/api/x' }));
  await requestDownload(f, { namespace: 'a b', exportId: 'c/d' }, { sleep: noSleep });
  expect(f.mock.calls[0][0]).toBe('/api/v1/exports/a%20b/c%2Fd/download-ticket');
});

test('prefixes the URL for a sub-path proxy and leaves the root alone', () => {
  const proxy = '/k8s/clusters/local/api/v1/namespaces/harvester-system/services/http:mig:8080/proxy/';
  expect(apiBaseFor(proxy)).toBe('/k8s/clusters/local/api/v1/namespaces/harvester-system/services/http:mig:8080/proxy');
  expect(apiBaseFor(`${proxy}index.html`)).toBe(apiBaseFor(proxy));
  expect(apiBaseFor('/')).toBe('');
  expect(apiBaseFor('/index.html')).toBe('');
  expect(withApiBase('/p', '/api/v1/x?ticket=t')).toBe('/p/api/v1/x?ticket=t');
  expect(withApiBase('', '/api/v1/x')).toBe('/api/v1/x');
});
