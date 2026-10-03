import { useState, useRef, useEffect, useCallback } from 'react';
import { getForkliftPlanStatus, ForkliftStatusBadge } from './status';
import { CheckCircle2, XCircle, Loader, AlertTriangle, Cpu, MemoryStick, HardDrive, Network, RefreshCw, ArrowRight, Server, RotateCcw, X, Play } from 'lucide-react';
import { formatDate, formatBytes, formatDuration } from '../../utils';
import { DownloadButton } from '../../shared/DownloadButton';
import { CopyButton } from '../../shared/CopyButton';

export const ForkliftPlanDetails = ({ plan, onClose, onRunMigration, forkliftNamespace }) => {
    const [activeTab, setActiveTab] = useState('overview');
    const [logs, setLogs] = useState('');
    const [yamlContent, setYamlContent] = useState('');
    const [yamlObject, setYamlObject] = useState('plan');
    const [debugMode, setDebugMode] = useState('logs'); // 'logs' or 'yaml'
    const [isLoadingDebug, setIsLoadingDebug] = useState(false);
    const [onlyRelevantLogs, setOnlyRelevantLogs] = useState(false);
    const [errorsOnlyLogs, setErrorsOnlyLogs] = useState(false);
    const [followLogs, setFollowLogs] = useState(true);
    const followLogsRef = useRef(true);
    const [fontSize, setFontSize] = useState(10);
    const logsEndRef = useRef(null);
    const [networkMap, setNetworkMap] = useState(null);
    const [storageMap, setStorageMap] = useState(null);
    const [migration, setMigration] = useState(null);
    const [provider, setProvider] = useState(null);

    // Keep ref in sync with state
    useEffect(() => { followLogsRef.current = followLogs; }, [followLogs]);

    // Fetch all related objects
    const fetchRelatedObjects = useCallback(() => {
        const netRef = plan.spec?.map?.network;
        const stRef = plan.spec?.map?.storage;
        const provRef = plan.spec?.provider?.source;
        if (netRef?.name && netRef?.namespace) {
            fetch(`/api/v1/forklift/networkmaps/${netRef.namespace}/${netRef.name}`)
                .then(r => r.ok ? r.json() : null).then(setNetworkMap).catch(() => {});
        }
        if (stRef?.name && stRef?.namespace) {
            fetch(`/api/v1/forklift/storagemaps/${stRef.namespace}/${stRef.name}`)
                .then(r => r.ok ? r.json() : null).then(setStorageMap).catch(() => {});
        }
        if (provRef?.name && provRef?.namespace) {
            fetch(`/api/v1/forklift/providers/${provRef.namespace}/${provRef.name}`)
                .then(r => r.ok ? r.json() : null).then(setProvider).catch(() => {});
        }
        fetch(`/api/v1/forklift/plans/${plan.metadata.namespace}/${plan.metadata.name}/migration`)
            .then(r => r.ok ? r.json() : null).then(data => {
                if (data && !data.message) setMigration(data);
            }).catch(() => {});
    }, [plan]);

    useEffect(() => { fetchRelatedObjects(); }, [fetchRelatedObjects]);

    const fetchLogs = useCallback(async (showAll = !onlyRelevantLogs, errorsFilter = errorsOnlyLogs, isBackground = false) => {
        if (!isBackground) setIsLoadingDebug(true);
        try {
            const ns = forkliftNamespace || plan.metadata.namespace;
            const params = new URLSearchParams({ forkliftNamespace: ns });
            if (showAll) params.set('all', 'true');
            if (errorsFilter) params.set('errors', 'true');
            const response = await fetch(`/api/v1/forklift/plans/${plan.metadata.namespace}/${plan.metadata.name}/logs?${params}`);
            const data = await response.text();
            setLogs(data || "No logs found.");
        } catch (err) {
            setLogs("Failed to fetch logs.");
        } finally {
            if (!isBackground) setIsLoadingDebug(false);
        }
    }, [onlyRelevantLogs, errorsOnlyLogs, plan.metadata.namespace, plan.metadata.name, forkliftNamespace]);

    useEffect(() => {
        let interval;
        if (activeTab === 'debug' && debugMode === 'logs') {
            interval = setInterval(() => {
                if (followLogsRef.current) fetchLogs(!onlyRelevantLogs, errorsOnlyLogs, true);
            }, 2000);
        }
        return () => clearInterval(interval);
    }, [activeTab, debugMode, onlyRelevantLogs, errorsOnlyLogs, fetchLogs]);

    useEffect(() => {
        if (followLogs && logsEndRef.current) {
            logsEndRef.current.scrollIntoView({ behavior: 'smooth' });
        }
    }, [logs, followLogs]);

    const fetchYaml = async (objectType = yamlObject) => {
        setIsLoadingDebug(true);
        try {
            const ns = plan.metadata.namespace;
            const name = plan.metadata.name;
            const urls = {
                plan: `/api/v1/forklift/plans/${ns}/${name}/yaml`,
                networkmap: `/api/v1/forklift/networkmaps/${plan.spec?.map?.network?.namespace || ns}/${plan.spec?.map?.network?.name}/yaml`,
                storagemap: `/api/v1/forklift/storagemaps/${plan.spec?.map?.storage?.namespace || ns}/${plan.spec?.map?.storage?.name}/yaml`,
                migration: `/api/v1/forklift/migrations/${ns}/${name}-migration/yaml`,
                provider: `/api/v1/forklift/providers/${plan.spec?.provider?.source?.namespace || ns}/${plan.spec?.provider?.source?.name}/yaml`,
            };
            const response = await fetch(urls[objectType] || urls.plan);
            const data = await response.ok ? await response.text() : `Failed to fetch ${objectType} YAML (${response.status}).`;
            setYamlContent(data || "Could not generate YAML.");
        } catch (err) {
            setYamlContent(`Failed to fetch ${objectType} YAML.`);
        } finally {
            setIsLoadingDebug(false);
        }
    };

    const handleTabChange = (tab) => {
        setActiveTab(tab);
        if (tab === 'debug' && !logs && debugMode === 'logs') fetchLogs();
        if (tab === 'debug' && !yamlContent && debugMode === 'yaml') fetchYaml();
    };

    const status = getForkliftPlanStatus(plan);
    const isMapReady = (obj) => obj?.status?.conditions?.find(c => c.type === 'Ready')?.status === 'True';

    const detailTabs = [
        { key: 'overview', label: 'Overview' },
        { key: 'mappings', label: 'Mappings' },
        { key: 'migration', label: 'Migration' },
        { key: 'conditions', label: 'Conditions' },
        { key: 'debug', label: 'Debug' },
    ];

    const renderConditionsList = (conditions, emptyMsg) => (
        <div className="space-y-1">
            {(conditions || []).length === 0
                ? <p className="text-xs text-secondary italic">{emptyMsg || 'None'}</p>
                : conditions.map((c, i) => (
                    <div key={i} className="flex items-start text-sm border-b border-gray-50 last:border-0 pb-1">
                        <span className="mt-0.5">
                            {c.status === 'True' ? <CheckCircle2 size={14} className="text-green-500 mr-1" /> : <XCircle size={14} className="text-red-500 mr-1" />}
                        </span>
                        <div className="flex-grow">
                            <div className="flex justify-between items-center">
                                <span className="font-medium text-main">{c.type}</span>
                                {c.lastTransitionTime && <span className="text-[10px] text-secondary opacity-70 font-mono ml-2">{formatDate(c.lastTransitionTime)}</span>}
                            </div>
                            {c.message && <div className="text-secondary text-xs italic">{c.message}</div>}
                            {c.reason && c.reason !== c.type && <div className="text-secondary text-[10px]">Reason: {c.reason}</div>}
                        </div>
                    </div>
                ))
            }
        </div>
    );

    const getAnnotation = (key) => plan.metadata?.annotations?.[key];
    const originalCpu = getAnnotation('migration.harvesterhci.io/original-cpu');
    const originalMemoryMB = getAnnotation('migration.harvesterhci.io/original-memory-mb');
    const originalDiskGB = getAnnotation('migration.harvesterhci.io/original-disk-size-gb');
    const originalDisks = (() => { try { return JSON.parse(getAnnotation('migration.harvesterhci.io/original-disks') || '[]'); } catch { return []; } })();
    const originalNetworks = (() => { try { return JSON.parse(getAnnotation('migration.harvesterhci.io/original-networks') || '[]'); } catch { return []; } })();
    const defaultNicModel = getAnnotation('migration.harvesterhci.io/default-nic-model');
    const hasVmCharacteristics = originalCpu || originalMemoryMB || originalDiskGB || originalDisks.length > 0 || originalNetworks.length > 0;

    const renderOverviewTab = () => (
        <div className="space-y-6">
            <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                <div>
                    <h3 className="text-sm font-bold text-secondary uppercase tracking-wider mb-2">Plan Info</h3>
                    <div className="p-3 bg-app rounded-md border text-sm">
                        <div className="grid grid-cols-2 gap-x-2 gap-y-1.5">
                            <span className="text-secondary">Status:</span>
                            <ForkliftStatusBadge conditions={plan.status?.conditions} />
                            <span className="text-secondary">Namespace:</span>
                            <span className="text-main font-mono text-xs">{plan.metadata.namespace}</span>
                            <span className="text-secondary">Target NS:</span>
                            <span className="text-main font-mono text-xs">{plan.spec?.targetNamespace}</span>
                            <span className="text-secondary">Shared Disks:</span>
                            <span className="text-main">{plan.spec?.migrateSharedDisks ? 'Yes' : 'No'}</span>
                            <span className="text-secondary">Warm Migration:</span>
                            <span className="text-main">{plan.spec?.warm ? 'Yes' : 'No'}</span>
                            {plan.spec?.preserveClusterCpuModel && <>
                                <span className="text-secondary">Preserve CPU Model:</span>
                                <span className="text-main">Yes</span>
                            </>}
                            {plan.spec?.preserveStaticIPs && <>
                                <span className="text-secondary">Preserve Static IPs:</span>
                                <span className="text-main">Yes</span>
                            </>}
                            <span className="text-secondary">Created:</span>
                            <span className="text-main text-xs">{formatDate(plan.metadata.creationTimestamp)}</span>
                            {defaultNicModel && <>
                                <span className="text-secondary">NIC Model:</span>
                                <span className="text-main">{defaultNicModel}</span>
                            </>}
                        </div>
                    </div>
                </div>
                <div>
                    <h3 className="text-sm font-bold text-secondary uppercase tracking-wider mb-2">Related Objects</h3>
                    <div className="p-3 bg-app rounded-md border text-sm space-y-2">
                        <div className="flex items-center gap-2">
                            {provider ? (isMapReady(provider) ? <CheckCircle2 size={14} className="text-green-500" /> : <XCircle size={14} className="text-red-500" />) : <Loader size={14} className="animate-spin text-secondary" />}
                            <span className="text-secondary">Provider:</span>
                            <span className="text-main font-mono text-xs">{plan.spec?.provider?.source?.name}</span>
                        </div>
                        <div className="flex items-center gap-2">
                            {networkMap ? (isMapReady(networkMap) ? <CheckCircle2 size={14} className="text-green-500" /> : <XCircle size={14} className="text-red-500" />) : <Loader size={14} className="animate-spin text-secondary" />}
                            <span className="text-secondary">NetworkMap:</span>
                            <span className="text-main font-mono text-xs">{plan.spec?.map?.network?.name}</span>
                        </div>
                        <div className="flex items-center gap-2">
                            {storageMap ? (isMapReady(storageMap) ? <CheckCircle2 size={14} className="text-green-500" /> : <XCircle size={14} className="text-red-500" />) : <Loader size={14} className="animate-spin text-secondary" />}
                            <span className="text-secondary">StorageMap:</span>
                            <span className="text-main font-mono text-xs">{plan.spec?.map?.storage?.name}</span>
                        </div>
                        {migration && (
                            <div className="flex items-center gap-2">
                                {migration.status?.conditions?.find(c => c.type === 'Succeeded')?.status === 'True'
                                    ? <CheckCircle2 size={14} className="text-green-500" />
                                    : migration.status?.conditions?.find(c => c.type === 'Running')?.status === 'True'
                                    ? <Loader size={14} className="animate-spin text-blue-500" />
                                    : <AlertTriangle size={14} className="text-yellow-500" />}
                                <span className="text-secondary">Migration:</span>
                                <span className="text-main font-mono text-xs">{migration.metadata?.name}</span>
                            </div>
                        )}
                    </div>
                </div>
            </div>
            {hasVmCharacteristics && (
                <div>
                    <h3 className="text-sm font-bold text-secondary uppercase tracking-wider mb-2">Source VM Characteristics</h3>
                    <div className="p-3 bg-app rounded-md border text-sm space-y-3">
                        <div className="flex flex-wrap gap-4">
                            {originalCpu && (
                                <div className="flex items-center gap-1">
                                    <Cpu size={14} className="text-secondary" />
                                    <span className="text-main font-medium">{originalCpu} vCPU</span>
                                </div>
                            )}
                            {originalMemoryMB && (
                                <div className="flex items-center gap-1">
                                    <MemoryStick size={14} className="text-secondary" />
                                    <span className="text-main font-medium">{formatBytes(parseInt(originalMemoryMB) * 1024 * 1024, 0)}</span>
                                </div>
                            )}
                            {originalDiskGB && (
                                <div className="flex items-center gap-1">
                                    <HardDrive size={14} className="text-secondary" />
                                    <span className="text-main font-medium">{originalDiskGB} GB total</span>
                                </div>
                            )}
                        </div>
                        {originalDisks.length > 0 && (
                            <div>
                                <h4 className="text-xs font-semibold text-secondary mb-1">Disks</h4>
                                <div className="space-y-1">
                                    {originalDisks.map((disk, i) => {
                                        const sizeGB = disk.capacityGB || (disk.capacity ? (disk.capacity / (1024 * 1024 * 1024)).toFixed(1) : null);
                                        return (
                                        <div key={i} className="flex items-center text-xs gap-2 p-1 bg-card rounded border">
                                            <HardDrive size={10} className="text-secondary" />
                                            <span className="text-main font-mono">{disk.name || disk.path || `Disk ${i + 1}`}</span>
                                            {sizeGB && <span className="text-secondary ml-auto">{sizeGB} GB</span>}
                                            {disk.busType && <span className="text-secondary text-[10px]">({disk.busType})</span>}
                                        </div>
                                        );
                                    })}
                                </div>
                            </div>
                        )}
                        {originalNetworks.length > 0 && (
                            <div>
                                <h4 className="text-xs font-semibold text-secondary mb-1">Networks</h4>
                                <div className="space-y-1">
                                    {originalNetworks.map((net, i) => (
                                        <div key={i} className="flex items-center text-xs gap-2 p-1 bg-card rounded border">
                                            <Network size={10} className="text-secondary" />
                                            <span className="text-main font-mono">{net.name || net.id || `NIC ${i + 1}`}</span>
                                            {net.id && net.name && <span className="text-secondary ml-1">({net.id})</span>}
                                            {net.mac && <span className="text-secondary ml-auto font-mono">{net.mac}</span>}
                                        </div>
                                    ))}
                                </div>
                            </div>
                        )}
                    </div>
                </div>
            )}
            <div>
                <h3 className="text-sm font-bold text-secondary uppercase tracking-wider mb-2">VMs in Plan</h3>
                <div className="space-y-2">
                    {(plan.spec?.vms || []).map((vm, i) => {
                        const vmStatus = migration?.status?.vms?.find(v => v.id === vm.id);
                        return (
                            <div key={i} className="p-2 bg-app rounded border text-sm">
                                <div className="flex items-center">
                                    <HardDrive size={14} className="mr-2 text-secondary" />
                                    <span className="text-main font-medium">{vm.name}</span>
                                    <span className="text-xs text-secondary ml-2 font-mono">({vm.id})</span>
                                    {vmStatus?.phase && <span className="ml-auto text-xs px-2 py-0.5 rounded-full bg-blue-100 text-blue-800">{vmStatus.phase}</span>}
                                </div>
                                {vmStatus?.pipeline && (
                                    <div className="mt-2 ml-6 space-y-1">
                                        {vmStatus.pipeline.map((step, j) => (
                                            <div key={j} className="flex items-center text-xs gap-2">
                                                {step.phase === 'Completed' ? <CheckCircle2 size={10} className="text-green-500" />
                                                    : step.phase === 'Running' ? <Loader size={10} className="animate-spin text-blue-500" />
                                                    : step.error ? <XCircle size={10} className="text-red-500" />
                                                    : <div className="w-2.5 h-2.5 rounded-full bg-gray-300" />}
                                                <span className="text-secondary">{step.name}</span>
                                                {step.progress?.completed !== undefined && step.progress?.total !== undefined && (
                                                    <span className="text-secondary font-mono">({step.progress.completed}/{step.progress.total})</span>
                                                )}
                                                {step.error && <span className="text-red-500 text-[10px] italic">{step.error.reasons?.[0] || ''}</span>}
                                            </div>
                                        ))}
                                    </div>
                                )}
                            </div>
                        );
                    })}
                </div>
            </div>
        </div>
    );

    const RefreshButton = () => (
        <div className="flex justify-end mb-2">
            <button onClick={fetchRelatedObjects} title="Refresh" className="text-secondary hover:text-main p-1 rounded hover:bg-app transition-colors">
                <RefreshCw size={16} />
            </button>
        </div>
    );

    const renderMappingsTab = () => (
        <div className="space-y-6">
            <RefreshButton />
            {/* Network Map */}
            <div>
                <h3 className="text-sm font-bold text-secondary uppercase tracking-wider mb-2 flex items-center">
                    <Network size={16} className="mr-2" /> Network Map
                    <span className="ml-2 font-normal font-mono text-xs">({plan.spec?.map?.network?.name})</span>
                    <span className="ml-auto">{networkMap && (isMapReady(networkMap)
                        ? <span className="text-xs text-green-600 flex items-center"><CheckCircle2 size={12} className="mr-1" />Ready</span>
                        : <span className="text-xs text-red-600 flex items-center"><XCircle size={12} className="mr-1" />Not Ready</span>
                    )}</span>
                </h3>
                {networkMap ? (
                    <div className="space-y-2">
                        {(networkMap.spec?.map || []).map((entry, i) => (
                            <div key={i} className="flex items-center text-sm p-2 bg-app rounded border gap-2">
                                <div className="flex-1 font-mono text-xs">
                                    <div className="text-main">{entry.source?.name || entry.source?.id}</div>
                                    {entry.source?.name && entry.source?.id && <div className="text-secondary">{entry.source.id}</div>}
                                </div>
                                <ArrowRight size={14} className="text-secondary flex-shrink-0" />
                                <div className="flex-1 font-mono text-xs">
                                    {entry.destination?.type === 'pod'
                                        ? <span className="text-secondary italic">pod network</span>
                                        : <div>
                                            <span className="text-main">{entry.destination?.namespace}/{entry.destination?.name}</span>
                                            <div className="text-secondary">{entry.destination?.type}</div>
                                          </div>}
                                </div>
                            </div>
                        ))}
                        {(!networkMap.spec?.map || networkMap.spec.map.length === 0) && <p className="text-sm text-secondary italic">No entries.</p>}
                        {networkMap.status?.conditions && (
                            <div className="mt-2 p-2 bg-card rounded border">
                                <p className="text-[10px] font-bold text-secondary uppercase mb-1">NetworkMap Conditions</p>
                                {renderConditionsList(networkMap.status.conditions)}
                            </div>
                        )}
                    </div>
                ) : <p className="text-sm text-secondary italic">Loading...</p>}
            </div>
            {/* Storage Map */}
            <div>
                <h3 className="text-sm font-bold text-secondary uppercase tracking-wider mb-2 flex items-center">
                    <HardDrive size={16} className="mr-2" /> Storage Map
                    <span className="ml-2 font-normal font-mono text-xs">({plan.spec?.map?.storage?.name})</span>
                    <span className="ml-auto">{storageMap && (isMapReady(storageMap)
                        ? <span className="text-xs text-green-600 flex items-center"><CheckCircle2 size={12} className="mr-1" />Ready</span>
                        : <span className="text-xs text-red-600 flex items-center"><XCircle size={12} className="mr-1" />Not Ready</span>
                    )}</span>
                </h3>
                {storageMap ? (
                    <div className="space-y-2">
                        {(storageMap.spec?.map || []).map((entry, i) => {
                            const resolved = storageMap.status?.references?.find(r => r.id === entry.source?.id);
                            return (
                                <div key={i} className="flex items-center text-sm p-2 bg-app rounded border gap-2">
                                    <div className="flex-1 font-mono text-xs">
                                        <div className="text-main">{resolved?.name || entry.source?.name || entry.source?.id}</div>
                                        {entry.source?.id && <div className="text-secondary">{entry.source.id}</div>}
                                    </div>
                                    <ArrowRight size={14} className="text-secondary flex-shrink-0" />
                                    <div className="flex-1 font-mono text-xs">
                                        <span className="text-main">{entry.destination?.storageClass || 'N/A'}</span>
                                        {entry.destination?.volumeMode && <span className="text-secondary ml-2">[{entry.destination.volumeMode}]</span>}
                                        {entry.destination?.accessMode && <span className="text-secondary ml-1">({entry.destination.accessMode})</span>}
                                    </div>
                                </div>
                            );
                        })}
                        {(!storageMap.spec?.map || storageMap.spec.map.length === 0) && <p className="text-sm text-secondary italic">No entries.</p>}
                        {storageMap.status?.conditions && (
                            <div className="mt-2 p-2 bg-card rounded border">
                                <p className="text-[10px] font-bold text-secondary uppercase mb-1">StorageMap Conditions</p>
                                {renderConditionsList(storageMap.status.conditions)}
                            </div>
                        )}
                    </div>
                ) : <p className="text-sm text-secondary italic">Loading...</p>}
            </div>
            {/* Provider */}
            {provider && (
                <div>
                    <h3 className="text-sm font-bold text-secondary uppercase tracking-wider mb-2 flex items-center">
                        <Server size={16} className="mr-2" /> Source Provider
                        <span className="ml-2 font-normal font-mono text-xs">({plan.spec?.provider?.source?.name})</span>
                        <span className="ml-auto">{isMapReady(provider)
                            ? <span className="text-xs text-green-600 flex items-center"><CheckCircle2 size={12} className="mr-1" />Ready</span>
                            : <span className="text-xs text-red-600 flex items-center"><XCircle size={12} className="mr-1" />Not Ready</span>
                        }</span>
                    </h3>
                    <div className="p-3 bg-app rounded-md border text-sm">
                        <div className="grid grid-cols-2 gap-x-2 gap-y-1">
                            <span className="text-secondary">URL:</span>
                            <span className="text-main font-mono text-xs break-all">{provider.spec?.url}</span>
                            <span className="text-secondary">Type:</span>
                            <span className="text-main">{provider.spec?.type || 'vsphere'}</span>
                            {provider.spec?.username && <><span className="text-secondary">User:</span><span className="text-main font-mono text-xs">{provider.spec.username}</span></>}
                        </div>
                    </div>
                </div>
            )}
        </div>
    );

    const renderMigrationTab = () => {
        if (!migration) {
            return <div><RefreshButton /><p className="text-sm text-secondary italic py-4">No migration has been started for this plan. Use "Run Migration" to begin.</p></div>;
        }
        const migVms = migration.status?.vms || [];
        const migConditions = migration.status?.conditions || [];
        const succeeded = migConditions.find(c => c.type === 'Succeeded')?.status === 'True';
        const running = migConditions.find(c => c.type === 'Running')?.status === 'True';
        const failed = migConditions.find(c => c.type === 'Failed')?.status === 'True';
        const overallStatus = succeeded ? 'Succeeded' : failed ? 'Failed' : running ? 'Running' : 'Pending';
        const statusColors = { Succeeded: 'bg-green-100 text-green-800', Failed: 'bg-red-100 text-red-800', Running: 'bg-blue-100 text-blue-800', Pending: 'bg-app text-main' };
        const vmPhaseBadge = (phase) => {
            const colors = phase === 'Completed' ? 'bg-green-100 text-green-800' : phase === 'Running' ? 'bg-blue-100 text-blue-800' : (phase === 'Failed' || phase === 'Error') ? 'bg-red-100 text-red-800' : 'bg-app text-main';
            return <span className={`text-xs px-2 py-0.5 rounded-full ${colors}`}>{phase}</span>;
        };

        // Aggregate errors across VMs
        const vmErrors = migVms.filter(vm => vm.pipeline?.some(s => s.error)).map(vm => ({
            name: vm.name || vm.id,
            errors: vm.pipeline.filter(s => s.error).map(s => ({ step: s.name, reason: s.error.reasons?.[0] || 'Unknown error' })),
        }));

        return (
            <div className="space-y-4">
                <RefreshButton />
                {/* Status Summary */}
                <div className="flex items-center gap-3 p-3 bg-app rounded-md border">
                    <span className={`inline-flex items-center px-3 py-1 rounded-full text-sm font-medium ${statusColors[overallStatus]}`}>
                        {running && <Loader size={14} className="animate-spin mr-1.5" />}
                        {succeeded && <CheckCircle2 size={14} className="mr-1.5" />}
                        {failed && <XCircle size={14} className="mr-1.5" />}
                        {overallStatus}
                    </span>
                    <span className="text-main font-mono text-xs">{migration.metadata?.name}</span>
                    {failed && (
                        <button
                            onClick={async () => {
                                if (!window.confirm('Delete the failed migration and start a new one?')) return;
                                try {
                                    const ns = plan.metadata.namespace;
                                    const name = plan.metadata.name;
                                    const delRes = await fetch(`/api/v1/forklift/plans/${ns}/${name}/migration`, { method: 'DELETE' });
                                    if (!delRes.ok) {
                                        const err = await delRes.json();
                                        throw new Error(err.error || 'Failed to delete migration');
                                    }
                                    const runRes = await fetch(`/api/v1/forklift/plans/${ns}/${name}/run`, { method: 'POST' });
                                    if (!runRes.ok) {
                                        const err = await runRes.json();
                                        throw new Error(err.error || 'Failed to create migration');
                                    }
                                    setMigration(null);
                                    fetchRelatedObjects();
                                } catch (err) {
                                    alert(`Error retrying migration: ${err.message}`);
                                }
                            }}
                            className="inline-flex items-center px-3 py-1 rounded-md text-xs font-medium bg-red-600 hover:bg-red-700 text-white transition-colors"
                            title="Delete the failed migration and start a new one"
                        >
                            <RotateCcw size={12} className="mr-1" /> Retry
                        </button>
                    )}
                    {migration.status?.phase && <span className="text-secondary text-xs ml-auto">Phase: {migration.status.phase}</span>}
                </div>

                {/* Migration Metadata */}
                <div className="p-3 bg-app rounded-md border text-sm">
                    <div className="grid grid-cols-2 gap-x-2 gap-y-1.5">
                        <span className="text-secondary">Created:</span>
                        <span className="text-main text-xs">{formatDate(migration.metadata?.creationTimestamp)}</span>
                        <span className="text-secondary">Started:</span>
                        <span className="text-main text-xs">{migration.status?.started ? formatDate(migration.status.started) : 'N/A'}</span>
                        <span className="text-secondary">Completed:</span>
                        <span className="text-main text-xs">{migration.status?.completed ? formatDate(migration.status.completed) : running ? 'In progress' : 'N/A'}</span>
                        <span className="text-secondary">Duration:</span>
                        <span className="text-main text-xs">
                            {migration.status?.started
                                ? migration.status?.completed
                                    ? formatDuration(migration.status.started, migration.status.completed)
                                    : `${formatDuration(migration.status.started)} (elapsed)`
                                : 'N/A'}
                        </span>
                    </div>
                </div>

                {/* Error Summary */}
                {vmErrors.length > 0 && (
                    <div className="p-3 bg-red-50 rounded-md border border-red-200 text-sm">
                        <div className="flex items-center gap-2 mb-2">
                            <AlertTriangle size={14} className="text-red-600" />
                            <span className="font-medium text-red-800">{vmErrors.length} VM(s) with errors</span>
                        </div>
                        <div className="space-y-1 ml-5">
                            {vmErrors.map((vm, i) => (
                                <div key={i}>
                                    {vm.errors.map((err, j) => (
                                        <div key={j} className="text-red-700 text-xs">
                                            <span className="font-medium">{vm.name}</span>: {err.reason} <span className="text-red-500 italic">(step: {err.step})</span>
                                        </div>
                                    ))}
                                </div>
                            ))}
                        </div>
                    </div>
                )}

                {renderConditionsList(migConditions, 'No migration conditions yet.')}

                {/* VM Progress */}
                {migVms.length > 0 && (
                    <div>
                        <h4 className="text-sm font-bold text-secondary uppercase tracking-wider mb-2">VM Progress</h4>
                        {migVms.map((vm, i) => {
                            const formatProgress = (progress, annotations) => {
                                const unit = annotations?.unit || '';
                                const completed = progress?.completed || 0;
                                const total = progress?.total || 0;
                                if (unit === 'MB' && total > 0) {
                                    const fmtSize = (mb) => mb >= 1024 ? `${(mb / 1024).toFixed(1)} GB` : `${mb} MB`;
                                    return `${fmtSize(completed)} / ${fmtSize(total)}`;
                                }
                                return `${completed}/${total}`;
                            };
                            const pct = (progress) => progress?.total ? (progress.completed / progress.total * 100) : 0;
                            return (
                            <div key={i} className="mb-3 p-3 bg-app rounded border">
                                <div className="flex items-center mb-2">
                                    <HardDrive size={14} className="mr-2 text-secondary" />
                                    <span className="text-main font-medium text-sm">{vm.name || vm.id}</span>
                                    <span className="text-xs text-secondary ml-2 font-mono">({vm.id})</span>
                                    {(vm.newName || vm.targetName) && (vm.newName || vm.targetName) !== vm.name && (
                                        <span className="text-xs text-secondary ml-2">→ {vm.newName || vm.targetName}</span>
                                    )}
                                    {vm.phase && <span className="ml-auto">{vmPhaseBadge(vm.phase)}</span>}
                                </div>
                                <div className="text-[10px] text-secondary ml-6 mb-1 flex flex-wrap gap-x-3">
                                    {vm.started && <span>Started: {formatDate(vm.started)}</span>}
                                    {vm.completed && <span>Completed: {formatDate(vm.completed)}</span>}
                                    {vm.started && <span>Duration: {vm.completed ? formatDuration(vm.started, vm.completed) : `${formatDuration(vm.started)} (elapsed)`}</span>}
                                    {vm.restorePowerState && <span>Power after migration: {vm.restorePowerState}</span>}
                                </div>
                                {vm.pipeline && (
                                    <div className="space-y-2 ml-6">
                                        {vm.pipeline.map((step, j) => (
                                            <div key={j} className="text-xs">
                                                <div className="flex items-center gap-2">
                                                    {step.phase === 'Completed' ? <CheckCircle2 size={11} className="text-green-500" />
                                                        : step.phase === 'Running' ? <Loader size={11} className="animate-spin text-blue-500" />
                                                        : step.error ? <XCircle size={11} className="text-red-500" />
                                                        : <div className="w-[11px] h-[11px] rounded-full border border-gray-300" />}
                                                    <span className="font-medium text-main">{step.name}</span>
                                                    {step.description && <span className="text-secondary hidden md:inline">- {step.description}</span>}
                                                    {step.phase && <span className="ml-auto text-secondary">{step.phase}</span>}
                                                </div>
                                                {step.progress && (step.progress.total > 0) && (
                                                    <div className="ml-5 mt-0.5">
                                                        <div className="w-full bg-gray-200 rounded-full h-1.5">
                                                            <div className={`h-1.5 rounded-full transition-all ${step.phase === 'Completed' ? 'bg-green-500' : 'bg-blue-500'}`} style={{ width: `${pct(step.progress)}%` }} />
                                                        </div>
                                                        <span className="text-[10px] text-secondary">{formatProgress(step.progress, step.annotations)}</span>
                                                        {step.started && (
                                                            <span className="text-[10px] text-secondary ml-2">
                                                                {step.completed ? formatDuration(step.started, step.completed) : `${formatDuration(step.started)} elapsed`}
                                                            </span>
                                                        )}
                                                    </div>
                                                )}
                                                {/* Per-task breakdown (e.g. individual disks) */}
                                                {step.tasks && step.tasks.length > 0 && (
                                                    <div className="ml-5 mt-1 space-y-1 border-l-2 border-gray-200 pl-3">
                                                        {step.tasks.map((task, k) => (
                                                            <div key={k} className="text-[10px]">
                                                                <div className="flex items-center gap-1.5">
                                                                    {task.phase === 'Completed' ? <CheckCircle2 size={9} className="text-green-500 shrink-0" />
                                                                        : task.phase === 'Running' ? <Loader size={9} className="animate-spin text-blue-500 shrink-0" />
                                                                        : task.error ? <XCircle size={9} className="text-red-500 shrink-0" />
                                                                        : <div className="w-[9px] h-[9px] rounded-full border border-gray-300 shrink-0" />}
                                                                    <span className="text-main font-mono truncate" title={task.name}>{task.name}</span>
                                                                    {task.phase && <span className="ml-auto text-secondary shrink-0">{task.phase}</span>}
                                                                </div>
                                                                {task.progress && task.progress.total > 0 && (
                                                                    <div className="ml-3 mt-0.5">
                                                                        <div className="w-full bg-gray-200 rounded-full h-1">
                                                                            <div className={`h-1 rounded-full transition-all ${task.phase === 'Completed' ? 'bg-green-500' : 'bg-blue-500'}`} style={{ width: `${pct(task.progress)}%` }} />
                                                                        </div>
                                                                        <span className="text-secondary">{formatProgress(task.progress, task.annotations)}</span>
                                                                    </div>
                                                                )}
                                                                {task.reason && <div className="ml-3 text-secondary italic">{task.reason}</div>}
                                                                {task.error && (
                                                                    <div className="ml-3 text-red-500">{task.error.reasons?.map((r, l) => <div key={l}>{r}</div>)}</div>
                                                                )}
                                                            </div>
                                                        ))}
                                                    </div>
                                                )}
                                                {step.error && (
                                                    <div className="ml-5 mt-0.5 text-red-500 text-[10px]">
                                                        {step.error.reasons?.map((r, k) => <div key={k}>{r}</div>)}
                                                    </div>
                                                )}
                                            </div>
                                        ))}
                                    </div>
                                )}
                            </div>
                            );
                        })}
                    </div>
                )}
            </div>
        );
    };

    const renderConditionsTab = () => (
        <div className="space-y-6">
            <RefreshButton />
            <div>
                <h3 className="text-sm font-bold text-secondary uppercase tracking-wider mb-2">Plan Conditions</h3>
                {renderConditionsList(plan.status?.conditions, 'No plan conditions reported.')}
            </div>
            {networkMap && (
                <div>
                    <h3 className="text-sm font-bold text-secondary uppercase tracking-wider mb-2">NetworkMap Conditions ({plan.spec?.map?.network?.name})</h3>
                    {renderConditionsList(networkMap.status?.conditions, 'No conditions.')}
                </div>
            )}
            {storageMap && (
                <div>
                    <h3 className="text-sm font-bold text-secondary uppercase tracking-wider mb-2">StorageMap Conditions ({plan.spec?.map?.storage?.name})</h3>
                    {renderConditionsList(storageMap.status?.conditions, 'No conditions.')}
                </div>
            )}
            {migration && (
                <div>
                    <h3 className="text-sm font-bold text-secondary uppercase tracking-wider mb-2">Migration Conditions ({migration.metadata?.name})</h3>
                    {renderConditionsList(migration.status?.conditions, 'No conditions.')}
                </div>
            )}
            {provider && (
                <div>
                    <h3 className="text-sm font-bold text-secondary uppercase tracking-wider mb-2">Provider Conditions ({plan.spec?.provider?.source?.name})</h3>
                    {renderConditionsList(provider.status?.conditions, 'No conditions.')}
                </div>
            )}
        </div>
    );

    const yamlObjects = [
        { key: 'plan', label: 'Plan' },
        { key: 'networkmap', label: 'NetworkMap' },
        { key: 'storagemap', label: 'StorageMap' },
        { key: 'migration', label: 'Migration' },
        { key: 'provider', label: 'Provider' },
    ];

    const renderDebugTab = () => (
        <div>
            <div className="flex items-center space-x-4 mb-3 border-b pb-2">
                <button onClick={() => { setDebugMode('logs'); if (!logs) fetchLogs(); }}
                    className={`text-sm font-medium pb-1 ${debugMode === 'logs' ? 'text-blue-600 border-b-2 border-blue-600' : 'text-secondary hover:text-main'}`}>
                    Logs
                </button>
                <button onClick={() => { setDebugMode('yaml'); if (!yamlContent) fetchYaml(yamlObject); }}
                    className={`text-sm font-medium pb-1 ${debugMode === 'yaml' ? 'text-blue-600 border-b-2 border-blue-600' : 'text-secondary hover:text-main'}`}>
                    YAML
                </button>
                <div className="flex items-center space-x-4 ml-auto">
                    <div className="flex items-center text-xs space-x-2">
                        <button onClick={() => setFontSize(Math.max(6, fontSize - 1))} className="text-secondary hover:text-main font-bold px-1.5 bg-app rounded border hover:bg-gray-200">-A</button>
                        <span className="text-secondary font-mono w-8 text-center">{fontSize}px</span>
                        <button onClick={() => setFontSize(Math.min(24, fontSize + 1))} className="text-secondary hover:text-main font-bold px-1.5 bg-app rounded border hover:bg-gray-200">+A</button>
                    </div>
                    {debugMode === 'logs' && <>
                        <label className="flex items-center text-xs text-secondary opacity-70 cursor-pointer">
                            <input type="checkbox" checked={followLogs} onChange={e => setFollowLogs(e.target.checked)} className="mr-1 h-3 w-3" /> Follow
                        </label>
                        <label className="flex items-center text-xs text-secondary opacity-70 cursor-pointer">
                            <input type="checkbox" checked={onlyRelevantLogs} onChange={e => { setOnlyRelevantLogs(e.target.checked); fetchLogs(!e.target.checked, errorsOnlyLogs, false); }} className="mr-1 h-3 w-3" /> Only relevant
                        </label>
                        <label className="flex items-center text-xs text-secondary opacity-70 cursor-pointer">
                            <input type="checkbox" checked={errorsOnlyLogs} onChange={e => { setErrorsOnlyLogs(e.target.checked); fetchLogs(!onlyRelevantLogs, e.target.checked, false); }} className="mr-1 h-3 w-3" /> Errors only
                        </label>
                        <button onClick={() => fetchLogs(!onlyRelevantLogs, errorsOnlyLogs, false)} className="text-secondary hover:text-main" title="Refresh logs"><RefreshCw size={14} /></button>
                    </>}
                </div>
            </div>
            {debugMode === 'yaml' && (
                <div className="flex items-center space-x-1 mb-2">
                    {yamlObjects.map(obj => (
                        <button key={obj.key}
                            onClick={() => { setYamlObject(obj.key); fetchYaml(obj.key); }}
                            className={`px-3 py-1 text-xs font-medium rounded-t-md transition-colors ${yamlObject === obj.key
                                ? 'bg-gray-800 text-white border border-gray-600 border-b-0'
                                : 'bg-app text-secondary hover:text-main border border-main'}`}>
                            {obj.label}
                        </button>
                    ))}
                    <button onClick={() => fetchYaml(yamlObject)} className="ml-2 text-secondary hover:text-main" title="Refresh YAML"><RefreshCw size={14} /></button>
                </div>
            )}
            <div className="p-4 border rounded-md bg-gray-900 text-white font-mono max-h-96 overflow-y-auto shadow-inner group relative" style={{ fontSize: `${fontSize}px` }}>
                <div className="absolute top-2 right-2 flex gap-2">
                    <DownloadButton text={debugMode === 'logs' ? logs : yamlContent} filename={debugMode === 'logs' ? `${plan.metadata.name}-logs.txt` : `${plan.metadata.name}-${yamlObject}.yaml`} />
                    <CopyButton text={debugMode === 'logs' ? logs : yamlContent} />
                </div>
                {isLoadingDebug ? (
                    <div className="flex items-center space-x-3 p-4">
                        <Loader className="animate-spin text-blue-400" size={18} />
                        <span className="text-secondary opacity-70">Loading...</span>
                    </div>
                ) : (
                    <>
                        <pre className="whitespace-pre-wrap leading-relaxed">{debugMode === 'logs' ? logs : yamlContent}</pre>
                        <div ref={logsEndRef} />
                    </>
                )}
            </div>
        </div>
    );

    return (
        <div className="fixed inset-0 bg-opacity-50 flex justify-center items-center p-4 z-50">
            <div className="bg-card rounded-lg shadow-xl w-[850px] max-w-[95vw] min-h-[50vh] flex flex-col max-h-[95vh] resize overflow-hidden">
                <div className="flex justify-between items-center p-4 border-b">
                    <div className="flex items-center gap-3">
                        <h2 className="text-xl font-semibold text-main">{plan.metadata.name}</h2>
                        <ForkliftStatusBadge conditions={plan.status?.conditions} />
                    </div>
                    <button onClick={onClose} className="p-2 rounded-full hover:bg-gray-200"><X size={20} /></button>
                </div>
                {/* Tab bar */}
                <div className="flex space-x-1 px-4 pt-2 border-b border-main bg-app">
                    {detailTabs.map(tab => (
                        <button key={tab.key} onClick={() => handleTabChange(tab.key)}
                            className={`px-4 py-2 text-sm font-medium transition-colors rounded-t-md ${activeTab === tab.key
                                ? 'bg-card border border-main border-b-0 text-blue-600 -mb-px'
                                : 'text-secondary hover:text-main hover:bg-card/50'}`}>
                            {tab.label}
                        </button>
                    ))}
                </div>
                <div className="p-6 overflow-y-auto flex-1">
                    {activeTab === 'overview' && renderOverviewTab()}
                    {activeTab === 'mappings' && renderMappingsTab()}
                    {activeTab === 'migration' && renderMigrationTab()}
                    {activeTab === 'conditions' && renderConditionsTab()}
                    {activeTab === 'debug' && renderDebugTab()}
                </div>
                <div className="p-4 border-t bg-app flex justify-between items-center rounded-b-lg">
                    {status === 'Ready' && (
                        <button onClick={() => onRunMigration(plan)} className="bg-green-500 hover:bg-green-600 text-white font-semibold py-2 px-4 rounded-md flex items-center">
                            <Play size={16} className="mr-2" /> Run Migration
                        </button>
                    )}
                    <div className="flex-grow" />
                    <button onClick={onClose} className="btn-secondary px-4 py-2 rounded-md font-semibold transition-colors">Close</button>
                </div>
            </div>
        </div>
    );
};
