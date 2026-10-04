// The VM Import Controller's resources have a fixed shape: the controller creates its CRDs at runtime
// from its Go types (see docs/contract.md). ../ui-backend/internal/vmic/testdata/upstream-contract.json
// lists every spec/status field path at the pinned version. These tests keep what the UI builds, and
// the fixtures the other tests render, inside it, so the fixtures cannot quietly describe a resource
// that does not exist.
import fs from 'fs';
import path from 'path';
import { buildVmicPlan } from './utils';
import { vmicPlans, vmwareSources, ovaSources } from './testing/fixtures';

const contract = JSON.parse(fs.readFileSync(path.join(__dirname, '..', '..', 'ui-backend', 'internal', 'vmic', 'testdata', 'upstream-contract.json'), 'utf8'));

// The same naming as the generator: a list is "k[]", its members "k[].field".
function flatten(prefix, value, out) {
  if (Array.isArray(value)) {
    value.forEach((item) => {
      if (item && typeof item === 'object') {
        Object.entries(item).forEach(([k, child]) => {
          const p = `${prefix.replace(/\.$/, '')}[].${k}`;
          out.add(p);
          flatten(`${p}.`, child, out);
        });
      }
    });
  } else if (value && typeof value === 'object') {
    Object.entries(value).forEach(([k, child]) => {
      out.add(Array.isArray(child) ? `${prefix}${k}[]` : `${prefix}${k}`);
      flatten(`${prefix}${k}.`, child, out);
    });
  }
}

const outside = (kind, obj) => {
  const got = new Set();
  ['spec', 'status'].forEach((root) => { if (obj[root]) { got.add(root); flatten(`${root}.`, obj[root], got); } });
  const k = contract.kinds[kind];
  const known = new Set(k.paths);
  return [...got].filter((p) => !known.has(p) && !k.opaque.some((o) => p.startsWith(`${o}.`) || p.startsWith(`${o}[]`))).sort();
};

test('the contract file is the one the backend tests use', () => {
  expect(contract.version).toMatch(/^v\d+\.\d+\.\d+/);
  expect(Object.keys(contract.kinds).sort()).toEqual(['OpenstackSource', 'OvaSource', 'VirtualMachineImport', 'VmwareSource']);
});

describe('fixtures describe resources that exist', () => {
  test.each(vmicPlans.map((p) => [p.metadata.name, p]))('plan %s', (_, plan) => {
    expect(outside('VirtualMachineImport', plan)).toEqual([]);
  });
  test.each(vmwareSources.map((p) => [p.metadata.name, p]))('vCenter source %s', (_, src) => {
    expect(outside('VmwareSource', src)).toEqual([]);
  });
  test.each(ovaSources.map((p) => [p.metadata.name, p]))('OVA source %s', (_, src) => {
    expect(outside('OvaSource', src)).toEqual([]);
  });
});

describe('the plan the wizard builds is one the controller accepts', () => {
  const base = {
    planName: 'My Plan', targetNamespace: 'labs', sourceName: 'vc', sourceNamespace: 'techday', storageClass: 'harvester-longhorn',
    networkMappings: { 'VM Network': 'default/mgmt' }, networkModels: { 'VM Network': 'virtio' },
    capabilities: { hasAdvancedPower: true }, forcePowerOff: true, shutdownTimeout: 60, defaultModel: 'virtio', skipPreflight: false, diskBus: 'virtio',
  };
  test('from vCenter', () => {
    const plan = buildVmicPlan({ ...base, sourceType: 'vmware', selectedVm: { name: 'rhel', folder: '/dc/vm', cpu: 2, memoryMB: 4096, diskSizeGB: 40, disks: [], networks: [] } });
    expect(plan.spec.networkMapping.length).toBeGreaterThan(0);
    expect(outside('VirtualMachineImport', plan)).toEqual([]);
  });
  test('from an OVA', () => {
    const plan = buildVmicPlan({ ...base, sourceType: 'ova', ovaVmName: 'imported' });
    expect(outside('VirtualMachineImport', plan)).toEqual([]);
  });
  test('the checker itself sees an unknown field and a misnamed list', () => {
    expect(outside('VirtualMachineImport', { spec: { virtualMachineName: 'x', bogus: 1 } })).toEqual(['spec.bogus']);
    expect(outside('VirtualMachineImport', { status: { conditions: [{ type: 'x' }] } })).toEqual(['status.conditions[]', 'status.conditions[].type']);
    expect(outside('VirtualMachineImport', { status: { importConditions: [{ type: 'x' }] } })).toEqual([]);
  });
});
