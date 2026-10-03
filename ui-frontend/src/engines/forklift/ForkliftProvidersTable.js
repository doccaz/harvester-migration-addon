import { SortableHeader } from '../../shared/SortableHeader';
import { formatDate } from '../../utils';
import { ForkliftStatusBadge } from './status';
import { Search, Edit, Trash2 } from 'lucide-react';

export const ForkliftProvidersTable = ({ providers, onEdit, onDelete, onViewDetails, onExplore, sortConfig, onSort }) => (
    <div className="bg-card shadow-md rounded-lg overflow-x-auto border border-main">
        <table className="min-w-full divide-y divide-main">
            <thead className="bg-app opacity-90">
                <tr>
                    <SortableHeader label="Created" sortKey="metadata.creationTimestamp" currentSort={sortConfig} onSort={onSort} />
                    <SortableHeader label="Name" sortKey="metadata.name" currentSort={sortConfig} onSort={onSort} />
                    <SortableHeader label="Namespace" sortKey="metadata.namespace" currentSort={sortConfig} onSort={onSort} />
                    <th className="px-6 py-3 text-left text-xs font-medium text-secondary uppercase tracking-wider">Type</th>
                    <th className="px-6 py-3 text-left text-xs font-medium text-secondary uppercase tracking-wider">URL</th>
                    <th className="px-6 py-3 text-left text-xs font-medium text-secondary uppercase tracking-wider">Status</th>
                    <th className="px-6 py-3 text-left text-xs font-medium text-secondary uppercase tracking-wider"></th>
                </tr>
            </thead>
            <tbody className="bg-card divide-y divide-main">
                {(!providers || providers.length === 0) ? (
                    <tr>
                        <td colSpan="7" className="px-6 py-4 text-center text-sm text-secondary">No Forklift source providers found.</td>
                    </tr>
                ) : (
                    providers.map(provider => (
                        <tr key={provider.metadata?.uid || provider.metadata?.name} className="hover:bg-app">
                            <td className="px-6 py-4 whitespace-nowrap text-sm text-secondary">{formatDate(provider.metadata?.creationTimestamp)}</td>
                            <td className="px-6 py-4 whitespace-nowrap text-sm font-medium text-main">{provider.metadata?.name}</td>
                            <td className="px-6 py-4 whitespace-nowrap text-sm text-secondary">{provider.metadata?.namespace}</td>
                            <td className="px-6 py-4 whitespace-nowrap text-sm text-secondary">{provider.spec?.type === 'ova' ? 'OVA (NFS)' : provider.spec?.settings?.sdkEndpoint === 'esxi' ? 'ESXi' : 'vCenter'}</td>
                            <td className="px-6 py-4 whitespace-nowrap text-sm text-secondary max-w-xs truncate" title={provider.spec?.url}>{provider.spec?.url}</td>
                            <td className="px-6 py-4 whitespace-nowrap text-sm">
                                <ForkliftStatusBadge conditions={provider.status?.conditions} />
                            </td>
                            <td className="px-6 py-4 whitespace-nowrap text-right text-sm font-medium space-x-2">
                                {provider.spec?.type !== 'ova' && <button onClick={() => onExplore(provider)} title="Explore Inventory" className="text-indigo-600 hover:text-indigo-800"><Search size={18} /></button>}
                                <button onClick={() => onEdit(provider)} title="Edit" className="text-blue-600 hover:text-blue-800"><Edit size={18} /></button>
                                <button onClick={() => onDelete(provider)} title="Delete" className="text-red-600 hover:text-red-800"><Trash2 size={18} /></button>
                                <button onClick={() => onViewDetails(provider)} className="text-blue-600 hover:text-blue-800">Details</button>
                            </td>
                        </tr>
                    ))
                )}
            </tbody>
        </table>
    </div>
);
