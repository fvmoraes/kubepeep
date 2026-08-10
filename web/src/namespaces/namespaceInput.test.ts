import { describe, expect, it } from 'vitest'

import { NamespaceInputError, parseNamespaceInput, validateNamespaceMode } from './namespaceInput'

describe('namespace input grammar', () => {
  it('accepts every plain-text delimiter and preserves the first stable occurrence', () => {
    const result = parseNamespaceInput('payments,billing;invoices\nreports shared\tplatform,payments')

    expect(result.format).toBe('text')
    expect(result.valid).toEqual(['payments', 'billing', 'invoices', 'reports', 'shared', 'platform'])
    expect(result.duplicateCount).toBe(1)
  })

  it('counts empty, duplicate, invalid, and valid JSON array items independently', () => {
    const result = parseNamespaceInput('["payments", "", "payments", "Bad_Name", "billing"]')

    expect(result).toMatchObject({
      format: 'json',
      valid: ['payments', 'billing'],
      validCount: 2,
      duplicateCount: 1,
      discardedEmptyCount: 1,
      invalid: [{ input: 'Bad_Name', code: 'INVALID_NAMESPACE_NAME' }],
      invalidCount: 1,
    })
  })

  it('accepts the strict JSON object form and strips a leading BOM', () => {
    const result = parseNamespaceInput('\uFEFF {"namespaces":["kube-system","default"]}')
    expect(result.format).toBe('json')
    expect(result.valid).toEqual(['kube-system', 'default'])
  })

  it('accepts top-level and mapped simple YAML sequences', () => {
    expect(parseNamespaceInput('- payments\n- billing').valid).toEqual(['payments', 'billing'])

    const mapped = parseNamespaceInput('---\nnamespaces:\n  - payments\n  - ""\n  - payments\n  - Bad_Name')
    expect(mapped).toMatchObject({
      format: 'yaml',
      valid: ['payments'],
      validCount: 1,
      duplicateCount: 1,
      discardedEmptyCount: 1,
      invalidCount: 1,
    })
  })

  it('counts consecutive explicit delimiters as discarded empty input', () => {
    const result = parseNamespaceInput('payments,,billing;;invoices')
    expect(result.valid).toEqual(['payments', 'billing', 'invoices'])
    expect(result.discardedEmptyCount).toBe(2)
  })

  it.each([
    '["payments",]',
    '{"namespaces":["payments"],"extra":[]}',
    '{"namespaces":[1]}',
    'namespaces:\n  - &shared payments',
    '---\n- payments\n---\n- billing',
    'namespaces:\n  nested:\n    - payments',
  ])('rejects malformed JSON/YAML without falling back to text: %s', (source) => {
    expect(() => parseNamespaceInput(source)).toThrow(NamespaceInputError)
  })

  it('validates Kubernetes DNS label names and rejects wildcard materialization', () => {
    const result = parseNamespaceInput('default kube-system * Upper 64-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa')
    expect(result.valid).toEqual(['default', 'kube-system'])
    expect(result.invalid.map(({ input }) => input)).toEqual([
      '*',
      'Upper',
      '64-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
    ])
  })

  it('enforces single/list mode cardinality while all needs no item', () => {
    const one = parseNamespaceInput('payments')
    const many = parseNamespaceInput('payments billing')
    const empty = parseNamespaceInput('')

    expect(validateNamespaceMode('single', one)).toBeNull()
    expect(validateNamespaceMode('single', many)).toMatch(/exactly one/)
    expect(validateNamespaceMode('list', many)).toBeNull()
    expect(validateNamespaceMode('list', empty)).toMatch(/at least one/)
    expect(validateNamespaceMode('all', empty)).toBeNull()
  })
})
