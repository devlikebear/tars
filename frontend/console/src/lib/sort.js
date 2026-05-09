/**
 * @param {string} a
 * @param {string} b
 * @returns {number}
 */
export function compareStrings(a, b) {
  return a.localeCompare(b)
}

/**
 * @param {Iterable<string>} values
 * @returns {string[]}
 */
export function sortStrings(values) {
  return [...values].sort(compareStrings)
}
