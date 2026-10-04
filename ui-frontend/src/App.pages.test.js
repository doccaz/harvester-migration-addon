// Page snapshots. These pin what every page renders for representative data, so
// the Phase 4 module split (docs/phase4-plan.md) can move code without changing a
// pixel of markup. After an intentional UI change run `yarn test -u` and review
// the diff; a pure code move must leave every snapshot untouched.
import React from 'react';
import { render, screen, fireEvent, within } from '@testing-library/react';
import App from './App';
import { installApi, settle, reply } from './testing/mockApi';
import { routes } from './testing/fixtures';

beforeEach(() => {
  window.localStorage.clear();
  window.history.replaceState({}, '', '/');
  // The plans footer shows when the list was last updated; freeze the clock for the snapshots.
  jest.spyOn(Date, 'now').mockReturnValue(Date.UTC(2026, 9, 4, 12, 0, 0));
});
afterEach(() => { jest.restoreAllMocks(); });

const tab = (name) => fireEvent.click(screen.getByRole('button', { name }));

test('plans: VM Import Controller', async () => {
  const calls = installApi(routes());
  const { container } = render(<App />);
  await settle(calls);
  expect(container.textContent).toContain('web-migration');
  expect(container).toMatchSnapshot();
});

// --- helpers -----------------------------------------------------------------

const open = async (setup) => {
  const calls = installApi(setup ? { ...routes(), ...setup } : routes());
  const utils = render(<App />);
  await settle(calls);
  return { calls, ...utils };
};
// Every snapshot also asserts a few strings that must be on the page, so a snapshot
// can never silently capture a blank or error page.
const snap = async (utils, ...mustShow) => {
  await settle(utils.calls);
  mustShow.forEach((t) => expect(utils.container.textContent).toContain(t));
  expect(utils.container).toMatchSnapshot();
};
const click = async (utils, el) => { fireEvent.click(el); await settle(utils.calls); };
const clickText = (utils, text, index = 0) => click(utils, screen.getAllByText(text)[index]);
// The Details button of the table row that contains `rowText`.
const detailsOf = (utils, rowText) => click(utils, within(screen.getAllByText(rowText)[0].closest('tr')).getByText('Details'));
const clickButton = (utils, name, index = 0) => click(utils, screen.getAllByRole('button', { name })[index]);

// --- plans -------------------------------------------------------------------

test('plans: expanded row', async () => {
  const u = await open();
  await click(u, u.container.querySelector('tbody tr button'));
  await snap(u, "web-migration", "virtualMachineRunning");
});

test('plans: Forklift subtab', async () => {
  const u = await open();
  await clickButton(u, 'Forklift');
  expect(u.container.textContent).toContain('migrate-web');
  await snap(u);
});

test('plans: empty lists', async () => {
  const u = await open({ '/api/v1/plans': [], '/api/v1/forklift/plans': [] });
  await snap(u, "No migration plans found");
});

test('plan details', async () => {
  const u = await open({
    '/api/v1/plans/techday/web-migration/logs': 'importing disk 1\nimport complete',
    '/api/v1/plans/techday/web-migration/yaml': 'kind: VirtualMachineImport\n',
  });
  await detailsOf(u, 'web-migration');
  await snap(u, "web-migration");
});

test('plan wizard', async () => {
  const u = await open();
  await clickButton(u, 'Create');
  await snap(u, "Create");
});

test('plan edit modal', async () => {
  const u = await open();
  await click(u, screen.getAllByTitle('Edit Plan')[0]);
  await snap(u, "sles16-db");
});

test('Forklift plan details', async () => {
  const u = await open({
    '/api/v1/forklift/plans/forklift/migrate-web/migration': reply(404, { error: 'not found' }),
  });
  await clickButton(u, 'Forklift');
  await detailsOf(u, 'migrate-web');
  await snap(u, "migrate-web");
});

test('Forklift unavailable: setup checklist', async () => {
  const u = await open({ '/api/v1/forklift/availability': { available: false, message: 'Forklift CRDs not found' } });
  await clickButton(u, 'Forklift');
  await snap(u, "Forklift CRDs not found");
});

// --- sources -----------------------------------------------------------------

test('vCenter sources: VM Import Controller', async () => {
  const u = await open();
  await click(u, screen.getByRole('button', { name: 'vCenter Sources' }));
  await snap(u, "vcenter-lab", "vcenter-old", "Not Ready");
});

test('vCenter sources: Forklift', async () => {
  const u = await open();
  await click(u, screen.getByRole('button', { name: 'vCenter Sources' }));
  await clickButton(u, 'Forklift');
  await snap(u, "vsphere-lab", "esxi-lab", "ConnectionFailed");
});

test('vCenter source details', async () => {
  const u = await open({ '/api/v1/harvester/vmwaresources/techday/vcenter-lab/yaml': 'kind: VmwareSource\n' });
  await click(u, screen.getByRole('button', { name: 'vCenter Sources' }));
  await detailsOf(u, 'vcenter-lab');
  await snap(u, "vcenter-lab", "https://vcenter.lab/sdk");
});

test('vCenter source wizard', async () => {
  const u = await open();
  await click(u, screen.getByRole('button', { name: 'vCenter Sources' }));
  await clickButton(u, /Add|Create|New/);
  await snap(u);
});

test('Forklift provider details', async () => {
  const u = await open({ '/api/v1/forklift/providers/forklift/vsphere-lab/yaml': 'kind: Provider\n' });
  await click(u, screen.getByRole('button', { name: 'vCenter Sources' }));
  await clickButton(u, 'Forklift');
  await detailsOf(u, 'vsphere-lab');
  await snap(u, "vsphere-lab");
});

test('OVA sources: VM Import Controller', async () => {
  const u = await open();
  await click(u, screen.getByRole('button', { name: 'OVA Sources' }));
  await snap(u, "ova-source", "http://files.lab/exports");
});

test('OVA sources: Forklift', async () => {
  const u = await open();
  await click(u, screen.getByRole('button', { name: 'OVA Sources' }));
  await clickButton(u, 'Forklift');
  await snap(u, "ova-nfs");
});

test('OVA source details', async () => {
  const u = await open({ '/api/v1/harvester/ovasources/labs/ova-source/yaml': 'kind: OvaSource\n' });
  await click(u, screen.getByRole('button', { name: 'OVA Sources' }));
  await clickText(u, 'Details');
  await snap(u, "ova-source", "ova-creds");
});

// --- export, about -----------------------------------------------------------

test('export page', async () => {
  const u = await open();
  await click(u, screen.getByRole('button', { name: 'Export VMs' }));
  await snap(u, "bastion", "downstream-01", "Download", "42%", "disk read failed", "4.24 GB");
});

test('export page: a stopped VM selected', async () => {
  const u = await open();
  await click(u, screen.getByRole('button', { name: 'Export VMs' }));
  await clickText(u, 'bastion');
  await snap(u, "bastion");
});

test('export page: a running VM selected', async () => {
  const u = await open();
  await click(u, screen.getByRole('button', { name: 'Export VMs' }));
  await clickText(u, 'downstream-01');
  await snap(u, "downstream-01");
});

test('about', async () => {
  const u = await open();
  await click(u, screen.getByRole('button', { name: 'About' }));
  await snap(u, "About VM Import UI");
});
