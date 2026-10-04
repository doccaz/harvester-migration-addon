import { ExternalLink } from 'lucide-react';
import { harvesterResourceUrl, MIGRATION } from './harvesterLinks';

// A link to Harvester's own page for an object. It renders nothing when this UI is not
// served through the dashboard's proxy, so a page looks exactly as before without one.
export const ViewInHarvester = ({ resource, object, kind = MIGRATION }) => {
    const url = harvesterResourceUrl(window.location, resource, object?.metadata?.namespace, object?.metadata?.name, kind);
    if (!url) return null;
    return (
        <p>
            <a href={url} target="_blank" rel="noopener noreferrer" className="inline-flex items-center text-blue-600 hover:text-blue-800 hover:underline">
                <ExternalLink size={14} className="mr-1" /> View in Harvester{kind.yaml ? ' (YAML)' : ''}
            </a>
        </p>
    );
};
