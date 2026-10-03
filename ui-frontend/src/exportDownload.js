// Starting the download of an exported OVA.
//
// The backend cannot let a browser fetch the file with a bare URL (a download
// manager cannot send the X-Migration-Token header), and loading it with fetch +
// blob() puts every byte in memory. So: ask for a signed, short-lived URL with the
// normal authenticated fetch, then let the browser download that URL itself.
// While the serve pod for the export is still starting the backend answers 202
// and we ask again.

export const POLL_MS = 2000;
export const MAX_ATTEMPTS = 90; // about three minutes for a volume to attach

const defaultSleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

async function errorMessage(response) {
  const text = await response.text();
  try { return JSON.parse(text).error || text; } catch (e) { return text; }
}

// Resolves to { url, fileName }. The URL starts with /api/ and is NOT prefixed
// for a sub-path proxy; use withApiBase for that.
export async function requestDownload(fetchFn, exp, { sleep = defaultSleep, onWaiting = () => {} } = {}) {
  const path = `/api/v1/exports/${encodeURIComponent(exp.namespace)}/${encodeURIComponent(exp.exportId)}/download-ticket`;
  for (let attempt = 0; attempt < MAX_ATTEMPTS; attempt += 1) {
    const response = await fetchFn(path, { method: 'POST' });
    if (response.status === 200) {
      const body = await response.json();
      return { url: body.url, fileName: body.fileName || `${exp.targetName || exp.vmName}.ova` };
    }
    if (response.status !== 202) {
      throw new Error(await errorMessage(response));
    }
    onWaiting(attempt);
    await sleep(POLL_MS);
  }
  throw new Error('The download service for this export did not become ready in time. Try again in a moment.');
}

// The API base for a page served under a sub-path such as the Rancher service
// proxy (".../services/http:svc:8080/proxy/"); empty at the app root.
export const apiBaseFor = (pathname) => pathname.replace(/[^/]*$/, '').replace(/\/$/, '');

export const withApiBase = (apiBase, url) => `${apiBase}${url}`;
