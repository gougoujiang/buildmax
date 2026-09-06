import { useEffect, useMemo, useRef, useState } from "react"
import Markdown from "react-markdown"
import remarkGfm from "remark-gfm"
import type { Components } from "react-markdown"
import { navigate } from "../../router"

/**
 * Help renders the end-user manual that ships inside the portal image. The pages
 * are plain markdown files under /help, mirrored from the repository-root help/
 * directory at build time (portal/scripts/sync-help.mjs). This page fetches
 * /help/manifest.json for the table of contents and /help/<slug>.md for a page,
 * so adding documentation never touches the portal bundle — only the markdown.
 */

interface HelpPage {
  slug: string
  title: string
}

interface HelpSection {
  title: string
  pages: HelpPage[]
}

interface HelpManifest {
  title: string
  sections: HelpSection[]
}

/** A bare `slug.md` (optionally `./slug.md#anchor`) is a link to another page. */
const INTERNAL_DOC = /^(?:\.\/)?([a-z0-9-]+)\.md(?:[#?].*)?$/i

function internalSlug(href: string): string | null {
  const match = href.match(INTERNAL_DOC)
  return match ? match[1] : null
}

export function Help({ slug }: { slug?: string }) {
  const [manifest, setManifest] = useState<HelpManifest | null>(null)
  const [manifestError, setManifestError] = useState(false)

  useEffect(() => {
    let alive = true
    fetch("/help/manifest.json")
      .then((r) => {
        if (!r.ok) throw new Error(`manifest ${r.status}`)
        return r.json()
      })
      .then((m: HelpManifest) => alive && setManifest(m))
      .catch(() => alive && setManifestError(true))
    return () => {
      alive = false
    }
  }, [])

  const pages = useMemo(
    () => manifest?.sections.flatMap((s) => s.pages) ?? [],
    [manifest],
  )

  // The manual opens on its first page; a slug in the URL selects one directly.
  const activeSlug = slug ?? pages[0]?.slug
  const known = activeSlug ? pages.some((p) => p.slug === activeSlug) : true

  const [content, setContent] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [notFound, setNotFound] = useState(false)
  const contentRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!activeSlug || !known) return
    let alive = true
    setLoading(true)
    setNotFound(false)
    setContent(null)
    fetch(`/help/${activeSlug}.md`)
      .then((r) => {
        if (!r.ok) throw new Error(String(r.status))
        return r.text()
      })
      .then((t) => alive && setContent(t))
      .catch(() => alive && setNotFound(true))
      .finally(() => alive && setLoading(false))
    return () => {
      alive = false
    }
  }, [activeSlug, known])

  // A new page starts at its top, not wherever the previous one was scrolled.
  useEffect(() => {
    contentRef.current?.scrollTo({ top: 0 })
  }, [activeSlug])

  const components: Components = useMemo(
    () => ({
      a(props) {
        const { href, children } = props
        const target = href ? internalSlug(href) : null
        if (target) {
          return (
            <a
              href={`#/help/${target}`}
              onClick={(e) => {
                e.preventDefault()
                navigate({ name: "help", slug: target })
              }}
            >
              {children}
            </a>
          )
        }
        // Everything else is an external link or an in-page anchor.
        const external = href?.startsWith("http")
        return (
          <a
            href={href}
            target={external ? "_blank" : undefined}
            rel={external ? "noreferrer noopener" : undefined}
          >
            {children}
          </a>
        )
      },
    }),
    [],
  )

  if (manifestError) {
    return (
      <div className="help">
        <p className="help__error" role="alert">
          The help manual could not be loaded.
        </p>
      </div>
    )
  }

  return (
    <div className="help">
      <nav className="help__nav" aria-label="Help contents">
        <p className="help__nav-title">{manifest?.title ?? "Help"}</p>
        {manifest?.sections.map((section) => (
          <div key={section.title} className="help__nav-section">
            <p className="help__nav-heading">{section.title}</p>
            <ul className="help__nav-list">
              {section.pages.map((page) => (
                <li key={page.slug}>
                  <button
                    type="button"
                    className={`help__nav-link ${
                      page.slug === activeSlug ? "help__nav-link--active" : ""
                    }`}
                    aria-current={page.slug === activeSlug ? "page" : undefined}
                    onClick={() => navigate({ name: "help", slug: page.slug })}
                  >
                    {page.title}
                  </button>
                </li>
              ))}
            </ul>
          </div>
        ))}
      </nav>

      <div className="help__content" ref={contentRef}>
        {!known ? (
          <div className="help__empty">
            <p className="help__empty-title">Page not found</p>
            <button
              type="button"
              className="help__empty-link"
              onClick={() => navigate({ name: "help" })}
            >
              Back to the start of the manual
            </button>
          </div>
        ) : notFound ? (
          <p className="help__error" role="alert">
            This help page could not be loaded.
          </p>
        ) : loading || content === null ? (
          <div className="help__skeleton" aria-hidden>
            <div className="help__skeleton-line help__skeleton-line--title" />
            <div className="help__skeleton-line" />
            <div className="help__skeleton-line" />
            <div className="help__skeleton-line help__skeleton-line--short" />
          </div>
        ) : (
          <article className="help__article markdown">
            <Markdown remarkPlugins={[remarkGfm]} components={components}>
              {content}
            </Markdown>
          </article>
        )}
      </div>
    </div>
  )
}
