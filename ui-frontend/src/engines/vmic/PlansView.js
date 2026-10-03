import { Header } from '../../shared/Header';
import { handleSort } from '../../shared/sorting';
import { ResourceTable } from './ResourceTable';

// The VM Import Controller's "Migration Plans" page.
export const PlansView = ({ vmic, nav, ui }) => (
    <>
        <Header title="VM Migration Plans" onButtonClick={() => nav.setPage('createPlan')} />
        {vmic.isLoading ? <p>Loading plans...</p> : <ResourceTable
            plans={vmic.sortedPlans}
            onViewDetails={nav.handleViewDetails}
            onDelete={vmic.setPlanToDelete}
            onEdit={vmic.setPlanToEdit}
            sortConfig={vmic.plansSort}
            onSort={handleSort(vmic.setPlansSort)}
            expandedPlans={ui.expandedPlans}
            toggleExpand={ui.toggleExpand}
            selectedDisks={ui.selectedDisks}
            setSelectedDisks={ui.setSelectedDisks}
        />}
    </>
);
