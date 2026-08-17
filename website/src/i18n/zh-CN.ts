const zhCN = {
  meta: {
    title: 'Bootimus — 现代化 PXE/HTTP 引导服务器',
    description:
      '一体化 PXE 与 HTTP 引导服务器。单一二进制。零配置。开箱即用 50+ 发行版。',
    downloadsTitle: '下载 — Bootimus',
    downloadsDescription:
      '下载 bootimus 二进制、Docker 镜像、USB 设备镜像，以及镜像的 PXE 工具。',
    docsTitle: '文档 — Bootimus',
    docsDescription:
      'Bootimus 文档:部署、镜像管理、DHCP、身份认证、客户端 ACL、发行版配置。',
  },

  nav: {
    features: '特性',
    howItWorks: '工作原理',
    downloads: '下载',
    docs: '文档',
    github: 'GitHub',
    toggleTheme: '切换主题',
    languageLabel: '语言',
    comingSoon: '即将推出',
    madeInBritain: 'made in britain',
  },

  footer: {
    project: '项目',
    licence: '许可',
    licenceValue: 'Apache 2.0',
    lang: '语言',
    repo: '仓库',
    docker: 'docker',
    docs: '文档',
    copy: '© {year} bootimus 贡献者',
  },

  hero: {
    badgeStable: 'v1.x · apache 2.0',
    badgeStack: 'go · iPXE · sqlite/postgres',
    titleLine1: 'PXE 引导,',
    titleLine2: '从此不再头疼',
    sub: '一体化 PXE 与 HTTP 引导服务器。一个二进制。零配置。内建 proxyDHCP — 路由器一动不用动。自动识别 50+ 发行版。',
    ctaPrimary: '$ get bootimus',
    ctaSecondary: '查看源码',
    quickstartTitle: 'bootimus — 快速开始',
    statDistrosN: '50+',
    statDistrosL: '已识别发行版',
    statBinaryN: '1',
    statBinaryL: '单一二进制,零依赖',
    statReconfigsN: '0',
    statReconfigsL: 'DHCP 重配次数',
    statArchN: '2',
    statArchL: '架构: amd64 · arm64',
  },

  features: {
    kicker: '// 特性',
    title: '现代 netboot 该有的样子,这里都有。',
    sub: '不是十五年前 Perl 脚本的分支。不是套在 dnsmasq 外面的壳子。一个用 Go 写的正经服务器,该有的全部内置。',
    items: {
      '01': {
        title: '单一二进制',
        body: 'Go 二进制内嵌 iPXE、Web UI、SQLite 和全部资源。无运行时依赖。scp 上去就能跑。',
      },
      '02': {
        title: '内建 proxyDHCP',
        body: '在 UDP/67 上应答 PXE,完全不干扰你现有的 DHCP。路由器零改动。任意局域网即插即用。',
      },
      '03': {
        title: '50+ 发行版',
        body: '自动提取 Ubuntu、Debian、Arch、Fedora、NixOS、Alpine、FreeBSD、Windows (wimboot) 等系统的 kernel/initrd。',
      },
      '04': {
        title: '按 MAC 地址 ACL',
        body: '为每个 MAC 指派专属镜像。首次 PXE 请求自动发现新客户端。准备就绪后将租约提升为静态。',
      },
      '05': {
        title: '工具一键启用',
        body: 'GParted、Clonezilla、Memtest86+、SystemRescue、ShredOS、netboot.xyz。UI 里勾选,菜单立即出现。',
      },
      '06': {
        title: 'JWT + LDAP',
        body: 'bcrypt 加密的 token 认证。可选 LDAP/AD 后端,基于组的管理员权限。本地账号始终作为兜底。',
      },
      '07': {
        title: 'REST API',
        body: 'UI 能做的事,API 全都能做。脚本化引导分配、扫描、WoL 触发。通过 SSE 实时推送日志流。',
      },
      '08': {
        title: '到处都能跑',
        body: '多架构 Docker (amd64/arm64)、静态二进制,或可烧录到 U 盘的 2GB 基于 Alpine 的设备镜像。',
      },
      '09': {
        title: '无人值守安装',
        body: '把 autounattend.xml、kickstart、preseed 或 cloud-init 文件丢进去。挂到镜像做默认,按客户端覆盖。Bootimus 在引导时自动投放 — 无需点击,无需向导。',
      },
    },
  },

  howItWorks: {
    kicker: '// 工作原理',
    title: '一次 PXE 引导的全生命周期。',
    sub: '客户端发出 DHCPDISCOVER。Bootimus 通过 proxyDHCP 回应 PXE 详情,你原本的 DHCP 继续分配 IP。iPXE 通过 TFTP 加载,链式跳转到 HTTP,拉取菜单。用户选择镜像。kernel 和 initrd 从服务器流式传输。完成。',
    termTitle: 'pxe boot trace — ubuntu-24.04',
  },

  transparency: {
    kicker: '// 透明度',
    title: '100% 开源。端到端可审计。',
    sub: '没有专有 blob。没有遥测。没有偷偷塞进来的固件二进制。整套技术栈在 GitHub 上以 Apache 2.0 发布 — 克隆、审计、分叉、自部署随你便。',
    items: {
      binary: {
        t: '单一 Go 二进制',
        d: '静态链接,ldd 返回 "not a dynamic executable"。make release 产出可复现构建。',
      },
      blobs: {
        t: '没有专有 blob',
        d: '内嵌的 iPXE 是上游 FOSS (GPL-2.0)。不附带任何闭源固件。',
      },
      telemetry: {
        t: '永远没有遥测',
        d: '零回家上报。零分析统计。零所谓"匿名使用数据"。物理隔离网络可用。',
      },
      licence: {
        t: 'Apache 2.0',
        d: '宽松许可证。可商用、可内部部署、可分叉,无附加条款。',
      },
      deps: {
        t: '依赖全部 vendored,全部 FOSS',
        d: '每一个传递性 Go 依赖都是开源的。任何包都可以 go mod why 查到。',
      },
      byo: {
        t: '自带 bootloader',
        d: '不信任内嵌的 iPXE?把你自己签名的二进制放进来。见下方。',
      },
    },
    termTitle: 'bootimus version --verbose',
  },

  bootloaders: {
    kicker: '// bootloaders',
    title: '把 iPXE 换成你需要的任何东西。',
    sub: 'Bootimus 为每种常见架构内嵌了 iPXE — 包括微软签名的 Secure Boot 引导链。需要定制主题的 iPXE、GRUB、syslinux,或你自家内部 CA 签名的 loader?把文件夹丢到 data/bootloaders/,UI 里勾选,搞定。缺失文件会透明地回退到内嵌套件 — 永远不会引导失败。',
    cards: {
      uefi64: {
        t: 'iPXE · UEFI x86_64',
        d: 'ipxe.efi · 默认。从上游 master 构建,内嵌进二进制。',
        tag: '内嵌 · 兜底',
      },
      uefiArm: {
        t: 'iPXE · UEFI ARM64',
        d: 'ipxe-arm64.efi · 适用于 Raspberry Pi 4/5、Apple Silicon 主机、ARM 服务器。',
        tag: '内嵌 · 兜底',
      },
      bios: {
        t: 'iPXE · 传统 BIOS',
        d: 'undionly.kpxe · 给那些不会 UEFI 的老设备。2026 年依然有市场。',
        tag: '内嵌 · 兜底',
      },
      shim: {
        t: 'Secure Boot · 签名 shim',
        d: 'ipxe-shimx64.efi · 微软签名 shim + 签名 iPXE,适用于强制 Secure Boot 的集群。无需固件 MOK 注册。',
        tag: '内嵌 · secureboot-official',
      },
      themed: {
        t: '定制主题 iPXE',
        d: '编译你自己的 iPXE,带品牌标识、定制菜单配色、内嵌脚本。把 .efi 放进去即可。',
        tag: '自定义 · BYO',
      },
      grub: {
        t: 'GRUB / syslinux / pxelinux',
        d: '不用 iPXE?没问题。任何会 TFTP 和 HTTP 的东西都行。Bootimus 只负责吐字节。',
        tag: '自定义 · BYO',
      },
    },
    termTitle: 'bootloader 套件 — 文件回退',
  },

  cta: {
    title: '不想再当 tftpd 的保姆了?',
    sub: 'Docker、裸金属,或可烧录的 U 盘。挑一个顺手的。',
    primary: '$ get bootimus',
    secondary: '查看文档 →',
  },

  downloads: {
    kicker: '// 下载',
    title: '拿一个构建走。',
    lede:
      '每个版本都提供:Docker 镜像、从最新 GitHub release 实时拉取的静态 Linux 二进制,以及一个 2 GiB 可烧录到 U 盘的设备镜像 (Alpine + bootimus,开机即用)。工具单独镜像,这样管理 UI 拉取时不必担心上游限速。',
    badgeStable: '最新 · 稳定',
    badgePrerelease: '预发布',
    badgeNone: '尚无发布',
    pillManifest: 'manifest.json',
    pillSource: 'github releases ↗',
    pillBuildSrc: '从源码构建',
    released: '发布于',
    via: '来自',
    sectionArtifacts: '发布构件',
    sectionTools: '镜像工具',
    sectionApi: '使用 manifest',
    emptyTitle: '尚未发布二进制',
    emptyBody:
      '在 GitHub 上打个 tag,make release 生成的二进制就会自动出现在这里。',
    buildFromSource: '从源码构建 ↗',
    verifyTitle: '校验',
    apiTitle: 'api · manifest.json',
    toolsLede:
      '这些是 netboot-ready 的镜像,管理 UI 可按需下载并作为 PXE 菜单项呈现。镜像在 dl.bootimus.com 上,这样在跑救援任务时就不必依赖上游可用性。上游 URL 仍是权威来源 — 任何镜像 URL 都可以在工具页面里覆盖。',
    apiLede:
      '管理 UI 读取 /api/manifest.json 来检查更新和列出可用工具。Schema 稳定 — 不做大版本升级就不会重命名字段。二进制的权威来源是 GitHub Releases API;这个端点只是做规范化并合入静态部分 (Docker 标签、设备镜像、镜像工具)。',
    mirror: '镜像 ↓',
    upstream: '上游 ↗',
    get: '下载 ↓',
  },

  docs: {
    title: '文档',
    subtitle: '部署、配置、运维 bootimus 所需的一切。',
    sectionsTitle: '章节',
    onThisPage: '本页目录',
    prev: '上一页',
    next: '下一页',
    editOnGithub: '在 GitHub 上编辑',
    notFound: '未找到该文档。',
    fallbackBanner: '此页面尚未翻译。显示英文版本。',
    translateCta: '帮忙翻译 →',
    pending: '翻译中',
  },
};

export default zhCN;
