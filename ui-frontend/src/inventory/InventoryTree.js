import { Cloud, Server, Folder, HardDrive, Boxes, X, Search, ChevronRight } from 'lucide-react';
import { useState, useMemo, useEffect } from 'react';

export const VmIcon = ({ type }) => {
    switch (type) {
        case 'datacenter': return <Cloud className="w-5 h-5 text-blue-500" />;
        case 'ClusterComputeResource': return <Server className="w-5 h-5 text-purple-500" />;
        case 'Folder': return <Folder className="w-5 h-5 text-yellow-600" />;
        case 'VirtualMachine': return <HardDrive className="w-5 h-5 text-secondary" />;
        case 'namespace': return <Boxes className="w-5 h-5 text-emerald-500" />;
        case 'disk': return <HardDrive className="w-4 h-4 text-blue-400" />;
        default: return null;
    }
};

export const filterInventory = (node, query) => {
    if (!query) return node;
    const lowerQuery = query.toLowerCase();

    if (node.type === 'VirtualMachine' || node.type === 'disk') {
        if (node.name.toLowerCase().includes(lowerQuery)) {
            return node;
        }
        return null;
    }

    if (node.children && node.children.length > 0) {
        const filteredChildren = node.children
            .map(child => filterInventory(child, query))
            .filter(child => child !== null);

        if (filteredChildren.length > 0) {
            return { ...node, children: filteredChildren };
        }
    }

    return null;
};

export const FilterableInventoryTree = ({ node, onVmSelect, currentlySelectedVm }) => {
    const [searchQuery, setSearchQuery] = useState('');

    const filteredNode = useMemo(() => {
        return filterInventory(node, searchQuery);
    }, [node, searchQuery]);

    return (
        <div className="flex flex-col h-full overflow-hidden">
            <div className="mb-2 shrink-0 flex items-center">
                {searchQuery && (
                    <button
                        onClick={() => setSearchQuery('')}
                        className="mr-2 p-1 text-secondary opacity-70 hover:text-secondary hover:bg-app rounded-full transition-colors focus:outline-none"
                        title="Clear search"
                    >
                        <X size={14} />
                    </button>
                )}
                <div className="relative flex-grow">
                    <div className="absolute inset-y-0 left-0 pl-2 flex items-center pointer-events-none">
                        <Search size={14} className="text-secondary opacity-70" />
                    </div>
                    <input
                        type="text"
                        placeholder="Search VMs..."
                        className="w-full pl-8 pr-2 py-1.5 text-sm border border-main rounded-md focus:outline-none focus:ring-1 focus:ring-blue-500"
                        value={searchQuery}
                        onChange={e => setSearchQuery(e.target.value)}
                    />
                </div>
            </div>
            <div className="flex-grow overflow-y-auto">
                {filteredNode ? (
                    <InventoryTree
                        node={filteredNode}
                        onVmSelect={onVmSelect}
                        currentlySelectedVm={currentlySelectedVm}
                        forceOpen={!!searchQuery}
                    />
                ) : (
                    <div className="text-center text-secondary text-sm py-8">No matching VMs found.</div>
                )}
            </div>
        </div>
    );
};

export const InventoryTree = ({ node, onVmSelect, currentlySelectedVm, level = 0, forceOpen = false }) => {
    const [isOpen, setIsOpen] = useState(level < 2 || forceOpen);
    const isParent = node.children && node.children.length > 0;

    useEffect(() => {
        if (forceOpen) setIsOpen(true);
    }, [forceOpen]);

    const handleNodeClick = () => {
        if (node.type === 'VirtualMachine' || node.type === 'disk') {
            onVmSelect(node);
        }
        if (isParent) {
            setIsOpen(!isOpen);
        }
    };

    return (
        <div style={{ paddingLeft: level > 0 ? '20px' : '0px' }}>
            <div
                className={`flex items-center p-2 rounded-md cursor-pointer ${currentlySelectedVm?.name === node.name ? 'bg-blue-100' : 'hover:bg-app'}`}
                onClick={handleNodeClick}
            >
                {isParent && <ChevronRight size={16} className={`mr-1 transform transition-transform ${isOpen ? 'rotate-90' : ''}`} />}
                <VmIcon type={node.type} />
                <span className="ml-2 text-main">{node.name}</span>
            </div>
            {isOpen && isParent && (
                <div>
                    {node.children.map((child, index) => (
                        <InventoryTree
                            key={index}
                            node={child}
                            onVmSelect={onVmSelect}
                            currentlySelectedVm={currentlySelectedVm}
                            level={level + 1}
                            forceOpen={forceOpen}
                        />
                    ))}
                </div>
            )}
        </div>
    );
};
