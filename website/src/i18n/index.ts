import en from './en';
import de from './de';
import fr from './fr';
import es from './es';
import ru from './ru';

export type Locale = 'en' | 'de' | 'fr' | 'es' | 'ru';

export const defaultLocale: Locale = 'en';
export const locales: Locale[] = ['en', 'de', 'fr', 'es', 'ru'];

export const localeMeta: Record<Locale, { label: string; native: string; flag: string; ready: boolean }> = {
  en: { label: 'English', native: 'English (UK)', flag: 'EN', ready: true },
  de: { label: 'German',  native: 'Deutsch',      flag: 'DE', ready: true },
  fr: { label: 'French',  native: 'Français',     flag: 'FR', ready: true },
  es: { label: 'Spanish', native: 'Español',      flag: 'ES', ready: true },
  ru: { label: 'Russian', native: 'Русский',      flag: 'RU', ready: false },
};

const dictionaries = { en, de, fr, es, ru };

type Dict = typeof en;
type Path<T> = T extends object
  ? { [K in keyof T]: K extends string
      ? T[K] extends object ? `${K}` | `${K}.${Path<T[K]>}` : `${K}`
      : never }[keyof T]
  : never;
export type TranslationKey = Path<Dict>;

function deepGet(obj: unknown, path: string): unknown {
  return path.split('.').reduce<unknown>((acc, k) => {
    if (acc && typeof acc === 'object') return (acc as Record<string, unknown>)[k];
    return undefined;
  }, obj);
}

function interpolate(s: string, vars?: Record<string, string | number>): string {
  if (!vars) return s;
  return s.replace(/\{(\w+)\}/g, (_, k) => (k in vars ? String(vars[k]) : `{${k}}`));
}

export function useTranslations(lang: Locale = defaultLocale) {
  const primary = dictionaries[lang];
  const fallback = dictionaries.en;

  function t(key: TranslationKey, vars?: Record<string, string | number>): string {
    const v = deepGet(primary, key) ?? deepGet(fallback, key);
    if (typeof v === 'string') return interpolate(v, vars);
    return key;
  }

  function tx<T = unknown>(key: TranslationKey): T {
    const v = deepGet(primary, key) ?? deepGet(fallback, key);
    return v as T;
  }

  return { t, tx };
}

export function getLocaleFromUrl(url: URL): Locale {
  const seg = url.pathname.split('/').filter(Boolean)[0];
  if (seg && (locales as string[]).includes(seg)) return seg as Locale;
  return defaultLocale;
}

export function pathForLocale(pathname: string, target: Locale): string {
  const parts = pathname.split('/').filter(Boolean);
  const first = parts[0];
  const isLocaleSeg = first && (locales as string[]).includes(first);
  const rest = isLocaleSeg ? parts.slice(1) : parts;
  if (target === defaultLocale) {
    return '/' + rest.join('/');
  }
  return '/' + [target, ...rest].join('/');
}
