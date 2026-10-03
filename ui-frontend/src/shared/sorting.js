import { getNestedValue } from './getNestedValue';

export const handleSort = (setter) => (key) => {
    setter(prev => ({
        key,
        direction: prev.key === key && prev.direction === 'asc' ? 'desc' : 'asc'
    }));
};

export const sortData = (data, sortConfig) => {
    if (!sortConfig.key) return data;
    return [...data].sort((a, b) => {
        const aVal = getNestedValue(a, sortConfig.key);
        const bVal = getNestedValue(b, sortConfig.key);
        if (aVal < bVal) return sortConfig.direction === 'asc' ? -1 : 1;
        if (aVal > bVal) return sortConfig.direction === 'asc' ? 1 : -1;
        return 0;
    });
};
