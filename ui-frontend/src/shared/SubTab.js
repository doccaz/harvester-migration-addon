export const SubTab = ({ tabs, activeTab, onTabChange }) => (
    <div className="flex space-x-1 mb-4 border-b border-main">
        {tabs.map(tab => (
            <button
                key={tab.key}
                onClick={() => onTabChange(tab.key)}
                className={`px-4 py-2 text-sm font-medium transition-colors rounded-t-md ${activeTab === tab.key
                    ? 'bg-card border border-main border-b-0 text-blue-600 -mb-px'
                    : 'text-secondary hover:text-main hover:bg-app'
                    }`}
            >
                {tab.label}
            </button>
        ))}
    </div>
);
