import { Plus } from 'lucide-react';
import { useState } from 'react';
import { Link, useSearchParams } from 'react-router';
import { roleLabels } from '../../app/navigation';
import { useSession } from '../../auth/AuthProvider';
import { formatDate } from '../../shared/format';
import { Ball, Button, ButtonLink, Card, EmptyState, Loading, PageHeader, QueryError, SelectField, Table, TextField, Toolbar, tableStyles, toOptions } from '../../shared/ui';
import { useDebounced } from '../../shared/useDebounced';
import { useDocumentTypes, useDocuments, type Document, type DocumentFilter } from './documentsApi';
import { DocDogName, DocMemberName, DocumentStatus } from './parts';
import styles from './documents.module.css';

const numberParam = (value: string | null) => Number(value) || undefined;

/** The server lists only what the profile may read; the screen does not filter by role. */
export function DocumentsPage() {
  const { profile } = useSession();
  const [params, setParams] = useSearchParams();
  const [query, setQuery] = useState(params.get('number') ?? '');
  const number = useDebounced(query.trim());
  const types = useDocumentTypes();
  const from = params.get('from') ?? '';
  const to = params.get('to') ?? '';
  const filter: DocumentFilter = {
    number,
    type_id: numberParam(params.get('type')),
    dog_id: numberParam(params.get('dog')),
    member_id: numberParam(params.get('member')),
    event_id: numberParam(params.get('event')),
    from: from || undefined,
    to: to || undefined,
  };
  const documents = useDocuments(filter);
  const rows = documents.data?.pages.flatMap((page) => page.records) ?? [];
  const writer = profile.role === 'staff' || profile.role === 'veterinarian';

  const setParam = (name: string, value: string) => {
    const next = new URLSearchParams(params);
    if (value) next.set(name, value);
    else next.delete(name);
    setParams(next, { replace: true });
  };
  const subject = filter.dog_id ? (
    <>
      собаки <DocDogName id={filter.dog_id} />
    </>
  ) : filter.member_id ? (
    <>
      члена клуба <DocMemberName id={filter.member_id} />
    </>
  ) : filter.event_id ? (
    <>
      мероприятия <Link to={`/events/${filter.event_id}`}>№{filter.event_id}</Link>
    </>
  ) : null;

  return (
    <>
      <PageHeader
        eyebrow={roleLabels[profile.role]}
        title={profile.role === 'veterinarian' ? 'Медицинские документы' : profile.role === 'owner' ? 'Мои документы' : 'Документы'}
        actions={
          writer && (
            <ButtonLink to={`/documents/new${params.toString() ? `?${params.toString()}` : ''}`}>
              <Plus size={18} strokeWidth={2.4} aria-hidden="true" />
              Новый документ
            </ButtonLink>
          )
        }
      />
      {subject && (
        <div className={styles.picked}>
          <span>Документы {subject}</span>
          <Button
            variant="link"
            onClick={() => {
              const next = new URLSearchParams(params);
              ['dog', 'member', 'event'].forEach((key) => next.delete(key));
              setParams(next, { replace: true });
            }}
          >
            Все документы
          </Button>
        </div>
      )}
      <Toolbar>
        <TextField
          label="Номер"
          type="search"
          placeholder="Часть номера"
          value={query}
          onChange={(event) => {
            setQuery(event.target.value);
            setParam('number', event.target.value.trim());
          }}
        />
        <SelectField
          label="Тип"
          placeholder="Все типы"
          options={toOptions(types.data ?? [])}
          value={params.get('type') ?? ''}
          onChange={(event) => setParam('type', event.target.value)}
        />
        <TextField label="Выдан с" type="date" value={from} onChange={(event) => setParam('from', event.target.value)} />
        <TextField label="Выдан по" type="date" min={from || undefined} value={to} onChange={(event) => setParam('to', event.target.value)} />
      </Toolbar>
      {documents.isPending ? (
        <Loading label="Загружаем документы…" />
      ) : documents.isError ? (
        <QueryError error={documents.error} onRetry={() => void documents.refetch()} />
      ) : rows.length === 0 ? (
        <Card>
          <EmptyState title={params.toString() ? 'Ничего не нашли' : 'Документов пока нет'} illustration={<Ball />}>
            {profile.role === 'owner' && <p>Здесь появятся документы, которые клуб выдал вам.</p>}
          </EmptyState>
        </Card>
      ) : (
        <>
          <Table caption="Документы">
            <thead>
              <tr>
                <th scope="col">Документ</th>
                <th scope="col">Предмет</th>
                <th scope="col">Выдан</th>
                <th scope="col">Статус</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((document) => (
                <tr key={document.id}>
                  <td>
                    <Link to={`/documents/${document.id}`}>{document.type_name}</Link>
                    <div className={tableStyles.muted}>{document.number ? `№ ${document.number}` : 'Номер не указан'}</div>
                  </td>
                  <td>
                    <Subject document={document} />
                  </td>
                  <td className={tableStyles.nowrap}>{formatDate(document.issue_date)}</td>
                  <td>
                    <DocumentStatus document={document} />
                  </td>
                </tr>
              ))}
            </tbody>
          </Table>
          {documents.hasNextPage && (
            <Button variant="secondary" className={tableStyles.more} busy={documents.isFetchingNextPage} onClick={() => void documents.fetchNextPage()}>
              Следующие 50
            </Button>
          )}
        </>
      )}
    </>
  );
}

/** One subject per record: a person, a dog or an event. */
export function Subject({ document }: { document: Document }) {
  if (document.dog_id) return <DocDogName id={document.dog_id} />;
  if (document.member_id) return <DocMemberName id={document.member_id} />;
  if (document.event_id) return <Link to={`/events/${document.event_id}`}>Мероприятие №{document.event_id}</Link>;
  return <>—</>;
}
