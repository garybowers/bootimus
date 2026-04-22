import { getCollection, type CollectionEntry } from 'astro:content';
import { defaultLocale, type Locale } from '../i18n';

export interface DocLookup {
  entry: CollectionEntry<'docs'>;
  requested: Locale;
  served: Locale;
  isFallback: boolean;
}

export async function getDoc(locale: Locale, slug: string): Promise<DocLookup | null> {
  const all = await getCollection('docs');
  const exact = all.find((e) => e.id === `${locale}/${slug}`);
  if (exact) {
    return { entry: exact, requested: locale, served: locale, isFallback: false };
  }
  const fallback = all.find((e) => e.id === `${defaultLocale}/${slug}`);
  if (fallback) {
    return {
      entry: fallback,
      requested: locale,
      served: defaultLocale,
      isFallback: locale !== defaultLocale,
    };
  }
  return null;
}

export async function listDocs(locale: Locale): Promise<CollectionEntry<'docs'>[]> {
  const all = await getCollection('docs');
  return all.filter((e) => e.id.startsWith(`${locale}/`));
}

export function slugFromId(id: string): string {
  const i = id.indexOf('/');
  return i >= 0 ? id.slice(i + 1) : id;
}

export function localeFromId(id: string): Locale | null {
  const i = id.indexOf('/');
  if (i < 0) return null;
  const l = id.slice(0, i) as Locale;
  return ['en', 'de', 'fr', 'es', 'ru'].includes(l) ? l : null;
}

export function docUrl(locale: Locale, slug?: string): string {
  const base = locale === defaultLocale ? '/docs' : `/${locale}/docs`;
  if (!slug) return base;
  return `${base}/${slug}`;
}
