import { useState, useEffect } from 'react';
import { Search, CheckCircle, X, Edit, Cpu, MemoryStick, HardDrive, Folder, Network, Check, Play, Power, Square, RotateCcw, Loader } from 'lucide-react';
import { formatBytes } from '../utils';

export const VmDetailsPanel = ({ vm, onPowerOp, onRename, isOperating, onMacUpdate }) => {
    const [isRenaming, setIsRenaming] = useState(false);
    const [newName, setNewName] = useState('');
    const [editingMacKey, setEditingMacKey] = useState(null);
    const [newMac, setNewMac] = useState('');
    const [isUpdatingMac, setIsUpdatingMac] = useState(false);

    useEffect(() => {
        if (vm) {
            setNewName(vm.name);
            setIsRenaming(false);
        }
    }, [vm]);

    if (!vm) {
        return (
            <div className="border border-main rounded-md p-6 bg-app flex flex-col items-center justify-center text-secondary opacity-70 h-96">
                <Search size={48} className="mb-4 opacity-20" />
                <p className="text-sm font-medium">Select an item from the tree to view details</p>
            </div>
        );
    }

    const handleRenameSubmit = () => {
        if (newName && newName !== vm.name) {
            onRename(vm.name, newName);
        }
        setIsRenaming(false);
    };

    const isVm = vm.type === 'VirtualMachine';
    const isDisk = vm.type === 'disk';

    const getPowerStateColor = (state) => {
        switch (state) {
            case 'poweredOn': return 'text-green-600';
            case 'poweredOff': return 'text-red-600';
            case 'suspended': return 'text-yellow-600';
            default: return 'text-secondary';
        }
    };

    return (
        <div className="p-4 border rounded-md bg-app h-full flex flex-col">
            <div className="flex justify-between items-center mb-4">
                {isRenaming ? (
                    <div className="flex items-center space-x-2 w-full">
                        <input
                            type="text"
                            value={newName}
                            onChange={(e) => setNewName(e.target.value)}
                            className="form-input text-lg font-medium flex-grow"
                            autoFocus
                        />
                        <button onClick={handleRenameSubmit} className="text-green-600 hover:text-green-800"><CheckCircle size={20} /></button>
                        <button onClick={() => setIsRenaming(false)} className="text-red-600 hover:text-red-800"><X size={20} /></button>
                    </div>
                ) : (
                    <div className="flex items-center justify-between w-full">
                        <h3 className="text-lg font-medium text-main truncate" title={vm.name}>{vm.name}</h3>
                        <button onClick={() => setIsRenaming(true)} className="ml-2 text-secondary opacity-70 hover:text-blue-600 transition-colors" title="Rename VM">
                            <Edit size={16} />
                        </button>
                    </div>
                )}
            </div>

            <div className="space-y-3 text-sm flex-grow">
                {isVm && (
                    <>
                        <div className="flex items-center justify-between p-2 bg-card rounded border">
                            <span className="text-secondary font-medium">Power State:</span>
                            <span className={`font-bold ${getPowerStateColor(vm.powerState)}`}>{vm.powerState}</span>
                        </div>

                        <div className="flex items-center">
                            <Cpu size={16} className="mr-2 text-secondary" />
                            <span>{vm.cpu || '0'} vCPU(s)</span>
                        </div>
                        <div className="flex items-center">
                            <MemoryStick size={16} className="mr-2 text-secondary" />
                            <span>{formatBytes((vm.memoryMB || 0) * 1024 * 1024, 0)} Memory</span>
                        </div>
                        <div className="flex items-center">
                            <HardDrive size={16} className="mr-2 text-secondary" />
                            <span>{vm.diskSizeGB || '0'} GB Storage (Committed)</span>
                        </div>
                        <div className="flex items-center">
                            <Folder size={16} className="mr-2 text-secondary" />
                            <span className="truncate" title={vm.folder || '/'}>{vm.folder || '/'}</span>
                        </div>
                        <div>
                            <h4 className="font-medium text-main mt-4 mb-1 border-b pb-1">Networks</h4>
                            <div className="space-y-2 mt-2">
                                {(vm.networks || []).map((net, i) => (
                                    <div key={i} className="flex flex-col p-2 bg-card rounded border shadow-sm">
                                        <div className="flex justify-between items-center">
                                            <div className="flex items-center space-x-2 truncate">
                                                <Network size={14} className="text-blue-500 shrink-0" />
                                                <span className="text-main truncate font-medium">{net.name || net}</span>
                                            </div>
                                            <div className="flex items-center space-x-2">
                                                {editingMacKey === net.key ? (
                                                    <div className="flex items-center space-x-1">
                                                        <input
                                                            type="text"
                                                            value={newMac}
                                                            onChange={(e) => setNewMac(e.target.value)}
                                                            className="text-[10px] font-mono border rounded px-1 w-32 py-0.5"
                                                            autoFocus
                                                        />
                                                        <button
                                                            onClick={() => {
                                                                setIsUpdatingMac(true);
                                                                onMacUpdate(vm.name, net.key, newMac)
                                                                    .then(() => setEditingMacKey(null))
                                                                    .catch(err => alert(err.message))
                                                                    .finally(() => setIsUpdatingMac(false));
                                                            }}
                                                            disabled={isUpdatingMac}
                                                            className="text-green-600 hover:text-green-700"
                                                        >
                                                            <Check size={14} />
                                                        </button>
                                                        <button
                                                            onClick={() => setEditingMacKey(null)}
                                                            className="text-red-600 hover:text-red-700"
                                                        >
                                                            <X size={14} />
                                                        </button>
                                                    </div>
                                                ) : (
                                                    <div className="flex items-center space-x-1 group/mac">
                                                        <span className="text-[10px] font-mono text-secondary">{net.mac || 'No MAC'}</span>
                                                        {net.mac && (
                                                            <button
                                                                onClick={() => {
                                                                    setEditingMacKey(net.key);
                                                                    setNewMac(net.mac);
                                                                }}
                                                                className="text-secondary opacity-70 hover:text-blue-600 p-0.5 opacity-0 group-hover/mac:opacity-100 transition-opacity"
                                                                title="Edit MAC Address"
                                                            >
                                                                <Edit size={12} />
                                                            </button>
                                                        )}
                                                    </div>
                                                )}
                                            </div>
                                        </div>
                                    </div>
                                ))}
                                {(!vm.networks || vm.networks.length === 0) && <p className="text-xs text-secondary opacity-70 italic">No network interfaces.</p>}
                            </div>
                        </div>
                        <div>
                            <h4 className="font-medium text-main mt-4 mb-1 border-b pb-1">Individual Disks</h4>
                            <div className="space-y-2 mt-2">
                                {(vm.disks || []).map((disk, i) => (
                                    <div key={i} className="flex flex-col p-2 bg-card rounded border shadow-sm">
                                        <div className="flex justify-between items-center bg-app -m-2 mb-2 px-2 py-1 rounded-t border-b overflow-hidden">
                                            <span className="font-bold text-main truncate text-[10px]">{disk.name}</span>
                                            <span className="text-[10px] font-mono text-blue-600">{disk.busType}</span>
                                        </div>
                                        <div className="flex justify-between items-center text-xs">
                                            <span className="text-secondary">{formatBytes(disk.capacity)}</span>
                                            <span className="text-secondary opacity-70">Unit: {disk.unitNum}</span>
                                        </div>
                                    </div>
                                ))}
                                {(!vm.disks || vm.disks.length === 0) && <p className="text-xs text-secondary opacity-70 italic">No detailed disk info available.</p>}
                            </div>
                        </div>
                    </>
                )}
                {isDisk && (
                    <div className="p-4 bg-card rounded-lg border shadow-sm space-y-4">
                        <div className="flex items-center space-x-3 text-blue-600">
                            <HardDrive size={24} />
                            <h4 className="text-lg font-semibold">Disk Selection</h4>
                        </div>
                        <div className="space-y-2">
                            <div className="flex justify-between text-sm">
                                <span className="text-secondary font-medium">Node:</span>
                                <span className="text-main font-bold">{vm.name}</span>
                            </div>
                            <p className="text-xs text-secondary italic">This is a virtual disk component of the parent VM. Select the VM node itself to perform power operations.</p>
                        </div>
                    </div>
                )}
            </div>

            <div className="mt-6 pt-4 border-t">
                <h4 className="text-sm font-semibold text-main mb-3 uppercase tracking-wider">VM Operations</h4>
                <div className="grid grid-cols-2 gap-2">
                    <button
                        onClick={() => onPowerOp('on')}
                        disabled={isOperating || vm.powerState === 'poweredOn'}
                        className="flex items-center justify-center px-3 py-2 bg-card border border-green-200 text-green-700 rounded-md hover:bg-green-50 disabled:opacity-50 disabled:bg-app transition-colors"
                        title="Power On"
                    >
                        <Play size={16} className="mr-2" /> Power On
                    </button>
                    <button
                        onClick={() => onPowerOp('shutdown')}
                        disabled={isOperating || vm.powerState === 'poweredOff'}
                        className="flex items-center justify-center px-3 py-2 bg-card border border-red-200 text-red-700 rounded-md hover:bg-red-50 disabled:opacity-50 disabled:bg-app transition-colors"
                        title="Guest Shutdown"
                    >
                        <Power size={16} className="mr-2" /> Shutdown
                    </button>
                    <button
                        onClick={() => onPowerOp('off')}
                        disabled={isOperating || vm.powerState === 'poweredOff'}
                        className="flex items-center justify-center px-3 py-2 bg-card border border-red-200 text-red-700 rounded-md hover:bg-red-50 disabled:opacity-50 disabled:bg-app transition-colors"
                        title="Power Off (Immediate)"
                    >
                        <Square size={16} className="mr-2 text-red-600" /> Power Off
                    </button>
                    <button
                        onClick={() => onPowerOp('reset')}
                        disabled={isOperating || vm.powerState === 'poweredOff'}
                        className="flex items-center justify-center px-3 py-2 bg-card border border-yellow-200 text-yellow-700 rounded-md hover:bg-yellow-50 disabled:opacity-50 disabled:bg-app transition-colors"
                        title="Reset"
                    >
                        <RotateCcw size={16} className="mr-2" /> Reset
                    </button>
                </div>
                {isOperating && (
                    <div className="mt-3 flex items-center justify-center text-xs text-blue-600 animate-pulse">
                        <Loader size={12} className="animate-spin mr-1" /> Executing operation...
                    </div>
                )}
            </div>
        </div>
    );
};
