/**
 * copyText copies text to the clipboard, returning whether it succeeded.
 *
 * The async Clipboard API is tried first, but it is not always available — an
 * insecure origin, a browser that withholds it, or a rejected permission all
 * leave `navigator.clipboard` unusable, and the earlier code that assumed it
 * (and never caught a rejection) is why "Copy" silently did nothing. The
 * execCommand path is the fallback every browser still honours from inside a
 * click handler.
 */
export async function copyText(text: string): Promise<boolean> {
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text)
      return true
    }
  } catch {
    // fall through to the legacy path
  }
  return legacyCopy(text)
}

function legacyCopy(text: string): boolean {
  try {
    const area = document.createElement("textarea")
    area.value = text
    // Kept out of view and out of the layout, but selectable.
    area.style.position = "fixed"
    area.style.top = "-9999px"
    area.setAttribute("readonly", "")
    document.body.appendChild(area)
    area.select()
    const ok = document.execCommand("copy")
    document.body.removeChild(area)
    return ok
  } catch {
    return false
  }
}
