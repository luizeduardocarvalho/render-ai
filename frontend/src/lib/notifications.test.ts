import type { TFunction } from "i18next";
import { describe, expect, it } from "vitest";
import type { JobNotification } from "../types";
import { alertTitle, isActive, isUnread, justFinished, notificationPath } from "./notifications";

function job(over: Partial<JobNotification> = {}): JobNotification {
  return {
    projectId: "p",
    projectName: "Casa",
    viewId: "v",
    viewName: "Fachada",
    jobId: "j",
    kind: "render",
    status: "done",
    createdAt: "2026-09-25T12:00:00Z",
    updatedAt: "2026-09-25T12:00:00Z",
    variations: { done: 1, failed: 0, total: 1 },
    seen: false,
    ...over,
  };
}

describe("isActive / isUnread", () => {
  it("counts queued and running as active, never as unread", () => {
    for (const status of ["queued", "running"] as const) {
      expect(isActive(job({ status }))).toBe(true);
      expect(isUnread(job({ status }))).toBe(false);
    }
  });

  it("counts a finished job as unread until it is seen", () => {
    for (const status of ["done", "failed"] as const) {
      expect(isActive(job({ status }))).toBe(false);
      expect(isUnread(job({ status }))).toBe(true);
      expect(isUnread(job({ status, seen: true }))).toBe(false);
    }
  });
});

describe("notificationPath", () => {
  it("opens the render a finished job made", () => {
    expect(notificationPath(job({ renderId: "r1" }))).toBe("/projects/p/views/v/renders/r1");
  });

  it("opens the view for a job still running, even one with a first render already", () => {
    expect(notificationPath(job({ status: "running", renderId: "r1" }))).toBe("/projects/p/views/v");
  });

  it("opens the view for a failed job", () => {
    expect(notificationPath(job({ status: "failed" }))).toBe("/projects/p/views/v");
  });
});

describe("justFinished", () => {
  const known = (...items: JobNotification[]) => new Map(items.map((n) => [n.jobId, n]));

  it("finds jobs that went from running to done or failed", () => {
    const before = known(job({ jobId: "a", status: "running" }), job({ jobId: "b", status: "queued" }), job({ jobId: "c", status: "running" }));
    const after = [job({ jobId: "a", status: "done" }), job({ jobId: "b", status: "failed" }), job({ jobId: "c", status: "running" })];
    expect(justFinished(before, after).map((n) => n.jobId)).toEqual(["a", "b"]);
  });

  it("ignores a job first seen already finished (the app just opened)", () => {
    expect(justFinished(known(), [job({ jobId: "a", status: "done" })])).toEqual([]);
  });

  it("ignores a job that was already finished", () => {
    expect(justFinished(known(job({ jobId: "a", status: "done" })), [job({ jobId: "a", status: "done" })])).toEqual([]);
  });

  it("ignores a job that is new and still running", () => {
    expect(justFinished(known(), [job({ jobId: "a", status: "running" })])).toEqual([]);
  });
});

describe("alertTitle", () => {
  // A stand-in for i18next: the key, plus the kind it was given.
  const t = ((key: string, opts?: { kind?: string }) => (opts?.kind ? `${key}(${opts.kind})` : key)) as unknown as TFunction;

  it("names what finished, per kind", () => {
    expect(alertTitle(t, job({ kind: "edit" }))).toBe("notifications.alert.done.edit");
    expect(alertTitle(t, job({ kind: "upscale" }))).toBe("notifications.alert.done.upscale");
  });

  it("names what failed with the kind's own label", () => {
    expect(alertTitle(t, job({ kind: "render", status: "failed" }))).toBe("notifications.alert.failed(notifications.kind.render)");
  });
});
