import { useState, useEffect } from 'react';

export const OvaSourceWizard = ({ onCancel, onSave, source }) => {
    const [name, setName] = useState('');
    const [namespace, setNamespace] = useState('default');
    const [url, setUrl] = useState('');
    const [httpTimeoutSeconds, setHttpTimeoutSeconds] = useState('');
    const [username, setUsername] = useState('');
    const [password, setPassword] = useState('');
    const isEditMode = !!source;

    useEffect(() => {
        if (isEditMode) {
            setName(source.metadata.name);
            setNamespace(source.metadata.namespace);
            setUrl(source.spec.url || '');
            setHttpTimeoutSeconds(source.spec.httpTimeoutSeconds || '');
            setUsername(source.spec.username || '');
        }
    }, [source, isEditMode]);

    const handleSubmit = () => {
        const payload = {
            name,
            namespace,
            url,
            httpTimeoutSeconds: httpTimeoutSeconds ? parseInt(httpTimeoutSeconds, 10) : 0,
            username,
            password
        };
        onSave(payload, isEditMode);
    };

    return (
        <div className="fixed inset-0 bg-opacity-50 flex justify-center items-center p-4 z-50">
            <div className="bg-card rounded-lg shadow-xl w-full max-w-lg">
                <div className="p-4 border-b">
                    <h2 className="text-xl font-semibold">{isEditMode ? 'Edit' : 'Create'} OVA Source</h2>
                </div>
                <div className="p-6 space-y-4">
                    <div>
                        <label className="block text-sm font-medium text-main">Name</label>
                        <input type="text" value={name} onChange={e => setName(e.target.value)} disabled={isEditMode} className="mt-1 block w-full form-input" />
                    </div>
                    <div>
                        <label className="block text-sm font-medium text-main">Namespace</label>
                        <input type="text" value={namespace} onChange={e => setNamespace(e.target.value)} disabled={isEditMode} className="mt-1 block w-full form-input" />
                    </div>
                    <div>
                        <label className="block text-sm font-medium text-main">OVA File URL</label>
                        <input type="text" placeholder="http://192.168.0.1:8080/example.ova" value={url} onChange={e => setUrl(e.target.value)} className="mt-1 block w-full form-input" />
                    </div>
                    <div>
                        <label className="block text-sm font-medium text-main">HTTP Timeout (Seconds)</label>
                        <input type="number" placeholder="Optional. Default is 600" value={httpTimeoutSeconds} onChange={e => setHttpTimeoutSeconds(e.target.value)} className="mt-1 block w-full form-input" />
                    </div>
                    <div>
                        <label className="block text-sm font-medium text-main">Username</label>
                        <input type="text" value={username} onChange={e => setUsername(e.target.value)} className="mt-1 block w-full form-input" placeholder="Optional" />
                    </div>
                    <div>
                        <label className="block text-sm font-medium text-main">Password</label>
                        <input type="password" value={password} onChange={e => setPassword(e.target.value)} className="mt-1 block w-full form-input" placeholder={isEditMode ? "Leave blank to keep existing password" : "Optional"} />
                    </div>
                </div>
                <div className="p-4 border-t flex justify-end space-x-2">
                    <button onClick={onCancel} className="btn-secondary">Cancel</button>
                    <button onClick={handleSubmit} className="bg-blue-500 hover:bg-blue-600 text-white font-semibold py-2 px-4 rounded-md">Save</button>
                </div>
            </div>
        </div>
    );
};
