import { useState, useRef, useEffect, useCallback } from 'react';
import { X, Cpu, MemoryStick, HardDrive, Folder, Network, RefreshCw, Loader } from 'lucide-react';
import { formatBytes } from '../../utils';
import { DownloadButton } from '../../shared/DownloadButton';
import { CopyButton } from '../../shared/CopyButton';
import { ViewInHarvester } from '../../shared/ViewInHarvester';

export const PlanDetails = ({ plan, onClose }) => {
    const [logs, setLogs] = useState('');
    const [yamlContent, setYamlContent] = useState('');
    const [showDebug, setShowDebug] = useState(null); // 'logs' or 'yaml'
    const [isLoadingDebug, setIsLoadingDebug] = useState(false);
    const [onlyRelevantLogs, setOnlyRelevantLogs] = useState(false);
    const [followLogs, setFollowLogs] = useState(true);
    const followLogsRef = useRef(true);
    const [fontSize, setFontSize] = useState(10); // px
    const logsEndRef = useRef(null);

    useEffect(() => { followLogsRef.current = followLogs; }, [followLogs]);

    const fetchLogs = useCallback(async (showAll = !onlyRelevantLogs, isBackground = false) => {
        if (!isBackground) setIsLoadingDebug(true);
        try {
            const response = await fetch(`/api/v1/plans/${plan.metadata.namespace}/${plan.metadata.name}/logs${showAll ? '?all=true' : ''}`);
            const data = await response.text();
            setLogs(data || "No logs found.");
        } catch (err) {
            setLogs("Failed to fetch logs.");
        } finally {
            if (!isBackground) setIsLoadingDebug(false);
        }
    }, [onlyRelevantLogs, plan.metadata.namespace, plan.metadata.name]);

    useEffect(() => {
        let interval;
        if (showDebug === 'logs') {
            interval = setInterval(() => {
                if (followLogsRef.current) fetchLogs(!onlyRelevantLogs, true);
            }, 2000);
        }
        return () => clearInterval(interval);
    }, [showDebug, onlyRelevantLogs, plan.metadata.namespace, plan.metadata.name, fetchLogs]);

    useEffect(() => {
        if (followLogs && logsEndRef.current) {
            logsEndRef.current.scrollIntoView({ behavior: 'smooth' });
        }
    }, [logs, followLogs]);

    const fetchYaml = async () => {
        setIsLoadingDebug(true);
        try {
            const response = await fetch(`/api/v1/plans/${plan.metadata.namespace}/${plan.metadata.name}/yaml`);
            const data = await response.text();
            setYamlContent(data || "Could not generate YAML.");
        } catch (err) {
            setYamlContent("Failed to fetch YAML.");
        } finally {
            setIsLoadingDebug(false);
        }
    };

    const handleShowDebug = (type) => {
        if (showDebug === type) {
            setShowDebug(null); // Toggle off
            return;
        }
        setShowDebug(type);
        if (type === 'logs') {
            fetchLogs();
        } else if (type === 'yaml') {
            fetchYaml();
        }
    };

    return (
        <div className="fixed inset-0 bg-opacity-50 flex justify-center items-center p-4 z-50">
            <div className="bg-card rounded-lg shadow-xl w-[768px] max-w-[95vw] min-h-[50vh] flex flex-col max-h-[95vh] resize overflow-hidden">
                <div className="flex justify-between items-center p-4 border-b">
                    <h2 className="text-xl font-semibold text-main">{plan.metadata.name}</h2>
                    <button onClick={onClose} className="p-2 rounded-full hover:bg-gray-200">
                        <X size={20} />
                    </button>
                </div>
                <div className="p-6 space-y-6 overflow-y-auto">
                    <ViewInHarvester resource="virtualmachineimport" object={plan} />
                    <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                        <div>
                            <h3 className="text-lg font-medium text-main mb-2">VM Characteristics</h3>
                            <div className="p-3 bg-app rounded-md border text-sm space-y-2">
                                {(() => {
                                    const getAnnotation = (key) => plan.metadata.annotations?.[key];
                                    const originalCpu = getAnnotation('migration.harvesterhci.io/original-cpu');
                                    const originalMemoryMB = getAnnotation('migration.harvesterhci.io/original-memory-mb');
                                    const originalDiskGB = getAnnotation('migration.harvesterhci.io/original-disk-size-gb');

                                    const cpu = plan.status?.cpu || originalCpu;
                                    const memoryMB = plan.status?.memoryMB || originalMemoryMB;
                                    const diskSizeGB = plan.status?.diskImportStatus
                                        ? (plan.status.diskImportStatus.reduce((acc, d) => acc + (d.diskSize || d.size || 0), 0) / (1024 * 1024 * 1024)).toFixed(0)
                                        : originalDiskGB;

                                    const annotationTitle = "Data from original VM characteristics (vCenter source)";

                                    return (
                                        <>
                                            <div className="flex items-center">
                                                <Cpu size={16} className="mr-2 text-secondary" />
                                                <span>
                                                    {cpu || 'N/A'} vCPU(s)
                                                    {plan.status?.cpu && originalCpu && originalCpu !== cpu && (
                                                        <span className="text-secondary opacity-70 ml-1">({originalCpu} originally)</span>
                                                    )}
                                                </span>
                                                {!plan.status?.cpu && originalCpu && (
                                                    <span className="ml-1 text-blue-500 cursor-help" title={annotationTitle}>*</span>
                                                )}
                                            </div>
                                            <div className="flex items-center">
                                                <MemoryStick size={16} className="mr-2 text-secondary" />
                                                <span>
                                                    {memoryMB ? formatBytes(parseInt(memoryMB) * 1024 * 1024, 0) : 'N/A'} Memory
                                                    {plan.status?.memoryMB && originalMemoryMB && originalMemoryMB !== memoryMB && (
                                                        <span className="text-secondary opacity-70 ml-1">({formatBytes(parseInt(originalMemoryMB) * 1024 * 1024, 0)} originally)</span>
                                                    )}
                                                </span>
                                                {!plan.status?.memoryMB && originalMemoryMB && (
                                                    <span className="ml-1 text-blue-500 cursor-help" title={annotationTitle}>*</span>
                                                )}
                                            </div>
                                            <div className="flex items-center">
                                                <HardDrive size={16} className="mr-2 text-secondary" />
                                                <span>
                                                    {diskSizeGB || 'N/A'} GB Storage
                                                    {plan.status?.diskImportStatus && originalDiskGB && originalDiskGB !== diskSizeGB && (
                                                        <span className="text-secondary opacity-70 ml-1">({originalDiskGB} GB originally)</span>
                                                    )}
                                                </span>
                                                {!plan.status?.diskImportStatus && originalDiskGB && (
                                                    <span className="ml-1 text-blue-500 cursor-help" title={annotationTitle}>*</span>
                                                )}
                                            </div>
                                        </>
                                    );
                                })()}
                                <div className="flex items-center">
                                    <Folder size={16} className="mr-2 text-secondary" />
                                    <span>{plan.spec?.folder || '/'}</span>
                                </div>
                                {plan.vms?.[0]?.networks?.[0]?.mac && (
                                    <div className="flex items-center text-xs text-secondary pt-1 border-t">
                                        <Network size={14} className="mr-2" />
                                        <span className="font-mono">Source MAC: {plan.vms[0].networks[0].mac}</span>
                                    </div>
                                )}
                            </div>
                        </div>
                        <div>
                            <h3 className="text-lg font-medium text-main mb-2 font-sans border-b pb-1">Configuration Parameters</h3>
                            <div className="p-3 bg-card rounded-md border shadow-sm text-xs space-y-2">
                                <div className="grid grid-cols-2 gap-x-2 gap-y-1">
                                    <span className="text-secondary font-medium">VM Name:</span>
                                    <span className="text-main break-all font-semibold">{plan.spec?.virtualMachineName || 'N/A'}</span>

                                    <span className="text-secondary font-medium">Source:</span>
                                    <span className="text-main break-all">{plan.spec?.sourceCluster?.namespace}/{plan.spec?.sourceCluster?.name || 'N/A'}</span>

                                    <span className="text-secondary font-medium">Storage Class:</span>
                                    <span className="text-main">{plan.spec?.storageClass || 'N/A'}</span>

                                    <span className="text-secondary font-medium">Force Power Off:</span>
                                    <span className={plan.spec?.forcePowerOff ? "text-orange-600 font-bold" : "text-secondary opacity-70"}>{plan.spec?.forcePowerOff ? "Yes" : "No"}</span>

                                    <span className="text-secondary font-medium">Shutdown Timeout:</span>
                                    <span className="text-main">{plan.spec?.gracefulShutdownTimeoutSeconds || '0'}s</span>

                                    <span className="text-secondary font-medium">Skip Validation:</span>
                                    <span className={plan.spec?.skipPreflightChecks ? "text-blue-600 font-bold" : "text-secondary opacity-70"}>{plan.spec?.skipPreflightChecks ? "Yes" : "No"}</span>

                                    <span className="text-secondary font-medium">Default Disk Bus:</span>
                                    <span className="text-main">{plan.spec?.defaultDiskBusType || 'virtio'}</span>

                                    <span className="text-secondary font-medium">Default Net Model:</span>
                                    <span className="text-main">{plan.spec?.defaultNetworkInterfaceModel || 'virtio'}</span>
                                </div>
                            </div>
                        </div>
                    </div>

                    <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                        <div>
                            <h3 className="text-lg font-medium text-main mb-2 font-sans border-b pb-1">Network Mappings</h3>
                            <div className="bg-card border rounded-lg shadow-sm overflow-hidden text-[10px]">
                                <table className="min-w-full divide-y divide-main">
                                    <thead className="bg-app">
                                        <tr>
                                            <th className="px-3 py-2 text-left font-bold text-secondary uppercase tracking-tight">Source Net</th>
                                            <th className="px-3 py-2 text-left font-bold text-secondary uppercase tracking-tight">Target VLAN</th>
                                            <th className="px-3 py-2 text-left font-bold text-secondary uppercase tracking-tight">Model</th>
                                        </tr>
                                    </thead>
                                    <tbody className="divide-y divide-main">
                                        {(plan.spec?.networkMapping || []).map((net, i) => (
                                            <tr key={i} className="hover:bg-app transition-colors">
                                                <td className="px-3 py-2 text-main">{net.sourceNetwork}</td>
                                                <td className="px-3 py-2 text-blue-600 font-medium">{net.destinationNetwork}</td>
                                                <td className="px-3 py-2 text-secondary font-mono italic">{net.networkInterfaceModel || plan.spec?.defaultNetworkInterfaceModel || 'virtio'}</td>
                                            </tr>
                                        ))}
                                        {(!plan.spec?.networkMapping || plan.spec.networkMapping.length === 0) && (
                                            <tr><td colSpan="3" className="px-3 py-4 text-center text-xs text-secondary italic">No network mappings defined.</td></tr>
                                        )}
                                    </tbody>
                                </table>
                            </div>
                        </div>
                        <div>
                            <h3 className="text-lg font-medium text-main mb-2 font-sans border-b pb-1">Disks</h3>
                            <div className="bg-card border rounded-lg shadow-sm overflow-hidden text-[10px]">
                                <table className="min-w-full divide-y divide-main">
                                    <thead className="bg-app">
                                        <tr>
                                            <th className="px-3 py-2 text-left font-bold text-secondary uppercase tracking-tight">Source Disk</th>
                                            <th className="px-3 py-2 text-left font-bold text-secondary uppercase tracking-tight">Size</th>
                                            <th className="px-3 py-2 text-left font-bold text-secondary uppercase tracking-tight">Storage Class</th>
                                            <th className="px-3 py-2 text-left font-bold text-secondary uppercase tracking-tight">Bus</th>
                                        </tr>
                                    </thead>
                                    <tbody className="divide-y divide-main">
                                        {(plan.status?.diskImportStatus || plan.spec?.disks || []).length > 0 ? (
                                            (plan.status?.diskImportStatus || plan.spec?.disks || []).map((disk, i) => {
                                                const name = disk.diskName || disk.sourceDisk || (i === 0 ? "Root Disk" : `Disk ${i}`);
                                                const size = disk.diskSize ? formatBytes(disk.diskSize) :
                                                    (disk.sizeGB || disk.size ? (disk.sizeGB || disk.size) + ' GB' : 'N/A');
                                                const sc = disk.storageClass || plan.spec?.storageClass || 'default';
                                                const bus = disk.busType || disk.bus || plan.spec?.defaultDiskBusType || 'virtio';

                                                return (
                                                    <tr key={i} className="hover:bg-app transition-colors">
                                                        <td className="px-3 py-2 text-main truncate max-w-[120px]" title={name}>{name}</td>
                                                        <td className="px-3 py-2 text-main font-medium">{size}</td>
                                                        <td className="px-3 py-2 text-xs text-main">{sc}</td>
                                                        <td className="px-3 py-2 text-secondary font-mono italic uppercase">{bus}</td>
                                                    </tr>
                                                );
                                            })
                                        ) : (
                                            <tr className="hover:bg-app transition-colors">
                                                <td className="px-3 py-2 text-main font-medium">Root Disk</td>
                                                <td className="px-3 py-2 text-main font-medium">{plan.vms?.[0]?.diskSizeGB || 'N/A'} GB</td>
                                                <td className="px-3 py-2 text-xs text-main">{plan.spec?.storageClass || 'default'}</td>
                                                <td className="px-3 py-2 text-secondary font-mono italic">{plan.spec?.defaultDiskBusType || 'virtio'}</td>
                                            </tr>
                                        )}
                                    </tbody>
                                </table>
                            </div>
                        </div>
                    </div>
                    <div>
                        <div className="flex space-x-4 border-b">
                            <button
                                onClick={() => handleShowDebug('logs')}
                                className={`pb-2 text-sm font-medium transition-colors ${showDebug === 'logs' ? 'text-blue-600 border-b-2 border-blue-600' : 'text-secondary hover:text-main'}`}
                            >
                                Debug Logs
                            </button>
                            <button
                                onClick={() => handleShowDebug('yaml')}
                                className={`pb-2 text-sm font-medium transition-colors ${showDebug === 'yaml' ? 'text-blue-600 border-b-2 border-blue-600' : 'text-secondary hover:text-main'}`}
                            >
                                View YAML
                            </button>
                            {showDebug && (
                                <div className="flex items-center space-x-4 ml-auto pb-2">
                                    <div className="flex items-center text-xs space-x-2">
                                        <button onClick={() => setFontSize(Math.max(6, fontSize - 1))} className="text-secondary hover:text-main font-bold px-1.5 bg-app rounded border hover:bg-gray-200">-A</button>
                                        <span className="text-secondary font-mono w-8 text-center">{fontSize}px</span>
                                        <button onClick={() => setFontSize(Math.min(24, fontSize + 1))} className="text-secondary hover:text-main font-bold px-1.5 bg-app rounded border hover:bg-gray-200">+A</button>
                                    </div>
                                    {showDebug === 'logs' && <>
                                        <label className="flex items-center text-xs text-secondary opacity-70 cursor-pointer">
                                            <input type="checkbox" checked={followLogs} onChange={(e) => setFollowLogs(e.target.checked)} className="mr-1 h-3 w-3" />
                                            Follow
                                        </label>
                                        <label className="flex items-center text-xs text-secondary opacity-70 cursor-pointer">
                                            <input type="checkbox" checked={onlyRelevantLogs}
                                                onChange={(e) => { const newVal = e.target.checked; setOnlyRelevantLogs(newVal); fetchLogs(!newVal, false); }}
                                                className="mr-1 h-3 w-3" />
                                            Only relevant
                                        </label>
                                        <button onClick={() => fetchLogs(!onlyRelevantLogs, false)} className="text-secondary hover:text-main" title="Refresh logs"><RefreshCw size={14} /></button>
                                    </>}
                                </div>
                            )}
                        </div>
                        {showDebug && (
                            <div className="mt-4 p-4 border rounded-md bg-gray-900 text-white font-mono max-h-96 overflow-y-auto shadow-inner group relative" style={{ fontSize: `${fontSize}px` }}>
                                <div className="absolute top-2 right-2 flex gap-2">
                                    <DownloadButton text={showDebug === 'logs' ? logs : yamlContent} filename={showDebug === 'logs' ? `${plan.metadata.name}-logs.txt` : `${plan.metadata.name}.yaml`} />
                                    <CopyButton text={showDebug === 'logs' ? logs : yamlContent} />
                                </div>
                                {isLoadingDebug ? (
                                    <div className="flex items-center space-x-3 p-4">
                                        <Loader className="animate-spin text-blue-400" size={18} />
                                        <span className="text-secondary opacity-70">Streaming {showDebug}...</span>
                                    </div>
                                ) : (
                                    <>
                                        <pre className="whitespace-pre-wrap leading-relaxed">{showDebug === 'logs' ? logs : yamlContent}</pre>
                                        <div ref={logsEndRef} />
                                    </>
                                )}
                            </div>
                        )}
                    </div>
                </div>
                <div className="p-4 border-t bg-app text-right rounded-b-lg">
                    <button onClick={onClose} className="btn-secondary px-4 py-2 rounded-md font-semibold transition-colors">Close</button>
                </div>
            </div>
        </div>
    );
};
