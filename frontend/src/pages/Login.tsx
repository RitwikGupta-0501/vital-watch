import { useSearchParams, useNavigate, Link } from 'react-router-dom'
import { Button } from '../components/ui/Button'
import { Card } from '../components/ui/Card'
import { Input } from '../components/ui/Input'
import { Label } from '../components/ui/Label'

export function Login() {
  const [searchParams] = useSearchParams()
  const navigate = useNavigate()
  const role = searchParams.get('role') || 'patient'
  const isPatient = role === 'patient'

  const handleLogin = (e: React.FormEvent) => {
    e.preventDefault()
    // TODO: Wire up actual auth client
    if (isPatient) {
      navigate('/patient/dashboard')
    } else {
      navigate('/doctor/dashboard')
    }
  }

  return (
    <div className="flex-1 flex flex-col items-center justify-center p-6 bg-slate-50">
      <div className="w-full max-w-md space-y-8">
        <div className="text-center">
          <h2 className="text-3xl font-bold tracking-tight text-slate-900">
            {isPatient ? 'Welcome back' : 'Doctor Workspace'}
          </h2>
          <p className="mt-2 text-sm text-slate-500">
            Sign in to your {isPatient ? 'patient account' : 'clinical account'} to continue.
          </p>
        </div>

        <Card className={`p-8 ${isPatient ? 'rounded-[2rem]' : 'rounded-xl'}`}>
          <form onSubmit={handleLogin} className="space-y-6">
            <div className="space-y-2">
              <Label htmlFor="email">Email address</Label>
              <Input id="email" type="email" required placeholder="you@example.com" />
            </div>
            
            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <Label htmlFor="password">Password</Label>
                <a href="#" className="text-sm font-semibold text-brand-600 hover:text-brand-700">Forgot password?</a>
              </div>
              <Input id="password" type="password" required />
            </div>

            <Button type="submit" className="w-full" size="lg" isPatientTheme={isPatient}>
              Sign In
            </Button>
          </form>
        </Card>

        <p className="text-center text-sm text-slate-500">
          {isPatient ? 'Are you a doctor? ' : 'Are you a patient? '}
          <Link 
            to={`/login?role=${isPatient ? 'doctor' : 'patient'}`} 
            className="font-semibold text-brand-600 hover:text-brand-700"
          >
            Sign in here
          </Link>
        </p>
      </div>
    </div>
  )
}
