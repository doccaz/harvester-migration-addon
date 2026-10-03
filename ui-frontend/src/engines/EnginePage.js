import { SubTab } from '../shared/SubTab';
import { ENGINES } from './registry';

// A page with one tab per engine. `ctx` carries the engines' state ({ vmic, forklift }),
// the navigation callbacks (nav) and the page-level UI state (ui) to the engine's view;
// `footer` is rendered below the active view.
export const EnginePage = ({ page, active, onChange, ctx, footer }) => {
    const engine = ENGINES.find((e) => e.id === active) || ENGINES[0];
    const View = engine.views[page];
    return (
        <div className="w-full">
            <SubTab
                tabs={ENGINES.map((e) => ({ key: e.id, label: e.label }))}
                activeTab={active}
                onTabChange={onChange}
            />
            <View {...ctx} />
            {footer}
        </div>
    );
};
