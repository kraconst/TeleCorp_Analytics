import { apiRequest, withCompany } from './client';
import type { Employee } from '@/types/api';

export interface EmployeeInput {
  name: string;
  position: string;
}

export const listEmployees = (companyId: number, signal?: AbortSignal) =>
  apiRequest<Employee[]>(withCompany('/employees', companyId), { signal }).then((data) => data ?? []);

export const createEmployee = (companyId: number, input: EmployeeInput, signal?: AbortSignal) =>
  apiRequest<Employee>(withCompany('/employees', companyId), {
    method: 'POST',
    body: { company_id: companyId, ...input },
    signal,
  });

export const updateEmployee = (
  companyId: number,
  employeeId: number,
  input: EmployeeInput,
  signal?: AbortSignal,
) =>
  apiRequest<{ status: string }>(withCompany(`/employees/${employeeId}`, companyId), {
    method: 'PUT',
    body: input,
    signal,
  });

export const deactivateEmployee = (companyId: number, employeeId: number, signal?: AbortSignal) =>
  apiRequest<{ status: string }>(withCompany(`/employees/${employeeId}/deactivate`, companyId), {
    method: 'POST',
    signal,
  });
