// Masks raw input as dd/mm/yyyy: digits only, slashes auto-inserted.
// Re-derived from digits on every change so backspace and paste behave.
export function formatDob(value: string): string {
  const digits = value.replace(/\D/g, '').slice(0, 8);
  return [digits.slice(0, 2), digits.slice(2, 4), digits.slice(4, 8)]
    .filter(Boolean)
    .join('/');
}
