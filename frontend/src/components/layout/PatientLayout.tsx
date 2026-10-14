import { Outlet, Link, useNavigate } from 'react-router-dom';
import { useAuth } from '../../context/useAuth';
import { LogOut, Home } from 'lucide-react';

export function PatientLayout() {
  const { user, logout } = useAuth();
  const navigate = useNavigate();

  const handleSignOut = async () => {
    await logout();
    navigate('/login?role=patient');
  };

  return (
    <div className="flex min-h-screen bg-surface w-full">
      {/* Patient Sidebar */}
      <aside className="w-64 bg-white border-r border-border flex flex-col sticky top-0 h-screen p-6 justify-between">
        <div>
          <Link to="/" className="flex items-center gap-3 mb-8">
            <div className="w-8 h-8 bg-brand-700 rounded-lg flex items-center justify-center text-white font-bold text-lg">
              V
            </div>
            <div>
              <span className="font-bold text-lg tracking-tight text-slate-900 block leading-tight">
                VitalWatch
              </span>
              <span className="text-[11px] text-slate-400 font-medium block">
                Patient Portal
              </span>
            </div>
          </Link>

          <nav className="space-y-1">
            <Link
              to="/patient/dashboard"
              className="flex items-center gap-3 px-3.5 py-2.5 bg-brand-50 text-brand-700 rounded-xl font-semibold text-sm transition-colors"
            >
              <Home className="w-4 h-4" />
              <span>Dashboard</span>
            </Link>
          </nav>
        </div>

        {/* User Card & Sign Out */}
        <div className="border-t border-border pt-4 space-y-3">
          <div className="flex items-center gap-3 px-1">
            <div className="w-9 h-9 rounded-full bg-brand-100 text-brand-800 flex items-center justify-center font-bold text-sm shrink-0">
              {user?.first_name ? user.first_name[0].toUpperCase() : 'P'}
            </div>
            <div className="min-w-0 flex-1">
              <p className="text-sm font-semibold text-slate-900 truncate">
                {user ? `${user.first_name} ${user.last_name}` : 'Patient'}
              </p>
              <p className="text-xs text-slate-400 truncate">
                {user?.email || ''}
              </p>
            </div>
          </div>

          <button
            onClick={handleSignOut}
            type="button"
            className="w-full flex items-center gap-2.5 px-3 py-2 text-xs font-semibold text-slate-600 hover:text-rose-600 hover:bg-rose-50 rounded-lg transition-colors cursor-pointer"
          >
            <LogOut className="w-4 h-4" />
            <span>Sign Out</span>
          </button>
        </div>
      </aside>

      <main className="flex-1 p-8 md:p-10 max-w-6xl mx-auto w-full">
        <Outlet />
      </main>
    </div>
  );
}
