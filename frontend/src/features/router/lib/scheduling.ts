export type SchedulingMode = 'cache_first' | 'balance' | 'performance'

export interface SchedulingModeCard {
  id: SchedulingMode
  label: string
  description: string
}

export const SCHEDULING_MODE_CARDS: SchedulingModeCard[] = [
  {
    id: 'cache_first',
    label: 'Cache First',
    description: 'Bind session to the same account for stronger cache locality.'
  },
  {
    id: 'balance',
    label: 'Balance',
    description: 'Spread load across accounts by favoring lower-usage accounts.'
  },
  {
    id: 'performance',
    label: 'Performance',
    description: 'Use pure round-robin ordering for high concurrency throughput.'
  }
]

export const toSchedulingMode = (value: string): SchedulingMode => {
  if (value === 'cache_first' || value === 'balance' || value === 'performance') {
    return value
  }
  return 'balance'
}
