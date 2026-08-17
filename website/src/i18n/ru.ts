const ru = {
  meta: {
    title: 'Bootimus — современный PXE/HTTP-сервер загрузки',
    description:
      'Автономный PXE- и HTTP-сервер загрузки. Один бинарник. Ноль настроек. 50+ дистрибутивов из коробки.',
    downloadsTitle: 'Загрузки — Bootimus',
    downloadsDescription:
      'Скачайте бинарники bootimus, Docker-образы, USB-аппарат и зеркала PXE-инструментов.',
    docsTitle: 'Документация — Bootimus',
    docsDescription:
      'Документация Bootimus: развёртывание, управление образами, DHCP, аутентификация, клиентские ACL, профили дистрибутивов.',
  },

  nav: {
    features: 'Возможности',
    howItWorks: 'Как это работает',
    downloads: 'Загрузки',
    docs: 'Документация',
    github: 'GitHub',
    toggleTheme: 'Сменить тему',
    languageLabel: 'Язык',
    comingSoon: 'скоро',
    madeInBritain: 'made in britain',
  },

  footer: {
    project: 'проект',
    licence: 'лицензия',
    licenceValue: 'Apache 2.0',
    lang: 'язык',
    repo: 'репозиторий',
    docker: 'docker',
    docs: 'документация',
    copy: '© {year} участники bootimus',
  },

  hero: {
    badgeStable: 'v1.x · apache 2.0',
    badgeStack: 'go · iPXE · sqlite/postgres',
    titleLine1: 'PXE-загрузка,',
    titleLine2: 'без боли',
    sub: 'Автономный PXE- и HTTP-сервер загрузки. Один бинарник. Ноль настроек. Встроенный proxyDHCP — роутер трогать не придётся. 50+ дистрибутивов распознаются автоматически.',
    ctaPrimary: '$ get bootimus',
    ctaSecondary: 'исходники',
    quickstartTitle: 'bootimus — быстрый старт',
    statDistrosN: '50+',
    statDistrosL: 'дистрибутивов распознано',
    statBinaryN: '1',
    statBinaryL: 'бинарник, ноль зависимостей',
    statReconfigsN: '0',
    statReconfigsL: 'переконфигов DHCP',
    statArchN: '2',
    statArchL: 'архитектуры: amd64 · arm64',
  },

  features: {
    kicker: '// возможности',
    title: 'Современный netboot должен выглядеть так.',
    sub: 'Не форк Perl-скриптов пятнадцатилетней давности. Не обёртка вокруг dnsmasq. Полноценный сервер на Go, со всем нужным внутри.',
    items: {
      '01': {
        title: 'Один бинарник',
        body: 'Go-бинарник с встроенным iPXE, веб-интерфейсом, SQLite и всеми ассетами. Никаких рантайм-зависимостей. scp и запускай.',
      },
      '02': {
        title: 'Встроенный proxyDHCP',
        body: 'Отвечает на PXE через UDP/67, не задевая ваш существующий DHCP. Ноль изменений в роутере. Подключается к любой LAN.',
      },
      '03': {
        title: '50+ дистрибутивов',
        body: 'Автоматическое извлечение kernel/initrd для Ubuntu, Debian, Arch, Fedora, NixOS, Alpine, FreeBSD, Windows (wimboot) и других.',
      },
      '04': {
        title: 'ACL по MAC-адресу',
        body: 'Назначайте конкретный образ на конкретный MAC. Новые клиенты автоматически обнаруживаются при первом PXE. Лизы повышаются до статических по готовности.',
      },
      '05': {
        title: 'Инструменты в один клик',
        body: 'GParted, Clonezilla, Memtest86+, SystemRescue, ShredOS, netboot.xyz. Включаете в UI — они появляются в меню.',
      },
      '06': {
        title: 'JWT + LDAP',
        body: 'Токен-аутентификация с bcrypt. Опциональный бэкенд LDAP/AD с админ-доступом по группам. Локальные учётки остаются как резерв.',
      },
      '07': {
        title: 'REST API',
        body: 'Всё, что делает UI, — это вызовы API. Скриптуйте назначения загрузки, сканирования, WoL. Поток логов в реальном времени через SSE.',
      },
      '08': {
        title: 'Работает где угодно',
        body: 'Multi-arch Docker (amd64/arm64), статический бинарник или 2-ГБ Alpine-образ, который можно прошить на USB.',
      },
      '09': {
        title: 'Автоматическая установка',
        body: 'Бросьте сюда autounattend.xml, kickstart, preseed или cloud-init. Привязывайте к образу как дефолт, переопределяйте на уровне клиента. Bootimus подсунет файл при загрузке — никаких кликов, никаких мастеров.',
      },
    },
  },

  howItWorks: {
    kicker: '// как это работает',
    title: 'Жизненный цикл PXE-загрузки.',
    sub: 'Клиент шлёт DHCPDISCOVER. Bootimus отвечает PXE-параметрами через proxyDHCP, пока ваш обычный DHCP продолжает раздавать IP. iPXE грузится по TFTP, переходит на HTTP, тянет меню. Пользователь выбирает образ. Kernel и initrd стримятся с сервера. Готово.',
    termTitle: 'pxe boot trace — ubuntu-24.04',
  },

  transparency: {
    kicker: '// прозрачность',
    title: '100% открыто. Аудит от и до.',
    sub: 'Никаких проприетарных блобов. Никакой телеметрии. Никакой втихую вшитой бинарной прошивки. Весь стек лежит на GitHub под Apache 2.0 — клонируйте, проверяйте, форкайте, поднимайте у себя.',
    items: {
      binary: {
        t: 'Один Go-бинарник',
        d: 'статически слинкован, ldd возвращает "not a dynamic executable". Воспроизводимые сборки через make release.',
      },
      blobs: {
        t: 'Никаких проприетарных блобов',
        d: 'встроенный iPXE — upstream FOSS (GPL-2.0). Закрытой прошивки в комплекте нет.',
      },
      telemetry: {
        t: 'Никакой телеметрии. Никогда.',
        d: 'ноль call-home. Ноль аналитики. Ноль "анонимной статистики". Подходит для air-gap-сетей.',
      },
      licence: {
        t: 'Apache 2.0',
        d: 'разрешительная лицензия. Используйте в коммерции, разворачивайте внутри компании, форкайте без оговорок.',
      },
      deps: {
        t: 'Vendored-зависимости, всё FOSS',
        d: 'каждая транзитивная Go-зависимость — open source. go mod why для любого пакета.',
      },
      byo: {
        t: 'Свой загрузчик',
        d: 'не доверяете встроенному iPXE? Положите свои подписанные бинарники. Подробности ниже.',
      },
    },
    termTitle: 'bootimus version --verbose',
  },

  bootloaders: {
    kicker: '// загрузчики',
    title: 'Замените iPXE на то, что нужно вам.',
    sub: 'Bootimus поставляется со встроенным iPXE под все распространённые архитектуры — включая Microsoft-подписанную цепочку Secure Boot. Нужен кастомно оформленный iPXE, GRUB, syslinux или собственный загрузчик, подписанный внутренним CA? Положите папку в data/bootloaders/, выберите в UI — и готово. Отсутствующие файлы прозрачно откатываются на встроенный набор — загрузка не сломается никогда.',
    cards: {
      uefi64: {
        t: 'iPXE · UEFI x86_64',
        d: 'ipxe.efi · по умолчанию. Собран из upstream master, встроен в бинарник.',
        tag: 'встроено · резерв',
      },
      uefiArm: {
        t: 'iPXE · UEFI ARM64',
        d: 'ipxe-arm64.efi · для Raspberry Pi 4/5, машин на Apple Silicon, ARM-серверов.',
        tag: 'встроено · резерв',
      },
      bios: {
        t: 'iPXE · Legacy BIOS',
        d: 'undionly.kpxe · для старого железа без UEFI. В 2026-м всё ещё актуально.',
        tag: 'встроено · резерв',
      },
      shim: {
        t: 'Secure Boot · подписанный shim',
        d: 'ipxe-shimx64.efi · Microsoft-подписанный shim + подписанный iPXE для парка с принудительным Secure Boot. Регистрация MOK в прошивке не нужна.',
        tag: 'встроено · secureboot-official',
      },
      themed: {
        t: 'Кастомно оформленный iPXE',
        d: 'Соберите свой iPXE с брендингом, цветами меню, встроенными скриптами. Положите .efi — и всё.',
        tag: 'свой · BYO',
      },
      grub: {
        t: 'GRUB / syslinux / pxelinux',
        d: 'Не iPXE? Не проблема. Всё, что говорит по TFTP и HTTP, работает. Bootimus просто отдаёт байты.',
        tag: 'свой · BYO',
      },
    },
    termTitle: 'наборы загрузчиков — отказоустойчивость файлов',
  },

  cta: {
    title: 'Надоело нянчиться с tftpd?',
    sub: 'Docker, голое железо или прошиваемая флешка. Выбирайте по вкусу.',
    primary: '$ get bootimus',
    secondary: 'к документации →',
  },

  downloads: {
    kicker: '// загрузки',
    title: 'Заберите сборку.',
    lede:
      'Каждый релиз поставляется как Docker-образ, статические Linux-бинарники из последнего GitHub-релиза и 2 ГиБ прошиваемый USB-образ (Alpine + bootimus, грузится сразу). Инструменты зеркалятся отдельно, чтобы админ-UI мог их тянуть, не упираясь в лимиты апстрима.',
    badgeStable: 'актуальный · стабильный',
    badgePrerelease: 'pre-release',
    badgeNone: 'релизов пока нет',
    pillManifest: 'manifest.json',
    pillSource: 'github releases ↗',
    pillBuildSrc: 'собрать из исходников',
    released: 'выпущено',
    via: 'через',
    sectionArtifacts: 'артефакты релиза',
    sectionTools: 'зеркала инструментов',
    sectionApi: 'манифест',
    emptyTitle: 'Бинарники пока не опубликованы',
    emptyBody:
      'Поставьте тег в GitHub — и бинарники из make release появятся здесь автоматически.',
    buildFromSource: 'собрать из исходников ↗',
    verifyTitle: 'проверить',
    apiTitle: 'api · manifest.json',
    toolsLede:
      'Это netboot-готовые образы, которые админ-UI может скачать по требованию и показать как пункты PXE-меню. Зеркалятся на dl.bootimus.com — чтобы вы не зависели от апстрима, когда срочно поднимаете rescue. Upstream-URL остаются источником истины — любой mirror-URL можно переопределить на странице Tools.',
    apiLede:
      'Админ-UI читает /api/manifest.json, чтобы проверять обновления и показывать доступные инструменты. Схема стабильная — поля без major-bump не переименовываются. Источник истины для бинарников — GitHub Releases API; этот эндпоинт лишь нормализует и подмешивает статику (Docker-тег, образ аппаратуры, зеркала инструментов).',
    mirror: 'зеркало ↓',
    upstream: 'апстрим ↗',
    get: 'скачать ↓',
  },

  docs: {
    title: 'Документация',
    subtitle: 'Всё, что нужно для развёртывания, настройки и эксплуатации bootimus.',
    sectionsTitle: 'Разделы',
    onThisPage: 'На этой странице',
    prev: 'Предыдущая',
    next: 'Следующая',
    editOnGithub: 'Править на GitHub',
    notFound: 'Страница не найдена.',
    fallbackBanner: 'Эта страница ещё не переведена. Показана английская версия.',
    translateCta: 'Помочь с переводом →',
    pending: 'перевод в работе',
  },
};

export default ru;
