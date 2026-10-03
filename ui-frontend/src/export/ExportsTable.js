import { useState } from 'react';
import { formatBytes } from '../utils';
import { Loader, Download } from 'lucide-react';
import { requestDownload, withApiBase, apiBaseFor } from '../exportDownload';

export const EXPORT_TERMINAL = ['Ready', 'Failed'];

export const ExportsTable = ({ exports, onDelete, onLogs, isBusy }) => {
    const [preparing, setPreparing] = useState({});
    if (!exports.length) {
        return <div className="text-sm text-secondary py-4 text-center">No exports yet.</div>;
    }
    return (
        <div className="border border-main rounded-md overflow-x-auto">
            <table className="w-full text-sm">
                <thead className="bg-app">
                    <tr className="text-left text-secondary">
                        <th className="px-3 py-2 font-medium">VM</th>
                        <th className="px-3 py-2 font-medium">Profile</th>
                        <th className="px-3 py-2 font-medium">Phase</th>
                        <th className="px-3 py-2 font-medium">Progress</th>
                        <th className="px-3 py-2 font-medium">Size</th>
                        <th className="px-3 py-2 font-medium">Actions</th>
                    </tr>
                </thead>
                <tbody>
                    {exports.map(e => {
                        const done = EXPORT_TERMINAL.includes(e.phase);
                        const failed = e.phase === 'Failed';
                        return (
                            <tr key={e.exportId} className="border-t border-main align-top">
                                <td className="px-3 py-2 text-main">
                                    {e.vmName}
                                    <div className="text-xs text-secondary">{e.namespace}</div>
                                </td>
                                <td className="px-3 py-2 text-secondary">{e.profile}</td>
                                <td className="px-3 py-2">
                                    <span className={`px-2 py-0.5 rounded-full text-xs font-medium ${
                                        e.phase === 'Ready' ? 'bg-green-100 text-green-800'
                                        : failed ? 'bg-red-100 text-red-800'
                                        : 'bg-blue-100 text-blue-800'}`}>{e.phase}</span>
                                    {e.error && <div className="text-xs text-red-600 mt-1 max-w-xs break-words">{e.error}</div>}
                                </td>
                                <td className="px-3 py-2 text-secondary" style={{ minWidth: '9rem' }}>
                                    {done ? (e.stage || '—') : (
                                        <>
                                            <div className="w-full bg-app rounded-full h-1.5 border border-main">
                                                <div className="bg-blue-500 h-full rounded-full transition-all" style={{ width: `${e.percent || 0}%` }} />
                                            </div>
                                            <div className="text-xs mt-1">{e.percent || 0}% {e.stage ? `· ${e.stage}` : ''}</div>
                                        </>
                                    )}
                                </td>
                                <td className="px-3 py-2 text-secondary">{e.sizeBytes ? formatBytes(e.sizeBytes) : '—'}</td>
                                <td className="px-3 py-2">
                                    <div className="flex items-center gap-2">
                                        {e.downloadable && (
                                            <button
                                                onClick={async () => {
                                                    setPreparing((p) => ({ ...p, [e.exportId]: true }));
                                                    await downloadExport(e);
                                                    setPreparing((p) => ({ ...p, [e.exportId]: false }));
                                                }}
                                                disabled={!!preparing[e.exportId]}
                                                title="Starts a short-lived service next to the export volume, then downloads through your browser"
                                                className="text-blue-600 hover:underline text-xs flex items-center disabled:opacity-60"
                                            >
                                                {preparing[e.exportId]
                                                    ? <><Loader size={12} className="mr-1 animate-spin" /> Preparing…</>
                                                    : <><Download size={12} className="mr-1" /> Download</>}
                                            </button>
                                        )}
                                        <button onClick={() => onLogs(e)} className="text-blue-600 hover:underline text-xs">Logs</button>
                                        <button onClick={() => onDelete(e)} disabled={isBusy} className="text-red-600 hover:underline text-xs">Delete</button>
                                    </div>
                                    {e.ovaPath && <div className="text-xs text-secondary mt-1 font-mono break-all">{e.ovaPath}</div>}
                                </td>
                            </tr>
                        );
                    })}
                </tbody>
            </table>
        </div>
    );
};

// The file is never loaded into the page. We ask the backend for a signed URL (a
// short-lived ticket; the export's serve pod may need a moment to start) and
// then let the browser download that URL itself, so it streams to disk with its
// own progress and resume. The URL is built from apiBase like every other API
// path, so it also works behind the Rancher proxy.
export const downloadExport = async (e, onWaiting) => {
    try {
        const { url, fileName } = await requestDownload(window.fetch.bind(window), e, { onWaiting });
        const a = document.createElement('a');
        a.href = withApiBase(apiBaseFor(window.location.pathname), url);
        a.download = fileName;
        document.body.appendChild(a);
        a.click();
        a.remove();
    } catch (err) {
        alert(`Download failed: ${err.message}`);
    }
};
