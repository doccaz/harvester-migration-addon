import { RefreshCw } from 'lucide-react';
import { Header } from '../../shared/Header';
import { handleSort } from '../../shared/sorting';
import { ForkliftProvidersTable } from './ForkliftProvidersTable';
import { ForkliftUnavailable } from './ForkliftUnavailable';

// Forklift's provider pages. The vCenter Sources page lists the vSphere providers and
// the OVA Sources page the OVA ones; everything else about them is the same.
const KINDS = {
    vsphere: { title: 'Forklift vSphere Providers', page: 'sources', list: (f) => f.sortedForkliftVsphereProviders },
    ova: { title: 'Forklift OVA Providers', page: 'ovaSources', list: (f) => f.sortedForkliftOvaProviders },
};

export const ProvidersView = ({ kind, forklift, nav }) => {
    const k = KINDS[kind];
    return forklift.forkliftAvailable ? (
        <>
            <Header title={k.title} onButtonClick={() => { forklift.setForkliftProviderToEdit(null); forklift.setForkliftWizardDefaultType(kind); forklift.setShowForkliftProviderWizard(true); }} />
            <ForkliftProvidersTable
                providers={k.list(forklift)}
                onEdit={forklift.handleEditForkliftProvider}
                onDelete={forklift.setForkliftProviderToDelete}
                onViewDetails={(provider) => { nav.setSelectedForkliftProvider(provider); nav.setForkliftProviderReturnPage(k.page); nav.setPage('forkliftProviderDetails'); }}
                onExplore={(provider) => { nav.setSelectedSource({ metadata: provider.metadata, _forkliftProvider: true }); nav.setPage('exploreForkliftSource'); }}
                sortConfig={forklift.forkliftProvidersSort}
                onSort={handleSort(forklift.setForkliftProvidersSort)}
            />
            <div className="flex justify-end items-center mt-4 space-x-2">
                <button onClick={forklift.fetchForkliftProviders} className="text-blue-500 hover:text-blue-700"><RefreshCw size={20} /></button>
            </div>
        </>
    ) : <ForkliftUnavailable state={forklift.forkliftState} message={forklift.forkliftMessage} namespace={forklift.forkliftNamespace} onChangeNamespace={forklift.setForkliftNamespace} onRetry={forklift.checkForkliftAvailability} />;
};
