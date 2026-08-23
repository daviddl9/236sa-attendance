// DOB helpers shared by the segmented DD/MM/YYYY fields.
// Values are stored as a single "dd/mm/yyyy" string and split/joined at the edges.

export function splitDob(value: string): { day: string; month: string; year: string } {
  const digits = value.replace(/\D/g, '');
  return {
    day: digits.slice(0, 2),
    month: digits.slice(2, 4),
    year: digits.slice(4, 8),
  };
}

export function joinDob(day: string, month: string, year: string): string {
  return [day, month, year].filter(Boolean).join('/');
}

export function isValidDob(value: string): boolean {
  const { day, month, year } = splitDob(value);
  if (!day || !month || year.length !== 4) return false;
  const d = Number(day);
  const m = Number(month);
  const y = Number(year);
  const currentYear = new Date().getFullYear();
  return d >= 1 && d <= 31 && m >= 1 && m <= 12 && y >= 1900 && y <= currentYear;
}
