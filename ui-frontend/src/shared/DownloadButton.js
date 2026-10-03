import { Download } from 'lucide-react';

export const DownloadButton = ({ text, filename, label = "Download", className = "" }) => {
    const handleDownload = () => {
        const blob = new Blob([text], { type: 'text/plain' });
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = filename;
        a.click();
        URL.revokeObjectURL(url);
    };
    return (
        <button onClick={handleDownload} className={`inline-flex items-center gap-1.5 bg-blue-600 hover:bg-blue-500 text-white text-xs font-semibold px-3 py-1.5 rounded-md shadow-lg transition-colors ${className}`} title={`Download as ${filename}`}>
            <Download size={14} />
            {label && <span>{label}</span>}
        </button>
    );
};
