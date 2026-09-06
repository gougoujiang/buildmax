// Offset paging math for the admin list pages, kept out of the component so the
// off-by-one that a "1–50 of N" label invites is a unit test rather than a bug
// someone finds on page two.

export interface PageWindow {
  /** 1-based index of the first row shown, or 0 when the page is empty. */
  from: number
  /** 1-based index of the last row shown. */
  to: number
  hasPrev: boolean
  hasNext: boolean
  /** The offset the Previous control should load, never below zero. */
  prevOffset: number
  /** The offset the Next control should load. */
  nextOffset: number
}

export function pageWindow(offset: number, pageSize: number, total: number): PageWindow {
  const start = Math.max(0, offset)
  const shown = Math.max(0, Math.min(pageSize, total - start))
  return {
    from: shown === 0 ? 0 : start + 1,
    to: start + shown,
    hasPrev: start > 0,
    hasNext: start + pageSize < total,
    prevOffset: Math.max(0, start - pageSize),
    nextOffset: start + pageSize,
  }
}
