import { useCallback, useEffect, useRef, useState } from 'react';

const FIRST_MIN = 160;
const SECOND_MIN = 200;

function readWidth(key, fallback) {
  try {
    const v = Number(localStorage.getItem(key));
    if (Number.isFinite(v) && v >= FIRST_MIN) return v;
  } catch {
    /* storage may be unavailable */
  }
  return fallback;
}

// usePaneResize drives a draggable divider for a two-pane split: the first pane
// gets an explicit width the caller applies as a CSS variable, the second pane
// fills the rest. Used by the file browser and the diff view in their expanded,
// side-by-side layout. Attach `ref` to the split container, `onMouseDown` to the
// divider, and feed `width` into the container's grid template. Width persists
// per machine.
export function usePaneResize(storageKey, defaultWidth = 260) {
  const [width, setWidth] = useState(() => readWidth(storageKey, defaultWidth));
  const ref = useRef(null);

  useEffect(() => {
    try { localStorage.setItem(storageKey, String(width)); } catch { /* ignore */ }
  }, [storageKey, width]);

  const onMouseDown = useCallback((e) => {
    e.preventDefault();
    const rect = ref.current?.getBoundingClientRect();
    if (!rect) return;
    const max = Math.max(FIRST_MIN, rect.width - SECOND_MIN);
    const move = (ev) => setWidth(Math.min(Math.max(FIRST_MIN, ev.clientX - rect.left), max));
    const up = () => {
      window.removeEventListener('mousemove', move);
      window.removeEventListener('mouseup', up);
      document.body.style.userSelect = '';
    };
    document.body.style.userSelect = 'none';
    window.addEventListener('mousemove', move);
    window.addEventListener('mouseup', up);
  }, []);

  return { width, ref, onMouseDown };
}
