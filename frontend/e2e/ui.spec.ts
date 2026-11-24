import { expect, test, type Page, type Route } from '@playwright/test';

function fulfill(route: Route, data: unknown, status = 200) {
  return route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(data) });
}

async function mockBackend(page: Page, options: { secondAlert?: boolean; unknownChatStatus?: boolean } = {}) {
  const requestedMessageOffsets: number[] = [];
  const accountRequests: number[] = [];
  const accounts = [
    { id: 1, company_id: 1, employee_id: 1, phone: '+79991112233', username: '@sales_anna', employee_name: 'Анна Ковалёва', telegram_user_id: 11, status: 'active', last_activity_at: '2026-11-25T10:00:00Z', created_at: '2026-09-01T10:00:00Z' },
    { id: 2, company_id: 1, employee_id: 2, phone: '+79994445566', username: '@support_oleg', employee_name: 'Олег Соколов', telegram_user_id: 12, status: 'active', last_activity_at: null, created_at: '2026-09-02T10:00:00Z' },
  ];
  const employees = [
    { id: 1, company_id: 1, name: 'Анна Ковалёва', position: 'Старший менеджер', is_active: true },
    { id: 2, company_id: 1, name: 'Олег Соколов', position: 'Менеджер поддержки', is_active: true },
  ];
  const contacts = [
    { id: 1, company_id: 1, telegram_user_id: 101, name: 'Алексей Смирнов', username: 'asmirnov' },
    { id: 2, company_id: 1, telegram_user_id: 102, name: 'Марина Белова', username: 'marina_b' },
  ];
  const chats = [
    { id: 1, account_id: 1, contact_id: 1, status: 'action_required', status_source: 'auto', priority: 'high', last_message_at: '2026-11-25T14:02:00Z', awaiting_reply_since: new Date(Date.now() - 3 * 3600_000).toISOString(), unread_count: 3 },
    { id: 2, account_id: 2, contact_id: 2, status: 'negotiation', status_source: 'manual', priority: 'normal', last_message_at: '2026-11-25T12:45:00Z', awaiting_reply_since: null, unread_count: 0 },
  ];
  const summary = { chat_id: 1, topic: 'КП и счёт с НДС на 15 лицензий', agreements: 'Согласована скидка 10% при оплате до пятницы', next_step: 'Выставить счёт и отправить PDF в чат', open_questions: 'Проверить реквизиты юридического лица', risk: 'high', last_message_id: 120, generated_at: '2026-11-25T14:05:00Z' };
  if (options.unknownChatStatus) chats[0].status = 'unexpected-db-value';
  const messages = Array.from({ length: 120 }, (_, index) => {
    const id = index + 1;
    return { id, chat_id: 1, telegram_message_id: id, direction: id % 2 ? 'in' : 'out', text: id === 120 ? 'Когда пришлёте счёт с НДС?' : `Сообщение ${id}`, media_type: null, sent_at: new Date(Date.UTC(2026, 10, 20, 9) + id * 60_000).toISOString() };
  });
  let alertResolved = false;
  let createdAccountPolls = 0;

  await page.route('**/api/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace(/^\/api/, '');
    if (path === '/accounts' && request.method() === 'GET') {
      accountRequests.push(Date.now());
      return fulfill(route, accounts);
    }
    if (path === '/accounts' && request.method() === 'POST') {
      const body = request.postDataJSON() as { employee_id: number; phone: string; username?: string };
      const employee = employees.find((item) => item.id === body.employee_id)!;
      const account = { id: 3, company_id: 1, employee_id: body.employee_id, phone: body.phone, username: body.username ?? '', employee_name: employee.name, telegram_user_id: 0, status: 'connecting', last_activity_at: null, created_at: new Date().toISOString() };
      accounts.push(account);
      return fulfill(route, account, 201);
    }
    if (path === '/accounts/3' && request.method() === 'GET') {
      createdAccountPolls += 1;
      const account = accounts.find((item) => item.id === 3)!;
      if (createdAccountPolls > 1) account.status = 'active';
      return fulfill(route, account);
    }
    const assignMatch = path.match(/^\/accounts\/(\d+)\/assign$/);
    if (assignMatch && request.method() === 'POST') {
      const account = accounts.find((item) => item.id === Number(assignMatch[1]))!;
      const body = request.postDataJSON() as { employee_id: number };
      account.employee_id = body.employee_id;
      account.employee_name = employees.find((item) => item.id === body.employee_id)!.name;
      return fulfill(route, { status: 'assigned' });
    }
    if (path === '/employees/1/deactivate' && request.method() === 'POST') {
      employees[0].is_active = false;
      return fulfill(route, { status: 'deactivated' });
    }
    if (path === '/employees') return fulfill(route, employees);
    if (path === '/contacts') return fulfill(route, contacts);
    if (path === '/companies/1') return fulfill(route, { id: 1, name: 'ООО «Север»', timezone: 'Europe/Moscow', no_reply_threshold: 7_200_000_000_000, created_at: '2026-09-01T00:00:00Z' });
    if (path === '/analytics/dashboard') return fulfill(route, { messages_in: 1420, messages_out: 1180, active_chats: 85, new_contacts: 34, avg_response_time: 240_000_000_000, unanswered_chats: 1 });
    if (path === '/analytics/accounts') return fulfill(route, [{ account_id: 1, username: '@sales_anna', employee_name: 'Анна Ковалёва', messages_in: 850, messages_out: 720, total_messages: 1570, active_chats: 52, new_contacts: 21, avg_response_time: 180_000_000_000 }]);
    if (path === '/alerts') {
      const activeAlerts = [{ id: 1, company_id: 1, chat_id: 1, type: 'no_reply', created_at: '2026-11-25T14:10:00Z', resolved_at: null }];
      if (options.secondAlert) activeAlerts.push({ id: 2, company_id: 1, chat_id: 2, type: 'complaint', created_at: '2026-11-25T14:12:00Z', resolved_at: null });
      return fulfill(route, alertResolved ? activeAlerts.filter((alert) => alert.id !== 1) : activeAlerts);
    }
    if (path === '/alerts/1/resolve' && request.method() === 'POST') {
      alertResolved = true;
      return fulfill(route, { status: 'resolved' });
    }
    if (path === '/chats/1/status' && request.method() === 'POST') {
      const body = request.postDataJSON() as { status: string };
      chats[0].status = body.status;
      chats[0].status_source = 'manual';
      return fulfill(route, { status: 'updated' });
    }
    if (path === '/chats/1/messages') {
      const offset = Number(url.searchParams.get('offset') ?? 0);
      const limit = Number(url.searchParams.get('limit') ?? 50);
      requestedMessageOffsets.push(offset);
      return fulfill(route, messages.slice(offset, offset + limit));
    }
    if (path === '/chats/1/analytics') return fulfill(route, { chat_id: 1, total_messages: 120, messages_in: 60, messages_out: 60, media_messages: 0, response_count: 60, avg_response_seconds: 185.5, max_response_seconds: 2280, unanswered_messages: 1, awaiting_reply_since: chats[0].awaiting_reply_since, first_message_at: messages[0].sent_at, last_message_at: messages[119].sent_at, daily_activity: [{ day: '2026-11-20', messages_in: 60, messages_out: 60 }] });
    if (path === '/chats/2/analytics') return fulfill(route, { chat_id: 2, total_messages: 0, messages_in: 0, messages_out: 0, media_messages: 0, response_count: 0, avg_response_seconds: null, max_response_seconds: null, unanswered_messages: 0, awaiting_reply_since: null, first_message_at: null, last_message_at: null, daily_activity: [] });
    if (path === '/chats/1') return fulfill(route, { ...chats[0], contact: contacts[0], summary });
    if (path === '/chats/2') return fulfill(route, { ...chats[1], contact: contacts[1] });
    if (path === '/chats') {
      const status = url.searchParams.get('status');
      const employeeId = Number(url.searchParams.get('employee_id') ?? 0);
      const filtered = chats.filter((chat) => (!status || chat.status === status) && (!employeeId || accounts.find((account) => account.id === chat.account_id)?.employee_id === employeeId));
      return fulfill(route, filtered);
    }
    return fulfill(route, { error: `Not mocked: ${request.method()} ${path}` }, 404);
  });
  return { requestedMessageOffsets, chats, accountRequests };
}

test('deep link loads latest history and persists manual status after reload', async ({ page }, testInfo) => {
  const backend = await mockBackend(page);
  await page.goto('/chats?view=split&status=action_required&chat_id=1');
  await expect(page.getByRole('heading', { name: 'Алексей Смирнов', exact: true })).toBeVisible();
  await expect(page.getByText('Когда пришлёте счёт с НДС?', { exact: true })).toBeVisible();
  await expect(page.getByText('КП и счёт с НДС на 15 лицензий', { exact: true })).toBeVisible();
  expect(backend.requestedMessageOffsets[0]).toBe(70);
  await page.screenshot({ path: testInfo.outputPath('chats-desktop.png'), fullPage: true });
  await page.getByLabel('Изменить статус чата').selectOption('interested');
  await expect(page.getByText('Статус чата обновлён', { exact: true })).toBeVisible();
  await page.reload();
  await expect(page.getByLabel('Изменить статус чата')).toHaveValue('interested');
  await expect(page.getByText('Вне фильтра', { exact: true })).toBeVisible();
});

test('older history decreases offset and keeps existing messages', async ({ page }) => {
  const backend = await mockBackend(page);
  await page.goto('/chats?view=split&chat_id=1');
  await expect(page.getByText('Когда пришлёте счёт с НДС?', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Более ранние сообщения' }).click();
  await expect(page.getByText('Сообщение 21', { exact: true })).toBeVisible();
  await expect(page.getByText('Когда пришлёте счёт с НДС?', { exact: true })).toBeVisible();
  expect(backend.requestedMessageOffsets.slice(0, 2)).toEqual([70, 20]);
});

test('alert resolution updates active queue and session archive', async ({ page }) => {
  await mockBackend(page);
  await page.goto('/alerts');
  await expect(page.getByRole('heading', { name: 'Алексей Смирнов', exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Обработано' }).click();
  await expect(page.getByRole('heading', { name: 'Активных алертов нет' })).toBeVisible();
  await page.getByRole('button', { name: /Обработанные в сессии/ }).click();
  await expect(page.getByRole('heading', { name: 'Алексей Смирнов', exact: true })).toBeVisible();
  await expect(page.getByText('Обработан', { exact: true })).toBeVisible();
});

test('dashboard overdue KPI opens the matching chat filter', async ({ page }) => {
  await mockBackend(page);
  await page.goto('/dashboard?from=2026-11-01&to=2026-11-25');
  await expect(page.getByRole('heading', { name: 'Состояние отдела', exact: true, level: 2 })).toBeVisible();
  await page.getByRole('link', { name: /Чаты без ответа/ }).click();
  await expect(page).toHaveURL(/\/chats\?unanswered=true/);
  await expect(page.getByRole('heading', { name: 'Алексей Смирнов', exact: true })).toBeVisible();
});

test('mobile keeps list, message stream and details drawer usable', async ({ page }, testInfo) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await mockBackend(page);
  await page.goto('/chats');
  await expect(page.getByPlaceholder('Поиск по клиенту')).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  await page.screenshot({ path: testInfo.outputPath('chats-mobile-list.png'), fullPage: true });
  await page.getByRole('button', { name: /Алексей Смирнов/ }).first().click();
  await expect(page.getByText('Когда пришлёте счёт с НДС?', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Открыть детали чата' }).click();
  await expect(page.getByRole('dialog')).toBeVisible();
  await expect(page.getByRole('dialog').getByText('КП и счёт с НДС на 15 лицензий', { exact: true })).toBeVisible();
});


test('kanban drag and drop moves a card to the exact target status', async ({ page }) => {
  await page.setViewportSize({ width: 1600, height: 900 });
  const backend = await mockBackend(page);
  await page.goto('/chats?view=kanban');
  const handle = page.getByRole('button', { name: 'Перетащить чат с Алексей Смирнов' });
  await expect(handle).toBeVisible();
  const source = await handle.boundingBox();
  const target = await page.getByRole('region', { name: 'Ждёт ответа', exact: true }).getByText('Перетащите карточку сюда').boundingBox();
  expect(source).not.toBeNull();
  expect(target).not.toBeNull();
  await page.mouse.move(source!.x + source!.width / 2, source!.y + source!.height / 2);
  await page.mouse.down();
  await page.mouse.move(target!.x + target!.width / 2, target!.y + target!.height / 2, { steps: 20 });
  await page.mouse.up();
  await expect(page.getByText('Статус чата обновлён', { exact: true })).toBeVisible();
  expect(backend.chats[0].status).toBe('waiting_reply');
  await expect(page.getByRole('region', { name: 'Ждёт ответа', exact: true }).getByText('Алексей Смирнов', { exact: true })).toBeVisible();
});

test('account creation polls until the server reports active', async ({ page }) => {
  await mockBackend(page);
  await page.goto('/accounts');
  await page.getByRole('button', { name: 'Добавить аккаунт' }).click();
  await page.getByLabel('Номер телефона').fill('+79990001122');
  await page.getByLabel(/Username/).fill('@sales_new');
  await page.getByLabel('Ответственный').selectOption('2');
  await page.getByRole('button', { name: 'Создать аккаунт' }).click();
  await expect(page.getByRole('heading', { name: 'Ожидает серверного подключения' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Аккаунт активен' })).toBeVisible({ timeout: 10_000 });
});

test('employee deactivation reassigns linked accounts first', async ({ page }) => {
  const backend = await mockBackend(page);
  await page.goto('/employees');
  const row = page.locator('li').filter({ hasText: 'Анна Ковалёва' });
  const initialAccountRequests = backend.accountRequests.length;
  await row.getByRole('button', { name: 'Деактивировать' }).click();
  await expect(page.getByRole('dialog')).toBeVisible();
  expect(backend.accountRequests.length).toBeGreaterThan(initialAccountRequests);
  await expect(page.getByText('@sales_anna', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Передать и деактивировать' }).click();
  await expect(page.getByText('Сотрудник деактивирован; история сохранена', { exact: true })).toBeVisible();
  await expect(page.getByRole('dialog')).not.toBeVisible();
  await expect(page.locator('li').filter({ hasText: 'Анна Ковалёва' })).toHaveCount(0);
});

test('manager deep link to dashboard returns to own chats', async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem('telecorp.dev-session.v1', JSON.stringify({ companyId: 1, role: 'manager', employeeId: 2 })));
  await mockBackend(page, { secondAlert: true });
  await page.goto('/dashboard');
  await expect(page).toHaveURL(/\/chats$/);
  await expect(page.getByLabel('Роль Dev Session')).toHaveValue('manager');
  await expect(page.getByRole('heading', { name: 'Марина Белова', exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: /Алексей Смирнов/ })).toHaveCount(0);
  await expect(page.getByLabel('Фильтр по аккаунту').locator('option')).toHaveText(['Все аккаунты', '@support_oleg']);
  const alertNavigation = page.getByRole('link', { name: /Алерты/ });
  await expect(alertNavigation).toContainText('1');
  await alertNavigation.click();
  await expect(page.getByRole('button', { name: /Активные\s+1/ })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Марина Белова', exact: true })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Алексей Смирнов', exact: true })).toHaveCount(0);
});


test('debounced search keeps keyboard focus and updates the URL', async ({ page }) => {
  await mockBackend(page);
  await page.goto('/chats');
  const search = page.getByPlaceholder('Поиск по клиенту');
  await search.focus();
  await search.fill('ива');
  await page.waitForTimeout(350);
  await expect(search).toBeFocused();
  await search.type('н');
  await page.waitForTimeout(350);
  await expect(search).toBeFocused();
  await expect(search).toHaveValue('иван');
  await expect(page).toHaveURL(/q=%D0%B8%D0%B2%D0%B0%D0%BD/);
});

test('unknown enum values render neutral labels without crashing the workspace', async ({ page }) => {
  await mockBackend(page, { unknownChatStatus: true });
  await page.goto('/chats');
  await expect(page.getByText('Неизвестный статус', { exact: true }).first()).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Алексей Смирнов', exact: true })).toBeVisible();
  await expect(page.getByText('Когда пришлёте счёт с НДС?', { exact: true })).toBeVisible();
});
