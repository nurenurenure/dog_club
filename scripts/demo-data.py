#!/usr/bin/env python3
"""Fill an empty local stand with synthetic demo data through the public API.

Requires Python 3 stdlib, Docker Compose and a running stack (api, mailpit).
The first step bootstraps the administrator: on a database that already has one
the script stops before changing anything. Every name, number and document is
fictional. One random password for all demo accounts is written to
var/demo-accounts.txt (mode 600) and never printed.
"""
import argparse
import datetime as dt
import json
import os
from pathlib import Path
import re
import secrets
import subprocess
import time
import urllib.error
import urllib.request

ROOT = Path(__file__).resolve().parent.parent
TZ = 'Asia/Barnaul'
TODAY = dt.datetime.now(dt.timezone.utc).date()
HTTP = urllib.request.build_opener(urllib.request.ProxyHandler({}))
# Unsplash photos (Unsplash License), credited in THIRD_PARTY_NOTICES.md.
PHOTOS = Path(__file__).resolve().parent / 'demo-photos'


class DemoError(SystemExit):
    pass


def day(offset):
    return (TODAY + dt.timedelta(days=offset)).isoformat()


def born(years, month=4, dom=12):
    """A birth date `years` ago; February 29 never occurs."""
    return dt.date(TODAY.year - years, month, dom).isoformat()


def at(offset, hour, minute=0):
    """RFC 3339 local time (UTC+7) `offset` days from today."""
    return f'{day(offset)}T{hour:02}:{minute:02}:00+07:00'


def compose(*args):
    result = subprocess.run(['docker', 'compose', *args], cwd=ROOT, capture_output=True, text=True, timeout=180)
    if result.returncode:
        raise DemoError('docker compose ' + ' '.join(args[:3]) + ' failed:\n' + result.stderr[-2000:])
    return result.stdout.strip()


def request(url, method='GET', body=None, token=None, want=(200,), raw=None):
    headers = {'Accept': 'application/json'}
    data = None
    if token:
        headers['Authorization'] = 'Bearer ' + token
    if body is not None:
        headers['Content-Type'] = 'application/json'
        data = json.dumps(body).encode()
    if raw is not None:
        data, headers['Content-Type'] = raw
    for attempt in range(4):
        req = urllib.request.Request(url, data=data, method=method, headers=headers)
        try:
            response = HTTP.open(req, timeout=60)
        except urllib.error.HTTPError as error:
            response = error
        except urllib.error.URLError as error:
            raise DemoError(f'{method} {url}: no connection ({error.reason}). Is the stack running?')
        with response:
            raw = response.read()
            status = response.status
            if status == 429 and attempt < 3:
                # Login and activation allow 10 attempts per minute per address.
                wait = int(response.headers.get('Retry-After') or 60)
                print(f'  rate limit, waiting {wait} s…', flush=True)
                time.sleep(wait)
                continue
        if status not in want:
            problem = {}
            try:
                problem = json.loads(raw)
            except ValueError:
                pass
            path = url.split('/api/v1', 1)[-1]
            raise DemoError(f'{method} {path}: HTTP {status} {problem.get("code", "")}: {problem.get("detail", "")} '
                            f'{problem.get("errors", "")} (request_id {problem.get("request_id", "-")})')
        return json.loads(raw) if raw else None
    raise DemoError(f'{method} {url}: still rate limited')


class Client:
    def __init__(self, base, token=None):
        self.base = base
        self.token = token

    def call(self, method, path, body=None, want=(200,)):
        return request(self.base + '/api/v1' + path, method, body, self.token, want)

    def get(self, path):
        return self.call('GET', path)

    def post(self, path, body=None, want=(200, 201)):
        return self.call('POST', path, body, want)

    def put(self, path, body):
        return self.call('PUT', path, body)

    def patch(self, path, body):
        return self.call('PATCH', path, body)

    def photo(self, dog, name):
        # One JPEG from scripts/demo-photos as multipart: file and version 0 (no photo yet).
        data = (PHOTOS / name).read_bytes()
        boundary = 'demo' + secrets.token_hex(12)
        body = (f'--{boundary}\r\nContent-Disposition: form-data; name="version"\r\n\r\n0\r\n'
                f'--{boundary}\r\nContent-Disposition: form-data; name="file"; filename="{name}"\r\n'
                f'Content-Type: image/jpeg\r\n\r\n').encode() + data + f'\r\n--{boundary}--\r\n'.encode()
        raw = (body, 'multipart/form-data; boundary=' + boundary)
        return request(self.base + '/api/v1' + f'/dogs/{dog["id"]}/photo', 'PUT', token=self.token, raw=raw)


class Mailpit:
    def __init__(self, base):
        self.base = base
        self.seen = set()

    def code(self, address):
        for _ in range(100):
            for message in request(self.base + '/api/v1/messages')['messages']:
                if message['ID'] in self.seen or not any(t['Address'] == address for t in message['To']):
                    continue
                text = request(self.base + '/api/v1/message/' + message['ID'])['Text']
                match = re.search(r'Код [^:\r\n]+: ([A-Za-z0-9_-]{43})', text)
                if match:
                    self.seen.add(message['ID'])
                    return match[1]
            time.sleep(0.2)
        raise DemoError(f'No activation code for {address} in Mailpit')


def by_name(items, name):
    for item in items:
        if item['name'] == name:
            return item['id']
    raise DemoError(f'Dictionary value «{name}» is missing')


def main():
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument('--api', help='API origin; default: the published port of the api service')
    parser.add_argument('--mailpit', help='Mailpit origin; default: the published port of mailpit')
    parser.add_argument('--out', default=str(ROOT / 'var' / 'demo-accounts.txt'), help='where to write demo logins and the password')
    args = parser.parse_args()
    api = args.api or 'http://' + compose('port', 'api', '8080')
    mail = Mailpit(args.mailpit or 'http://' + compose('port', 'mailpit', '8025'))
    request(api + '/health/ready')
    password = 'Demo-' + secrets.token_urlsafe(18)

    print('Accounts', flush=True)
    # Fails when an administrator exists: the script never touches a used database.
    try:
        compose('run', '--rm', '-T', '--no-deps', '--entrypoint', '/admin', 'api', 'bootstrap', 'admin', 'admin@demo.dogclub.test')
    except DemoError as error:
        raise DemoError('the database already has accounts; demo data is loaded only into an empty stand.\n' + str(error))
    anon = Client(api)
    anon.post('/auth/activate', {'token': mail.code('admin@demo.dogclub.test'), 'password': password}, want=(204,))

    def login(name, profile=None):
        body = {'login': name, 'password': password}
        if profile:
            body['profile_id'] = profile
        credentials = anon.post('/auth/login', body, want=(200,))
        return Client(api, credentials['access_token']), credentials

    admin, _ = login('admin')

    # Staff exists before any member card: the administrator gets a staff profile first.
    admin_id = admin.get('/auth/me')['user']['id']
    staff_profile = admin.post(f'/users/{admin_id}/profiles', {'role': 'staff'}, want=(201,))
    bootstrap_staff, _ = login('admin', staff_profile['id'])

    md = bootstrap_staff.get('/member-dictionaries')
    active = by_name(md['membership_statuses'], 'Активный')
    inactive = by_name(md['membership_statuses'], 'Неактивный')
    phone, email = by_name(md['contact_types'], 'Телефон'), by_name(md['contact_types'], 'Email')
    home = by_name(md['address_types'], 'Адрес проживания')

    def member(full_name, birth, status=active, contacts=(), street=None):
        m = bootstrap_staff.post('/members', {'full_name': full_name, 'membership_status_id': status,
                                              'date_of_birth': birth, 'membership_date': day(-700)}, want=(201,))
        for type_id, value in contacts:
            m = bootstrap_staff.post(f'/members/{m["id"]}/contacts', {'type_id': type_id, 'value': value, 'version': m['version']})
        if street:
            m = bootstrap_staff.post(f'/members/{m["id"]}/addresses', {'type_id': home, 'locality': 'Барнаул', 'street': street,
                                                                       'building_number': str(secrets.randbelow(150) + 1), 'version': m['version']})
        return m

    people = {
        'staff': member('Ольга Петрова', '1988-03-14', contacts=[(phone, '+7 900 000-01-01')]),
        'veterinarian': member('Виктор Смирнов', '1979-11-02', contacts=[(phone, '+7 900 000-01-02')]),
        'expert': member('Елена Кузнецова', '1975-06-21'),
        'leader': member('Дмитрий Орлов', '1970-01-30'),
        'maria': member('Мария Иванова', '1992-08-09', contacts=[(phone, '+7 900 000-02-01'), (email, 'maria@demo.dogclub.test')], street='ул. Демонстрационная'),
        'alexey': member('Алексей Соколов', '1985-12-17', contacts=[(phone, '+7 900 000-02-02')], street='пр. Учебный'),
        'natalia': member('Наталья Волкова', '1990-05-05', contacts=[(email, 'natalia@demo.dogclub.test')]),
        'igor': member('Игорь Лебедев', '1983-09-28'),
        'pavel': member('Павел Новиков', '1996-02-11'),
        'ksenia': member('Ксения Фёдорова', '2000-07-19'),
        'svetlana': member('Светлана Морозова', '1965-04-03', status=inactive),
    }

    accounts = [('admin', 'admin', 'Администратор (также сотрудник клуба)')]
    sessions = {}
    for login_name, role, key, title in [
        ('petrova.o', 'staff', 'staff', 'Сотрудник клуба'),
        ('smirnov.v', 'veterinarian', 'veterinarian', 'Ветеринар'),
        ('kuznetsova.e', 'expert', 'expert', 'Эксперт-кинолог'),
        ('orlov.d', 'leader', 'leader', 'Руководитель клуба'),
        ('ivanova.m', 'owner', 'maria', 'Владелец собаки'),
        ('sokolov.a', 'owner', 'alexey', 'Владелец собаки'),
    ]:
        address = f'{login_name}@demo.dogclub.test'
        admin.post('/invitations', {'login': login_name, 'email': address, 'role': role, 'club_member_id': people[key]['id']}, want=(201,))
        anon.post('/auth/activate', {'token': mail.code(address), 'password': password}, want=(204,))
        sessions[role if role != 'owner' else login_name] = login(login_name)
        accounts.append((login_name, role, title))
    staff, staff_credentials = sessions['staff']
    vet, _ = sessions['veterinarian']
    expert, expert_credentials = sessions['expert']

    print('Dogs', flush=True)
    dd = staff.get('/dog-dictionaries')
    sexes = {x['name']: x['id'] for x in dd['sexes']}
    on_book = by_name(dd['statuses'], 'На учёте')
    chip = by_name(dd['identifier_types'], 'Микрочип')
    breeds = {x['name']: x['id'] for x in dd['breeds']}
    colors = {x['name']: x['id'] for x in dd['coat_colors']}
    serial = iter(range(100001, 199999))

    def dog(nickname, sex, breed, birth, color, owner, height=None, weight=None):
        return staff.post('/dogs', {
            'nickname': nickname, 'date_of_birth': birth, 'status_id': on_book, 'breed_id': breeds[breed],
            'coat_color_id': colors[color], 'sex_id': sexes[sex], 'owner_ids': [people[owner]['id']],
            'identifier': {'type_id': chip, 'number': f'643-DEMO-{next(serial)}', 'date': None},
            'height_cm': height, 'weight_kg': weight}, want=(201,))

    def pedigree(d, mother=None, father=None, number='', issuer=''):
        def parent(p):
            if p is None:
                return None
            if isinstance(p, str):
                return {'external': {'nickname': p, 'pedigree_number': 'ДЕМО-ВН-' + str(next(serial)), 'issuer': 'Кинологическая организация (демо)'}}
            return {'dog_id': p['id']}
        return staff.put(f'/dogs/{d["id"]}/pedigree', {'version': d['version'], 'number': number, 'issuer': issuer,
                                                       'mother': parent(mother), 'father': parent(father)})

    def document_pedigree(d):
        return pedigree(d, number=f'ДЕМО-РОД-{next(serial)}', issuer='Кинологическая организация (демо)')

    def set_status(d, name):
        return staff.patch(f'/dogs/{d["id"]}', {'version': d['version'], 'status_id': by_name(dd['statuses'], name)})

    def registered(d, offset):
        return staff.put(f'/dogs/{d["id"]}/registration', {'version': d['version'], 'reason': 'Поставлена на учёт (демо)',
                                                           'registration_date': day(offset)})

    lab = 'Лабрадор-ретривер'
    gsd = 'Немецкая овчарка'
    beagle = 'Бигль'
    richie = document_pedigree(dog('Ричи', 'Кобель', lab, born(6), 'Рыжий', 'maria', 57, 34.5))
    alma = document_pedigree(dog('Альма', 'Сука', lab, born(5, 2, 3), 'Чёрный', 'alexey', 55, 30.2))
    lord = document_pedigree(dog('Лорд', 'Кобель', lab, born(7, 9, 1), 'Чёрный', 'maria'))
    naida = dog('Найда', 'Сука', lab, born(3, 6, 15), 'Белый', 'natalia')
    tyson = set_status(dog('Тайсон', 'Кобель', gsd, born(6, 1, 20), 'Чёрно-подпалый', 'igor'), 'Потеряна')

    # Beagles: Барон × Герда are the parents of the siblings Бублик and Булка.
    baron = document_pedigree(dog('Барон', 'Кобель', beagle, born(9, 3, 8), 'Рыжий', 'pavel'))
    gerda = document_pedigree(dog('Герда', 'Сука', beagle, born(9, 5, 30), 'Чёрно-подпалый', 'pavel'))
    bublik = pedigree(dog('Бублик', 'Кобель', beagle, born(4, 7, 7), 'Рыжий', 'ksenia'), gerda, baron)
    bulka = pedigree(dog('Булка', 'Сука', beagle, born(4, 7, 7), 'Рыжий', 'pavel'), gerda, baron)
    vega = dog('Вега', 'Сука', beagle, born(2, 3, 25), 'Чёрно-подпалый', 'ksenia')

    # German shepherds: a pedigree known through club cards only, without a document.
    grand = [dog(name, sex, gsd, born(13, month, 10), 'Чёрно-подпалый', 'igor')
             for name, sex, month in [('Грета', 'Сука', 2), ('Граф', 'Кобель', 3), ('Дана', 'Сука', 4), ('Дик', 'Кобель', 5)]]
    grand = [pedigree(g, f'Прабабушка {i + 1}', f'Прадедушка {i + 1}') for i, g in enumerate(grand)]
    irma = pedigree(dog('Ирма', 'Сука', gsd, born(8, 8, 8), 'Чёрно-подпалый', 'igor'), grand[0], grand[1])
    rex = pedigree(dog('Рекс', 'Кобель', gsd, born(8, 9, 9), 'Чёрно-подпалый', 'igor'), grand[2], grand[3])
    sherlock = pedigree(dog('Шерлок', 'Кобель', gsd, born(4, 10, 10), 'Чёрно-подпалый', 'svetlana'), irma, rex)
    for old in grand[:2]:
        set_status(old, 'Снята с учёта')
    richie, alma, lord = registered(richie, -700), registered(alma, -650), registered(lord, -900)

    print('Photos', flush=True)
    for d, name in [(richie, 'labrador.jpg'), (bublik, 'beagle.jpg'), (irma, 'german-shepherd.jpg')]:
        staff.photo(d, name)

    print('Veterinary records', flush=True)
    rabies = vet.post('/vaccine-types', {'name': 'Антирабическая вакцина (демо)', 'maker': 'Демо-производитель',
                                         'validity': {'value': 12, 'unit': 'months'}, 'revaccination': {'value': 12, 'unit': 'months'}}, want=(201,))
    complex_ = vet.post('/vaccine-types', {'name': 'Комплексная вакцина DHPPi (демо)', 'maker': 'Демо-производитель',
                                           'validity': {'value': 12, 'unit': 'months'}, 'revaccination': {'value': 12, 'unit': 'months'}}, want=(201,))

    def vaccinate(d, vaccine, offset):
        return vet.post(f'/dogs/{d["id"]}/vaccinations', {'date': day(offset), 'vaccine_type_id': vaccine['id']}, want=(201,))

    def examine(d, offset, result, diagnoses=None):
        return vet.post(f'/dogs/{d["id"]}/examinations', {'date': day(offset), 'description': 'Плановый осмотр (демо)',
                                                          'result': result, 'recommendations': 'Повторный осмотр через год.',
                                                          'diagnoses': diagnoses or []}, want=(201,))

    def clearance(d, exam, purpose, doses, until):
        return vet.post(f'/dogs/{d["id"]}/clearances', {'examination_id': exam['id'], 'purpose': purpose, 'decision': 'allowed',
                                                        'valid_until': day(until), 'vaccination_ids': [x['id'] for x in doses],
                                                        'comment': 'Допуск оформлен для демонстрации.'}, want=(201,))

    doses = {}
    for d in [richie, alma, lord, naida, bublik, bulka, vega, sherlock, irma, rex]:
        # Найда's complex dose is due again in two weeks: a reminder-worthy example.
        doses[d['id']] = [vaccinate(d, rabies, -200), vaccinate(d, complex_, -350 if d is naida else -190)]
    for d in [alma, richie]:
        clearance(d, examine(d, -140, 'Здорова, к вязке допускается.'), 'breeding', doses[d['id']][:1], 200)
    for d in [richie, lord, naida]:
        clearance(d, examine(d, -45, 'Здоров, к выставкам допускается.'), 'events', doses[d['id']][:1], 300)
    examine(vega, -60, 'Перенесла парвовирусную инфекцию, выздоровела.', [{'disease_id': 2, 'is_hereditary': False}])

    print('Breeding', flush=True)
    done = staff.post('/breeding-requests', {'female_dog_id': alma['id'], 'male_dog_id': richie['id'], 'planned_date': day(-120),
                                             'expected_birth_date': day(-57), 'comment': 'Плановая вязка (демо).'}, want=(201,))
    done = expert.post(f'/breeding-requests/{done["id"]}/decisions', {'version': done['version'], 'decision': 'approved',
                                                                     'reason': 'Пара проверена, препятствий нет.'}, want=(200,))
    mating = staff.post(f'/breeding-requests/{done["id"]}/perform', {'version': done['version'], 'date': day(-120),
                                                                     'reason': 'Вязка проведена.'}, want=(201,))
    litter = staff.post('/litters', {'breeding_id': mating['id'], 'date_of_birth': day(-57), 'total_puppies': 3,
                                     'version': mating['version'], 'reason': 'Рождение помёта подтверждено.'}, want=(201,))
    for nickname, sex, color in [('Луна', 'Сука', 'Рыжий'), ('Марс', 'Кобель', 'Рыжий'), ('Нора', 'Сука', 'Чёрный')]:
        litter = staff.post(f'/litters/{litter["id"]}/puppies', {
            'version': litter['version'], 'reason': 'Регистрация щенка.', 'nickname': nickname, 'sex_id': sexes[sex],
            'coat_color_id': colors[color], 'status_id': on_book, 'owner_ids': [people['alexey']['id']],
            'identifier': {'type_id': chip, 'number': f'643-DEMO-{next(serial)}', 'date': day(-20)}})
    staff.post('/breeding-requests', {'female_dog_id': vega['id'], 'male_dog_id': bublik['id'], 'planned_date': day(30),
                                      'comment': 'Ждёт ветеринарных допусков.'}, want=(201,))
    # Siblings: the check shows the lab 3 kinship block shared_parent.
    staff.post('/breeding-requests', {'female_dog_id': bulka['id'], 'male_dog_id': bublik['id'], 'planned_date': day(45),
                                      'comment': 'Пример запрещённой близкородственной вязки.'}, want=(201,))

    print('Events', flush=True)
    ed = staff.get('/event-dictionaries')
    venue = staff.post('/event-venues', {'name': 'Манеж «Лапа» (демо)', 'locality': 'Барнаул', 'street': 'ул. Демонстрационная',
                                         'building_number': '7'}, want=(201,))

    def event(name, type_name, start, hours, audience, classes):
        return staff.post('/events', {'name': name, 'type_id': by_name(ed['types'], type_name), 'venue_id': venue['id'],
                                      'starts_at': at(start, 10), 'ends_at': at(start, 10 + hours), 'timezone': TZ,
                                      'audience': audience, 'uses_classes': classes,
                                      'description': 'Демонстрационное мероприятие.'}, want=(201,))

    def version(e):
        return staff.get(f'/events/{e["id"]}')['version']

    def state(e, status):
        return staff.post(f'/events/{e["id"]}/state', {'version': version(e), 'status': status, 'reason': 'Смена состояния (демо).'})

    def show(name, start):
        e = event(name, 'Выставка', start, 8, 'dogs', True)
        staff.put(f'/events/{e["id"]}/experts', {'version': version(e), 'profile_ids': [expert_credentials['profile']['id']],
                                                 'reason': 'Назначение эксперта.'})
        cls = staff.post(f'/events/{e["id"]}/classes', {'version': version(e), 'name': 'Открытый класс', 'min_age_months': 15,
                                                        'max_age_months': None, 'requires_expert': True, 'enabled': True,
                                                        'requirements': 'Документ родословной и ветеринарный допуск.',
                                                        'reason': 'Настройка класса.'}, want=(201,))
        slot = staff.post(f'/events/{e["id"]}/schedule', {'version': version(e), 'class_id': cls['id'], 'name': 'Ринг 1',
                                                          'starts_at': at(start, 10), 'ends_at': at(start, 13), 'enabled': True,
                                                          'reason': 'Расписание.'}, want=(201,))
        state(e, 'open')
        return e, cls, slot

    def enter(e, member_key, d=None, cls=None, slot=None):
        body = {'member_id': people[member_key]['id']}
        if d:
            body.update(dog_id=d['id'], class_id=cls['id'], schedule_id=slot['id'])
        return staff.post(f'/events/{e["id"]}/registrations', body, want=(201,))

    def qualify(e, r):
        return expert.post(f'/events/{e["id"]}/registrations/{r["id"]}/qualify', {'version': r['version'], 'decision': 'approved',
                                                                                'reason': 'Документы и квалификация проверены.'})

    spring, spring_class, spring_slot = show('Весенняя клубная выставка (демо)', -30)
    entries = [qualify(spring, enter(spring, 'maria', d, spring_class, spring_slot)) for d in (richie, lord)]
    state(spring, 'closed')
    state(spring, 'in_progress')
    entries = [staff.post(f'/events/{spring["id"]}/registrations/{r["id"]}/attend', {'version': r['version'], 'reason': 'Явка отмечена.'})
               for r in entries]
    results = []
    for r, grade, place in zip(entries, ['Отлично', 'Очень хорошо'], [1, 2]):
        results.append(expert.post(f'/events/{spring["id"]}/registrations/{r["id"]}/result', {
            'version': r['version'], 'grade_id': by_name(ed['grades'], grade), 'place': place,
            'comment': 'Оценка эксперта (демо).', 'reason': 'Итог выступления.'}, want=(201,)))
    state(spring, 'completed')
    richie = staff.get(f'/dogs/{richie["id"]}')
    richie = staff.post(f'/dogs/{richie["id"]}/titles', {'version': richie['version'], 'title_id': by_name(dd['titles'], 'Чемпион'),
                                                        'awarded_on': day(-30), 'basis': 'Весенняя клубная выставка (демо), 1 место.'})

    autumn, autumn_class, autumn_slot = show('Осенняя выставка (демо)', 21)
    qualify(autumn, enter(autumn, 'maria', richie, autumn_class, autumn_slot))
    enter(autumn, 'natalia', naida, autumn_class, autumn_slot)   # pedigree_incomplete
    enter(autumn, 'igor', tyson, autumn_class, autumn_slot)      # dog_inactive
    seminar = event('Семинар по уходу за шерстью (демо)', 'Семинар', 10, 3, 'people', False)
    state(seminar, 'open')
    for key in ('maria', 'alexey', 'natalia'):
        enter(seminar, key)

    print('Training', flush=True)
    types = staff.get('/training-types')
    admin.put(f'/training-instructors/{staff_credentials["profile"]["id"]}', {
        'version': 0, 'enabled': True, 'qualification': 'Инструктор по послушанию и социализации (демо).',
        'type_ids': [by_name(types, 'Обучение послушанию'), by_name(types, 'Социализация')], 'reason': 'Допуск инструктора.'})
    for d, offset, type_name, result in [(bublik, -14, 'Обучение послушанию', 'Отработаны команды «сидеть» и «рядом».'),
                                         (vega, -7, 'Социализация', 'Спокойно вела себя среди других собак.')]:
        staff.post(f'/dogs/{d["id"]}/training-sessions', {'type_id': by_name(types, type_name), 'starts_at': at(offset, 10),
                                                          'ends_at': at(offset, 11), 'timezone': TZ, 'result': result}, want=(201,))

    print('Documents', flush=True)
    doc_types = staff.get('/document-types')

    def document(client, type_name, number, template, **links):
        body = {'type_id': by_name(doc_types, type_name), 'number': number, 'issuer': 'Клуб собаководства (демо)', 'issue_date': day(0)}
        body.update(links)
        d = client.post('/documents', body, want=(201,))
        return client.post(f'/documents/{d["id"]}/generate', {'version': d['version'], 'template': template,
                                                              'reason': 'Сформировано для демонстрации.'})

    maria = people['maria']['id']
    document(staff, 'Карточка собаки', 'ДЕМО-КС-001', 'dog_card', dog_id=richie['id'], recipient_member_id=maria)
    document(staff, 'Диплом', 'ДЕМО-ДП-001', 'event_diploma', event_id=spring['id'], event_result_id=results[0]['id'],
             dog_id=richie['id'], recipient_member_id=maria)
    document(staff, 'Акт о регистрации помёта', 'ДЕМО-АП-001', 'litter_act', litter_id=litter['id'], dog_id=alma['id'],
             recipient_member_id=people['alexey']['id'])

    out = Path(args.out)
    out.parent.mkdir(parents=True, exist_ok=True)
    lines = ['Демо-стенд клуба собаководства: синтетические данные.', f'Пароль всех аккаунтов: {password}', '']
    lines += [f'{name:14} {title}' for name, _, title in accounts]
    fd = os.open(out, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(fd, 'w') as handle:
        handle.write('\n'.join(lines) + '\n')
    print(f'Done: 7 accounts, 11 members, {count_dogs(staff)} dogs, 3 events. Logins and the password: {out}', flush=True)


def count_dogs(client):
    total, after = 0, 0
    while True:
        page = client.get(f'/dogs?limit=100&after={after}')
        total += len(page['dogs'])
        if not page.get('next_after'):
            return total
        after = page['next_after']


if __name__ == '__main__':
    try:
        main()
    except DemoError as error:
        raise SystemExit('Demo data stopped: ' + str(error))
