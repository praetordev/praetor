INSERT INTO organizations (id, name, description)
VALUES (1, 'Default', 'Default Organization')
ON CONFLICT (id) DO NOTHING;

-- Reset sequence to ensure future inserts don't collide if id=1 was manually forced
SELECT setval('organizations_id_seq', (SELECT MAX(id) FROM organizations));
