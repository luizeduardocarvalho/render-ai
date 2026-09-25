/**
 * Where the user is in the app, kept in the URL so a link (a notification, a
 * reload, the back button) lands on the same project, view and render:
 *
 *   /                                            the project list
 *   /projects/:pid                               a project
 *   /projects/:pid/views/:vid                    one of its views
 *   /projects/:pid/views/:vid/renders/:rid       one of that view's renders
 */
export interface AppRoute {
  pid: string | null;
  vid: string | null;
  rid: string | null;
}

const APP_PATH = /^\/projects\/([^/]+)(?:\/views\/([^/]+)(?:\/renders\/([^/]+))?)?\/?$/;

// A malformed escape ("%zz") is kept as typed: it names nothing, so the
// project or view lookup fails cleanly instead of this throwing.
function decode(segment: string | undefined): string | null {
  if (!segment) return null;
  try {
    return decodeURIComponent(segment);
  } catch {
    return segment;
  }
}

export function parseAppPath(pathname: string): AppRoute {
  const m = APP_PATH.exec(pathname);
  if (!m) return { pid: null, vid: null, rid: null };
  return { pid: decode(m[1]), vid: decode(m[2]), rid: decode(m[3]) };
}

export function appPath(pid: string, vid?: string | null, rid?: string | null): string {
  let path = `/projects/${encodeURIComponent(pid)}`;
  if (vid) {
    path += `/views/${encodeURIComponent(vid)}`;
    if (rid) path += `/renders/${encodeURIComponent(rid)}`;
  }
  return path;
}
