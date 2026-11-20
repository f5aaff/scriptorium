import { apiFetch } from './api';
import { API_BASE_URL } from './config';

export type ImportStatus = 'queued' | 'running' | 'completed' | 'completed_with_errors' | 'failed';

/** ImportFileResult is the public outcome of a single processed file. */
export interface ImportFileResult {
  filename: string;
  status: 'imported' | 'duplicate' | 'failed';
  document_id?: string;
  error?: string;
}

/** ImportJob contains durable progress; only the detail endpoint includes results. */
export interface ImportJob {
  id: string;
  status: ImportStatus;
  total: number;
  processed: number;
  failed: number;
  error?: string;
  created_at: string;
  updated_at: string;
  results?: ImportFileResult[];
}

/** isActiveImport reports whether a job still needs automatic progress updates. */
export function isActiveImport(job: ImportJob): boolean {
  return job.status === 'queued' || job.status === 'running';
}

export const importStatusLabels: Record<ImportStatus, string> = {
  queued: 'Queued', running: 'Running', completed: 'Completed',
  completed_with_errors: 'Completed with errors', failed: 'Failed'
};

/** listImportJobs retrieves the latest 25 job summaries from the shared catalogue. */
export async function listImportJobs(signal: AbortSignal): Promise<ImportJob[]> {
  const response = await apiFetch(`${API_BASE_URL}/file/imports/jobs`, { signal });
  if (!response.ok) throw new Error(`Unable to retrieve imports (${response.status}).`);
  const data = await response.json();
  return data.jobs || [];
}

/** getImportJob retrieves durable file outcomes without any private staging data. */
export async function getImportJob(id: string, signal: AbortSignal): Promise<ImportJob> {
  const response = await apiFetch(`${API_BASE_URL}/file/imports/jobs/${encodeURIComponent(id)}`, { signal });
  if (!response.ok) throw new Error(`Unable to retrieve import details (${response.status}).`);
  return (await response.json()).job;
}
