import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { fetchApi } from '../../lib/api/client';
import type { DoctorPatient, PaginatedResponse } from '../../lib/api/types';
import { formatShortDate } from '../../lib/datetime';
import { EmptyState } from '../../components/ui/EmptyState';
import { FileText, Search, ChevronLeft, ChevronRight, Activity } from 'lucide-react';

export function DoctorPatientRoster() {
  const [searchQuery, setSearchQuery] = useState('');
  const [page, setPage] = useState(0);
  const limit = 20;

  const { data, isLoading, isError, refetch } = useQuery<PaginatedResponse<DoctorPatient>>({
    queryKey: ['doctor-patients', page],
    queryFn: () =>
      fetchApi<PaginatedResponse<DoctorPatient>>(`/api/doctor/patients?limit=${limit}&offset=${page * limit}`),
  });

  const patients = data?.data || [];

  const filteredPatients = patients.filter((p) => {
    if (!searchQuery.trim()) return true;
    const q = searchQuery.toLowerCase();
    const fullName = `${p.first_name} ${p.last_name}`.toLowerCase();
    const email = p.email.toLowerCase();
    const id = p.id.toLowerCase();
    return fullName.includes(q) || email.includes(q) || id.includes(q);
  });

  return (
    <div className="space-y-6 max-w-6xl mx-auto">
      {/* Header */}
      <div className="flex flex-col md:flex-row md:items-center justify-between gap-4 pb-6 border-b border-slate-200">
        <div>
          <span className="font-mono text-xs uppercase tracking-wider text-slate-500 font-semibold">
            Clinical Roster • Screen C3
          </span>
          <h1 className="text-2xl font-bold tracking-tight text-slate-900 mt-1">
            Associated Patient Roster
          </h1>
          <p className="text-sm text-slate-500 mt-0.5">
            Relationship-verified patient directory with access to longitudinal charts and vitals
          </p>
        </div>

        <div className="flex items-center gap-2">
          <div className="relative">
            <Search className="w-3.5 h-3.5 text-slate-400 absolute left-3 top-1/2 -translate-y-1/2" />
            <input
              type="text"
              placeholder="Search patients by name or ID..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="pl-8 pr-3 py-1.5 text-xs bg-white border border-slate-200 rounded-md focus:outline-none focus:ring-1 focus:ring-slate-400 w-60 md:w-72 font-mono shadow-2xs"
            />
          </div>
        </div>
      </div>

      {/* Roster Table Card */}
      <div className="bg-white border border-slate-200 rounded-lg shadow-2xs overflow-hidden">
        {isLoading ? (
          <div className="p-8 space-y-3">
            {[1, 2, 3, 4].map((i) => (
              <div key={i} className="h-12 bg-slate-100 rounded animate-pulse" />
            ))}
          </div>
        ) : isError ? (
          <div className="p-8">
            <EmptyState
              variant="error"
              title="Unable to Load Patient Roster"
              description="Could not synchronize clinical patient records. Please retry."
              actionLabel="Retry"
              onAction={() => refetch()}
            />
          </div>
        ) : filteredPatients.length === 0 ? (
          <div className="p-8">
            <EmptyState
              variant="no-records"
              title={searchQuery ? 'No Matching Patients' : 'No Patients in Roster'}
              description={
                searchQuery
                  ? 'No patient records matched your active search query.'
                  : 'Patients appear in your clinical roster once an appointment is scheduled or completed.'
              }
              actionLabel={searchQuery ? 'Clear Search' : undefined}
              onAction={searchQuery ? () => setSearchQuery('') : undefined}
            />
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left border-collapse text-xs">
              <thead>
                <tr className="border-b border-slate-200 bg-slate-50/75 text-slate-500 font-mono uppercase tracking-wider">
                  <th className="py-3 px-4 font-semibold">Patient</th>
                  <th className="py-3 px-4 font-semibold">Email</th>
                  <th className="py-3 px-4 font-semibold">Clinical UUID</th>
                  <th className="py-3 px-4 font-semibold">Joined</th>
                  <th className="py-3 px-4 font-semibold text-right">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100">
                {filteredPatients.map((patient) => {
                  const initials = `${patient.first_name[0] || ''}${patient.last_name[0] || ''}`.toUpperCase();

                  return (
                    <tr key={patient.id} className="hover:bg-slate-50/60 transition-colors">
                      {/* Name & Avatar */}
                      <td className="py-3.5 px-4">
                        <div className="flex items-center gap-3">
                          <div className="w-8 h-8 rounded-lg bg-slate-100 border border-slate-200 flex items-center justify-center font-bold text-slate-700 text-xs shrink-0 font-mono">
                            {initials}
                          </div>
                          <div>
                            <span className="font-semibold text-slate-900">
                              {patient.first_name} {patient.last_name}
                            </span>
                            <div className="text-[11px] text-slate-400 capitalize">Active Patient</div>
                          </div>
                        </div>
                      </td>

                      {/* Email */}
                      <td className="py-3.5 px-4 font-mono text-slate-600">
                        {patient.email}
                      </td>

                      {/* UUID */}
                      <td className="py-3.5 px-4 font-mono text-slate-400 text-[11px]">
                        {patient.id}
                      </td>

                      {/* Joined Date */}
                      <td className="py-3.5 px-4 text-slate-500 font-mono text-[11px]">
                        {formatShortDate(patient.created_at)}
                      </td>

                      {/* Actions */}
                      <td className="py-3.5 px-4 text-right">
                        <div className="flex items-center justify-end gap-2">
                          <Link
                            to={`/doctor/patients/${patient.id}/prescribe/new`}
                            className="inline-flex items-center gap-1 px-2.5 py-1 text-slate-700 bg-white border border-slate-200 rounded hover:bg-slate-50 transition-colors"
                            title="Prescribe medication"
                          >
                            <FileText className="w-3 h-3 text-slate-500" />
                            Prescribe
                          </Link>
                          <Link
                            to={`/doctor/patients/${patient.id}`}
                            className="inline-flex items-center gap-1 px-2.5 py-1 text-white bg-slate-900 rounded hover:bg-slate-800 transition-colors shadow-2xs font-medium"
                          >
                            <Activity className="w-3 h-3" />
                            Open Chart
                          </Link>
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}

        {/* Pagination footer */}
        <div className="py-3 px-4 bg-slate-50 border-t border-slate-200 flex items-center justify-between text-xs text-slate-500 font-mono">
          <span>
            Page {page + 1} • {filteredPatients.length} records shown
          </span>
          <div className="flex items-center gap-1">
            <button
              disabled={page === 0}
              onClick={() => setPage((p) => Math.max(0, p - 1))}
              className="p-1.5 border border-slate-200 rounded bg-white hover:bg-slate-50 disabled:opacity-40 disabled:pointer-events-none"
            >
              <ChevronLeft className="w-3.5 h-3.5" />
            </button>
            <button
              disabled={patients.length < limit}
              onClick={() => setPage((p) => p + 1)}
              className="p-1.5 border border-slate-200 rounded bg-white hover:bg-slate-50 disabled:opacity-40 disabled:pointer-events-none"
            >
              <ChevronRight className="w-3.5 h-3.5" />
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
