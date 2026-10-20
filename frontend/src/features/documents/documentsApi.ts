import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, call } from '../../api/client';
import { downloadFile, postForm } from '../../api/files';
import type { components } from '../../api/schema';

type Schemas = components['schemas'];
export type Document = Schemas['Document'];
export type DocumentFile = Schemas['DocumentFile'];
export type DocumentType = Schemas['DocumentType'];
export type DocumentInput = Schemas['DocumentInput'];
export type DocumentUpdate = Schemas['DocumentUpdate'];
export type Template = Schemas['DocumentGenerate']['template'];

export interface DocumentFilter {
  number?: string;
  type_id?: number;
  member_id?: number;
  dog_id?: number;
  event_id?: number;
  from?: string;
  to?: string;
}

export const documentKeys = {
  all: ['documents'] as const,
  list: (filter: DocumentFilter) => ['documents', 'list', filter] as const,
  document: (id: number) => ['documents', 'document', id] as const,
  history: (id: number) => ['documents', 'history', id] as const,
  types: ['document-types'] as const,
};

/** Types the active profile may see: staff all, the vet medical, the expert breeding ones. */
export function useDocumentTypes() {
  return useQuery({ queryKey: documentKeys.types, staleTime: Infinity, queryFn: () => call(api.GET('/api/v1/document-types')) });
}

export function useDocuments(filter: DocumentFilter) {
  return useInfiniteQuery({
    queryKey: documentKeys.list(filter),
    initialPageParam: 0,
    queryFn: ({ pageParam }) => call(api.GET('/api/v1/documents', { params: { query: { ...compact(filter), after: pageParam || undefined, limit: 50 } } })),
    getNextPageParam: (page) => page.next_after ?? undefined,
  });
}

const path = (documentID: number) => ({ params: { path: { documentID } } });

export function useDocument(id: number) {
  return useQuery({ queryKey: documentKeys.document(id), queryFn: () => call(api.GET('/api/v1/documents/{documentID}', path(id))) });
}

export function useDocumentHistory(id: number) {
  return useInfiniteQuery({
    queryKey: documentKeys.history(id),
    initialPageParam: 0,
    queryFn: ({ pageParam }) => call(api.GET('/api/v1/documents/{documentID}/history', { params: { path: { documentID: id }, query: { after: pageParam || undefined } } })),
    getNextPageParam: (page) => page.next_after ?? undefined,
  });
}

export function useDocumentsSaved() {
  const client = useQueryClient();
  return () => client.invalidateQueries({ queryKey: documentKeys.all });
}

/** Copies the API accepts: the type is checked again on the server by signature. */
export const uploadTypes: Record<string, string> = { 'application/pdf': 'pdf', 'image/jpeg': 'jpg', 'image/png': 'png' };
export const MAX_UPLOAD = 10 * 1024 * 1024;

export const documentsApi = {
  create: (body: DocumentInput) => call(api.POST('/api/v1/documents', { body })),
  update: (id: number, body: DocumentUpdate) => call(api.PUT('/api/v1/documents/{documentID}', { ...path(id), body })),
  void: (id: number, version: number, reason: string) => call(api.POST('/api/v1/documents/{documentID}/void', { ...path(id), body: { version, reason } })),
  generate: (id: number, body: Schemas['DocumentGenerate']) => call(api.POST('/api/v1/documents/{documentID}/generate', { ...path(id), body })),
  upload: (id: number, file: File, version: number, reason: string) => {
    const form = new FormData();
    form.append('version', String(version));
    form.append('reason', reason);
    form.append('file', file, file.name);
    return postForm<Document>(`/api/v1/documents/${id}/files`, form);
  },
  download: (file: DocumentFile) => downloadFile(`/api/v1/documents/${file.document_id}/files/${file.id}`, file.name),
};

function compact(filter: DocumentFilter) {
  const out: Record<string, string | number> = {};
  for (const [key, value] of Object.entries(filter)) {
    if (value !== undefined && value !== '' && value !== 0) out[key] = value;
  }
  return out as DocumentFilter;
}
