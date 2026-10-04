import { useState } from 'react';
import { AlertTriangle, CheckCircle2 } from 'lucide-react';

const CONTROLLER_EXAMPLE = `apiVersion: forklift.konveyor.io/v1beta1
kind: ForkliftController
metadata:
  name: forklift-controller
  namespace: forklift
spec:
  feature_ui_plugin: "false"`;

// What to do for each reason the backend can give (see docs/contract-forklift.md for the
// sequence). `steps` is the setup sequence and `done` how many of them already hold.
const SETUP_STEPS = [
    {
        title: 'cert-manager is installed',
        body: <>Forklift needs <code>cert-manager</code> (the <code>certificates.cert-manager.io</code> CRD).</>,
    },
    {
        title: 'Enable the Forklift operator add-on',
        body: <>In Harvester: Advanced → Add-ons → <code>forklift-operator</code> → Enable. It installs the operator and the <code>forklift.konveyor.io</code> CRDs.</>,
    },
    {
        title: 'Create the ForkliftController',
        body: (
            <>
                The operator starts the Forklift services once a <code>ForkliftController</code> exists (example; adjust to your setup):
                <pre className="mt-2 p-2 bg-app border border-main rounded text-xs text-left overflow-x-auto">{CONTROLLER_EXAMPLE}</pre>
            </>
        ),
    },
];

const REASONS = {
    'not-installed': { title: 'Forklift is not installed', done: 0 },
    'not-ready': { title: 'Forklift is installed but not ready', done: 2 },
    forbidden: {
        title: 'No permission to check Forklift',
        help: <>Your account may not read <code>providers.forklift.konveyor.io</code> in this namespace, so Forklift may well be set up. Ask a cluster admin for read access, or try another namespace.</>,
    },
    unknown: {
        title: 'Could not check Forklift',
        help: <>The check itself failed, so nothing is known about Forklift. Try again; if it keeps failing, the message above is the cause.</>,
    },
};

export const ForkliftUnavailable = ({ state, message, namespace, onChangeNamespace, onRetry }) => {
    const [editNs, setEditNs] = useState(namespace || 'forklift');
    const reason = REASONS[state];
    return (
        <div className="flex flex-col items-center justify-center py-16 text-center">
            <AlertTriangle size={48} className="text-yellow-500 mb-4" />
            <h3 className="text-lg font-semibold text-main mb-2">{reason ? reason.title : 'Forklift Unavailable'}</h3>
            <p className="text-secondary text-sm max-w-md mb-4">{message || 'Forklift host Provider not found. Install and configure Forklift to use this feature.'}</p>
            {reason && reason.help && <p className="text-secondary text-sm max-w-md mb-4">{reason.help}</p>}
            {reason && reason.done !== undefined && (
                <ol className="text-left text-sm max-w-xl mb-4 space-y-3">
                    {SETUP_STEPS.map((step, i) => (
                        <li key={step.title} className="flex items-start">
                            {i < reason.done
                                ? <CheckCircle2 aria-label="done" size={18} className="text-green-500 mr-2 mt-0.5 shrink-0" />
                                : <span className="mr-2 mt-0.5 w-[18px] h-[18px] shrink-0 rounded-full border border-main text-[11px] flex items-center justify-center text-secondary">{i + 1}</span>}
                            <div>
                                <div className={`font-medium ${i < reason.done ? 'text-secondary' : 'text-main'}`}>{step.title}</div>
                                <div className="text-secondary">{step.body}</div>
                            </div>
                        </li>
                    ))}
                </ol>
            )}
            <div className="flex items-center space-x-2 mt-2">
                <label className="text-sm text-secondary">Namespace:</label>
                <input
                    type="text"
                    value={editNs}
                    onChange={e => setEditNs(e.target.value)}
                    className="form-input text-sm w-48"
                    placeholder="e.g. forklift"
                />
                <button
                    onClick={() => { onChangeNamespace(editNs); onRetry(editNs); }}
                    className="bg-blue-500 hover:bg-blue-600 text-white text-sm font-semibold py-1.5 px-4 rounded-md"
                >
                    Retry
                </button>
            </div>
        </div>
    );
};
