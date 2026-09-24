import { useEffect, useState } from "react";
import { fetchSignedImageUrl } from "../api";
import { useProject } from "../state/ProjectContext";

interface Options {
  /** Bump above 0 to force a re-sign (e.g. after the blob changed). */
  nonce?: number;
  /**
   * Owning project id. Defaults to the currently open project. Pass explicitly
   * to load a blob from a project that is not open yet (e.g. picker thumbnails).
   */
  projectId?: string;
}

/**
 * Resolves a loadable URL for an image blob, for use as an <img src>. Returns
 * undefined while resolving or when there is no image/project.
 */
export function useSignedImageUrl(
  imageId: string | null | undefined,
  opts: Options = {},
): string | undefined {
  const { project } = useProject();
  const pid = opts.projectId ?? project?.id;
  const nonce = opts.nonce;
  const [url, setUrl] = useState<string>();

  useEffect(() => {
    if (!pid || !imageId) {
      setUrl(undefined);
      return;
    }
    let active = true;
    fetchSignedImageUrl(pid, imageId, nonce !== undefined && nonce > 0)
      .then((u) => {
        if (active) setUrl(u);
      })
      .catch(() => {
        if (active) setUrl(undefined);
      });
    return () => {
      active = false;
    };
  }, [pid, imageId, nonce]);

  return url;
}
