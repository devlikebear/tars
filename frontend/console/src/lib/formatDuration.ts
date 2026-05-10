// Human-readable countdown for "resets in" labels. Inputs are seconds.
// Renders as `<1m`, `12m`, or `2h 14m` (no decimals; minutes rounded down).
// Unknown / negative inputs collapse to the empty string so callers can omit
// the slot entirely.
export function formatResetCountdown(seconds: number | undefined | null): string {
  if (seconds === undefined || seconds === null || !Number.isFinite(seconds) || seconds < 0) {
    return ''
  }
  const total = Math.floor(seconds)
  if (total < 60) return '<1m'
  const hours = Math.floor(total / 3600)
  const minutes = Math.floor((total % 3600) / 60)
  if (hours === 0) return `${minutes}m`
  if (minutes === 0) return `${hours}h`
  return `${hours}h ${minutes}m`
}
