import { describe, expect, it } from 'vitest';
import { planSummary, pluginSummary } from './Modals';

describe('pluginSummary', () => {
  it('says where a Marketplace copy came from and what loaded', () => {
    expect(pluginSummary({
      source: 'marketplace', version: '1.2.0', skills: ['review'], hooks: ['a', 'b'],
    })).toBe('marketplace · 1.2.0 · 1 skill · 2 hooks');
  });

  // A checkout is identified by its commit, and a dirty tree is not the commit
  // it names.
  it('identifies a checkout by commit, marking a dirty tree', () => {
    expect(pluginSummary({
      source: 'repository', commit: '0123456789abcdef0123', dirty: true, skills: ['review'],
    })).toBe('repository · 0123456789ab (dirty) · 1 skill');
  });

  // Half-loaded must not read as fully active.
  it('names what a higher layer overrode', () => {
    expect(pluginSummary({
      source: 'local', skills: ['review'], shadowed: ['review (overridden by workspace)'],
    })).toBe('local · 1 skill · 1 overridden');
  });

  it('says nothing extra for a plugin that contributes nothing', () => {
    expect(pluginSummary({ source: 'local' })).toBe('local');
  });
});

describe('planSummary', () => {
  it('counts what installing would add', () => {
    expect(planSummary({ skills: ['a'], subagents: ['b', 'c'] })).toBe('1 skill, 2 subagents');
  });

  it('names an empty release rather than showing nothing', () => {
    expect(planSummary({})).toBe('contributes nothing this build recognises');
  });
});
