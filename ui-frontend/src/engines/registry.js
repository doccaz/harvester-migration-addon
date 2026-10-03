// The migration engines the UI can drive. Each engine contributes a view for every
// page that has per-engine content (plans, vCenter sources, OVA sources); the page
// builds its sub-tabs from this list, so a new engine is one entry here plus its views.
import { PlansView as VmicPlansView } from './vmic/PlansView';
import { SourcesView as VmicSourcesView } from './vmic/SourcesView';
import { OvaSourcesView as VmicOvaSourcesView } from './vmic/OvaSourcesView';
import { PlansView as ForkliftPlansView } from './forklift/PlansView';
import { ProvidersView as ForkliftProvidersView } from './forklift/ProvidersView';

export const ENGINES = [
    {
        id: 'vmic',
        label: 'VM Import Controller',
        views: { plans: VmicPlansView, sources: VmicSourcesView, ovaSources: VmicOvaSourcesView },
    },
    {
        id: 'forklift',
        label: 'Forklift',
        views: {
            plans: ForkliftPlansView,
            sources: (props) => <ForkliftProvidersView kind="vsphere" {...props} />,
            ovaSources: (props) => <ForkliftProvidersView kind="ova" {...props} />,
        },
    },
];
