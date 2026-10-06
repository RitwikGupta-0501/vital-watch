import { Link } from 'react-router-dom'
import { Button } from '../components/ui/Button'

export function Landing() {
  return (
    <div className="flex-1 flex items-center justify-center p-8">
      <div className="max-w-3xl text-center space-y-8">
        <h1 className="text-5xl font-bold tracking-tight text-slate-900">
          Clinical intelligence, <br/> built for both sides of care.
        </h1>
        <p className="text-xl text-slate-500 max-w-2xl mx-auto">
          VitalWatch is a modern platform that connects patients and doctors through a unified system, enforcing clinical safety without sacrificing consumer experience.
        </p>
        
        <div className="pt-8 flex flex-col sm:flex-row items-center justify-center gap-6">
          <Link to="/login?role=patient" className="w-full sm:w-auto">
            <Button size="lg" isPatientTheme className="w-full rounded-[2rem] px-8">
              For Patients
            </Button>
          </Link>
          <Link to="/login?role=doctor" className="w-full sm:w-auto">
            <Button size="lg" variant="outline" className="w-full px-8">
              For Doctors & Clinics
            </Button>
          </Link>
        </div>
      </div>
    </div>
  )
}
