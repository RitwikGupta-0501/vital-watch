import { Outlet, Link } from 'react-router-dom';
import { useAuth } from '../../context/useAuth';
import { Button } from '../ui/Button';

export function PublicLayout() {
  const { isAuthenticated, user } = useAuth();

  return (
    <div className="min-h-screen bg-slate-50 flex flex-col font-sans">
      <header className="px-6 md:px-8 py-4 flex items-center justify-between border-b border-border bg-white sticky top-0 z-30">
        <Link to="/" className="flex items-center gap-3">
          <div className="w-8 h-8 bg-brand-700 rounded-lg flex items-center justify-center text-white font-bold text-lg">
            V
          </div>
          <span className="font-bold text-xl tracking-tight text-slate-900">
            VitalWatch
          </span>
        </Link>

        <div className="flex items-center gap-3">
          {isAuthenticated && user ? (
            <Link to={user.role === 'doctor' ? '/doctor/dashboard' : '/patient/dashboard'}>
              <Button size="sm" isPatientTheme={user.role === 'patient'}>
                Go to Dashboard
              </Button>
            </Link>
          ) : (
            <>
              <Link
                to="/login?role=patient"
                className="text-sm font-semibold text-slate-600 hover:text-slate-900 px-2 py-1.5 hidden sm:inline-block"
              >
                Patient Portal
              </Link>
              <Link
                to="/login?role=doctor"
                className="text-sm font-semibold text-slate-600 hover:text-slate-900 px-2 py-1.5 hidden sm:inline-block"
              >
                Clinician Portal
              </Link>
              <Link to="/login?role=patient">
                <Button size="sm" isPatientTheme>
                  Sign In
                </Button>
              </Link>
            </>
          )}
        </div>
      </header>

      <main className="flex-1 flex flex-col">
        <Outlet />
      </main>

      <footer className="border-t border-border bg-white py-8 px-6 text-xs text-slate-500">
        <div className="max-w-6xl mx-auto flex flex-col sm:flex-row items-center justify-between gap-4">
          <p>© 2026 VitalWatch Healthcare Intelligence. All rights reserved.</p>
          <div className="flex items-center gap-6">
            <Link to="/notice-of-privacy-practices" className="hover:text-slate-900 transition-colors">
              Notice of Privacy Practices
            </Link>
            <Link to="/terms" className="hover:text-slate-900 transition-colors">
              Terms of Service
            </Link>
            <Link to="/security" className="hover:text-slate-900 transition-colors">
              HIPAA Security Posture
            </Link>
          </div>
        </div>
      </footer>
    </div>
  );
}
