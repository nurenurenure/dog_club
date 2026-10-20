import { Plus } from 'lucide-react';
import { useState } from 'react';
import { fieldErrors } from '../../api/problem';
import { useSession } from '../../auth/AuthProvider';
import { formatDate, todayUTC } from '../../shared/format';
import { Button, Card, Chip, EmptyState, FormDialog, Loading, QueryError, ReasonDialog, TextArea, TextField, fieldStyles } from '../../shared/ui';
import clsx from '../../shared/ui/clsx';
import { hereditary, isMine } from './labels';
import { useClinicalSaved, useExaminations, useVetDictionaries, vetApi, type Examination } from './vetApi';
import styles from './vet.module.css';

interface Props {
  dogID: number;
  birth: string;
  active: boolean;
}

type Hereditary = 'unknown' | 'yes' | 'no';

interface Draft {
  date: string;
  description: string;
  result: string;
  recommendations: string;
  diagnoses: Record<number, Hereditary>;
}

const toHereditary = (value: boolean | null): Hereditary => (value === true ? 'yes' : value === false ? 'no' : 'unknown');
const fromHereditary = (value: Hereditary) => (value === 'yes' ? true : value === 'no' ? false : null);

function draftOf(exam?: Examination): Draft {
  if (!exam) return { date: todayUTC(), description: '', result: '', recommendations: '', diagnoses: {} };
  return {
    date: exam.date,
    description: exam.description,
    result: exam.result,
    recommendations: exam.recommendations,
    diagnoses: Object.fromEntries(exam.diagnoses.map((item) => [item.disease.id, toHereditary(item.is_hereditary)])),
  };
}

function bodyOf(draft: Draft) {
  return {
    date: draft.date,
    description: draft.description.trim(),
    result: draft.result.trim(),
    recommendations: draft.recommendations.trim(),
    diagnoses: Object.entries(draft.diagnoses).map(([id, value]) => ({ disease_id: Number(id), is_hereditary: fromHereditary(value) })),
  };
}

export function ExaminationsCard({ dogID, birth, active }: Props) {
  const { profile, user } = useSession();
  const vet = profile.role === 'veterinarian';
  const examinations = useExaminations(dogID);
  const saved = useClinicalSaved(dogID);
  const [adding, setAdding] = useState<Draft | null>(null);
  const [correcting, setCorrecting] = useState<{ record: Examination; draft: Draft } | null>(null);
  const [voiding, setVoiding] = useState<Examination | null>(null);
  const [error, setError] = useState<unknown>(null);

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
  const close = () => {
    setAdding(null);
    setCorrecting(null);
    setError(null);
  };

  return (
    <Card
      heading={
        <span className={styles.heading}>
          Осмотры
          {vet && active && (
            <Button variant="secondary" size="small" onClick={() => setAdding(draftOf())}>
              <Plus size={16} strokeWidth={2.4} aria-hidden="true" />
              Добавить осмотр
            </Button>
          )}
        </span>
      }
    >
      {examinations.isPending ? (
        <Loading />
      ) : examinations.isError ? (
        <QueryError error={examinations.error} onRetry={() => void examinations.refetch()} />
      ) : examinations.data.length === 0 ? (
        <EmptyState title="Осмотров пока нет" />
      ) : (
        <ul className={styles.records}>
          {examinations.data.map((exam) => (
            <li key={exam.id} className={clsx(styles.record, exam.is_void && styles.void)}>
              <div className={styles.recordHead}>
                <b className={styles.date}>{formatDate(exam.date)}</b>
                {exam.result && <span>{exam.result}</span>}
                {exam.is_void && <Chip struck>Ошибочная запись</Chip>}
              </div>
              {exam.diagnoses.length > 0 && (
                <div className={styles.chips}>
                  {exam.diagnoses.map((item) => (
                    <Chip key={item.disease.id} tone={item.is_hereditary ? 'pink' : 'sand'}>
                      {item.disease.name} · {hereditary(item.is_hereditary)}
                    </Chip>
                  ))}
                </div>
              )}
              {exam.description && <p>{exam.description}</p>}
              {exam.recommendations && <p className={styles.sub}>Рекомендации: {exam.recommendations}</p>}
              {vet && isMine(exam.author, user.id) && !exam.is_void && (
                <span className={styles.recordActions}>
                  {active && (
                    <Button variant="link" onClick={() => setCorrecting({ record: exam, draft: draftOf(exam) })}>
                      Исправить
                    </Button>
                  )}
                  <Button variant="danger" onClick={() => setVoiding(exam)}>
                    Отметить ошибочным
                  </Button>
                </span>
              )}
            </li>
          ))}
        </ul>
      )}

      <FormDialog
        open={adding !== null}
        title="Новый осмотр"
        confirmLabel="Сохранить"
        ready={Boolean(adding?.date)}
        onClose={close}
        onSubmit={() => run(() => vetApi.addExamination(dogID, bodyOf(adding!)))}
      >
        {adding && <ExaminationFields draft={adding} onChange={setAdding} birth={birth} error={error} />}
      </FormDialog>
      <ReasonDialog
        open={correcting !== null}
        title="Исправить осмотр"
        confirmLabel="Сохранить"
        ready={Boolean(correcting?.draft.date)}
        onReload={() => void saved()}
        onClose={close}
        onConfirm={(reason) => run(() => vetApi.correctExamination(dogID, correcting!.record.id, { ...bodyOf(correcting!.draft), version: correcting!.record.version, reason }))}
      >
        {correcting && <ExaminationFields draft={correcting.draft} onChange={(draft) => setCorrecting({ ...correcting, draft })} birth={birth} error={error} />}
      </ReasonDialog>
      <ReasonDialog
        open={voiding !== null}
        title="Отметить осмотр ошибочным"
        description="Заключения по этому осмотру перестанут давать допуск. Запись останется в истории."
        confirmLabel="Отметить ошибочным"
        onReload={() => void saved()}
        onClose={() => setVoiding(null)}
        onConfirm={(reason) => run(() => vetApi.voidExamination(dogID, voiding!.id, voiding!.version, reason))}
      />
    </Card>
  );
}

function ExaminationFields({ draft, onChange, birth, error }: { draft: Draft; onChange: (draft: Draft) => void; birth: string; error: unknown }) {
  const dictionaries = useVetDictionaries();
  const errors = fieldErrors(error);
  const set = (patch: Partial<Draft>) => onChange({ ...draft, ...patch });
  const toggle = (id: number, checked: boolean) => {
    const next = { ...draft.diagnoses };
    if (checked) next[id] = 'unknown';
    else delete next[id];
    set({ diagnoses: next });
  };
  return (
    <>
      <TextField
        label="Дата осмотра"
        type="date"
        required
        min={birth}
        max={todayUTC()}
        value={draft.date}
        onChange={(event) => set({ date: event.target.value })}
        error={errors.date ?? errors.dog_id}
      />
      <TextField
        label="Результат"
        maxLength={4000}
        value={draft.result}
        onChange={(event) => set({ result: event.target.value })}
        error={errors.result}
        hint="Например, «здоров»."
      />
      <TextArea label="Описание состояния" maxLength={4000} value={draft.description} onChange={(event) => set({ description: event.target.value })} error={errors.description} />
      <TextArea
        label="Рекомендации"
        maxLength={4000}
        value={draft.recommendations}
        onChange={(event) => set({ recommendations: event.target.value })}
        error={errors.recommendations}
      />
      <fieldset className={fieldStyles.fieldset}>
        <legend className={fieldStyles.legend}>Диагнозы</legend>
        <ul className={styles.diagnoses}>
          {(dictionaries.data?.diseases ?? []).map((disease) => {
            const value = draft.diagnoses[disease.id];
            return (
              <li key={disease.id} className={styles.diagnosis}>
                <label className={styles.check}>
                  <input type="checkbox" checked={value !== undefined} onChange={(event) => toggle(disease.id, event.target.checked)} />
                  {disease.name}
                </label>
                {value !== undefined && (
                  <select
                    aria-label={`Наследственность: ${disease.name}`}
                    className={styles.inlineSelect}
                    value={value}
                    onChange={(event) => set({ diagnoses: { ...draft.diagnoses, [disease.id]: event.target.value as Hereditary } })}
                  >
                    <option value="unknown">не установлено</option>
                    <option value="yes">наследственное</option>
                    <option value="no">не наследственное</option>
                  </select>
                )}
              </li>
            );
          })}
        </ul>
        {errors.diagnoses && <span className={styles.error}>{errors.diagnoses}</span>}
      </fieldset>
    </>
  );
}
