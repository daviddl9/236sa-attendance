import type { User, AccessTier } from './api-client';

// Rank order map (matching backend) - lower number = lower rank
const RANK_ORDER: Record<string, number> = {
  'REC': 0, 'PTE': 1, 'LCP': 2, 'CPL': 3, 'CFC': 4,
  '3SG': 5, '2SG': 6, '1SG': 7, 'SSG': 8, 'MSG': 9,
  '3WO': 10, '2WO': 11, '1WO': 12, 'MWO': 13, 'SWO': 14,
  '2LT': 15, 'LTA': 16, 'CPT': 17, 'MAJ': 18, 'LTC': 19,
};

/** Returns the server-computed tier. Falls back to rank derivation for compatibility. */
export function getUserTier(user: User | null | undefined): AccessTier {
  if (!user) return 1;
  // Prefer the server-computed tier (included in /auth/session response).
  if (user.tier) return user.tier;
  // Fallback: derive locally (matches backend logic).
  if (user.isSuperadmin) return 4;
  const order = user.rank ? (RANK_ORDER[user.rank.toUpperCase()] ?? -1) : -1;
  if (order >= RANK_ORDER['CPT']) return 4;
  if (order >= RANK_ORDER['SSG']) return 3;
  if (order >= RANK_ORDER['3SG']) return 2;
  return 1;
}

/** Tier 2+: can view sessions and users (own battery for Tier 2). */
export function canViewBatteryData(user: User | null | undefined): boolean {
  return getUserTier(user) >= 2;
}

/** Tier 3+: can create/close sessions, manage statuses, export. */
export function canManageUnit(user: User | null | undefined): boolean {
  return getUserTier(user) >= 3;
}

/** Tier 4: full admin. */
export function isSuperadmin(user: User | null | undefined): boolean {
  return user?.isSuperadmin ?? false;
}

// Legacy aliases kept for existing callers.
export function isCommander(user: User | null | undefined): boolean {
  return canViewBatteryData(user);
}

export function canAccessCommanderFeatures(user: User | null | undefined): boolean {
  return canViewBatteryData(user);
}
