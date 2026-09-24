package store

import "log"

// logStoreErr logs a non-fatal storage error. The cloud stores (GCS,
// Firestore) use best-effort semantics for cleanup and writes to match the
// in-memory store's interface, which returns no error from those paths; a
// failure is logged here rather than surfaced.
func logStoreErr(format string, args ...any) {
	log.Printf(format, args...)
}
