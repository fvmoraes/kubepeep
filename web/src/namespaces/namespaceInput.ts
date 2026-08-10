import type { InvalidNamespace, NamespaceScopeMode, NamespaceScopeValidation } from '../api/types'

export const INVALID_NAMESPACE_INPUT = 'INVALID_NAMESPACE_INPUT'

export class NamespaceInputError extends Error {
  readonly code = INVALID_NAMESPACE_INPUT

  constructor(message = 'Use plain text, a JSON string array, or a simple YAML string sequence.') {
    super(message)
    this.name = 'NamespaceInputError'
  }
}

export interface LocalNamespaceValidation extends NamespaceScopeValidation {
  format: 'empty' | 'text' | 'json' | 'yaml'
}

const namespaceName = /^[a-z0-9](?:[-a-z0-9]*[a-z0-9])?$/

export function isValidNamespaceName(value: string): boolean {
  return value.length > 0 && value.length <= 63 && namespaceName.test(value)
}

function strictJSONStringArray(value: unknown): string[] {
  if (Array.isArray(value) && value.every((item) => typeof item === 'string')) {
    return value
  }
  if (value !== null && typeof value === 'object' && !Array.isArray(value)) {
    const record = value as Record<string, unknown>
    if (Object.keys(record).length === 1 && Array.isArray(record.namespaces) && record.namespaces.every((item) => typeof item === 'string')) {
      return record.namespaces
    }
  }
  throw new NamespaceInputError('JSON must be a string array or an object containing only a namespaces string array.')
}

function parseJSON(source: string): string[] {
  try {
    return strictJSONStringArray(JSON.parse(source) as unknown)
  } catch (error) {
    if (error instanceof NamespaceInputError) {
      throw error
    }
    throw new NamespaceInputError('The JSON namespace input is malformed.')
  }
}

function decodeYAMLScalar(source: string): string {
  const value = source.trim()
  if (value === '') {
    return ''
  }
  if (/^[&*!]|^<<\s*:|[{}\[\]]/.test(value) || /\s+#/.test(value)) {
    throw new NamespaceInputError('YAML aliases, anchors, tags, comments, and nested values are not supported.')
  }
  if (value.startsWith('"')) {
    if (!value.endsWith('"')) {
      throw new NamespaceInputError('The YAML string is not closed.')
    }
    try {
      const decoded = JSON.parse(value) as unknown
      if (typeof decoded !== 'string') {
        throw new NamespaceInputError('YAML namespace values must be strings.')
      }
      return decoded
    } catch (error) {
      if (error instanceof NamespaceInputError) {
        throw error
      }
      throw new NamespaceInputError('The YAML string is malformed.')
    }
  }
  if (value.startsWith("'")) {
    if (!value.endsWith("'")) {
      throw new NamespaceInputError('The YAML string is not closed.')
    }
    return value.slice(1, -1).replaceAll("''", "'")
  }
  if (/^(?:null|~|true|false|[-+]?\d+(?:\.\d+)?)$/i.test(value) || value.includes(':')) {
    throw new NamespaceInputError('YAML namespace values must be strings.')
  }
  return value
}

function parseYAML(source: string): string[] {
  const normalized = source.replaceAll('\r\n', '\n').replaceAll('\r', '\n')
  const lines = normalized.split('\n')
  let index = 0
  while (index < lines.length && lines[index].trim() === '') {
    index += 1
  }
  if (lines[index]?.trim() === '---') {
    index += 1
  }
  while (index < lines.length && lines[index].trim() === '') {
    index += 1
  }

  let mapping = false
  if (lines[index]?.trim() === 'namespaces:') {
    mapping = true
    index += 1
  }

  const values: string[] = []
  for (; index < lines.length; index += 1) {
    const line = lines[index]
    if (line.trim() === '') {
      continue
    }
    if (line.trim() === '---' || line.trim() === '...') {
      throw new NamespaceInputError('Multiple YAML documents are not supported.')
    }
    const match = /^(\s*)-\s?(.*)$/.exec(line)
    if (!match || (!mapping && match[1].length > 0) || (mapping && match[1].length === 0)) {
      throw new NamespaceInputError('YAML must be a top-level sequence or a namespaces mapping with one sequence.')
    }
    values.push(decodeYAMLScalar(match[2]))
  }
  return values
}

function parseText(source: string): string[] {
  const tokens: string[] = []
  let current = ''
  let index = 0
  const push = () => {
    tokens.push(current)
    current = ''
  }
  while (index < source.length) {
    const character = source[index]
    if (character === ',' || character === ';' || character === '\n' || character === '\r') {
      push()
      if (character === '\r' && source[index + 1] === '\n') {
        index += 1
      }
      index += 1
      continue
    }
    if (character === ' ' || character === '\t') {
      push()
      while (source[index + 1] === ' ' || source[index + 1] === '\t') {
        index += 1
      }
      index += 1
      continue
    }
    current += character
    index += 1
  }
  push()
  return tokens
}

function report(items: string[], format: LocalNamespaceValidation['format']): LocalNamespaceValidation {
  const valid: string[] = []
  const invalid: InvalidNamespace[] = []
  const seen = new Set<string>()
  let duplicateCount = 0
  let discardedEmptyCount = 0

  for (const rawItem of items) {
    const item = rawItem.trim()
    if (item === '') {
      discardedEmptyCount += 1
      continue
    }
    if (seen.has(item)) {
      duplicateCount += 1
      continue
    }
    seen.add(item)
    if (item === '*' || !isValidNamespaceName(item)) {
      invalid.push({ input: item, code: 'INVALID_NAMESPACE_NAME' })
      continue
    }
    valid.push(item)
  }

  return {
    format,
    valid,
    validCount: valid.length,
    duplicateCount,
    discardedEmptyCount,
    invalid,
    invalidCount: invalid.length,
    existence: { checked: false, reasonCode: 'LOCAL_VALIDATION_ONLY' },
  }
}

export function parseNamespaceInput(rawInput: string): LocalNamespaceValidation {
  const source = rawInput.replace(/^\uFEFF/, '').trim()
  if (source === '') {
    return report([], 'empty')
  }
  if (source.startsWith('[') || source.startsWith('{')) {
    return report(parseJSON(source), 'json')
  }
  if (source.startsWith('---') || source.startsWith('namespaces:') || source.startsWith('- ')) {
    return report(parseYAML(source), 'yaml')
  }
  return report(parseText(source), 'text')
}

export function validateNamespaceMode(mode: NamespaceScopeMode, validation: NamespaceScopeValidation): string | null {
  if (mode === 'all') {
    return null
  }
  if (validation.invalidCount > 0) {
    return 'Remove invalid namespace names before saving.'
  }
  if (mode === 'single' && validation.validCount !== 1) {
    return 'Single mode requires exactly one namespace.'
  }
  if (mode === 'list' && validation.validCount === 0) {
    return 'List mode requires at least one namespace.'
  }
  return null
}
