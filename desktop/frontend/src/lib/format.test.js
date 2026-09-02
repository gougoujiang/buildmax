import { describe, expect, it } from 'vitest'
import { formatRunStatus } from './format.js'

const status = {
  context_tokens: 500,
  context_window: 1000,
  prompt_tokens: 100,
  completion_tokens: 20,
  total_prompt_tokens: 3334,
  total_completion_tokens: 998,
}

describe('formatRunStatus', () => {
  it('shows only the context share', () => {
    expect(formatRunStatus(status)).toBe('ctx: 50% (500/1k)')
  })

  it('reports an unknown context when no window is set', () => {
    expect(formatRunStatus({})).toBe('ctx: unknown')
  })

  it('drops the token breakdown from the bar', () => {
    expect(formatRunStatus(status)).not.toContain('tokens')
  })
})
