import { createContext, useContext, useState } from 'react';
import type { ReactNode } from 'react';
import { apiClient } from './api-client';
import type { User } from './api-client';
import { useQuery, useQueryClient } from '@tanstack/react-query';

interface AuthContextType {
  user: User | null;
  isLoading: boolean;
  isAuthenticated: boolean;
  signOut: () => Promise<void>;
  refetch: () => void;
  setUser: (user: User | null) => void;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

export function AuthProvider({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient();
  const [manualUser, setManualUser] = useState<User | null>(null);

  const { data, isLoading, refetch } = useQuery({
    queryKey: ['session'],
    queryFn: async () => {
      try {
        return await apiClient.getSession();
      } catch {
        return null;
      }
    },
    retry: false,
    staleTime: 5 * 60 * 1000, // 5 minutes
    throwOnError: false,
  });

  // Use manual user if set, otherwise use query data
  const user = manualUser || data || null;

  const signOut = async () => {
    try {
      await apiClient.signOut();
      setManualUser(null);
      queryClient.clear();
    } catch (error) {
      console.error('Sign out error:', error);
    }
  };

  return (
    <AuthContext.Provider
      value={{
        user,
        isLoading,
        isAuthenticated: !!user,
        signOut,
        refetch: () => {
          refetch();
        },
        setUser: setManualUser,
      }}
    >
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth() {
  const context = useContext(AuthContext);
  if (context === undefined) {
    throw new Error('useAuth must be used within an AuthProvider');
  }
  return context;
}

