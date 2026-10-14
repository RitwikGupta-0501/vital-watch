import { useState } from 'react';
import { useSearchParams, useNavigate, Link } from 'react-router-dom';
import { useAuth } from '../context/useAuth';
import { Button } from '../components/ui/Button';
import { Card } from '../components/ui/Card';
import { Input } from '../components/ui/Input';
import { Label } from '../components/ui/Label';
import type { RegisterRequest } from '../lib/api/types';
import { RateLimitError } from '../lib/api/client';
import {
  AlertCircle,
  CheckCircle2,
  Stethoscope,
  User,
} from 'lucide-react';

export function Register() {
  const [searchParams, setSearchParams] = useSearchParams();
  const navigate = useNavigate();
  const { register } = useAuth();

  const roleParam = searchParams.get('role');
  const role = roleParam === 'doctor' ? 'doctor' : 'patient';
  const isPatient = role === 'patient';

  const [firstName, setFirstName] = useState('');
  const [lastName, setLastName] = useState('');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [specialty, setSpecialty] = useState('');
  const [experience, setExperience] = useState<number>(5);
  const [inviteCode, setInviteCode] = useState('');

  const [isSubmitting, setIsSubmitting] = useState(false);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [success, setSuccess] = useState(false);
  const [rateLimitSeconds, setRateLimitSeconds] = useState<number | null>(null);

  const handleRoleChange = (newRole: 'patient' | 'doctor') => {
    setSearchParams({ role: newRole });
    setErrorMessage(null);
  };

  const validatePasswordClient = (pwd: string): string | null => {
    if (pwd.length < 8) return 'Password must be at least 8 characters long';
    if (!/[A-Z]/.test(pwd)) return 'Password must contain at least one uppercase letter';
    if (!/[a-z]/.test(pwd)) return 'Password must contain at least one lowercase letter';
    if (!/[0-9]/.test(pwd)) return 'Password must contain at least one digit';
    if (!/[^A-Za-z0-9]/.test(pwd)) return 'Password must contain at least one special character';
    return null;
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (rateLimitSeconds !== null) return;

    setErrorMessage(null);

    const pwdError = validatePasswordClient(password);
    if (pwdError) {
      setErrorMessage(pwdError);
      return;
    }

    if (!isPatient && !inviteCode.trim()) {
      setErrorMessage('A verified clinic invite code is required to register as a clinician.');
      return;
    }

    setIsSubmitting(true);

    try {
      let payload: RegisterRequest;
      if (isPatient) {
        payload = {
          role: 'patient',
          first_name: firstName.trim(),
          last_name: lastName.trim(),
          email: email.trim().toLowerCase(),
          password,
        };
      } else {
        payload = {
          role: 'doctor',
          first_name: firstName.trim(),
          last_name: lastName.trim(),
          email: email.trim().toLowerCase(),
          password,
          specialty: specialty.trim() || 'General Medicine',
          experience: Number(experience) || 1,
          invite_code: inviteCode.trim(),
        };
      }

      await register(payload);
      setSuccess(true);
      setTimeout(() => {
        navigate(`/login?role=${role}`, { replace: true });
      }, 1500);
    } catch (err: unknown) {
      if (err instanceof RateLimitError) {
        setRateLimitSeconds(err.retryAfterSeconds);
        setErrorMessage(
          `Registration rate limited. Please wait ${err.retryAfterSeconds}s before retrying.`
        );
      } else if (err instanceof Error) {
        setErrorMessage(err.message);
      } else {
        setErrorMessage('Failed to create account. Please verify your details.');
      }
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <div className="flex-1 flex flex-col items-center justify-center p-6 bg-slate-50 min-h-[calc(100vh-80px)]">
      <div className="w-full max-w-lg space-y-6">
        {/* Role Toggle */}
        <div className="bg-slate-200/80 p-1 rounded-xl flex">
          <button
            type="button"
            onClick={() => handleRoleChange('patient')}
            className={`flex-1 py-2 text-sm font-semibold rounded-lg flex items-center justify-center gap-2 transition-all ${
              isPatient
                ? 'bg-white text-slate-900 shadow-sm'
                : 'text-slate-600 hover:text-slate-900'
            }`}
          >
            <User className="w-4 h-4" />
            <span>Patient Account</span>
          </button>
          <button
            type="button"
            onClick={() => handleRoleChange('doctor')}
            className={`flex-1 py-2 text-sm font-semibold rounded-lg flex items-center justify-center gap-2 transition-all ${
              !isPatient
                ? 'bg-white text-slate-900 shadow-sm'
                : 'text-slate-600 hover:text-slate-900'
            }`}
          >
            <Stethoscope className="w-4 h-4" />
            <span>Clinician Account</span>
          </button>
        </div>

        <div className="text-center">
          <h2 className="text-3xl font-bold tracking-tight text-slate-900">
            {isPatient ? 'Create Patient Profile' : 'Register Clinical Provider'}
          </h2>
          <p className="mt-2 text-sm text-slate-500">
            {isPatient
              ? 'Join VitalWatch to manage your medications, telehealth, and vitals.'
              : 'Authorized healthcare providers join through validated clinic invites.'}
          </p>
        </div>

        {success ? (
          <Card className="p-8 bg-white border border-border rounded-2xl text-center space-y-4">
            <div className="w-12 h-12 rounded-full bg-emerald-50 text-emerald-600 mx-auto flex items-center justify-center">
              <CheckCircle2 className="w-6 h-6" />
            </div>
            <h3 className="text-xl font-bold text-slate-900">Account Created Successfully</h3>
            <p className="text-sm text-slate-600">
              Redirecting you to the sign-in portal...
            </p>
          </Card>
        ) : (
          <>
            {errorMessage && (
              <div className="bg-rose-50 border border-rose-200 p-4 rounded-xl flex items-start gap-3 text-rose-800">
                <AlertCircle className="w-5 h-5 text-rose-600 shrink-0 mt-0.5" />
                <div className="text-sm">
                  <p className="font-semibold">Unable to complete registration</p>
                  <p className="mt-0.5 text-xs text-rose-700 leading-relaxed">{errorMessage}</p>
                </div>
              </div>
            )}

            <Card className={`p-8 bg-white border border-border ${isPatient ? 'rounded-[2rem] shadow-sm' : 'rounded-xl shadow-none'}`}>
              <form onSubmit={handleSubmit} className="space-y-4">
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                  <div className="space-y-1.5">
                    <Label htmlFor="first_name">First name</Label>
                    <Input
                      id="first_name"
                      required
                      value={firstName}
                      onChange={(e) => setFirstName(e.target.value)}
                      placeholder="Jane"
                      disabled={isSubmitting}
                    />
                  </div>
                  <div className="space-y-1.5">
                    <Label htmlFor="last_name">Last name</Label>
                    <Input
                      id="last_name"
                      required
                      value={lastName}
                      onChange={(e) => setLastName(e.target.value)}
                      placeholder="Doe"
                      disabled={isSubmitting}
                    />
                  </div>
                </div>

                <div className="space-y-1.5">
                  <Label htmlFor="email">Email address</Label>
                  <Input
                    id="email"
                    type="email"
                    required
                    value={email}
                    onChange={(e) => setEmail(e.target.value)}
                    placeholder={isPatient ? 'jane.doe@example.com' : 'j.doe@clinic.org'}
                    autoComplete="email"
                    disabled={isSubmitting}
                  />
                </div>

                <div className="space-y-1.5">
                  <Label htmlFor="password">Password</Label>
                  <Input
                    id="password"
                    type="password"
                    required
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    placeholder="••••••••••••"
                    autoComplete="new-password"
                    disabled={isSubmitting}
                  />
                  <p className="text-[11px] text-slate-400">
                    Min 8 characters, including uppercase, lowercase, number, and special character.
                  </p>
                </div>

                {!isPatient && (
                  <div className="pt-2 border-t border-slate-100 space-y-4">
                    <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                      <div className="space-y-1.5">
                        <Label htmlFor="specialty">Medical Specialty</Label>
                        <Input
                          id="specialty"
                          required
                          value={specialty}
                          onChange={(e) => setSpecialty(e.target.value)}
                          placeholder="e.g. Cardiology"
                          disabled={isSubmitting}
                        />
                      </div>
                      <div className="space-y-1.5">
                        <Label htmlFor="experience">Years of Experience</Label>
                        <Input
                          id="experience"
                          type="number"
                          min="0"
                          max="60"
                          required
                          value={experience}
                          onChange={(e) => setExperience(Number(e.target.value))}
                          disabled={isSubmitting}
                        />
                      </div>
                    </div>

                    <div className="space-y-1.5">
                      <div className="flex items-center justify-between">
                        <Label htmlFor="inviteCode">Clinic Invite Code</Label>
                        <span className="text-[11px] font-mono text-brand-600 bg-brand-50 px-2 py-0.5 rounded">
                          Required
                        </span>
                      </div>
                      <Input
                        id="inviteCode"
                        required
                        value={inviteCode}
                        onChange={(e) => setInviteCode(e.target.value)}
                        placeholder="Provided by clinic administrator"
                        disabled={isSubmitting}
                      />
                    </div>
                  </div>
                )}

                <Button
                  type="submit"
                  className="w-full mt-4"
                  size="lg"
                  isPatientTheme={isPatient}
                  disabled={isSubmitting || rateLimitSeconds !== null}
                >
                  {isSubmitting
                    ? 'Creating Account...'
                    : isPatient
                    ? 'Register Patient Account'
                    : 'Register Clinician Profile'}
                </Button>
              </form>
            </Card>
          </>
        )}

        <div className="text-center">
          <p className="text-sm text-slate-500">
            Already registered?{' '}
            <Link
              to={`/login?role=${role}`}
              className="font-semibold text-brand-600 hover:text-brand-700 hover:underline"
            >
              Sign in to your account
            </Link>
          </p>
        </div>
      </div>
    </div>
  );
}
