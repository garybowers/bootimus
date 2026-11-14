export interface StaticArtifact {
  kind: 'container' | 'appliance';
  platform: string;
  label: string;
  url: string;
  filename?: string;
  size?: string;
  notes?: string;
}

export interface Tool {
  id: string;
  name: string;
  summary: string;
  upstream: string;
  // Empty when no project mirror is available.
  mirror: string;
  version: string;
  size: string;
}

export const dockerImage: StaticArtifact = {
  kind: 'container',
  platform: 'docker',
  label: 'Docker image (multi-arch)',
  url: 'https://hub.docker.com/r/garybowers/bootimus',
  notes: 'amd64 + arm64 · pull garybowers/bootimus:latest',
};

export const tools: Tool[] = [
  {
    id: 'gparted',
    name: 'GParted Live',
    summary: 'Partition editor. Resize, move, copy, create, delete partitions on a running system.',
    upstream: 'https://gparted.org/download.php',
    mirror: '',
    version: '',
    size: '',
  },
  {
    id: 'clonezilla',
    name: 'Clonezilla Live',
    summary: 'Disk cloning and imaging. Bare-metal backup and restore over the network.',
    upstream: 'https://clonezilla.org/',
    mirror: '',
    version: '',
    size: '',
  },
  {
    id: 'memtest',
    name: 'Memtest86+',
    summary: 'RAM diagnostics. Pre-boot memory test, UEFI + BIOS.',
    upstream: 'https://memtest.org/',
    mirror: '',
    version: '',
    size: '',
  },
  {
    id: 'systemrescue',
    name: 'SystemRescue',
    summary: 'Full rescue toolkit. File recovery, disk repair, network diagnostics, partition tools.',
    upstream: 'https://www.system-rescue.org/',
    mirror: '',
    version: '',
    size: '',
  },
  {
    id: 'shredos',
    name: 'ShredOS',
    summary: 'Secure disk wiping. nwipe-based. DoD 5220.22-M, Gutmann, custom passes.',
    upstream: 'https://github.com/PartialVolume/shredos.x86_64',
    mirror: '',
    version: '',
    size: '',
  },
  {
    id: 'netbootxyz',
    name: 'netboot.xyz',
    summary: 'Chainloads into hundreds of OS installers, live distros, and utilities.',
    upstream: 'https://netboot.xyz/',
    mirror: '',
    version: '',
    size: '',
  },
];
