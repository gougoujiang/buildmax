import { describe, expect, it } from 'vitest'
import { langForPath } from './highlight.js'

describe('langForPath', () => {
  it('maps common extensions to Shiki language ids', () => {
    expect(langForPath('main.go')).toBe('go')
    expect(langForPath('src/App.tsx')).toBe('tsx')
    expect(langForPath('README.md')).toBe('markdown')
    expect(langForPath('config.yaml')).toBe('yaml')
    expect(langForPath('script.sh')).toBe('shellscript')
  })

  it('matches basenames without an extension', () => {
    expect(langForPath('Dockerfile')).toBe('dockerfile')
    expect(langForPath('deployment/Makefile')).toBe('makefile')
  })

  it('falls back to plaintext for unknown or missing extensions', () => {
    expect(langForPath('LICENSE')).toBe('plaintext')
    expect(langForPath('file.unknownext')).toBe('plaintext')
    expect(langForPath('')).toBe('plaintext')
  })
})
