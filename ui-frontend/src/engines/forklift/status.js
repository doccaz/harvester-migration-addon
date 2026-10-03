// --- Forklift Components ---

export const getForkliftConditionStatus = (conditions, type) => {
    if (!conditions || !Array.isArray(conditions)) return null;
    return conditions.find(c => c.type === type);
};

export const ForkliftStatusBadge = ({ conditions }) => {
    const readyCond = getForkliftConditionStatus(conditions, 'Ready');
    if (!readyCond) return <span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-app text-main">Pending</span>;
    if (readyCond.status === 'True') return <span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-green-100 text-green-800">Ready</span>;
    return <span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-red-100 text-red-800" title={readyCond.message}>{readyCond.reason || 'Not Ready'}</span>;
};

export const getForkliftPlanStatus = (plan) => {
    const conditions = plan.status?.conditions || [];
    const readyCond = conditions.find(c => c.type === 'Ready');
    const executingCond = conditions.find(c => c.type === 'Executing');
    const succeededCond = conditions.find(c => c.type === 'Succeeded');
    const failedCond = conditions.find(c => c.type === 'Failed');

    if (failedCond && failedCond.status === 'True') return 'Failed';
    if (succeededCond && succeededCond.status === 'True') return 'Succeeded';
    if (executingCond && executingCond.status === 'True') return 'Executing';
    if (readyCond && readyCond.status === 'True') return 'Ready';
    if (readyCond && readyCond.status === 'False') return 'Not Ready';
    return 'Pending';
};
