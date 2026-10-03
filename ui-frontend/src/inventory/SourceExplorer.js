import { useState, useCallback, useEffect } from 'react';
import { RefreshCw, X, Loader, AlertTriangle } from 'lucide-react';
import { FilterableInventoryTree } from './InventoryTree';
import { VmDetailsPanel } from './VmDetailsPanel';

export const SourceExplorer = ({ source, onClose, inventoryApiBase }) => {
    const [inventory, setInventory] = useState(null);
    const [isLoading, setIsLoading] = useState(false);
    const [error, setError] = useState('');
    const [selectedVm, setSelectedVm] = useState(null);
    const [isOperating, setIsOperating] = useState(false);
    const isForklift = !!inventoryApiBase;

    const fetchInventory = useCallback(async (keepSelection = false) => {
        setIsLoading(true);
        setError('');
        try {
            const apiBase = inventoryApiBase || '/api/v1/vcenter/inventory';
            const response = await fetch(`${apiBase}/${source.metadata.namespace}/${source.metadata.name}`);
            if (!response.ok) {
                const errData = await response.json();
                throw new Error(errData.error || "Failed to fetch inventory");
            }
            const data = await response.json();
            setInventory(data);

            if (!keepSelection) {
                setSelectedVm(null);
            } else {
                setSelectedVm(current => {
                    if (!current) return null;
                    // Find and update selected VM in new data
                    const findVm = (node, name) => {
                        if (node.type === 'VirtualMachine' && node.name === name) return node;
                        if (node.children) {
                            for (const child of node.children) {
                                const found = findVm(child, name);
                                if (found) return found;
                            }
                        }
                        return null;
                    };
                    const updated = findVm(data, current.name);
                    return updated || current;
                });
            }
        } catch (err) {
            setError(err.message);
        } finally {
            setIsLoading(false);
        }
    }, [source, inventoryApiBase]); // Now stable regardless of selection

    const handlePowerOp = async (op) => {
        if (!selectedVm) return;
        setIsOperating(true);
        try {
            const response = await fetch(`/api/v1/vcenter/vm/${source.metadata.namespace}/${source.metadata.name}/power`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ vmName: selectedVm.name, operation: op })
            });
            if (!response.ok) {
                const data = await response.json();
                throw new Error(data.error || 'Operation failed');
            }
            // Refresh inventory to see state change
            await fetchInventory(true);
        } catch (err) {
            alert(`Error: ${err.message}`);
        } finally {
            setIsOperating(false);
        }
    };

    const handleRename = async (oldName, newName) => {
        setIsOperating(true);
        try {
            const response = await fetch(`/api/v1/vcenter/vm/${source.metadata.namespace}/${source.metadata.name}/rename`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ oldName, newName })
            });
            if (!response.ok) {
                const data = await response.json();
                throw new Error(data.error || 'Rename failed');
            }
            // Update local selection to new name before refresh
            setSelectedVm(prev => ({ ...prev, name: newName }));
            await fetchInventory(true);
        } catch (err) {
            alert(`Error: ${err.message}`);
        } finally {
            setIsOperating(false);
        }
    };

    const handleMacUpdate = async (vmName, networkKey, newMac) => {
        setIsOperating(true); // Use isOperating for any VM-level operation
        try {
            const response = await fetch(`/api/v1/vcenter/vm/${source.metadata.namespace}/${source.metadata.name}/mac`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ vmName, deviceKey: networkKey, newMac })
            });
            if (!response.ok) {
                const data = await response.json();
                throw new Error(data.error || 'MAC update failed');
            }
            await fetchInventory(true); // Refresh inventory to show updated MAC
        } finally {
            setIsOperating(false);
        }
    };

    useEffect(() => {
        fetchInventory();
    }, [source, fetchInventory]);

    return (
        <div className="fixed inset-0 bg-opacity-50 flex justify-center items-center p-4 z-50">
            <div className="bg-card rounded-lg shadow-xl w-full max-w-5xl flex flex-col max-h-[90vh]">
                <div className="flex justify-between items-center p-4 border-b">
                    <div className="flex items-center space-x-4">
                        <h2 className="text-xl font-semibold text-main">Explore: {source.metadata.name}</h2>
                        <button onClick={() => fetchInventory(true)} className="text-blue-500 hover:text-blue-700 p-1 rounded-full hover:bg-app transition-colors" title="Refresh Inventory">
                            <RefreshCw size={18} className={isLoading ? 'animate-spin' : ''} />
                        </button>
                    </div>
                    <button onClick={onClose} className="p-2 rounded-full hover:bg-gray-200">
                        <X size={20} />
                    </button>
                </div>
                <div className="p-6 flex-grow overflow-hidden flex flex-col">
                    {isLoading && !inventory ? (
                        <div className="flex flex-col items-center justify-center flex-grow">
                            <Loader className="animate-spin text-blue-500 mb-2" size={32} />
                            <p className="text-secondary">Loading vCenter inventory...</p>
                        </div>
                    ) : error ? (
                        <div className="flex flex-col items-center justify-center flex-grow text-center">
                            <AlertTriangle className="text-red-500 mb-2" size={32} />
                            <p className="text-red-600 font-medium">Error loading inventory</p>
                            <p className="text-secondary text-sm mt-1">{error}</p>
                            <button onClick={fetchInventory} className="mt-4 bg-blue-500 hover:bg-blue-600 text-white font-semibold py-2 px-4 rounded-md">Retry</button>
                        </div>
                    ) : (
                        <div className="grid grid-cols-1 md:grid-cols-2 gap-4 h-full overflow-hidden">
                            <div className="border border-main rounded-md p-2 flex flex-col overflow-hidden bg-card shadow-sm font-sans">
                                {inventory && <FilterableInventoryTree node={inventory} onVmSelect={setSelectedVm} currentlySelectedVm={selectedVm} />}
                            </div>
                            <div className="overflow-y-auto">
                                <VmDetailsPanel
                                    vm={selectedVm}
                                    onPowerOp={isForklift ? null : handlePowerOp}
                                    onRename={isForklift ? null : handleRename}
                                    onMacUpdate={isForklift ? null : handleMacUpdate}
                                    isOperating={isOperating}
                                />
                            </div>
                        </div>
                    )}
                </div>
                <div className="p-4 border-t bg-app flex justify-between items-center rounded-b-lg">
                    <p className="text-xs text-secondary">Browsing data from {source.spec?.endpoint || source.spec?.url || source.metadata.name}</p>
                    <button onClick={onClose} className="btn-secondary px-4 py-2 rounded-md font-semibold transition-colors shadow-sm transition-all active:scale-95">Close Explorer</button>
                </div>
            </div>
        </div>
    );
};
