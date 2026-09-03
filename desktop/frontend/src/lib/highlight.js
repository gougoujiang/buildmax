// Syntax highlighting for the file browser and diff panel, backed by Shiki.
// Two choices keep this from shipping more than it needs to inside the
// Wails-embedded binary:
//  - the pure-JS regex engine (no WASM) instead of the default oniguruma one.
//  - a fine-grained core bundle (shiki/core + per-language imports below)
//    instead of the top-level `shiki` package, which bundles a lazy chunk for
//    every language Shiki knows about (~150) regardless of which ones this
//    app ever requests.

// Dynamic-import thunks, one per supported language, so Rollup only emits a
// chunk for languages actually listed here instead of Shiki's full catalog.
const LANG_LOADERS = {
  go: () => import('shiki/langs/go.mjs'),
  typescript: () => import('shiki/langs/typescript.mjs'),
  tsx: () => import('shiki/langs/tsx.mjs'),
  jsx: () => import('shiki/langs/jsx.mjs'),
  javascript: () => import('shiki/langs/javascript.mjs'),
  json: () => import('shiki/langs/json.mjs'),
  jsonc: () => import('shiki/langs/jsonc.mjs'),
  yaml: () => import('shiki/langs/yaml.mjs'),
  toml: () => import('shiki/langs/toml.mjs'),
  markdown: () => import('shiki/langs/markdown.mjs'),
  html: () => import('shiki/langs/html.mjs'),
  css: () => import('shiki/langs/css.mjs'),
  scss: () => import('shiki/langs/scss.mjs'),
  less: () => import('shiki/langs/less.mjs'),
  python: () => import('shiki/langs/python.mjs'),
  ruby: () => import('shiki/langs/ruby.mjs'),
  rust: () => import('shiki/langs/rust.mjs'),
  java: () => import('shiki/langs/java.mjs'),
  kotlin: () => import('shiki/langs/kotlin.mjs'),
  c: () => import('shiki/langs/c.mjs'),
  cpp: () => import('shiki/langs/cpp.mjs'),
  csharp: () => import('shiki/langs/csharp.mjs'),
  php: () => import('shiki/langs/php.mjs'),
  shellscript: () => import('shiki/langs/shellscript.mjs'),
  sql: () => import('shiki/langs/sql.mjs'),
  xml: () => import('shiki/langs/xml.mjs'),
  proto: () => import('shiki/langs/proto.mjs'),
  dockerfile: () => import('shiki/langs/dockerfile.mjs'),
  ini: () => import('shiki/langs/ini.mjs'),
  graphql: () => import('shiki/langs/graphql.mjs'),
  swift: () => import('shiki/langs/swift.mjs'),
  lua: () => import('shiki/langs/lua.mjs'),
  makefile: () => import('shiki/langs/makefile.mjs'),
  diff: () => import('shiki/langs/diff.mjs'),
};

const EXT_TO_LANG = {
  go: 'go', mod: 'go', sum: 'go',
  ts: 'typescript', mts: 'typescript', cts: 'typescript',
  tsx: 'tsx', jsx: 'jsx',
  js: 'javascript', mjs: 'javascript', cjs: 'javascript',
  json: 'json', jsonc: 'jsonc',
  yaml: 'yaml', yml: 'yaml',
  toml: 'toml',
  md: 'markdown', markdown: 'markdown',
  html: 'html', htm: 'html',
  css: 'css', scss: 'scss', less: 'less',
  py: 'python',
  rb: 'ruby',
  rs: 'rust',
  java: 'java',
  kt: 'kotlin', kts: 'kotlin',
  c: 'c', h: 'c',
  cpp: 'cpp', cc: 'cpp', cxx: 'cpp', hpp: 'cpp',
  cs: 'csharp',
  php: 'php',
  sh: 'shellscript', bash: 'shellscript', zsh: 'shellscript',
  sql: 'sql',
  xml: 'xml',
  proto: 'proto',
  ini: 'ini', cfg: 'ini',
  graphql: 'graphql', gql: 'graphql',
  swift: 'swift',
  lua: 'lua',
  diff: 'diff', patch: 'diff',
};

const BASENAME_TO_LANG = {
  dockerfile: 'dockerfile',
  makefile: 'makefile',
};

export function langForPath(path) {
  const base = String(path ?? '').split(/[/\\]/).pop() ?? '';
  const baseLang = BASENAME_TO_LANG[base.toLowerCase()];
  if (baseLang) return baseLang;
  const dot = base.lastIndexOf('.');
  if (dot <= 0) return 'plaintext';
  const ext = base.slice(dot + 1).toLowerCase();
  return EXT_TO_LANG[ext] ?? 'plaintext';
}

function themeId(theme) {
  return theme === 'dark' ? 'github-dark' : 'github-light';
}

let highlighterPromise = null;

function getHighlighter() {
  if (!highlighterPromise) {
    highlighterPromise = Promise.all([
      import('shiki/core'),
      import('shiki/engine/javascript'),
    ]).then(([{ createHighlighterCore }, { createJavaScriptRegexEngine }]) =>
      createHighlighterCore({
        themes: [import('shiki/themes/github-light.mjs'), import('shiki/themes/github-dark.mjs')],
        langs: [],
        engine: createJavaScriptRegexEngine(),
      })
    );
  }
  return highlighterPromise;
}

// Loads `lang` on first use, falling back to 'plaintext' if it can't be
// loaded (unsupported id, or any other Shiki error).
async function ensureLang(highlighter, lang) {
  if (lang === 'plaintext' || highlighter.getLoadedLanguages().includes(lang)) return lang;
  const load = LANG_LOADERS[lang];
  if (!load) return 'plaintext';
  try {
    await highlighter.loadLanguage(load());
    return lang;
  } catch {
    return 'plaintext';
  }
}

export async function highlightToHtml(code, path, theme) {
  const highlighter = await getHighlighter();
  const lang = await ensureLang(highlighter, langForPath(path));
  return highlighter.codeToHtml(code, { lang, theme: themeId(theme) });
}

export async function highlightToLines(code, path, theme) {
  const highlighter = await getHighlighter();
  const lang = await ensureLang(highlighter, langForPath(path));
  return highlighter.codeToTokens(code, { lang, theme: themeId(theme) }).tokens;
}
