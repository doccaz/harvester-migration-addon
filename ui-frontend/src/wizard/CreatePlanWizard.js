import { useState, useEffect, useMemo } from 'react';
import { slugify, buildVmicPlan, vmImportNameError } from '../utils';
import { RefreshCw, Loader, AlertTriangle, ArrowRight } from 'lucide-react';
import { VmDetailsPanel } from '../inventory/VmDetailsPanel';
import { FilterableInventoryTree } from '../inventory/InventoryTree';
import { Header } from '../shared/Header';

// --- UPDATED WIZARD COMPONENT ---
export const CreatePlanWizard = ({ onCancel, onCreatePlan, capabilities, forkliftAvailable, forkliftNamespace }) => {
    const [step, setStep] = useState(1);
    const [engine, setEngine] = useState('vmic'); // 'vmic' or 'forklift'
    const [sourceType, setSourceType] = useState('vmware');
    const [vmwareSources, setVmwareSources] = useState([]);
    const [ovaSources, setOvaSources] = useState([]);
    const [selectedSource, setSelectedSource] = useState("");
    const [vcenterInventory, setVcenterInventory] = useState(null);
    const [isConnecting, setIsConnecting] = useState(false);
    const [connectionError, setConnectionError] = useState('');
    const [selectedVm, setSelectedVm] = useState(null);
    const [planName, setPlanName] = useState('');
    const [targetNamespace, setTargetNamespace] = useState('');
    const [newNamespace, setNewNamespace] = useState('');
    const [namespaces, setNamespaces] = useState([]);
    const [existingVmNames, setExistingVmNames] = useState([]);
    const [vmNameConflict, setVmNameConflict] = useState(false);
    const [ovaVmName, setOvaVmName] = useState('');

    // Updated state for mappings
    const [networkMappings, setNetworkMappings] = useState({});
    const [networkModels, setNetworkModels] = useState({});

    const [harvesterNetworks, setHarvesterNetworks] = useState([]);
    const [storageClass, setStorageClass] = useState('');
    const [storageClasses, setStorageClasses] = useState([]);

    // New state for advanced options
    const [forcePowerOff, setForcePowerOff] = useState(false);
    const [shutdownTimeout, setShutdownTimeout] = useState('');
    const [defaultModel, setDefaultModel] = useState('');

    // NEW: States for v1.6 features
    const [skipPreflight, setSkipPreflight] = useState(false);
    const [diskBus, setDiskBus] = useState('');

    // Forklift-specific state
    const [forkliftProviders, setForkliftProviders] = useState([]);
    const [forkliftTargetName, setForkliftTargetName] = useState('');
    const [migrateSharedDisks, setMigrateSharedDisks] = useState(true);
    const [populatorLabels, setPopulatorLabels] = useState(true);
    const [warmMigration, setWarmMigration] = useState(false);
    const [preserveClusterCpuModel, setPreserveClusterCpuModel] = useState(false);
    const [preserveStaticIPs, setPreserveStaticIPs] = useState(false);
    const [forkliftStorageMappings, setForkliftStorageMappings] = useState({});
    const [forkliftVolumeModes, setForkliftVolumeModes] = useState({});
    const [forkliftAccessModes, setForkliftAccessModes] = useState({});
    const [selectedProviderType, setSelectedProviderType] = useState('vsphere'); // 'vsphere' or 'ova'
    const [ovaInventory, setOvaInventory] = useState(null); // OVA provider inventory (flat VM list)

    const fetchNamespaces = () => {
        fetch('/api/v1/harvester/namespaces')
            .then(res => res.json())
            .then(data => setNamespaces(data.map(ns => ns.metadata.name)))
            .catch(err => console.error("Failed to fetch namespaces:", err));
    };

    const fetchSources = () => {
        fetch('/api/v1/harvester/vmwaresources')
            .then(res => res.json())
            .then(data => setVmwareSources(data || []))
            .catch(err => console.error("Failed to fetch VmwareSources:", err));
    };

    const fetchOvaSources = () => {
        fetch('/api/v1/harvester/ovasources')
            .then(res => res.json())
            .then(data => setOvaSources(data || []))
            .catch(err => console.error("Failed to fetch OvaSources:", err));
    };

    const fetchForkliftProvidersList = () => {
        fetch('/api/v1/forklift/providers')
            .then(res => res.json())
            .then(data => setForkliftProviders(data || []))
            .catch(err => console.error("Failed to fetch Forklift Providers:", err));
    };

    const fetchNetworks = () => {
        fetch('/api/v1/harvester/vlanconfigs')
            .then(res => res.json())
            .then(data => {
                console.log("Raw VLAN data from API:", JSON.stringify(data, null, 2));
                if (!Array.isArray(data)) {
                    console.warn("VLAN data is not an array:", data);
                    setHarvesterNetworks([]);
                    return;
                }
                const networks = data.map(net => {
                    const ns = net?.metadata?.namespace || 'default';
                    const name = net?.metadata?.name || 'unknown';
                    return ns + "/" + name;
                }).filter(Boolean);
                console.log("Parsed Harvester networks:", networks);
                setHarvesterNetworks(networks);
            })
            .catch(err => {
                console.error("Failed to fetch networks:", err);
                setHarvesterNetworks([]);
            });
    };

    const fetchStorageClasses = () => {
        fetch('/api/v1/harvester/storageclasses').then(res => res.json()).then(data => setStorageClasses(data.map(sc => sc.metadata.name)));
    };

    const fetchVmsInNamespace = async (namespace) => {
        if (!namespace) {
            setExistingVmNames([]);
            return;
        }
        try {
            const response = await fetch(`/api/v1/harvester/virtualmachines/${namespace}`);
            const data = await response.json();
            setExistingVmNames(data.map(vm => vm.metadata.name));
        } catch (err) {
            console.error("Failed to fetch VMs in namespace:", err);
        }
    };

    useEffect(() => {
        fetchSources();
        fetchOvaSources();
        fetchNamespaces();
        fetchNetworks();
        fetchStorageClasses();
        if (forkliftAvailable) fetchForkliftProvidersList();
    }, [forkliftAvailable]);

    useEffect(() => {
        fetchVmsInNamespace(targetNamespace);
    }, [targetNamespace]);

    useEffect(() => {
        const nameToCheck = sourceType === 'ova' ? ovaVmName : (selectedVm ? selectedVm.name : '');
        if (nameToCheck && existingVmNames.includes(nameToCheck)) {
            setVmNameConflict(true);
        } else {
            setVmNameConflict(false);
        }
    }, [selectedVm, ovaVmName, sourceType, existingVmNames]);

    // Auto-suggest RFC-1123 compliant target name for Forklift when VM name is non-compliant
    useEffect(() => {
        if (engine !== 'forklift' || !selectedVm?.name) {
            setForkliftTargetName('');
            return;
        }
        const rfcRegex = /^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/;
        const vmName = selectedVm.name;
        if (rfcRegex.test(vmName) && vmName.length <= 63) {
            setForkliftTargetName('');
        } else {
            const slugified = slugify(vmName);
            setForkliftTargetName(slugified);
        }
    }, [selectedVm, engine]);

    const handleSourceChange = async (sourceIdentifier) => {
        setSelectedSource(sourceIdentifier);
        setSelectedVm(null);
        setOvaInventory(null);
        if (!sourceIdentifier) {
            setVcenterInventory(null);
            return;
        }

        if (sourceType === 'ova' && engine === 'vmic') {
            return;
        }

        const [namespace, name] = sourceIdentifier.split('/');

        // Detect OVA provider type from the provider list
        if (engine === 'forklift') {
            const provider = forkliftProviders.find(p => `${p.metadata.namespace}/${p.metadata.name}` === sourceIdentifier);
            const provType = provider?.spec?.type || 'vsphere';
            setSelectedProviderType(provType);

            if (provType === 'ova') {
                setIsConnecting(true);
                setConnectionError('');
                try {
                    const response = await fetch(`/api/v1/forklift/inventory/ova/${namespace}/${name}/vms`);
                    if (!response.ok) {
                        const errData = await response.json();
                        throw new Error(errData.error || "Failed to fetch OVA inventory");
                    }
                    const vms = await response.json();
                    setOvaInventory(vms || []);
                } catch (error) {
                    setConnectionError(error.message);
                } finally {
                    setIsConnecting(false);
                }
                return;
            }
        }

        setIsConnecting(true);
        setConnectionError('');
        try {
            // Use different API endpoint depending on the engine
            const apiUrl = engine === 'forklift'
                ? `/api/v1/forklift/inventory/${namespace}/${name}`
                : `/api/v1/vcenter/inventory/${namespace}/${name}`;
            const response = await fetch(apiUrl);
            if (!response.ok) {
                const errData = await response.json();
                throw new Error(errData.error || "Failed to fetch inventory");
            }
            const data = await response.json();
            setVcenterInventory(data);
        } catch (error) {
            setConnectionError(error.message);
        } finally {
            setIsConnecting(false);
        }
    };

    const sourceNetworks = useMemo(() => {
        if (!selectedVm) return [];
        const networks = selectedVm.networks || [];
        if (engine === 'forklift') {
            // For Forklift, use network ID as key (required for NetworkMap sourceId)
            const seen = new Set();
            return networks.filter(n => {
                const id = n.id || n.name || 'unknown';
                if (seen.has(id)) return false;
                seen.add(id);
                return true;
            }).map(n => ({
                key: n.id || n.name,
                displayName: n.name || n.Name || 'Unknown Network',
                id: n.id || n.name
            }));
        }
        // For VMIC, use network name as key (existing behavior)
        return [...new Set(networks.map(n => n.name || n.Name || 'Unknown Network'))].map(name => ({
            key: name,
            displayName: name
        }));
    }, [selectedVm, engine]);

    // Extract unique datastores from the selected VM for Forklift storage mapping
    // For OVA providers, each disk is a separate storage mapping entry
    const sourceDatastores = useMemo(() => {
        if (!selectedVm || engine !== 'forklift') return [];

        if (selectedProviderType === 'ova') {
            // OVA: per-disk model — each VMDK file is a separate mapping entry
            const disks = selectedVm.disks || [];
            return disks.map((disk, idx) => ({
                id: disk.id || `disk-${idx}`,
                name: disk.name || disk.filePath || `Disk ${idx + 1}`,
                isOvaDisk: true,
            }));
        }

        // vSphere: per-datastore model
        const datastores = [];
        const seen = new Set();
        if (selectedVm.datastoreId) {
            seen.add(selectedVm.datastoreId);
            datastores.push({
                id: selectedVm.datastoreId,
                name: selectedVm.datastoreName || selectedVm.datastoreId,
            });
        }
        return datastores;
    }, [selectedVm, engine, selectedProviderType]);

    const handleCreateNamespace = async () => {
        if (!newNamespace) {
            alert("New namespace name cannot be empty.");
            return false;
        }
        try {
            const response = await fetch('/api/v1/harvester/namespaces', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ name: newNamespace }),
            });
            if (!response.ok) {
                const err = await response.json();
                throw new Error(`Failed to create namespace: ${err.error}`);
            }
            fetchNamespaces();
            setTargetNamespace(newNamespace);
            setNewNamespace('');
            return true;
        } catch (error) {
            console.error(error);
            alert(error.message);
            return false;
        }
    };

    const handleSubmit = async () => {
        let finalTargetNamespace = targetNamespace;
        if (targetNamespace === 'create_new') {
            const success = await handleCreateNamespace();
            if (!success) return;
            finalTargetNamespace = newNamespace;
        }

        if (!finalTargetNamespace) {
            alert("Please select a target namespace before creating the plan.");
            return;
        }

        const [sourceNamespace, sourceName] = selectedSource.split('/');

        // --- Forklift Plan Creation ---
        if (engine === 'forklift') {
            // Validate target VM name is RFC-1123 compliant
            const rfcRegex = /^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/;
            const vmName = selectedVm?.name || '';
            const effectiveTargetName = forkliftTargetName || vmName;
            if (!rfcRegex.test(effectiveTargetName) || effectiveTargetName.length > 63) {
                alert("The target VM name is not RFC-1123 compliant. Please provide a valid target name (lowercase alphanumeric and hyphens, max 63 characters).");
                return;
            }

            // Use the deduplicated sourceNetworks memo for network mappings
            const networkMappingsForklift = sourceNetworks.map(net => {
                const dest = networkMappings[net.key] || '';
                // Harvester networks come as "namespace/name" — split for Forklift's separate fields
                const destParts = dest ? dest.split('/') : [];
                const destNamespace = destParts.length > 1 ? destParts[0] : '';
                const destName = destParts.length > 1 ? destParts.slice(1).join('/') : dest;
                return {
                    sourceId: net.id || net.key,
                    sourceName: net.displayName,
                    destinationType: dest ? 'multus' : 'pod',
                    destinationName: destName,
                    destinationNamespace: destNamespace,
                };
            });

            // Build storage mappings from the per-datastore/per-disk selections
            const storageMappingsForklift = sourceDatastores.map(ds => {
                const entry = {
                    destinationStorageClass: forkliftStorageMappings[ds.id] || storageClass,
                    volumeMode: forkliftVolumeModes[ds.id] || undefined,
                    accessMode: forkliftAccessModes[ds.id] || undefined,
                };
                // OVA uses source.name (disk filename), vSphere uses source.id (moRef)
                if (ds.isOvaDisk) {
                    entry.sourceName = ds.name;
                    entry.sourceId = ds.id;
                } else {
                    entry.sourceId = ds.id;
                }
                return entry;
            }).filter(sm => sm.destinationStorageClass); // Only include if a storage class is selected

            const forkliftPayload = {
                name: slugify(planName),
                namespace: sourceNamespace,
                providerName: sourceName,
                providerNamespace: sourceNamespace,
                providerType: selectedProviderType,
                hostProviderNamespace: forkliftNamespace || 'forklift',
                targetNamespace: finalTargetNamespace,
                networkMappings: networkMappingsForklift,
                storageMappings: storageMappingsForklift,
                vms: [{ id: selectedVm?.id, name: selectedVm?.name, targetName: forkliftTargetName || undefined }],
                migrateSharedDisks: migrateSharedDisks,
                populatorLabels: populatorLabels,
                warm: selectedProviderType === 'ova' ? false : warmMigration,
                preserveClusterCpuModel,
                preserveStaticIPs,
                defaultNetworkInterfaceModel: defaultModel || undefined,
                sourceVmCpu: selectedVm?.cpu || 0,
                sourceVmMemoryMB: selectedVm?.memoryMB || 0,
                sourceVmDiskSizeGB: selectedVm?.diskSizeGB || 0,
                sourceVmDisks: JSON.stringify(selectedVm?.disks || []),
                sourceVmNetworks: JSON.stringify(selectedVm?.networks || []),
            };

            try {
                const response = await fetch('/api/v1/forklift/plans', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(forkliftPayload),
                });
                if (!response.ok) {
                    const errData = await response.json();
                    throw new Error(errData.error || "Failed to create Forklift plan");
                }
                onCancel(); // Return to plans list
            } catch (err) {
                console.error("Failed to create Forklift plan:", err);
                alert(`Error creating Forklift plan: ${err.message}`);
            }
            return;
        }

        // --- VM Import Controller Plan Creation ---
        const plan = buildVmicPlan({
            sourceType,
            ovaVmName,
            selectedVm,
            planName,
            targetNamespace: finalTargetNamespace,
            sourceName,
            sourceNamespace,
            storageClass,
            networkMappings,
            networkModels,
            capabilities,
            forcePowerOff,
            shutdownTimeout,
            defaultModel,
            skipPreflight,
            diskBus,
        });

        onCreatePlan(plan);
    };

    const renderStepContent = () => {
        switch (step) {
            case 1:
                return (
                    <div className="space-y-4">
                        <h3 className="text-lg font-medium text-main mb-2">Select Source & VM</h3>

                        <div>
                            <label className="block text-sm font-medium text-main mb-1">Migration Engine</label>
                            <div className="flex items-center space-x-6">
                                <label className="inline-flex items-center">
                                    <input type="radio" className="form-radio text-blue-600" name="engine" value="vmic" checked={engine === 'vmic'} onChange={() => { setEngine('vmic'); setSelectedSource(''); setSelectedVm(null); setVcenterInventory(null); setOvaInventory(null); setSourceType('vmware'); setNetworkMappings({}); setForkliftStorageMappings({}); setForkliftVolumeModes({}); setForkliftAccessModes({}); setForkliftTargetName(''); setSelectedProviderType('vsphere'); }} />
                                    <span className="ml-2">VM Import Controller</span>
                                </label>
                                <label className={`inline-flex items-center ${!forkliftAvailable ? 'opacity-50' : ''}`}>
                                    <input type="radio" className="form-radio text-blue-600" name="engine" value="forklift" checked={engine === 'forklift'} disabled={!forkliftAvailable} onChange={() => { setEngine('forklift'); setSelectedSource(''); setSelectedVm(null); setVcenterInventory(null); setOvaInventory(null); setSourceType('vmware'); setNetworkMappings({}); setForkliftStorageMappings({}); setForkliftVolumeModes({}); setForkliftAccessModes({}); setForkliftTargetName(''); setSelectedProviderType('vsphere'); }} />
                                    <span className="ml-2">Forklift{!forkliftAvailable ? ' (unavailable)' : ''}</span>
                                </label>
                            </div>
                        </div>

                        <div>
                            <label className="block text-sm font-medium text-main">Plan Name</label>
                            <input type="text" value={planName} onChange={e => setPlanName(e.target.value)} className="mt-1 block w-full form-input" />
                        </div>

                        {engine === 'vmic' && (
                            <div className="mb-4 flex items-center space-x-6">
                                <label className="inline-flex items-center">
                                    <input type="radio" className="form-radio text-blue-600" name="sourceType" value="vmware" checked={sourceType === 'vmware'} onChange={() => { setSourceType('vmware'); setSelectedSource(''); setSelectedVm(null); setForkliftTargetName(''); }} />
                                    <span className="ml-2">VMware vCenter</span>
                                </label>
                                <label className="inline-flex items-center">
                                    <input type="radio" className="form-radio text-blue-600" name="sourceType" value="ova" checked={sourceType === 'ova'} onChange={() => { setSourceType('ova'); setSelectedSource(''); setOvaVmName(''); }} />
                                    <span className="ml-2">OVA File</span>
                                </label>
                            </div>
                        )}

                        {engine === 'forklift' ? (
                            <div className="space-y-4 p-4 border rounded-md bg-app">
                                <div className="flex items-center">
                                    <label className="block text-sm font-medium text-main flex-grow">Forklift Source Provider</label>
                                    <button onClick={fetchForkliftProvidersList} className="ml-2 text-blue-500 hover:text-blue-700"><RefreshCw size={16} /></button>
                                </div>
                                <select value={selectedSource} onChange={e => handleSourceChange(e.target.value)} className="mt-1 block w-full form-select">
                                    <option value="">Select a provider...</option>
                                    {forkliftProviders.map(provider => (
                                        <option key={provider.metadata.uid} value={`${provider.metadata.namespace}/${provider.metadata.name}`}>
                                            [{provider.spec?.type === 'ova' ? 'OVA' : provider.spec?.settings?.sdkEndpoint === 'esxi' ? 'ESXi' : 'vCenter'}] {provider.metadata.namespace}/{provider.metadata.name}
                                        </option>
                                    ))}
                                </select>
                            </div>
                        ) : sourceType === 'vmware' ? (
                            <div className="space-y-4 p-4 border rounded-md bg-app">
                                <div className="flex items-center">
                                    <label className="block text-sm font-medium text-main flex-grow">vCenter Source</label>
                                    <button onClick={fetchSources} className="ml-2 text-blue-500 hover:text-blue-700"><RefreshCw size={16} /></button>
                                </div>
                                <select value={selectedSource} onChange={e => handleSourceChange(e.target.value)} className="mt-1 block w-full form-select">
                                    <option value="">Select a source...</option>
                                    {vmwareSources.map(source => (
                                        <option key={source.metadata.uid} value={`${source.metadata.namespace}/${source.metadata.name}`}>
                                            {source.metadata.namespace}/{source.metadata.name}
                                        </option>
                                    ))}
                                </select>
                            </div>
                        ) : (
                            <div className="space-y-4 p-4 border rounded-md bg-app">
                                <div className="flex items-center">
                                    <label className="block text-sm font-medium text-main flex-grow">OVA Source</label>
                                    <button onClick={fetchOvaSources} className="ml-2 text-blue-500 hover:text-blue-700"><RefreshCw size={16} /></button>
                                </div>
                                <select value={selectedSource} onChange={e => handleSourceChange(e.target.value)} className="mt-1 block w-full form-select">
                                    <option value="">Select an OVA source...</option>
                                    {ovaSources.map(source => (
                                        <option key={source.metadata.uid} value={`${source.metadata.namespace}/${source.metadata.name}`}>
                                            {source.metadata.namespace}/{source.metadata.name}
                                        </option>
                                    ))}
                                </select>
                                <div>
                                    <label className="block text-sm font-medium text-main">Target VM Name</label>
                                    <input type="text" placeholder="e.g. My-Imported-OVA" value={ovaVmName} onChange={e => setOvaVmName(e.target.value)} className="mt-1 block w-full form-input" />
                                </div>
                            </div>
                        )}

                        {isConnecting && <Loader className="animate-spin mt-4" />}
                        {connectionError && <p className="text-sm text-red-600 mt-2">{connectionError}</p>}

                        {/* OVA inventory: flat VM list */}
                        {engine === 'forklift' && selectedProviderType === 'ova' && ovaInventory && (
                            <div className="mt-4">
                                <div className="border border-main rounded-md bg-card overflow-hidden">
                                    <div className="flex items-center justify-between p-2 bg-app border-b">
                                        <span className="text-sm font-medium text-main">OVA Virtual Machines ({ovaInventory.length})</span>
                                        <button onClick={() => handleSourceChange(selectedSource)} className="text-blue-500 hover:text-blue-700"><RefreshCw size={16} /></button>
                                    </div>
                                    <div className="max-h-80 overflow-y-auto">
                                        {ovaInventory.length === 0 ? (
                                            <p className="p-4 text-sm text-secondary">No VMs found in OVA inventory. The OVA server pod may still be scanning the NFS share.</p>
                                        ) : (
                                            <table className="min-w-full divide-y divide-main">
                                                <thead className="bg-app">
                                                    <tr>
                                                        <th className="px-4 py-2 text-left text-xs font-medium text-secondary uppercase">Name</th>
                                                        <th className="px-4 py-2 text-left text-xs font-medium text-secondary uppercase">CPU</th>
                                                        <th className="px-4 py-2 text-left text-xs font-medium text-secondary uppercase">Memory</th>
                                                        <th className="px-4 py-2 text-left text-xs font-medium text-secondary uppercase">Disks</th>
                                                    </tr>
                                                </thead>
                                                <tbody className="divide-y divide-main">
                                                    {ovaInventory.map(vm => (
                                                        <tr
                                                            key={vm.id}
                                                            className={`cursor-pointer hover:bg-blue-50 ${selectedVm?.id === vm.id ? 'bg-blue-100 border-l-4 border-blue-500' : ''}`}
                                                            onClick={() => setSelectedVm(vm)}
                                                        >
                                                            <td className="px-4 py-2 text-sm text-main font-medium">{vm.name}</td>
                                                            <td className="px-4 py-2 text-sm text-secondary">{vm.cpuCount || vm.cpu || '-'}</td>
                                                            <td className="px-4 py-2 text-sm text-secondary">{vm.memoryMB ? `${vm.memoryMB} MB` : '-'}</td>
                                                            <td className="px-4 py-2 text-sm text-secondary">{vm.disks?.length || 0} disk(s)</td>
                                                        </tr>
                                                    ))}
                                                </tbody>
                                            </table>
                                        )}
                                    </div>
                                </div>
                                {selectedVm && <VmDetailsPanel vm={selectedVm} />}
                            </div>
                        )}

                        {sourceType === 'vmware' && !(engine === 'forklift' && selectedProviderType === 'ova') && vcenterInventory && (
                            <div className="grid grid-cols-1 md:grid-cols-2 gap-4 mt-4">
                                <div className="border border-main rounded-md p-2 h-96 flex flex-col overflow-hidden bg-card">
                                    <div className="flex justify-end shrink-0 mb-1">
                                        <button onClick={() => handleSourceChange(selectedSource)} className="text-blue-500 hover:text-blue-700"><RefreshCw size={16} /></button>
                                    </div>
                                    <div className="flex-grow overflow-hidden">
                                        <FilterableInventoryTree node={vcenterInventory} onVmSelect={setSelectedVm} currentlySelectedVm={selectedVm} />
                                    </div>
                                </div>
                                <VmDetailsPanel vm={selectedVm} />
                            </div>
                        )}
                        {engine === 'forklift' && selectedVm && (() => {
                            const rfcRegex = /^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/;
                            const vmName = selectedVm.name || '';
                            const isCompliant = rfcRegex.test(vmName) && vmName.length <= 63;
                            if (isCompliant) return null;
                            return (
                                <div className="mt-3 p-3 bg-yellow-50 border border-yellow-300 rounded-md">
                                    <div className="flex items-start">
                                        <AlertTriangle className="text-yellow-500 w-5 h-5 mr-2 mt-0.5 shrink-0" />
                                        <div className="flex-1">
                                            <p className="text-sm font-medium text-yellow-800">VM name "{vmName}" is not RFC-1123 compliant</p>
                                            <p className="text-xs text-yellow-700 mt-1">Kubernetes requires names to be lowercase alphanumeric with hyphens, max 63 characters. Please provide a valid target name for the destination VM.</p>
                                            <div className="mt-2">
                                                <label className="block text-xs font-medium text-yellow-800 mb-1">Target VM Name</label>
                                                <input
                                                    type="text"
                                                    value={forkliftTargetName}
                                                    onChange={e => setForkliftTargetName(e.target.value)}
                                                    placeholder="Enter RFC-1123 compliant name"
                                                    className="block w-full form-input text-sm"
                                                />
                                                {forkliftTargetName && !rfcRegex.test(forkliftTargetName) && (
                                                    <p className="text-xs text-red-600 mt-1">Target name is still not RFC-1123 compliant.</p>
                                                )}
                                                {forkliftTargetName && forkliftTargetName.length > 63 && (
                                                    <p className="text-xs text-red-600 mt-1">Target name must be 63 characters or less.</p>
                                                )}
                                            </div>
                                        </div>
                                    </div>
                                </div>
                            );
                        })()}
                        <p className="text-sm text-secondary mt-2">
                            {sourceType === 'ova' ? (ovaVmName && selectedSource ? '1 VM configured for import.' : 'Select source and enter VM name.') : (selectedVm ? '1 VM selected for migration.' : '0 VMs selected.')}
                        </p>
                    </div>
                );
            case 2:
                return (
                    <div>
                        <div className="flex justify-between items-center">
                            <h3 className="text-lg font-medium text-main mb-2">Configuration</h3>
                            <button onClick={() => { fetchNamespaces(); fetchStorageClasses(); }} className="text-blue-500 hover:text-blue-700"><RefreshCw size={16} /></button>
                        </div>
                        <p className="text-sm text-secondary mb-4">Define the migration plan details and target resources.</p>

                        {/* WARNING BANNER if capabilities are missing */}
                        {(!capabilities.hasAdvancedPower && capabilities.harvesterVersion) && (
                            <div className="mb-4 p-4 bg-yellow-50 border border-yellow-200 rounded-md flex items-start">
                                <AlertTriangle className="text-yellow-500 w-5 h-5 mr-2 mt-0.5" />
                                <div>
                                    <h4 className="text-sm font-semibold text-yellow-800">Compatibility Mode</h4>
                                    <p className="text-sm text-yellow-700 mt-1">
                                        You are connected to Harvester <strong>{capabilities.harvesterVersion}</strong>.
                                        Advanced options (Force Power Off, Interface Models, Disk Bus) require Harvester v1.6.0+ and have been hidden.
                                    </p>
                                </div>
                            </div>
                        )}

                        <div className="space-y-4">
                            <div>
                                <label className="block text-sm font-medium text-main">Target Namespace</label>
                                <select value={targetNamespace} onChange={e => setTargetNamespace(e.target.value)} className="mt-1 block w-full form-select" size={Math.min(namespaces.length + 2, 10)}>
                                    <option value="">Select a namespace</option>
                                    <option value="create_new">--- Create New Namespace ---</option>
                                    {namespaces.map(ns => <option key={ns} value={ns}>{ns}</option>)}
                                </select>
                                {targetNamespace === 'create_new' && (
                                    <input
                                        type="text"
                                        value={newNamespace}
                                        onChange={e => setNewNamespace(e.target.value)}
                                        placeholder="Enter new namespace name"
                                        className="mt-2 block w-full form-input"
                                    />
                                )}
                                {vmNameConflict && <p className="text-sm text-red-600 mt-1">A VM with the name "{selectedVm.name}" already exists in this namespace. Please choose a different namespace.</p>}
                            </div>
                            <div>
                                <label className="block text-sm font-medium text-main">Target Storage Class</label>
                                <select value={storageClass} onChange={e => setStorageClass(e.target.value)} className="mt-1 block w-full form-select">
                                    <option value="">Select a Storage Class</option>
                                    {storageClasses.map(sc => <option key={sc} value={sc}>{sc}</option>)}
                                </select>
                            </div>
                        </div>

                        {/* Forklift-specific options */}
                        {engine === 'forklift' && (
                            <div className="mt-6 border-t pt-4">
                                <h4 className="text-md font-medium text-main mb-3">Forklift Options</h4>
                                <div className="grid grid-cols-1 md:grid-cols-2 gap-6 bg-app p-4 rounded-md border">
                                    <div className="flex items-center" title={"When enabled, disks that are shared between multiple VMs (e.g. shared VMDK, multi-writer) will be included in the migration.\n\nIf disabled, shared disks are skipped and only exclusive disks are migrated. Enable this if the VM depends on shared storage that must be preserved."}>
                                        <input
                                            id="migrateSharedDisks"
                                            type="checkbox"
                                            checked={migrateSharedDisks}
                                            onChange={e => setMigrateSharedDisks(e.target.checked)}
                                            className="h-4 w-4 text-blue-600 focus:ring-blue-500 border-main rounded"
                                        />
                                        <label htmlFor="migrateSharedDisks" className="ml-2 block text-sm text-main cursor-help">
                                            Migrate Shared Disks
                                            <p className="text-xs text-secondary mt-0.5">Include disks shared between VMs.</p>
                                        </label>
                                    </div>
                                    <div className="flex items-center" title={"Uses the Kubernetes Volume Populator mechanism (CDI) for disk data transfer instead of the legacy importer approach.\n\nRecommended for Harvester/KubeVirt environments. Populator labels are added to PVCs so the CDI controller can track and manage the data import lifecycle."}>
                                        <input
                                            id="populatorLabels"
                                            type="checkbox"
                                            checked={populatorLabels}
                                            onChange={e => setPopulatorLabels(e.target.checked)}
                                            className="h-4 w-4 text-blue-600 focus:ring-blue-500 border-main rounded"
                                        />
                                        <label htmlFor="populatorLabels" className="ml-2 block text-sm text-main cursor-help">
                                            Populator Labels
                                            <p className="text-xs text-secondary mt-0.5">Use volume populator for data transfer.</p>
                                        </label>
                                    </div>
                                    <div className="flex items-center" title={selectedProviderType === 'ova' ? "Warm migration is not supported for OVA providers." : "Requires VMware Changed Block Tracking (CBT) enabled on the source VM.\n\n1. VM must have no snapshots, or CBT must be enabled before any snapshots are taken.\n2. Enable CBT: VM Settings → Options → Advanced → Configuration Parameters → add ctkEnabled = TRUE.\n3. The vSphere user must have permissions for QueryChangedDiskAreas.\n4. VDDK (Virtual Disk Development Kit) must be available to the Forklift controller."}>
                                        <input
                                            id="warmMigration"
                                            type="checkbox"
                                            checked={selectedProviderType === 'ova' ? false : warmMigration}
                                            onChange={e => setWarmMigration(e.target.checked)}
                                            disabled={selectedProviderType === 'ova'}
                                            className="h-4 w-4 text-blue-600 focus:ring-blue-500 border-main rounded"
                                        />
                                        <label htmlFor="warmMigration" className={`ml-2 block text-sm text-main cursor-help ${selectedProviderType === 'ova' ? 'opacity-50' : ''}`}>
                                            Warm Migration
                                            <p className="text-xs text-secondary mt-0.5">
                                                {selectedProviderType === 'ova' ? 'Not supported for OVA providers.' : 'Pre-copy disks while VM is running, then do a final short cutover.'}
                                            </p>
                                        </label>
                                    </div>
                                    <div className="flex items-center" title={"Preserve the source cluster's CPU model on the target VM.\n\nUseful when migrating VMs that depend on specific CPU features or instruction sets. If disabled, the target VM will use the destination cluster's default CPU model."}>
                                        <input
                                            id="preserveClusterCpuModel"
                                            type="checkbox"
                                            checked={preserveClusterCpuModel}
                                            onChange={e => setPreserveClusterCpuModel(e.target.checked)}
                                            className="h-4 w-4 text-blue-600 focus:ring-blue-500 border-main rounded"
                                        />
                                        <label htmlFor="preserveClusterCpuModel" className="ml-2 block text-sm text-main cursor-help">
                                            Preserve CPU Model
                                            <p className="text-xs text-secondary mt-0.5">Keep source cluster's CPU model on target VM.</p>
                                        </label>
                                    </div>
                                    <div className="flex items-center" title={"Preserve static IP configurations from the source VM.\n\nWhen enabled, Forklift will attempt to maintain the same IP addresses on the migrated VM. Useful for VMs with hardcoded IPs or specific network configurations."}>
                                        <input
                                            id="preserveStaticIPs"
                                            type="checkbox"
                                            checked={preserveStaticIPs}
                                            onChange={e => setPreserveStaticIPs(e.target.checked)}
                                            className="h-4 w-4 text-blue-600 focus:ring-blue-500 border-main rounded"
                                        />
                                        <label htmlFor="preserveStaticIPs" className="ml-2 block text-sm text-main cursor-help">
                                            Preserve Static IPs
                                            <p className="text-xs text-secondary mt-0.5">Maintain source VM's static IP addresses.</p>
                                        </label>
                                    </div>
                                </div>
                            </div>
                        )}

                        {/* Advanced Options Section (VM Import Controller only) */}
                        {engine !== 'forklift' && capabilities.hasAdvancedPower && (
                            <div className="mt-6 border-t pt-4">
                                <h4 className="text-md font-medium text-main mb-3">Advanced Options</h4>
                                <div className="grid grid-cols-1 md:grid-cols-2 gap-6 bg-app p-4 rounded-md border">
                                    <div className="flex items-center">
                                        <input
                                            id="forcePowerOff"
                                            type="checkbox"
                                            checked={forcePowerOff}
                                            onChange={e => setForcePowerOff(e.target.checked)}
                                            className="h-4 w-4 text-blue-600 focus:ring-blue-500 border-main rounded"
                                        />
                                        <label htmlFor="forcePowerOff" className="ml-2 block text-sm text-main">
                                            Force Power Off Source VM
                                            <p className="text-xs text-secondary mt-0.5">Required if VMware Tools is not installed.</p>
                                        </label>
                                    </div>

                                    {/* NEW: Skip Preflight Checks */}
                                    <div className="flex items-center">
                                        <input
                                            id="skipPreflight"
                                            type="checkbox"
                                            checked={skipPreflight}
                                            onChange={e => setSkipPreflight(e.target.checked)}
                                            className="h-4 w-4 text-blue-600 focus:ring-blue-500 border-main rounded"
                                        />
                                        <label htmlFor="skipPreflight" className="ml-2 block text-sm text-main">
                                            Skip Preflight Checks
                                            <p className="text-xs text-secondary mt-0.5">Bypass validation (use with caution).</p>
                                        </label>
                                    </div>

                                    <div>
                                        <label className="block text-sm font-medium text-main">Graceful Shutdown Timeout (Seconds)</label>
                                        <input
                                            type="number"
                                            value={shutdownTimeout}
                                            onChange={e => setShutdownTimeout(e.target.value)}
                                            placeholder="e.g. 300"
                                            className="mt-1 block w-full form-input text-sm"
                                        />
                                    </div>

                                    <div>
                                        <label className="block text-sm font-medium text-main">Default Network Interface Model</label>
                                        <select
                                            value={defaultModel}
                                            onChange={e => setDefaultModel(e.target.value)}
                                            className="mt-1 block w-full form-select text-sm"
                                        >
                                            <option value="">Auto (Default)</option>
                                            <option value="e1000">e1000</option>
                                            <option value="e1000e">e1000e</option>
                                            <option value="ne2k_pci">ne2k_pci</option>
                                            <option value="pcnet">pcnet</option>
                                            <option value="rtl8139">rtl8139</option>
                                            <option value="virtio">virtio</option>
                                        </select>
                                    </div>

                                    {/* NEW: Default Disk Bus Type */}
                                    <div className="md:col-span-2">
                                        <label className="block text-sm font-medium text-main">Default Disk Bus Type</label>
                                        <select
                                            value={diskBus}
                                            onChange={e => setDiskBus(e.target.value)}
                                            className="mt-1 block w-full form-select text-sm"
                                        >
                                            <option value="">Auto (Default)</option>
                                            <option value="virtio">virtio (High Performance)</option>
                                            <option value="scsi">scsi</option>
                                            <option value="sata">sata</option>
                                            <option value="usb">usb</option>
                                        </select>
                                        <p className="text-xs text-secondary mt-1">Specify bus type if automatic detection fails.</p>
                                    </div>
                                </div>
                            </div>
                        )}
                    </div>
                );
            case 3:
                return (
                    <div>
                        <div className="flex items-center">
                            <h3 className="text-lg font-medium text-main mb-2 flex-grow">Network Mapping</h3>
                            <button onClick={fetchNetworks} className="ml-2 text-blue-500 hover:text-blue-700"><RefreshCw size={16} /></button>
                        </div>
                        <p className="text-sm text-secondary mb-4">
                            {sourceType === 'ova'
                                ? 'Select the target Harvester network for the imported VM.'
                                : 'Map source networks to target Harvester networks.'}
                        </p>
                        {harvesterNetworks.length === 0 ? (
                            <p className="text-sm text-secondary">No VLANs defined in Harvester.</p>
                        ) : sourceType === 'ova' ? (
                            <div className="space-y-4">
                                <div className="grid grid-cols-1 md:grid-cols-12 gap-4 items-center border-b pb-4 mb-2">
                                    <div className="md:col-span-5">
                                        <label className="block text-sm font-medium text-main mb-1">Target Harvester Network</label>
                                        <select
                                            value={networkMappings['default'] || ''}
                                            onChange={e => setNetworkMappings(prev => ({ ...prev, 'default': e.target.value }))}
                                            className="form-select w-full text-sm"
                                        >
                                            <option value="">Select Harvester Network</option>
                                            {harvesterNetworks.map(hnet => <option key={hnet} value={hnet}>{hnet}</option>)}
                                        </select>
                                    </div>

                                    {capabilities.hasAdvancedPower && (
                                        <div className="md:col-span-4">
                                            <label className="block text-sm font-medium text-main mb-1">Interface Model</label>
                                            <select
                                                onChange={e => setNetworkModels(prev => ({ ...prev, 'default': e.target.value }))}
                                                className="form-select w-full text-sm text-secondary"
                                                title="Specific Interface Model"
                                            >
                                                <option value="">Default Model</option>
                                                <option value="e1000">e1000</option>
                                                <option value="e1000e">e1000e</option>
                                                <option value="ne2k_pci">ne2k_pci</option>
                                                <option value="pcnet">pcnet</option>
                                                <option value="rtl8139">rtl8139</option>
                                                <option value="virtio">virtio</option>
                                            </select>
                                        </div>
                                    )}
                                </div>
                            </div>
                        ) : (
                            <div className="space-y-4">
                                {sourceNetworks.length === 0 ? (
                                    <p className="text-sm text-secondary italic">No source networks found for the selected VM. You can proceed without network mapping.</p>
                                ) : sourceNetworks.map(net => (
                                    <div key={net.key} className="grid grid-cols-1 md:grid-cols-12 gap-4 items-center border-b pb-4 mb-2 last:border-0 last:pb-0">
                                        <div className="md:col-span-4 font-mono text-sm text-main break-all flex items-center">
                                            <div className="bg-app text-secondary px-2 py-1 rounded text-xs mr-2 border border-main">Source</div>
                                            <div>
                                                {net.displayName}
                                                {engine === 'forklift' && net.id && <div className="text-xs text-secondary">{net.id}</div>}
                                            </div>
                                        </div>
                                        <div className="md:col-span-1 text-center hidden md:block">
                                            <ArrowRight className="mx-auto text-secondary opacity-70" />
                                        </div>
                                        <div className="md:col-span-4">
                                            <select
                                                onChange={e => setNetworkMappings(prev => ({ ...prev, [net.key]: e.target.value }))}
                                                className="form-select w-full text-sm"
                                            >
                                                <option value="">Select Harvester Network</option>
                                                {harvesterNetworks.map(hnet => <option key={hnet} value={hnet}>{hnet}</option>)}
                                            </select>
                                        </div>

                                        {engine !== 'forklift' && capabilities.hasAdvancedPower && (
                                            <div className="md:col-span-3">
                                                <select
                                                    onChange={e => setNetworkModels(prev => ({ ...prev, [net.key]: e.target.value }))}
                                                    className="form-select w-full text-sm text-secondary"
                                                    title="Specific Interface Model"
                                                >
                                                    <option value="">Default Model</option>
                                                    <option value="e1000">e1000</option>
                                                    <option value="e1000e">e1000e</option>
                                                    <option value="ne2k_pci">ne2k_pci</option>
                                                    <option value="pcnet">pcnet</option>
                                                    <option value="rtl8139">rtl8139</option>
                                                    <option value="virtio">virtio</option>
                                                </select>
                                            </div>
                                        )}
                                    </div>
                                ))}
                            </div>
                        )}

                        {/* Forklift Default NIC Model */}
                        {engine === 'forklift' && sourceNetworks.length > 0 && (
                            <div className="mt-4 flex items-center gap-3 p-3 bg-app rounded border">
                                <label className="text-sm text-secondary whitespace-nowrap">Default NIC Model:</label>
                                <select value={defaultModel} onChange={e => setDefaultModel(e.target.value)} className="form-select text-sm w-48">
                                    <option value="">Default (virtio)</option>
                                    <option value="e1000">e1000</option>
                                    <option value="e1000e">e1000e</option>
                                    <option value="virtio">virtio</option>
                                    <option value="rtl8139">rtl8139</option>
                                    <option value="pcnet">pcnet</option>
                                </select>
                                <span className="text-xs text-secondary italic">Stored as Plan annotation</span>
                            </div>
                        )}

                        {/* Forklift Storage Mapping */}
                        {engine === 'forklift' && sourceDatastores.length > 0 && (
                            <div className="mt-6">
                                <h3 className="text-lg font-medium text-main mb-2">Storage Mapping</h3>
                                <p className="text-sm text-secondary mb-4">
                                    {selectedProviderType === 'ova'
                                        ? 'Map each OVA disk file to a target Harvester storage class.'
                                        : 'Map source datastores to target Harvester storage classes.'}
                                </p>
                                <div className="space-y-4">
                                    {sourceDatastores.map(ds => (
                                        <div key={ds.id} className="border-b pb-4 mb-2 last:border-0 last:pb-0">
                                            <div className="grid grid-cols-1 md:grid-cols-12 gap-4 items-center">
                                                <div className="md:col-span-4 font-mono text-sm text-main break-all flex items-center">
                                                    <div className="bg-app text-secondary px-2 py-1 rounded text-xs mr-2 border border-main">Source</div>
                                                    <div>
                                                        {ds.name}
                                                        <div className="text-xs text-secondary">{ds.id}</div>
                                                    </div>
                                                </div>
                                                <div className="md:col-span-1 text-center hidden md:block">
                                                    <ArrowRight className="mx-auto text-secondary opacity-70" />
                                                </div>
                                                <div className="md:col-span-4">
                                                    <select
                                                        value={forkliftStorageMappings[ds.id] || storageClass}
                                                        onChange={e => setForkliftStorageMappings(prev => ({ ...prev, [ds.id]: e.target.value }))}
                                                        className="form-select w-full text-sm"
                                                    >
                                                        <option value="">Select Storage Class</option>
                                                        {storageClasses.map(sc => <option key={sc} value={sc}>{sc}</option>)}
                                                    </select>
                                                </div>
                                                <div className="md:col-span-3 flex gap-2">
                                                    <select
                                                        value={forkliftVolumeModes[ds.id] || ''}
                                                        onChange={e => setForkliftVolumeModes(prev => ({ ...prev, [ds.id]: e.target.value }))}
                                                        className="form-select w-full text-sm"
                                                        title="Volume Mode"
                                                    >
                                                        <option value="">Vol. Mode</option>
                                                        <option value="Block">Block</option>
                                                        <option value="Filesystem">Filesystem</option>
                                                    </select>
                                                    <select
                                                        value={forkliftAccessModes[ds.id] || ''}
                                                        onChange={e => setForkliftAccessModes(prev => ({ ...prev, [ds.id]: e.target.value }))}
                                                        className="form-select w-full text-sm"
                                                        title="Access Mode"
                                                    >
                                                        <option value="">Access Mode</option>
                                                        <option value="ReadWriteOnce">RWO</option>
                                                        <option value="ReadWriteMany">RWX</option>
                                                        <option value="ReadOnlyMany">ROX</option>
                                                    </select>
                                                </div>
                                            </div>
                                        </div>
                                    ))}
                                </div>
                            </div>
                        )}
                    </div>
                );
            case 4:
                return (
                    <div>
                        <h3 className="text-lg font-medium text-main mb-2">Review Plan</h3>
                        <div className="space-y-4 text-sm">
                            <div><strong>Engine:</strong> {engine === 'forklift' ? 'Forklift' : 'VM Import Controller'}</div>
                            <div><strong>Plan Name:</strong> {planName}</div>
                            <div><strong>Destination VM Name:</strong> {sourceType === 'ova' ? ovaVmName : (engine === 'forklift' && forkliftTargetName ? `${forkliftTargetName} (original: ${selectedVm?.name})` : selectedVm?.name)}</div>
                            {engine !== 'forklift' && vmImportNameError(sourceType === 'ova' ? ovaVmName : selectedVm?.name) && (
                                <div className="p-2 bg-red-50 rounded border border-red-200 text-red-700 flex items-start">
                                    <AlertTriangle size={14} className="mr-2 mt-0.5 shrink-0" />
                                    <span>{vmImportNameError(sourceType === 'ova' ? ovaVmName : selectedVm?.name)}</span>
                                </div>
                            )}
                            {engine !== 'forklift' && <div><strong>Source Type:</strong> {sourceType === 'ova' ? 'OVA' : 'VMware vCenter'}</div>}
                            <div><strong>Target Namespace:</strong> {targetNamespace === 'create_new' ? `${newNamespace} (new)` : targetNamespace}</div>
                            <div><strong>Storage Class:</strong> {storageClass}</div>

                            {/* Review Forklift Options */}
                            {engine === 'forklift' && (
                                <div className="mt-2 p-2 bg-blue-50 rounded border border-blue-100">
                                    <h4 className="font-medium text-blue-800">Forklift Settings:</h4>
                                    <ul className="list-disc list-inside pl-2 text-blue-800">
                                        <li>VM ID: {selectedVm?.id}</li>
                                        <li>Migrate Shared Disks: {migrateSharedDisks ? 'Yes' : 'No'}</li>
                                        <li>Populator Labels: {populatorLabels ? 'Yes' : 'No'}</li>
                                        <li>Warm Migration: {warmMigration ? 'Yes' : 'No'}</li>
                                        {preserveClusterCpuModel && <li>Preserve CPU Model: Yes</li>}
                                        {preserveStaticIPs && <li>Preserve Static IPs: Yes</li>}
                                        {defaultModel && <li>Default NIC Model: {defaultModel}</li>}
                                    </ul>
                                    {sourceNetworks.length > 0 && (
                                        <div className="mt-2">
                                            <h5 className="font-medium text-blue-800 text-xs">Network Mappings:</h5>
                                            <ul className="list-disc list-inside pl-2 text-blue-800 text-xs">
                                                {sourceNetworks.map(net => (
                                                    <li key={net.key}>{net.displayName} ({net.id}) → {networkMappings[net.key] || '(pod network)'}</li>
                                                ))}
                                            </ul>
                                        </div>
                                    )}
                                    {sourceDatastores.length > 0 && (
                                        <div className="mt-2">
                                            <h5 className="font-medium text-blue-800 text-xs">Storage Mappings:</h5>
                                            <ul className="list-disc list-inside pl-2 text-blue-800 text-xs">
                                                {sourceDatastores.map(ds => {
                                                    const vm = forkliftVolumeModes[ds.id];
                                                    const am = forkliftAccessModes[ds.id];
                                                    const extra = [vm, am].filter(Boolean).join(', ');
                                                    return <li key={ds.id}>{ds.name} ({ds.id}) → {forkliftStorageMappings[ds.id] || storageClass || '(none)'}{extra ? ` [${extra}]` : ''}</li>;
                                                })}
                                            </ul>
                                        </div>
                                    )}
                                </div>
                            )}

                            {/* Review Advanced Options (VMIC only) */}
                            {engine !== 'forklift' && capabilities.hasAdvancedPower && (forcePowerOff || shutdownTimeout || defaultModel || skipPreflight || diskBus) && (
                                <div className="mt-2 p-2 bg-yellow-50 rounded border border-yellow-100">
                                    <h4 className="font-medium text-yellow-800">Advanced Settings:</h4>
                                    <ul className="list-disc list-inside pl-2 text-yellow-800">
                                        {forcePowerOff && <li>Force Power Off: Enabled</li>}
                                        {shutdownTimeout && <li>Shutdown Timeout: {shutdownTimeout}s</li>}
                                        {defaultModel && <li>Default Interface: {defaultModel}</li>}
                                        {skipPreflight && <li>Skip Validation: Yes</li>}
                                        {diskBus && <li>Disk Bus: {diskBus}</li>}
                                    </ul>
                                </div>
                            )}

                            <div>
                                <h4 className="font-medium mt-2">VM to Migrate:</h4>
                                <ul className="list-disc list-inside pl-4">
                                    <li>
                                        {sourceType === 'ova' ? ovaVmName : selectedVm?.name}
                                        {engine === 'forklift' && selectedVm?.id && <span className="text-secondary text-xs"> (ID: {selectedVm.id})</span>}
                                        {engine !== 'forklift' && sourceType !== 'ova' && <span className="text-secondary text-xs"> (Folder: {selectedVm?.folder || '/'})</span>}
                                    </li>
                                </ul>
                            </div>
                            <div>
                                <h4 className="font-medium mt-2">Network Mappings:</h4>
                                <ul className="list-disc list-inside pl-4">
                                    {Object.entries(networkMappings).map(([key, value]) => (
                                        <li key={key}>
                                            {key} &rarr; {value}
                                            {engine !== 'forklift' && capabilities.hasAdvancedPower && networkModels[key] && <span className="text-xs text-secondary ml-2">[{networkModels[key]}]</span>}
                                        </li>
                                    ))}
                                </ul>
                            </div>
                        </div>
                    </div>
                );
            default: return null;
        }
    };

    return (
        <div>
            <Header title="Create Migration Plan" />
            <div className="bg-card p-6 shadow-md rounded-lg">
                {renderStepContent()}
            </div>
            <div className="mt-6 flex justify-between">
                <button onClick={onCancel} className="btn-secondary">Cancel</button>
                <div>
                    {step > 1 && <button onClick={() => setStep(s => s - 1)} className="btn-secondary mr-2">Back</button>}
                    {step < 4 && (
                        <button
                            onClick={() => {
                                if (step === 1) {
                                    const dnsRegex = /^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$/;
                                    if (!planName) {
                                        alert("Plan name is required.");
                                        return;
                                    }
                                    if (!dnsRegex.test(planName)) {
                                        alert("Plan name must consist of lower case alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character.");
                                        return;
                                    }
                                    if (planName.length > 63) {
                                        alert("Plan name must be no more than 63 characters.");
                                        return;
                                    }
                                }
                                setStep(s => s + 1);
                            }}
                            disabled={vmNameConflict}
                            className="bg-blue-500 hover:bg-blue-600 text-white font-semibold py-2 px-4 rounded-md disabled:opacity-50"
                        >
                            Next
                        </button>
                    )}
                    {step === 4 && <button onClick={handleSubmit} className="bg-green-500 hover:bg-green-600 text-white font-semibold py-2 px-4 rounded-md">Create Plan</button>}
                </div>
            </div>
        </div>
    );
};
