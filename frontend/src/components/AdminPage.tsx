import { UserButton } from "@clerk/clerk-react";
import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { ApiError, getAdminUserCredits, grantAdminCredits, listAdminUsers } from "../api";
import type { AdminUser, CreditEntry } from "../types";
import { formatCredits } from "../lib/credits";
import { LanguageSwitcher } from "./LanguageSwitcher";
import { Modal } from "./Modal";
import "./AdminPage.css";

const PAGE_SIZE = 50;

export function AdminPage() {
  const { t, i18n } = useTranslation();
  const [query, setQuery] = useState("");
  const [appliedQuery, setAppliedQuery] = useState("");
  const [offset, setOffset] = useState(0);
  const [users, setUsers] = useState<AdminUser[] | null>(null);
  const [totalCount, setTotalCount] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [grantingFor, setGrantingFor] = useState<AdminUser | null>(null);
  const [ledgerFor, setLedgerFor] = useState<AdminUser | null>(null);

  const load = useCallback(async (q: string, off: number) => {
    setLoading(true);
    setError(null);
    try {
      const res = await listAdminUsers(q, PAGE_SIZE, off);
      setUsers(res.users);
      setTotalCount(res.totalCount);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to load users");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load(appliedQuery, offset);
  }, [load, appliedQuery, offset]);

  function handleSearch(e: React.FormEvent) {
    e.preventDefault();
    setOffset(0);
    setAppliedQuery(query.trim());
  }

  function handleCreditsGranted(uid: string, credits: number) {
    setUsers((prev) => (prev ? prev.map((u) => (u.id === uid ? { ...u, credits } : u)) : prev));
    setGrantingFor(null);
  }

  const rangeStart = totalCount === 0 ? 0 : offset + 1;
  const rangeEnd = Math.min(offset + PAGE_SIZE, totalCount);

  return (
    <div className="picker-screen">
      <header className="picker-topbar">
        <div className="app-brand">{t("app.brand")} · {t("admin.title")}</div>
        <div className="app-topbar-right">
          <Link to="/" className="btn btn-ghost btn-sm">
            {t("admin.backToApp")}
          </Link>
          <LanguageSwitcher />
          <UserButton afterSignOutUrl="/sign-in" />
        </div>
      </header>

      <main className="picker-main">
        <div className="picker-head">
          <div>
            <h1 className="picker-heading">{t("admin.title")}</h1>
            <p className="field-hint">{t("admin.subtitle")}</p>
          </div>
        </div>

        <form className="admin-search" onSubmit={handleSearch}>
          <input
            className="input"
            placeholder={t("admin.searchPlaceholder")}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
          <button type="submit" className="btn btn-sm">
            {t("admin.search")}
          </button>
        </form>

        {error && <div className="error-banner">{error}</div>}

        {loading && (
          <div className="picker-status">
            <span className="spinner" /> {t("admin.loading")}
          </div>
        )}

        {!loading && users && (
          <section className="panel">
            <div className="panel-body admin-table-body">
              <table className="render-history-table admin-users-table">
                <thead>
                  <tr>
                    <th>{t("admin.columns.user")}</th>
                    <th>{t("admin.columns.role")}</th>
                    <th>{t("admin.columns.credits")}</th>
                    <th>{t("admin.columns.joined")}</th>
                    <th />
                  </tr>
                </thead>
                <tbody>
                  {users.map((u) => (
                    <tr key={u.id}>
                      <td>
                        <div className="admin-user-cell">
                          <span className="admin-user-name">
                            {[u.firstName, u.lastName].filter(Boolean).join(" ") || u.email}
                          </span>
                          <span className="admin-user-email">{u.email}</span>
                        </div>
                      </td>
                      <td>{u.role ?? "—"}</td>
                      <td>{formatCredits(u.credits, i18n.language)}</td>
                      <td>{new Date(u.createdAt).toLocaleDateString()}</td>
                      <td>
                        <div className="admin-row-actions">
                          <button type="button" className="btn btn-ghost btn-sm" onClick={() => setLedgerFor(u)}>
                            {t("admin.viewLedger")}
                          </button>
                          <button type="button" className="btn btn-sm" onClick={() => setGrantingFor(u)}>
                            {t("admin.giveCredits")}
                          </button>
                        </div>
                      </td>
                    </tr>
                  ))}
                  {users.length === 0 && (
                    <tr>
                      <td colSpan={5}>
                        <div className="empty-state">
                          <strong>{t("admin.empty.title")}</strong>
                          <span>{t("admin.empty.body")}</span>
                        </div>
                      </td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>
          </section>
        )}

        {!loading && totalCount > 0 && (
          <div className="admin-pagination">
            <span className="field-hint">
              {t("admin.pagination.range", { start: rangeStart, end: rangeEnd, total: totalCount })}
            </span>
            <div className="admin-pagination-buttons">
              <button
                type="button"
                className="btn btn-sm"
                onClick={() => setOffset((o) => Math.max(0, o - PAGE_SIZE))}
                disabled={offset === 0}
              >
                {t("admin.pagination.prev")}
              </button>
              <button
                type="button"
                className="btn btn-sm"
                onClick={() => setOffset((o) => o + PAGE_SIZE)}
                disabled={offset + PAGE_SIZE >= totalCount}
              >
                {t("admin.pagination.next")}
              </button>
            </div>
          </div>
        )}
      </main>

      {grantingFor && (
        <GrantCreditsDialog
          user={grantingFor}
          onGranted={(credits) => handleCreditsGranted(grantingFor.id, credits)}
          onClose={() => setGrantingFor(null)}
        />
      )}

      {ledgerFor && <UserLedgerDialog user={ledgerFor} onClose={() => setLedgerFor(null)} />}
    </div>
  );
}

function GrantCreditsDialog({
  user,
  onGranted,
  onClose,
}: {
  user: AdminUser;
  onGranted: (credits: number) => void;
  onClose: () => void;
}) {
  const { t, i18n } = useTranslation();
  const [amount, setAmount] = useState("");
  const [note, setNote] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const parsed = Number(amount);
  const valid = amount.trim() !== "" && Number.isFinite(parsed) && parsed !== 0 && Math.round(parsed * 4) === parsed * 4;

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!valid) return;
    setBusy(true);
    setError(null);
    try {
      const res = await grantAdminCredits(user.id, parsed, note.trim() || undefined);
      onGranted(res.credits);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t("admin.grant.error"));
      setBusy(false);
    }
  }

  return (
    <Modal title={t("admin.grant.title", { name: user.email })} onClose={onClose} busy={busy}>
      <form className="admin-grant-form" onSubmit={handleSubmit}>
        <p className="field-hint">
          {t("admin.grant.currentBalance", { credits: formatCredits(user.credits, i18n.language) })}
        </p>
        <div className="field">
          <label className="field-label" htmlFor="grant-amount">
            {t("admin.grant.amountLabel")}
          </label>
          <input
            id="grant-amount"
            className="input"
            type="number"
            step="0.25"
            placeholder={t("admin.grant.amountPlaceholder")}
            value={amount}
            onChange={(e) => setAmount(e.target.value)}
            autoFocus
          />
          <span className="field-hint">{t("admin.grant.amountHint")}</span>
        </div>
        <div className="field">
          <label className="field-label" htmlFor="grant-note">
            {t("admin.grant.noteLabel")}
          </label>
          <input
            id="grant-note"
            className="input"
            placeholder={t("admin.grant.notePlaceholder")}
            value={note}
            onChange={(e) => setNote(e.target.value)}
          />
        </div>
        {error && <div className="error-banner">{error}</div>}
        <div className="modal-actions">
          <button type="button" className="btn btn-ghost" onClick={onClose} disabled={busy}>
            {t("common.cancel")}
          </button>
          <button type="submit" className="btn btn-primary" disabled={busy || !valid}>
            {busy ? <span className="spinner" /> : null}
            {t("admin.grant.submit")}
          </button>
        </div>
      </form>
    </Modal>
  );
}

function UserLedgerDialog({ user, onClose }: { user: AdminUser; onClose: () => void }) {
  const { t, i18n } = useTranslation();
  const [ledger, setLedger] = useState<CreditEntry[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let active = true;
    getAdminUserCredits(user.id)
      .then((r) => {
        if (active) setLedger(r.ledger);
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
  }, [user.id, t]);

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
    <Modal title={t("admin.ledger.title", { name: user.email })} onClose={onClose} className="modal-wide">
      {loading && (
        <div className="picker-status">
          <span className="spinner" /> {t("credits.ledger.loading")}
        </div>
      )}
      {error && <div className="error-banner">{error}</div>}
      {!loading && !error && ledger && ledger.length === 0 && (
        <div className="empty-state">
          <strong>{t("credits.ledger.empty.title")}</strong>
          <span>{t("credits.ledger.empty.body")}</span>
        </div>
      )}
      {!loading && !error && ledger && ledger.length > 0 && (
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
      <div className="modal-actions">
        <button type="button" className="btn btn-ghost" onClick={onClose}>
          {t("common.close")}
        </button>
      </div>
    </Modal>
  );
}
