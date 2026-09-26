import { Link } from "react-router-dom";
import { useTranslation } from "react-i18next";

/**
 * Symbol + wordmark used in every top bar (app, project picker, admin, auth
 * screens). The symbol's five paths each read a CSS variable (--sym-black,
 * --sym-orange, --sym-teal, --sym-red, --sym-navy - see src/index.css),
 * synced with landing/symbol.svg. `oneColor` renders every piece in
 * currentColor, for empty states rather than the header.
 */
export function BrandMark({ oneColor = false, className }: { oneColor?: boolean; className?: string }) {
  const { t } = useTranslation();
  return (
    <Link to="/" className={`brand-mark-link ${className ?? ""}`} aria-label={t("app.brand")}>
      <BrandSymbol oneColor={oneColor} />
      <span className="brand-name">
        Studio<b>IA</b>
      </span>
    </Link>
  );
}

export function BrandSymbol({ oneColor = false, className }: { oneColor?: boolean; className?: string }) {
  const fill = (token: string, fallback: string) => (oneColor ? "currentColor" : `var(${token}, ${fallback})`);
  return (
    <svg className={`brand-symbol ${className ?? ""}`} viewBox="0 0 348 354" aria-hidden="true">
      <path fill={fill("--sym-teal", "#18726E")} d="M120 122L186 122C221 122 230 156 230 206L230 243L120 243Z" />
      <path fill={fill("--sym-black", "#2A2929")} d="M48 14C8 14 0 44 0 124C0 204 8 234 48 234L110 234L110 14Z" />
      <path fill={fill("--sym-orange", "#C25939")} d="M120 49C120 9 150 1 230 1C310 1 340 9 340 49L340 111L120 111Z" />
      <path fill={fill("--sym-red", "#A53B3F")} d="M300 124C340 124 348 154 348 234C348 314 340 344 300 344L238 344L238 124Z" />
      <path fill={fill("--sym-navy", "#263E5A")} d="M10 305C10 345 40 353 120 353C200 353 230 345 230 305L230 243L10 243Z" />
    </svg>
  );
}
