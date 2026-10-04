import { useState, useEffect, useRef } from 'react';
import { Server, RefreshCw, List, Package, Info, Palette, Upload } from 'lucide-react';
import { useCapabilities } from './hooks/useCapabilities';
import { useVmic } from './engines/vmic/useVmic';
import { useForklift } from './engines/forklift/useForklift';
import { EnginePage } from './engines/EnginePage';
import { SourceExplorer } from './inventory/SourceExplorer';
import { CreatePlanWizard } from './wizard/CreatePlanWizard';
import { ForkliftProviderWizard } from './engines/forklift/ForkliftProviderWizard';
import { ForkliftProviderDetails } from './engines/forklift/ForkliftProviderDetails';
import { ForkliftPlanDetails } from './engines/forklift/ForkliftPlanDetails';
import { getPlanStatus } from './engines/vmic/ResourceTable';
import { SourceWizard } from './engines/vmic/SourceWizard';
import { SourceDetails } from './engines/vmic/SourceDetails';
import { OvaSourceWizard } from './engines/vmic/OvaSourceWizard';
import { OvaSourceDetails } from './engines/vmic/OvaSourceDetails';
import { EditVmicPlanModal } from './engines/vmic/EditVmicPlanModal';
import { PlanDetails } from './engines/vmic/PlanDetails';
import { ExportPage } from './export/ExportPage';
import { AboutPage } from './about/AboutPage';

const formatTime = (ms) => new Date(ms).toLocaleString(undefined, { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false });

export default function App() {
    const [expandedPlans, setExpandedPlans] = useState(new Set());
    const [selectedDisks, setSelectedDisks] = useState({}); // planUid -> diskIndex
    const [theme, setTheme] = useState(localStorage.getItem('vm-import-theme') || 'light');

    useEffect(() => {
        document.body.className = `theme-${theme}`;
        localStorage.setItem('vm-import-theme', theme);
    }, [theme]);
    const [autoRefresh, setAutoRefresh] = useState(true);
    // Initial page can come from ?page=<name> or #<name> so a second Rancher
    // NavLink can deep-link straight to the export page.
    const [page, setPage] = useState(() => {
        const valid = ['plans', 'sources', 'ovaSources', 'export', 'about'];
        const fromQuery = new URLSearchParams(window.location.search).get('page');
        const fromHash = window.location.hash.replace(/^#/, '');
        return valid.includes(fromQuery) ? fromQuery : (valid.includes(fromHash) ? fromHash : 'plans');
    });

    const toggleExpand = (uid) => {
        setExpandedPlans(prev => {
            const next = new Set(prev);
            if (next.has(uid)) {
                next.delete(uid);
            } else {
                next.add(uid);
            }
            return next;
        });
    };
    const [selectedPlan, setSelectedPlan] = useState(null);
    const [selectedSource, setSelectedSource] = useState(null);
    const [refreshInterval, setRefreshInterval] = useState(10);
    const [selectedOvaSource, setSelectedOvaSource] = useState(null);

    const capabilities = useCapabilities();
    const vmic = useVmic({ onPlanCreated: () => setPage('plans') });
    const {
        planToDelete,
        setPlanToDelete,
        planToEdit,
        setPlanToEdit,
        sourceToEdit,
        setSourceToEdit,
        sourceToDelete,
        setSourceToDelete,
        showSourceWizard,
        setShowSourceWizard,
        showOvaSourceWizard,
        setShowOvaSourceWizard,
        ovaSourceToEdit,
        setOvaSourceToEdit,
        ovaSourceToDelete,
        setOvaSourceToDelete,
        fetchPlans,
        fetchSources,
        fetchOvaSources,
        handleCreatePlan,
        handleDeletePlan,
        handleSaveVmicPlan,
        handleSaveSource,
        handleDeleteSource,
        handleSaveOvaSource,
        handleDeleteOvaSource,
        lastUpdated,
        refreshError,
    } = vmic;
    const forklift = useForklift();
    const {
        forkliftAvailable,
        forkliftNamespace,
        showForkliftProviderWizard,
        setShowForkliftProviderWizard,
        forkliftWizardDefaultType,
        forkliftProviderToEdit,
        setForkliftProviderToEdit,
        forkliftProviderToDelete,
        setForkliftProviderToDelete,
        forkliftPlanToDelete,
        setForkliftPlanToDelete,
        fetchForkliftProviders,
        fetchForkliftPlans,
        handleSaveForkliftProvider,
        handleDeleteForkliftProvider,
        handleDeleteForkliftPlan,
        handleRunForkliftMigration,
    } = forklift;

    const [plansSubTab, setPlansSubTab] = useState('vmic');
    const [sourcesSubTab, setSourcesSubTab] = useState('vmic');
    const [ovaSourcesSubTab, setOvaSourcesSubTab] = useState('vmic');
    const [selectedForkliftProvider, setSelectedForkliftProvider] = useState(null);
    const [forkliftProviderReturnPage, setForkliftProviderReturnPage] = useState('sources');
    const [selectedForkliftPlan, setSelectedForkliftPlan] = useState(null);

    // Load everything once at start-up.
    useEffect(() => {
        fetchPlans();
        fetchSources();
        fetchOvaSources();
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    // Poll on the chosen interval. The tick reads the latest values through a ref, so
    // flipping the auto-refresh switch, Forklift becoming available or expanding a row
    // neither restarts the timer nor triggers an extra fetch. A period that is not a
    // positive number (an emptied field) means no polling, not a tight loop.
    const tick = useRef(null);
    tick.current = () => {
        if (autoRefresh) {
            fetchPlans();
            if (forkliftAvailable) {
                fetchForkliftPlans();
                fetchForkliftProviders();
            }
        }
    };
    useEffect(() => {
        const seconds = Number(refreshInterval);
        if (!(seconds > 0)) return undefined;
        const intervalId = setInterval(() => tick.current(), seconds * 1000);
        return () => clearInterval(intervalId);
    }, [refreshInterval]);

    const handleViewDetails = (plan) => {
        const detailedPlan = {
            ...plan,
            name: plan.metadata.name,
            // Mock data for VM spec if it's missing in the list view return
            vms: [{
                name: plan.spec.virtualMachineName,
                status: getPlanStatus(plan),
                progress: 0,
                cpu: plan.status?.cpu || 'N/A',
                memoryMB: plan.status?.memoryMB || 0,
                diskSizeGB: plan.status?.diskImportStatus ? Math.round(plan.status.diskImportStatus.reduce((acc, d) => acc + (d.diskSize || 0), 0) / (1024 * 1024 * 1024)) : 'N/A',
            }]
        };
        setSelectedPlan(detailedPlan);
        setPage('planDetails');
    };

    // What the engines' page views need from the app: their own state, navigation, and page-level UI state.
    const nav = { setPage, setSelectedSource, setSelectedOvaSource, setSelectedForkliftProvider, setForkliftProviderReturnPage, setSelectedForkliftPlan, handleViewDetails };
    const ui = { expandedPlans, toggleExpand, selectedDisks, setSelectedDisks, refreshInterval, setRefreshInterval };
    const engineCtx = { vmic, forklift, nav, ui };

    const renderPage = () => {
        switch (page) {
            case 'createPlan':
                return <CreatePlanWizard onCancel={() => setPage('plans')} onCreatePlan={handleCreatePlan} capabilities={capabilities} forkliftAvailable={forkliftAvailable} forkliftNamespace={forkliftNamespace} />;
            case 'planDetails':
                return <PlanDetails plan={selectedPlan} onClose={() => setPage('plans')} />;
            case 'forkliftPlanDetails':
                return selectedForkliftPlan ? <ForkliftPlanDetails plan={selectedForkliftPlan} onClose={() => { setSelectedForkliftPlan(null); setPage('plans'); }} onRunMigration={handleRunForkliftMigration} forkliftNamespace={forkliftNamespace} /> : null;
            case 'sourceDetails':
                return <SourceDetails source={selectedSource} onClose={() => setPage('sources')} />;
            case 'sources':
                return <EnginePage page="sources" active={sourcesSubTab} onChange={setSourcesSubTab} ctx={engineCtx} />;
            case 'forkliftProviderDetails':
                return selectedForkliftProvider ? <ForkliftProviderDetails provider={selectedForkliftProvider} onClose={() => { setSelectedForkliftProvider(null); setPage(forkliftProviderReturnPage); }} /> : null;
            case 'exploreForkliftSource':
                return <SourceExplorer source={selectedSource} onClose={() => setPage('sources')} inventoryApiBase="/api/v1/forklift/inventory" />;
            case 'exploreSource':
                return <SourceExplorer source={selectedSource} onClose={() => setPage('sources')} />;
            case 'ovaSourceDetails':
                return <OvaSourceDetails source={selectedOvaSource} onClose={() => setPage('ovaSources')} />;
            case 'ovaSources':
                return <EnginePage page="ovaSources" active={ovaSourcesSubTab} onChange={setOvaSourcesSubTab} ctx={engineCtx} />;
            case 'export':
                return <ExportPage />;
            case 'about':
                return <AboutPage />;
            case 'plans':
            default:
                return <EnginePage page="plans" active={plansSubTab} onChange={setPlansSubTab} ctx={engineCtx} footer={
                    <div className="flex justify-end items-center mt-4 space-x-6">
                            {/* Proof that refreshing works: when the list was last fetched, or why it could not be. */}
                            <span className={`text-xs ${refreshError ? 'text-red-600' : 'text-secondary'}`} aria-live="polite">
                                {refreshError
                                    ? `Refresh failed: ${refreshError}${lastUpdated ? ` (showing data from ${formatTime(lastUpdated)})` : ''}`
                                    : (lastUpdated ? `Updated ${formatTime(lastUpdated)}` : '')}
                            </span>
                            <div className="flex items-center space-x-2">
                                <input
                                    type="checkbox"
                                    id="autoRefreshPlans"
                                    checked={autoRefresh}
                                    onChange={e => setAutoRefresh(e.target.checked)}
                                    className="w-4 h-4 text-blue-600 border-main rounded focus:ring-blue-500"
                                />
                                <label htmlFor="autoRefreshPlans" className="text-sm font-medium text-main cursor-pointer">Auto-refresh</label>
                            </div>
                            <div className="flex items-center space-x-2">
                                <button onClick={() => { fetchPlans(); if (forkliftAvailable) fetchForkliftPlans(); }} className="text-blue-500 hover:text-blue-700" title="Refresh Now"><RefreshCw size={20} /></button>
                                <div className="flex items-center space-x-1">
                                    <input type="number" value={refreshInterval} onChange={e => setRefreshInterval(e.target.value)} className="w-16 form-input text-sm border rounded px-1" />
                                    <span className="text-xs text-secondary">s</span>
                                </div>
                            </div>
                        </div>
                } />;
        }
    };

    return (
        <div className="min-h-screen p-2 md:p-4 font-sans transition-colors duration-300">
            <div className="flex justify-between items-center mb-6 border-b pb-2">
                <nav className="flex space-x-4">
                    <div className="flex items-center mr-6 pr-6 border-r border-main">
                        <img
                            src="https://harvesterhci.io/img/logo_horizontal.svg"
                            alt="Harvester"
                            className="h-8 transition-opacity"
                        />
                    </div>
                    <button onClick={() => setPage('plans')} className={`px-4 py-2 flex items-center font-medium transition-colors ${page === 'plans' ? 'border-b-2 border-blue-500 text-blue-600' : 'text-secondary hover:text-main'}`}><List size={18} className="mr-2" /> Migration Plans</button>
                    <button onClick={() => setPage('sources')} className={`px-4 py-2 flex items-center font-medium transition-colors ${page === 'sources' ? 'border-b-2 border-blue-500 text-blue-600' : 'text-secondary hover:text-main'}`}><Server size={18} className="mr-2" /> vCenter Sources</button>
                    <button onClick={() => setPage('ovaSources')} className={`px-4 py-2 flex items-center font-medium transition-colors ${page === 'ovaSources' ? 'border-b-2 border-blue-500 text-blue-600' : 'text-secondary hover:text-main'}`}><Package size={18} className="mr-2" /> OVA Sources</button>
                    <button onClick={() => setPage('export')} className={`px-4 py-2 flex items-center font-medium transition-colors ${page === 'export' ? 'border-b-2 border-blue-500 text-blue-600' : 'text-secondary hover:text-main'}`}><Upload size={18} className="mr-2" /> Export VMs</button>
                    <button onClick={() => setPage('about')} className={`px-4 py-2 flex items-center font-medium transition-colors ${page === 'about' ? 'border-b-2 border-blue-500 text-blue-600' : 'text-secondary hover:text-main'}`}><Info size={18} className="mr-2" /> About</button>
                </nav>

                <div className="flex items-center space-x-2 relative group">
                    <div className="flex items-center bg-card border border-main rounded-md px-3 py-1.5 shadow-sm">
                        <Palette size={16} className="mr-2 text-blue-500" />
                        <select
                            value={theme}
                            onChange={(e) => setTheme(e.target.value)}
                            className="bg-transparent text-sm font-medium focus:outline-none cursor-pointer text-main"
                        >
                            <option value="light">Light</option>
                            <option value="suse">SUSE Green</option>
                            <option value="dark">Dark</option>
                        </select>
                    </div>
                </div>
            </div>
            <div className="w-full">
                {renderPage()}
            </div>

            {planToEdit && <EditVmicPlanModal plan={planToEdit} onCancel={() => setPlanToEdit(null)} onSave={handleSaveVmicPlan} capabilities={capabilities} />}
            {showSourceWizard && <SourceWizard onCancel={() => { setShowSourceWizard(false); setSourceToEdit(null); }} onSave={handleSaveSource} source={sourceToEdit} />}
            {showOvaSourceWizard && <OvaSourceWizard onCancel={() => { setShowOvaSourceWizard(false); setOvaSourceToEdit(null); }} onSave={handleSaveOvaSource} source={ovaSourceToEdit} />}

            {planToDelete && (
                <div className="fixed inset-0 bg-opacity-50 flex justify-center items-center p-4 z-50">
                    <div className="bg-card rounded-lg shadow-xl p-6">
                        <h3 className="text-lg font-bold">Confirm Deletion</h3>
                        <p className="my-4">Are you sure you want to delete the plan "{planToDelete.metadata.name}"?</p>
                        <div className="flex justify-end space-x-4">
                            <button onClick={() => setPlanToDelete(null)} className="btn-secondary">Cancel</button>
                            <button onClick={handleDeletePlan} className="bg-red-600 hover:bg-red-700 text-white font-semibold py-2 px-4 rounded-md">Delete</button>
                        </div>
                    </div>
                </div>
            )}
            {sourceToDelete && (
                <div className="fixed inset-0 bg-opacity-50 flex justify-center items-center p-4 z-50">
                    <div className="bg-card rounded-lg shadow-xl p-6">
                        <h3 className="text-lg font-bold">Confirm Deletion</h3>
                        <p className="my-4">Are you sure you want to delete the vCenter source "{sourceToDelete.metadata.name}"? This will also delete the associated credentials secret.</p>
                        <div className="flex justify-end space-x-4">
                            <button onClick={() => setSourceToDelete(null)} className="btn-secondary">Cancel</button>
                            <button onClick={handleDeleteSource} className="bg-red-600 hover:bg-red-700 text-white font-semibold py-2 px-4 rounded-md">Delete</button>
                        </div>
                    </div>
                </div>
            )}
            {ovaSourceToDelete && (
                <div className="fixed inset-0 bg-opacity-50 flex justify-center items-center p-4 z-50">
                    <div className="bg-card rounded-lg shadow-xl p-6">
                        <h3 className="text-lg font-bold">Confirm Deletion</h3>
                        <p className="my-4">Are you sure you want to delete the OVA source "{ovaSourceToDelete.metadata.name}"? This will also delete the associated credentials secret.</p>
                        <div className="flex justify-end space-x-4">
                            <button onClick={() => setOvaSourceToDelete(null)} className="btn-secondary">Cancel</button>
                            <button onClick={handleDeleteOvaSource} className="bg-red-600 hover:bg-red-700 text-white font-semibold py-2 px-4 rounded-md">Delete</button>
                        </div>
                    </div>
                </div>
            )}

            {/* Forklift Modals */}
            {showForkliftProviderWizard && <ForkliftProviderWizard onCancel={() => { setShowForkliftProviderWizard(false); setForkliftProviderToEdit(null); }} onSave={handleSaveForkliftProvider} source={forkliftProviderToEdit} defaultNamespace={forkliftNamespace} defaultProviderType={forkliftWizardDefaultType} />}
            {forkliftProviderToDelete && (
                <div className="fixed inset-0 bg-opacity-50 flex justify-center items-center p-4 z-50">
                    <div className="bg-card rounded-lg shadow-xl p-6">
                        <h3 className="text-lg font-bold">Confirm Deletion</h3>
                        <p className="my-4">Are you sure you want to delete the Forklift provider "{forkliftProviderToDelete.metadata.name}"? This will also delete the associated credentials secret.</p>
                        <div className="flex justify-end space-x-4">
                            <button onClick={() => setForkliftProviderToDelete(null)} className="btn-secondary">Cancel</button>
                            <button onClick={handleDeleteForkliftProvider} className="bg-red-600 hover:bg-red-700 text-white font-semibold py-2 px-4 rounded-md">Delete</button>
                        </div>
                    </div>
                </div>
            )}
            {forkliftPlanToDelete && (
                <div className="fixed inset-0 bg-opacity-50 flex justify-center items-center p-4 z-50">
                    <div className="bg-card rounded-lg shadow-xl p-6">
                        <h3 className="text-lg font-bold">Confirm Deletion</h3>
                        <p className="my-4">Are you sure you want to delete the Forklift plan "{forkliftPlanToDelete.metadata.name}"? This will also delete the associated NetworkMap and StorageMap.</p>
                        <div className="flex justify-end space-x-4">
                            <button onClick={() => setForkliftPlanToDelete(null)} className="btn-secondary">Cancel</button>
                            <button onClick={handleDeleteForkliftPlan} className="bg-red-600 hover:bg-red-700 text-white font-semibold py-2 px-4 rounded-md">Delete</button>
                        </div>
                    </div>
                </div>
            )}
        </div>
    );
}