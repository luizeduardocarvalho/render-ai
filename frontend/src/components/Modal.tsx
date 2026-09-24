import { useEffect, useId, useRef, type ReactNode } from "react";
import "./Modal.css";

interface ModalProps {
  titleId?: string;
  title: ReactNode;
  children: ReactNode;
  onClose: () => void;
  /** "dialog" for informational content, "alertdialog" for a confirmation. */
  role?: "dialog" | "alertdialog";
  /** Disables Esc/backdrop dismissal while true (e.g. an action is in flight). */
  busy?: boolean;
  className?: string;
}

/**
 * Minimal accessible modal shell: focus trapped inside, initial focus on the
 * first focusable element, Esc and backdrop-click both call `onClose` (unless
 * `busy`). Built for the app's small set of dialogs (confirmations, credit
 * ledger, admin forms) rather than as a general-purpose library component.
 */
export function Modal({ titleId, title, children, onClose, role = "dialog", busy, className }: ModalProps) {
  const generatedId = useId();
  const headingId = titleId ?? generatedId;
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    const focusable = () =>
      Array.from(
        container.querySelectorAll<HTMLElement>(
          'button:not(:disabled), [href], input:not(:disabled), select:not(:disabled), textarea:not(:disabled), [tabindex]:not([tabindex="-1"])',
        ),
      );

    const first = focusable()[0];
    first?.focus();

    function handleKeyDown(e: KeyboardEvent) {
      if (e.key === "Escape") {
        if (busy) return;
        e.stopPropagation();
        onClose();
        return;
      }
      if (e.key !== "Tab") return;
      const items = focusable();
      if (items.length === 0) return;
      const firstEl = items[0];
      const lastEl = items[items.length - 1];
      if (e.shiftKey && document.activeElement === firstEl) {
        e.preventDefault();
        lastEl.focus();
      } else if (!e.shiftKey && document.activeElement === lastEl) {
        e.preventDefault();
        firstEl.focus();
      }
    }

    document.addEventListener("keydown", handleKeyDown, true);
    return () => document.removeEventListener("keydown", handleKeyDown, true);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [busy]);

  return (
    <div
      className="modal-backdrop"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget && !busy) onClose();
      }}
    >
      <div
        ref={containerRef}
        className={`modal panel ${className ?? ""}`}
        role={role}
        aria-modal="true"
        aria-labelledby={headingId}
      >
        <div className="modal-header">
          <h2 className="modal-title" id={headingId}>
            {title}
          </h2>
        </div>
        <div className="modal-body">{children}</div>
      </div>
    </div>
  );
}
