import React, { useState, useEffect } from 'react';
import { useSearchParams, useNavigate, Link } from 'react-router-dom';
import { useAuth } from '../context/useAuth';
import { Button } from '../components/ui/Button';
import { Card } from '../components/ui/Card';
import { Input } from '../components/ui/Input';
import { Label } from '../components/ui/Label';
import { RateLimitError } from '../lib/api/client';
import { AlertCircle, Clock } from 'lucide-react';

export function Login() {
  const [searchParams, setSearchParams] = useSearchParams();
  const navigate = useNavigate();
  const { login, isAuthenticated, user } = useAuth();

  const roleParam = searchParams.get('role');
  const role = roleParam === 'doctor' ? 'doctor' : 'patient';
  const isPatient = role === 'patient';
  const returnUrl = searchParams.get('returnUrl');

  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [rateLimitSeconds, setRateLimitSeconds] = useState<number | null>(null);

  // If already logged in, redirect to respective dashboard
  useEffect(() => {
    if (isAuthenticated && user) {
      if (returnUrl) {
        navigate(decodeURIComponent(returnUrl), { replace: true });
      } else if (user.role === 'doctor') {
        navigate('/doctor/dashboard', { replace: true });
      } else {
        navigate('/patient/dashboard', { replace: true });
      }
    }
  }, [isAuthenticated, user, navigate, returnUrl]);

  // Rate limit countdown timer
  useEffect(() => {
    if (rateLimitSeconds === null || rateLimitSeconds <= 0) return;
    const interval = setInterval(() => {
      setRateLimitSeconds((prev) => {
        if (prev === null || prev <= 1) return null;
        return prev - 1;
      });
    }, 1000);
    return () => clearInterval(interval);
  }, [rateLimitSeconds]);

  const handleRoleChange = (newRole: 'patient' | 'doctor') => {
    setSearchParams({ role: newRole });
    setErrorMessage(null);
  };

  const handleLogin = async (e: React.FormEvent) => {
    e.preventDefault();
    if (rateLimitSeconds !== null) return;

    setIsSubmitting(true);
    setErrorMessage(null);

    try {
      await login({
        role,
        email,
        password,
      });

      if (returnUrl) {
        navigate(decodeURIComponent(returnUrl), { replace: true });
      } else if (role === 'doctor') {
        navigate('/doctor/dashboard', { replace: true });
      } else {
        navigate('/patient/dashboard', { replace: true });
      }
    } catch (err: unknown) {
      if (err instanceof RateLimitError) {
        setRateLimitSeconds(err.retryAfterSeconds);
        setErrorMessage(
          `Security rate limit engaged. Too many login attempts. Please wait ${err.retryAfterSeconds} seconds.`
        );
      } else {
        const message = err instanceof Error ? err.message : 'Invalid credentials';
        setErrorMessage(message);
      }
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <div className="flex-1 flex flex-col items-center justify-center p-6 bg-slate-50 min-h-[calc(100vh-80px)]">
      <div className="w-full max-w-md space-y-6">
        {/* Role Toggle Selector */}
        <div className="bg-slate-200/80 p-1 rounded-xl flex">
          <button
            type="button"
            onClick={() => handleRoleChange('patient')}
            className={`flex-1 py-2 text-sm font-semibold rounded-lg transition-all ${
              isPatient
                ? 'bg-white text-slate-900 shadow-sm'
                : 'text-slate-600 hover:text-slate-900'
            }`}
          >
            Patient Portal
          </button>
          <button
            type="button"
            onClick={() => handleRoleChange('doctor')}
            className={`flex-1 py-2 text-sm font-semibold rounded-lg transition-all ${
              !isPatient
                ? 'bg-white text-slate-900 shadow-sm'
                : 'text-slate-600 hover:text-slate-900'
            }`}
          >
            Clinician Workspace
          </button>
        </div>

        <div className="text-center">
          <h2 className="text-3xl font-bold tracking-tight text-slate-900">
            {isPatient ? 'Welcome to VitalWatch' : 'Clinician Authentication'}
          </h2>
          <p className="mt-2 text-sm text-slate-500">
            {isPatient
              ? 'Sign in to access your prescriptions, vitals, and appointments.'
              : 'Sign in with your verified clinical credentials.'}
          </p>
        </div>

        {/* Rate Limit Notice */}
        {rateLimitSeconds !== null && (
          <div className="bg-amber-50 border border-amber-200 p-4 rounded-xl flex items-start gap-3">
            <Clock className="w-5 h-5 text-amber-600 shrink-0 mt-0.5" />
            <div>
              <p className="text-sm font-semibold text-amber-900">
                Rate Limit Cooling Period
              </p>
              <p className="text-xs text-amber-800 mt-1">
                Please wait <span className="font-mono font-bold">{rateLimitSeconds}s</span> before attempting another sign-in.
              </p>
            </div>
          </div>
        )}

        {/* General Error Notice */}
        {errorMessage && rateLimitSeconds === null && (
          <div className="bg-rose-50 border border-rose-200 p-4 rounded-xl flex items-start gap-3 text-rose-800">
            <AlertCircle className="w-5 h-5 text-rose-600 shrink-0 mt-0.5" />
            <div className="text-sm">
              <p className="font-semibold">Sign-in failed</p>
              <p className="mt-0.5 text-xs text-rose-700 leading-relaxed">{errorMessage}</p>
              {errorMessage.toLowerCase().includes('credential') && (
                <p className="mt-2 text-xs text-rose-600">
                  Tip: Ensure you have selected the correct portal ({isPatient ? 'Patient' : 'Clinician'}).
                </p>
              )}
            </div>
          </div>
        )}

        <Card className={`p-8 bg-white border border-border ${isPatient ? 'rounded-[2rem] shadow-sm' : 'rounded-xl shadow-none'}`}>
          <form onSubmit={handleLogin} className="space-y-5">
            <div className="space-y-2">
              <Label htmlFor="email">Email address</Label>
              <Input
                id="email"
                type="email"
                required
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder={isPatient ? 'patient@example.com' : 'doctor@clinic.org'}
                autoComplete="email"
                disabled={isSubmitting || rateLimitSeconds !== null}
              />
            </div>

            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <Label htmlFor="password">Password</Label>
              </div>
              <Input
                id="password"
                type="password"
                required
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                autoComplete="current-password"
                disabled={isSubmitting || rateLimitSeconds !== null}
              />
            </div>

            <Button
              type="submit"
              className="w-full"
              size="lg"
              isPatientTheme={isPatient}
              disabled={isSubmitting || rateLimitSeconds !== null}
            >
              {isSubmitting ? 'Authenticating...' : isPatient ? 'Sign In as Patient' : 'Sign In to Workspace'}
            </Button>
          </form>
        </Card>

        <div className="text-center space-y-2">
          <p className="text-sm text-slate-500">
            Don't have an account?{' '}
            <Link
              to={`/register?role=${role}`}
              className="font-semibold text-brand-600 hover:text-brand-700 hover:underline"
            >
              Create {isPatient ? 'patient account' : 'clinician profile'}
            </Link>
          </p>

          <div className="pt-4 flex items-center justify-center gap-6 text-xs text-slate-400">
            <Link to="/notice-of-privacy-practices" className="hover:text-slate-600">Privacy Notice</Link>
            <span>•</span>
            <Link to="/terms" className="hover:text-slate-600">Terms of Care</Link>
            <span>•</span>
            <Link to="/security" className="hover:text-slate-600">HIPAA Security</Link>
          </div>
        </div>
      </div>
    </div>
  );
}
