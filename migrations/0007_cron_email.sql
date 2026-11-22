-- Where an account's cron output is mailed; empty discards it.
ALTER TABLE accounts ADD COLUMN cron_email TEXT NOT NULL DEFAULT '';
