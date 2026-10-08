import { ExternalLink, List, Server, Package, Upload, Info } from 'lucide-react';
import { SupportBundleCard } from '../support/SupportBundleCard';

export const AboutPage = () => (
    <div className="space-y-8">
        <div className="flex justify-between items-center mb-6">
            <h1 className="text-2xl font-bold text-main">
                About VM Import UI
            </h1>
        </div>

        <div role="note" className="p-4 rounded-lg border-l-4 border-yellow-500 bg-yellow-50 text-yellow-900 text-sm">
            <strong>Disclaimer:</strong> This is not an official SUSE product, and it is not supported by SUSE or the Harvester project. It is provided for exploration and evaluation only, as is and without warranty. Do not rely on it for production migrations.
        </div>

        {/* Feature Cards Section */}
        <div className="grid grid-cols-1 md:grid-cols-2 gap-8 mb-8">
            {/* Harvester Card */}
            <div className="bg-card text-main rounded-2xl p-8 flex flex-col items-center text-center shadow-xl border border-main">
                <div className="mb-6 h-16 flex items-center justify-center">
                    <img
                        src="https://harvesterhci.io/img/logo_horizontal.svg"
                        alt="Harvester"
                        className="h-full"
                    />
                </div>
                <h2 className="text-2xl font-bold mb-4">Harvester</h2>
                <p className="text-secondary leading-relaxed mb-8 flex-grow">
                    Harvester is a modern, open-source hyperconverged infrastructure (HCI) solution built on Kubernetes, KubeVirt, and Longhorn. It provides a familiar virtualization management interface on top of cloud-native technologies.
                </p>
                <a
                    href="https://harvesterhci.io/"
                    target="_blank"
                    rel="noopener noreferrer"
                    className="bg-[#00af7e] hover:bg-[#00966c] text-white font-bold py-3 px-8 rounded-lg flex items-center transition-colors shadow-lg"
                >
                    Get Harvester <ExternalLink size={18} className="ml-2" />
                </a>
            </div>

            {/* SUSE Virtualization Card */}
            <div className="bg-[#0c322c] text-white rounded-2xl p-8 flex flex-col items-center text-center shadow-xl border border-[#1e4a3f]">
                <div className="mb-6 h-12 flex items-center">
                    <img
                        src="https://d12w0ryu9hjsx8.cloudfront.net/shared-header/1.7/assets/SUSE_Logo.svg"
                        alt="SUSE"
                        className="h-full"
                    />
                </div>
                <h2 className="text-2xl font-bold mb-4 text-white">SUSE Virtualization</h2>
                <p className="text-[#00af7e] leading-relaxed mb-8 flex-grow font-medium">
                    Harvester is the foundation for <strong className="text-white">SUSE Virtualization</strong>, an enterprise-grade platform offering world-class support, enhanced security, and seamless Rancher integration for mission-critical workloads.
                </p>
                <a
                    href="https://www.suse.com/products/virtualization"
                    target="_blank"
                    rel="noopener noreferrer"
                    className="bg-white hover:bg-gray-100 text-[#0c322c] font-bold py-3 px-8 rounded-lg flex items-center transition-colors shadow-lg"
                >
                    Learn More <ExternalLink size={18} className="ml-2" />
                </a>
            </div>
        </div>

        <div className="bg-card shadow-md rounded-lg p-6 relative overflow-hidden border border-main">
            <a href="https://github.com/doccaz/vm-import-ui" target="_blank" rel="noopener noreferrer" aria-label="View source on GitHub" className="fixed top-0 right-0 z-50">
                <svg width="80" height="80" viewBox="0 0 250 250" style={{ fill: '#151513', color: '#fff', position: 'absolute', top: 0, border: 0, right: 0 }} aria-hidden="true">
                    <path d="M0,0 L115,115 L130,115 L142,142 L250,250 L250,0 Z"></path>
                    <path d="M128.3,109.0 C113.8,99.7 119.0,89.6 119.0,89.6 C122.0,82.7 120.5,78.6 120.5,78.6 C119.2,72.0 123.4,76.3 123.4,76.3 C127.3,80.9 125.5,87.3 125.5,87.3 C122.9,97.6 130.6,101.9 134.4,103.2" fill="currentColor" style={{ transformOrigin: '130px 106px' }} className="octo-arm"></path>
                    <path d="M115.0,115.0 C114.9,115.1 118.7,116.5 119.8,115.4 L133.7,101.6 C136.9,99.2 139.9,98.4 142.2,98.6 C133.8,88.0 127.5,74.4 143.8,58.0 C148.5,53.4 154.0,51.2 159.7,51.0 C160.3,49.4 163.2,43.6 171.4,40.1 C171.4,40.1 176.1,42.5 178.8,56.2 C183.1,58.6 187.2,61.8 190.9,65.4 C194.5,69.0 197.7,73.2 200.1,77.6 C213.8,80.2 216.3,84.9 216.3,84.9 C212.7,93.1 206.9,96.0 205.4,96.6 C205.1,102.4 203.0,107.8 198.3,112.5 C181.9,128.9 168.3,122.5 157.7,114.1 C157.9,116.9 156.7,120.9 152.7,124.9 L141.0,136.5 C139.8,137.7 141.6,141.9 141.8,141.8 Z" fill="currentColor" className="octo-body"></path>
                </svg>
            </a>
            <style>{`.github-corner:hover .octo-arm{animation:octocat-wave 560ms ease-in-out}@keyframes octocat-wave{0%,100%{transform:rotate(0)}20%,60%{transform:rotate(-25deg)}40%,80%{transform:rotate(10deg)}}@media (max-width:500px){.github-corner:hover .octo-arm{animation:none}.github-corner .octo-arm{animation:octocat-wave 560ms ease-in-out}}`}</style>

            <h2 className="text-xl font-semibold mb-4 z-10 relative text-main">Harvester VM Import UI</h2>
            <p className="mb-2 z-10 relative text-main"><strong>Version:</strong> 1.9.2</p>
            <p className="mb-2 z-10 relative text-secondary font-medium">This UI provides a user-friendly interface for migrating virtual machines into Harvester / SUSE Virtualization clusters. It supports two migration engines: the native VM Import Controller and the Forklift (Konveyor) project, with sources including VMware vCenter, standalone ESXi hosts, and OVA/OVF files on NFS shares.</p>
            <p className="mb-6 italic text-sm text-secondary z-10 relative mt-2 border-l-4 border-blue-400 pl-3">Based off of an idea by Erico Mendonca (erico.mendonca@suse.com)</p>

            <h3 className="text-lg font-semibold mb-3 z-10 relative text-main">How to Use</h3>
            <div className="space-y-4 z-10 relative text-secondary">
                <div>
                    <h4 className="font-semibold text-main flex items-center mb-1"><List size={16} className="mr-1 text-blue-600" /> Migration Plans Tab</h4>
                    <p className="text-sm">
                        Your primary dashboard for managing imports, with subtabs for <strong>VM Import Controller</strong> and <strong>Forklift</strong> plans. Click <strong>Create</strong> to launch the Migration Plan Wizard. The wizard will guide you through:
                    </p>
                    <ul className="list-disc list-inside text-sm mt-1 ml-2 space-y-1">
                        <li>Choosing the migration engine (VM Import Controller or Forklift).</li>
                        <li>Selecting a source (vCenter, ESXi, OVA for VMIC; vSphere or OVA provider for Forklift).</li>
                        <li>Browsing the inventory tree and selecting a VM to import.</li>
                        <li>Configuring target Namespace, Storage Class, and network mappings.</li>
                        <li>Forklift-specific options: warm migration (vSphere only), shared disks, populator labels, per-datastore/per-disk storage mappings.</li>
                    </ul>
                    <p className="text-sm mt-1">From the dashboard, you can track <strong>live progress with per-disk conditions</strong>, examine YAML configurations, view debug logs, and re-run migrations (replacing existing Migration CRs if needed).</p>
                </div>

                <div>
                    <h4 className="font-semibold text-main flex items-center mb-1"><Server size={16} className="mr-1 text-blue-600" /> vCenter Sources Tab</h4>
                    <p className="text-sm">
                        Register VMware sources here, with subtabs for <strong>VM Import Controller</strong> sources and <strong>Forklift vSphere providers</strong>.
                    </p>
                    <ul className="list-disc list-inside text-sm mt-1 ml-2 space-y-1">
                        <li><strong>VM Import Controller:</strong> Add vCenter sources with endpoint URL, datacenter name, and credentials.</li>
                        <li><strong>Forklift:</strong> Create Forklift vSphere providers (vCenter or standalone ESXi) with their associated Secrets.</li>
                    </ul>
                    <p className="text-sm mt-1">Use the <strong>Explore</strong> function to browse inventory, manage VM power states, rename VMs, and edit MAC addresses.</p>
                </div>

                <div>
                    <h4 className="font-semibold text-main flex items-center mb-1"><Package size={16} className="mr-1 text-blue-600" /> OVA Sources Tab</h4>
                    <p className="text-sm">
                        Manage OVA-based sources, with subtabs for <strong>VM Import Controller</strong> and <strong>Forklift OVA providers</strong>.
                    </p>
                    <ul className="list-disc list-inside text-sm mt-1 ml-2 space-y-1">
                        <li><strong>VM Import Controller:</strong> Define HTTP/HTTPS endpoints serving .ova files, with optional authentication.</li>
                        <li><strong>Forklift:</strong> Create OVA (NFS) providers pointing to NFS shares containing OVA/OVF files. Forklift auto-deploys an OVA server pod to scan and discover available VMs.</li>
                    </ul>
                </div>

                <div>
                    <h4 className="font-semibold text-main flex items-center mb-1"><Upload size={16} className="mr-1 text-blue-600" /> Export VMs Tab</h4>
                    <p className="text-sm">
                        Browse every Harvester virtual machine in the cluster, grouped by namespace, and export one to a
                        standards-conformant <strong>OVA</strong> (DMTF DSP0243) for use in vSphere, VirtualBox, Proxmox or plain KVM.
                    </p>
                    <ul className="list-disc list-inside text-sm mt-1 ml-2 space-y-1">
                        <li>Select a VM to see its vCPUs, memory, firmware, disks and network interfaces.</li>
                        <li>The <strong>In OVA</strong> column shows which disks are included. Only PVC-backed disks are exported &mdash; CD-ROMs become empty drives and cloud-init volumes are excluded because they carry credentials.</li>
                        <li>A VM must be <strong>powered off</strong> to be exported. Its disks are ReadWriteMany block volumes, so reading them while the VM runs would produce a torn, inconsistent image.</li>
                        <li><strong>Prepare the guest first</strong> for the VMware and Portable profiles: rebuild its initramfs without host-only mode (<code>dracut --regenerate-all --no-hostonly</code>), or it will not find its root disk after the virtio&rarr;LSI Logic remap. Verified against ESXi 8.0.3.</li>
                    </ul>
                </div>

                <div>
                    <h4 className="font-semibold text-main flex items-center mb-1"><Info size={16} className="mr-1 text-blue-600" /> About Tab</h4>
                    <p className="text-sm">
                        You are here! This tab provides application versioning, attribution, and documentation on how to navigate the utility.
                    </p>
                </div>
            </div>
        </div>

        <SupportBundleCard />
    </div>
);
