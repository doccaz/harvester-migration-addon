// Links from this UI to Harvester's own pages for the same objects.
//
// The dashboard lists the VM Import Controller's resources under
//   <origin>/dashboard/c/<cluster>/explorer/migration.harvesterhci.io.<resource>
// and an object under .../<resource>/<namespace>/<name>. This UI is normally served
// through the same origin by the Kubernetes service proxy
//   .../k8s/clusters/<cluster>/api/v1/namespaces/<ns>/services/<svc>/proxy/
// so the dashboard address follows from where the page was loaded. Served any other way
// (a NodePort, a port-forward) there is no dashboard to point at, and no link is made.

const GROUP = 'migration.harvesterhci.io';

export function dashboardBase(location) {
  const m = location.pathname.match(/^(.*?)\/k8s\/clusters\/([^/]+)\//);
  if (!m) return null;
  return `${location.origin}${m[1]}/dashboard/c/${m[2]}/explorer`;
}

// `resource` is the singular lower-case kind: vmwaresource, ovasource, virtualmachineimport.
export function harvesterListUrl(location, resource) {
  const base = dashboardBase(location);
  return base ? `${base}/${GROUP}.${resource}` : null;
}

export function harvesterResourceUrl(location, resource, namespace, name) {
  const list = harvesterListUrl(location, resource);
  if (!list || !namespace || !name) return null;
  return `${list}/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`;
}
