import { useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Modal } from "./Modal";

interface ConfirmDialogProps {
  title: ReactNode;
  body?: ReactNode;
  confirmLabel?: string;
  cancelLabel?: string;
  /** Runs on confirm; a thrown error is shown inline and the dialog stays open. */
  onConfirm: () => Promise<void> | void;
  onCancel: () => void;
  /** Solid danger styling for the confirm button. Defaults to true - every current use is a delete. */
  danger?: boolean;
}

/**
 * Reusable accessible confirmation modal, replacing window.confirm across the
 * app. role="alertdialog", Esc/backdrop cancel, focus starts on Cancel (the
 * safer default for a destructive action), shows a spinner + disables both
 * buttons while the action runs, and surfaces a failure inline without closing.
 */
export function ConfirmDialog({
  title,
  body,
  confirmLabel,
  cancelLabel,
  onConfirm,
  onCancel,
  danger = true,
}: ConfirmDialogProps) {
  const { t } = useTranslation();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function handleConfirm() {
    setBusy(true);
    setError(null);
    try {
      await onConfirm();
    } catch (err) {
      setError(err instanceof Error ? err.message : t("confirmDialog.error"));
      setBusy(false);
    }
  }

  return (
    <Modal title={title} onClose={onCancel} role="alertdialog" busy={busy}>
      {body && <div className="confirm-dialog-body">{body}</div>}
      {error && <div className="error-banner">{error}</div>}
      <div className="modal-actions">
        <button type="button" className="btn btn-ghost" onClick={onCancel} disabled={busy}>
          {cancelLabel ?? t("confirmDialog.cancel")}
        </button>
        <button
          type="button"
          className={`btn ${danger ? "btn-danger-solid" : "btn-primary"}`}
          onClick={() => void handleConfirm()}
          disabled={busy}
        >
          {busy ? <span className="spinner" /> : null}
          {confirmLabel ?? t("confirmDialog.confirm")}
        </button>
      </div>
    </Modal>
  );
}
