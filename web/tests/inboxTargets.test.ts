import { describe, it, expect } from 'vitest'
import {
  applySourceContext,
  fromDraft,
  toDraft,
  withSourceContext,
  type CaptureAction,
} from '../utils/inboxTargets'

const CAPTURE = 'amanhã:\n- adicionar recursos no projeto do substack\n- ler pdfs do plainenglish'

describe('withSourceContext', () => {
  it('carries the capture in under the fragment the action owns', () => {
    const out = withSourceContext('ler pdfs do plainenglish', CAPTURE, 'whatsapp · 4/1/26 21:08')
    expect(out).toContain('ler pdfs do plainenglish')
    expect(out).toContain('From the capture · whatsapp · 4/1/26 21:08')
    expect(out).toContain(CAPTURE)
  })

  // The drawer prefills the description with the composed text and posts it back
  // verbatim. Composing twice must be a no-op, or an edited task would carry the
  // capture two, three, four times over.
  it('is idempotent: recomposing an already-composed body changes nothing', () => {
    const once = withSourceContext('ler pdfs', CAPTURE, 'whatsapp')
    expect(withSourceContext(once, CAPTURE, 'whatsapp')).toBe(once)
  })

  it('does not repeat a capture the action already owns whole', () => {
    expect(withSourceContext(CAPTURE, CAPTURE)).toBe(CAPTURE)
    expect(withSourceContext('', CAPTURE)).toBe(CAPTURE)
  })
})

describe('applySourceContext', () => {
  it('touches tasks only — a note body IS the capture, a bookmark body is a URL', () => {
    const actions: CaptureAction[] = [
      { target: 'task', title: 'Read the PDFs', body: 'ler pdfs' },
      { target: 'note', title: 'A reflection', body: 'só um fragmento' },
      { target: 'bookmark', title: 'The Selfish Gene', body: 'https://example.com' },
    ]
    const [task, note, bookmark] = applySourceContext(actions, CAPTURE, 'whatsapp')
    expect(task.body).toContain(CAPTURE)
    expect(note.body).toBe('só um fragmento')
    expect(bookmark.body).toBe('https://example.com')
  })
})

describe('the draft round trip', () => {
  it('keeps a task’s project, deadline and tags, and drops the deadline off a note', () => {
    const draft = toDraft(
      { target: 'task', title: 'Read the PDFs', body: 'ler pdfs', projectId: 'p1', dueDate: '2026-04-02', tags: ['to-read'] },
      CAPTURE,
      'whatsapp',
    )
    expect(draft.tagsText).toBe('to-read')

    const task = fromDraft(draft)
    expect(task).toMatchObject({ target: 'task', projectId: 'p1', dueDate: '2026-04-02', tags: ['to-read'] })
    expect(task.body).toContain(CAPTURE)

    // Flip the target in the editor: the deadline belongs to a task, not a note.
    const note = fromDraft({ ...draft, target: 'note' })
    expect(note.dueDate).toBe('')
    expect(note.projectId).toBe('')
    expect(note.linkTo).toBe('p1')
  })

  it('reads tags off a comma-separated field, ignoring the gaps', () => {
    const draft = toDraft({ target: 'task', title: 'x', body: '' }, CAPTURE)
    draft.tagsText = ' to-read , , idea '
    expect(fromDraft(draft).tags).toEqual(['to-read', 'idea'])
  })
})
