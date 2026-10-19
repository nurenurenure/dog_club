#!/usr/bin/env python3
"""Disposable Docker acceptance/load/recovery check. Uses synthetic data only.
Requires Python 3 stdlib and Docker Compose, never host Go or the working .env.
"""
import base64
import concurrent.futures
import hashlib
import http.cookiejar
import json
import os
from pathlib import Path
import re
import secrets
import statistics
import subprocess
import tempfile
import threading
import time
import urllib.error
import urllib.request

ROOT = Path(__file__).resolve().parent.parent
os.chdir(ROOT)
PROJECT = 'dogclub_verify_' + secrets.token_hex(6)
RESTORE = PROJECT + '_restore'
ENV = os.environ.copy()
ENV.update(COMPOSE_PROJECT_NAME=PROJECT, POSTGRES_USER='dogclub', POSTGRES_DB='dogclub',
           POSTGRES_PASSWORD=secrets.token_hex(24), AUTH_JWT_KEY=base64.b64encode(secrets.token_bytes(32)).decode(),
           API_PORT='0', POSTGRES_PORT='0', MAILPIT_SMTP_PORT='0', MAILPIT_HTTP_PORT='0',
           NOTIFICATIONS_ENABLED='true', LOG_LEVEL='info')
# Compose must not consult the real .env for verification.
ENV['COMPOSE_ENV_FILES'] = '/dev/null'
ENV['COMPOSE_DISABLE_ENV_FILE'] = 'true'
ENV['DOGCLUB_IMAGE'] = 'dogclub-backend:verify'
for variable in ('COMPOSE_FILE', 'COMPOSE_PROFILES', 'COMPOSE_PATH_SEPARATOR'):
    ENV.pop(variable, None)
HTTP = urllib.request.build_opener(urllib.request.ProxyHandler({}))
SECRETS = [ENV['POSTGRES_PASSWORD'], ENV['AUTH_JWT_KEY']]
MAIL_IDS = set()
BASE = MAIL = ''

def run(args, env=None, data=None, timeout=180):
    r = subprocess.run(args, env=env or ENV, input=data, capture_output=True, timeout=timeout)
    if r.returncode:
        # Docker output can include process environment: don't dump it on error.
        detail = r.stderr.decode(errors='replace')[-3000:]
        for value in SECRETS:
            if value: detail = detail.replace(value, '[redacted]')
        raise RuntimeError('Command failed: ' + ' '.join(args[:4]) + '\n' + detail)
    return r.stdout

def compose(*args, **kw):
    return run(['docker', 'compose', *args], **kw)

def sql(query):
    return compose('exec', '-T', 'db', 'psql', '-U', 'dogclub', '-d', 'dogclub', '-At', '-v', 'ON_ERROR_STOP=1', '-c', query).decode().strip()

def endpoint(service, port, env=None):
    return 'http://' + compose('port', service, str(port), env=env).decode().strip()

def request(url, method='GET', body=None, token=None, want=200):
    headers = {}
    if token: headers['Authorization'] = 'Bearer ' + token
    data = None
    if body is not None:
        headers['Content-Type'] = 'application/json'
        data = json.dumps(body).encode()
    req = urllib.request.Request(url, data=data, method=method, headers=headers)
    try: response = HTTP.open(req, timeout=30)
    except urllib.error.HTTPError as error: response = error
    with response:
        raw = response.read()
        assert response.status == want, f'{method} {url.split("?")[0]} status={response.status}, expected={want}'
        if '/api/v1/' in url and url.startswith(BASE):
            assert response.headers['Cache-Control'] == 'no-store'
            assert response.headers['X-Request-ID']
        if response.headers.get_content_type() in ('application/json', 'application/problem+json'):
            return json.loads(raw) if raw else None
        return raw

class Client:
    def __init__(self, token=None): self.token = token
    def call(self, method, path, body=None, want=200):
        out = request(BASE + '/api/v1' + path, method, body, self.token, want)
        if isinstance(out, dict) and 'access_token' in out:
            self.token = out['access_token']; SECRETS.append(self.token)
        return out

def code(address):
    for _ in range(50):
        for m in request(MAIL + '/api/v1/messages')['messages']:
            if m['ID'] in MAIL_IDS or not any(t['Address'] == address for t in m['To']): continue
            text = request(MAIL + '/api/v1/message/' + m['ID'])['Text']
            match = re.search(r'Код [^:\r\n]+: ([A-Za-z0-9_-]{43})', text)
            if match:
                MAIL_IDS.add(m['ID']); SECRETS.append(match[1]); return match[1]
        time.sleep(.1)
    raise AssertionError('Activation code was not delivered')

def ready(url=None):
    global BASE
    if url is None: BASE = endpoint("api", 8080)
    for _ in range(100):
        try:
            if request((url or BASE) + '/health/ready') == {'status': 'ready'}: return
        except (AssertionError, urllib.error.URLError, ConnectionError): pass
        time.sleep(.2)
    raise AssertionError('Readiness did not recover')

def counts():
    return sql("SELECT (SELECT count(*) FROM goose_db_version WHERE is_applied AND version_id>0),(SELECT count(*) FROM users),(SELECT count(*) FROM dog),(SELECT count(*) FROM document_file),(SELECT count(*) FROM dog_training_session)")

def check():
    global BASE, MAIL
    print('Building and starting an isolated project: ' + PROJECT, flush=True)
    compose('build', 'api', 'checks', timeout=600)
    compose('up', '-d', '--wait', 'api', timeout=180)
    BASE = endpoint('api', 8080); MAIL = endpoint('mailpit', 8025)
    ready(); assert sql('SELECT max(version_id) FROM goose_db_version WHERE is_applied') == '14'
    assert compose('exec', '-T', 'api', 'id', '-u').strip() == b'65532'
    compose('exec', '-T', 'api', '/maintenance')
    admin = Client(); anonymous = Client()
    anonymous.call('GET', '/operations/metrics', want=401)
    compose('run', '--rm', '-T', '--no-deps', '--entrypoint', '/admin', 'api', 'bootstrap', 'admin', 'admin@verify.test')
    password = 'Synthetic-' + secrets.token_hex(16); SECRETS.append(password)
    admin.call('POST', '/auth/activate', {'token': code('admin@verify.test'), 'password': password}, 204)
    admin.call('POST', '/auth/login', {'login': 'admin', 'password': password})
    uid = admin.call('GET', '/auth/me')['user']['id']
    profiles = {}
    for role in ('staff', 'leader', 'veterinarian', 'expert'):
        profiles[role] = admin.call('POST', f'/users/{uid}/profiles', {'role': role}, 201)
    clients = {}
    for role, profile in profiles.items():
        c = Client(); c.call('POST', '/auth/login', {'login': 'admin', 'password': password, 'profile_id': profile['id']}); clients[role] = c
    staff, leader, vet, expert = (clients[r] for r in ('staff', 'leader', 'veterinarian', 'expert'))
    for c in clients.values(): c.call('GET', '/operations/metrics', want=403)
    md = staff.call('GET', '/member-dictionaries')
    active = next(x['id'] for x in md['membership_statuses'] if x['name'] == 'Активный')
    inactive = next(x['id'] for x in md['membership_statuses'] if x['name'] == 'Неактивный')
    person = staff.call('POST', '/members', {'full_name': 'Synthetic owner', 'membership_status_id': active}, 201)
    other = staff.call('POST', '/members', {'full_name': 'Synthetic other', 'membership_status_id': active}, 201)
    doctor = staff.call('POST', '/members', {'full_name': 'Synthetic instructor and vet', 'membership_status_id': inactive}, 201)
    admin.call('PUT', f'/users/{uid}/member', {'member_id': doctor['id']}, 204)
    admin.call('POST', '/invitations', {'login': 'owner', 'email': 'owner@verify.test', 'role': 'owner', 'club_member_id': person['id']}, 201)
    owner = Client(); owner.call('POST', '/auth/activate', {'token': code('owner@verify.test'), 'password': password}, 204)
    owner.call('POST', '/auth/login', {'login': 'owner', 'password': password})
    dd = staff.call('GET', '/dog-dictionaries')
    dog = None
    for i in range(100):
        d = staff.call('POST', '/dogs', {'nickname': f'Synthetic dog {i:03}', 'date_of_birth': '2023-03-01',
            'identifier': {'type_id': dd['identifier_types'][0]['id'], 'number': f'VERIFY-{i:04}'},
            'status_id': dd['statuses'][0]['id'], 'breed_id': dd['breeds'][0]['id'], 'coat_color_id': dd['coat_colors'][0]['id'],
            'sex_id': dd['sexes'][0]['id'], 'owner_ids': [person['id']]}, 201)
        if dog is None: dog = d
    dr = f'/dogs/{dog["id"]}'
    dog = staff.call('PUT', dr + '/registration', {'version': dog['version'], 'reason': 'Synthetic confirmed date', 'registration_date': '2026-10-01'})
    exam = vet.call('POST', dr + '/examinations', {'date': '2026-10-01', 'description': 'Synthetic examination', 'result': 'Synthetic private clinical text'}, 201)
    SECRETS.append('Synthetic private clinical text')
    tt = staff.call('GET', '/training-types')
    grant = admin.call('PUT', '/training-instructors/' + profiles['staff']['id'], {'version': 0, 'enabled': True, 'qualification': 'Synthetic verified qualification', 'type_ids': [tt[0]['id']], 'reason': 'Synthetic grant'})
    training = staff.call('POST', dr + '/training-sessions', {'type_id': tt[0]['id'], 'starts_at': '2026-10-01T10:00:00+07:00', 'ends_at': '2026-10-01T11:00:00+07:00', 'timezone': 'Asia/Barnaul', 'result': 'Synthetic completed training'}, 201)
    assert training['warnings'], 'Missing medical warning'
    owner.call('GET', dr + '/training-sessions/' + str(training['id']))
    admin.call('PUT', '/training-instructors/' + profiles['staff']['id'], {'version': grant['version'], 'enabled': False, 'qualification': 'Synthetic verified qualification', 'type_ids': [tt[0]['id']], 'reason': 'Synthetic revoke'})
    staff.call('POST', dr + '/training-sessions/' + str(training['id']) + '/void', {'version': training['version'], 'reason': 'Should be denied'}, 403)
    types = staff.call('GET', '/document-types')
    typeid = lambda name: next(x['id'] for x in types if x['name'] == name)
    doc = staff.call('POST', '/documents', {'number': 'SYN-001', 'type_id': typeid('Карточка собаки'), 'issuer': 'Synthetic club', 'dog_id': dog['id'], 'recipient_member_id': person['id']}, 201)
    doc = staff.call('POST', f'/documents/{doc["id"]}/generate', {'version': doc['version'], 'reason': 'Synthetic card', 'template': 'dog_card'})
    download = f'/documents/{doc["id"]}/files/{doc["files"][0]["id"]}'
    original = owner.call('GET', download); assert original.startswith(b'%PDF-')
    assert hashlib.sha256(original).hexdigest() == doc['files'][0]['sha256']
    med = vet.call('POST', '/documents', {'number': 'VET-001', 'type_id': typeid('Ветеринарная справка'), 'issuer': 'Synthetic vet', 'dog_id': dog['id'], 'recipient_member_id': person['id']}, 201)
    vet.call('POST', f'/documents/{med["id"]}/generate', {'version': med['version'], 'reason': 'Synthetic conclusion', 'template': 'veterinary_conclusion', 'examination_id': exam['id']})
    catalog = staff.call('GET', '/event-dictionaries')
    venue = staff.call('POST', '/event-venues', {'name': 'Synthetic venue', 'locality': 'Synthetic town'}, 201)
    event = staff.call('POST', '/events', {'name': 'Synthetic seminar', 'type_id': next(x['id'] for x in catalog['types'] if x['name'] == 'Семинар'), 'venue_id': venue['id'], 'starts_at': '2024-02-29T10:00:00+07:00', 'ends_at': '2024-02-29T18:00:00+07:00', 'timezone': 'Asia/Barnaul', 'audience': 'people', 'uses_classes': False}, 201)
    for state in ('open', 'closed', 'in_progress', 'completed'):
        event = staff.call('POST', f'/events/{event["id"]}/state', {'version': event['version'], 'reason': 'Synthetic transition', 'status': state})
    report = '/reports/club-activity?from=2024-02-01&to=2024-02-29'
    assert leader.call('GET', report)['activity']['events_completed'] == 1
    # Membership, transfer, and account revocation are checked against current DB state.
    person = staff.call('PATCH', f'/members/{person["id"]}', {'version': person['version'], 'membership_status_id': inactive})
    owner.call('GET', download, want=403)
    person = staff.call('PATCH', f'/members/{person["id"]}', {'version': person['version'], 'membership_status_id': active})
    dog = staff.call('PUT', dr + '/owners', {'version': dog['version'], 'owner_ids': [other['id']]})
    owner.call('GET', download, want=404)
    dog = staff.call('PUT', dr + '/owners', {'version': dog['version'], 'owner_ids': [person['id']]})
    owner_id = owner.call('GET', '/auth/me')['user']['id']
    admin.call('PATCH', f'/users/{owner_id}', {'blocked': True}, 204)
    owner.call('GET', '/auth/me', want=401)
    admin.call('PATCH', f'/users/{owner_id}', {'blocked': False}, 204)
    # Observe the real SMTP worker; restart API to trigger a cycle immediately.
    compose('restart', 'api'); ready()
    assert int(sql("SELECT count(*) FROM notification_delivery WHERE status='sent'")) >= 1
    # Existing sessions survive process restarts; account state is still checked.
    admin.call('GET', '/operations/metrics')
    compose('stop', 'mailpit')
    staff.call('PUT', dr + '/registration', {'version': dog['version'], 'reason': 'Synthetic SMTP outage', 'registration_date': '2026-10-02'})
    compose('restart', 'api'); ready()
    for _ in range(50):
        if int(sql("SELECT count(*) FROM notification_delivery WHERE status='pending' AND attempts>0")): break
        time.sleep(.1)
    else: raise AssertionError('SMTP failure was not retained for retry')
    compose('start', 'mailpit')
    print('SMTP retry and load preparation respect the existing one-minute intervals.', flush=True)
    # This is test fixture time acceleration in the disposable DB only.
    sql("UPDATE notification_delivery SET next_attempt_at=now() WHERE status='pending'")
    compose('restart', 'api'); ready()
    for _ in range(50):
        if sql("SELECT count(*) FROM notification_delivery WHERE status='pending'") == '0': break
        time.sleep(.1)
    else: raise AssertionError('SMTP retry did not recover')
    before = counts()
    compose('up', '-d', '--wait', 'api'); ready(); assert counts() == before
    compose('stop', 'db'); request(BASE + '/health/live'); request(BASE + '/health/ready', want=503)
    compose('start', 'db'); ready(); assert counts() == before
    print('HTTP workflows, access, SMTP retry, restart and DB outage passed.', flush=True)
    # 50 separately logged-in clients. Rate limiting is preserved; preload over time.
    clients50 = []
    while len(clients50) < 50:
        c = Client()
        try:
            c.call('POST', '/auth/login', {'login': 'admin', 'password': password, 'profile_id': profiles['staff']['id']})
            clients50.append(c)
        except AssertionError as e:
            if 'status=429' not in str(e): raise
            print(f'Pre-login: {len(clients50)}/50; waiting for the login rate window.', flush=True)
            time.sleep(60)
    def load(path, rounds, bound):
        gate = threading.Barrier(50)
        def one(c):
            gate.wait(); durations = []
            for _ in range(rounds):
                started = time.perf_counter(); c.call('GET', path); durations.append(time.perf_counter() - started)
            return durations
        with concurrent.futures.ThreadPoolExecutor(max_workers=50) as pool:
            durations = sorted(d for batch in pool.map(one, clients50) for d in batch)
        out = {'clients': 50, 'requests': len(durations), 'errors': 0, 'p50_ms': round(statistics.median(durations)*1000,2), 'p95_ms': round(durations[int(.95*(len(durations)-1))]*1000,2), 'max_ms': round(max(durations)*1000,2), 'bound_seconds': bound, 'all_within_bound': max(durations) <= bound}
        print(json.dumps(out), flush=True); return out
    loadresults = {'search': load('/dogs?q=Synthetic&limit=50', 4, 2), 'pdf': load('/reports/club-activity/export?from=2024-02-01&to=2024-02-29&format=pdf', 2, 3)}
    m = admin.call('GET', '/operations/metrics')
    assert m['notifications_enabled'] and m['notifications']['last_successful_cycle_unix'] > 0
    assert any(x['route'] == '/api/v1/dogs' and x['requests'] >= 200 for x in m['http'])
    assert 'Synthetic' not in json.dumps(m) and '/dogs/1' not in json.dumps(m)
    (ROOT / 'var').mkdir(exist_ok=True)
    (ROOT / 'var/stage12-load.json').write_text(json.dumps(loadresults, indent=2) + '\n')
    with tempfile.TemporaryDirectory(prefix='dogclub-backup-') as tmp:
        destination = str(Path(tmp) / 'set')
        run(['scripts/backup.sh', destination], timeout=180)
        manifest = json.loads((Path(destination) / 'storage.json').read_text()); assert manifest['registered_files'] >= 2
        ready(); assert counts() == before
        restored_env = ENV.copy(); restored_env.update(COMPOSE_PROJECT_NAME=RESTORE, NOTIFICATIONS_ENABLED='false')
        run(['scripts/restore.sh', destination, RESTORE], timeout=180)
        restored_url = endpoint('api', 8080, restored_env); ready(restored_url)
        state = request(restored_url + '/api/v1/operations/metrics', token=admin.token)
        assert state['notifications_enabled'] is False and state['notifications']['last_successful_cycle_unix'] == 0
        assert request(restored_url + '/api/v1' + download, token=staff.token) == original
        # Refuse overwriting a restored project; corruption fails before creating a project.
        failed = subprocess.run(['scripts/restore.sh', destination, RESTORE], env=ENV, capture_output=True)
        assert failed.returncode != 0
        with open(Path(destination) / 'documents.tar', 'ab') as f: f.write(b'corruption')
        failed = subprocess.run(['scripts/restore.sh', destination, RESTORE + '_bad'], env=ENV, capture_output=True)
        assert failed.returncode != 0
        assert compose('ps', '-aq', env={**ENV, 'COMPOSE_PROJECT_NAME': RESTORE + '_bad'}).strip() == b''
    logs = compose('logs', '--no-color', 'api').decode()
    assert all(not secret or secret not in logs for secret in SECRETS), 'Sensitive value in API logs'
    print('Nonempty backup/restore, file hashes, overwrite/corruption refusal and safe logs passed.', flush=True)
    result = {'dataset': {'dogs': 100, 'members': 3, 'documents_with_files': 2}, 'load': loadresults, 'backup': manifest}
    (ROOT / 'var').mkdir(exist_ok=True)
    (ROOT / 'var/stage12-verification.json').write_text(json.dumps(result, indent=2) + '\n')
    assert all(x['all_within_bound'] for x in loadresults.values()), 'Measured latency exceeds the agreed limits; see var/stage12-verification.json'

try:
    check()
finally:
    # Only names generated above are eligible for deletion. No working project resources.
    for project in (RESTORE, PROJECT):
        cleanup_env = {**ENV, 'COMPOSE_PROJECT_NAME': project, 'NOTIFICATIONS_ENABLED': 'false'}
        run(['docker', 'compose', 'down', '--volumes', '--remove-orphans'], env=cleanup_env)
