import { useState, useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { fetchApi } from '../../lib/api/client';
import type { Doctor, PaginatedResponse } from '../../lib/api/types';
import { Card } from '../../components/ui/Card';
import { Button } from '../../components/ui/Button';
import { EmptyState } from '../../components/ui/EmptyState';
import {
  Search,
  Stethoscope,
  Award,
  Calendar,
} from 'lucide-react';

export function DoctorDirectory() {
  const [searchQuery, setSearchQuery] = useState('');
  const [page, setPage] = useState(0);
  const limit = 12;

  const { data, isLoading, error } = useQuery({
    queryKey: ['doctors-directory', limit, page * limit],
    queryFn: () =>
      fetchApi<PaginatedResponse<Doctor>>(`/api/doctors?limit=${limit}&offset=${page * limit}`),
  });

  const doctors = data?.data || [];

  // Filter doctors by name or specialty
  const filteredDoctors = useMemo(() => {
    if (!searchQuery.trim()) return doctors;
    const q = searchQuery.toLowerCase();
    return doctors.filter(
      (d) =>
        d.first_name.toLowerCase().includes(q) ||
        d.last_name.toLowerCase().includes(q) ||
        (d.specialty && d.specialty.toLowerCase().includes(q))
    );
  }, [doctors, searchQuery]);

  return (
    <div className="space-y-8 animate-in fade-in duration-300">
      {/* Editorial Header */}
      <div className="flex flex-col md:flex-row md:items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl sm:text-3xl font-bold tracking-tight text-slate-900">
            Clinical Specialists
          </h1>
          <p className="text-sm text-slate-500 mt-1 max-w-xl">
            Discover verified healthcare providers, view available practice hours, and reserve consultation slots.
          </p>
        </div>

        {/* Search Bar */}
        <div className="relative w-full md:w-72">
          <Search className="w-4 h-4 text-slate-400 absolute left-3.5 top-1/2 -translate-y-1/2" />
          <input
            type="text"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            placeholder="Search name or specialty..."
            className="w-full pl-9 pr-4 py-2 text-sm bg-white border border-border rounded-xl focus:outline-none focus:ring-2 focus:ring-brand-700/20 focus:border-brand-700 transition-all placeholder:text-slate-400"
          />
        </div>
      </div>

      {/* Directory Grid */}
      {isLoading ? (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
          {Array.from({ length: 6 }).map((_, i) => (
            <Card key={i} className="rounded-3xl p-6 bg-white border border-border animate-pulse space-y-4">
              <div className="w-12 h-12 bg-slate-100 rounded-full" />
              <div className="h-5 bg-slate-100 rounded w-1/2" />
              <div className="h-4 bg-slate-100 rounded w-1/3" />
              <div className="h-10 bg-slate-100 rounded-full mt-4" />
            </Card>
          ))}
        </div>
      ) : error ? (
        <EmptyState
          variant="error"
          title="Unable to Load Directory"
          description="Failed to retrieve clinical practitioners. Please verify your connection."
          actionLabel="Retry"
          onAction={() => window.location.reload()}
          isPatientTheme
        />
      ) : filteredDoctors.length > 0 ? (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
          {filteredDoctors.map((doc) => {
            const expYears = doc.experience_years ?? doc.experience ?? 0;
            return (
              <Card
                key={doc.id}
                className="rounded-3xl p-6 bg-white border border-border hover:border-brand-300 hover:shadow-xs transition-all flex flex-col justify-between"
              >
                <div className="space-y-4">
                  <div className="flex items-start justify-between gap-3">
                    <div className="w-12 h-12 rounded-2xl bg-brand-50 text-brand-800 flex items-center justify-center font-bold text-base shrink-0">
                      {doc.first_name[0]}
                      {doc.last_name[0]}
                    </div>
                    {doc.available ? (
                      <span className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-[11px] font-medium bg-emerald-50 text-emerald-800 border border-emerald-200">
                        <span className="w-1.5 h-1.5 rounded-full bg-emerald-600 animate-pulse" />
                        Available
                      </span>
                    ) : (
                      <span className="inline-flex items-center gap-1 px-2.5 py-0.5 rounded-full text-[11px] font-medium bg-slate-100 text-slate-600 border border-slate-200">
                        Unavailable
                      </span>
                    )}
                  </div>

                  <div>
                    <h3 className="text-lg font-bold text-slate-900 tracking-tight">
                      Dr. {doc.first_name} {doc.last_name}
                    </h3>
                    <p className="text-xs font-semibold text-brand-700 mt-0.5 flex items-center gap-1.5">
                      <Stethoscope className="w-3.5 h-3.5" />
                      <span>{doc.specialty || 'General Practitioner'}</span>
                    </p>
                  </div>

                  <div className="pt-3 border-t border-slate-100 flex items-center gap-2 text-xs text-slate-500">
                    <Award className="w-4 h-4 text-slate-400" />
                    <span>
                      {expYears > 0 ? `${expYears} years practice` : 'Licensed Physician'}
                    </span>
                  </div>
                </div>

                <div className="mt-6 pt-2">
                  {doc.available ? (
                    <Link to={`/patient/doctors/${doc.id}`}>
                      <Button
                        isPatientTheme
                        size="md"
                        className="w-full gap-2 font-medium"
                      >
                        <Calendar className="w-4 h-4" />
                        <span>View Schedule & Book</span>
                      </Button>
                    </Link>
                  ) : (
                    <Button
                      variant="outline"
                      isPatientTheme
                      size="md"
                      disabled
                      className="w-full text-xs text-slate-400 cursor-not-allowed"
                    >
                      Not Accepting Bookings
                    </Button>
                  )}
                </div>
              </Card>
            );
          })}
        </div>
      ) : searchQuery ? (
        <EmptyState
          variant="no-records"
          title="No Clinicians Found"
          description={`No specialists match the query "${searchQuery}". Try searching for another name or medical specialty.`}
          actionLabel="Clear Filter"
          onAction={() => setSearchQuery('')}
          isPatientTheme
        />
      ) : (
        <EmptyState
          variant="no-records"
          title="Directory Empty"
          description="There are currently no clinicians registered in this facility."
          isPatientTheme
        />
      )}

      {/* Pagination Controls */}
      {doctors.length >= limit && (
        <div className="flex items-center justify-between pt-6 border-t border-border">
          <Button
            variant="outline"
            size="sm"
            isPatientTheme
            disabled={page === 0}
            onClick={() => setPage((p) => Math.max(0, p - 1))}
          >
            Previous
          </Button>
          <span className="text-xs font-mono text-slate-500">
            Page {page + 1}
          </span>
          <Button
            variant="outline"
            size="sm"
            isPatientTheme
            disabled={doctors.length < limit}
            onClick={() => setPage((p) => p + 1)}
          >
            Next
          </Button>
        </div>
      )}
    </div>
  );
}
