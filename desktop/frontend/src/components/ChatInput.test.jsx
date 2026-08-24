import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { ChatInput } from './ChatInput';

afterEach(cleanup);

// ChatInput reaches for the Wails runtime on mount. There is none in a test, and
// the real module already degrades to a no-op — this only keeps that explicit.
vi.mock('../lib/wailsRuntime', () => ({
  EventsOn: () => () => {},
  EventsOff: () => {},
}));

// The project and app bindings ChatInput loads its status bar from. Every call
// resolves empty: none of it is what these tests are about.
const app = {
  ListJobs: () => Promise.resolve([]),
  GetSlashModels: () => Promise.resolve({ models: [], current: '' }),
  GetSlashSkills: () => Promise.resolve({ skills: [] }),
  GetGitBranch: () => Promise.resolve(''),
};

function renderInput(props = {}) {
  return render(
    <ChatInput
      onSend={() => {}}
      onCancel={() => {}}
      loading={false}
      error={null}
      onDismissError={() => {}}
      currentProject={{ id: 'p1' }}
      app={app}
      approvalRequest={null}
      onRespond={() => {}}
      toolActivity=""
      runStatus={null}
      sessionId="s1"
      {...props}
    />,
  );
}

function composer() {
  return screen.getByLabelText('Message');
}

function pressTab() {
  fireEvent(
    composer(),
    new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true }),
  );
}

describe('ChatInput suggestion', () => {
  it('accepts the suggestion into the box rather than sending it', () => {
    const onSend = vi.fn();
    const onAcceptSuggestion = vi.fn();
    renderInput({ suggestion: 'yes, commit it', onSend, onAcceptSuggestion });

    expect(composer().placeholder).toBe('yes, commit it');
    pressTab();

    expect(composer().value).toBe('yes, commit it');
    expect(onAcceptSuggestion).toHaveBeenCalledOnce();
    // Accepting fills the box. Sending is still the user's keystroke.
    expect(onSend).not.toHaveBeenCalled();
  });

  it('sends the accepted suggestion on the next Enter', () => {
    const onSend = vi.fn();
    renderInput({ suggestion: 'yes, commit it', onSend, onAcceptSuggestion: () => {} });

    pressTab();
    fireEvent.keyDown(composer(), { key: 'Enter' });
    expect(onSend).toHaveBeenCalledWith('yes, commit it');
  });

  it('offers nothing when the turn produced no suggestion', () => {
    renderInput({ suggestion: '', onAcceptSuggestion: () => {} });
    pressTab();
    expect(composer().value).toBe('');
  });
});
