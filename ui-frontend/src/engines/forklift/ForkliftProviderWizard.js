import { useState, useEffect } from 'react';
import { Info } from 'lucide-react';

export const ForkliftProviderWizard = ({ onCancel, onSave, source, defaultNamespace, defaultProviderType }) => {
    const [name, setName] = useState('');
    const [namespace, setNamespace] = useState(defaultNamespace || 'forklift');
    const [url, setUrl] = useState('');
    const [username, setUsername] = useState('');
    const [password, setPassword] = useState('');
    const [sdkEndpoint, setSdkEndpoint] = useState('vcenter');
    const [providerType, setProviderType] = useState(defaultProviderType || 'vsphere');
    const [insecureSkipVerify, setInsecureSkipVerify] = useState(true);
    const [cacert, setCacert] = useState('');
    const [vddkInitImage, setVddkInitImage] = useState('');
    const isEditMode = !!source;

    useEffect(() => {
        if (isEditMode) {
            setName(source.metadata.name);
            setNamespace(source.metadata.namespace);
            setUrl(source.spec.url || '');
            setUsername(source.spec.username || '');
            setSdkEndpoint(source.spec?.settings?.sdkEndpoint || 'vcenter');
            setProviderType(source.spec?.type || 'vsphere');
            setInsecureSkipVerify(source.spec?.insecureSkipVerify !== 'false');
            setVddkInitImage(source.spec?.settings?.vddkInitImage || '');
        }
    }, [source, isEditMode]);

    const handleSubmit = () => {
        if (providerType === 'ova') {
            onSave({ name, namespace, url, providerType: 'ova' }, isEditMode);
        } else {
            onSave({
                name, namespace, url, username, password, sdkEndpoint, providerType: 'vsphere',
                insecureSkipVerify,
                cacert: insecureSkipVerify ? '' : cacert,
                vddkInitImage,
            }, isEditMode);
        }
    };

    return (
        <div className="fixed inset-0 bg-opacity-50 flex justify-center items-center p-4 z-50">
            <div className="bg-card rounded-lg shadow-xl w-full max-w-lg">
                <div className="p-4 border-b">
                    <h2 className="text-xl font-semibold">{isEditMode ? 'Edit' : 'Create'} Forklift Provider</h2>
                </div>
                <div className="p-6 space-y-4">
                    {!isEditMode && (
                        <div>
                            <label className="block text-sm font-medium text-main">Provider Type</label>
                            <div className="mt-1 flex space-x-4">
                                <label className="inline-flex items-center cursor-pointer" title="Import VMs from a VMware vSphere environment (vCenter or ESXi).">
                                    <input type="radio" className="form-radio text-blue-600" name="providerType" value="vsphere" checked={providerType === 'vsphere'} onChange={() => setProviderType('vsphere')} />
                                    <span className="ml-2 text-sm text-main">vSphere</span>
                                </label>
                                <label className="inline-flex items-center cursor-pointer" title="Import VMs from OVA/OVF files on an NFS share.">
                                    <input type="radio" className="form-radio text-blue-600" name="providerType" value="ova" checked={providerType === 'ova'} onChange={() => setProviderType('ova')} />
                                    <span className="ml-2 text-sm text-main">OVA (NFS)</span>
                                </label>
                            </div>
                            <p className="text-xs text-secondary mt-1">
                                {providerType === 'ova'
                                    ? 'Import virtual machines from OVA/OVF files stored on an NFS share. Forklift will scan the share and discover available VMs.'
                                    : 'Import virtual machines from a live VMware vSphere environment.'}
                            </p>
                        </div>
                    )}
                    <div>
                        <label className="block text-sm font-medium text-main">Name</label>
                        <input type="text" value={name} onChange={e => setName(e.target.value)} disabled={isEditMode} className="mt-1 block w-full form-input" />
                    </div>
                    <div>
                        <label className="block text-sm font-medium text-main">Namespace</label>
                        <input type="text" value={namespace} onChange={e => setNamespace(e.target.value)} disabled={isEditMode} className="mt-1 block w-full form-input" />
                    </div>
                    {providerType === 'vsphere' && (
                        <div>
                            <label className="block text-sm font-medium text-main">Endpoint Type</label>
                            <div className="mt-1 flex space-x-4">
                                <label className="inline-flex items-center cursor-pointer" title="Connect to a VMware vCenter Server that manages one or more ESXi hosts.">
                                    <input type="radio" className="form-radio text-blue-600" name="sdkEndpoint" value="vcenter" checked={sdkEndpoint === 'vcenter'} onChange={() => setSdkEndpoint('vcenter')} />
                                    <span className="ml-2 text-sm text-main">vCenter Server</span>
                                </label>
                                <label className="inline-flex items-center cursor-pointer" title="Connect directly to a standalone ESXi host (not managed by vCenter).">
                                    <input type="radio" className="form-radio text-blue-600" name="sdkEndpoint" value="esxi" checked={sdkEndpoint === 'esxi'} onChange={() => setSdkEndpoint('esxi')} />
                                    <span className="ml-2 text-sm text-main">Standalone ESXi</span>
                                </label>
                            </div>
                            <p className="text-xs text-secondary mt-1">
                                {sdkEndpoint === 'esxi'
                                    ? 'Connects directly to an ESXi host using the esx:// protocol. Use this when your host is not managed by a vCenter Server.'
                                    : 'Connects to a vCenter Server using the vpx:// protocol. This is the default for most VMware environments.'}
                            </p>
                        </div>
                    )}
                    <div>
                        <label className="block text-sm font-medium text-main">
                            {providerType === 'ova' ? 'NFS Path' : sdkEndpoint === 'esxi' ? 'ESXi Host URL' : 'vCenter URL'}
                        </label>
                        <input type="text" placeholder={providerType === 'ova' ? '10.0.0.1:/exports/vms' : sdkEndpoint === 'esxi' ? 'https://esxi-host.example.com/sdk' : 'https://vcenter.example.com/sdk'} value={url} onChange={e => setUrl(e.target.value)} className="mt-1 block w-full form-input" />
                    </div>
                    {providerType === 'vsphere' && (
                        <>
                            <div>
                                <label className="block text-sm font-medium text-main">Username</label>
                                <input type="text" value={username} onChange={e => setUsername(e.target.value)} className="mt-1 block w-full form-input" placeholder={sdkEndpoint === 'esxi' ? 'root' : 'administrator@vsphere.local'} />
                            </div>
                            <div>
                                <label className="block text-sm font-medium text-main">Password</label>
                                <input type="password" value={password} onChange={e => setPassword(e.target.value)} className="mt-1 block w-full form-input" placeholder={isEditMode ? "Leave blank to keep existing password" : ""} />
                            </div>
                            <div className="border-t pt-4 mt-2">
                                <div className="flex items-center">
                                    <input type="checkbox" id="insecureSkipVerify"
                                        checked={insecureSkipVerify}
                                        onChange={e => setInsecureSkipVerify(e.target.checked)}
                                        className="h-4 w-4 text-blue-600 focus:ring-blue-500 border-main rounded" />
                                    <label htmlFor="insecureSkipVerify" className="ml-2 block text-sm font-medium text-main">
                                        Skip TLS certificate verification
                                    </label>
                                </div>
                                <p className="text-xs text-secondary mt-1">
                                    {insecureSkipVerify
                                        ? 'All TLS certificates will be accepted without validation. Not recommended for production.'
                                        : 'TLS certificates will be validated. Provide a CA certificate below if using a private CA.'}
                                </p>
                                {!insecureSkipVerify && (
                                    <div className="mt-3">
                                        <label className="block text-sm font-medium text-main">CA Certificate (PEM)</label>
                                        <textarea rows={5} value={cacert} onChange={e => setCacert(e.target.value)}
                                            className="mt-1 block w-full form-input font-mono text-xs"
                                            placeholder={"-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----"} />
                                        <p className="text-xs text-secondary mt-1">
                                            Paste the PEM-encoded CA certificate used to sign your {sdkEndpoint === 'esxi' ? 'ESXi host' : 'vCenter server'} certificate.
                                        </p>
                                    </div>
                                )}
                            </div>
                            <div>
                                <div className="flex items-center space-x-1">
                                    <label className="block text-sm font-medium text-main">VDDK Init Image</label>
                                    <Info size={14} className="text-secondary cursor-help"
                                        title="VMware Virtual Disk Development Kit (VDDK) container image. Dramatically improves disk transfer speed and is required for warm migrations and vSAN-backed VMs. Build from VMware's VDDK SDK (download from Broadcom/VMware), create a container image, and push to your registry." />
                                </div>
                                <input type="text" placeholder="registry.example.com/vddk:v8.0.3"
                                    value={vddkInitImage} onChange={e => setVddkInitImage(e.target.value)}
                                    className="mt-1 block w-full form-input" />
                                <p className="text-xs text-secondary mt-1">Optional. When empty, Forklift uses a slower fallback transfer method.</p>
                            </div>
                        </>
                    )}
                </div>
                <div className="p-4 border-t flex justify-end space-x-2">
                    <button onClick={onCancel} className="btn-secondary">Cancel</button>
                    <button onClick={handleSubmit} className="bg-blue-500 hover:bg-blue-600 text-white font-semibold py-2 px-4 rounded-md">Save</button>
                </div>
            </div>
        </div>
    );
};
