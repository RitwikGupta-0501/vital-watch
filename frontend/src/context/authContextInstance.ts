import { createContext } from 'react';
import type {
  UserProfile,
  LoginRequest,
  LoginResponse,
  RegisterRequest,
  RegisterResponse,
} from '../lib/api/types';

export interface AuthContextValue {
  user: UserProfile | null;
  role: string | null;
  isAuthenticated: boolean;
  isLoading: boolean;
  login: (credentials: LoginRequest) => Promise<LoginResponse>;
  register: (payload: RegisterRequest) => Promise<RegisterResponse>;
  logout: (allDevices?: boolean) => Promise<void>;
  refreshProfile: () => Promise<UserProfile | null>;
  saveDraftForm: (formId: string, data: unknown) => void;
  getDraftForm: <T>(formId: string) => T | null;
  clearDraftForm: (formId: string) => void;
}

export const AuthContext = createContext<AuthContextValue | null>(null);
