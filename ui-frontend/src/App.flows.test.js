// Behaviour of the app's create/edit/delete/run flows and its polling, pinned
// before and after the App() split (docs/phase4-plan.md, step 4.3). The page
// snapshots cover what renders; these cover what the controls do: which request
// goes out, what is refetched afterwards, and when dialogs open and close.
import React from 'react';
import { render, screen, fireEvent, within, act } from '@testing-library/react';
import App from './App';
import { installApi, settle, requests, reply } from './testing/mockApi';
import { routes, forkliftPlans } from './testing/fixtures';

beforeEach(() => {
  window.localStorage.clear();
  window.history.replaceState({}, '', '/');
  window.alert = jest.fn();
  window.confirm = jest.fn(() => true);
});

const open = async (extra) => {
  const calls = installApi({ ...routes(), ...extra });
  const utils = render(<App />);
  await settle(calls);
  return { calls, ...utils };
};
const click = async (u, el) => { fireEvent.click(el); await settle(u.calls); };
const nav = (u, name) => click(u, screen.getByRole('button', { name }));
const inRow = (text, title) => within(screen.getAllByText(text)[0].closest('tr')).getByTitle(title);
const dialog = () => screen.queryByText('Confirm Deletion');
const confirmDelete = (u) => click(u, within(screen.getByText('Confirm Deletion').closest('.fixed')).getByText('Delete'));
const cancelDelete = (u) => click(u, within(screen.getByText('Confirm Deletion').closest('.fixed')).getByText('Cancel'));
const ok = reply(200, {});

describe('delete flows: confirm, request, refetch, close', () => {
  const cases = [
    { name: 'VMIC plan', setup: async () => {}, row: 'web-migration', del: 'DELETE /api/v1/plans/techday/web-migration', list: '/api/v1/plans' },
    { name: 'vCenter source', setup: (u) => nav(u, 'vCenter Sources'), row: 'vcenter-lab', del: 'DELETE /api/v1/harvester/vmwaresources/techday/vcenter-lab', list: '/api/v1/harvester/vmwaresources' },
    { name: 'OVA source', setup: (u) => nav(u, 'OVA Sources'), row: 'ova-source', del: 'DELETE /api/v1/harvester/ovasources/labs/ova-source', list: '/api/v1/harvester/ovasources' },
    {
      name: 'Forklift provider',
      setup: async (u) => { await nav(u, 'vCenter Sources'); await click(u, screen.getAllByRole('button', { name: 'Forklift' })[0]); },
      row: 'vsphere-lab', del: 'DELETE /api/v1/forklift/providers/forklift/vsphere-lab', list: '/api/v1/forklift/providers',
    },
    {
      name: 'Forklift plan',
      setup: async (u) => { await click(u, screen.getAllByRole('button', { name: 'Forklift' })[0]); },
      row: 'migrate-web', del: 'DELETE /api/v1/forklift/plans/forklift/migrate-web', list: '/api/v1/forklift/plans',
    },
  ];

  cases.forEach(({ name, setup, row, del, list }) => {
    const [method, ...rest] = del.split(' ');
    const url = rest.join(' ');

    test(`${name}: confirming deletes and refreshes the list`, async () => {
      const u = await open({ [del]: ok });
      await setup(u);
      await click(u, inRow(row, 'Delete'));
      expect(dialog()).toBeInTheDocument();
      expect(requests(u.calls, method, url)).toHaveLength(0);
      const before = requests(u.calls, 'GET', list).length;
      await confirmDelete(u);
      expect(requests(u.calls, method, url)).toHaveLength(1);
      expect(requests(u.calls, 'GET', list).length).toBeGreaterThan(before);
      expect(dialog()).not.toBeInTheDocument();
    });

    test(`${name}: cancelling sends nothing`, async () => {
      const u = await open({ [del]: ok });
      await setup(u);
      await click(u, inRow(row, 'Delete'));
      await cancelDelete(u);
      expect(requests(u.calls, method, url)).toHaveLength(0);
      expect(dialog()).not.toBeInTheDocument();
    });
  });
});

describe('edit flows fetch the full object and open the wizard on it', () => {
  test('vCenter source', async () => {
    const full = { metadata: { name: 'vcenter-lab', namespace: 'techday' }, spec: { endpoint: 'https://vcenter.lab/sdk', dc: 'Datacenter', credentials: { name: 'c', namespace: 'techday' } } };
    const u = await open({ '/api/v1/harvester/vmwaresources/techday/vcenter-lab': full });
    await nav(u, 'vCenter Sources');
    await click(u, inRow('vcenter-lab', 'Edit'));
    expect(requests(u.calls, 'GET', '/api/v1/harvester/vmwaresources/techday/vcenter-lab')).toHaveLength(1);
    expect(screen.getByDisplayValue('vcenter-lab')).toBeInTheDocument();
  });

  test('OVA source', async () => {
    const full = { metadata: { name: 'ova-source', namespace: 'labs' }, spec: { url: 'http://files.lab/exports', credentials: { name: 'ova-creds', namespace: 'labs' } } };
    const u = await open({ '/api/v1/harvester/ovasources/labs/ova-source': full });
    await nav(u, 'OVA Sources');
    await click(u, inRow('ova-source', 'Edit'));
    expect(requests(u.calls, 'GET', '/api/v1/harvester/ovasources/labs/ova-source')).toHaveLength(1);
    expect(screen.getByDisplayValue('ova-source')).toBeInTheDocument();
  });

  test('Forklift provider', async () => {
    const full = { metadata: { name: 'vsphere-lab', namespace: 'forklift' }, spec: { type: 'vsphere', url: 'https://vcenter.lab/sdk', secret: { name: 's', namespace: 'forklift' }, settings: { sdkEndpoint: 'vcenter' } } };
    const u = await open({ '/api/v1/forklift/providers/forklift/vsphere-lab': full });
    await nav(u, 'vCenter Sources');
    await click(u, screen.getAllByRole('button', { name: 'Forklift' })[0]);
    await click(u, inRow('vsphere-lab', 'Edit'));
    expect(requests(u.calls, 'GET', '/api/v1/forklift/providers/forklift/vsphere-lab')).toHaveLength(1);
    expect(screen.getByDisplayValue('vsphere-lab')).toBeInTheDocument();
  });

  test('a failed fetch alerts and opens nothing', async () => {
    const u = await open({ '/api/v1/harvester/vmwaresources/techday/vcenter-lab': reply(500, { error: 'boom' }) });
    await nav(u, 'vCenter Sources');
    await click(u, inRow('vcenter-lab', 'Edit'));
    expect(window.alert).toHaveBeenCalledWith(expect.stringContaining('boom'));
    expect(screen.queryByDisplayValue('vcenter-lab')).not.toBeInTheDocument();
  });
});

describe('Forklift provider details return to the page they were opened from', () => {
  const cases = [
    { page: 'vCenter Sources', row: 'vsphere-lab', heading: 'Forklift vSphere Providers' },
    { page: 'OVA Sources', row: 'ova-nfs', heading: 'Forklift OVA Providers' },
  ];
  cases.forEach(({ page, row, heading }) => {
    test(page, async () => {
      const u = await open({ '/api/v1/forklift/providers/forklift/vsphere-lab/yaml': 'kind: Provider\n', '/api/v1/forklift/providers/forklift/ova-nfs/yaml': 'kind: Provider\n' });
      await nav(u, page);
      await click(u, screen.getAllByRole('button', { name: 'Forklift' })[0]);
      await click(u, within(screen.getAllByText(row)[0].closest('tr')).getByText('Details'));
      expect(screen.queryByText(heading)).not.toBeInTheDocument();
      await click(u, screen.getByRole('button', { name: /Close/ }));
      expect(screen.getByText(heading)).toBeInTheDocument();
    });
  });
});

describe('the Forklift provider wizard opens preset for the page it is opened from', () => {
  [['vCenter Sources', 'vsphere'], ['OVA Sources', 'ova']].forEach(([page, type]) => {
    test(`${page}: ${type}`, async () => {
      const u = await open();
      await nav(u, page);
      await click(u, screen.getAllByRole('button', { name: 'Forklift' })[0]);
      await click(u, screen.getByRole('button', { name: 'Create' }));
      const radios = screen.getAllByRole('radio');
      expect(radios.find((r) => r.checked).value).toBe(type);
    });
  });
});

test('saving an edited VMIC plan PUTs the changes, closes the modal and refetches', async () => {
  const u = await open({ 'PUT /api/v1/plans/labs/db-migration': ok });
  await click(u, inRow('db-migration', 'Edit Plan'));
  const before = requests(u.calls, 'GET', '/api/v1/plans').length;
  await click(u, screen.getByText('Save'));
  const puts = requests(u.calls, 'PUT', '/api/v1/plans/labs/db-migration');
  expect(puts).toHaveLength(1);
  expect(JSON.parse(puts[0].body)).toEqual(expect.objectContaining({ virtualMachineName: expect.any(String) }));
  expect(requests(u.calls, 'GET', '/api/v1/plans').length).toBeGreaterThan(before);
  expect(screen.queryByText('Save')).not.toBeInTheDocument();
});

// A plan the UI offers to run: Ready, and not yet Succeeded.
const readyPlan = { ...forkliftPlans[0], status: { conditions: [{ type: 'Ready', status: 'True' }] } };
const withReadyPlan = { '/api/v1/forklift/plans': [readyPlan, forkliftPlans[1]] };

describe('running a Forklift migration', () => {
  const toForklift = async (u) => click(u, screen.getAllByRole('button', { name: 'Forklift' })[0]);

  test('with no migration yet: starts it and refreshes', async () => {
    const u = await open({
      ...withReadyPlan,
      '/api/v1/forklift/plans/forklift/migrate-web/migration': reply(404, { error: 'none' }),
      'POST /api/v1/forklift/plans/forklift/migrate-web/run': ok,
    });
    await toForklift(u);
    const before = requests(u.calls, 'GET', '/api/v1/forklift/plans').length;
    await click(u, inRow('migrate-web', 'Run Migration'));
    expect(requests(u.calls, 'POST', '/migrate-web/run')).toHaveLength(1);
    expect(window.alert).toHaveBeenCalledWith('Migration started successfully!');
    expect(requests(u.calls, 'GET', '/api/v1/forklift/plans').length).toBeGreaterThan(before);
  });

  test('with an existing migration: asks, deletes it, then starts a new one', async () => {
    const u = await open({
      ...withReadyPlan,
      'GET /api/v1/forklift/plans/forklift/migrate-web/migration': { metadata: { name: 'old-run' } },
      'DELETE /api/v1/forklift/plans/forklift/migrate-web/migration': ok,
      'POST /api/v1/forklift/plans/forklift/migrate-web/run': ok,
    });
    await toForklift(u);
    await click(u, inRow('migrate-web', 'Run Migration'));
    expect(window.confirm).toHaveBeenCalledWith(expect.stringContaining('old-run'));
    expect(requests(u.calls, 'DELETE', '/migrate-web/migration')).toHaveLength(1);
    expect(requests(u.calls, 'POST', '/migrate-web/run')).toHaveLength(1);
  });

  test('declining the confirmation changes nothing', async () => {
    window.confirm = jest.fn(() => false);
    const u = await open({ ...withReadyPlan, 'GET /api/v1/forklift/plans/forklift/migrate-web/migration': { metadata: { name: 'old-run' } } });
    await toForklift(u);
    await click(u, inRow('migrate-web', 'Run Migration'));
    expect(requests(u.calls, 'DELETE', '/migration')).toHaveLength(0);
    expect(requests(u.calls, 'POST', '/run')).toHaveLength(0);
  });
});

describe('auto-refresh', () => {
  // The polling interval is observed, not waited for: setInterval is replaced by a
  // recorder so a test can see which interval is live, with what period, and run its
  // callback on demand.
  let live;
  beforeEach(() => {
    live = new Map();
    let id = 0;
    jest.spyOn(window, 'setInterval').mockImplementation((fn, ms) => { id += 1; live.set(id, { fn, ms }); return id; });
    jest.spyOn(window, 'clearInterval').mockImplementation((i) => { live.delete(i); });
  });
  afterEach(() => { jest.restoreAllMocks(); });

  const polls = () => [...live.values()].filter((i) => i.ms >= 1000);
  const fire = async (u) => { await act(async () => { polls().forEach((i) => i.fn()); }); await settle(u.calls); };
  const count = (u, part) => requests(u.calls, 'GET', part).length;

  test('one polling interval, 10 s by default', async () => {
    await open();
    expect(polls().map((i) => i.ms)).toEqual([10000]);
  });

  test('a poll refetches the plans, and the Forklift lists when Forklift is available', async () => {
    const u = await open();
    const p0 = count(u, '/api/v1/plans'); const f0 = count(u, '/api/v1/forklift/plans'); const g0 = count(u, '/api/v1/forklift/providers');
    await fire(u);
    expect(count(u, '/api/v1/plans')).toBe(p0 + 1);
    expect(count(u, '/api/v1/forklift/plans')).toBe(f0 + 1);
    expect(count(u, '/api/v1/forklift/providers')).toBe(g0 + 1);
    await fire(u);
    expect(count(u, '/api/v1/plans')).toBe(p0 + 2);
  });

  test('a poll leaves Forklift alone when it is unavailable', async () => {
    const u = await open({ '/api/v1/forklift/availability': { available: false } });
    const f0 = count(u, '/api/v1/forklift/plans'); const p0 = count(u, '/api/v1/plans');
    await fire(u);
    expect(count(u, '/api/v1/plans')).toBe(p0 + 1);
    expect(count(u, '/api/v1/forklift/plans')).toBe(f0);
  });

  test('switching it off makes the polls do nothing', async () => {
    const u = await open();
    fireEvent.click(screen.getByLabelText('Auto-refresh'));
    await settle(u.calls);
    const p0 = count(u, '/api/v1/plans');
    await fire(u);
    expect(count(u, '/api/v1/plans')).toBe(p0);
  });

  test('a changed interval replaces the old one', async () => {
    const u = await open();
    fireEvent.change(screen.getAllByRole('spinbutton')[0], { target: { value: '3' } });
    await settle(u.calls);
    expect(polls().map((i) => i.ms)).toEqual([3000]);
  });

  test('leaving no interval behind on unmount', async () => {
    const u = await open();
    u.unmount();
    expect(polls()).toEqual([]);
  });
});
