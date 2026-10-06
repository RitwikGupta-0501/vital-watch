import { Outlet } from 'react-router-dom'

export function PatientLayout() {
  return (
    <div className="flex min-h-screen bg-surface w-full">
        {/* Placeholder for real Sidebar */}
        <aside className="w-64 bg-white border-r border-border flex flex-col sticky top-0 h-screen p-6">
            <div className="flex items-center gap-3 mb-8">
                <div className="w-8 h-8 bg-brand-700 rounded-lg flex items-center justify-center text-white font-bold text-xl">V</div>
                <span className="font-bold text-xl tracking-tight">VitalWatch</span>
            </div>
            <nav className="flex-1 space-y-2">
                <div className="px-4 py-3 bg-brand-50 text-brand-700 rounded-xl font-semibold">Dashboard</div>
            </nav>
        </aside>
        <main className="flex-1 p-10 max-w-5xl mx-auto">
            <Outlet />
        </main>
    </div>
  )
}
