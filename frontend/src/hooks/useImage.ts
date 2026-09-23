import { useEffect, useState } from "react";

interface ImageState {
  image: HTMLImageElement | null;
  loading: boolean;
  error: boolean;
}

/**
 * Loads an HTMLImageElement for a URL. Uses crossOrigin="anonymous" so the
 * resulting element can be drawn into a <canvas> and read back with
 * getImageData without tainting the canvas (required for mask bitmap import).
 */
export function useImage(src: string | null): ImageState {
  const [state, setState] = useState<ImageState>({ image: null, loading: !!src, error: false });

  useEffect(() => {
    if (!src) {
      setState({ image: null, loading: false, error: false });
      return;
    }
    let cancelled = false;
    setState({ image: null, loading: true, error: false });
    const img = new window.Image();
    img.crossOrigin = "anonymous";
    img.onload = () => {
      if (!cancelled) setState({ image: img, loading: false, error: false });
    };
    img.onerror = () => {
      if (!cancelled) setState({ image: null, loading: false, error: true });
    };
    img.src = src;
    return () => {
      cancelled = true;
    };
  }, [src]);

  return state;
}
