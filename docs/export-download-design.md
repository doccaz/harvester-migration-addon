# Design: downloading an exported OVA from the UI

Status: proposal, 2026-10-02. Nothing implemented yet.

## Problem
An export Job runs in the VM's namespace and writes the OVA to that namespace's export PVC.
The UI pod (in `harvester-system`) cannot mount a PVC from another namespace, so
`exportView` marks such exports `downloadable: false` and the button is hidden. Where the
volume is mounted, the download still goes through `fetch` + `blob()` (the whole file in
browser memory) and the backend refuses files over 2 GiB (`EXPORT_DOWNLOAD_MAX_BYTES`).
Real exports are several GB, so the UI cannot deliver them today. `kubectl exec`/`cp` are no
alternative: they dropped multi-GB streams in the lab (see phase2-verification.md).

## Design

### 1. A short-lived serve pod in the export's namespace
New mode of the same binary, next to `export-worker` and `export-cleanup`:
`/usr/local/bin/vm-import-ui export-serve`.

- A Pod `<export-job>-serve` in the export's namespace, same image, `restartPolicy: Never`,
  same hardened securityContext as the export Job, small resource requests.
- Mounts the export PVC **read-only** (RWX, so it can run beside a finished Job).
- Serves exactly one fixed file (`<target>.ova`, given by env) on `:8081` with
  `http.ServeContent` (Range, ETag, Last-Modified, no buffering). No directory listing, no
  other paths.
- Requires `Authorization: Bearer <secret>`; the secret is random, kept in a Secret owned by
  the pod and injected with `secretKeyRef`.
- Exits by itself after an idle timeout (default 30 min without requests) and has an
  `activeDeadlineSeconds` (4 h). Both the pod and its Secret carry an ownerReference to the
  export Job, so deleting/purging the export removes them; `Delete` also removes them
  explicitly.

### 2. Who creates it: the user, not the ServiceAccount
The ticket endpoint creates the pod and Secret with the **caller's scoped client**
(`kube.Scoped`), the same principle as export creation. The UI ServiceAccount gains no new
permissions. A user who may create exports (Jobs) but not pods/secrets gets a clear 403.

### 3. Backend proxy
`GET /api/v1/exports/{ns}/{id}/download?ticket=...` streams from the serve pod
(`podIP:8081`, found through the user-scoped pod Get) with `httputil.ReverseProxy`
(`FlushInterval: -1`), passing Range / If-Range / ETag through. No size cap on this path.
The chart's NetworkPolicy must allow UI egress to pods on 8081.

### 4. Tickets: how a plain browser download carries authorisation
A browser download manager cannot send `X-Migration-Token`, and `fetch`+`blob` is what
broke large files. So:

- `POST /api/v1/exports/{ns}/{id}/download-ticket` is a normal token-authenticated route.
  It checks the user can read the export Job, ensures the serve pod is Ready (idempotent;
  returns `202 {state: starting}` until then, the UI polls), and returns
  `{url, size, expiresAt}`.
- The ticket is an HMAC (`ns|id|exp`), key from a chart-generated Secret so it works with
  more than one replica and across restarts. TTL 30 min, bound to that one export,
  reusable inside its TTL because browsers resume interrupted downloads with Range.
- The download route is the **single, explicit exception** to "every API route needs a user
  token": it accepts only a valid ticket. `TestEveryAPIRouteRequiresAUserToken` gets an
  explicit allow-list entry plus tests that bad, expired, tampered and other-export tickets
  get 401.
- The UI starts the download with a real navigation (`<a href download>`), building the URL
  from `apiBase` (the Rancher proxy sub-path logic in `index.js`), so the file streams to
  disk through the browser's download manager: progress, no memory use, resume.
- Trade-off: the ticket is in the URL, so it can appear in apiserver/Rancher access logs.
  Mitigated by the short TTL and export binding; it grants one file already readable by the
  user who minted it.

### 5. UI changes
`downloadable` becomes true for any finished export (Job Succeeded and a recorded target),
regardless of namespace. The button becomes "Download" -> mint ticket -> poll while
`starting` -> navigate. Show the file size from the ticket response.
The legacy in-pod `Download` handler and the `blob()` path are removed once this works
(one path, not two); `EXPORT_DOWNLOAD_MAX_BYTES` goes away with them.

## Risks to settle in the lab
1. **Long streams through Rancher and the API service proxy.** `kubectl exec` streams were
   cut; a 4.5 GB proxied GET may be too. Range resume is the safety net; the lab test must
   include killing the connection mid-download and resuming.
2. RWX volume attach for the serve pod while the Job pod's node differs (Longhorn share
   manager).
3. Users with Jobs but not pods/secrets permissions (the 403 path).

## Plan
1. `export-serve` mode + tests (path fixed, auth, Range, idle exit).
2. Ticket signer + verifier, ticket endpoint, proxy route, route-golden and auth-test
   updates, handler tests with a fake clientset and an `httptest` server as the pod.
3. Chart: key Secret, NetworkPolicy egress, env wiring; RBAC unchanged for the SA.
4. Frontend: ticket flow + navigation; remove `blob()` download and the legacy handler.
5. Lab: download a multi-GB OVA in the browser through the Rancher proxy, compare SHA-256,
   kill the connection and resume, delete the export and confirm pod/Secret are gone.
6. Release.

## Decisions needed
- Create the pod/Secret with the user's identity (proposed) rather than the ServiceAccount?
- One serve pod per export, 30 min idle timeout, 30 min ticket TTL: acceptable defaults?
