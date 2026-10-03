import { SortableHeader } from '../../shared/SortableHeader';
import { formatDate } from '../../utils';
import { Edit, Trash2 } from 'lucide-react';

export const getOvaSourceStatus = (source) => {
    const statusText = source.status?.status || 'Pending';
    const conditions = source.status?.conditions || [];
    const hasError = conditions.some(c => c.type === 'ClusterError' && c.status === 'True');
    const isReady = conditions.some(c => c.type === 'ClusterReady' && c.status === 'True');

    let color = 'bg-app text-main';
    let label = statusText;
    if (isReady) {
        color = 'bg-green-100 text-green-800';
        label = 'Ready';
    } else if (hasError) {
        color = 'bg-red-100 text-red-800';
        label = statusText === 'clusterNotReady' ? 'Not Ready' : statusText;
    } else if (statusText === 'clusterReady') {
        color = 'bg-green-100 text-green-800';
        label = 'Ready';
    } else if (statusText === 'clusterNotReady') {
        color = 'bg-yellow-100 text-yellow-800';
        label = 'Not Ready';
    }
    return { label, color };
};

export const OvaSourcesTable = ({ sources, onEdit, onDelete, onViewDetails, sortConfig, onSort }) => (
    <div className="bg-card shadow-md rounded-lg overflow-x-auto border border-main">
        <table className="min-w-full divide-y divide-main">
            <thead className="bg-app opacity-90">
                <tr>
                    <SortableHeader label="Created" sortKey="metadata.creationTimestamp" currentSort={sortConfig} onSort={onSort} />
                    <SortableHeader label="Name" sortKey="metadata.name" currentSort={sortConfig} onSort={onSort} />
                    <SortableHeader label="Namespace" sortKey="metadata.namespace" currentSort={sortConfig} onSort={onSort} />
                    <SortableHeader label="URL" sortKey="spec.url" currentSort={sortConfig} onSort={onSort} />
                    <SortableHeader label="Status" sortKey="status.status" currentSort={sortConfig} onSort={onSort} />
                    <th className="px-6 py-3 text-left text-xs font-medium text-secondary uppercase tracking-wider"></th>
                </tr>
            </thead>
            <tbody className="bg-card divide-y divide-main">
                {sources.length === 0 ? (
                    <tr>
                        <td colSpan="6" className="px-6 py-4 text-center text-sm text-secondary">No OVA sources found.</td>
                    </tr>
                ) : (
                    sources.map(source => (
                        <tr key={source.metadata.uid} className="hover:bg-app">
                            <td className="px-6 py-4 whitespace-nowrap text-sm text-secondary">{formatDate(source.metadata.creationTimestamp)}</td>
                            <td className="px-6 py-4 whitespace-nowrap text-sm font-medium text-main">{source.metadata.name}</td>
                            <td className="px-6 py-4 whitespace-nowrap text-sm text-secondary">{source.metadata.namespace}</td>
                            <td className="px-6 py-4 whitespace-nowrap text-sm text-secondary max-w-xs truncate" title={source.spec.url}>{source.spec.url}</td>
                            <td className="px-6 py-4 whitespace-nowrap text-sm">
                                {(() => {
                                    const { label, color } = getOvaSourceStatus(source);
                                    return <span className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium ${color}`}>{label}</span>;
                                })()}
                            </td>
                            <td className="px-6 py-4 whitespace-nowrap text-right text-sm font-medium space-x-2">
                                <button onClick={() => onEdit(source)} title="Edit" className="text-blue-600 hover:text-blue-800"><Edit size={18} /></button>
                                <button onClick={() => onDelete(source)} title="Delete" className="text-red-600 hover:text-red-800"><Trash2 size={18} /></button>
                                <button onClick={() => onViewDetails(source)} className="text-blue-600 hover:text-blue-800">Details</button>
                            </td>
                        </tr>
                    ))
                )}
            </tbody>
        </table>
    </div>
);
