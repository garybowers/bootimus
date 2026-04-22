const es = {
  meta: {
    title: 'Bootimus — Servidor de arranque PXE/HTTP moderno',
    description:
      'Servidor de arranque PXE y HTTP autocontenido. Un solo binario. Cero configuración. 50+ distros listas para usar.',
    downloadsTitle: 'Descargas — Bootimus',
    downloadsDescription:
      'Descarga los binarios de bootimus, las imágenes Docker, la appliance USB y las herramientas PXE mirrorizadas.',
    docsTitle: 'Documentación — Bootimus',
    docsDescription:
      'Documentación de Bootimus: despliegue, gestión de imágenes, DHCP, autenticación, ACLs de cliente, perfiles de distro.',
  },

  nav: {
    features: 'Características',
    howItWorks: 'Cómo funciona',
    downloads: 'Descargas',
    docs: 'Docs',
    github: 'GitHub',
    toggleTheme: 'Cambiar tema',
    languageLabel: 'Idioma',
    comingSoon: 'próximamente',
  },

  footer: {
    project: 'proyecto',
    licence: 'licencia',
    licenceValue: 'Apache 2.0',
    lang: 'lenguaje',
    repo: 'repo',
    docker: 'docker',
    docs: 'docs',
    copy: '© {year} contribuidores de bootimus',
  },

  hero: {
    badgeStable: 'v1.x · apache 2.0',
    badgeStack: 'go · iPXE · sqlite/postgres',
    titleLine1: 'Arranque PXE,',
    titleLine2: 'sin tanto lío',
    sub: 'Servidor de arranque PXE y HTTP autocontenido. Un solo binario. Cero configuración. proxyDHCP integrado: no vuelves a tocar tu router. 50+ distros detectadas automáticamente.',
    ctaPrimary: '$ get bootimus',
    ctaSecondary: 'ver el código',
    quickstartTitle: 'bootimus — inicio rápido',
    statDistrosN: '50+',
    statDistrosL: 'distros detectadas',
    statBinaryN: '1',
    statBinaryL: 'binario, cero dependencias',
    statReconfigsN: '0',
    statReconfigsL: 'reconfiguraciones DHCP',
    statArchN: '2',
    statArchL: 'arch: amd64 · arm64',
  },

  features: {
    kicker: '// características',
    title: 'Todo lo que un setup netboot moderno debería ser.',
    sub: 'No es un fork de scripts Perl de hace 15 años. No es un wrapper sobre dnsmasq. Un servidor de verdad, escrito en Go, con todo incluido.',
    items: {
      '01': {
        title: 'Un solo binario',
        body: 'Binario Go con iPXE embebido, UI web, SQLite y todos los assets. Sin dependencias en runtime. scp y listo.',
      },
      '02': {
        title: 'proxyDHCP integrado',
        body: 'Responde PXE en UDP/67 sin tocar tu DHCP actual. Cero reconfiguración del router. Enchufa y va en cualquier LAN.',
      },
      '03': {
        title: '50+ distros',
        body: 'Extracción automática de kernel/initrd para Ubuntu, Debian, Arch, Fedora, NixOS, Alpine, FreeBSD, Windows (wimboot) y más.',
      },
      '04': {
        title: 'ACL por MAC',
        body: 'Asigna imágenes específicas por MAC. Los clientes nuevos se detectan solos en el primer PXE. Asciende leases a estáticos cuando quieras.',
      },
      '05': {
        title: 'Tools en un clic',
        body: 'GParted, Clonezilla, Memtest86+, SystemRescue, ShredOS, netboot.xyz. Actívalos desde la UI y aparecen en el menú.',
      },
      '06': {
        title: 'JWT + LDAP',
        body: 'Auth por token con bcrypt. Backend LDAP/AD opcional con admin basado en grupos. Las cuentas locales siguen como fallback.',
      },
      '07': {
        title: 'API REST',
        body: 'Todo lo que hace la UI es una llamada a la API. Asignaciones de arranque scripteables, scans, triggers WoL. Stream de logs en vivo por SSE.',
      },
      '08': {
        title: 'Corre en todos lados',
        body: 'Docker multi-arch (amd64/arm64), binario estático, o una imagen appliance basada en Alpine de 2 GB para flashear a USB.',
      },
    },
  },

  howItWorks: {
    kicker: '// cómo funciona',
    title: 'El ciclo de vida de un arranque PXE.',
    sub: 'El cliente manda un DHCPDISCOVER. Bootimus responde los detalles PXE vía proxyDHCP mientras tu DHCP normal sigue repartiendo IPs. iPXE carga por TFTP, encadena a HTTP, trae el menú. El usuario elige una imagen. Kernel e initrd stream desde el servidor. Listo.',
    termTitle: 'pxe boot trace — ubuntu-24.04',
  },

  transparency: {
    kicker: '// transparencia',
    title: '100% abierto. Auditable de punta a punta.',
    sub: 'Cero blobs propietarios. Cero telemetría. Cero firmware binario escondido en un vendor. El stack entero está en GitHub bajo Apache 2.0 — clona, audita, forkea, monta el tuyo.',
    items: {
      binary: {
        t: 'Un solo binario Go',
        d: 'enlazado estáticamente, ldd dice "not a dynamic executable". Builds reproducibles desde make release.',
      },
      blobs: {
        t: 'Cero blobs propietarios',
        d: 'el iPXE embebido es FOSS upstream (GPL-2.0). Ningún firmware closed-source.',
      },
      telemetry: {
        t: 'Cero telemetría, nunca',
        d: 'cero call-home. Cero analytics. Cero "estadísticas anónimas de uso". Apto para redes air-gapped.',
      },
      licence: {
        t: 'Apache 2.0',
        d: 'licencia permisiva. Úsalo en entornos comerciales, distribúyelo internamente, forkéalo sin ataduras.',
      },
      deps: {
        t: 'Deps vendoreadas, todas FOSS',
        d: 'cada dependencia Go transitiva es open source. go mod why sobre cualquier paquete.',
      },
      byo: {
        t: 'Trae tu propio bootloader',
        d: '¿no te fías del iPXE embebido? Mete tus propios binarios firmados. Mira abajo.',
      },
    },
    termTitle: 'bootimus version --verbose',
  },

  bootloaders: {
    kicker: '// bootloaders',
    title: 'Cambia iPXE por lo que necesites.',
    sub: 'Bootimus trae iPXE embebido para cada arquitectura común. ¿Necesitas binarios firmados por Microsoft para Secure Boot, un iPXE con branding propio, GRUB, syslinux, o tu loader firmado por tu CA interna? Mete una carpeta en data/bootloaders/, selecciónala en la UI, listo. Los archivos que falten vuelven transparente al set embebido — nunca un arranque roto.',
    cards: {
      uefi64: {
        t: 'iPXE · UEFI x86_64',
        d: 'ipxe.efi · el default. Compilado desde upstream master, embebido en el binario.',
        tag: 'embebido · fallback',
      },
      uefiArm: {
        t: 'iPXE · UEFI ARM64',
        d: 'ipxe-arm64.efi · para Raspberry Pi 4/5, hosts Apple Silicon, servidores ARM.',
        tag: 'embebido · fallback',
      },
      bios: {
        t: 'iPXE · BIOS legacy',
        d: 'undionly.kpxe · para cacharros viejos que no quieren saber nada de UEFI. En 2026 todavía relevante.',
        tag: 'embebido · fallback',
      },
      shim: {
        t: 'Shim firmado por Microsoft',
        d: 'Mete un shimx64.efi + grubx64.efi firmados para flotas con Secure Boot obligatorio. No hace falta enrolar MOK en el firmware.',
        tag: 'custom · BYO',
      },
      themed: {
        t: 'iPXE con branding propio',
        d: 'Compila tu propio iPXE con branding, colores de menú a medida, scripts embebidos. Suelta el .efi y ya.',
        tag: 'custom · BYO',
      },
      grub: {
        t: 'GRUB / syslinux / pxelinux',
        d: '¿No quieres iPXE? No problem. Cualquier cosa que hable TFTP y HTTP funciona. Bootimus solo sirve bytes.',
        tag: 'custom · BYO',
      },
    },
    termTitle: 'sets de bootloader — file fallthrough',
  },

  cta: {
    title: '¿Harto de hacer de niñera de tftpd?',
    sub: 'Docker, bare metal o USB flasheable. Elige tu veneno.',
    primary: '$ get bootimus',
    secondary: 'leer la docu →',
  },

  downloads: {
    kicker: '// descargas',
    title: 'Pilla un build.',
    lede:
      'Cada release sale como imagen Docker, como binarios Linux estáticos tirados en vivo del último release de GitHub, y como una imagen appliance USB flasheable de 2 GiB (Alpine + bootimus, arranca directo). Las herramientas se mirrorizan por separado para que la UI admin las baje sin chocar contra los rate limits de upstream.',
    badgeStable: 'latest · estable',
    badgePrerelease: 'pre-release',
    badgeNone: 'aún sin releases',
    pillManifest: 'manifest.json',
    pillSource: 'github releases ↗',
    pillBuildSrc: 'compilar desde fuentes',
    released: 'publicado',
    via: 'vía',
    sectionArtifacts: 'artefactos del release',
    sectionTools: 'tools mirrorizadas',
    sectionApi: 'consumir el manifest',
    emptyTitle: 'Aún no hay binarios publicados',
    emptyBody:
      'Lanza un tag en GitHub y los binarios de make release aparecerán aquí automáticamente.',
    buildFromSource: 'compilar desde fuentes ↗',
    verifyTitle: 'verificar',
    apiTitle: 'api · manifest.json',
    toolsLede:
      'Estas son imágenes netboot-ready que la UI admin puede bajar a demanda y exponer como entradas del menú PXE. Mirrorizadas en dl.bootimus.com para que no dependas de la disponibilidad de upstream cuando arrancas un trabajo de rescate. Las URLs upstream siguen siendo la fuente de verdad — puedes sobrescribir cualquier URL mirror desde la página de Tools.',
    apiLede:
      'La UI admin lee /api/manifest.json para chequear updates y listar las tools disponibles. Schema estable — los campos no se renombran sin un bump mayor. La fuente de verdad para los binarios es la API de GitHub Releases; este endpoint solo normaliza y merges los pedacitos estáticos (tag Docker, imagen appliance, tools mirrorizadas).',
    mirror: 'mirror ↓',
    upstream: 'upstream ↗',
    get: 'bajar ↓',
  },

  docs: {
    title: 'Documentación',
    subtitle: 'Todo lo que necesitas para desplegar, configurar y operar bootimus.',
    sectionsTitle: 'Secciones',
    onThisPage: 'En esta página',
    prev: 'Anterior',
    next: 'Siguiente',
    editOnGithub: 'Editar en GitHub',
    notFound: 'Documento no encontrado.',
    fallbackBanner: 'Esta página aún no está traducida. Mostrando la versión en inglés.',
    translateCta: 'Ayuda a traducir →',
    pending: 'traducción pendiente',
  },
};

export default es;
