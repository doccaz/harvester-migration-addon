import { useState, useCallback, useEffect, useMemo } from 'react';
import { EXPORT_TERMINAL, ExportsTable } from './ExportsTable';
import { RefreshCw, XCircle, Loader, X, AlertTriangle } from 'lucide-react';
import { FilterableInventoryTree } from '../inventory/InventoryTree';
import { HarvesterVmPanel } from './HarvesterVmPanel';

export const ExportPage = () => {
    const [inventory, setInventory] = useState(null);
    const [selectedVm, setSelectedVm] = useState(null);
    const [isLoading, setIsLoading] = useState(false);
    const [error, setError] = useState('');
    const [exports, setExports] = useState([]);
    const [isBusy, setIsBusy] = useState(false);
    const [logs, setLogs] = useState(null);

    const fetchInventory = useCallback(async () => {
        setIsLoading(true);
        setError('');
        try {
            const response = await fetch('/api/v1/harvester/inventory');
            if (!response.ok) {
                const errData = await response.json().catch(() => ({}));
                throw new Error(errData.error || 'Failed to fetch Harvester inventory');
            }
            const data = await response.json();
            setInventory(data);
            // Keep the selection pointing at fresh data across refreshes.
            setSelectedVm(current => {
                if (!current) return null;
                const all = [];
                const walk = (n) => { if (n.type === 'VirtualMachine') all.push(n); (n.children || []).forEach(walk); };
                walk(data);
                return all.find(v => v.id === current.id) || null;
            });
        } catch (err) {
            setError(err.message);
        } finally {
            setIsLoading(false);
        }
    }, []);

    useEffect(() => { fetchInventory(); }, [fetchInventory]);

    const fetchExports = useCallback(async () => {
        try {
            const response = await fetch('/api/v1/exports');
            if (!response.ok) return; // export may not be configured; stay quiet
            setExports(await response.json());
        } catch (err) {
            console.error('Failed to list exports', err);
        }
    }, []);

    useEffect(() => { fetchExports(); }, [fetchExports]);

    // Poll only while something is in flight, so an idle page is silent.
    useEffect(() => {
        const active = exports.some(e => !EXPORT_TERMINAL.includes(e.phase));
        if (!active) return undefined;
        const t = setInterval(fetchExports, 5000);
        return () => clearInterval(t);
    }, [exports, fetchExports]);

    const handleExport = async (vm, profile) => {
        setIsBusy(true);
        try {
            const response = await fetch('/api/v1/exports', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ namespace: vm.namespace, name: vm.name, profile, preserveMacs: true }),
            });
            const data = await response.json().catch(() => ({}));
            if (!response.ok) throw new Error(data.error || 'Failed to start export');
            await fetchExports();
        } catch (err) {
            alert(`Export failed to start: ${err.message}`);
        } finally {
            setIsBusy(false);
        }
    };

    const handleDeleteExport = async (e) => {
        if (!window.confirm(`Delete the export of ${e.vmName}? This also removes the generated OVA.`)) return;
        setIsBusy(true);
        try {
            const response = await fetch(`/api/v1/exports/${e.namespace}/${e.exportId}?purge=true`, { method: 'DELETE' });
            if (!response.ok) {
                const data = await response.json().catch(() => ({}));
                throw new Error(data.error || 'Failed to delete export');
            }
            await fetchExports();
        } catch (err) {
            alert(err.message);
        } finally {
            setIsBusy(false);
        }
    };

    const handleLogs = async (e) => {
        setLogs({ name: e.vmName, text: 'Loading…' });
        try {
            const response = await fetch(`/api/v1/exports/${e.namespace}/${e.exportId}/logs`);
            const text = await response.text();
            setLogs({ name: e.vmName, text: response.ok ? text : `Failed to fetch logs: ${text}` });
        } catch (err) {
            setLogs({ name: e.vmName, text: `Failed to fetch logs: ${err.message}` });
        }
    };

    const vmCount = useMemo(() => {
        if (!inventory) return 0;
        let n = 0;
        const walk = (x) => { if (x.type === 'VirtualMachine') n++; (x.children || []).forEach(walk); };
        walk(inventory);
        return n;
    }, [inventory]);

    return (
        <div className="w-full">
            <div className="flex items-center justify-between mb-4">
                <div>
                    <h2 className="text-2xl font-bold text-main">Export VMs</h2>
                    <p className="text-sm text-secondary">Export a Harvester virtual machine to a standards-conformant OVA.</p>
                </div>
                <button onClick={fetchInventory} disabled={isLoading} className="btn-secondary px-3 py-2 rounded-md flex items-center text-sm">
                    <RefreshCw size={16} className={`mr-2 ${isLoading ? 'animate-spin' : ''}`} /> Refresh
                </button>
            </div>

            {error && (
                <div className="mb-4 p-3 rounded-md border border-red-400 bg-red-50 text-red-800 text-sm flex items-center">
                    <XCircle size={16} className="mr-2" /> {error}
                </div>
            )}

            {/* What the inventory could not read: shown so a blank disk size is not mistaken for an empty one. */}
            {inventory?.warnings?.length > 0 && (
                <div role="alert" className="mb-4 p-3 rounded-md border border-yellow-400 bg-yellow-50 text-yellow-800 text-sm">
                    {inventory.warnings.map((w) => (
                        <div key={w} className="flex items-start"><AlertTriangle size={16} className="mr-2 mt-0.5 shrink-0" /> {w}</div>
                    ))}
                </div>
            )}

            <div className="flex gap-4" style={{ height: '70vh' }}>
                <div className="w-1/3 bg-card border border-main rounded-lg p-3 flex flex-col">
                    <div className="text-xs text-secondary mb-2 shrink-0">{vmCount} virtual machine{vmCount === 1 ? '' : 's'}</div>
                    {isLoading && !inventory ? (
                        <div className="flex-grow flex items-center justify-center text-secondary"><Loader size={20} className="animate-spin mr-2" /> Loading…</div>
                    ) : inventory ? (
                        <FilterableInventoryTree node={inventory} onVmSelect={setSelectedVm} currentlySelectedVm={selectedVm} />
                    ) : (
                        <div className="flex-grow flex items-center justify-center text-secondary text-sm">No inventory</div>
                    )}
                </div>
                <div className="w-2/3 bg-card border border-main rounded-lg p-4">
                    <HarvesterVmPanel vm={selectedVm} onExport={handleExport} isBusy={isBusy} />
                </div>
            </div>

            <div className="mt-6">
                <div className="flex items-center justify-between mb-2">
                    <h3 className="text-lg font-semibold text-main">Exports</h3>
                    <button onClick={fetchExports} className="btn-secondary px-3 py-1.5 rounded-md flex items-center text-xs">
                        <RefreshCw size={14} className="mr-2" /> Refresh
                    </button>
                </div>
                <ExportsTable exports={exports} onDelete={handleDeleteExport} onLogs={handleLogs} isBusy={isBusy} />
            </div>

            {logs && (
                <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50 p-6" onClick={() => setLogs(null)}>
                    <div className="bg-card border border-main rounded-lg w-full max-w-4xl max-h-[80vh] flex flex-col" onClick={ev => ev.stopPropagation()}>
                        <div className="p-4 border-b border-main flex items-center justify-between">
                            <h3 className="font-semibold text-main">Export logs — {logs.name}</h3>
                            <button onClick={() => setLogs(null)} className="text-secondary hover:text-main"><X size={18} /></button>
                        </div>
                        <div className="p-3 overflow-auto bg-gray-900 text-white font-mono text-xs flex-1">
                            <pre className="whitespace-pre-wrap">{logs.text}</pre>
                        </div>
                    </div>
                </div>
            )}
        </div>
    );
};
