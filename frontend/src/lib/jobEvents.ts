/**
 * Lets the code that starts and follows a render tell the notification bell
 * that its list of jobs has changed, without the two knowing about each other.
 */
const bus = new EventTarget();
const CHANGED = "render-jobs-changed";

/** A render, edit or upscale job was just started, or has just finished. */
export function announceJobsChanged(): void {
  bus.dispatchEvent(new Event(CHANGED));
}

/** Calls fn on every announceJobsChanged; returns the function to stop. */
export function onJobsChanged(fn: () => void): () => void {
  bus.addEventListener(CHANGED, fn);
  return () => bus.removeEventListener(CHANGED, fn);
}
