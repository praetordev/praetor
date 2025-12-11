-- Add content field to inventories for raw INI-format inventory
ALTER TABLE inventories ADD COLUMN IF NOT EXISTS content TEXT;

-- Comment: content stores the Ansible inventory in INI format
-- Example:
-- [webservers]
-- web1 ansible_host=192.168.1.10 ansible_user=root
-- web2 ansible_host=192.168.1.11 ansible_user=root
