#!/usr/bin/env python3
"""Fill an empty local stand with lifelike club data through the public API.

Requires Python 3 stdlib, Docker Compose and a running stack (api, mailpit).
Two scales: small (a fixed set for UI tests) and large (about 160 people and
250 dogs with three years of history). The same seed gives the same data
relative to today. People, addresses and nicknames are random combinations from
scripts/club-data/*.json; every e-mail is on the local domain dogclub.local, so
mail goes only to Mailpit. The first step bootstraps the administrator: on a
database that already has one the script stops before changing anything. One
random password for all accounts is written to var/club-accounts.txt (mode 600)
and never printed.
"""
import argparse
import calendar
import datetime as dt
import json
import os
from pathlib import Path
import random
import re
import secrets
import subprocess
import time
import urllib.error
import urllib.parse
import urllib.request

ROOT = Path(__file__).resolve().parent.parent
DATA = Path(__file__).resolve().parent / 'club-data'
# Unsplash photos (Unsplash License), credited in THIRD_PARTY_NOTICES.md.
PHOTOS = Path(__file__).resolve().parent / 'demo-photos'
TZ = 'Asia/Novosibirsk'
HOME = 'Новосибирск'
DOMAIN = 'dogclub.local'
TODAY = dt.datetime.now(dt.timezone.utc).date()
HTTP = urllib.request.build_opener(urllib.request.ProxyHandler({}))

SCALES = {
    'small': dict(members=14, founders=14, litters=2, staff=1, vets=1, experts=1, owners=3, shows=1, other_events=1, training=8, years=1),
    'large': dict(members=160, founders=130, litters=32, staff=3, vets=2, experts=3, owners=40, shows=6, other_events=6, training=150, years=3),
}
LAB, GSD, BEAGLE = 'Лабрадор-ретривер', 'Немецкая овчарка', 'Бигль'
KENNELS = ['Северная Звезда', 'Сибирский Простор', 'Лесная Сказка', 'Золотой Берег', 'Серебряный Ветер', 'Радуга Оби',
           'Тёплый Дом', 'Алый Парус']
TRANSLIT = dict(zip('абвгдеёжзийклмнопрстуфхцчшщъыьэюя',
                    ['a', 'b', 'v', 'g', 'd', 'e', 'e', 'zh', 'z', 'i', 'y', 'k', 'l', 'm', 'n', 'o', 'p', 'r', 's', 't', 'u', 'f',
                     'kh', 'ts', 'ch', 'sh', 'shch', '', 'y', '', 'e', 'yu', 'ya']))


class StopLoad(SystemExit):
    pass


# --- dates -----------------------------------------------------------------

def iso(d):
    return d.isoformat()


def days(n):
    return TODAY + dt.timedelta(days=n)


def add_months(d, months):
    month = d.month - 1 + months
    year, month = d.year + month // 12, month % 12 + 1
    return dt.date(year, month, min(d.day, calendar.monthrange(year, month)[1]))


def age_months(birth, on):
    return (on.year - birth.year) * 12 + on.month - birth.month - (on.day < birth.day)


def at(d, hour, minute=0):
    """RFC 3339 local time in Novosibirsk (UTC+7)."""
    return f'{iso(d)}T{hour:02}:{minute:02}:00+07:00'


# --- HTTP ------------------------------------------------------------------

def compose(*args):
    result = subprocess.run(['docker', 'compose', *args], cwd=ROOT, capture_output=True, text=True, timeout=180)
    if result.returncode:
        raise StopLoad('docker compose ' + ' '.join(args[:3]) + ' failed:\n' + result.stderr[-2000:])
    return result.stdout.strip()


class ApiError(StopLoad):
    def __init__(self, message, status, problem):
        super().__init__(message)
        self.status, self.problem = status, problem


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
    for attempt in range(6):
        req = urllib.request.Request(url, data=data, method=method, headers=headers)
        try:
            response = HTTP.open(req, timeout=60)
        except urllib.error.HTTPError as error:
            response = error
        except urllib.error.URLError as error:
            raise StopLoad(f'{method} {url}: no connection ({error.reason}). Is the stack running?')
        with response:
            payload = response.read()
            status = response.status
            if status == 429 and attempt < 5:
                # Login, activation and invitations are limited per minute and address.
                wait = int(response.headers.get('Retry-After') or 60)
                print(f'  rate limit, waiting {wait} s…', flush=True)
                time.sleep(wait)
                continue
        if status not in want:
            problem = {}
            try:
                problem = json.loads(payload)
            except ValueError:
                pass
            path = url.split('/api/v1', 1)[-1]
            raise ApiError(f'{method} {path}: HTTP {status} {problem.get("code", "")}: {problem.get("detail", "")} '
                           f'{problem.get("errors", "")} (request_id {problem.get("request_id", "-")})', status, problem)
        return json.loads(payload) if payload else None
    raise StopLoad(f'{method} {url}: still rate limited')


class Client:
    def __init__(self, base, token=None, profile=None):
        self.base, self.token, self.profile = base, token, profile

    def call(self, method, path, body=None, want=(200,)):
        return request(self.base + '/api/v1' + path, method, body, self.token, want)

    def get(self, path):
        return self.call('GET', path)

    def post(self, path, body=None, want=(200, 201)):
        return self.call('POST', path, body, want)

    def put(self, path, body, want=(200, 201)):
        return self.call('PUT', path, body, want)

    def patch(self, path, body):
        return self.call('PATCH', path, body, want=(200, 204))

    def photo(self, dog_id, name):
        data = (PHOTOS / name).read_bytes()
        boundary = 'club' + secrets.token_hex(12)
        body = (f'--{boundary}\r\nContent-Disposition: form-data; name="version"\r\n\r\n0\r\n'
                f'--{boundary}\r\nContent-Disposition: form-data; name="file"; filename="{name}"\r\n'
                f'Content-Type: image/jpeg\r\n\r\n').encode() + data + f'\r\n--{boundary}--\r\n'.encode()
        return request(f'{self.base}/api/v1/dogs/{dog_id}/photo', 'PUT', token=self.token,
                       raw=(body, 'multipart/form-data; boundary=' + boundary))


class Mailpit:
    def __init__(self, base):
        self.base = base
        self.seen = set()

    def code(self, address):
        query = urllib.parse.quote(f'to:"{address}"')
        for _ in range(150):
            for message in request(f'{self.base}/api/v1/search?query={query}')['messages']:
                if message['ID'] in self.seen:
                    continue
                text = request(self.base + '/api/v1/message/' + message['ID'])['Text']
                match = re.search(r'Код [^:\r\n]+: ([A-Za-z0-9_-]{43})', text)
                if match:
                    self.seen.add(message['ID'])
                    return match[1]
            time.sleep(0.2)
        raise StopLoad(f'No activation code for {address} in Mailpit')


def by_name(items, name):
    for item in items:
        if item['name'] == name:
            return item['id']
    raise StopLoad(f'Dictionary value «{name}» is missing')


# --- the club --------------------------------------------------------------

class Person:
    def __init__(self, full_name, male, birth, joined, city, login):
        self.full_name, self.male, self.birth, self.joined, self.city, self.login = full_name, male, birth, joined, city, login
        self.id = None
        self.version = None
        self.dogs = []
        self.active = True


class Dog:
    def __init__(self, nickname, male, breed, color, birth, owners, joined, mother=None, father=None):
        self.nickname, self.male, self.breed, self.color, self.birth = nickname, male, breed, color, birth
        self.owners, self.joined, self.mother, self.father = owners, joined, mother, father
        self.id = None
        self.version = None
        self.doses = []          # (type key, date, valid_until, id)
        self.litters = []        # birth dates of own litters (females)
        self.end = None          # (status, date) when the dog leaves the register
        self.pedigree_doc = False
        self.registered = False

    def alive_on(self, d):
        return self.birth <= d and (self.end is None or d < self.end[1])

    def ancestors(self, depth=3):
        out = set()
        frontier = [self]
        for _ in range(depth):
            nxt = []
            for d in frontier:
                for p in (d.mother, d.father):
                    if p is not None:
                        out.add(p)
                        nxt.append(p)
            frontier = nxt
        return out


class Loader:
    def __init__(self, args):
        self.args = args
        self.scale = SCALES[args.scale]
        self.rng = random.Random(f'{args.seed}:{args.scale}')
        self.people_data = json.loads((DATA / 'people.json').read_text())
        self.places = json.loads((DATA / 'places.json').read_text())
        self.nicknames = json.loads((DATA / 'nicknames.json').read_text())
        self.photos = json.loads((PHOTOS / 'photos.json').read_text())
        self.api = args.api or 'http://' + compose('port', 'api', '8080')
        self.mail = Mailpit(args.mailpit or 'http://' + compose('port', 'mailpit', '8025'))
        self.password = 'Club-' + secrets.token_urlsafe(18)
        self.anon = Client(self.api)
        self.start = days(-365 * self.scale['years'])
        self.people, self.dogs, self.accounts = [], [], []
        self.logins, self.serials = set(), set()
        self.counts = {}

    # helpers
    def pick(self, items):
        return self.rng.choice(items)

    def chance(self, p):
        return self.rng.random() < p

    def between(self, a, b):
        return a + dt.timedelta(days=self.rng.randint(0, max(0, (b - a).days)))

    def serial(self, prefix, digits=6):
        while True:
            value = f'{prefix}{self.rng.randint(10 ** (digits - 1), 10 ** digits - 1)}'
            if value not in self.serials:
                self.serials.add(value)
                return value

    def login_for(self, surname, first):
        base = ''.join(TRANSLIT.get(ch, ch) for ch in surname.lower()) + '.' + TRANSLIT[first[0].lower()]
        login, n = base, 1
        while login in self.logins:
            n += 1
            login = f'{base}{n}'
        self.logins.add(login)
        return login

    def login(self, name, profile=None):
        body = {'login': name, 'password': self.password}
        if profile:
            body['profile_id'] = profile
        credentials = self.anon.post('/auth/login', body, want=(200,))
        return Client(self.api, credentials['access_token'], credentials['profile']['id']), credentials

    def bump(self, key, n=1):
        self.counts[key] = self.counts.get(key, 0) + n

    # --- accounts and people ---------------------------------------------

    def bootstrap(self):
        print('Accounts', flush=True)
        try:
            compose('run', '--rm', '-T', '--no-deps', '--entrypoint', '/admin', 'api', 'bootstrap', 'admin', f'admin@{DOMAIN}')
        except StopLoad as error:
            raise StopLoad('the database already has accounts; club data is loaded only into an empty stand.\n' + str(error))
        self.anon.post('/auth/activate', {'token': self.mail.code(f'admin@{DOMAIN}'), 'password': self.password}, want=(204,))
        self.admin, _ = self.login('admin')
        admin_id = self.admin.get('/auth/me')['user']['id']
        # The administrator keeps a staff profile: member cards come before any staff account.
        staff_profile = self.admin.post(f'/users/{admin_id}/profiles', {'role': 'staff'}, want=(201,))
        self.office, _ = self.login('admin', staff_profile['id'])
        self.accounts.append(('admin', 'Администратор, также сотрудник клуба'))
        md = self.office.get('/member-dictionaries')
        self.md = {'active': by_name(md['membership_statuses'], 'Активный'), 'inactive': by_name(md['membership_statuses'], 'Неактивный'),
                   'phone': by_name(md['contact_types'], 'Телефон'), 'email': by_name(md['contact_types'], 'Email'),
                   'home': by_name(md['address_types'], 'Адрес проживания')}

    def make_people(self):
        print('Members', flush=True)
        pd = self.people_data
        cities = self.places['cities']
        home = next(c for c in cities if c['name'] == HOME)
        for _ in range(self.scale['members']):
            male = self.chance(0.42)
            surname = self.pick(pd['surnames'])[0 if male else 1]
            first = self.pick(pd['male_names'] if male else pd['female_names'])
            patronymic = self.pick(pd['patronymics'])[0 if male else 1]
            birth = self.between(dt.date(TODAY.year - 72, 1, 1), dt.date(TODAY.year - 19, 12, 31))
            joined = self.between(self.start - dt.timedelta(days=900), days(-20))
            city = home if self.chance(0.6) else self.pick(cities)
            person = Person(f'{surname} {first} {patronymic}', male, birth, joined, city, self.login_for(surname, first))
            m = self.office.post('/members', {'full_name': person.full_name, 'membership_status_id': self.md['active'],
                                              'date_of_birth': iso(birth), 'membership_date': iso(joined)}, want=(201,))
            if self.chance(0.92):
                phone = f'+7 9{self.rng.randint(0, 99):02} {self.rng.randint(100, 999)}-{self.rng.randint(0, 99):02}-{self.rng.randint(0, 99):02}'
                m = self.office.post(f'/members/{m["id"]}/contacts', {'type_id': self.md['phone'], 'value': phone, 'version': m['version']})
            if self.chance(0.7):
                m = self.office.post(f'/members/{m["id"]}/contacts', {'type_id': self.md['email'], 'value': f'{person.login}@{DOMAIN}',
                                                                       'version': m['version']})
            if self.chance(0.8):
                address = {'type_id': self.md['home'], 'locality': city['name'], 'street': self.pick(city['streets']),
                           'building_number': str(self.rng.randint(1, 160)), 'version': m['version']}
                if self.chance(0.75):
                    address['apartment_number'] = str(self.rng.randint(1, 280))
                m = self.office.post(f'/members/{m["id"]}/addresses', address)
            person.id, person.version = m['id'], m['version']
            self.people.append(person)
        self.rng.shuffle(self.people)

    def invite(self, person, role):
        address = f'{person.login}@{DOMAIN}'
        return self.admin.post('/invitations', {'login': person.login, 'email': address, 'role': role, 'club_member_id': person.id}, want=(201,))

    def activate(self, person):
        self.anon.post('/auth/activate', {'token': self.mail.code(f'{person.login}@{DOMAIN}'), 'password': self.password}, want=(204,))

    def make_staff(self):
        """Club workers come from members without dogs of their own at first."""
        pool = list(self.people)
        self.workers = {}
        titles = {'staff': 'Сотрудник клуба', 'veterinarian': 'Ветеринар', 'expert': 'Эксперт-кинолог', 'leader': 'Руководитель клуба'}
        for role, n in [('leader', 1), ('staff', self.scale['staff']), ('veterinarian', self.scale['vets']), ('expert', self.scale['experts'])]:
            for _ in range(n):
                person = pool.pop()
                self.invite(person, role)
                self.activate(person)
                client, credentials = self.login(person.login)
                self.workers.setdefault(role, []).append((person, client, credentials))
                self.accounts.append((person.login, f'{titles[role]} — {person.full_name}'))
        self.staff = [c for _, c, _ in self.workers['staff']]
        self.vets = [c for _, c, _ in self.workers['veterinarian']]
        self.experts = [(c, cr['profile']['id']) for _, c, cr in self.workers['expert']]
        self.worker_people = {p for v in self.workers.values() for p, _, _ in v}

    # --- dogs ------------------------------------------------------------

    def dog_dictionaries(self):
        dd = self.office.get('/dog-dictionaries')
        self.dd = dd
        self.sexes = {x['name']: x['id'] for x in dd['sexes']}
        self.statuses = {x['name']: x['id'] for x in dd['statuses']}
        self.breeds = {x['name']: x['id'] for x in dd['breeds']}
        self.colors = {x['name']: x['id'] for x in dd['coat_colors']}
        self.chip = by_name(dd['identifier_types'], 'Микрочип')
        self.tattoo = by_name(dd['identifier_types'], 'Татуировка')

    def nickname(self, male):
        return self.pick(self.nicknames['male' if male else 'female'])

    def founder_color(self, breed):
        if breed == LAB:
            return self.rng.choices(['Рыжий', 'Чёрный', 'Белый'], [50, 40, 10])[0]
        if breed == GSD:
            return self.rng.choices(['Чёрно-подпалый', 'Чёрный'], [92, 8])[0]
        return self.rng.choices(['Чёрно-подпалый', 'Рыжий'], [82, 18])[0]

    def size(self, breed, male):
        base = {LAB: (56, 30), GSD: (62, 34), BEAGLE: (37, 12)}[breed]
        height = base[0] + (3 if male else 0) + self.rng.randint(-2, 2)
        weight = round(base[1] + (4 if male else 0) + self.rng.uniform(-3, 3), 1)
        return height, weight

    def identifier(self, birth):
        if self.chance(0.88):
            return {'type_id': self.chip, 'number': self.serial('643094', 9), 'date': iso(min(birth + dt.timedelta(days=self.rng.randint(40, 90)), TODAY))}
        return {'type_id': self.tattoo, 'number': self.serial(self.pick(['ABC', 'NSK', 'SBR', 'OBR']), 4), 'date': None}

    def create_dog(self, dog):
        height, weight = self.size(dog.breed, dog.male)
        filled = age_months(dog.birth, TODAY) >= 12 and self.chance(0.7)
        d = self.office.post('/dogs', {
            'nickname': dog.nickname, 'date_of_birth': iso(dog.birth), 'status_id': self.statuses['На учёте'],
            'breed_id': self.breeds[dog.breed], 'coat_color_id': self.colors[dog.color], 'sex_id': self.sexes['Кобель' if dog.male else 'Сука'],
            'owner_ids': [p.id for p in dog.owners], 'identifier': self.identifier(dog.birth),
            'height_cm': height if filled else None, 'weight_kg': weight if filled else None}, want=(201,))
        dog.id, dog.version = d['id'], d['version']
        for p in dog.owners:
            p.dogs.append(dog)
        self.dogs.append(dog)

    def external(self, male):
        return {'external': {'nickname': f'{self.nickname(male)} {self.pick(["фон", "из", "де"])} {self.pick(KENNELS)}'
                             if self.chance(0.3) else self.nickname(male),
                             'pedigree_number': self.serial('РКФ ', 7), 'issuer': 'РКФ'}}

    def make_founders(self):
        print('Dogs', flush=True)
        self.dog_dictionaries()
        owners = [p for p in self.people if p not in self.worker_people] + list(self.worker_people)[:2]
        breeds = [LAB] * 4 + [GSD] * 3 + [BEAGLE] * 3
        for i in range(self.scale['founders']):
            breed = breeds[i % len(breeds)]
            male = i % 2 == 0
            birth = self.between(days(-365 * 13), days(-365 * 2 - 120))
            owner = owners[i % len(owners)] if i < len(owners) else self.pick(owners)
            group = [owner] + ([self.pick(owners)] if self.chance(0.12) else [])
            # Most dogs were already in the club when its records begin; the rest join later.
            came = self.between(self.start - dt.timedelta(days=400), self.start + dt.timedelta(days=150)) if self.chance(0.7) \
                else self.between(self.start, days(-200))
            joined = max(came, birth + dt.timedelta(days=60), owner.joined)
            dog = Dog(self.nickname(male), male, breed, self.founder_color(breed), birth, list(dict.fromkeys(group)), min(joined, days(-30)))
            self.create_dog(dog)
            if self.chance(0.85):
                dog.pedigree_doc = True
                d = self.office.put(f'/dogs/{dog.id}/pedigree', {'version': dog.version, 'number': self.serial('РКФ ', 7), 'issuer': 'РКФ',
                                                                 'mother': self.external(False), 'father': self.external(True)})
                dog.version = d['version']
            self.register(dog, dog.joined)
        self.bump('founders', self.scale['founders'])

    def register(self, dog, on):
        d = self.office.put(f'/dogs/{dog.id}/registration', {'version': dog.version, 'reason': 'Поставлена на учёт в клубе.',
                                                             'registration_date': iso(min(on, TODAY))})
        dog.version = d['version']
        dog.registered = True

    # --- veterinary ------------------------------------------------------

    def vaccine_types(self):
        vet = self.vets[0]
        twelve = {'value': 12, 'unit': 'months'}
        self.vaccines = {
            'rabies': vet.post('/vaccine-types', {'name': 'Рабикан', 'maker': 'Щёлковский биокомбинат', 'validity': twelve, 'revaccination': twelve}, want=(201,)),
            'complex': vet.post('/vaccine-types', {'name': 'Мультикан-8', 'maker': 'НПО «Нарвак»', 'validity': twelve, 'revaccination': twelve}, want=(201,)),
            'lepto': vet.post('/vaccine-types', {'name': 'Нобивак Lepto', 'maker': 'MSD Animal Health', 'validity': twelve, 'revaccination': twelve}, want=(201,)),
        }

    def vaccinate_history(self, dog):
        """Yearly doses from the dog's first club record; some owners skip or delay a dose."""
        vet = self.pick(self.vets)
        first = max(dog.joined, dog.birth + dt.timedelta(days=60))
        for key, offset in [('complex', 0), ('rabies', 28), ('lepto', 35)]:
            if key == 'lepto' and not self.chance(0.4):
                continue
            date = first + dt.timedelta(days=offset + self.rng.randint(0, 10))
            while date <= TODAY and (dog.end is None or date < dog.end[1]):
                dose = vet.post(f'/dogs/{dog.id}/vaccinations', {'date': iso(date), 'vaccine_type_id': self.vaccines[key]['id']}, want=(201,))
                dog.doses.append((key, date, add_months(date, 12), dose['id']))
                self.bump('vaccinations')
                if self.chance(0.04):
                    break                       # the owner stopped coming
                late = self.rng.randint(60, 200) if self.chance(0.12) else self.rng.randint(0, 25)
                date = add_months(date, 12) + dt.timedelta(days=late)

    def valid_doses(self, dog, exam, until):
        chosen = []
        for key in ('rabies', 'complex'):
            fit = [d for d in dog.doses if d[0] == key and d[1] <= exam and d[2] >= until]
            if not fit:
                return None
            chosen.append(max(fit, key=lambda d: d[1])[3])
        return chosen

    def clearance(self, dog, purpose, until, comment):
        """An examination shortly before `until` with an allowed clearance, or None when doses do not cover it."""
        for back in (self.rng.randint(5, 30), 3, 1):
            exam = min(until - dt.timedelta(days=back), TODAY)
            if exam < dog.birth:
                continue
            doses = self.valid_doses(dog, exam, until)
            if doses:
                break
        else:
            return None
        vet = self.pick(self.vets)
        result = 'Клинически здоров.' if purpose == 'events' else 'Клинически здорова, к вязке допускается.' if not dog.male else 'Клинически здоров, к вязке допускается.'
        e = vet.post(f'/dogs/{dog.id}/examinations', {'date': iso(exam), 'description': self.pick([
            'Плановый осмотр перед ' + ('выставкой.' if purpose == 'events' else 'вязкой.'),
            'Общий клинический осмотр, температура и пульс в норме.', 'Осмотр кожи, слизистых, сердца и суставов.']),
            'result': result, 'recommendations': self.pick(['Повторный осмотр через год.', 'Сохранить текущий рацион.', 'Обработка от клещей весной.']),
            'diagnoses': []}, want=(201,))
        self.bump('examinations')
        c = vet.post(f'/dogs/{dog.id}/clearances', {'examination_id': e['id'], 'purpose': purpose, 'decision': 'allowed',
                                                    'valid_until': iso(max(until, exam + dt.timedelta(days=self.rng.randint(90, 180)))),
                                                    'vaccination_ids': doses, 'comment': comment}, want=(201,))
        self.bump('clearances')
        return e, c

    def routine_exams(self):
        """Ordinary visits: most dogs see the vet once or twice a year; a few have diagnoses."""
        for dog in self.dogs:
            visits = self.rng.randint(0, 2 if self.scale['years'] > 1 else 1)
            for _ in range(visits):
                when = self.between(max(dog.joined, dog.birth + dt.timedelta(days=90)), dog.end[1] - dt.timedelta(days=1) if dog.end else TODAY)
                if when > TODAY or when < dog.birth:
                    continue
                diagnoses, result = [], 'Клинически здоров.' if dog.male else 'Клинически здорова.'
                if self.chance(0.08):
                    diagnoses = [{'disease_id': self.rng.choice([2, 3, 5, 7]), 'is_hereditary': False}]
                    result = 'Перенесла инфекцию, выздоровление.' if not dog.male else 'Перенёс инфекцию, выздоровление.'
                self.pick(self.vets).post(f'/dogs/{dog.id}/examinations', {
                    'date': iso(when), 'description': self.pick(['Плановый осмотр.', 'Жалобы на снижение аппетита.', 'Осмотр после прогулки, хромота.',
                                                                 'Профилактический осмотр, чистка ушей.', 'Контроль веса.']),
                    'result': result, 'recommendations': self.pick(['Повторный осмотр через год.', 'Контроль через две недели.',
                                                                    'Ограничить нагрузку на неделю.', 'Сбалансировать рацион.']),
                    'diagnoses': diagnoses}, want=(201,))
                self.bump('examinations')

    # --- breeding --------------------------------------------------------

    def related(self, a, b):
        aa, ba = a.ancestors(), b.ancestors()
        return a in ba or b in aa or bool(aa & ba) or bool({a.mother, a.father} & {b.mother, b.father} - {None})

    def pairs(self, on):
        """Unrelated pairs of one breed and breeding age, a female at most once a year."""
        females = [d for d in self.dogs if not d.male and d.alive_on(on) and 22 <= age_months(d.birth, on) <= 96
                   and all((on - b).days > 330 for b in d.litters)]
        self.rng.shuffle(females)
        for female in females:
            males = [d for d in self.dogs if d.male and d.breed == female.breed and d.alive_on(on) and 18 <= age_months(d.birth, on) <= 120
                     and not self.related(female, d)]
            self.rng.shuffle(males)
            for male in males[:3]:
                yield female, male

    def pair(self, on, vaccinated=False):
        for female, male in self.pairs(on):
            exam = min(on - dt.timedelta(days=10), TODAY)
            if not vaccinated or (self.valid_doses(female, exam, on) and self.valid_doses(male, exam, on)):
                return female, male
        return None

    def expert(self):
        return self.pick(self.experts)[0]

    def request_pair(self, female, male, planned, comment, expected=None):
        body = {'female_dog_id': female.id, 'male_dog_id': male.id, 'planned_date': iso(planned), 'comment': comment}
        if expected:
            body['expected_birth_date'] = iso(expected)
        self.bump('breeding_requests')
        return self.pick(self.staff).post('/breeding-requests', body, want=(201,))

    def make_litters(self):
        print('Breeding', flush=True)
        self.litters = []
        n = self.scale['litters']
        span = (days(-75) - (self.start + dt.timedelta(days=60))).days
        for i in range(n):
            mating = self.start + dt.timedelta(days=60 + span * i // max(1, n - 1) + self.rng.randint(-10, 10))
            mating = min(mating, days(-75))
            found = self.pair(mating, vaccinated=True)
            if not found:
                self.bump('no_pair')
                continue
            female, male = found
            if not (self.clearance(female, 'breeding', mating, 'Допуск к вязке.') and self.clearance(male, 'breeding', mating, 'Допуск к вязке.')):
                continue
            expected = mating + dt.timedelta(days=63)
            if female.litters:
                expected = max(expected, max(female.litters) + dt.timedelta(days=365))
            r = self.request_pair(female, male, mating, self.pick(['Плановая вязка.', 'Вязка по рекомендации эксперта.', 'Первая вязка суки.'
                                                                     if not female.litters else 'Повторная вязка.']), expected)
            try:
                r = self.expert().post(f'/breeding-requests/{r["id"]}/decisions', {'version': r['version'], 'decision': 'approved',
                                                                                   'reason': 'Пара проверена, препятствий нет.'}, want=(200,))
            except ApiError as error:
                print('  skipped pair:', error.problem.get('detail', error), flush=True)
                continue
            staff = self.pick(self.staff)
            done = staff.post(f'/breeding-requests/{r["id"]}/perform', {'version': r['version'], 'date': iso(mating), 'reason': 'Вязка проведена.'}, want=(201,))
            if self.chance(0.08):
                staff.put(f'/breedings/{done["id"]}', {'version': done['version'], 'date': iso(mating), 'outcome': 'no_litter',
                                                        'reason': 'Сука не ощенилась.'})
                self.bump('no_litter')
                continue
            birth = mating + dt.timedelta(days=self.rng.randint(59, 65))
            total = self.rng.randint(3, 9) if female.breed != BEAGLE else self.rng.randint(3, 7)
            litter = staff.post('/litters', {'breeding_id': done['id'], 'date_of_birth': iso(birth), 'total_puppies': total,
                                             'version': done['version'], 'reason': 'Рождение помёта подтверждено.'}, want=(201,))
            female.litters.append(birth)
            self.bump('litters')
            self.puppies(litter, female, male, birth, total)

    def puppy_color(self, mother, father):
        if mother.breed == LAB:
            return self.pick([mother.color, father.color])
        return mother.color if self.chance(0.7) else father.color

    def puppies(self, litter, mother, father, birth, total):
        kennel = self.pick(KENNELS)
        letter = None
        registered = min(total, self.rng.randint(3, 6))
        breeder = mother.owners[0]
        buyers = [p for p in self.people if p.joined <= birth + dt.timedelta(days=90) and p not in self.worker_people] or self.people
        staff = self.pick(self.staff)
        for k in range(registered):
            male = self.chance(0.5)
            for _ in range(40):
                name = self.nickname(male)
                if letter is None or name[0] == letter:
                    break
            letter = letter or name[0]
            owner = breeder if k == 0 and self.chance(0.5) else self.pick(buyers)
            puppy = Dog(f'{kennel} {name}', male, mother.breed, self.puppy_color(mother, father), birth, [owner],
                        max(birth + dt.timedelta(days=45), owner.joined), mother, father)
            ident = self.identifier(birth)
            ident['date'] = ident['date'] and iso(min(birth + dt.timedelta(days=45), TODAY))
            litter = staff.post(f'/litters/{litter["id"]}/puppies', {
                'version': litter['version'], 'reason': 'Регистрация щенка.', 'nickname': puppy.nickname, 'sex_id': self.sexes['Кобель' if male else 'Сука'],
                'coat_color_id': self.colors[puppy.color], 'status_id': self.statuses['На учёте'], 'owner_ids': [owner.id], 'identifier': ident})
            known = {d.id for d in self.dogs}
            puppy.id = next(i for i in reversed(litter['dog_ids']) if i not in known)
            puppy.version = self.office.get(f'/dogs/{puppy.id}')['version']
            owner.dogs.append(puppy)
            self.dogs.append(puppy)
            if puppy.joined <= TODAY:
                self.register(puppy, puppy.joined)
            self.vaccinate_history(puppy)
            self.bump('puppies')

    def open_requests(self):
        """Requests that are still in the works today: pending, approved for later, rejected and cancelled."""
        made = 0
        for planned, comment in [(days(20), 'Ждёт решения эксперта.'), (days(35), 'Владелец просит подобрать дату.'), (days(50), 'Плановая вязка.')]:
            found = self.pair(planned)
            if found:
                self.request_pair(*found, planned, comment)
                made += 1
        siblings = [(a, b) for a in self.dogs for b in self.dogs if not a.male and b.male and a.mother is not None and a.mother is b.mother
                    and a.alive_on(TODAY) and b.alive_on(TODAY)]
        if siblings:
            female, male = siblings[0]
            r = self.request_pair(female, male, days(40), 'Владелец предлагает вязку брата и сестры.')
            self.expert().post(f'/breeding-requests/{r["id"]}/decisions', {'version': r['version'], 'decision': 'rejected',
                                                                           'reason': 'Близкородственная вязка: общие родители.'}, want=(200,))
            self.bump('rejected')
        found = self.pair(days(60))
        if found:
            r = self.request_pair(*found, days(60), 'Предварительная заявка.')
            self.pick(self.staff).post(f'/breeding-requests/{r["id"]}/cancel', {'version': r['version'], 'reason': 'Владелец отказался от вязки.'})
            self.bump('cancelled')

    # --- events ----------------------------------------------------------

    def event_setup(self):
        staff = self.staff[0]
        ed = staff.get('/event-dictionaries')
        self.ed = ed
        self.venues = []
        venues = sorted(self.places['venues'], key=lambda v: v['city'] != HOME)
        for v in venues[: 3 if self.scale['years'] == 1 else 6]:
            self.venues.append(staff.post('/event-venues', {'name': v['name'], 'locality': v['city'], 'street': v['street'],
                                                            'building_number': str(self.rng.randint(1, 90))}, want=(201,)))

    def event(self, staff, name, type_name, start, hours, audience, classes, description):
        return staff.post('/events', {'name': name, 'type_id': by_name(self.ed['types'], type_name), 'venue_id': self.pick(self.venues[:3])['id'],
                                      'starts_at': at(start, 10), 'ends_at': at(start, 10 + hours), 'timezone': TZ,
                                      'audience': audience, 'uses_classes': classes, 'description': description}, want=(201,))

    def state(self, staff, e, status, reason='Смена состояния.'):
        v = staff.get(f'/events/{e["id"]}')['version']
        return staff.post(f'/events/{e["id"]}/state', {'version': v, 'status': status, 'reason': reason})

    def show(self, name, start, past):
        staff = self.pick(self.staff)
        e = self.event(staff, name, 'Выставка', start, 8, 'dogs', True,
                       'Клубная выставка всех пород: ринги юниоров, открытого класса и ветеранов.')
        experts = self.rng.sample(self.experts, min(2, len(self.experts)))
        v = staff.get(f'/events/{e["id"]}')['version']
        staff.put(f'/events/{e["id"]}/experts', {'version': v, 'profile_ids': [p for _, p in experts], 'reason': 'Назначение экспертов.'})
        rings = []
        for i, (cls_name, lo, hi) in enumerate([('Класс юниоров', 9, 18), ('Открытый класс', 15, None), ('Класс ветеранов', 96, None)]):
            v = staff.get(f'/events/{e["id"]}')['version']
            cls = staff.post(f'/events/{e["id"]}/classes', {'version': v, 'name': cls_name, 'min_age_months': lo, 'max_age_months': hi,
                                                            'requires_expert': True, 'enabled': True,
                                                            'requirements': 'Родословная, клеймо или чип, ветеринарный допуск.',
                                                            'reason': 'Настройка класса.'}, want=(201,))
            v = staff.get(f'/events/{e["id"]}')['version']
            slot = staff.post(f'/events/{e["id"]}/schedule', {'version': v, 'class_id': cls['id'], 'name': f'Ринг {i + 1}',
                                                              'starts_at': at(start, 10 + i * 2), 'ends_at': at(start, 12 + i * 2), 'enabled': True,
                                                              'reason': 'Расписание рингов.'}, want=(201,))
            rings.append((cls, slot, lo, hi))
        self.state(staff, e, 'open', 'Запись открыта.')
        entries = []
        candidates = [d for d in self.dogs if d.alive_on(start) and (d.pedigree_doc or d.mother is not None) and age_months(d.birth, start) >= 9]
        self.rng.shuffle(candidates)
        limit = self.rng.randint(14, 30) if self.scale['years'] > 1 else 4
        for dog in candidates:
            if len(entries) >= limit:
                break
            months = age_months(dog.birth, start)
            ring = next(((c, s) for c, s, lo, hi in reversed(rings) if months >= lo and (hi is None or months <= hi)), None)
            if not ring:
                continue
            if past and not self.clearance(dog, 'events', start, 'Допуск к выставке.'):
                continue
            if not past and self.chance(0.6):
                self.clearance(dog, 'events', start, 'Допуск к выставке.')
            r = staff.post(f'/events/{e["id"]}/registrations', {'member_id': dog.owners[0].id, 'dog_id': dog.id,
                                                                'class_id': ring[0]['id'], 'schedule_id': ring[1]['id']}, want=(201,))
            entries.append((dog, ring, r))
            self.bump('show_entries')
        expert_client = experts[0][0]
        qualified = []
        for dog, ring, r in entries:
            if not past and self.chance(0.5):
                continue
            try:
                r = expert_client.post(f'/events/{e["id"]}/registrations/{r["id"]}/qualify', {'version': r['version'], 'decision': 'approved',
                                                                                             'reason': 'Документы и квалификация проверены.'})
                qualified.append((dog, ring, r))
            except ApiError as error:
                if past and error.status != 422:
                    raise
        if not past:
            return e, []
        self.state(staff, e, 'closed', 'Запись закрыта.')
        self.state(staff, e, 'in_progress', 'Выставка началась.')
        results = []
        by_ring = {}
        for dog, ring, r in qualified:
            if self.chance(0.08):
                continue                                  # did not come
            r = staff.post(f'/events/{e["id"]}/registrations/{r["id"]}/attend', {'version': r['version'], 'reason': 'Явка отмечена.'})
            by_ring.setdefault(ring[0]['id'], []).append((dog, r))
        for ring_entries in by_ring.values():
            self.rng.shuffle(ring_entries)
            for place, (dog, r) in enumerate(ring_entries, 1):
                grade = 'Отлично' if place <= 2 else self.pick(['Отлично', 'Очень хорошо', 'Очень хорошо', 'Хорошо'])
                result = expert_client.post(f'/events/{e["id"]}/registrations/{r["id"]}/result', {
                    'version': r['version'], 'grade_id': by_name(self.ed['grades'], grade), 'place': place if place <= 4 else None,
                    'comment': self.pick(['Хорошая голова, правильный прикус.', 'Свободные движения, отличная шерсть.', 'Крепкий костяк, нужна выставочная подготовка.',
                                          'Уверенный темперамент.', 'Гармоничное сложение.', 'Немного прямые углы задних конечностей.']),
                    'reason': 'Итог выступления.'}, want=(201,))
                results.append((dog, place, result))
                self.bump('results')
        self.state(staff, e, 'completed', 'Выставка завершена.')
        for dog, place, _ in results:
            if place == 1 and self.chance(0.5):
                d = self.office.get(f'/dogs/{dog.id}')
                title = 'Юный чемпион' if age_months(dog.birth, start) <= 18 else self.pick(['Чемпион', 'Чемпион', 'Чемпион породы'])
                try:
                    self.office.post(f'/dogs/{dog.id}/titles', {'version': d['version'], 'title_id': by_name(self.dd['titles'], title),
                                                                 'awarded_on': iso(start), 'basis': f'{name}, 1 место.'})
                    self.bump('titles')
                except ApiError:
                    pass
        return e, results

    def people_event(self, name, type_name, start, hours, past, description):
        staff = self.pick(self.staff)
        e = self.event(staff, name, type_name, start, hours, 'people', False, description)
        self.state(staff, e, 'open', 'Запись открыта.')
        guests = self.rng.sample(self.people, min(len(self.people), self.rng.randint(6, 25) if self.scale['years'] > 1 else 4))
        regs = [staff.post(f'/events/{e["id"]}/registrations', {'member_id': p.id}, want=(201,)) for p in guests]
        self.bump('event_guests', len(regs))
        if past:
            self.state(staff, e, 'closed', 'Запись закрыта.')
            self.state(staff, e, 'in_progress', 'Мероприятие началось.')
            for r in regs:
                if self.chance(0.85):
                    staff.post(f'/events/{e["id"]}/registrations/{r["id"]}/attend', {'version': r['version'], 'reason': 'Явка отмечена.'})
            self.state(staff, e, 'completed', 'Мероприятие завершено.')
        return e

    def make_events(self):
        print('Events', flush=True)
        self.event_setup()
        self.shows = []
        n = self.scale['shows']
        for i in range(n):
            start = days(-30 - (n - 1 - i) * (365 * self.scale['years'] - 60) // max(1, n))
            season = 'Весенняя' if start.month in (3, 4, 5, 6) else 'Летняя' if start.month in (7, 8) else 'Осенняя' if start.month in (9, 10, 11) else 'Зимняя'
            self.shows.append(self.show(f'{season} клубная выставка {start.year}', start, past=True))
        self.upcoming, _ = self.show(f'Выставка «Золотая осень» {days(25).year}' if days(25).month in (9, 10, 11) else f'Клубная выставка {days(25).year}',
                                     days(25), past=False)
        topics = [('Семинар', 'Семинар по уходу за шерстью', 3), ('Семинар', 'Семинар «Первая помощь собаке»', 3),
                  ('Конференция', 'Конференция заводчиков клуба', 5), ('Форум', 'Форум владельцев собак', 4),
                  ('Семинар', 'Семинар по подготовке к выставке', 3), ('Конференция', 'Ветеринарная конференция', 6)]
        k = self.scale['other_events']
        for i in range(k):
            type_name, name, hours = topics[i % len(topics)]
            start = days(-20 - (k - i) * (365 * self.scale['years'] - 40) // max(1, k + 1))
            self.people_event(f'{name} {start.year}', type_name, start, hours, True, 'Открытое мероприятие для членов клуба.')
        self.people_event('Семинар «Собака в городе»', 'Семинар', days(12), 3, False, 'Как приучить собаку к шуму, транспорту и лифту.')
        if self.scale['years'] > 1:
            staff = self.pick(self.staff)
            e = self.event(staff, 'Летние соревнования по послушанию', 'Соревнование', days(-100), 6, 'dogs', False, 'Перенесено из-за погоды.')
            self.state(staff, e, 'cancelled', 'Отменено из-за штормового предупреждения.')
            self.bump('cancelled_events')

    # --- training --------------------------------------------------------

    def make_training(self):
        print('Training', flush=True)
        types = self.staff[0].get('/training-types')
        instructors = self.workers['staff'][: max(1, len(self.workers['staff']) - 1)]
        for person, client, credentials in instructors:
            self.admin.put(f'/training-instructors/{credentials["profile"]["id"]}', {
                'version': 0, 'enabled': True, 'qualification': self.pick(['Инструктор по дрессировке, ОКД и ЗКС.', 'Инструктор по послушанию и социализации.']),
                'type_ids': [x['id'] for x in types], 'reason': 'Допуск инструктора.'})
        results = {'Обучение послушанию': ['Отработаны команды «сидеть» и «рядом».', 'Выдержка на месте 1 минута.', 'Подзыв с длинного поводка.'],
                   'Подготовка к соревнованиям': ['Апорт на 10 метров.', 'Движение рядом без поводка.', 'Барьер 50 см.'],
                   'Подготовка к выставкам': ['Стойка в ринге, показ зубов.', 'Движение по кругу рысью.', 'Спокойно позволяет осмотр эксперту.'],
                   'Социализация': ['Спокойно вела себя среди других собак.', 'Прогулка у дороги, реакция на транспорт ровная.', 'Знакомство с новыми людьми.']}
        alive = [d for d in self.dogs if d.alive_on(TODAY) and age_months(d.birth, TODAY) >= 4]
        for _ in range(self.scale['training']):
            dog = self.pick(alive)
            when = self.between(max(dog.joined, days(-365)), days(-1))
            t = self.pick(types)
            hour = self.rng.choice([9, 10, 11, 17, 18, 19])
            _, client, _ = self.pick(instructors)
            try:
                client.post(f'/dogs/{dog.id}/training-sessions', {'type_id': t['id'], 'starts_at': at(when, hour), 'ends_at': at(when, hour + 1),
                                                                   'timezone': TZ, 'result': self.pick(results.get(t['name'], ['Занятие проведено.']))}, want=(201,))
                self.bump('training')
            except ApiError as error:
                if error.status != 409:
                    raise

    # --- accounts of owners ------------------------------------------------

    def make_owner_accounts(self):
        print('Owner accounts', flush=True)
        owners = [p for p in self.people if p.dogs and p not in self.worker_people]
        self.rng.shuffle(owners)
        owners = owners[: self.scale['owners']]
        for i, person in enumerate(owners):
            invitation = self.invite(person, 'owner')
            if self.scale['years'] > 1 and i in (1, 2):
                continue                                   # invited, not yet signed in
            if self.scale['years'] > 1 and i == 3:
                self.admin.call('DELETE', f'/invitations/{invitation["id"]}', want=(204,))
                continue
            self.activate(person)
            self.accounts.append((person.login, f'Владелец собаки — {person.full_name}'))
            self.bump('owner_accounts')
        if self.scale['years'] > 1:
            users = self.users()
            for person in owners[4:6]:
                self.admin.patch(f'/users/{users[person.login]}', {'blocked': True})
                self.accounts = [a for a in self.accounts if a[0] != person.login]
                self.accounts.append((person.login, f'Заблокирован — {person.full_name}'))
            # A former staff member: the staff profile is off, the owner profile works.
            person, _, _ = self.workers['staff'][-1]
            uid = users[person.login]
            profiles = self.admin.get(f'/users/{uid}/profiles')['profiles']
            self.admin.post(f'/users/{uid}/profiles', {'role': 'owner'}, want=(201,))
            staff_profile = next(p for p in profiles if p['role'] == 'staff')
            self.admin.patch(f'/users/{uid}/profiles/{staff_profile["id"]}', {'enabled': False})
            self.accounts = [a for a in self.accounts if a[0] != person.login]
            self.accounts.append((person.login, f'Бывший сотрудник: профиль сотрудника отключён — {person.full_name}'))
            # An expert who also owns dogs has a second profile.
            person, _, _ = self.workers['expert'][0]
            self.admin.post(f'/users/{users[person.login]}/profiles', {'role': 'owner'}, want=(201,))

    def users(self):
        out, after = {}, ''
        while True:
            page = self.admin.get('/users?limit=100' + (f'&after={after}' if after else ''))
            for u in page['users']:
                out[u['login']] = u['id']
            after = page.get('next_after')
            if not after:
                return out

    # --- endings, photos, documents --------------------------------------

    def endings(self):
        """Some dogs leave the register; a few members stop paying dues."""
        for dog in self.dogs:
            years = age_months(dog.birth, TODAY) / 12
            status = None
            if years > 11.5 and self.chance(0.45):
                status = 'Умерла'
            elif self.chance(0.025):
                status = 'Снята с учёта'
            elif self.chance(0.006):
                status = 'Потеряна'
            if status:
                d = self.office.get(f'/dogs/{dog.id}')
                self.office.patch(f'/dogs/{dog.id}', {'version': d['version'], 'status_id': self.statuses[status]})
                dog.end = (status, TODAY)
                self.bump('left_register')
        for person in self.people:
            if person not in self.worker_people and self.chance(0.08):
                m = self.office.get(f'/members/{person.id}')
                self.office.patch(f'/members/{person.id}', {'version': m['version'], 'membership_status_id': self.md['inactive']})
                person.active = False
                self.bump('inactive_members')

    def make_photos(self):
        print('Photos', flush=True)
        photos = list(self.photos)
        self.rng.shuffle(photos)
        limit = len(photos) if self.scale['years'] > 1 else 10
        dogs = [d for d in self.dogs if d.end is None or d.end[0] != 'Потеряна']
        self.rng.shuffle(dogs)
        used = set()
        for photo in photos[:limit]:
            for dog in dogs:
                young = age_months(dog.birth, TODAY) < 12
                if dog.id in used or dog.breed != photo['breed'] or dog.color != photo['color'] or young != photo['puppy']:
                    continue
                self.office.photo(dog.id, photo['file'])
                used.add(dog.id)
                self.bump('photos')
                break

    def make_documents(self):
        print('Documents', flush=True)
        staff = self.staff[0]
        types = staff.get('/document-types')

        def document(client, type_name, number, template, issued, **links):
            body = {'type_id': by_name(types, type_name), 'number': number, 'issuer': 'Клуб собаководства', 'issue_date': iso(issued)}
            body.update(links)
            d = client.post('/documents', body, want=(201,))
            client.post(f'/documents/{d["id"]}/generate', {'version': d['version'], 'template': template, 'reason': 'Документ сформирован.'})
            self.bump('documents')

        year = TODAY.year
        alive = [d for d in self.dogs if d.end is None]
        for i, dog in enumerate(self.rng.sample(alive, min(len(alive), 12 if self.scale['years'] > 1 else 2))):
            document(staff, 'Карточка собаки', f'КС-{year}-{i + 1:04}', 'dog_card', days(-self.rng.randint(1, 200)),
                     dog_id=dog.id, recipient_member_id=dog.owners[0].id)
        registered = [d for d in alive if d.registered]
        for i, dog in enumerate(self.rng.sample(registered, min(len(registered), 10 if self.scale['years'] > 1 else 1))):
            document(staff, 'Регистрационное свидетельство', f'РС-{year}-{i + 1:04}', 'registration_certificate', days(-self.rng.randint(1, 300)),
                     dog_id=dog.id, recipient_member_id=dog.owners[0].id)
        members = [p for p in self.people if p.active]
        for i, person in enumerate(self.rng.sample(members, min(len(members), 6 if self.scale['years'] > 1 else 1))):
            document(staff, 'Карточка члена клуба', f'КЧ-{year}-{i + 1:04}', 'member_card', days(-self.rng.randint(1, 300)), member_id=person.id)
        n = 0
        for (e, results) in self.shows[-2:]:
            for dog, place, result in results:
                if place == 1:
                    n += 1
                    document(staff, 'Диплом', f'ДП-{year}-{n:04}', 'event_diploma', TODAY, event_id=e['id'], event_result_id=result['id'],
                             dog_id=dog.id, recipient_member_id=dog.owners[0].id)

    # --- run -------------------------------------------------------------

    def write_accounts(self):
        out = Path(self.args.out)
        out.parent.mkdir(parents=True, exist_ok=True)
        lines = [f'Стенд клуба собаководства ({self.args.scale}): вымышленные данные.', f'Пароль всех аккаунтов: {self.password}', '']
        lines += [f'{name:20} {title}' for name, title in self.accounts]
        fd = os.open(out, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
        with os.fdopen(fd, 'w') as handle:
            handle.write('\n'.join(lines) + '\n')
        return out

    def run(self):
        request(self.api + '/health/ready')
        began = time.monotonic()
        self.bootstrap()
        self.make_people()
        self.make_staff()
        self.make_founders()
        print('Veterinary records', flush=True)
        self.vaccine_types()
        for dog in list(self.dogs):
            self.vaccinate_history(dog)
        self.make_litters()
        self.open_requests()
        self.routine_exams()
        self.make_events()
        self.make_training()
        self.make_owner_accounts()
        self.make_documents()
        self.endings()
        self.make_photos()
        out = self.write_accounts()
        summary = ', '.join(f'{k} {v}' for k, v in sorted(self.counts.items()))
        print(f'Done in {int(time.monotonic() - began)} s: {len(self.people)} members, {len(self.dogs)} dogs; {summary}.', flush=True)
        print(f'Logins and the password: {out}', flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument('--scale', choices=sorted(SCALES), default='large', help='small: a fixed set for UI tests; large: a lived-in club')
    parser.add_argument('--seed', default='2026', help='the same seed gives the same data relative to today')
    parser.add_argument('--api', help='API origin; default: the published port of the api service')
    parser.add_argument('--mailpit', help='Mailpit origin; default: the published port of mailpit')
    parser.add_argument('--out', default=str(ROOT / 'var' / 'club-accounts.txt'), help='where to write logins and the password')
    Loader(parser.parse_args()).run()


if __name__ == '__main__':
    try:
        main()
    except StopLoad as error:
        raise SystemExit('Club data stopped: ' + str(error))
