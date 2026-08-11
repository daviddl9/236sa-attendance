export interface SignUpFormValues {
  username: string;
  password: string;
  confirmPassword: string;
  fullName: string;
  rank: string;
  battery: string;
}

export const PASSWORD_IDENTIFIER_MESSAGE = 'Do not use your NRIC digits as your password';
export const PASSWORD_LENGTH_MESSAGE = 'Password must be at least 8 characters';
export const PASSWORD_MISMATCH_MESSAGE = 'Passwords do not match';
export const USERNAME_UNAVAILABLE_MESSAGE = 'Username is unavailable';

export const VALID_RANKS = [
  'REC', 'PTE', 'LCP', 'CPL', 'CFC',
  '3SG', '2SG', '1SG', 'SSG', 'MSG',
  '3WO', '2WO', '1WO', 'MWO', 'SWO',
  '2LT', 'LTA', 'CPT', 'MAJ', 'LTC',
] as const;

export const VALID_BATTERIES = ['HQ', 'Alpha', 'Bravo'] as const;

const MIGRATED_USERNAME_PREFIX = '__migrated_pending__';
const IDENTIFIER_SHAPED_PASSWORD = /^\d{4}[A-Za-z]$/;

export function validateSignUp(values: SignUpFormValues): string | null {
  const username = values.username.trim();
  const fullName = values.fullName.trim();
  const rank = values.rank.trim();
  const battery = values.battery.trim();

  if (!username) return 'Username is required';
  if (username.toLowerCase().startsWith(MIGRATED_USERNAME_PREFIX)) {
    return USERNAME_UNAVAILABLE_MESSAGE;
  }
  if (!values.password) return 'Password is required';
  if (IDENTIFIER_SHAPED_PASSWORD.test(values.password)) {
    return PASSWORD_IDENTIFIER_MESSAGE;
  }
  if (Array.from(values.password).length < 8) {
    return PASSWORD_LENGTH_MESSAGE;
  }
  if (values.password !== values.confirmPassword) {
    return PASSWORD_MISMATCH_MESSAGE;
  }
  if (!fullName) return 'Full name is required';
  if (!VALID_RANKS.some((validRank) => validRank === rank)) return 'Invalid rank';
  if (!VALID_BATTERIES.some((validBattery) => validBattery === battery)) {
    return 'Invalid battery (must be HQ, Alpha, or Bravo)';
  }
  return null;
}
