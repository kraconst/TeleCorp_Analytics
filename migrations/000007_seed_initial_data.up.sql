-- Seed initial company
INSERT INTO companies (id, name, timezone, no_reply_threshold)
VALUES (1, 'Default Company', 'Europe/Moscow', 7200000000000)
ON CONFLICT (id) DO NOTHING;

SELECT setval('companies_id_seq', GREATEST(1, (SELECT MAX(id) FROM companies)));

-- Seed initial employee
INSERT INTO employees (id, company_id, name, position, is_active)
VALUES (1, 1, 'Default Employee', 'Admin', TRUE)
ON CONFLICT (id) DO NOTHING;

SELECT setval('employees_id_seq', GREATEST(1, (SELECT MAX(id) FROM employees)));

