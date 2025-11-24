import type { User } from './api-client';

// Rank order map (matching backend) - lower number = lower rank
const RANK_ORDER: Record<string, number> = {
  'REC': 0, 'PTE': 1, 'LCP': 2, 'CPL': 3, 'CFC': 4,
  '3SG': 5, '2SG': 6, '1SG': 7, 'SSG': 8, 'MSG': 9,
  '3WO': 10, '2WO': 11, '1WO': 12, 'MWO': 13, 'SWO': 14,
  '2LT': 15, 'LTA': 16, 'CPT': 17, 'MAJ': 18, 'LTC': 19,
};

/**
 * Checks if a user is a commander (rank 3SG or above)
 * Matches backend logic: rank order >= 5 means commander
 */
export function isCommander(user: User | null | undefined): boolean {
  if (!user?.rank) {
    return false;
  }
  
  const rank = user.rank.toUpperCase();
  const order = RANK_ORDER[rank];
  
  if (order === undefined) {
    return false;
  }
  
  // 3SG has order 5, so order >= 5 means commander
  return order >= 5;
}

/**
 * Checks if a user can access commander-level features
 * (Commander rank or superadmin)
 */
export function canAccessCommanderFeatures(user: User | null | undefined): boolean {
  return isCommander(user) || (user?.isSuperadmin ?? false);
}

/**
 * Checks if a user can access superadmin features
 */
export function isSuperadmin(user: User | null | undefined): boolean {
  return user?.isSuperadmin ?? false;
}

