export interface GhAsset {
  name: string;
  browser_download_url: string;
  size: number;
  digest?: string | null;
  download_count?: number;
  content_type?: string;
}

export interface GhRelease {
  tag_name: string;
  name: string;
  published_at: string;
  html_url: string;
  body: string;
  prerelease: boolean;
  draft: boolean;
  assets: GhAsset[];
}

const REPO = 'garybowers/bootimus';
const CACHE_TTL_MS = 5 * 60 * 1000;
const STALE_TTL_MS = 24 * 60 * 60 * 1000;

interface CacheEntry {
  data: GhRelease | null;
  fetchedAt: number;
}

let cache: CacheEntry | null = null;
let inFlight: Promise<GhRelease | null> | null = null;

export async function getLatestRelease(): Promise<GhRelease | null> {
  const now = Date.now();
  if (cache && now - cache.fetchedAt < CACHE_TTL_MS) return cache.data;
  if (inFlight) return inFlight;

  inFlight = (async () => {
    try {
      const headers: Record<string, string> = {
        Accept: 'application/vnd.github+json',
        'X-GitHub-Api-Version': '2022-11-28',
        'User-Agent': 'bootimus-site/1.0 (+https://bootimus.com)',
      };
      const token = process.env.GITHUB_TOKEN;
      if (token) headers.Authorization = `Bearer ${token}`;

      const res = await fetch(
        `https://api.github.com/repos/${REPO}/releases/latest`,
        { headers, signal: AbortSignal.timeout(8000) },
      );

      if (res.status === 404) {
        cache = { data: null, fetchedAt: now };
        return null;
      }
      if (!res.ok) {
        if (cache && now - cache.fetchedAt < STALE_TTL_MS) return cache.data;
        return null;
      }

      const data = (await res.json()) as GhRelease;
      cache = { data, fetchedAt: now };
      return data;
    } catch {
      if (cache && now - cache.fetchedAt < STALE_TTL_MS) return cache.data;
      return null;
    } finally {
      inFlight = null;
    }
  })();

  return inFlight;
}

export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return '—';
  const u = ['B', 'KiB', 'MiB', 'GiB', 'TiB'];
  let i = 0;
  let v = bytes;
  while (v >= 1024 && i < u.length - 1) { v /= 1024; i++; }
  return `${v < 10 && i > 0 ? v.toFixed(2) : v.toFixed(i === 0 ? 0 : 1)} ${u[i]}`;
}

export function strV(tag: string | undefined | null): string | null {
  if (!tag) return null;
  return tag.replace(/^v/i, '');
}

export interface DerivedArtifact {
  kind: 'binary' | 'checksum' | 'archive' | 'other';
  platform?: string;
  label: string;
  filename: string;
  url: string;
  size: string;
  sha256?: string;
}

const PLATFORM_PATTERNS: Array<[RegExp, string, string]> = [
  [/linux[-_]?amd64|linux[-_]?x86[-_]?64/i, 'linux/amd64', 'Linux · amd64'],
  [/linux[-_]?arm64|linux[-_]?aarch64/i,    'linux/arm64', 'Linux · arm64'],
  [/linux[-_]?armv?7|linux[-_]?arm(?!64)/i, 'linux/arm',   'Linux · arm'],
  [/darwin[-_]?amd64|macos[-_]?amd64/i,     'darwin/amd64', 'macOS · Intel'],
  [/darwin[-_]?arm64|macos[-_]?arm64/i,     'darwin/arm64', 'macOS · Apple Silicon'],
  [/windows[-_]?amd64|win[-_]?amd64/i,      'windows/amd64', 'Windows · amd64'],
  [/freebsd[-_]?amd64/i,                    'freebsd/amd64', 'FreeBSD · amd64'],
];

export function deriveArtifact(a: GhAsset): DerivedArtifact {
  const name = a.name;
  const lower = name.toLowerCase();

  if (/sha256|sha512|checksum|sums/i.test(name)) {
    return {
      kind: 'checksum',
      label: name,
      filename: name,
      url: a.browser_download_url,
      size: formatBytes(a.size),
    };
  }

  for (const [re, platform, label] of PLATFORM_PATTERNS) {
    if (re.test(lower)) {
      return {
        kind: 'binary',
        platform,
        label,
        filename: name,
        url: a.browser_download_url,
        size: formatBytes(a.size),
        sha256: a.digest?.replace(/^sha256:/, ''),
      };
    }
  }

  if (/\.(tar\.gz|tgz|zip|tar\.xz|tar\.bz2)$/i.test(lower)) {
    return {
      kind: 'archive',
      label: name,
      filename: name,
      url: a.browser_download_url,
      size: formatBytes(a.size),
    };
  }

  return {
    kind: 'other',
    label: name,
    filename: name,
    url: a.browser_download_url,
    size: formatBytes(a.size),
  };
}
