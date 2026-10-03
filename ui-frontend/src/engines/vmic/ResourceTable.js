import { CheckCircle2, XCircle, HelpCircle, ChevronDown, ChevronRight, Edit, Trash2, Cpu, MemoryStick, HardDrive } from 'lucide-react';
import { SortableHeader } from '../../shared/SortableHeader';
import React from 'react';
import { formatDate, formatBytes } from '../../utils';

export const getPlanStatus = (plan) => {
    if (plan.status?.importStatus) {
        return plan.status.importStatus;
    }
    if (plan.status?.conditions?.[0]?.type) {
        return plan.status.conditions[0].type;
    }
    return 'Pending';
};

export const ResourceTable = ({ plans, onViewDetails, onDelete, onEdit, sortConfig, onSort, expandedPlans, toggleExpand, selectedDisks, setSelectedDisks }) => {
    const renderStatusIcon = (status) => {
        if (status === 'True') return <CheckCircle2 size={14} className="text-green-500 mr-1" />;
        if (status === 'False') return <XCircle size={14} className="text-red-500 mr-1" />;
        return <HelpCircle size={14} className="text-secondary opacity-70 mr-1" />;
    };

    return (
        <div className="bg-card shadow-md rounded-lg overflow-x-auto border border-main">
            <table className="min-w-full divide-y divide-main">
                <thead className="bg-app opacity-90">
                    <tr>
                        <th className="px-4 py-3"></th>
                        <SortableHeader label="Created" sortKey="metadata.creationTimestamp" currentSort={sortConfig} onSort={onSort} />
                        <SortableHeader label="Name" sortKey="metadata.name" currentSort={sortConfig} onSort={onSort} />
                        <SortableHeader label="Status" sortKey="status.importStatus" currentSort={sortConfig} onSort={onSort} />
                        <SortableHeader label="VM Name" sortKey="spec.virtualMachineName" currentSort={sortConfig} onSort={onSort} />
                        <SortableHeader label="Target Namespace" sortKey="metadata.namespace" currentSort={sortConfig} onSort={onSort} />
                        <SortableHeader label="Storage Class" sortKey="spec.storageClass" currentSort={sortConfig} onSort={onSort} />
                        <th className="px-6 py-3 text-left text-xs font-medium text-secondary uppercase tracking-wider"></th>
                    </tr>
                </thead>
                <tbody className="bg-card divide-y divide-main">
                    {plans.length === 0 ? (
                        <tr>
                            <td colSpan="8" className="px-6 py-4 text-center text-sm text-secondary">No migration plans found.</td>
                        </tr>
                    ) : (
                        plans.map(plan => (
                            <React.Fragment key={plan.metadata.uid}>
                                <tr className="hover:bg-app transition-colors">
                                    <td className="px-4 py-4 whitespace-nowrap text-sm text-secondary">
                                        <button onClick={() => toggleExpand(plan.metadata.uid)} className="p-1 hover:bg-app rounded-full transition-colors">
                                            {expandedPlans.has(plan.metadata.uid) ? <ChevronDown size={18} /> : <ChevronRight size={18} />}
                                        </button>
                                    </td>
                                    <td className="px-6 py-4 whitespace-nowrap text-sm text-secondary">{formatDate(plan.metadata.creationTimestamp)}</td>
                                    <td className="px-6 py-4 whitespace-nowrap text-sm font-medium text-main">{plan.metadata.name}</td>
                                    <td className="px-6 py-4 whitespace-nowrap text-sm text-secondary">{getPlanStatus(plan)}</td>
                                    <td className="px-6 py-4 whitespace-nowrap text-sm text-secondary">{plan.spec?.virtualMachineName || 'N/A'}</td>
                                    <td className="px-6 py-4 whitespace-nowrap text-sm text-secondary">{plan.metadata.namespace}</td>
                                    <td className="px-6 py-4 whitespace-nowrap text-sm text-secondary">{plan.spec.storageClass}</td>
                                    <td className="px-6 py-4 whitespace-nowrap text-right text-sm font-medium space-x-2 pr-4">
                                        {(() => {
                                            const status = getPlanStatus(plan);
                                            const canEdit = !['Running', 'Completed', 'ImportCompleted', 'virtualMachineRunning'].includes(status);
                                            return canEdit ? (
                                                <button onClick={() => onEdit(plan)} title="Edit Plan" className="text-blue-600 hover:text-blue-800"><Edit size={18} /></button>
                                            ) : (
                                                <button title="Edit" className="text-secondary opacity-40 cursor-not-allowed"><Edit size={18} /></button>
                                            );
                                        })()}
                                        <button onClick={() => onDelete(plan)} title="Delete" className="text-red-600 hover:text-red-800"><Trash2 size={18} /></button>
                                        <button onClick={() => onViewDetails(plan)} className="text-blue-600 hover:text-blue-800">Details</button>
                                    </td>
                                </tr>
                                {expandedPlans.has(plan.metadata.uid) && (
                                    <tr className="bg-app">
                                        <td colSpan="8" className="px-6 py-4">
                                            {(() => {
                                                const cpu = plan.status?.cpu || plan.metadata.annotations?.['migration.harvesterhci.io/original-cpu'];
                                                const mem = plan.status?.memoryMB || plan.metadata.annotations?.['migration.harvesterhci.io/original-memory-mb'];
                                                const disks = plan.status?.diskImportStatus || plan.status?.diskStatus || plan.status?.planStatus?.disks || [];
                                                const diskSizeGB = disks.length > 0
                                                    ? (disks.reduce((acc, d) => acc + (d.diskSize || d.size || 0), 0) / (1024 * 1024 * 1024)).toFixed(0)
                                                    : plan.metadata.annotations?.['migration.harvesterhci.io/original-disk-size-gb'];

                                                const annotationTitle = "Data from original VM characteristics (vCenter source)";

                                                return (cpu || mem || diskSizeGB) && (
                                                    <div className="mb-4 flex flex-wrap gap-4 text-[11px] p-2 bg-blue-500/10 rounded-md border border-blue-500/20 items-center">
                                                        <span className="font-bold text-blue-700 uppercase tracking-tight">Source Characteristics:</span>
                                                        <div className="flex items-center" title={!plan.status?.cpu && plan.metadata.annotations?.['migration.harvesterhci.io/original-cpu'] ? annotationTitle : undefined}>
                                                            <Cpu size={12} className="mr-1 text-secondary opacity-70" />
                                                            <span>{cpu || 'N/A'} vCPU</span>
                                                            {!plan.status?.cpu && plan.metadata.annotations?.['migration.harvesterhci.io/original-cpu'] && <span className="ml-0.5 text-blue-500 cursor-help">*</span>}
                                                        </div>
                                                        <div className="flex items-center" title={!plan.status?.memoryMB && plan.metadata.annotations?.['migration.harvesterhci.io/original-memory-mb'] ? annotationTitle : undefined}>
                                                            <MemoryStick size={12} className="mr-1 text-secondary opacity-70" />
                                                            <span>{mem ? formatBytes(parseInt(mem) * 1024 * 1024, 0) : 'N/A'}</span>
                                                            {!plan.status?.memoryMB && plan.metadata.annotations?.['migration.harvesterhci.io/original-memory-mb'] && <span className="ml-0.5 text-blue-500 cursor-help">*</span>}
                                                        </div>
                                                        <div className="flex items-center" title={!plan.status?.diskImportStatus && plan.metadata.annotations?.['migration.harvesterhci.io/original-disk-size-gb'] ? annotationTitle : undefined}>
                                                            <HardDrive size={12} className="mr-1 text-secondary opacity-70" />
                                                            <span>
                                                                {diskSizeGB || 'N/A'} GB
                                                                {(() => {
                                                                    const originalDiskGB = plan.metadata.annotations?.['migration.harvesterhci.io/original-disk-size-gb'];
                                                                    if (disks.length > 0 && originalDiskGB && originalDiskGB !== diskSizeGB) {
                                                                        return <span className="text-secondary opacity-70 ml-1">({originalDiskGB} GB originally)</span>;
                                                                    }
                                                                    return null;
                                                                })()}
                                                            </span>
                                                            {!(disks.length > 0) && plan.metadata.annotations?.['migration.harvesterhci.io/original-disk-size-gb'] && <span className="ml-0.5 text-blue-500 cursor-help">*</span>}
                                                        </div>
                                                    </div>
                                                );
                                            })()}
                                            <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                                                <div>
                                                    <h4 className="text-xs font-bold text-secondary uppercase tracking-wider mb-2">Import Conditions</h4>
                                                    <div className="space-y-1 max-h-48 overflow-y-auto pr-2">
                                                        {(plan.status?.importConditions || plan.status?.conditions || []).map((c, i) => (
                                                            <div key={i} className="flex items-start text-sm border-b border-gray-50 last:border-0 pb-1">
                                                                <span className="mt-0.5">{renderStatusIcon(c.status || c.Status)}</span>
                                                                <div className="flex-grow">
                                                                    <div className="flex justify-between items-center">
                                                                        <span className="font-medium text-main">{c.type || c.Type}</span>
                                                                        <span className="text-[10px] text-secondary opacity-70 font-mono">{formatDate(c.lastTransitionTime || c.lastUpdateTime || c.LastUpdateTime)}</span>
                                                                    </div>
                                                                    <div className="text-secondary text-xs italic">{c.message || c.Message || (c.status === 'True' ? 'Step completed successfully' : '')}</div>
                                                                </div>
                                                            </div>
                                                        ))}
                                                        {(!plan.status?.importConditions && (!plan.status?.conditions || plan.status.conditions.length === 0)) && <p className="text-xs text-secondary italic">No conditions reported yet.</p>}
                                                    </div>
                                                </div>
                                                <div>
                                                    <h4 className="text-xs font-bold text-secondary uppercase tracking-wider mb-2">Disk Import Status</h4>
                                                    <div className="space-y-3">
                                                        {(() => {
                                                            const disks = plan.status?.diskImportStatus || plan.status?.diskStatus || plan.status?.planStatus?.disks || [];
                                                            if (disks.length === 0) return <p className="text-xs text-secondary italic">No disk progress reported yet.</p>;

                                                            const diskIndex = selectedDisks[plan.metadata.uid] || 0;
                                                            const d = disks[diskIndex] || disks[0];

                                                            return (
                                                                <div className="space-y-4">
                                                                    {disks.length > 1 && (
                                                                        <div className="flex items-center space-x-2">
                                                                            <label className="text-[10px] font-bold text-secondary opacity-70 uppercase">Select Disk:</label>
                                                                            <select
                                                                                className="text-xs border rounded pl-1 pr-8 py-0.5 bg-card focus:outline-none focus:ring-1 focus:ring-blue-500 form-select"
                                                                                value={diskIndex}
                                                                                onChange={(e) => setSelectedDisks(prev => ({ ...prev, [plan.metadata.uid]: parseInt(e.target.value, 10) }))}
                                                                            >
                                                                                {disks.map((disk, idx) => (
                                                                                    <option key={idx} value={idx}>
                                                                                        {disk.diskName || disk.name || disk.Name || `Disk ${idx}`}
                                                                                    </option>
                                                                                ))}
                                                                            </select>
                                                                        </div>
                                                                    )}

                                                                    <div className="text-sm bg-card p-3 rounded border shadow-sm">
                                                                        <div className="flex justify-between items-start mb-1">
                                                                            <div className="flex flex-col truncate mr-2">
                                                                                <span className="font-medium text-main truncate" title={d.diskName || d.name || d.Name}>{d.diskName || d.name || d.Name || `Disk ${diskIndex}`}</span>
                                                                                <span className="text-[10px] text-secondary opacity-70 font-mono">Size: {formatBytes(d.diskSize || d.size || 0)}</span>
                                                                            </div>
                                                                        </div>

                                                                        <div className="mt-3 space-y-1.5 max-h-40 overflow-y-auto pr-1">
                                                                            <h5 className="text-[10px] font-bold text-secondary opacity-70 uppercase tracking-tight">Disk Events</h5>
                                                                            {(() => {
                                                                                const conditions = d.diskConditions || d.conditions || [];
                                                                                return conditions.length > 0 ? (
                                                                                    conditions.map((c, ci) => (
                                                                                        <div key={`${diskIndex}-${ci}`} className="flex items-start text-[11px] border-b border-gray-50 last:border-0 pb-1">
                                                                                            <span className="mt-0.5">{renderStatusIcon(c.status || c.Status)}</span>
                                                                                            <div className="flex-grow">
                                                                                                <div className="flex justify-between items-center">
                                                                                                    <span className="font-medium text-main">{c.type || c.Type}</span>
                                                                                                    <span className="text-[9px] text-secondary opacity-70 font-mono">{formatDate(c.lastTransitionTime || c.lastUpdateTime || c.LastUpdateTime)}</span>
                                                                                                </div>
                                                                                                <div className="text-secondary text-[10px] leading-tight">{c.message || c.Message || (c.status === 'True' || c.Status === 'True' ? 'Task completed' : '')}</div>
                                                                                            </div>
                                                                                        </div>
                                                                                    ))
                                                                                ) : (
                                                                                    <p className="text-[10px] text-secondary opacity-70 italic">{d.status || d.Status || 'Initialising...'}</p>
                                                                                );
                                                                            })()}
                                                                        </div>
                                                                    </div>
                                                                </div>
                                                            );
                                                        })()}
                                                    </div>
                                                </div>
                                            </div>
                                        </td>
                                    </tr>
                                )}
                            </React.Fragment>
                        ))
                    )}
                </tbody>
            </table>
        </div>
    );
};
