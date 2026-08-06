export interface SidebarNote {
  id: string
  path: string
  title: string
  type: string
  updatedAt: string
}

export interface SidebarBookmark {
  id: string
  title: string
  url: string
  createdAt: string
  updatedAt: string
}

export interface SidebarProject {
  id: string
  title: string
  status: string
  updatedAt: string
}

export interface SidebarTask {
  id: string
  title: string
  status: string
  dueDate: string
  tags: string[]
  updatedAt: string
}

function timestamp(value: string): number {
  const parsed = Date.parse(value)
  return Number.isNaN(parsed) ? 0 : parsed
}

function recentFirst<T>(items: readonly T[], dateOf: (item: T) => string): T[] {
  return [...items].sort((left, right) => timestamp(dateOf(right)) - timestamp(dateOf(left)))
}

export function recentNotes(notes: readonly SidebarNote[], limit: number): SidebarNote[] {
  return recentFirst(notes.filter((note) => note.type === 'note'), (note) => note.updatedAt).slice(0, limit)
}

export function recentBookmarks(
  bookmarks: readonly SidebarBookmark[],
  limit: number,
): SidebarBookmark[] {
  return recentFirst(bookmarks, (bookmark) => bookmark.updatedAt || bookmark.createdAt).slice(0, limit)
}

export function activeProjects(
  projects: readonly SidebarProject[],
  limit: number,
): SidebarProject[] {
  return recentFirst(projects.filter((project) => project.status === 'active'), (project) => project.updatedAt).slice(0, limit)
}

function calendarDay(date: Date): string {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

function addDays(date: Date, days: number): Date {
  const copy = new Date(date)
  copy.setDate(copy.getDate() + days)
  return copy
}

function taskRank(task: SidebarTask, today: string, soon: string): number {
  if (task.dueDate && task.dueDate < today) return 0
  if (task.tags.includes('next')) return 1
  if (task.dueDate && task.dueDate <= soon) return 2
  return 3
}

export function sidebarTasks(
  tasks: readonly SidebarTask[],
  now: Date,
  limit: number,
): SidebarTask[] {
  const today = calendarDay(now)
  const soon = calendarDay(addDays(now, 7))

  return tasks
    .filter((task) => task.status !== 'done' && taskRank(task, today, soon) < 3)
    .sort((left, right) => {
      const rank = taskRank(left, today, soon) - taskRank(right, today, soon)
      if (rank !== 0) return rank
      if (left.dueDate !== right.dueDate) return left.dueDate.localeCompare(right.dueDate)
      return timestamp(right.updatedAt) - timestamp(left.updatedAt)
    })
    .slice(0, limit)
}
