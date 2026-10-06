import { Outlet } from 'react-router-dom'

export function DoctorLayout() {
  return (
    <div className="flex min-h-screen bg-white w-full">
        {/* Dark dense sidebar */}
        <aside className="w-16 bg-slate-900 border-r border-slate-800 flex flex-col items-center py-4 sticky top-0 h-screen">
            <div className="w-10 h-10 bg-brand-600 rounded-lg flex items-center justify-center text-white font-bold mb-8">V</div>
            <nav className="flex-1 flex flex-col gap-4 w-full px-2">
                <div className="w-full aspect-square bg-brand-700/20 text-brand-400 rounded-xl flex items-center justify-center border-l-4 border-brand-500">
                    <span className="text-xs">Dash</span>
                </div>
            </nav>
        </aside>
        <main className="flex-1 flex bg-surface">
            <Outlet />
        </main>
    </div>
  )
}
