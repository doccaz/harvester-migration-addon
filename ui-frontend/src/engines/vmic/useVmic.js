import { useState, useMemo } from 'react';
import { sortData } from '../../shared/sorting';

// The VM Import Controller engine's state: plans, vCenter and OVA sources, their
// sorting, the edit / delete dialogs and the handlers that call the API.
// onPlanCreated is how a created plan sends the user back to the plans page.
export function useVmic({ onPlanCreated }) {
    const [plans, setPlans] = useState([]);
    const [sources, setSources] = useState([]);
    const [ovaSources, setOvaSources] = useState([]);
    const [isLoading, setIsLoading] = useState(true);
    const [planToDelete, setPlanToDelete] = useState(null);
    const [planToEdit, setPlanToEdit] = useState(null);
    const [sourceToEdit, setSourceToEdit] = useState(null);
    const [sourceToDelete, setSourceToDelete] = useState(null);
    const [showSourceWizard, setShowSourceWizard] = useState(false);
    const [showOvaSourceWizard, setShowOvaSourceWizard] = useState(false);
    const [ovaSourceToEdit, setOvaSourceToEdit] = useState(null);
    const [ovaSourceToDelete, setOvaSourceToDelete] = useState(null);
    const [plansSort, setPlansSort] = useState({ key: 'metadata.creationTimestamp', direction: 'desc' });
    const [sourcesSort, setSourcesSort] = useState({ key: 'metadata.creationTimestamp', direction: 'desc' });
    const [ovaSourcesSort, setOvaSourcesSort] = useState({ key: 'metadata.creationTimestamp', direction: 'desc' });
    const sortedPlans = useMemo(() => sortData(plans, plansSort), [plans, plansSort]);
    const sortedSources = useMemo(() => sortData(sources, sourcesSort), [sources, sourcesSort]);
    const sortedOvaSources = useMemo(() => sortData(ovaSources, ovaSourcesSort), [ovaSources, ovaSourcesSort]);

    // isLoading starts true and only the first load clears it: a background refresh must
    // not swap the table for "Loading plans...".
    const fetchPlans = async () => {
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
            onPlanCreated();
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

    return {
        plans,
        setPlans,
        sources,
        setSources,
        ovaSources,
        setOvaSources,
        isLoading,
        setIsLoading,
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
    };
}
