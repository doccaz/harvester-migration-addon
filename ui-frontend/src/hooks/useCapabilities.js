import { useState, useEffect } from 'react';

export function useCapabilities() {
    // NEW: Capability State
    const [capabilities, setCapabilities] = useState({ harvesterVersion: '', hasAdvancedPower: false });

    // NEW: Fetch Capabilities on Mount
    useEffect(() => {
        fetch('/api/v1/capabilities')
            .then(res => res.json())
            .then(data => {
                console.log("Cluster Capabilities:", data);
                setCapabilities(data);
            })
            .catch(err => console.error("Failed to fetch capabilities:", err));
    }, []);
    return capabilities;
}
