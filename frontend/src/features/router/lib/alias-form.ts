export interface AliasFormRow {
  from: string
  to: string
}

export const createEmptyAliasRow = (): AliasFormRow => ({
  from: '',
  to: ''
})

export const cloneAliasRows = (rows: AliasFormRow[]): AliasFormRow[] => {
  return rows.map((row) => ({ from: row.from, to: row.to }))
}

export const aliasRowsFromRecord = (aliases: Record<string, string>): AliasFormRow[] => {
  return Object.entries(aliases).map(([from, to]) => ({ from, to }))
}

const normalizeAliasRows = (rows: AliasFormRow[]): AliasFormRow[] => {
  return rows.map((row) => ({
    from: row.from.trim(),
    to: row.to.trim()
  }))
}

export const validateAliasRows = (rows: AliasFormRow[]): string => {
  const normalizedRows = normalizeAliasRows(rows)
  const hasIncompleteRow = normalizedRows.some((row) => row.from.length === 0 || row.to.length === 0)
  if (hasIncompleteRow) {
    return 'All alias fields must be filled.'
  }

  const uniqueSources = new Set(normalizedRows.map((row) => row.from))
  if (uniqueSources.size !== normalizedRows.length) {
    return 'Duplicate source model names found.'
  }

  return ''
}

export const serializeAliasRows = (rows: AliasFormRow[]): Record<string, string> => {
  const validationError = validateAliasRows(rows)
  if (validationError) {
    throw new Error(validationError)
  }

  return Object.fromEntries(normalizeAliasRows(rows).map((row) => [row.from, row.to]))
}
