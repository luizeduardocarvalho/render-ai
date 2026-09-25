import { describe, expect, it } from "vitest";
import { appPath, parseAppPath } from "./routes";

describe("parseAppPath", () => {
  it("reads the project list as no project", () => {
    expect(parseAppPath("/")).toEqual({ pid: null, vid: null, rid: null });
  });

  it("reads a project, a view and a render", () => {
    expect(parseAppPath("/projects/p1")).toEqual({ pid: "p1", vid: null, rid: null });
    expect(parseAppPath("/projects/p1/views/v1")).toEqual({ pid: "p1", vid: "v1", rid: null });
    expect(parseAppPath("/projects/p1/views/v1/renders/r1")).toEqual({ pid: "p1", vid: "v1", rid: "r1" });
  });

  it("accepts a trailing slash", () => {
    expect(parseAppPath("/projects/p1/views/v1/")).toEqual({ pid: "p1", vid: "v1", rid: null });
  });

  it("does not read a render without its view, or anything else, as a project", () => {
    for (const path of ["/projects", "/projects/", "/projects/p1/renders/r1", "/projects/p1/views/v1/renders", "/admin", "/foo/projects/p1"]) {
      expect(parseAppPath(path), path).toEqual({ pid: null, vid: null, rid: null });
    }
  });

  it("keeps a malformed escape as typed instead of throwing", () => {
    expect(parseAppPath("/projects/%zz").pid).toBe("%zz");
  });
});

describe("appPath", () => {
  it("builds each level", () => {
    expect(appPath("p1")).toBe("/projects/p1");
    expect(appPath("p1", "v1")).toBe("/projects/p1/views/v1");
    expect(appPath("p1", "v1", "r1")).toBe("/projects/p1/views/v1/renders/r1");
  });

  it("leaves a render out when there is no view to put it under", () => {
    expect(appPath("p1", null, "r1")).toBe("/projects/p1");
  });

  it("round-trips ids that need escaping", () => {
    const path = appPath("a/b", "c d", "e?f");
    expect(parseAppPath(path)).toEqual({ pid: "a/b", vid: "c d", rid: "e?f" });
  });
});
