/** Formats a credits amount, e.g. 12.25 or 4, without forcing decimals. */
export function formatCredits(credits: number, locale: string): string {
  return new Intl.NumberFormat(locale, { maximumFractionDigits: 2 }).format(credits);
}
