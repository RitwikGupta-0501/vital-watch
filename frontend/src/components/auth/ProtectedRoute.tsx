import type { ReactElement } from 'react';
import { Navigate, useLocation } from 'react-router-dom';
import { useAuth } from '../../context/useAuth';
import type { UserRole } from '../../lib/api/types';
import { ShieldAlert } from 'lucide-react';
import { Button } from '../ui/Button';

interface ProtectedRouteProps {
  children: ReactElement;
  allowedRoles?: UserRole[];
}

export function ProtectedRoute({
  children,
  allowedRoles,
}: ProtectedRouteProps) {
  const { user, isAuthenticated, isLoading, logout } = useAuth();
  const location = useLocation();

  if (isLoading) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-surface">
        <div className="flex flex-col items-center gap-3">
          <div className="w-8 h-8 border-2 border-brand-600 border-t-transparent rounded-full animate-spin" />
          <p className="text-sm font-medium text-slate-500">Verifying clinical credentials...</p>
        </div>
      </div>
    );
  }

  if (!isAuthenticated || !user) {
    const targetRole = allowedRoles?.[0] || 'patient';
    const returnUrl = encodeURIComponent(location.pathname + location.search);
    return <Navigate to={`/login?role=${targetRole}&returnUrl=${returnUrl}`} replace />;
  }

  if (allowedRoles && !allowedRoles.includes(user.role)) {
    return (
      <div className="min-h-[70vh] flex flex-col items-center justify-center p-6 text-center">
        <div className="w-14 h-14 rounded-2xl bg-rose-50 border border-rose-200 text-rose-600 flex items-center justify-center mb-4">
          <ShieldAlert className="w-8 h-8" />
        </div>
        <h2 className="text-2xl font-bold tracking-tight text-slate-900 mb-2">
          Access Restricted (403)
        </h2>
        <p className="text-slate-500 max-w-md mb-6 text-sm">
          Your account is logged in with role <strong className="text-slate-800">{user.role}</strong>,
          which does not possess permission to access this clinical surface.
        </p>
        <div className="flex gap-4">
          <Button
            variant="outline"
            onClick={() => window.history.back()}
          >
            Go Back
          </Button>
          <Button
            onClick={() => logout()}
          >
            Switch Account
          </Button>
        </div>
      </div>
    );
  }

  return children;
}
