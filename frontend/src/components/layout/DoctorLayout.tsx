import { Outlet, Link, useNavigate, useLocation } from 'react-router-dom';
import { useAuth } from '../../context/useAuth';
import type { DoctorProfile } from '../../lib/api/types';
import { useQuery } from '@tanstack/react-query';
import { Calendar, CalendarDays, Users, FileText, Clock, LogOut, ClipboardCheck } from 'lucide-react';
import { getPendingReviewPrescriptions } from '../../lib/api/prescriptions';
import type { PaginatedResponse, Prescription } from '../../lib/api/types';

export function DoctorLayout() {
  const { user, logout } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();
  const doctor = user as DoctorProfile | null;

  const { data: pendingData } = useQuery<PaginatedResponse<Prescription>>({
    queryKey: ['pending-review-prescriptions'],
    queryFn: () => getPendingReviewPrescriptions(10, 0),
    enabled: !!user && user.role === 'doctor',
    refetchInterval: 30000,
  });

  const pendingCount = pendingData?.data?.length || 0;

  const handleSignOut = async () => {
    await logout();
    navigate('/login?role=doctor');
  };

  const navItems = [
    {
      to: '/doctor/today',
      title: "Today's Agenda",
      icon: Calendar,
      isActive: location.pathname === '/doctor/today' || location.pathname === '/doctor/dashboard' || location.pathname === '/doctor',
    },
    {
      to: '/doctor/appointments',
      title: 'Appointments Ledger',
      icon: CalendarDays,
      isActive: location.pathname === '/doctor/appointments',
    },
    {
      to: '/doctor/patients',
      title: 'Patient Roster',
      icon: Users,
      isActive: location.pathname.startsWith('/doctor/patients'),
    },
    {
      to: '/doctor/review',
      title: 'OCR Review Queue',
      icon: ClipboardCheck,
      isActive: location.pathname.startsWith('/doctor/review'),
      badge: pendingCount > 0 ? pendingCount : undefined,
    },
    {
      to: '/doctor/prescriptions',
      title: 'Prescriptions Directory',
      icon: FileText,
      isActive: location.pathname === '/doctor/prescriptions',
    },
    {
      to: '/doctor/settings/schedule',
      title: 'Office Hours & Schedule',
      icon: Clock,
      isActive: location.pathname === '/doctor/settings/schedule',
    },
  ];

  return (
    <div className="flex min-h-screen bg-slate-950 text-slate-100 w-full font-sans">
      {/* Dark Dense Sidebar Rail */}
      <aside className="w-16 bg-slate-900 border-r border-slate-800 flex flex-col items-center py-5 justify-between sticky top-0 h-screen select-none shrink-0 z-40">
        <div className="flex flex-col items-center gap-6 w-full px-2">
          <Link
            to="/"
            title="VitalWatch Home"
            className="w-10 h-10 bg-brand-600 hover:bg-brand-500 rounded-lg flex items-center justify-center text-white font-bold transition-colors"
          >
            V
          </Link>

          <nav className="flex flex-col items-center gap-2 w-full">
            {navItems.map((item) => {
              const Icon = item.icon;
              return (
                <Link
                  key={item.to}
                  to={item.to}
                  title={item.title}
                  className={`relative w-10 h-10 rounded-lg flex items-center justify-center transition-all ${
                    item.isActive
                      ? 'bg-brand-500/10 text-brand-400 border border-brand-500/30'
                      : 'text-slate-400 hover:text-slate-200 hover:bg-slate-800'
                  }`}
                >
                  <Icon className="w-5 h-5" />
                  {item.badge !== undefined && (
                    <span className="absolute -top-1 -right-1 min-w-4 h-4 px-1 rounded-full bg-amber-500 text-slate-950 text-[10px] font-mono font-bold flex items-center justify-center shadow-xs">
                      {item.badge}
                    </span>
                  )}
                </Link>
              );
            })}
          </nav>
        </div>

        {/* Clinician Profile & Logout */}
        <div className="flex flex-col items-center gap-3 w-full px-2">
          <div
            title={doctor ? `Dr. ${doctor.first_name} ${doctor.last_name} (${doctor.specialty || 'Clinician'})` : 'Doctor'}
            className="w-9 h-9 rounded-lg bg-slate-800 border border-slate-700 flex items-center justify-center text-xs font-mono font-bold text-slate-200"
          >
            {doctor?.first_name ? `Dr` : 'MD'}
          </div>

          <button
            onClick={handleSignOut}
            type="button"
            title="Sign out of workspace"
            className="w-10 h-10 rounded-lg flex items-center justify-center text-slate-400 hover:text-rose-400 hover:bg-rose-500/10 transition-colors cursor-pointer"
          >
            <LogOut className="w-5 h-5" />
          </button>
        </div>
      </aside>

      {/* Main Workspace Surface */}
      <div className="flex-1 flex flex-col bg-slate-50 text-slate-900 min-w-0">
        {/* Clinician Top Bar */}
        <header className="h-14 bg-white border-b border-border px-6 flex items-center justify-between shrink-0">
          <div className="flex items-center gap-3">
            <span className="font-mono text-xs font-semibold uppercase tracking-wider text-slate-500 bg-slate-100 px-2 py-0.5 rounded border border-slate-200">
              Clinical Workspace
            </span>
            <span className="text-slate-300">•</span>
            <span className="text-xs text-slate-600 font-medium">
              {doctor ? `Dr. ${doctor.first_name} ${doctor.last_name} — ${doctor.specialty || 'Staff Physician'}` : 'Signed In'}
            </span>
          </div>

          <div className="flex items-center gap-4 text-xs text-slate-500 font-mono">
            <span className="inline-flex items-center gap-1.5 text-emerald-700 bg-emerald-50 px-2 py-0.5 rounded border border-emerald-200">
              <span className="w-1.5 h-1.5 rounded-full bg-emerald-500 animate-pulse" />
              HIPAA Encrypted
            </span>
          </div>
        </header>

        <main className="flex-1 p-6 md:p-8 overflow-auto">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
