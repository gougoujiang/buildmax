import { describe, expect, it } from 'vitest';
import { FORK, REWIND, consequenceText, moveReport, pointLabel, visiblePoints } from './HistoryModal';

const head = { item_id: 'i3', role: 'assistant', content: 'you are welcome', is_head: true, messages: 0 };
const afterTool = { item_id: 'i2', role: 'user', content: 'thanks', messages: 2 };
const beforeTool = {
  item_id: 'i1', role: 'user', content: 'write notes.txt', messages: 4,
  tools: [{ name: 'Write', interrupted: false }],
};

describe('visiblePoints', () => {
  // Rewinding to where you already are is not a move; forking from there is
  // the common case.
  it('offers the head to fork but not to rewind', () => {
    expect(visiblePoints([beforeTool, afterTool, head], REWIND)).toEqual([beforeTool, afterTool]);
    expect(visiblePoints([beforeTool, afterTool, head], FORK)).toHaveLength(3);
  });

  it('survives a session with no points', () => {
    expect(visiblePoints(null, REWIND)).toEqual([]);
  });
});

describe('pointLabel', () => {
  it('does not call a background event the user words', () => {
    expect(pointLabel({ role: 'user' })).toBe('you');
    expect(pointLabel({ role: 'event' })).toBe('event');
    expect(pointLabel({ role: 'assistant' })).toBe('agent');
  });
});

describe('consequenceText', () => {
  it('counts what a rewind drops and names what it leaves behind', () => {
    expect(consequenceText(beforeTool, REWIND)).toBe('drops 4 messages · leaves in place: Write');
    expect(consequenceText(afterTool, REWIND)).toBe('drops 2 messages · nothing outside the conversation ran');
  });

  // A fork takes nothing from the original, so counting losses would be false.
  // The same span still matters, from the other side: the copy will not know
  // that work happened.
  it('frames a fork as what the new session will not know, never as loss', () => {
    const got = consequenceText(beforeTool, FORK);
    expect(got).toBe('new session starts here · will not know about: Write');
    expect(got).not.toContain('drops');
  });

  it('says a fork from the head simply copies the conversation', () => {
    expect(consequenceText(head, FORK)).toBe('copies this conversation up to here into a new session');
  });

  it('marks a tool that never reported back', () => {
    const interrupted = { messages: 1, tools: [{ name: 'Bash', interrupted: true }] };
    expect(consequenceText(interrupted, REWIND)).toContain('Bash (interrupted)');
  });
});

describe('moveReport', () => {
  it('warns after a rewind that the effects are still in place', () => {
    const got = moveReport({ tools: [{ name: 'Write' }] }, REWIND);
    expect(got).toContain('Write');
    expect(got).toContain('does not undo');
  });

  // The rewind wording would be wrong here: nothing was undone or dropped.
  it('says after a fork that the original is unchanged', () => {
    const got = moveReport({ tools: [{ name: 'Write' }] }, FORK);
    expect(got).toContain('unchanged');
    expect(got).toContain('Write');
    expect(got).not.toContain('does not undo');
  });

  it('says plainly when nothing outside the conversation ran', () => {
    expect(moveReport({}, REWIND)).toContain('nothing left over');
    expect(moveReport({}, FORK)).toContain('unchanged');
  });
});
