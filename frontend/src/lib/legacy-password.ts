import { APIError, apiClient } from './api-client';

// Exists only for pre-008 credentials; remove this compatibility helper in PR5.
export function normalizeLegacyPassword(value: string): string {
  return value.toUpperCase();
}

// Pre-008 passwords were uppercased before hashing, so a legacy user typing a
// lowercase character fails the first attempt. Retry uppercased only when that
// actually changes the password, which skips a pointless second bcrypt for
// already-uppercase input without guessing from the shape of the identifier.
export async function signInWithLegacyFallback(identifier: string, password: string) {
  try {
    return await apiClient.signIn({ identifier, password });
  } catch (error) {
    const legacyPassword = normalizeLegacyPassword(password);
    const canRetry =
      error instanceof APIError && error.status === 401 && legacyPassword !== password;
    if (!canRetry) {
      throw error;
    }
    return apiClient.signIn({ identifier, password: legacyPassword });
  }
}
