import { useState } from 'react';
import { Check, Copy } from 'lucide-react';

// --- Copy to Clipboard Button ---
export const CopyButton = ({ text, label = "Copy", className = "" }) => {
    const [copied, setCopied] = useState(false);
    const handleCopy = () => {
        navigator.clipboard.writeText(text);
        setCopied(true);
        setTimeout(() => setCopied(false), 2000);
    };
    return (
        <button onClick={handleCopy} className={`inline-flex items-center gap-1.5 bg-blue-600 hover:bg-blue-500 text-white text-xs font-semibold px-3 py-1.5 rounded-md shadow-lg transition-colors ${className}`} title="Copy to clipboard">
            {copied ? <Check size={14} className="text-green-300" /> : <Copy size={14} />}
            {label && <span>{copied ? "Copied!" : label}</span>}
        </button>
    );
};
