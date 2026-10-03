import { useState } from 'react';
import { AlertTriangle } from 'lucide-react';

export const ForkliftUnavailable = ({ message, namespace, onChangeNamespace, onRetry }) => {
    const [editNs, setEditNs] = useState(namespace || 'forklift');
    return (
        <div className="flex flex-col items-center justify-center py-16 text-center">
            <AlertTriangle size={48} className="text-yellow-500 mb-4" />
            <h3 className="text-lg font-semibold text-main mb-2">Forklift Unavailable</h3>
            <p className="text-secondary text-sm max-w-md mb-4">{message || 'Forklift host Provider not found. Install and configure Forklift to use this feature.'}</p>
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
