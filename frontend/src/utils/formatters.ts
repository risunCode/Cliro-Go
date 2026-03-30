export const formatNumber = (value: number | undefined): string => {
  if (typeof value !== 'number' || Number.isNaN(value)) {
    return '0'
  }
  return new Intl.NumberFormat().format(value)
}

export const nowUnixSeconds = (): number => {
  return Math.floor(Date.now() / 1000)
}

export const formatDateTime = (value: number | undefined): string => {
  if (!value) {
    return '-'
  }

  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return '-'
  }

  return new Intl.DateTimeFormat(undefined, {
    year: 'numeric',
    month: 'short',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  }).format(date)
}

export const formatUnixSeconds = (value: number | undefined): string => {
  if (!value || value <= 0) {
    return '-'
  }
  return formatDateTime(value * 1000)
}
