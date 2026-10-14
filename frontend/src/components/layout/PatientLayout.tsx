import { Outlet, Link, useNavigate, useLocation } from 'react-router-dom';
import { useAuth } from '../../context/useAuth';
import {
  LogOut,
  Home,
  Stethoscope,
  Calendar,
  FileText,
} from 'lucide-react';
import { cn } from '../../lib/utils';

export function PatientLayout() {
  const { user, logout } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();

  const handleSignOut = async () => {
    await logout();
    navigate('/login?role=patient');
  };

  const navItems = [
    {
      label: 'Dashboard',
      path: '/patient/dashboard',
      icon: Home,
    },
    {
      label: 'Find Doctors',
      path: '/patient/doctors',
      icon: Stethoscope,
    },
    {
      label: 'Appointments',
      path: '/patient/appointments',
      icon: Calendar,
    },
    {
      label: 'Prescriptions',
      path: '/patient/prescriptions',
      icon: FileText,
    },
  ];

  const isNavActive = (path: string) => {
    if (path === '/patient/dashboard') {
      return location.pathname === path;
    }
    return location.pathname.startsWith(path);
  };

  return (
    <div className="flex flex-col md:flex-row min-h-screen bg-surface w-full">
      {/* Desktop Patient Sidebar */}
      <aside className="hidden md:flex w-64 bg-white border-r border-border flex-col sticky top-0 h-screen p-6 justify-between shrink-0">
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
            {navItems.map((item) => {
              const Icon = item.icon;
              const active = isNavActive(item.path);
              return (
                <Link
                  key={item.path}
                  to={item.path}
                  className={cn(
                    'flex items-center gap-3 px-3.5 py-2.5 rounded-xl text-sm transition-all',
                    active
                      ? 'bg-brand-50 text-brand-700 font-semibold shadow-xs'
                      : 'text-slate-600 hover:text-slate-900 hover:bg-slate-50 font-medium'
                  )}
                >
                  <Icon className={cn('w-4 h-4', active ? 'text-brand-700' : 'text-slate-400')} />
                  <span>{item.label}</span>
                </Link>
              );
            })}
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

      {/* Mobile Top Header */}
      <header className="md:hidden flex items-center justify-between p-4 bg-white border-b border-border sticky top-0 z-30">
        <Link to="/" className="flex items-center gap-2">
          <div className="w-7 h-7 bg-brand-700 rounded-md flex items-center justify-center text-white font-bold text-sm">
            V
          </div>
          <span className="font-bold text-base tracking-tight text-slate-900">
            VitalWatch
          </span>
        </Link>
        <button
          onClick={handleSignOut}
          type="button"
          className="p-1.5 text-slate-500 hover:text-rose-600 hover:bg-rose-50 rounded-md transition-colors"
          title="Sign Out"
        >
          <LogOut className="w-4 h-4" />
        </button>
      </header>

      {/* Main Content Area */}
      <main className="flex-1 p-4 sm:p-6 md:p-10 max-w-6xl mx-auto w-full pb-24 md:pb-10">
        <Outlet />
      </main>

      {/* Mobile Bottom Navigation Bar (WCAG Touch Targets >= 44px) */}
      <nav className="md:hidden fixed bottom-0 left-0 right-0 bg-white border-t border-border z-40 flex items-center justify-around px-2 py-1 shadow-lg">
        {navItems.map((item) => {
          const Icon = item.icon;
          const active = isNavActive(item.path);
          return (
            <Link
              key={item.path}
              to={item.path}
              className={cn(
                'flex flex-col items-center justify-center min-w-[64px] min-h-[48px] px-2 py-1 rounded-lg text-[10px] font-medium transition-colors',
                active ? 'text-brand-700 font-semibold' : 'text-slate-500 hover:text-slate-900'
              )}
            >
              <Icon className={cn('w-5 h-5 mb-0.5', active ? 'text-brand-700' : 'text-slate-400')} />
              <span>{item.label}</span>
            </Link>
          );
        })}
      </nav>
    </div>
  );
}
