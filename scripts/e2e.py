#!/usr/bin/env python3
"""Browser tests on a disposable stand: build, load the small data set, run Playwright.

Requires Python 3 stdlib and Docker Compose; the working .env is never used.
Arguments after the options go to `playwright test`, for example:

    python3 scripts/e2e.py                      # every test
    python3 scripts/e2e.py -- -g crawl          # only the role crawl
    python3 scripts/e2e.py -- --update-snapshots

The report, traces and failure screenshots land in var/e2e/ (outside Git).
"""
import argparse
import base64
import os
from pathlib import Path
import secrets
import shutil
import subprocess
import sys

ROOT = Path(__file__).resolve().parent.parent
OUT = ROOT / 'var' / 'e2e'


def environment():
    env = os.environ.copy()
    for variable in ('COMPOSE_FILE', 'COMPOSE_PROFILES', 'COMPOSE_PATH_SEPARATOR'):
        env.pop(variable, None)
    env.update(COMPOSE_PROJECT_NAME='dogclub_e2e_' + secrets.token_hex(4), POSTGRES_USER='dogclub', POSTGRES_DB='dogclub',
               POSTGRES_PASSWORD=secrets.token_hex(24), AUTH_JWT_KEY=base64.b64encode(secrets.token_bytes(32)).decode(),
               API_PORT='0', POSTGRES_PORT='0', MAILPIT_SMTP_PORT='0', MAILPIT_HTTP_PORT='0', WEB_PORT='0',
               DOGCLUB_IMAGE='dogclub-backend:e2e', DOGCLUB_WEB_IMAGE='dogclub-web:e2e',
               COMPOSE_ENV_FILES='/dev/null', COMPOSE_DISABLE_ENV_FILE='true')
    return env


def run(args, env, check=True):
    print('$ ' + ' '.join(args[:6]) + (' …' if len(args) > 6 else ''), flush=True)
    result = subprocess.run(args, cwd=ROOT, env=env)
    if check and result.returncode:
        raise SystemExit(f'{args[0]} {args[1] if len(args) > 1 else ""} failed with code {result.returncode}')
    return result.returncode


def main():
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument('--keep', action='store_true', help='leave the stand running for a look after the tests')
    parser.add_argument('playwright', nargs=argparse.REMAINDER, help='arguments for playwright test, after --')
    args = parser.parse_args()
    extra = [a for a in args.playwright if a != '--']
    env = environment()
    project = env['COMPOSE_PROJECT_NAME']
    shutil.rmtree(OUT / 'results', ignore_errors=True)
    shutil.rmtree(OUT / 'report', ignore_errors=True)
    OUT.mkdir(parents=True, exist_ok=True)
    code = 1
    try:
        run(['docker', 'compose', 'up', '--build', '-d', '--wait', 'web', 'mailpit'], env)
        run([sys.executable, 'scripts/club-data.py', '--scale', 'small', '--out', str(OUT / 'accounts.txt')], env)
        code = run(['docker', 'compose', '--profile', 'tools', 'run', '--rm', '--build', 'e2e', *extra], env, check=False)
    finally:
        if args.keep:
            print(f'The stand is kept: COMPOSE_PROJECT_NAME={project} docker compose ps', flush=True)
        else:
            run(['docker', 'compose', '--profile', 'tools', 'down', '--volumes', '--remove-orphans'], env, check=False)
    print(f'Report: {OUT / "report" / "index.html"}', flush=True)
    raise SystemExit(code)


if __name__ == '__main__':
    main()
