import { Plus } from 'lucide-react';

// --- Components ---
export const Header = ({ title, onButtonClick }) => (
    <div className="flex justify-between items-center mb-6 pb-4 border-b border-main">
        <h1 className="text-2xl font-semibold text-main">{title}</h1>
        {onButtonClick && (
            <button onClick={onButtonClick} className="flex items-center bg-blue-500 hover:bg-blue-600 text-white font-semibold py-2 px-4 rounded-md shadow">
                <Plus size={20} className="mr-2" />
                Create
            </button>
        )}
    </div>
);
