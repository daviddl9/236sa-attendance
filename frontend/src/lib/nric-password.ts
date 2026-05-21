export const NRIC_LAST5_FORMAT_MESSAGE =
  'Password must be exactly 5 characters: 4 numbers followed by an alphabet letter (e.g., 1234A)';

export const NRIC_LAST5_FIELD_MESSAGE =
  'NRIC Last 5 must be exactly 5 characters: 4 numbers followed by an alphabet letter (e.g., 1234A)';

export function isValidNricLast5(value: string): boolean {
  return /^\d{4}[A-Za-z]$/.test(value);
}

export function normalizeNricLast5(value: string): string {
  return value.toUpperCase();
}
