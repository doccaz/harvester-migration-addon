import { useState, useMemo, useEffect } from 'react';
import { vmImportNameError } from '../../utils';
import { AlertTriangle, ArrowRight } from 'lucide-react';

export const EditVmicPlanModal = ({ plan, onCancel, onSave, capabilities = {} }) => {
    const [vmName, setVmName] = useState(plan.spec?.virtualMachineName || '');
    const [storageClass, setStorageClass] = useState(plan.spec?.storageClass || '');
    const [folder, setFolder] = useState(plan.spec?.folder || '');
    const [forcePowerOff, setForcePowerOff] = useState(!!plan.spec?.forcePowerOff);
    const [skipPreflight, setSkipPreflight] = useState(!!plan.spec?.skipPreflightChecks);
    const [shutdownTimeout, setShutdownTimeout] = useState(
        plan.spec?.gracefulShutdownTimeoutSeconds ? String(plan.spec.gracefulShutdownTimeoutSeconds) : ''
    );
    const [defaultModel, setDefaultModel] = useState(plan.spec?.defaultNetworkInterfaceModel || '');
    const [diskBus, setDiskBus] = useState(plan.spec?.defaultDiskBusType || '');
    const [storageClasses, setStorageClasses] = useState([]);
    const [harvesterNetworks, setHarvesterNetworks] = useState([]);

    // Source NICs are recorded on the plan when it's created (annotation), so we can
    // rebuild the network-mapping table without re-fetching vCenter inventory.
    const sourceNetworks = useMemo(() => {
        try {
            const raw = JSON.parse(plan.metadata?.annotations?.['migration.harvesterhci.io/original-networks'] || '[]');
            return [...new Set(raw.map(n => n.name || n.Name).filter(Boolean))];
        } catch { return []; }
    }, [plan]);

    const [networkMappings, setNetworkMappings] = useState(() => {
        const init = {};
        (plan.spec?.networkMapping || []).forEach(m => { if (m.sourceNetwork) init[m.sourceNetwork] = m.destinationNetwork || ''; });
        return init;
    });
    const [networkModels, setNetworkModels] = useState(() => {
        const init = {};
        (plan.spec?.networkMapping || []).forEach(m => { if (m.sourceNetwork && m.networkInterfaceModel) init[m.sourceNetwork] = m.networkInterfaceModel; });
        return init;
    });

    useEffect(() => {
        fetch('/api/v1/harvester/storageclasses')
            .then(res => res.json())
            .then(data => setStorageClasses(data.map(sc => sc.metadata.name)))
            .catch(() => {});
        fetch('/api/v1/harvester/vlanconfigs')
            .then(res => res.json())
            .then(data => setHarvesterNetworks(Array.isArray(data) ? data.map(n => `${n?.metadata?.namespace || 'default'}/${n?.metadata?.name}`) : []))
            .catch(() => setHarvesterNetworks([]));
    }, []);

    const unmappedNetworks = sourceNetworks.filter(n => !networkMappings[n]);

    const handleSave = () => {
        const updates = {
            virtualMachineName: vmName,
            storageClass,
            folder,
            networkMapping: sourceNetworks
                .map(name => ({
                    sourceNetwork: name,
                    destinationNetwork: networkMappings[name] || '',
                    networkInterfaceModel: capabilities.hasAdvancedPower ? (networkModels[name] || undefined) : undefined,
                }))
                .filter(m => m.destinationNetwork),
        };
        if (capabilities.hasAdvancedPower) {
            updates.forcePowerOff = forcePowerOff;
            updates.skipPreflightChecks = skipPreflight;
            updates.gracefulShutdownTimeoutSeconds = shutdownTimeout ? parseInt(shutdownTimeout, 10) : 0;
            updates.defaultNetworkInterfaceModel = defaultModel;
            updates.defaultDiskBusType = diskBus;
        }
        onSave(plan, updates);
    };

    return (
        <div className="fixed inset-0 bg-opacity-50 flex justify-center items-center p-4 z-50">
            <div className="bg-card rounded-lg shadow-xl w-full max-w-lg max-h-[90vh] overflow-y-auto">
                <div className="p-4 border-b">
                    <h2 className="text-xl font-semibold">Edit Plan: {plan.metadata.name}</h2>
                    <p className="text-xs text-secondary mt-1">Adjust plan fields to fix an invalid plan, then re-run it.</p>
                </div>
                <div className="p-6 space-y-4">
                    <div>
                        <label className="block text-sm font-medium text-main">VM Name (source)</label>
                        <input type="text" value={vmName} onChange={e => setVmName(e.target.value)} className="mt-1 block w-full form-input" />
                        <p className="text-xs text-secondary mt-1">Case-sensitive VM name as it appears in vCenter.</p>
                        {vmImportNameError(vmName) && (
                            <p className="text-xs text-red-600 mt-1 flex items-start"><AlertTriangle size={12} className="mr-1 mt-0.5 shrink-0" />{vmImportNameError(vmName)}</p>
                        )}
                    </div>
                    <div>
                        <label className="block text-sm font-medium text-main">Folder</label>
                        <input type="text" value={folder} onChange={e => setFolder(e.target.value)} placeholder="(leave blank for datacenter root)" className="mt-1 block w-full form-input" />
                        <p className="text-xs text-secondary mt-1">vCenter inventory folder path of the source VM.</p>
                    </div>
                    <div>
                        <label className="block text-sm font-medium text-main">Storage Class</label>
                        {storageClasses.length > 0 ? (
                            <select value={storageClass} onChange={e => setStorageClass(e.target.value)} className="mt-1 block w-full form-input">
                                <option value="">Select storage class</option>
                                {storageClasses.map(sc => <option key={sc} value={sc}>{sc}</option>)}
                            </select>
                        ) : (
                            <input type="text" value={storageClass} onChange={e => setStorageClass(e.target.value)} className="mt-1 block w-full form-input" />
                        )}
                    </div>

                    <div>
                        <label className="block text-sm font-medium text-main">Network Mapping</label>
                        {sourceNetworks.length === 0 ? (
                            <p className="text-xs text-secondary mt-1 italic">No source networks recorded on this plan.</p>
                        ) : (
                            <>
                                {unmappedNetworks.length > 0 && (
                                    <p className="text-xs text-orange-600 mt-1 flex items-center"><AlertTriangle size={12} className="mr-1" />Unmapped source networks make the plan invalid.</p>
                                )}
                                <div className="mt-2 space-y-2">
                                    {sourceNetworks.map(net => (
                                        <div key={net} className="flex items-center gap-2">
                                            <span className="font-mono text-xs text-main flex-1 break-all" title={net}>{net}</span>
                                            <ArrowRight size={14} className="text-secondary opacity-70 shrink-0" />
                                            <select value={networkMappings[net] || ''} onChange={e => setNetworkMappings(prev => ({ ...prev, [net]: e.target.value }))} className="form-select text-sm flex-1">
                                                <option value="">Select Harvester Network</option>
                                                {harvesterNetworks.map(hnet => <option key={hnet} value={hnet}>{hnet}</option>)}
                                            </select>
                                            {capabilities.hasAdvancedPower && (
                                                <select value={networkModels[net] || ''} onChange={e => setNetworkModels(prev => ({ ...prev, [net]: e.target.value }))} className="form-select text-sm w-28" title="Interface Model">
                                                    <option value="">Model</option>
                                                    <option value="e1000">e1000</option>
                                                    <option value="e1000e">e1000e</option>
                                                    <option value="ne2k_pci">ne2k_pci</option>
                                                    <option value="pcnet">pcnet</option>
                                                    <option value="rtl8139">rtl8139</option>
                                                    <option value="virtio">virtio</option>
                                                </select>
                                            )}
                                        </div>
                                    ))}
                                </div>
                            </>
                        )}
                    </div>

                    {capabilities.hasAdvancedPower && (
                        <div className="border-t pt-4 space-y-4">
                            <h4 className="text-sm font-medium text-main">Advanced Options</h4>
                            <div className="flex items-center">
                                <input id="editForcePowerOff" type="checkbox" checked={forcePowerOff} onChange={e => setForcePowerOff(e.target.checked)} className="h-4 w-4 text-blue-600 focus:ring-blue-500 border-main rounded" />
                                <label htmlFor="editForcePowerOff" className="ml-2 block text-sm text-main">Force Power Off Source VM</label>
                            </div>
                            <div className="flex items-center">
                                <input id="editSkipPreflight" type="checkbox" checked={skipPreflight} onChange={e => setSkipPreflight(e.target.checked)} className="h-4 w-4 text-blue-600 focus:ring-blue-500 border-main rounded" />
                                <label htmlFor="editSkipPreflight" className="ml-2 block text-sm text-main">Skip Preflight Checks</label>
                            </div>
                            <div>
                                <label className="block text-sm font-medium text-main">Graceful Shutdown Timeout (Seconds)</label>
                                <input type="number" value={shutdownTimeout} onChange={e => setShutdownTimeout(e.target.value)} placeholder="e.g. 300" className="mt-1 block w-full form-input text-sm" />
                            </div>
                            <div>
                                <label className="block text-sm font-medium text-main">Default Network Interface Model</label>
                                <select value={defaultModel} onChange={e => setDefaultModel(e.target.value)} className="mt-1 block w-full form-select text-sm">
                                    <option value="">Auto (Default)</option>
                                    <option value="e1000">e1000</option>
                                    <option value="e1000e">e1000e</option>
                                    <option value="ne2k_pci">ne2k_pci</option>
                                    <option value="pcnet">pcnet</option>
                                    <option value="rtl8139">rtl8139</option>
                                    <option value="virtio">virtio</option>
                                </select>
                            </div>
                            <div>
                                <label className="block text-sm font-medium text-main">Default Disk Bus Type</label>
                                <select value={diskBus} onChange={e => setDiskBus(e.target.value)} className="mt-1 block w-full form-select text-sm">
                                    <option value="">Auto (Default)</option>
                                    <option value="virtio">virtio (High Performance)</option>
                                    <option value="scsi">scsi</option>
                                    <option value="sata">sata</option>
                                    <option value="usb">usb</option>
                                </select>
                            </div>
                        </div>
                    )}
                </div>
                <div className="p-4 border-t flex justify-end space-x-2">
                    <button onClick={onCancel} className="btn-secondary">Cancel</button>
                    <button onClick={handleSave} className="bg-blue-500 hover:bg-blue-600 text-white font-semibold py-2 px-4 rounded-md">Save</button>
                </div>
            </div>
        </div>
    );
};
