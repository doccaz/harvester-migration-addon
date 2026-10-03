import { RefreshCw } from 'lucide-react';
import { Header } from '../../shared/Header';
import { handleSort } from '../../shared/sorting';
import { OvaSourcesTable } from './OvaSourcesTable';

// The VM Import Controller's "OVA Sources" page.
export const OvaSourcesView = ({ vmic, nav, ui }) => (
    <>
        <Header title="OVA Sources" onButtonClick={() => { vmic.setOvaSourceToEdit(null); vmic.setShowOvaSourceWizard(true); }} />
        <OvaSourcesTable sources={vmic.sortedOvaSources} onEdit={vmic.handleEditOvaSource} onDelete={vmic.setOvaSourceToDelete} onViewDetails={(source) => { nav.setSelectedOvaSource(source); nav.setPage('ovaSourceDetails'); }} sortConfig={vmic.ovaSourcesSort} onSort={handleSort(vmic.setOvaSourcesSort)} />
        <div className="flex justify-end items-center mt-4 space-x-2">
            <button onClick={vmic.fetchOvaSources} className="text-blue-500 hover:text-blue-700"><RefreshCw size={20} /></button>
            <input type="number" value={ui.refreshInterval} onChange={e => ui.setRefreshInterval(e.target.value)} className="w-20 form-input text-sm" />
            <span className="text-sm text-secondary">seconds</span>
        </div>
    </>
);
