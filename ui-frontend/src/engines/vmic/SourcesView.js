import { Header } from '../../shared/Header';
import { handleSort } from '../../shared/sorting';
import { SourcesTable } from './SourcesTable';

// The VM Import Controller's "vCenter Sources" page.
export const SourcesView = ({ vmic, nav }) => (
    <>
        <Header title="vCenter Sources" onButtonClick={() => { vmic.setSourceToEdit(null); vmic.setShowSourceWizard(true); }} />
        <SourcesTable
            sources={vmic.sortedSources}
            onEdit={vmic.handleEditSource}
            onDelete={vmic.setSourceToDelete}
            onViewDetails={(source) => { nav.setSelectedSource(source); nav.setPage('sourceDetails'); }}
            onExplore={(source) => { nav.setSelectedSource(source); nav.setPage('exploreSource'); }}
            sortConfig={vmic.sourcesSort}
            onSort={handleSort(vmic.setSourcesSort)}
        />
    </>
);
