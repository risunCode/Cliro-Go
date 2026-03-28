export const toggleSelectedID = (selectedIds: string[], id: string): string[] => {
  if (selectedIds.includes(id)) {
    return selectedIds.filter((selectedID) => selectedID !== id)
  }
  return [...selectedIds, id]
}

export const areAllVisibleSelected = (selectedIds: string[], visibleAccountIds: string[]): boolean => {
  if (visibleAccountIds.length === 0) {
    return false
  }
  return visibleAccountIds.every((id) => selectedIds.includes(id))
}

export const toggleSelectAllVisible = (selectedIds: string[], visibleAccountIds: string[], allVisibleSelected: boolean): string[] => {
  if (visibleAccountIds.length === 0) {
    return selectedIds
  }

  if (allVisibleSelected) {
    const visibleSet = new Set(visibleAccountIds)
    return selectedIds.filter((id) => !visibleSet.has(id))
  }

  return [...selectedIds.filter((id, index) => selectedIds.indexOf(id) === index), ...visibleAccountIds.filter((id) => !selectedIds.includes(id))]
}
