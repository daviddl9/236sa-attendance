export const NRIC_LAST5_FORMAT_MESSAGE =
  'Password must be 4 digits followed by a letter (e.g., 1234A)';

export const NRIC_LAST5_FIELD_MESSAGE =
  'NRIC Last 5 must be 4 digits followed by a letter (e.g., 1234A)';

export function isValidNricLast5(value: string): boolean {
  return /^\d{4}[A-Za-z]$/.test(value);
}

export function normalizeNricLast5(value: string): string {
  return value.toUpperCase();
}
