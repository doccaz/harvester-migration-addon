import React, { useState, useEffect } from 'react';
import { CheckCircle2, XCircle, Loader, AlertTriangle, ChevronDown, ChevronRight, Play, Trash2, HardDrive, Cpu, MemoryStick } from 'lucide-react';
import { SortableHeader } from '../../shared/SortableHeader';
import { getForkliftPlanStatus } from './status';
import { formatDate, formatBytes } from '../../utils';

// Small helper component for lazy-fetching related object status in the expanded row
export const ForkliftRelatedObjectStatus = ({ url, label }) => {
    const [status, setStatus] = useState(null);
    useEffect(() => {
        if (!url) { setStatus('N/A'); return; }
        fetch(url).then(r => r.ok ? r.json() : null).then(data => {
            if (!data) { setStatus('Not Found'); return; }
            const conds = data.status?.conditions || [];
            const ready = conds.find(c => c.type === 'Ready');
            setStatus(ready?.status === 'True' ? 'Ready' : ready ? 'Not Ready' : 'Pending');
        }).catch(() => setStatus('Error'));
    }, [url]);
    const icon = status === 'Ready' ? <CheckCircle2 size={12} className="text-green-500" />
        : status === 'Not Ready' ? <XCircle size={12} className="text-red-500" />
        : status === 'Pending' || status === null ? <Loader size={12} className="text-secondary animate-spin" />
        : <AlertTriangle size={12} className="text-yellow-500" />;
    return (
        <div className="flex items-center gap-1.5 text-xs text-main">
            {icon} <span className="text-secondary">{label}:</span> <span>{status || '...'}</span>
        </div>
    );
};

export const ForkliftPlansTable = ({ plans, onDelete, onViewDetails, sortConfig, onSort, expandedPlans, toggleExpand, onRunMigration }) => {
    const renderStatusBadge = (status) => {
        const colors = {
            'Ready': 'bg-blue-100 text-blue-800',
            'Executing': 'bg-yellow-100 text-yellow-800',
            'Succeeded': 'bg-green-100 text-green-800',
            'Failed': 'bg-red-100 text-red-800',
            'Not Ready': 'bg-red-100 text-red-800',
            'Pending': 'bg-app text-main',
        };
        return <span className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium ${colors[status] || colors['Pending']}`}>{status}</span>;
    };

    return (
        <div className="bg-card shadow-md rounded-lg overflow-x-auto border border-main">
            <table className="min-w-full divide-y divide-main">
                <thead className="bg-app opacity-90">
                    <tr>
                        <th className="px-4 py-3"></th>
                        <SortableHeader label="Created" sortKey="metadata.creationTimestamp" currentSort={sortConfig} onSort={onSort} />
                        <SortableHeader label="Name" sortKey="metadata.name" currentSort={sortConfig} onSort={onSort} />
                        <th className="px-6 py-3 text-left text-xs font-medium text-secondary uppercase tracking-wider">Status</th>
                        <th className="px-6 py-3 text-left text-xs font-medium text-secondary uppercase tracking-wider">VMs</th>
                        <SortableHeader label="Target NS" sortKey="spec.targetNamespace" currentSort={sortConfig} onSort={onSort} />
                        <th className="px-6 py-3 text-left text-xs font-medium text-secondary uppercase tracking-wider"></th>
                    </tr>
                </thead>
                <tbody className="bg-card divide-y divide-main">
                    {(!plans || plans.length === 0) ? (
                        <tr>
                            <td colSpan="7" className="px-6 py-4 text-center text-sm text-secondary">No Forklift migration plans found.</td>
                        </tr>
                    ) : (
                        plans.map(plan => {
                            const uid = plan.metadata?.uid || plan.metadata?.name;
                            const status = getForkliftPlanStatus(plan);
                            const vms = plan.spec?.vms || [];
                            const netRef = plan.spec?.map?.network;
                            const stRef = plan.spec?.map?.storage;
                            const provRef = plan.spec?.provider?.source;
                            return (
                                <React.Fragment key={uid}>
                                    <tr className="hover:bg-app transition-colors">
                                        <td className="px-4 py-4 whitespace-nowrap text-sm text-secondary">
                                            <button onClick={() => toggleExpand(uid)} className="p-1 hover:bg-app rounded-full transition-colors">
                                                {expandedPlans.has(uid) ? <ChevronDown size={18} /> : <ChevronRight size={18} />}
                                            </button>
                                        </td>
                                        <td className="px-6 py-4 whitespace-nowrap text-sm text-secondary">{formatDate(plan.metadata?.creationTimestamp)}</td>
                                        <td className="px-6 py-4 whitespace-nowrap text-sm font-medium text-main">{plan.metadata?.name}</td>
                                        <td className="px-6 py-4 whitespace-nowrap text-sm">{renderStatusBadge(status)}</td>
                                        <td className="px-6 py-4 whitespace-nowrap text-sm text-secondary">{vms.map(v => v.name).join(', ') || 'N/A'}</td>
                                        <td className="px-6 py-4 whitespace-nowrap text-sm text-secondary">{plan.spec?.targetNamespace}</td>
                                        <td className="px-6 py-4 whitespace-nowrap text-right text-sm font-medium space-x-2 pr-4">
                                            {status === 'Ready' && (
                                                <button onClick={() => onRunMigration(plan)} title="Run Migration" className="text-green-600 hover:text-green-800"><Play size={18} /></button>
                                            )}
                                            <button onClick={() => onDelete(plan)} title="Delete" className="text-red-600 hover:text-red-800"><Trash2 size={18} /></button>
                                            <button onClick={() => onViewDetails(plan)} className="text-blue-600 hover:text-blue-800">Details</button>
                                        </td>
                                    </tr>
                                    {expandedPlans.has(uid) && (
                                        <tr className="bg-app">
                                            <td colSpan="7" className="px-6 py-4">
                                                <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
                                                    <div>
                                                        <h4 className="text-xs font-bold text-secondary uppercase tracking-wider mb-2">Related Objects</h4>
                                                        <div className="space-y-1.5 p-2 bg-card rounded border">
                                                            <ForkliftRelatedObjectStatus
                                                                url={provRef ? `/api/v1/forklift/providers/${provRef.namespace}/${provRef.name}` : null}
                                                                label="Provider"
                                                            />
                                                            <ForkliftRelatedObjectStatus
                                                                url={netRef ? `/api/v1/forklift/networkmaps/${netRef.namespace}/${netRef.name}` : null}
                                                                label="NetworkMap"
                                                            />
                                                            <ForkliftRelatedObjectStatus
                                                                url={stRef ? `/api/v1/forklift/storagemaps/${stRef.namespace}/${stRef.name}` : null}
                                                                label="StorageMap"
                                                            />
                                                        </div>
                                                    </div>
                                                    <div>
                                                        <h4 className="text-xs font-bold text-secondary uppercase tracking-wider mb-2">Conditions</h4>
                                                        <div className="space-y-1 max-h-48 overflow-y-auto pr-2">
                                                            {(plan.status?.conditions || []).map((c, i) => (
                                                                <div key={i} className="flex items-start text-sm border-b border-gray-50 last:border-0 pb-1">
                                                                    <span className="mt-0.5">
                                                                        {c.status === 'True' ? <CheckCircle2 size={14} className="text-green-500 mr-1" /> : <XCircle size={14} className="text-red-500 mr-1" />}
                                                                    </span>
                                                                    <div className="flex-grow">
                                                                        <div className="flex justify-between items-center">
                                                                            <span className="font-medium text-main">{c.type}</span>
                                                                            <span className="text-[10px] text-secondary opacity-70 font-mono">{formatDate(c.lastTransitionTime)}</span>
                                                                        </div>
                                                                        <div className="text-secondary text-xs italic">{c.message || ''}</div>
                                                                    </div>
                                                                </div>
                                                            ))}
                                                            {(!plan.status?.conditions || plan.status.conditions.length === 0) && <p className="text-xs text-secondary italic">No conditions reported yet.</p>}
                                                        </div>
                                                    </div>
                                                    <div>
                                                        <h4 className="text-xs font-bold text-secondary uppercase tracking-wider mb-2">VMs</h4>
                                                        <div className="space-y-2">
                                                            {vms.map((vm, i) => (
                                                                <div key={i} className="flex items-center p-2 bg-card rounded border shadow-sm text-sm">
                                                                    <HardDrive size={14} className="mr-2 text-secondary" />
                                                                    <span className="text-main font-medium">{vm.name}</span>
                                                                    <span className="text-xs text-secondary ml-2 font-mono">({vm.id})</span>
                                                                </div>
                                                            ))}
                                                            {(() => {
                                                                const ann = plan.metadata?.annotations || {};
                                                                const cpu = ann['migration.harvesterhci.io/original-cpu'];
                                                                const mem = ann['migration.harvesterhci.io/original-memory-mb'];
                                                                const disk = ann['migration.harvesterhci.io/original-disk-size-gb'];
                                                                return (cpu || mem || disk) ? (
                                                                    <div className="flex flex-wrap gap-3 text-[11px] p-2 bg-blue-500/10 rounded-md border border-blue-500/20 items-center mt-1">
                                                                        <span className="font-bold text-blue-700 uppercase tracking-tight">Source:</span>
                                                                        {cpu && <span className="flex items-center gap-1"><Cpu size={10} className="text-secondary" />{cpu} vCPU</span>}
                                                                        {mem && <span className="flex items-center gap-1"><MemoryStick size={10} className="text-secondary" />{formatBytes(parseInt(mem) * 1024 * 1024, 0)}</span>}
                                                                        {disk && <span className="flex items-center gap-1"><HardDrive size={10} className="text-secondary" />{disk} GB</span>}
                                                                    </div>
                                                                ) : null;
                                                            })()}
                                                        </div>
                                                    </div>
                                                </div>
                                            </td>
                                        </tr>
                                    )}
                                </React.Fragment>
                            );
                        })
                    )}
                </tbody>
            </table>
        </div>
    );
};
