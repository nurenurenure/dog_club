import { Plus } from 'lucide-react';
import { useState } from 'react';
import { fieldErrors } from '../../api/problem';
import { useSession } from '../../auth/AuthProvider';
import { formatDate, todayUTC } from '../../shared/format';
import { Button, Card, Chip, EmptyState, Loading, QueryError, ReasonDialog, SelectField, Table, TextField, FormDialog, tableStyles, toOptions } from '../../shared/ui';
import { isMine } from './labels';
import { useClinicalSaved, useVaccinations, useVetDictionaries, vetApi, type Vaccination } from './vetApi';
import styles from './vet.module.css';

interface Props {
  dogID: number;
  birth: string;
  active: boolean;
}

type Draft = { vaccine_type_id: string; date: string };

export function VaccinationsCard({ dogID, birth, active }: Props) {
  const { profile, user } = useSession();
  const vet = profile.role === 'veterinarian';
  const vaccinations = useVaccinations(dogID);
  const dictionaries = useVetDictionaries();
  const saved = useClinicalSaved(dogID);
  const [adding, setAdding] = useState<Draft | null>(null);
  const [correcting, setCorrecting] = useState<{ record: Vaccination; draft: Draft } | null>(null);
  const [voiding, setVoiding] = useState<Vaccination | null>(null);
  const [error, setError] = useState<unknown>(null);
  const errors = fieldErrors(error);
  const today = todayUTC();

  // Lab 3: doses of one vaccine must not overlap [date, next_date).
  const nextDose = (vaccineID: string, except?: number) =>
    vaccinations.data
      ?.filter((item) => !item.is_void && item.id !== except && String(item.vaccine.id) === vaccineID && item.next_date && item.next_date > today)
      .map((item) => item.next_date!)
      .sort()
      .at(-1);

  const fields = (draft: Draft, set: (draft: Draft) => void, except?: number) => {
    const next = draft.vaccine_type_id ? nextDose(draft.vaccine_type_id, except) : undefined;
    return (
      <>
        <SelectField
          label="Препарат"
          required
          placeholder="Выберите…"
          options={toOptions(dictionaries.data?.vaccine_types ?? [])}
          value={draft.vaccine_type_id}
          onChange={(event) => set({ ...draft, vaccine_type_id: event.target.value })}
          error={errors.vaccine_type_id}
          hint={next ? `Следующая доза этого препарата не раньше ${formatDate(next)}.` : 'Сроки действия и ревакцинации сервер рассчитает по препарату.'}
        />
        <TextField
          label="Дата прививки"
          type="date"
          required
          min={birth}
          max={today}
          value={draft.date}
          onChange={(event) => set({ ...draft, date: event.target.value })}
          error={errors.date ?? errors.dog_id}
        />
      </>
    );
  };

  const run = async (action: () => Promise<unknown>) => {
    try {
      await action();
      setError(null);
      await saved();
    } catch (failure) {
      setError(failure);
      throw failure;
    }
  };

  return (
    <Card
      heading={
        <span className={styles.heading}>
          Вакцинации
          {vet && active && (
            <Button variant="secondary" size="small" onClick={() => setAdding({ vaccine_type_id: '', date: today })}>
              <Plus size={16} strokeWidth={2.4} aria-hidden="true" />
              Добавить прививку
            </Button>
          )}
        </span>
      }
    >
      {vaccinations.isPending ? (
        <Loading />
      ) : vaccinations.isError ? (
        <QueryError error={vaccinations.error} onRetry={() => void vaccinations.refetch()} />
      ) : vaccinations.data.length === 0 ? (
        <EmptyState title="Прививок пока нет" />
      ) : (
        <Table caption="Вакцинации">
          <thead>
            <tr>
              <th scope="col">Препарат</th>
              <th scope="col">Проведена</th>
              <th scope="col">Действует до</th>
              <th scope="col">Следующая</th>
              <th scope="col">Статус</th>
              {vet && <th scope="col">Действия</th>}
            </tr>
          </thead>
          <tbody>
            {vaccinations.data.map((item) => {
              const mine = vet && isMine(item.author, user.id);
              const expired = item.valid_until !== null && item.valid_until < today;
              return (
                <tr key={item.id} className={item.is_void ? styles.void : undefined}>
                  <td>
                    <b>{item.vaccine.name}</b>
                    {item.vaccine.maker && <span className={styles.sub}>{item.vaccine.maker}</span>}
                  </td>
                  <td className={tableStyles.nowrap}>{formatDate(item.date)}</td>
                  <td className={tableStyles.nowrap}>{formatDate(item.valid_until)}</td>
                  <td className={tableStyles.nowrap}>{formatDate(item.next_date)}</td>
                  <td>{item.is_void ? <Chip struck>Ошибочная запись</Chip> : expired ? <Chip tone="sun">Истекла</Chip> : <Chip tone="lime">Действует</Chip>}</td>
                  {vet && (
                    <td>
                      {mine && !item.is_void && (
                        <span className={tableStyles.actions}>
                          {active && (
                            <Button variant="link" onClick={() => setCorrecting({ record: item, draft: { vaccine_type_id: String(item.vaccine.id), date: item.date } })}>
                              Исправить
                            </Button>
                          )}
                          <Button variant="danger" onClick={() => setVoiding(item)}>
                            Отметить ошибочной
                          </Button>
                        </span>
                      )}
                    </td>
                  )}
                </tr>
              );
            })}
          </tbody>
        </Table>
      )}

      <FormDialog
        open={adding !== null}
        title="Новая прививка"
        confirmLabel="Сохранить"
        ready={Boolean(adding?.vaccine_type_id && adding.date)}
        onClose={() => {
          setAdding(null);
          setError(null);
        }}
        onSubmit={() => run(() => vetApi.addVaccination(dogID, { vaccine_type_id: Number(adding!.vaccine_type_id), date: adding!.date }))}
      >
        {adding && fields(adding, setAdding)}
      </FormDialog>
      <ReasonDialog
        open={correcting !== null}
        title="Исправить прививку"
        confirmLabel="Сохранить"
        ready={Boolean(correcting?.draft.vaccine_type_id && correcting.draft.date)}
        onReload={() => void saved()}
        onClose={() => {
          setCorrecting(null);
          setError(null);
        }}
        onConfirm={(reason) =>
          run(() =>
            vetApi.correctVaccination(dogID, correcting!.record.id, {
              version: correcting!.record.version,
              vaccine_type_id: Number(correcting!.draft.vaccine_type_id),
              date: correcting!.draft.date,
              reason,
            }),
          )
        }
      >
        {correcting && fields(correcting.draft, (draft) => setCorrecting({ ...correcting, draft }), correcting.record.id)}
      </ReasonDialog>
      <ReasonDialog
        open={voiding !== null}
        title="Отметить прививку ошибочной"
        description="Запись останется в истории зачёркнутой. Если она была основанием допуска, допуск потребует проверки."
        confirmLabel="Отметить ошибочной"
        onReload={() => void saved()}
        onClose={() => setVoiding(null)}
        onConfirm={(reason) => run(() => vetApi.voidVaccination(dogID, voiding!.id, voiding!.version, reason))}
      />
    </Card>
  );
}
