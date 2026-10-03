import { useState, useEffect, useMemo, useCallback } from 'react';
import { Server, RefreshCw, List, Package, Info, Palette, Upload } from 'lucide-react';
import { Header } from './shared/Header';
import { SubTab } from './shared/SubTab';
import { getNestedValue } from './shared/getNestedValue';
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
    const [plans, setPlans] = useState([]);
    const [sources, setSources] = useState([]);
    const [isLoading, setIsLoading] = useState(true);

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
    const [planToDelete, setPlanToDelete] = useState(null);
    const [planToEdit, setPlanToEdit] = useState(null);
    const [sourceToEdit, setSourceToEdit] = useState(null);
    const [sourceToDelete, setSourceToDelete] = useState(null);
    const [showSourceWizard, setShowSourceWizard] = useState(false);
    const [refreshInterval, setRefreshInterval] = useState(10);
    const [ovaSources, setOvaSources] = useState([]);
    const [showOvaSourceWizard, setShowOvaSourceWizard] = useState(false);
    const [ovaSourceToEdit, setOvaSourceToEdit] = useState(null);
    const [ovaSourceToDelete, setOvaSourceToDelete] = useState(null);
    const [selectedOvaSource, setSelectedOvaSource] = useState(null);

    // Sorting State
    const [plansSort, setPlansSort] = useState({ key: 'metadata.creationTimestamp', direction: 'desc' });
    const [sourcesSort, setSourcesSort] = useState({ key: 'metadata.creationTimestamp', direction: 'desc' });
    const [ovaSourcesSort, setOvaSourcesSort] = useState({ key: 'metadata.creationTimestamp', direction: 'desc' });

    const handleSort = (setter) => (key) => {
        setter(prev => ({
            key,
            direction: prev.key === key && prev.direction === 'asc' ? 'desc' : 'asc'
        }));
    };

    const sortData = (data, sortConfig) => {
        if (!sortConfig.key) return data;
        return [...data].sort((a, b) => {
            const aVal = getNestedValue(a, sortConfig.key);
            const bVal = getNestedValue(b, sortConfig.key);
            if (aVal < bVal) return sortConfig.direction === 'asc' ? -1 : 1;
            if (aVal > bVal) return sortConfig.direction === 'asc' ? 1 : -1;
            return 0;
        });
    };

    const sortedPlans = useMemo(() => sortData(plans, plansSort), [plans, plansSort]);
    const sortedSources = useMemo(() => sortData(sources, sourcesSort), [sources, sourcesSort]);
    const sortedOvaSources = useMemo(() => sortData(ovaSources, ovaSourcesSort), [ovaSources, ovaSourcesSort]);

    // NEW: Capability State
    const [capabilities, setCapabilities] = useState({ harvesterVersion: '', hasAdvancedPower: false });

    // NEW: Fetch Capabilities on Mount
    useEffect(() => {
        fetch('/api/v1/capabilities')
            .then(res => res.json())
            .then(data => {
                console.log("Cluster Capabilities:", data);
                setCapabilities(data);
            })
            .catch(err => console.error("Failed to fetch capabilities:", err));
    }, []);

    // --- Forklift State ---
    const [forkliftAvailable, setForkliftAvailable] = useState(null); // null = loading, true/false
    const [forkliftMessage, setForkliftMessage] = useState('');
    const [forkliftNamespace, setForkliftNamespace] = useState('forklift');
    const [plansSubTab, setPlansSubTab] = useState('vmic');
    const [sourcesSubTab, setSourcesSubTab] = useState('vmic');
    const [ovaSourcesSubTab, setOvaSourcesSubTab] = useState('vmic');
    const [forkliftProviders, setForkliftProviders] = useState([]);
    const [forkliftPlans, setForkliftPlans] = useState([]);
    const [showForkliftProviderWizard, setShowForkliftProviderWizard] = useState(false);
    const [forkliftWizardDefaultType, setForkliftWizardDefaultType] = useState('vsphere');
    const [forkliftProviderToEdit, setForkliftProviderToEdit] = useState(null);
    const [forkliftProviderToDelete, setForkliftProviderToDelete] = useState(null);
    const [selectedForkliftProvider, setSelectedForkliftProvider] = useState(null);
    const [forkliftProviderReturnPage, setForkliftProviderReturnPage] = useState('sources');
    const [selectedForkliftPlan, setSelectedForkliftPlan] = useState(null);
    const [forkliftPlanToDelete, setForkliftPlanToDelete] = useState(null);
    const [forkliftProvidersSort, setForkliftProvidersSort] = useState({ key: 'metadata.creationTimestamp', direction: 'desc' });
    const [forkliftPlansSort, setForkliftPlansSort] = useState({ key: 'metadata.creationTimestamp', direction: 'desc' });

    const sortedForkliftProviders = useMemo(() => sortData(forkliftProviders, forkliftProvidersSort), [forkliftProviders, forkliftProvidersSort]);
    const sortedForkliftVsphereProviders = useMemo(() => sortedForkliftProviders.filter(p => p.spec?.type !== 'ova'), [sortedForkliftProviders]);
    const sortedForkliftOvaProviders = useMemo(() => sortedForkliftProviders.filter(p => p.spec?.type === 'ova'), [sortedForkliftProviders]);
    const sortedForkliftPlans = useMemo(() => sortData(forkliftPlans, forkliftPlansSort), [forkliftPlans, forkliftPlansSort]);

    const checkForkliftAvailability = useCallback((ns) => {
        const checkNs = ns || forkliftNamespace;
        setForkliftAvailable(null);
        fetch(`/api/v1/forklift/availability?namespace=${checkNs}`)
            .then(res => res.json())
            .then(data => {
                setForkliftAvailable(data.available);
                setForkliftMessage(data.message || '');
                if (data.defaultNamespace) setForkliftNamespace(data.defaultNamespace);
            })
            .catch(err => {
                console.error("Failed to check Forklift availability:", err);
                setForkliftAvailable(false);
                setForkliftMessage("Failed to check Forklift availability.");
            });
    }, [forkliftNamespace]);

    // Check Forklift availability on mount
    useEffect(() => {
        checkForkliftAvailability();
    }, [checkForkliftAvailability]);

    const fetchForkliftProviders = async () => {
        try {
            const response = await fetch('/api/v1/forklift/providers');
            if (!response.ok) throw new Error("Failed to fetch Forklift providers");
            const data = await response.json();
            setForkliftProviders(data || []);
        } catch (err) {
            console.error("Failed to fetch Forklift providers:", err);
        }
    };

    const fetchForkliftPlans = async () => {
        try {
            const response = await fetch('/api/v1/forklift/plans');
            if (!response.ok) throw new Error("Failed to fetch Forklift plans");
            const data = await response.json();
            setForkliftPlans(data || []);
        } catch (err) {
            console.error("Failed to fetch Forklift plans:", err);
        }
    };

    // Fetch Forklift data when available
    useEffect(() => {
        if (forkliftAvailable) {
            fetchForkliftProviders();
            fetchForkliftPlans();
        }
    }, [forkliftAvailable]);

    const handleSaveForkliftProvider = async (payload, isEdit) => {
        const url = isEdit ? `/api/v1/forklift/providers/${payload.namespace}/${payload.name}` : '/api/v1/forklift/providers';
        const method = isEdit ? 'PUT' : 'POST';

        try {
            const response = await fetch(url, {
                method: method,
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(payload),
            });
            if (!response.ok) {
                const errData = await response.json();
                throw new Error(errData.error || `Failed to ${isEdit ? 'update' : 'create'} Forklift provider`);
            }
            fetchForkliftProviders();
            setShowForkliftProviderWizard(false);
            setForkliftProviderToEdit(null);
        } catch (err) {
            console.error("Failed to save Forklift provider:", err);
            alert(`Error saving Forklift provider: ${err.message}`);
        }
    };

    const handleDeleteForkliftProvider = async () => {
        if (!forkliftProviderToDelete) return;
        try {
            await fetch(`/api/v1/forklift/providers/${forkliftProviderToDelete.metadata.namespace}/${forkliftProviderToDelete.metadata.name}`, { method: 'DELETE' });
            fetchForkliftProviders();
            setForkliftProviderToDelete(null);
        } catch (err) {
            console.error("Failed to delete Forklift provider:", err);
        }
    };

    const handleEditForkliftProvider = async (provider) => {
        try {
            const response = await fetch(`/api/v1/forklift/providers/${provider.metadata.namespace}/${provider.metadata.name}`);
            if (!response.ok) throw new Error("Failed to fetch Forklift provider details");
            const data = await response.json();
            setForkliftProviderToEdit(data);
            setShowForkliftProviderWizard(true);
        } catch (err) {
            console.error("Failed to fetch Forklift provider details:", err);
            alert(`Error: ${err.message}`);
        }
    };

    const handleDeleteForkliftPlan = async () => {
        if (!forkliftPlanToDelete) return;
        try {
            await fetch(`/api/v1/forklift/plans/${forkliftPlanToDelete.metadata.namespace}/${forkliftPlanToDelete.metadata.name}`, { method: 'DELETE' });
            fetchForkliftPlans();
            setForkliftPlanToDelete(null);
        } catch (err) {
            console.error("Failed to delete Forklift plan:", err);
        }
    };

    const handleRunForkliftMigration = async (plan) => {
        const ns = plan.metadata.namespace;
        const name = plan.metadata.name;
        try {
            // Check if a migration already exists for this plan
            const statusRes = await fetch(`/api/v1/forklift/plans/${ns}/${name}/migration`);
            if (statusRes.ok) {
                const statusData = await statusRes.json();
                if (statusData.metadata && statusData.metadata.name) {
                    // A migration CR already exists
                    if (!window.confirm(
                        `A migration "${statusData.metadata.name}" already exists for plan "${name}".\n\n` +
                        `Do you want to delete the existing migration and start a new one?`
                    )) return;

                    // Delete the existing migration
                    const delRes = await fetch(`/api/v1/forklift/plans/${ns}/${name}/migration`, { method: 'DELETE' });
                    if (!delRes.ok) {
                        const errData = await delRes.json();
                        throw new Error(errData.error || "Failed to delete existing migration");
                    }
                } else {
                    // No existing migration — confirm normally
                    if (!window.confirm(`Start migration for plan "${name}"? This will create a Migration CR.`)) return;
                }
            }

            // Create the new migration
            const response = await fetch(`/api/v1/forklift/plans/${ns}/${name}/run`, { method: 'POST' });
            if (!response.ok) {
                const errData = await response.json();
                throw new Error(errData.error || "Failed to start migration");
            }
            alert("Migration started successfully!");
            fetchForkliftPlans();
        } catch (err) {
            console.error("Failed to start migration:", err);
            alert(`Error starting migration: ${err.message}`);
        }
    };

    const fetchPlans = async () => {
        setIsLoading(true);
        try {
            const response = await fetch('/api/v1/plans');
            if (!response.ok) {
                const errData = await response.json();
                throw new Error(errData.error || "Failed to fetch plans");
            }
            const data = await response.json();
            setPlans(data || []);
        } catch (err) {
            console.error("Failed to fetch plans:", err);
            // alert(`Error fetching plans: ${err.message}`);
            setPlans([]); // Ensure plans is an array on error
        } finally {
            setIsLoading(false);
        }
    };

    const fetchSources = async () => {
        try {
            const response = await fetch('/api/v1/harvester/vmwaresources');
            if (!response.ok) {
                const errData = await response.json();
                throw new Error(errData.error || "Failed to fetch sources");
            }
            const data = await response.json();
            setSources(data || []);
        } catch (err) {
            console.error("Failed to fetch sources:", err);
            // alert(`Error fetching sources: ${err.message}`);
        }
    };

    const fetchOvaSources = async () => {
        try {
            const response = await fetch('/api/v1/harvester/ovasources');
            if (!response.ok) {
                const errData = await response.json();
                throw new Error(errData.error || "Failed to fetch OVA sources");
            }
            const data = await response.json();
            setOvaSources(data || []);
        } catch (err) {
            console.error("Failed to fetch OVA sources:", err);
            // alert(`Error fetching OVA sources: ${err.message}`);
        }
    };

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
    }, [refreshInterval, expandedPlans, autoRefresh, forkliftAvailable]);

    const handleCreatePlan = async (planPayload) => {
        try {
            const response = await fetch('/api/v1/plans', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(planPayload),
            });
            if (!response.ok) {
                const errData = await response.json();
                throw new Error(errData.error || "Failed to create plan");
            }
            await response.json();
            fetchPlans(); // Refresh the list
            setPage('plans');
        } catch (err) {
            console.error("Failed to create plan:", err);
            alert(`Error creating plan: ${err.message}`);
        }
    };

    const handleDeletePlan = async () => {
        if (!planToDelete) return;
        try {
            await fetch(`/api/v1/plans/${planToDelete.metadata.namespace}/${planToDelete.metadata.name}`, {
                method: 'DELETE',
            });
            fetchPlans(); // Refresh the list
            setPlanToDelete(null); // Close the modal
        } catch (err) {
            console.error("Failed to delete plan:", err);
        }
    };

    const handleSaveVmicPlan = async (plan, updates) => {
        try {
            const response = await fetch(`/api/v1/plans/${plan.metadata.namespace}/${plan.metadata.name}`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(updates),
            });
            if (!response.ok) {
                const errData = await response.json();
                throw new Error(errData.error || 'Failed to update plan');
            }
            setPlanToEdit(null);
            fetchPlans();
        } catch (err) {
            console.error("Failed to update plan:", err);
            alert(`Error updating plan: ${err.message}`);
        }
    };

    const handleSaveSource = async (payload, isEdit) => {
        const url = isEdit ? `/api/v1/harvester/vmwaresources/${payload.namespace}/${payload.name}` : '/api/v1/harvester/vmwaresources';
        const method = isEdit ? 'PUT' : 'POST';

        try {
            const response = await fetch(url, {
                method: method,
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(payload),
            });
            if (!response.ok) {
                const errData = await response.json();
                throw new Error(errData.error || `Failed to ${isEdit ? 'update' : 'create'} source`);
            }
            fetchSources();
            setShowSourceWizard(false);
            setSourceToEdit(null);
        } catch (err) {
            console.error(`Failed to save source:`, err);
            alert(`Error saving source: ${err.message}`);
        }
    };

    const handleDeleteSource = async () => {
        if (!sourceToDelete) return;
        try {
            await fetch(`/api/v1/harvester/vmwaresources/${sourceToDelete.metadata.namespace}/${sourceToDelete.metadata.name}`, {
                method: 'DELETE',
            });
            fetchSources();
            setSourceToDelete(null);
        } catch (err) {
            console.error("Failed to delete source:", err);
        }
    };

    const handleSaveOvaSource = async (payload, isEdit) => {
        const url = isEdit ? `/api/v1/harvester/ovasources/${payload.namespace}/${payload.name}` : '/api/v1/harvester/ovasources';
        const method = isEdit ? 'PUT' : 'POST';

        try {
            const response = await fetch(url, {
                method: method,
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(payload),
            });
            if (!response.ok) {
                const errData = await response.json();
                throw new Error(errData.error || `Failed to ${isEdit ? 'update' : 'create'} OVA source`);
            }
            fetchOvaSources();
            setShowOvaSourceWizard(false);
            setOvaSourceToEdit(null);
        } catch (err) {
            console.error(`Failed to save OVA source:`, err);
            alert(`Error saving OVA source: ${err.message}`);
        }
    };

    const handleDeleteOvaSource = async () => {
        if (!ovaSourceToDelete) return;
        try {
            await fetch(`/api/v1/harvester/ovasources/${ovaSourceToDelete.metadata.namespace}/${ovaSourceToDelete.metadata.name}`, {
                method: 'DELETE',
            });
            fetchOvaSources();
            setOvaSourceToDelete(null);
        } catch (err) {
            console.error("Failed to delete OVA source:", err);
        }
    };

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

    const handleEditSource = async (source) => {
        try {
            const response = await fetch(`/api/v1/harvester/vmwaresources/${source.metadata.namespace}/${source.metadata.name}`);
            if (!response.ok) {
                const errData = await response.json();
                throw new Error(errData.error || "Failed to fetch source details");
            }
            const data = await response.json();
            setSourceToEdit(data);
            setShowSourceWizard(true);
        } catch (err) {
            console.error("Failed to fetch source details:", err);
            alert(`Error fetching source details: ${err.message}`);
        }
    };

    const handleEditOvaSource = async (source) => {
        try {
            const response = await fetch(`/api/v1/harvester/ovasources/${source.metadata.namespace}/${source.metadata.name}`);
            if (!response.ok) {
                const errData = await response.json();
                throw new Error(errData.error || "Failed to fetch OVA source details");
            }
            const data = await response.json();
            setOvaSourceToEdit(data);
            setShowOvaSourceWizard(true);
        } catch (err) {
            console.error("Failed to fetch OVA source details:", err);
            alert(`Error fetching OVA source details: ${err.message}`);
        }
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