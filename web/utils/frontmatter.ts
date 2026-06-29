import { parseDocument, stringify } from 'yaml'

export type FrontmatterData = Record<string, unknown>
export type PropertyType = 'text' | 'list' | 'date'

export interface FrontmatterSplit {
  data: FrontmatterData
  body: string
  hasBlock: boolean
}

const FRONTMATTER_RE = /^---\r?\n([\s\S]*?)\r?\n---(?:\r?\n|$)/
const DATE_RE = /^\d{4}-\d{2}-\d{2}$/

export function splitFrontmatter(content: string): FrontmatterSplit {
  if (!content.startsWith('---\n') && !content.startsWith('---\r\n')) {
    return { data: {}, body: content, hasBlock: false }
  }

  const match = content.match(FRONTMATTER_RE)
  if (!match) {
    throw new Error('Frontmatter block is missing its closing --- delimiter.')
  }

  const doc = parseDocument(match[1])
  if (doc.errors.length > 0) {
    throw new Error(doc.errors[0]?.message || 'Frontmatter is not valid YAML.')
  }

  const value = doc.toJSON()
  const data = isRecord(value) ? value : {}
  return {
    data,
    body: content.slice(match[0].length),
    hasBlock: true,
  }
}

export function replaceFrontmatter(content: string, data: FrontmatterData): string {
  const split = splitFrontmatter(content)
  const block = serializeFrontmatter(data)
  if (!block) return split.body
  return `${block}\n\n${split.body.replace(/^\r?\n/, '')}`
}

export function serializeFrontmatter(data: FrontmatterData): string {
  const cleaned = cleanFrontmatter(data)
  if (Object.keys(cleaned).length === 0) return ''

  const yaml = stringify(cleaned, { lineWidth: 0 }).trimEnd()
  return `---\n${yaml}\n---`
}

export function inferPropertyType(key: string, value: unknown): PropertyType {
  if (key === 'tags' || Array.isArray(value)) return 'list'
  if (typeof value === 'string' && DATE_RE.test(value)) return 'date'
  return 'text'
}

export function normalizeValueForType(value: string, type: PropertyType): unknown {
  if (type === 'list') {
    return value
      .split(',')
      .map((part) => normalizeTag(part))
      .filter(Boolean)
  }
  return value.trim()
}

export function normalizeTag(value: string): string {
  return value.trim().replace(/^#+/, '')
}

function cleanFrontmatter(data: FrontmatterData): FrontmatterData {
  const cleaned: FrontmatterData = {}
  for (const [key, value] of Object.entries(data)) {
    if (!key || value === undefined) continue
    cleaned[key] = value
  }
  return cleaned
}

function isRecord(value: unknown): value is FrontmatterData {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value)
}
