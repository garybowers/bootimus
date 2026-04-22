export interface DocLink {
  slug: string;
  title: string;
  description?: string;
}

export interface DocSection {
  title: string;
  items: DocLink[];
}

export const docsNav: DocSection[] = [
  {
    title: 'Getting started',
    items: [
      { slug: 'deployment', title: 'Deployment', description: 'Docker, binary, networking, and storage.' },
      { slug: 'appliance',  title: 'USB Appliance', description: 'Flashable Alpine + bootimus image for portable PXE servers.' },
    ],
  },
  {
    title: 'Configuration',
    items: [
      { slug: 'dhcp',           title: 'DHCP',           description: 'proxyDHCP and external DHCP server config (ISC, Dnsmasq, MikroTik, etc.).' },
      { slug: 'authentication', title: 'Authentication', description: 'JWT, LDAP/Active Directory, group-based admin.' },
    ],
  },
  {
    title: 'Operations',
    items: [
      { slug: 'admin',           title: 'Admin Console',   description: 'Web UI tour and REST API reference.' },
      { slug: 'images',          title: 'Image Management', description: 'Upload ISOs, extract kernels, netboot support.' },
      { slug: 'clients',         title: 'Client Management', description: 'MAC ACLs, auto-discovery, next boot, WoL.' },
      { slug: 'distro-profiles', title: 'Distro Profiles',  description: 'Data-driven distro detection and boot params.' },
    ],
  },
];

export function flatNav(): DocLink[] {
  return docsNav.flatMap((s) => s.items);
}

export function findNeighbours(slug: string): { prev?: DocLink; next?: DocLink } {
  const flat = flatNav();
  const i = flat.findIndex((l) => l.slug === slug);
  if (i < 0) return {};
  return {
    prev: i > 0 ? flat[i - 1] : undefined,
    next: i < flat.length - 1 ? flat[i + 1] : undefined,
  };
}
