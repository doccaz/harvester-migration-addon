// Representative API payloads for rendering every page. Hand-written to cover the
// branches the UI renders (each status, both providers' types, failed/ready exports);
// the shapes follow the CRDs recorded in docs/contract*.md.

const ts = (s) => `2026-09-${s}Z`;
const cond = (type, status, extra = {}) => ({ type, status, lastTransitionTime: ts('20T10:00:00'), ...extra });

export const capabilities = {
  harvesterVersion: '1.8.2',
  hasAdvancedPower: true,
  hasForklift: true,
};

export const vmicPlans = [
  {
    metadata: { name: 'web-migration', namespace: 'techday', uid: 'p1', creationTimestamp: ts('21T09:00:00') },
    spec: { virtualMachineName: 'rhel-8-web', storageClass: 'harvester-longhorn', sourceCluster: { name: 'vcenter-lab', namespace: 'techday', kind: 'VmwareSource', apiVersion: 'migration.harvesterhci.io/v1beta1' }, networkMapping: [{ sourceNetwork: 'VM Network', destinationNetwork: 'default/mgmt' }] },
    status: { importStatus: 'virtualMachineRunning', conditions: [{ type: 'ready', status: 'True', lastUpdateTime: ts('21T09:30:00') }] },
  },
  {
    metadata: { name: 'db-migration', namespace: 'labs', uid: 'p2', creationTimestamp: ts('22T09:00:00') },
    spec: { virtualMachineName: 'sles16-db', storageClass: 'harvester-longhorn', sourceCluster: { name: 'vcenter-lab', namespace: 'techday', kind: 'VmwareSource', apiVersion: 'migration.harvesterhci.io/v1beta1' } },
    status: { importStatus: 'virtualMachineImportInvalid', conditions: [{ type: 'invalid', status: 'False', reason: 'VMNotFound', message: 'virtual machine not found', lastUpdateTime: ts('22T09:05:00') }] },
  },
  {
    metadata: { name: 'fresh-plan', namespace: 'labs', uid: 'p3', creationTimestamp: ts('23T09:00:00') },
    spec: { virtualMachineName: 'new-vm', storageClass: 'harvester-longhorn', sourceCluster: { name: 'ova-source', namespace: 'labs', kind: 'OvaSource', apiVersion: 'migration.harvesterhci.io/v1beta1' } },
  },
];

export const vmwareSources = [
  {
    metadata: { name: 'vcenter-lab', namespace: 'techday', uid: 's1', creationTimestamp: ts('10T08:00:00') },
    spec: { endpoint: 'https://vcenter.lab/sdk', dc: 'Datacenter', credentials: { name: 'vcenter-lab-creds', namespace: 'techday' } },
    status: { status: 'clusterReady', conditions: [cond('ClusterReady', 'True')] },
  },
  {
    metadata: { name: 'vcenter-old', namespace: 'labs', uid: 's2', creationTimestamp: ts('11T08:00:00') },
    spec: { endpoint: 'https://old.lab/sdk', dc: 'DC2', credentials: { name: 'old-creds', namespace: 'labs' } },
    status: { status: 'clusterNotReady', conditions: [cond('ClusterError', 'True', { reason: 'LoginFailed' })] },
  },
];

export const ovaSources = [
  {
    metadata: { name: 'ova-source', namespace: 'labs', uid: 'o1', creationTimestamp: ts('12T08:00:00') },
    spec: { url: 'http://files.lab/exports', allowInsecure: true, httpTimeoutSeconds: 600, credentials: { name: 'ova-creds', namespace: 'labs' } },
    status: { status: 'sourceReady', conditions: [cond('Ready', 'True')] },
  },
];

export const forkliftProviders = [
  {
    metadata: { name: 'vsphere-lab', namespace: 'forklift', uid: 'f1', creationTimestamp: ts('13T08:00:00') },
    spec: { type: 'vsphere', url: 'https://vcenter.lab/sdk', secret: { name: 'vsphere-lab-creds', namespace: 'forklift' }, settings: { sdkEndpoint: 'vcenter' } },
    status: { conditions: [cond('Ready', 'True')] },
  },
  {
    metadata: { name: 'esxi-lab', namespace: 'forklift', uid: 'f2', creationTimestamp: ts('14T08:00:00') },
    spec: { type: 'vsphere', url: 'https://esxi.lab/sdk', secret: { name: 'esxi-creds', namespace: 'forklift' }, settings: { sdkEndpoint: 'esxi' } },
    status: { conditions: [cond('Ready', 'False', { reason: 'ConnectionFailed', message: 'dial tcp: i/o timeout' })] },
  },
  {
    metadata: { name: 'ova-nfs', namespace: 'forklift', uid: 'f3', creationTimestamp: ts('15T08:00:00') },
    spec: { type: 'ova', url: '10.0.0.5:/exports/ova', secret: { name: 'x', namespace: 'forklift' } },
    status: { conditions: [cond('Ready', 'True')] },
  },
];

export const forkliftPlans = [
  {
    metadata: { name: 'migrate-web', namespace: 'forklift', uid: 'fp1', creationTimestamp: ts('16T08:00:00') },
    spec: { provider: { source: { name: 'vsphere-lab', namespace: 'forklift' }, destination: { name: 'host', namespace: 'forklift' } }, targetNamespace: 'techday', map: { network: { name: 'nm', namespace: 'forklift' }, storage: { name: 'sm', namespace: 'forklift' } }, vms: [{ id: 'vm-1', name: 'rhel-web' }] },
    status: { conditions: [cond('Ready', 'True'), cond('Succeeded', 'True')] },
  },
  {
    metadata: { name: 'migrate-db', namespace: 'forklift', uid: 'fp2', creationTimestamp: ts('17T08:00:00') },
    spec: { provider: { source: { name: 'esxi-lab', namespace: 'forklift' }, destination: { name: 'host', namespace: 'forklift' } }, targetNamespace: 'labs', vms: [{ id: 'vm-2', name: 'sles-db' }] },
    status: { conditions: [cond('Ready', 'False', { reason: 'ProviderNotReady' }), cond('Failed', 'True', { reason: 'Failed' })] },
  },
];

export const harvesterInventory = {
  id: 'harvester',
  name: 'Harvester Cluster',
  type: 'datacenter',
  children: [
    {
      id: 'ns-labs', name: 'labs', type: 'namespace',
      children: [
        {
          id: 'labs/downstream-01', name: 'downstream-01', type: 'VirtualMachine', namespace: 'labs', powerState: 'poweredOn',
          cpu: 4, memoryMB: 8192, firmware: 'efi', machineType: 'q35', architecture: 'amd64', runStrategy: 'RerunOnFailure',
          disks: [{ name: 'disk-0', capacity: 42949672960, busType: 'virtio', kind: 'pvc', device: 'disk', pvcName: 'downstream-01-disk-0', storageClass: 'harvester-longhorn', volumeMode: 'Block', bootOrder: 1 }],
          networks: [{ name: 'default', macAddress: '02:00:00:00:00:01', network: 'default/mgmt' }],
          exportBlockers: ['the VM is running; stop it first'],
        },
        {
          id: 'labs/bastion', name: 'bastion', type: 'VirtualMachine', namespace: 'labs', powerState: 'poweredOff',
          cpu: 2, memoryMB: 4096, firmware: 'bios', machineType: 'q35', architecture: 'amd64', runStrategy: 'Halted',
          disks: [
            { name: 'disk-0', capacity: 85899345920, busType: 'virtio', kind: 'pvc', device: 'disk', pvcName: 'bastion-disk-0', storageClass: 'harvester-longhorn', volumeMode: 'Block', bootOrder: 1 },
            { name: 'cloudinit', capacity: 0, busType: 'virtio', kind: 'cloudinit', device: 'disk' },
          ],
          networks: [],
        },
      ],
    },
  ],
};

export const exportsList = [
  { exportId: 'aaa111', namespace: 'labs', jobName: 'vm-export-aaa111', vmName: 'bastion', profile: 'vmware', targetName: 'bastion-aaa111', phase: 'Ready', createdAt: ts('24T08:00:00'), downloadable: true, sizeBytes: 4556208640, percent: 100, stage: 'done' },
  { exportId: 'bbb222', namespace: 'techday', jobName: 'vm-export-bbb222', vmName: 'sles16', profile: 'portable', targetName: 'sles16-bbb222', phase: 'Converting', createdAt: ts('25T08:00:00'), percent: 42, stage: 'converting' },
  { exportId: 'ccc333', namespace: 'labs', jobName: 'vm-export-ccc333', vmName: 'db', profile: 'vmware', targetName: 'db-ccc333', phase: 'Failed', createdAt: ts('26T08:00:00'), error: 'disk read failed' },
];

export const namespaces = [{ name: 'default' }, { name: 'labs' }, { name: 'techday' }];

export const routes = () => ({
  '/api/v1/capabilities': capabilities,
  '/api/v1/plans': vmicPlans,
  '/api/v1/harvester/vmwaresources': vmwareSources,
  '/api/v1/harvester/ovasources': ovaSources,
  '/api/v1/forklift/availability': { available: true, namespace: 'forklift' },
  '/api/v1/forklift/providers': forkliftProviders,
  '/api/v1/forklift/plans': forkliftPlans,
  '/api/v1/harvester/inventory': harvesterInventory,
  '/api/v1/exports': exportsList,
  '/api/v1/harvester/namespaces': namespaces,
  '/api/v1/harvester/storageclasses': [{ metadata: { name: 'harvester-longhorn' } }, { metadata: { name: 'harvester-1replica' } }],
  '/api/v1/harvester/vlanconfigs': [],
});
