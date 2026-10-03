import { useState } from 'react';
import { Download, AlertTriangle, Loader } from 'lucide-react';

export const SupportBundleCard = () => {
    const [includeInventory, setIncludeInventory] = useState(false);
    const [anonymize, setAnonymize] = useState(false);
    const [generating, setGenerating] = useState(false);
    const [error, setError] = useState(null);

    const handleGenerate = async () => {
        const params = new URLSearchParams();
        if (includeInventory) params.set('inventory', 'true');
        if (anonymize) params.set('anonymize', 'true');
        const qs = params.toString();
        const url = `/api/v1/support-bundle${qs ? `?${qs}` : ''}`;

        setGenerating(true);
        setError(null);
        try {
            const resp = await fetch(url);
            if (!resp.ok) throw new Error(`Server returned ${resp.status}`);
            const blob = await resp.blob();
            const disposition = resp.headers.get('Content-Disposition') || '';
            const match = disposition.match(/filename="?([^"]+)"?/);
            const filename = match ? match[1] : 'vm-import-support.tar.gz';
            const objUrl = URL.createObjectURL(blob);
            const a = document.createElement('a');
            a.href = objUrl;
            a.download = filename;
            document.body.appendChild(a);
            a.click();
            a.remove();
            URL.revokeObjectURL(objUrl);
        } catch (err) {
            setError(err.message);
        } finally {
            setGenerating(false);
        }
    };

    return (
        <div className="bg-card shadow-md rounded-lg p-6 border border-main">
            <h3 className="text-lg font-semibold mb-2 text-main flex items-center">
                <Download size={18} className="mr-2 text-blue-600" /> Support Bundle
            </h3>
            <p className="text-sm text-secondary mb-4">
                Generate a redacted <code>.tar.gz</code> snapshot of cluster capabilities, migration plans (with
                status conditions), network/storage maps, and source/provider definitions — useful for
                troubleshooting and for reproducing issues from a customer's environment. Secret values are
                never included.
            </p>
            <div className="space-y-2 mb-4">
                <label className="flex items-center text-sm text-main">
                    <input
                        type="checkbox"
                        className="form-checkbox text-blue-600 mr-2"
                        checked={includeInventory}
                        disabled={generating}
                        onChange={e => setIncludeInventory(e.target.checked)}
                    />
                    Include vCenter inventory
                    <span className="text-secondary ml-1">(slower; fetches live inventory for every source)</span>
                </label>
                <label className="flex items-center text-sm text-main">
                    <input
                        type="checkbox"
                        className="form-checkbox text-blue-600 mr-2"
                        checked={anonymize}
                        disabled={generating}
                        onChange={e => setAnonymize(e.target.checked)}
                    />
                    Anonymize inventory names
                    <span className="text-secondary ml-1">(hashes VM/folder/network names; name-specific bugs won't reproduce)</span>
                </label>
            </div>
            {error && (
                <p className="text-sm text-red-500 mb-3 flex items-center gap-1">
                    <AlertTriangle size={14} /> {error}
                </p>
            )}
            <button
                onClick={handleGenerate}
                disabled={generating}
                className="inline-flex items-center bg-blue-500 hover:bg-blue-600 disabled:opacity-60 disabled:cursor-not-allowed text-white font-semibold py-2 px-4 rounded-md transition-colors"
            >
                {generating
                    ? <><Loader size={16} className="mr-2 animate-spin" /> Generating…</>
                    : <><Download size={16} className="mr-2" /> Generate Support Bundle</>
                }
            </button>
        </div>
    );
};
