import { describe, expect, it } from 'vitest'
import {
  recentBookmarks,
  recentNotes,
  sidebarTasks,
  activeProjects,
} from '../utils/sidebarContent'

describe('sidebar content selection', () => {
  it('keeps only ordinary notes and sorts them by most recent update', () => {
    const notes = [
      { id: 'old', path: 'old.md', title: 'Old', type: 'note', updatedAt: '2026-07-01T10:00:00Z' },
      { id: 'project', path: 'projects/p.md', title: 'Project', type: 'project', updatedAt: '2026-08-02T10:00:00Z' },
      { id: 'new', path: 'new.md', title: 'New', type: 'note', updatedAt: '2026-08-01T10:00:00Z' },
    ]

    expect(recentNotes(notes, 2).map((note) => note.id)).toEqual(['new', 'old'])
  })

  it('sorts bookmarks by their latest meaningful timestamp', () => {
    const bookmarks = [
      { id: 'created', title: 'Created', url: '', createdAt: '2026-08-01T10:00:00Z', updatedAt: '' },
      { id: 'updated', title: 'Updated', url: '', createdAt: '2026-07-01T10:00:00Z', updatedAt: '2026-08-02T10:00:00Z' },
    ]

    expect(recentBookmarks(bookmarks, 2).map((bookmark) => bookmark.id)).toEqual(['updated', 'created'])
  })

  it('keeps active projects and orders them by update time', () => {
    const projects = [
      { id: 'paused', title: 'Paused', status: 'paused', updatedAt: '2026-08-03T10:00:00Z' },
      { id: 'older', title: 'Older', status: 'active', updatedAt: '2026-07-01T10:00:00Z' },
      { id: 'newer', title: 'Newer', status: 'active', updatedAt: '2026-08-02T10:00:00Z' },
    ]

    expect(activeProjects(projects, 2).map((project) => project.id)).toEqual(['newer', 'older'])
  })

  it('orders overdue, next-tagged, and soon-due open tasks without showing ordinary backlog', () => {
    const tasks = [
      { id: 'ordinary', title: 'Ordinary', status: 'todo', dueDate: '', tags: [], updatedAt: '2026-08-02T10:00:00Z' },
      { id: 'done', title: 'Done', status: 'done', dueDate: '2026-07-01', tags: ['next'], updatedAt: '' },
      { id: 'soon', title: 'Soon', status: 'todo', dueDate: '2026-08-05', tags: [], updatedAt: '' },
      { id: 'next', title: 'Next', status: 'in_progress', dueDate: '', tags: ['next'], updatedAt: '' },
      { id: 'overdue', title: 'Overdue', status: 'todo', dueDate: '2026-08-01', tags: [], updatedAt: '' },
      { id: 'later', title: 'Later', status: 'todo', dueDate: '2026-09-01', tags: [], updatedAt: '' },
    ]

    expect(sidebarTasks(tasks, new Date('2026-08-02T12:00:00Z'), 5).map((task) => task.id)).toEqual([
      'overdue',
      'next',
      'soon',
    ])
  })
})
