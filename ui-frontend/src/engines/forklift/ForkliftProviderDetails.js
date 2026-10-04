import { useState } from 'react';
import { X, CheckCircle2, XCircle } from 'lucide-react';
import { formatDate } from '../../utils';
import { CopyButton } from '../../shared/CopyButton';
import { ViewInHarvester } from '../../shared/ViewInHarvester';
import { FORKLIFT } from '../../shared/harvesterLinks';

export const ForkliftProviderDetails = ({ provider, onClose }) => {
    const [yamlContent, setYamlContent] = useState('');
    const [showYaml, setShowYaml] = useState(false);
    const [isLoadingYaml, setIsLoadingYaml] = useState(false);

    const fetchYaml = async () => {
        setIsLoadingYaml(true);
        try {
            const response = await fetch(`/api/v1/forklift/providers/${provider.metadata.namespace}/${provider.metadata.name}/yaml`);
            const data = await response.text();
            setYamlContent(data || "Could not generate YAML.");
        } catch (err) {
            setYamlContent("Failed to fetch YAML.");
        } finally {
            setIsLoadingYaml(false);
        }
    };

    const handleShowYaml = () => {
        if (showYaml) {
            setShowYaml(false);
        } else {
            setShowYaml(true);
            fetchYaml();
        }
    };

    return (
        <div className="fixed inset-0 bg-opacity-50 flex justify-center items-center p-4 z-50">
            <div className="bg-card rounded-lg shadow-xl w-full max-w-2xl flex flex-col max-h-[90vh]">
                <div className="flex justify-between items-center p-4 border-b">
                    <h2 className="text-xl font-semibold text-main">{provider.metadata.name}</h2>
                    <button onClick={onClose} className="p-2 rounded-full hover:bg-gray-200">
                        <X size={20} />
                    </button>
                </div>
                <div className="p-6 space-y-6 overflow-y-auto">
                    <div>
                        <h3 className="text-lg font-medium text-main mb-2">Provider Summary</h3>
                        <div className="p-3 bg-app rounded-md border text-sm space-y-1">
                            <p><strong>Type:</strong> {provider.spec?.type}</p>
                            <p><strong>URL:</strong> {provider.spec?.url}</p>
                            <p><strong>Secret:</strong> {provider.spec?.secret?.namespace}/{provider.spec?.secret?.name}</p>
                            <p><strong>SDK Endpoint:</strong> {provider.spec?.settings?.sdkEndpoint || 'default'}</p>
                            {provider.spec?.type === 'vsphere' && (
                                <>
                                    <p><strong>TLS Verification:</strong> {provider.spec?.insecureSkipVerify === 'false' ? 'Enabled' : 'Skipped (insecure)'}</p>
                                    {provider.spec?.hasCACert && <p><strong>CA Certificate:</strong> Provided</p>}
                                    <p><strong>VDDK Init Image:</strong> {provider.spec?.settings?.vddkInitImage || <span className="text-secondary italic">Not configured (slower fallback)</span>}</p>
                                </>
                            )}
                            <ViewInHarvester resource="provider" object={provider} kind={FORKLIFT} />
                        </div>
                    </div>
                    <div>
                        <h3 className="text-lg font-medium text-main mb-2">Conditions</h3>
                        <div className="space-y-1">
                            {(provider.status?.conditions || []).map((c, i) => (
                                <div key={i} className="flex items-start text-sm border-b border-gray-50 last:border-0 pb-1">
                                    <span className="mt-0.5">
                                        {c.status === 'True' ? <CheckCircle2 size={14} className="text-green-500 mr-1" /> : <XCircle size={14} className="text-red-500 mr-1" />}
                                    </span>
                                    <div className="flex-grow">
                                        <div className="flex justify-between items-center">
                                            <span className="font-medium text-main">{c.type}</span>
                                            <span className="text-[10px] text-secondary opacity-70 font-mono">{formatDate(c.lastTransitionTime)}</span>
                                        </div>
                                        <div className="text-secondary text-xs italic">{c.message || ''}</div>
                                    </div>
                                </div>
                            ))}
                            {(!provider.status?.conditions || provider.status.conditions.length === 0) && <p className="text-xs text-secondary italic">No conditions reported yet.</p>}
                        </div>
                    </div>
                    <div>
                        <button onClick={handleShowYaml} className="text-sm text-blue-600 hover:underline">
                            {showYaml ? 'Hide' : 'View'} YAML
                        </button>
                        {showYaml && (
                            <div className="mt-2 p-2 border rounded-md bg-gray-900 text-white font-mono text-xs max-h-64 overflow-y-auto relative group">
                                <CopyButton text={yamlContent} className="absolute top-2 right-2" />
                                <pre>{isLoadingYaml ? 'Loading...' : yamlContent}</pre>
                            </div>
                        )}
                    </div>
                </div>
                <div className="p-4 border-t bg-app text-right rounded-b-lg">
                    <button onClick={onClose} className="btn-secondary px-4 py-2 rounded-md font-semibold transition-colors">Close</button>
                </div>
            </div>
        </div>
    );
};
