// Exists only for pre-008 credentials; remove this compatibility helper in PR5.
export function normalizeLegacyPassword(value: string): string {
  return value.toUpperCase();
}
