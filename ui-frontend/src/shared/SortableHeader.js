import { ChevronUp, ChevronDown } from 'lucide-react';

// --- Sortable Header Component ---
export const SortableHeader = ({ label, sortKey, currentSort, onSort }) => {
    const isActive = currentSort.key === sortKey;
    return (
        <th
            className="px-6 py-3 text-left text-xs font-medium text-secondary uppercase tracking-wider cursor-pointer hover:bg-app transition-colors"
            onClick={() => onSort(sortKey)}
        >
            <div className="flex items-center space-x-1">
                <span>{label}</span>
                <div className="flex flex-col">
                    <ChevronUp size={12} className={`${isActive && currentSort.direction === 'asc' ? 'text-blue-600' : 'opacity-30'}`} />
                    <ChevronDown size={12} className={`${isActive && currentSort.direction === 'desc' ? 'text-blue-600' : 'opacity-30'}`} />
                </div>
            </div>
        </th>
    );
};
