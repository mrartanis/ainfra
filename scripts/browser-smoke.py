"""Opt-in local UI smoke. Requires playwright; never prints credentials."""
import os
import time
import uuid
import urllib.request
from pathlib import Path
from playwright.sync_api import sync_playwright
url = os.environ.get('INFRA_UI_URL', 'http://127.0.0.1:18081')
token_file = Path(os.environ.get('INFRA_ADMIN_TOKEN_FILE', '.local/admin.token'))
api = os.environ.get('INFRA_API_URL', 'http://127.0.0.1:18080')
for attempt in range(100):
    try:
        with urllib.request.urlopen(api + '/readyz', timeout=1) as response:
            if response.status == 200:
                break
    except OSError:
        pass
    time.sleep(.1)
else:
    raise RuntimeError('API did not become ready')

with sync_playwright() as p:
    browser = p.chromium.launch(headless=True)
    page = browser.new_page(viewport={'width': 1440, 'height': 900})
    page.goto(url + '/login')
    page.get_by_label('Токен доступа').fill(token_file.read_text().strip())
    page.get_by_role('button', name='Войти', exact=True).click()
    page.wait_for_url(url + '/')
    assert page.get_by_role('heading', name='Узлы', exact=True).is_visible()
    prefix = 'browser-' + uuid.uuid4().hex[:8]
    nodes = {}
    for name, kind in [('host', 'physical'), ('vm', 'vm'), ('router', 'router'), ('backup', 'physical')]:
        page.goto(url + '/nodes/new')
        page.get_by_label('Имя', exact=True).fill(prefix + '-' + name)
        page.locator('select[name=kind]').select_option(kind)
        page.get_by_label('Назначение', exact=True).fill('Browser acceptance fixture')
        page.get_by_label('Hostname', exact=True).fill(prefix + '-' + name + '.invalid')
        page.get_by_label('Адреса через запятую').fill('192.0.2.15')
        page.get_by_role('button', name='Сохранить', exact=True).click()
        assert page.get_by_role('heading', name=prefix + '-' + name, exact=True).is_visible()
        nodes[name] = page.url.rsplit('/', 1)[1]
    for target, relation in [('host', 'runs_on'), ('router', 'uses_gateway'), ('backup', 'backs_up_to')]:
        page.goto(url + '/nodes/' + nodes['vm'])
        page.get_by_text('Добавить связь', exact=True).click()
        form = page.locator('form[action="/relations/save"]')
        form.locator('select[name=type]').select_option(relation)
        form.locator('select[name=target]').select_option(nodes[target])
        form.get_by_role('button', name='Добавить', exact=True).click()
        page.goto(url + '/nodes/' + nodes[target])
        assert page.locator('a[href="/nodes/' + nodes['vm'] + '"]').count() > 0
    page.goto(url + '/nodes/' + nodes['vm'])
    page.get_by_text('Добавить сервис', exact=True).click()
    form = page.locator('form[action="/services/save"]')
    form.get_by_label('Название', exact=True).fill(prefix + ' service')
    form.get_by_label('URL', exact=True).fill('https://example.invalid')
    form.get_by_role('button', name='Добавить', exact=True).click()
    page.get_by_text('Изменить сервис', exact=True).click()
    form = page.locator('form[action="/services/save"]:has(input[name="index"])')
    form.get_by_label('Название', exact=True).fill(prefix + ' edited service')
    form.get_by_role('button', name='Сохранить сервис', exact=True).click()
    assert page.get_by_text(prefix + ' edited service', exact=True).is_visible()
    page.get_by_text('Изменить сервис', exact=True).click()
    page.get_by_role('button', name='Удалить сервис', exact=True).click()
    assert page.get_by_text(prefix + ' edited service', exact=True).count() == 0
    page.get_by_text('Бэкапы и оплата', exact=True).click()
    form = page.locator('form[action="/backups/save"]')
    form.get_by_label('ID проверки', exact=True).fill(prefix + '-backup')
    form.get_by_label('Название бэкапа', exact=True).fill(prefix + ' backup')
    form.get_by_role('button', name='Добавить бэкап', exact=True).click()
    page.get_by_text('Бэкапы и оплата', exact=True).click()
    page.get_by_text('Изменить описание бэкапа', exact=True).click()
    form = page.locator('form[action="/backups/save"]:has(input[name="index"])')
    form.get_by_label('Название бэкапа', exact=True).fill(prefix + ' edited backup')
    form.get_by_label('Ожидаемый интервал, часы', exact=True).fill('48')
    form.get_by_role('button', name='Сохранить описание бэкапа', exact=True).click()
    page.get_by_text('Бэкапы и оплата', exact=True).click()
    assert page.get_by_text(prefix + ' edited backup', exact=True).is_visible()
    page.get_by_text('Изменить описание бэкапа', exact=True).click()
    page.get_by_role('button', name='Удалить описание бэкапа', exact=True).click()
    assert page.get_by_text(prefix + ' edited backup', exact=True).count() == 0
    forms = page.locator('form[action="/relations/delete"]')
    assert forms.count() == 3
    forms.first.get_by_role('button', name='Удалить связь', exact=True).click()
    assert page.locator('form[action="/relations/delete"]').count() == 2
    page.goto(url + '/docs/new')
    page.get_by_label('Заголовок').fill(prefix + ' shared runbook')
    page.get_by_label('Связанные узлы (UUID через запятую)').fill(nodes['vm'] + ',' + nodes['host'])
    page.get_by_label('Markdown', exact=True).fill('# Preview heading\n\n**Evidence** ' + prefix)
    page.get_by_role('button', name='Предпросмотр', exact=True).click()
    assert page.locator('.markdown h1').inner_text() == 'Preview heading'
    page.get_by_role('button', name='Сохранить новую ревизию').click()
    document_url = page.url
    for name in ['vm', 'host']:
        page.goto(url + '/nodes/' + nodes[name] + '?tab=records')
        link = page.get_by_role('link', name=prefix + ' shared runbook')
        assert link.count() == 1 and url + link.get_attribute('href') == document_url
    page.goto(url + '/nodes/' + nodes['vm'] + '?edit=true')
    page.get_by_label('Назначение', exact=True).fill(prefix + ' edited purpose')
    page.get_by_role('button', name='Сохранить', exact=True).click()
    page.reload()
    assert page.get_by_text(prefix + ' edited purpose', exact=False).count() > 0
    page.goto(url + '/search?q=' + prefix)
    assert page.get_by_role('link', name=prefix + ' shared runbook').count() == 1
    page.goto(url + '/')
    Path('.local/screenshots').mkdir(parents=True, exist_ok=True)
    page.screenshot(path='.local/screenshots/nodes-desktop.png', full_page=True)
    page.get_by_role('link', name='local-ubuntu', exact=True).click()
    assert page.get_by_role('heading', name='local-ubuntu', exact=True).is_visible()
    page.get_by_role('link', name='Записи', exact=True).click()
    page.get_by_role('link', name='+ Запись', exact=True).click()
    page.get_by_label('Заголовок').fill('Browser smoke: инструкция')
    page.get_by_label('Markdown', exact=True).fill('# Проверка\n\nТекст <script>alert(1)</script> остаётся данными.')
    page.get_by_role('button', name='Сохранить новую ревизию').click()
    assert page.get_by_role('heading', name='Browser smoke: инструкция').is_visible()
    assert page.locator('script').count() == 0
    page.goto(url + '/')
    page.set_viewport_size({'width': 390, 'height': 844})
    assert page.evaluate('document.documentElement.scrollWidth <= innerWidth')
    page.screenshot(path='.local/screenshots/nodes-mobile.png', full_page=True)
    page.get_by_role('link', name='local-ubuntu', exact=True).click()
    assert page.evaluate('document.documentElement.scrollWidth <= innerWidth')
    page.screenshot(path='.local/screenshots/node-mobile.png', full_page=True)
    page.goto(url + '/settings')
    assert page.get_by_role('heading', name='Хранение', exact=True).is_visible()
    browser.close()
print('Browser smoke passed: login, four node types, edit/reload, bidirectional relations and deletion, service/backup editing and deletion, shared document, Markdown preview, search, escaping, mobile widths, settings')
