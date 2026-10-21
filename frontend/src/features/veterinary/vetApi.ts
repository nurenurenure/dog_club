import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, call } from '../../api/client';
import type { components } from '../../api/schema';
import { pluralize } from '../../shared/format';

type Schemas = components['schemas'];
export type Examination = Schemas['MedicalExamination'];
export type Vaccination = Schemas['MedicalVaccination'];
export type Clearance = Schemas['MedicalClearance'];
export type MedicalStatus = Schemas['MedicalStatus'];
export type Admission = Schemas['MedicalAdmission'];
export type VaccineType = Schemas['MedicalVaccineType'];
export type Period = Schemas['MedicalPeriod'];
export type MedicalChange = Schemas['MedicalChange'];
export type Author = Schemas['MedicalAuthor'];

export const vetKeys = {
  dictionaries: ['veterinary-dictionaries'] as const,
  patients: (q: string) => ['veterinary', 'patients', q] as const,
  patient: (id: number) => ['veterinary', 'patient', id] as const,
  dog: (id: number) => ['veterinary', 'dog', id] as const,
  status: (id: number) => ['veterinary', 'dog', id, 'status'] as const,
  examinations: (id: number) => ['veterinary', 'dog', id, 'examinations'] as const,
  vaccinations: (id: number) => ['veterinary', 'dog', id, 'vaccinations'] as const,
  clearances: (id: number) => ['veterinary', 'dog', id, 'clearances'] as const,
  history: (id: number) => ['veterinary', 'dog', id, 'history'] as const,
  vaccineHistory: (id: number) => ['veterinary', 'vaccine', id, 'history'] as const,
};

/** Diseases and vaccines: read by every profile of the module, changed rarely. */
export function useVetDictionaries() {
  return useQuery({ queryKey: vetKeys.dictionaries, queryFn: () => call(api.GET('/api/v1/veterinary-dictionaries')), staleTime: 5 * 60_000 });
}

export function usePatients(q: string) {
  return useInfiniteQuery({
    queryKey: vetKeys.patients(q),
    initialPageParam: 0,
    queryFn: ({ pageParam }) => call(api.GET('/api/v1/veterinary/dogs', { params: { query: { q: q || undefined, after: pageParam || undefined } } })),
    getNextPageParam: (page) => page.next_after ?? undefined,
  });
}

/**
 * The veterinarian has no GET /dogs/{id}; the patient search ordered by ID with
 * after=id-1 and limit=1 returns exactly this dog when it exists.
 */
export function usePatient(id: number) {
  return useQuery({
    queryKey: vetKeys.patient(id),
    queryFn: async () => {
      const page = await call(api.GET('/api/v1/veterinary/dogs', { params: { query: { after: id - 1, limit: 1 } } }));
      const dog = page.dogs[0];
      if (!dog || dog.id !== id) return null;
      return dog;
    },
  });
}

// Clinical lists are short: load every page and show the newest first.
async function all<T extends { id: number }>(load: (after: number) => Promise<{ records: T[]; next_after: number | null }>): Promise<T[]> {
  const out: T[] = [];
  let after = 0;
  for (let page = 0; page < 20; page += 1) {
    const result = await load(after);
    out.push(...result.records);
    if (result.next_after === null) break;
    after = result.next_after;
  }
  return out.reverse();
}

const dogPath = (dogID: number, after = 0) => ({ params: { path: { dogID }, query: { after: after || undefined, limit: 100 } } });

export function useMedicalStatus(dogID: number) {
  return useQuery({ queryKey: vetKeys.status(dogID), queryFn: () => call(api.GET('/api/v1/dogs/{dogID}/medical-status', { params: { path: { dogID } } })) });
}

export function useExaminations(dogID: number) {
  return useQuery({ queryKey: vetKeys.examinations(dogID), queryFn: () => all((after) => call(api.GET('/api/v1/dogs/{dogID}/examinations', dogPath(dogID, after)))) });
}

export function useVaccinations(dogID: number) {
  return useQuery({ queryKey: vetKeys.vaccinations(dogID), queryFn: () => all((after) => call(api.GET('/api/v1/dogs/{dogID}/vaccinations', dogPath(dogID, after)))) });
}

export function useClearances(dogID: number) {
  return useQuery({ queryKey: vetKeys.clearances(dogID), queryFn: () => all((after) => call(api.GET('/api/v1/dogs/{dogID}/clearances', dogPath(dogID, after)))) });
}

export function useVetHistory(dogID: number) {
  return useInfiniteQuery({
    queryKey: vetKeys.history(dogID),
    initialPageParam: 0,
    queryFn: ({ pageParam }) => call(api.GET('/api/v1/dogs/{dogID}/veterinary-history', { params: { path: { dogID }, query: { after: pageParam || undefined } } })),
    getNextPageParam: (page) => page.next_after ?? undefined,
  });
}

export function useVaccineHistory(typeID: number, enabled: boolean) {
  return useInfiniteQuery({
    queryKey: vetKeys.vaccineHistory(typeID),
    enabled,
    initialPageParam: 0,
    queryFn: ({ pageParam }) => call(api.GET('/api/v1/vaccine-types/{typeID}/history', { params: { path: { typeID }, query: { after: pageParam || undefined } } })),
    getNextPageParam: (page) => page.next_after ?? undefined,
  });
}

/** Any clinical change can move the admission status: refresh the whole dog. */
export function useClinicalSaved(dogID: number) {
  const client = useQueryClient();
  return () => client.invalidateQueries({ queryKey: vetKeys.dog(dogID) });
}

export const vetApi = {
  addExamination: (dogID: number, body: Schemas['MedicalExaminationInput']) => call(api.POST('/api/v1/dogs/{dogID}/examinations', { params: { path: { dogID } }, body })),
  correctExamination: (dogID: number, id: number, body: Schemas['MedicalExaminationUpdate']) =>
    call(api.PUT('/api/v1/dogs/{dogID}/examinations/{examinationID}', { params: { path: { dogID, examinationID: id } }, body })),
  voidExamination: (dogID: number, id: number, version: number, reason: string) =>
    call(api.POST('/api/v1/dogs/{dogID}/examinations/{examinationID}/void', { params: { path: { dogID, examinationID: id } }, body: { version, reason } })),
  addVaccination: (dogID: number, body: Schemas['MedicalVaccinationInput']) => call(api.POST('/api/v1/dogs/{dogID}/vaccinations', { params: { path: { dogID } }, body })),
  correctVaccination: (dogID: number, id: number, body: Schemas['MedicalVaccinationUpdate']) =>
    call(api.PUT('/api/v1/dogs/{dogID}/vaccinations/{vaccinationID}', { params: { path: { dogID, vaccinationID: id } }, body })),
  voidVaccination: (dogID: number, id: number, version: number, reason: string) =>
    call(api.POST('/api/v1/dogs/{dogID}/vaccinations/{vaccinationID}/void', { params: { path: { dogID, vaccinationID: id } }, body: { version, reason } })),
  addClearance: (dogID: number, body: Schemas['MedicalClearanceInput']) => call(api.POST('/api/v1/dogs/{dogID}/clearances', { params: { path: { dogID } }, body })),
  correctClearance: (dogID: number, id: number, body: Schemas['MedicalClearanceUpdate']) =>
    call(api.PUT('/api/v1/dogs/{dogID}/clearances/{clearanceID}', { params: { path: { dogID, clearanceID: id } }, body })),
  voidClearance: (dogID: number, id: number, version: number, reason: string) =>
    call(api.POST('/api/v1/dogs/{dogID}/clearances/{clearanceID}/void', { params: { path: { dogID, clearanceID: id } }, body: { version, reason } })),
  addVaccineType: (body: Schemas['MedicalVaccineTypeInput']) => call(api.POST('/api/v1/vaccine-types', { body })),
  updateVaccineType: (typeID: number, body: Schemas['MedicalVaccineTypeUpdate']) => call(api.PUT('/api/v1/vaccine-types/{typeID}', { params: { path: { typeID } }, body })),
};

export function formatPeriod(period: Period | null): string {
  if (!period) return 'не задан';
  return period.unit === 'days' ? pluralize(period.value, 'день', 'дня', 'дней') : pluralize(period.value, 'месяц', 'месяца', 'месяцев');
}
