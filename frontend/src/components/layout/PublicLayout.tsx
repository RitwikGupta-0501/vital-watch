import { Outlet, Link } from 'react-router-dom'

export function PublicLayout() {
  return (
    <div className="min-h-screen bg-slate-50 flex flex-col font-sans">
      <header className="px-8 py-6 flex items-center justify-between border-b border-border bg-white">
        <Link to="/" className="flex items-center gap-3">
          <div className="w-8 h-8 bg-brand-700 rounded-lg flex items-center justify-center text-white font-bold text-xl">V</div>
          <span className="font-bold text-xl tracking-tight text-slate-900">VitalWatch</span>
        </Link>
        <div className="flex gap-4">
            <Link to="/login?role=patient" className="text-sm font-semibold text-slate-600 hover:text-slate-900 px-3 py-2">Patient Login</Link>
            <Link to="/login?role=doctor" className="text-sm font-semibold text-slate-600 hover:text-slate-900 px-3 py-2">Doctor Login</Link>
        </div>
      </header>
      <main className="flex-1 flex flex-col">
        <Outlet />
      </main>
    </div>
  )
}
