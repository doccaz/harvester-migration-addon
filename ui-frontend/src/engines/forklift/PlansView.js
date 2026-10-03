import { Header } from '../../shared/Header';
import { handleSort } from '../../shared/sorting';
import { ForkliftPlansTable } from './ForkliftPlansTable';
import { ForkliftUnavailable } from './ForkliftUnavailable';

// Forklift's "Migration Plans" page.
export const PlansView = ({ forklift, nav, ui }) => (
    forklift.forkliftAvailable ? (
        <>
            <Header title="Forklift Migration Plans" onButtonClick={() => nav.setPage('createPlan')} />
            <ForkliftPlansTable
                plans={forklift.sortedForkliftPlans}
                onDelete={forklift.setForkliftPlanToDelete}
                onViewDetails={(plan) => { nav.setSelectedForkliftPlan(plan); nav.setPage('forkliftPlanDetails'); }}
                sortConfig={forklift.forkliftPlansSort}
                onSort={handleSort(forklift.setForkliftPlansSort)}
                expandedPlans={ui.expandedPlans}
                toggleExpand={ui.toggleExpand}
                onRunMigration={forklift.handleRunForkliftMigration}
            />
        </>
    ) : <ForkliftUnavailable message={forklift.forkliftMessage} namespace={forklift.forkliftNamespace} onChangeNamespace={forklift.setForkliftNamespace} onRetry={forklift.checkForkliftAvailability} />
);
