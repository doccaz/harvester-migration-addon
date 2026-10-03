import { useState, useEffect } from 'react';
import { Server, RefreshCw, List, Package, Info, Palette, Upload } from 'lucide-react';
import { Header } from './shared/Header';
import { SubTab } from './shared/SubTab';
import { handleSort } from './shared/sorting';
import { useCapabilities } from './hooks/useCapabilities';
import { useVmic } from './engines/vmic/useVmic';
import { useForklift } from './engines/forklift/useForklift';
import { SourceExplorer } from './inventory/SourceExplorer';
import { CreatePlanWizard } from './wizard/CreatePlanWizard';
import { ForkliftProvidersTable } from './engines/forklift/ForkliftProvidersTable';
import { ForkliftProviderWizard } from './engines/forklift/ForkliftProviderWizard';
import { ForkliftProviderDetails } from './engines/forklift/ForkliftProviderDetails';
import { ForkliftPlansTable } from './engines/forklift/ForkliftPlansTable';
import { ForkliftPlanDetails } from './engines/forklift/ForkliftPlanDetails';
import { ForkliftUnavailable } from './engines/forklift/ForkliftUnavailable';
import { getPlanStatus, ResourceTable } from './engines/vmic/ResourceTable';
import { SourcesTable } from './engines/vmic/SourcesTable';
import { SourceWizard } from './engines/vmic/SourceWizard';
import { SourceDetails } from './engines/vmic/SourceDetails';
import { OvaSourcesTable } from './engines/vmic/OvaSourcesTable';
import { OvaSourceWizard } from './engines/vmic/OvaSourceWizard';
import { OvaSourceDetails } from './engines/vmic/OvaSourceDetails';
import { EditVmicPlanModal } from './engines/vmic/EditVmicPlanModal';
import { PlanDetails } from './engines/vmic/PlanDetails';
import { ExportPage } from './export/ExportPage';
import { AboutPage } from './about/AboutPage';

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
    const {
        isLoading,
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
        plansSort,
        setPlansSort,
        sourcesSort,
        setSourcesSort,
        ovaSourcesSort,
        setOvaSourcesSort,
        sortedPlans,
        sortedSources,
        sortedOvaSources,
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
        handleEditSource,
        handleEditOvaSource,
    } = useVmic({ onPlanCreated: () => setPage('plans') });
    const {
        forkliftAvailable,
        forkliftMessage,
        forkliftNamespace,
        setForkliftNamespace,
        showForkliftProviderWizard,
        setShowForkliftProviderWizard,
        forkliftWizardDefaultType,
        setForkliftWizardDefaultType,
        forkliftProviderToEdit,
        setForkliftProviderToEdit,
        forkliftProviderToDelete,
        setForkliftProviderToDelete,
        forkliftPlanToDelete,
        setForkliftPlanToDelete,
        forkliftProvidersSort,
        setForkliftProvidersSort,
        forkliftPlansSort,
        setForkliftPlansSort,
        sortedForkliftVsphereProviders,
        sortedForkliftOvaProviders,
        sortedForkliftPlans,
        checkForkliftAvailability,
        fetchForkliftProviders,
        fetchForkliftPlans,
        handleSaveForkliftProvider,
        handleDeleteForkliftProvider,
        handleEditForkliftProvider,
        handleDeleteForkliftPlan,
        handleRunForkliftMigration,
    } = useForklift();

    const [plansSubTab, setPlansSubTab] = useState('vmic');
    const [sourcesSubTab, setSourcesSubTab] = useState('vmic');
    const [ovaSourcesSubTab, setOvaSourcesSubTab] = useState('vmic');
    const [selectedForkliftProvider, setSelectedForkliftProvider] = useState(null);
    const [forkliftProviderReturnPage, setForkliftProviderReturnPage] = useState('sources');
    const [selectedForkliftPlan, setSelectedForkliftPlan] = useState(null);

    useEffect(() => {
        fetchPlans();
        fetchSources();
        fetchOvaSources();
        const intervalId = setInterval(() => {
            // Refresh if autoRefresh is enabled
            if (autoRefresh) {
                fetchPlans();
                if (forkliftAvailable) {
                    fetchForkliftPlans();
                    fetchForkliftProviders();
                }
            }
        }, refreshInterval * 1000);
        return () => clearInterval(intervalId);
    // The fetchers are not listed on purpose: they are re-created on every render, and the
    // effect is meant to restart only when one of the listed values changes (step 4.3 reworks it).
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [refreshInterval, expandedPlans, autoRefresh, forkliftAvailable]);

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
                return (
                    <div className="w-full">
                        <SubTab
                            tabs={[{ key: 'vmic', label: 'VM Import Controller' }, { key: 'forklift', label: 'Forklift' }]}
                            activeTab={sourcesSubTab}
                            onTabChange={setSourcesSubTab}
                        />
                        {sourcesSubTab === 'vmic' ? (
                            <>
                                <Header title="vCenter Sources" onButtonClick={() => { setSourceToEdit(null); setShowSourceWizard(true); }} />
                                <SourcesTable
                                    sources={sortedSources}
                                    onEdit={handleEditSource}
                                    onDelete={setSourceToDelete}
                                    onViewDetails={(source) => { setSelectedSource(source); setPage('sourceDetails'); }}
                                    onExplore={(source) => { setSelectedSource(source); setPage('exploreSource'); }}
                                    sortConfig={sourcesSort}
                                    onSort={handleSort(setSourcesSort)}
                                />
                            </>
                        ) : (
                            forkliftAvailable ? (
                                <>
                                    <Header title="Forklift vSphere Providers" onButtonClick={() => { setForkliftProviderToEdit(null); setForkliftWizardDefaultType('vsphere'); setShowForkliftProviderWizard(true); }} />
                                    <ForkliftProvidersTable
                                        providers={sortedForkliftVsphereProviders}
                                        onEdit={handleEditForkliftProvider}
                                        onDelete={setForkliftProviderToDelete}
                                        onViewDetails={(provider) => { setSelectedForkliftProvider(provider); setForkliftProviderReturnPage('sources'); setPage('forkliftProviderDetails'); }}
                                        onExplore={(provider) => { setSelectedSource({ metadata: provider.metadata, _forkliftProvider: true }); setPage('exploreForkliftSource'); }}
                                        sortConfig={forkliftProvidersSort}
                                        onSort={handleSort(setForkliftProvidersSort)}
                                    />
                                    <div className="flex justify-end items-center mt-4 space-x-2">
                                        <button onClick={fetchForkliftProviders} className="text-blue-500 hover:text-blue-700"><RefreshCw size={20} /></button>
                                    </div>
                                </>
                            ) : <ForkliftUnavailable message={forkliftMessage} namespace={forkliftNamespace} onChangeNamespace={setForkliftNamespace} onRetry={checkForkliftAvailability} />
                        )}
                    </div>
                );
            case 'forkliftProviderDetails':
                return selectedForkliftProvider ? <ForkliftProviderDetails provider={selectedForkliftProvider} onClose={() => { setSelectedForkliftProvider(null); setPage(forkliftProviderReturnPage); }} /> : null;
            case 'exploreForkliftSource':
                return <SourceExplorer source={selectedSource} onClose={() => setPage('sources')} inventoryApiBase="/api/v1/forklift/inventory" />;
            case 'exploreSource':
                return <SourceExplorer source={selectedSource} onClose={() => setPage('sources')} />;
            case 'ovaSourceDetails':
                return <OvaSourceDetails source={selectedOvaSource} onClose={() => setPage('ovaSources')} />;
            case 'ovaSources':
                return (
                    <div className="w-full">
                        <SubTab
                            tabs={[{ key: 'vmic', label: 'VM Import Controller' }, { key: 'forklift', label: 'Forklift' }]}
                            activeTab={ovaSourcesSubTab}
                            onTabChange={setOvaSourcesSubTab}
                        />
                        {ovaSourcesSubTab === 'vmic' ? (
                            <>
                                <Header title="OVA Sources" onButtonClick={() => { setOvaSourceToEdit(null); setShowOvaSourceWizard(true); }} />
                                <OvaSourcesTable sources={sortedOvaSources} onEdit={handleEditOvaSource} onDelete={setOvaSourceToDelete} onViewDetails={(source) => { setSelectedOvaSource(source); setPage('ovaSourceDetails'); }} sortConfig={ovaSourcesSort} onSort={handleSort(setOvaSourcesSort)} />
                                <div className="flex justify-end items-center mt-4 space-x-2">
                                    <button onClick={fetchOvaSources} className="text-blue-500 hover:text-blue-700"><RefreshCw size={20} /></button>
                                    <input type="number" value={refreshInterval} onChange={e => setRefreshInterval(e.target.value)} className="w-20 form-input text-sm" />
                                    <span className="text-sm text-secondary">seconds</span>
                                </div>
                            </>
                        ) : (
                            forkliftAvailable ? (
                                <>
                                    <Header title="Forklift OVA Providers" onButtonClick={() => { setForkliftProviderToEdit(null); setForkliftWizardDefaultType('ova'); setShowForkliftProviderWizard(true); }} />
                                    <ForkliftProvidersTable
                                        providers={sortedForkliftOvaProviders}
                                        onEdit={handleEditForkliftProvider}
                                        onDelete={setForkliftProviderToDelete}
                                        onViewDetails={(provider) => { setSelectedForkliftProvider(provider); setForkliftProviderReturnPage('ovaSources'); setPage('forkliftProviderDetails'); }}
                                        onExplore={(provider) => { setSelectedSource({ metadata: provider.metadata, _forkliftProvider: true }); setPage('exploreForkliftSource'); }}
                                        sortConfig={forkliftProvidersSort}
                                        onSort={handleSort(setForkliftProvidersSort)}
                                    />
                                    <div className="flex justify-end items-center mt-4 space-x-2">
                                        <button onClick={fetchForkliftProviders} className="text-blue-500 hover:text-blue-700"><RefreshCw size={20} /></button>
                                    </div>
                                </>
                            ) : <ForkliftUnavailable message={forkliftMessage} namespace={forkliftNamespace} onChangeNamespace={setForkliftNamespace} onRetry={checkForkliftAvailability} />
                        )}
                    </div>
                );
            case 'export':
                return <ExportPage />;
            case 'about':
                return <AboutPage />;
            case 'plans':
            default:
                return (
                    <div className="w-full">
                        <SubTab
                            tabs={[{ key: 'vmic', label: 'VM Import Controller' }, { key: 'forklift', label: 'Forklift' }]}
                            activeTab={plansSubTab}
                            onTabChange={setPlansSubTab}
                        />
                        {plansSubTab === 'vmic' ? (
                            <>
                                <Header title="VM Migration Plans" onButtonClick={() => setPage('createPlan')} />
                                {isLoading ? <p>Loading plans...</p> : <ResourceTable
                                    plans={sortedPlans}
                                    onViewDetails={handleViewDetails}
                                    onDelete={setPlanToDelete}
                                    onEdit={setPlanToEdit}
                                    sortConfig={plansSort}
                                    onSort={handleSort(setPlansSort)}
                                    expandedPlans={expandedPlans}
                                    toggleExpand={toggleExpand}
                                    selectedDisks={selectedDisks}
                                    setSelectedDisks={setSelectedDisks}
                                />}
                            </>
                        ) : (
                            forkliftAvailable ? (
                                <>
                                    <Header title="Forklift Migration Plans" onButtonClick={() => setPage('createPlan')} />
                                    <ForkliftPlansTable
                                        plans={sortedForkliftPlans}
                                        onDelete={setForkliftPlanToDelete}
                                        onViewDetails={(plan) => { setSelectedForkliftPlan(plan); setPage('forkliftPlanDetails'); }}
                                        sortConfig={forkliftPlansSort}
                                        onSort={handleSort(setForkliftPlansSort)}
                                        expandedPlans={expandedPlans}
                                        toggleExpand={toggleExpand}
                                        onRunMigration={handleRunForkliftMigration}
                                    />
                                </>
                            ) : <ForkliftUnavailable message={forkliftMessage} namespace={forkliftNamespace} onChangeNamespace={setForkliftNamespace} onRetry={checkForkliftAvailability} />
                        )}
                        <div className="flex justify-end items-center mt-4 space-x-6">
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
                    </div>
                );
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