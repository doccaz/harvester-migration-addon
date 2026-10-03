import { useState, useEffect, useMemo, useCallback } from 'react';
import { sortData } from '../../shared/sorting';

// The Forklift engine's state: availability, providers, plans, their sorting,
// the provider wizard / delete dialogs and the handlers that call the API.
export function useForklift() {
    // --- Forklift State ---
    const [forkliftAvailable, setForkliftAvailable] = useState(null); // null = loading, true/false
    const [forkliftMessage, setForkliftMessage] = useState('');
    const [forkliftNamespace, setForkliftNamespace] = useState('forklift');
    const [forkliftProviders, setForkliftProviders] = useState([]);
    const [forkliftPlans, setForkliftPlans] = useState([]);
    const [showForkliftProviderWizard, setShowForkliftProviderWizard] = useState(false);
    const [forkliftWizardDefaultType, setForkliftWizardDefaultType] = useState('vsphere');
    const [forkliftProviderToEdit, setForkliftProviderToEdit] = useState(null);
    const [forkliftProviderToDelete, setForkliftProviderToDelete] = useState(null);
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

    return {
        forkliftAvailable,
        setForkliftAvailable,
        forkliftMessage,
        setForkliftMessage,
        forkliftNamespace,
        setForkliftNamespace,
        forkliftProviders,
        setForkliftProviders,
        forkliftPlans,
        setForkliftPlans,
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
        sortedForkliftProviders,
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
    };
}
