import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { ApiError, getMyCredits } from "../api";
import { formatCredits } from "../lib/credits";
import { useProject } from "../state/ProjectContext";
import type { CreditEntry } from "../types";
import { Modal } from "./Modal";

/**
 * Balance chip shown in the app top bar and the project picker header. Click
 * to open the usage/ledger dialog. Renders nothing until /api/me resolves.
 */
export function CreditsChip() {
  const { t, i18n } = useTranslation();
  const { me } = useProject();
  const [open, setOpen] = useState(false);

  if (!me) return null;

  return (
    <>
      <button type="button" className="credits-chip" onClick={() => setOpen(true)}>
        <span className="credits-chip-value">{formatCredits(me.credits, i18n.language)}</span>
        <span className="credits-chip-label">{t("credits.chip.label")}</span>
      </button>
      {open && <CreditsLedgerDialog onClose={() => setOpen(false)} />}
    </>
  );
}

function CreditsLedgerDialog({ onClose }: { onClose: () => void }) {
  const { t, i18n } = useTranslation();
  const [credits, setCredits] = useState<number | null>(null);
  const [ledger, setLedger] = useState<CreditEntry[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let active = true;
    getMyCredits()
      .then((r) => {
        if (!active) return;
        setCredits(r.credits);
        setLedger(r.ledger);
      })
      .catch((err) => {
        if (active) setError(err instanceof ApiError ? err.message : t("credits.ledger.error"));
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [t]);

  const reasonLabels = useMemo(
    () => ({
      grant: t("credits.ledger.reasons.grant"),
      render: t("credits.ledger.reasons.render"),
      refund: t("credits.ledger.reasons.refund"),
    }),
    [t],
  );

  const numberFormat = useMemo(
    () => new Intl.NumberFormat(i18n.language, { maximumFractionDigits: 2, signDisplay: "always" }),
    [i18n.language],
  );

  return (
    <Modal title={t("credits.ledger.title")} onClose={onClose}>
      {loading && (
        <div className="picker-status">
          <span className="spinner" /> {t("credits.ledger.loading")}
        </div>
      )}
      {error && <div className="error-banner">{error}</div>}
      {!loading && !error && (
        <>
          {credits !== null && (
            <div className="credits-ledger-balance">
              {t("credits.ledger.balance", { credits: formatCredits(credits, i18n.language) })}
            </div>
          )}
          {ledger && ledger.length === 0 && (
            <div className="empty-state">
              <strong>{t("credits.ledger.empty.title")}</strong>
              <span>{t("credits.ledger.empty.body")}</span>
            </div>
          )}
          {ledger && ledger.length > 0 && (
            <div className="credits-ledger-body">
              <table className="render-history-table">
                <thead>
                  <tr>
                    <th>{t("credits.ledger.columns.date")}</th>
                    <th>{t("credits.ledger.columns.reason")}</th>
                    <th>{t("credits.ledger.columns.delta")}</th>
                    <th>{t("credits.ledger.columns.balance")}</th>
                  </tr>
                </thead>
                <tbody>
                  {ledger.map((entry) => (
                    <tr key={entry.id}>
                      <td>{new Date(entry.createdAt).toLocaleString(i18n.language)}</td>
                      <td>
                        {reasonLabels[entry.reason]}
                        {entry.note ? ` — ${entry.note}` : ""}
                      </td>
                      <td>{numberFormat.format(entry.delta)}</td>
                      <td>{formatCredits(entry.balanceAfter, i18n.language)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </>
      )}
      <div className="modal-actions">
        <button type="button" className="btn btn-ghost" onClick={onClose}>
          {t("common.close")}
        </button>
      </div>
    </Modal>
  );
}
