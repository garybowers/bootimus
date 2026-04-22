const fr = {
  meta: {
    title: 'Bootimus — Serveur de boot PXE/HTTP moderne',
    description:
      'Serveur de boot PXE et HTTP autonome. Un binaire. Zéro config. 50+ distros out-of-the-box.',
    downloadsTitle: 'Téléchargements — Bootimus',
    downloadsDescription:
      'Télécharge les binaires bootimus, les images Docker, l\'appliance USB et les outils PXE mirrorés.',
    docsTitle: 'Documentation — Bootimus',
    docsDescription:
      'Documentation Bootimus : déploiement, gestion des images, DHCP, authentification, ACL client, profils de distro.',
  },

  nav: {
    features: 'Fonctionnalités',
    howItWorks: 'Fonctionnement',
    downloads: 'Téléchargements',
    docs: 'Docs',
    github: 'GitHub',
    toggleTheme: 'Changer de thème',
    languageLabel: 'Langue',
    comingSoon: 'bientôt',
  },

  footer: {
    project: 'projet',
    licence: 'licence',
    licenceValue: 'Apache 2.0',
    lang: 'langage',
    repo: 'repo',
    docker: 'docker',
    docs: 'docs',
    copy: '© {year} contributeurs bootimus',
  },

  hero: {
    badgeStable: 'v1.x · apache 2.0',
    badgeStack: 'go · iPXE · sqlite/postgres',
    titleLine1: 'Boot PXE,',
    titleLine2: 'sans prise de tête',
    sub: 'Serveur de boot PXE et HTTP autonome. Un seul binaire. Zéro config. proxyDHCP intégré, tu ne touches jamais à ton routeur. 50+ distros détectées automatiquement.',
    ctaPrimary: '$ get bootimus',
    ctaSecondary: 'voir le code source',
    quickstartTitle: 'bootimus — démarrage rapide',
    statDistrosN: '50+',
    statDistrosL: 'distros détectées',
    statBinaryN: '1',
    statBinaryL: 'binaire, zéro dépendance',
    statReconfigsN: '0',
    statReconfigsL: 'DHCP à reconfigurer',
    statArchN: '2',
    statArchL: 'arch : amd64 · arm64',
  },

  features: {
    kicker: '// fonctionnalités',
    title: 'Tout ce qu\'un setup netboot moderne doit être.',
    sub: 'Pas un fork de scripts Perl vieux de 15 ans. Pas un wrapper autour de dnsmasq. Un vrai serveur, écrit en Go, batteries incluses.',
    items: {
      '01': {
        title: 'Un seul binaire',
        body: 'Binaire Go avec iPXE embarqué, UI web, SQLite et tous les assets. Aucune dépendance à l\'exécution. scp et go.',
      },
      '02': {
        title: 'proxyDHCP intégré',
        body: 'Répond au PXE sur UDP/67 sans toucher à ton DHCP existant. Zéro reconfiguration du routeur. Plug & play sur n\'importe quel LAN.',
      },
      '03': {
        title: '50+ distros',
        body: 'Extraction automatique du kernel/initrd pour Ubuntu, Debian, Arch, Fedora, NixOS, Alpine, FreeBSD, Windows (wimboot) et plus.',
      },
      '04': {
        title: 'ACL par MAC',
        body: 'Assigne des images spécifiques par MAC. Les nouveaux clients sont auto-détectés au premier PXE. Passe les leases en statique quand c\'est prêt.',
      },
      '05': {
        title: 'Outils en un clic',
        body: 'GParted, Clonezilla, Memtest86+, SystemRescue, ShredOS, netboot.xyz. Active-les depuis l\'UI, ils apparaissent dans le menu.',
      },
      '06': {
        title: 'JWT + LDAP',
        body: 'Auth par token avec bcrypt. Backend LDAP/AD optionnel avec admin par groupes. Les comptes locaux restent en fallback.',
      },
      '07': {
        title: 'API REST',
        body: 'Tout ce que fait l\'UI passe par un appel API. Assignations de boot scriptables, scans, WoL. Flux de logs live en SSE.',
      },
      '08': {
        title: 'Tourne partout',
        body: 'Docker multi-arch (amd64/arm64), binaire statique, ou une image appliance Alpine de 2 Go à flasher sur USB.',
      },
    },
  },

  howItWorks: {
    kicker: '// fonctionnement',
    title: 'Le cycle de vie d\'un boot PXE.',
    sub: 'Le client envoie un DHCPDISCOVER. Bootimus répond avec les détails PXE via proxyDHCP pendant que ton DHCP habituel distribue toujours l\'IP. iPXE charge via TFTP, chain vers HTTP, récupère le menu. L\'utilisateur choisit une image. Kernel et initrd streament depuis le serveur. Fin.',
    termTitle: 'pxe boot trace — ubuntu-24.04',
  },

  transparency: {
    kicker: '// transparence',
    title: '100% ouvert. Auditable de bout en bout.',
    sub: 'Pas de blobs propriétaires. Pas de télémétrie. Pas de firmware binaire planqué dans un vendor/. Tout le stack est sur GitHub sous Apache 2.0 — clone, audite, fork, auto-héberge.',
    items: {
      binary: {
        t: 'Un seul binaire Go',
        d: 'lié statiquement, ldd renvoie "not a dynamic executable". Builds reproductibles via make release.',
      },
      blobs: {
        t: 'Zéro blob propriétaire',
        d: 'l\'iPXE embarqué est du FOSS upstream (GPL-2.0). Aucun firmware closed-source embarqué.',
      },
      telemetry: {
        t: 'Aucune télémétrie, jamais',
        d: 'zéro call-home. Zéro analytics. Zéro "stats d\'usage anonymes". Compatible air-gap.',
      },
      licence: {
        t: 'Apache 2.0',
        d: 'licence permissive. Utilise-le en commercial, distribue-le en interne, forke-le sans contraintes.',
      },
      deps: {
        t: 'Dépendances vendored, toutes FOSS',
        d: 'chaque dépendance Go transitive est open source. go mod why sur n\'importe quel package.',
      },
      byo: {
        t: 'Apporte ton propre bootloader',
        d: 'pas confiance dans l\'iPXE embarqué ? Dépose tes binaires signés. Voir plus bas.',
      },
    },
    termTitle: 'bootimus version --verbose',
  },

  bootloaders: {
    kicker: '// bootloaders',
    title: 'Remplace iPXE par ce qu\'il te faut.',
    sub: 'Bootimus embarque iPXE pour chaque architecture courante. Besoin de binaires signés Microsoft pour Secure Boot, d\'un iPXE customisé, de GRUB, syslinux, ou de ton loader signé par ta CA interne ? Dépose un dossier dans data/bootloaders/, sélectionne-le dans l\'UI, c\'est fait. Les fichiers manquants basculent transparent sur le set embarqué — jamais un boot cassé.',
    cards: {
      uefi64: {
        t: 'iPXE · UEFI x86_64',
        d: 'ipxe.efi · la valeur par défaut. Buildé depuis upstream master, embarqué dans le binaire.',
        tag: 'embarqué · fallback',
      },
      uefiArm: {
        t: 'iPXE · UEFI ARM64',
        d: 'ipxe-arm64.efi · pour les Raspberry Pi 4/5, les hôtes Apple Silicon, les serveurs ARM.',
        tag: 'embarqué · fallback',
      },
      bios: {
        t: 'iPXE · BIOS legacy',
        d: 'undionly.kpxe · pour le vieux matos qui ne veut rien savoir de l\'UEFI. Toujours d\'actualité en 2026.',
        tag: 'embarqué · fallback',
      },
      shim: {
        t: 'Shim signé Microsoft',
        d: 'Dépose un shimx64.efi + grubx64.efi signés pour les flottes en Secure Boot obligatoire. Pas besoin d\'enrôlement MOK dans le firmware.',
        tag: 'custom · BYO',
      },
      themed: {
        t: 'iPXE customisé',
        d: 'Compile ton propre iPXE avec branding, couleurs de menu perso, scripts embarqués. Dépose le .efi.',
        tag: 'custom · BYO',
      },
      grub: {
        t: 'GRUB / syslinux / pxelinux',
        d: 'Pas d\'iPXE ? Pas de souci. Tout ce qui parle TFTP et HTTP fonctionne. Bootimus sert juste des octets.',
        tag: 'custom · BYO',
      },
    },
    termTitle: 'sets de bootloader — file fallthrough',
  },

  cta: {
    title: 'Marre de babysitter tftpd ?',
    sub: 'Docker, bare metal, ou USB flashable. Choisis ton poison.',
    primary: '$ get bootimus',
    secondary: 'lire la doc →',
  },

  downloads: {
    kicker: '// téléchargements',
    title: 'Chope un build.',
    lede:
      'Chaque release sort en image Docker, en binaires Linux statiques tirés en direct de la dernière release GitHub, et en image appliance USB flashable de 2 Gio (Alpine + bootimus, boot direct). Les outils sont mirrorés séparément pour que l\'UI admin puisse les récupérer sans dépendre des rate limits upstream.',
    badgeStable: 'latest · stable',
    badgePrerelease: 'pre-release',
    badgeNone: 'aucune release pour l\'instant',
    pillManifest: 'manifest.json',
    pillSource: 'github releases ↗',
    pillBuildSrc: 'builder depuis les sources',
    released: 'publié le',
    via: 'via',
    sectionArtifacts: 'artefacts de release',
    sectionTools: 'outils mirrorés',
    sectionApi: 'utiliser le manifest',
    emptyTitle: 'Aucun binaire publié pour l\'instant',
    emptyBody:
      'Pose un tag sur GitHub et les binaires de make release apparaîtront ici automatiquement.',
    buildFromSource: 'builder depuis les sources ↗',
    verifyTitle: 'vérifier',
    apiTitle: 'api · manifest.json',
    toolsLede:
      'Ce sont des images netboot-ready que l\'UI admin peut télécharger à la demande et exposer comme entrées du menu PXE. Mirrorées sur dl.bootimus.com pour que tu ne dépendes pas de la dispo upstream quand tu pars en mission de rescue. Les URL upstream restent la source de vérité — tu peux override n\'importe quelle URL mirror depuis la page Tools.',
    apiLede:
      'L\'UI admin lit /api/manifest.json pour vérifier les mises à jour et lister les outils disponibles. Schéma stable — les champs ne seront pas renommés sans un major bump. La source de vérité pour les binaires c\'est l\'API Releases de GitHub ; cet endpoint ne fait que normaliser et merger les bouts statiques (tag Docker, image appliance, outils mirrorés).',
    mirror: 'mirror ↓',
    upstream: 'upstream ↗',
    get: 'get ↓',
  },

  docs: {
    title: 'Documentation',
    subtitle: 'Tout ce qu\'il te faut pour déployer, configurer et opérer bootimus.',
    sectionsTitle: 'Sections',
    onThisPage: 'Sur cette page',
    prev: 'Précédent',
    next: 'Suivant',
    editOnGithub: 'Éditer sur GitHub',
    notFound: 'Document introuvable.',
    fallbackBanner: 'Cette page n\'est pas encore traduite. Affichage de la version anglaise.',
    translateCta: 'Aider à traduire →',
    pending: 'traduction en attente',
  },
};

export default fr;
