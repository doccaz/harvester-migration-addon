import { dashboardBase, harvesterResourceUrl, harvesterListUrl, FORKLIFT } from './harvesterLinks';

const loc = (pathname, origin = 'https://harvester.example.org') => ({ pathname, origin });
const proxy = '/k8s/clusters/local/api/v1/namespaces/harvester-system/services/http:mig-harvester-migration-ui:8080/proxy/';

describe('the dashboard address is derived from where this UI is served', () => {
  test('through the cluster proxy', () => {
    expect(dashboardBase(loc(proxy))).toBe('https://harvester.example.org/dashboard/c/local/explorer');
  });
  test('the cluster id comes from the path', () => {
    expect(dashboardBase(loc('/k8s/clusters/c-m-abc123/api/v1/namespaces/x/services/y/proxy/index.html')))
      .toBe('https://harvester.example.org/dashboard/c/c-m-abc123/explorer');
  });
  test('a path prefix in front of Rancher is kept', () => {
    expect(dashboardBase(loc(`/rancher${proxy}`))).toBe('https://harvester.example.org/rancher/dashboard/c/local/explorer');
  });
  test('served directly (NodePort, port-forward, ingress at the root) there is no dashboard to link to', () => {
    expect(dashboardBase(loc('/'))).toBeNull();
    expect(dashboardBase(loc('/index.html'))).toBeNull();
    expect(dashboardBase(loc('/ui/'))).toBeNull();
  });
});

describe('resource addresses', () => {
  const l = loc(proxy);
  test('a namespaced object', () => {
    expect(harvesterResourceUrl(l, 'vmwaresource', 'techday', 'vcenter-lab'))
      .toBe('https://harvester.example.org/dashboard/c/local/explorer/migration.harvesterhci.io.vmwaresource/techday/vcenter-lab');
    expect(harvesterResourceUrl(l, 'virtualmachineimport', 'labs', 'db'))
      .toBe('https://harvester.example.org/dashboard/c/local/explorer/migration.harvesterhci.io.virtualmachineimport/labs/db');
  });
  test('a list', () => {
    expect(harvesterListUrl(l, 'ovasource')).toBe('https://harvester.example.org/dashboard/c/local/explorer/migration.harvesterhci.io.ovasource');
  });
  test('names are escaped', () => {
    expect(harvesterResourceUrl(l, 'ovasource', 'a b', 'x/y')).toContain('/a%20b/x%2Fy');
  });
  test('no link when there is no dashboard, or the object has no name', () => {
    expect(harvesterResourceUrl(loc('/'), 'vmwaresource', 'ns', 'n')).toBeNull();
    expect(harvesterResourceUrl(l, 'vmwaresource', 'ns', '')).toBeNull();
    expect(harvesterResourceUrl(l, 'vmwaresource', '', 'n')).toBeNull();
  });
});

describe('objects of other API groups, opened in the YAML editor', () => {
  const l = loc(proxy);
  test('Forklift plans and providers have no page of their own, so the link opens the YAML view', () => {
    expect(harvesterResourceUrl(l, 'plan', 'forklift', 'forklift-test', FORKLIFT))
      .toBe('https://harvester.example.org/dashboard/c/local/explorer/forklift.konveyor.io.plan/forklift/forklift-test?mode=edit&as=yaml');
    expect(harvesterResourceUrl(l, 'provider', 'forklift', 'vsphere-lab', FORKLIFT))
      .toContain('/forklift.konveyor.io.provider/forklift/vsphere-lab?mode=edit&as=yaml');
  });
  test('the default group is unchanged', () => {
    expect(harvesterResourceUrl(l, 'vmwaresource', 'techday', 'x')).not.toContain('?');
  });
  test('no dashboard, no link', () => {
    expect(harvesterResourceUrl(loc('/'), 'plan', 'forklift', 'x', FORKLIFT)).toBeNull();
  });
});
