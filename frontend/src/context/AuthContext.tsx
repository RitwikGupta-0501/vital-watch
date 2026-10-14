import {
  useState,
  useEffect,
  useCallback,
  type ReactNode,
  type FC,
} from 'react';
import type {
  UserProfile,
  LoginRequest,
  LoginResponse,
  RegisterRequest,
  RegisterResponse,
  LogoutResponse,
} from '../lib/api/types';
import {
  fetchApi,
  subscribeAuthExpired,
} from '../lib/api/client';
import { AuthContext } from './authContextInstance';

export const AuthProvider: FC<{ children: ReactNode }> = ({ children }) => {
  const [user, setUser] = useState<UserProfile | null>(null);
  const [isLoading, setIsLoading] = useState(true);

  // Restore current user session from existing httpOnly cookies
  const refreshProfile = useCallback(async (): Promise<UserProfile | null> => {
    try {
      const profile = await fetchApi<UserProfile>('/api/profile');
      setUser(profile);
      return profile;
    } catch {
      setUser(null);
      return null;
    }
  }, []);

  useEffect(() => {
    let isMounted = true;
    (async () => {
      try {
        const profile = await fetchApi<UserProfile>('/api/profile');
        if (isMounted) setUser(profile);
      } catch {
        if (isMounted) setUser(null);
      } finally {
        if (isMounted) setIsLoading(false);
      }
    })();

    // Subscribe to unrecoverable token expiration
    const unsubscribe = subscribeAuthExpired(() => {
      if (isMounted) {
        setUser(null);
      }
    });

    return () => {
      isMounted = false;
      unsubscribe();
    };
  }, []);

  const login = async (credentials: LoginRequest): Promise<LoginResponse> => {
    const res = await fetchApi<LoginResponse>('/api/login', {
      method: 'POST',
      body: JSON.stringify(credentials),
      skipAuthRefresh: true,
    });

    await refreshProfile();
    return res;
  };

  const register = async (payload: RegisterRequest): Promise<RegisterResponse> => {
    return fetchApi<RegisterResponse>('/api/register', {
      method: 'POST',
      body: JSON.stringify(payload),
      skipAuthRefresh: true,
    });
  };

  const logout = async (allDevices = false): Promise<void> => {
    try {
      await fetchApi<LogoutResponse>('/api/auth/logout', {
        method: 'POST',
        body: JSON.stringify({ all_devices: allDevices }),
        skipAuthRefresh: true,
      });
    } catch {
      // Ignore network errors on logout
    } finally {
      setUser(null);
    }
  };

  // Draft form state preservation across expired sessions
  const saveDraftForm = (formId: string, data: unknown) => {
    try {
      sessionStorage.setItem(`vital_watch_draft_${formId}`, JSON.stringify(data));
    } catch {
      // Ignore quota errors
    }
  };

  const getDraftForm = <T,>(formId: string): T | null => {
    try {
      const item = sessionStorage.getItem(`vital_watch_draft_${formId}`);
      return item ? (JSON.parse(item) as T) : null;
    } catch {
      return null;
    }
  };

  const clearDraftForm = (formId: string) => {
    try {
      sessionStorage.removeItem(`vital_watch_draft_${formId}`);
    } catch {
      // Ignore
    }
  };

  return (
    <AuthContext.Provider
      value={{
        user,
        role: user?.role || null,
        isAuthenticated: !!user,
        isLoading,
        login,
        register,
        logout,
        refreshProfile,
        saveDraftForm,
        getDraftForm,
        clearDraftForm,
      }}
    >
      {children}
    </AuthContext.Provider>
  );
};
