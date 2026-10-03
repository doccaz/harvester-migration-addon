import { useState, useEffect } from 'react';
import { HardDrive, AlertTriangle, Cpu, MemoryStick, Check, Network, Package, Loader, Search, XCircle, Upload } from 'lucide-react';
import { formatBytes } from '../utils';
import { CopyButton } from '../shared/CopyButton';

// --- VM Export (Harvester -> OVA) ---

// A disk contributes a virtual disk to the OVA only when it is a PVC-backed
// "disk" device. CD-ROMs become empty drives, containerDisks are image layers,
// and cloud-init volumes are excluded deliberately (they carry credentials).
export const isExportableDisk = (d) => d.kind === 'pvc' && d.device === 'disk';

export const EXPORT_PROFILES = [
    {
        key: 'vmware',
        label: 'VMware / vSphere',
        blurb: 'OVF 1.1 with VMware extensions. virtio devices are remapped to LSI Logic SCSI and E1000E, the hardware ESXi implements.',
        prep: true,
        warn: null,
    },
    {
        key: 'portable',
        label: 'Portable (OVF 1.0)',
        blurb: 'Strict OVF 1.0, no vendor extensions, E1000 NIC. Widest compatibility: VirtualBox, Proxmox, oVirt.',
        prep: true,
        warn: null,
    },
    {
        key: 'faithful',
        label: 'Faithful (KVM)',
        blurb: 'Preserves virtio devices verbatim and uses qcow2 disks for a lossless KVM/libvirt round-trip.',
        warn: 'Will NOT boot on ESXi/vSphere, and ovftool cannot read qcow2-based packages.',
    },
];

export const HarvesterVmPanel = ({ vm, onExport, isBusy }) => {
    const [profile, setProfile] = useState('vmware');
    const [preview, setPreview] = useState('');
    const [isPreviewing, setIsPreviewing] = useState(false);
    const [previewError, setPreviewError] = useState('');

    // Drop any stale preview when the selected VM or profile changes.
    useEffect(() => { setPreview(''); setPreviewError(''); }, [vm, profile]);

    const handlePreview = async () => {
        if (!vm) return;
        setIsPreviewing(true);
        setPreviewError('');
        try {
            const response = await fetch('/api/v1/exports/preview', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ namespace: vm.namespace, name: vm.name, profile, preserveMacs: true }),
            });
            const text = await response.text();
            if (!response.ok) {
                let msg = text;
                try { msg = JSON.parse(text).error || text; } catch (e) { /* not JSON */ }
                throw new Error(msg);
            }
            setPreview(text);
        } catch (err) {
            setPreviewError(err.message);
        } finally {
            setIsPreviewing(false);
        }
    };

    if (!vm) {
        return (
            <div className="flex flex-col items-center justify-center h-full text-secondary">
                <HardDrive size={48} className="opacity-30 mb-3" />
                <p className="text-sm">Select a virtual machine to see its export details.</p>
            </div>
        );
    }

    const blockers = vm.exportBlockers || [];
    const canExport = blockers.length === 0;
    const disks = vm.disks || [];
    const exportable = disks.filter(isExportableDisk);

    return (
        <div className="h-full overflow-y-auto pr-1">
            <div className="flex items-start justify-between mb-4">
                <div>
                    <h3 className="text-xl font-semibold text-main">{vm.name}</h3>
                    <p className="text-sm text-secondary">{vm.namespace}</p>
                </div>
                <span className={`px-2 py-1 rounded-full text-xs font-medium ${vm.powerState === 'poweredOn' ? 'bg-green-100 text-green-800' : 'bg-app text-main border border-main'}`}>
                    {vm.powerState === 'poweredOn' ? 'Running' : 'Stopped'}
                </span>
            </div>

            {blockers.length > 0 && (
                <div className="mb-4 p-3 rounded-md border border-amber-400 bg-amber-50">
                    <div className="flex items-center text-amber-800 font-medium text-sm mb-1">
                        <AlertTriangle size={16} className="mr-2" /> Cannot export right now
                    </div>
                    <ul className="list-disc list-inside text-sm text-amber-900">
                        {blockers.map((b, i) => <li key={i}>{b}</li>)}
                    </ul>
                </div>
            )}

            <div className="grid grid-cols-2 gap-3 mb-4">
                <div className="bg-app border border-main rounded-md p-3">
                    <div className="flex items-center text-secondary text-xs mb-1"><Cpu size={14} className="mr-1" /> vCPUs</div>
                    <div className="text-main font-semibold">{vm.cpu || '—'}</div>
                </div>
                <div className="bg-app border border-main rounded-md p-3">
                    <div className="flex items-center text-secondary text-xs mb-1"><MemoryStick size={14} className="mr-1" /> Memory</div>
                    <div className="text-main font-semibold">{vm.memoryMB ? `${(vm.memoryMB / 1024).toFixed(1)} GiB` : '—'}</div>
                </div>
                <div className="bg-app border border-main rounded-md p-3">
                    <div className="text-secondary text-xs mb-1">Firmware / Machine</div>
                    <div className="text-main font-semibold">{(vm.firmware || '—').toUpperCase()} · {vm.machineType || '—'}</div>
                </div>
                <div className="bg-app border border-main rounded-md p-3">
                    <div className="text-secondary text-xs mb-1">Architecture</div>
                    <div className="text-main font-semibold">{vm.architecture || '—'}</div>
                </div>
            </div>

            <h4 className="font-medium text-main mb-2 flex items-center"><HardDrive size={16} className="mr-2" /> Disks</h4>
            <div className="border border-main rounded-md overflow-hidden mb-4">
                <table className="w-full text-sm">
                    <thead className="bg-app">
                        <tr className="text-left text-secondary">
                            <th className="px-3 py-2 font-medium">Name</th>
                            <th className="px-3 py-2 font-medium">Device</th>
                            <th className="px-3 py-2 font-medium">Bus</th>
                            <th className="px-3 py-2 font-medium">Size</th>
                            <th className="px-3 py-2 font-medium">In OVA</th>
                        </tr>
                    </thead>
                    <tbody>
                        {disks.map((d, i) => (
                            <tr key={i} className="border-t border-main">
                                <td className="px-3 py-2 text-main">{d.name}</td>
                                <td className="px-3 py-2 text-secondary">{d.device || '—'}</td>
                                <td className="px-3 py-2 text-secondary">{d.busType || '—'}</td>
                                <td className="px-3 py-2 text-secondary">{d.capacity ? formatBytes(d.capacity) : '—'}</td>
                                <td className="px-3 py-2">
                                    {isExportableDisk(d)
                                        ? <span className="text-green-700 flex items-center"><Check size={14} className="mr-1" /> Yes</span>
                                        : <span className="text-secondary" title={d.kind === 'cloudinit' ? 'Excluded: cloud-init volumes carry credentials' : 'Not a PVC-backed disk'}>No</span>}
                                </td>
                            </tr>
                        ))}
                        {disks.length === 0 && <tr><td colSpan="5" className="px-3 py-3 text-secondary text-center">No disks</td></tr>}
                    </tbody>
                </table>
            </div>

            <h4 className="font-medium text-main mb-2 flex items-center"><Network size={16} className="mr-2" /> Network interfaces</h4>
            <div className="border border-main rounded-md overflow-hidden mb-4">
                <table className="w-full text-sm">
                    <thead className="bg-app">
                        <tr className="text-left text-secondary">
                            <th className="px-3 py-2 font-medium">Name</th>
                            <th className="px-3 py-2 font-medium">Model</th>
                            <th className="px-3 py-2 font-medium">MAC</th>
                        </tr>
                    </thead>
                    <tbody>
                        {(vm.networks || []).map((n, i) => (
                            <tr key={i} className="border-t border-main">
                                <td className="px-3 py-2 text-main">{n.name}</td>
                                <td className="px-3 py-2 text-secondary">{n.id || '—'}</td>
                                <td className="px-3 py-2 text-secondary font-mono text-xs">{n.mac || '—'}</td>
                            </tr>
                        ))}
                        {(vm.networks || []).length === 0 && <tr><td colSpan="3" className="px-3 py-3 text-secondary text-center">No interfaces</td></tr>}
                    </tbody>
                </table>
            </div>

            <h4 className="font-medium text-main mb-2 flex items-center"><Package size={16} className="mr-2" /> OVF target profile</h4>
            <div className="space-y-2 mb-4">
                {EXPORT_PROFILES.map(p => (
                    <label key={p.key} className={`flex items-start p-3 border rounded-md cursor-pointer ${profile === p.key ? 'border-blue-500 bg-app' : 'border-main'}`}>
                        <input
                            type="radio"
                            name="export-profile"
                            className="mt-1 mr-3"
                            checked={profile === p.key}
                            onChange={() => setProfile(p.key)}
                        />
                        <span className="flex-1">
                            <span className="block text-main font-medium text-sm">{p.label}</span>
                            <span className="block text-secondary text-xs mt-0.5">{p.blurb}</span>
                            {p.warn && (
                                <span className="block text-xs mt-1 text-red-600 font-medium flex items-center">
                                    <AlertTriangle size={12} className="mr-1 shrink-0" /> {p.warn}
                                </span>
                            )}
                        </span>
                    </label>
                ))}
            </div>

            {EXPORT_PROFILES.find(p => p.key === profile)?.prep && (
                <div className="mb-4 p-3 rounded-md border border-amber-400 bg-amber-50">
                    <div className="flex items-center text-amber-800 font-medium text-sm mb-1">
                        <AlertTriangle size={16} className="mr-2" /> Prepare the guest before exporting
                    </div>
                    <p className="text-sm text-amber-900">
                        This profile remaps virtio devices to hardware VMware implements. A guest installed on
                        Harvester usually has a <strong>virtio-only initramfs</strong> and will drop to an emergency
                        shell because it cannot see its own root disk. Run this <em>inside the VM</em>, then power it
                        off and export:
                    </p>
                    <pre className="mt-2 p-2 rounded bg-gray-900 text-white text-xs overflow-x-auto">dracut --regenerate-all --force --no-hostonly    # RHEL/SLES/Fedora
update-initramfs -u -k all                       # Debian/Ubuntu (MODULES=most)</pre>
                    <p className="text-xs text-amber-900 mt-1">
                        Windows guests need the LSI Logic / pvscsi storage driver installed and set to boot-start first.
                    </p>
                </div>
            )}

            <div className="mb-4">
                <button onClick={handlePreview} disabled={isPreviewing} className="btn-secondary px-3 py-2 rounded-md text-sm flex items-center">
                    {isPreviewing ? <Loader size={14} className="mr-2 animate-spin" /> : <Search size={14} className="mr-2" />}
                    Preview OVF descriptor
                </button>
                {previewError && (
                    <div className="mt-2 p-2 rounded-md border border-red-400 bg-red-50 text-red-800 text-xs flex items-start">
                        <XCircle size={14} className="mr-2 mt-0.5 shrink-0" /> {previewError}
                    </div>
                )}
                {preview && (
                    <div className="mt-2 p-2 border border-main rounded-md bg-gray-900 text-white font-mono text-xs max-h-80 overflow-auto relative group">
                        <CopyButton text={preview} className="absolute top-2 right-2 z-10" />
                        <pre className="whitespace-pre">{preview}</pre>
                    </div>
                )}
            </div>

            <button
                onClick={() => onExport && onExport(vm, profile)}
                disabled={!canExport || isBusy}
                title={canExport ? 'Export this VM to an OVA' : blockers.join('; ')}
                className="w-full px-4 py-2 rounded-md flex items-center justify-center font-medium bg-blue-600 text-white hover:bg-blue-700 disabled:opacity-50 disabled:bg-app disabled:text-secondary disabled:cursor-not-allowed transition-colors"
            >
                <Upload size={16} className="mr-2" />
                Export to OVA
                {canExport && exportable.length > 0 && ` (${exportable.length} disk${exportable.length > 1 ? 's' : ''})`}
            </button>
            <p className="text-xs text-secondary mt-2 text-center">
                Runs as a Kubernetes Job: the VM's disks are mounted read-only, converted with qemu-img, and packaged as an OVA.
            </p>
        </div>
    );
};
