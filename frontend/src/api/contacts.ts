import { apiRequest, withCompany } from './client';
import type { Contact } from '@/types/api';

export const listContacts = (companyId: number, signal?: AbortSignal) =>
  apiRequest<Contact[]>(withCompany('/contacts', companyId), { signal }).then((data) => data ?? []);

export const getContact = (companyId: number, contactId: number, signal?: AbortSignal) =>
  apiRequest<Contact>(withCompany(`/contacts/${contactId}`, companyId), { signal });
